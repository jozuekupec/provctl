package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

// BackupStore provides the read-only state needed before archive operations.
type BackupStore interface {
	SubscriptionExists(context.Context, string) (bool, error)
	SubscriptionUIDExists(context.Context, int) (bool, error)
	ListSubscriptions(context.Context) ([]domain.Subscription, error)
	SubscriptionByName(context.Context, string) (domain.Subscription, error)
	CreateSubscription(context.Context, domain.Subscription) error
	DeleteSubscription(context.Context, string) error
	SetSubscriptionStatus(context.Context, int64, string) error
	ListBackups(context.Context, int64) ([]domain.Backup, error)
	BackupByID(context.Context, int64, int64) (domain.Backup, error)
	BackupByIDAny(context.Context, int64) (domain.Backup, error)
	CreateBackup(context.Context, domain.Backup) (int64, error)
	FinishBackup(context.Context, int64, int64, string) error
	ListDatabases(context.Context, int64) ([]domain.Database, error)
	ListWebsites(context.Context, int64) ([]domain.Website, error)
	ListCronJobs(context.Context, int64) ([]domain.CronJob, error)
	ListSSHKeys(context.Context, int64) ([]domain.SSHKey, error)
	ListCertificates(context.Context, int64) ([]domain.Certificate, error)
	CreateDatabase(context.Context, domain.Database) error
	DeleteDatabase(context.Context, int64, string) error
}

type BackupService struct {
	Store     BackupStore
	FS        system.FS
	Config    config.Config
	Commands  system.Commander
	Users     system.Users
	MariaDB   MariaDBExecutor
	Executor  plan.Executor
	Locker    system.Locker
	LockerFor func(string) system.Locker
}

// Restore restores a verified file archive onto a clean server. Existing
// subscriptions are deliberately refused until overwrite recovery can first
// make a new backup of their current state.
type RestoreResult struct {
	OperationID       int64
	DatabasePasswords map[string]string
}

func (service BackupService) Restore(ctx context.Context, name string, id int64, force bool) (RestoreResult, error) {
	metadata, err := service.PrepareRestore(ctx, name, id)
	if err != nil {
		return RestoreResult{}, err
	}
	if force {
		return RestoreResult{}, fmt.Errorf("restore --force is not available until current-state backup is implemented")
	}
	if service.Users == nil || service.Executor.Journal == nil || service.Executor.Locker == nil {
		return RestoreResult{}, fmt.Errorf("restore user manager and executor are required")
	}
	if len(metadata.Databases) > 0 && service.MariaDB == nil {
		return RestoreResult{}, fmt.Errorf("restore MariaDB executor is required for database payloads")
	}
	if len(metadata.Databases) > 0 && !service.Config.MariaDB.Enabled {
		return RestoreResult{}, fmt.Errorf("MariaDB is disabled in configuration")
	}
	exists, err := service.Store.SubscriptionExists(ctx, name)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("check restore subscription: %w", err)
	}
	if exists {
		return RestoreResult{}, fmt.Errorf("subscription %q already exists; restore requires --force after a current-state backup", name)
	}
	target := filepath.Join(service.Config.Paths.VHosts, name)
	if _, err := service.FS.Stat(target); err == nil {
		return RestoreResult{}, fmt.Errorf("restore target %q already exists", target)
	} else if !os.IsNotExist(err) {
		return RestoreResult{}, fmt.Errorf("inspect restore target: %w", err)
	}
	uid, err := service.nextRestoreUID(ctx, name)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("choose restore UID: %w", err)
	}
	subscription := metadata.Subscription
	subscription.ID, subscription.Name, subscription.UnixUser, subscription.UnixUID, subscription.Home = 0, name, name, uid, target
	backup, err := service.Store.BackupByIDAny(ctx, id)
	if err != nil {
		return RestoreResult{}, err
	}
	staging := filepath.Join(service.Config.Paths.VHosts, ".restore-"+name+"-"+strconv.FormatInt(time.Now().UTC().UnixNano(), 10))
	passwords, err := service.restoreDatabasePasswords(metadata.Databases)
	if err != nil {
		return RestoreResult{}, err
	}
	operationID, err := service.restoreFiles(ctx, backup.Path, subscription, staging, metadata.Databases, passwords)
	if err != nil {
		return RestoreResult{OperationID: operationID}, err
	}
	return RestoreResult{OperationID: operationID, DatabasePasswords: passwords}, nil
}

func (service BackupService) restoreDatabasePasswords(databases []domain.Database) (map[string]string, error) {
	passwords := make(map[string]string, len(databases))
	for _, database := range databases {
		if err := domain.ValidateDatabaseName(database.Name); err != nil {
			return nil, fmt.Errorf("validate backup database %q: %w", database.Name, err)
		}
		password, err := GenerateDatabasePassword(24)
		if err != nil {
			return nil, err
		}
		passwords[database.Name] = password
	}
	return passwords, nil
}

func (service BackupService) nextRestoreUID(ctx context.Context, name string) (int, error) {
	if _, err := service.Users.Lookup(name); err == nil {
		return 0, fmt.Errorf("unix user %q already exists", name)
	} else if !isUnknownUser(err) {
		return 0, fmt.Errorf("look up unix user %q: %w", name, err)
	}
	for uid := service.Config.Users.UIDMin; uid <= service.Config.Users.UIDMax; uid++ {
		reserved, err := service.Store.SubscriptionUIDExists(ctx, uid)
		if err != nil {
			return 0, fmt.Errorf("check subscription UID %d: %w", uid, err)
		}
		if reserved {
			continue
		}
		if _, err := service.Users.LookupID(strconv.Itoa(uid)); err == nil {
			continue
		} else if !isUnknownUser(err) {
			return 0, fmt.Errorf("look up UID %d: %w", uid, err)
		}
		return uid, nil
	}
	return 0, fmt.Errorf("no free UID in configured range")
}

func (service BackupService) restoreFiles(ctx context.Context, archivePath string, subscription domain.Subscription, staging string, databases []domain.Database, passwords map[string]string) (int64, error) {
	archive := filepath.Join(archivePath, "files.tar.zst")
	steps := []plan.Step{
		{Name: "extract backup archive", Preview: "extract " + archive + " to staging", Do: func(ctx context.Context) error {
			return service.extractArchive(ctx, archive, staging)
		}, Undo: func(context.Context) error { return service.FS.RemoveAll(staging) }},
		{Name: "create locked Unix user", Preview: fmt.Sprintf("create locked user %s with UID %d", subscription.UnixUser, subscription.UnixUID), Do: func(ctx context.Context) error {
			if err := service.Users.Create(ctx, system.CreateUserOptions{Name: subscription.UnixUser, UID: subscription.UnixUID, Home: subscription.Home, Shell: meta.NoLoginShell, UserGroup: true, NoCreateHome: true}); err != nil {
				return err
			}
			if err := service.Users.LockPassword(ctx, subscription.UnixUser); err != nil {
				_ = service.Users.Delete(ctx, subscription.UnixUser, false)
				return err
			}
			return nil
		}, Undo: func(ctx context.Context) error { return service.Users.Delete(ctx, subscription.UnixUser, false) }},
		{Name: "promote restored files", Preview: "atomically move staged files to " + subscription.Home, Do: func(context.Context) error {
			return service.promoteStaging(staging, subscription.Home)
		}, Undo: func(context.Context) error { return service.FS.RemoveAll(subscription.Home) }},
		{Name: "restore file ownership", Preview: fmt.Sprintf("chown restored files to %d:%d", subscription.UnixUID, subscription.UnixUID), Do: func(ctx context.Context) error {
			_, err := service.Commands.Run(ctx, "/usr/bin/chown", "-R", "--", fmt.Sprintf("%d:%d", subscription.UnixUID, subscription.UnixUID), subscription.Home)
			return err
		}},
		{Name: "create restored runtime directories", Preview: "create private restore runtime directories", Do: func(context.Context) error {
			for _, directory := range []string{filepath.Join(subscription.Home, "tmp"), filepath.Join(subscription.Home, "tmp", "sessions")} {
				if err := service.FS.MkdirAll(directory, 0o700); err != nil {
					return err
				}
				if err := service.FS.Chown(directory, subscription.UnixUID, subscription.UnixUID); err != nil {
					return err
				}
				if err := service.FS.Chmod(directory, 0o700); err != nil {
					return err
				}
			}
			return nil
		}},
		{Name: "record restored subscription", Preview: "insert subscription into SQLite", Do: func(ctx context.Context) error {
			return service.Store.CreateSubscription(ctx, subscription)
		}, Undo: func(ctx context.Context) error { return service.Store.DeleteSubscription(ctx, subscription.Name) }},
	}
	for _, database := range databases {
		database := database
		password := passwords[database.Name]
		steps = append(steps, service.restoreDatabaseSteps(archivePath, subscription, database, password)...)
	}
	return service.Executor.Run(ctx, plan.Plan{Action: "backup.restore", Target: subscription.Name, Steps: steps})
}

func (service BackupService) restoreDatabaseSteps(archivePath string, subscription domain.Subscription, database domain.Database, password string) []plan.Step {
	dump := filepath.Join(archivePath, "db", database.Name+".sql.zst")
	raw := filepath.Join(subscription.Home, "tmp", ".restore-"+database.Name+".sql")
	return []plan.Step{{Name: "create restored MariaDB database", Preview: "create database " + database.Name + " with a new password", Do: func(ctx context.Context) error {
		query, err := RestoreDatabaseSQL(database, password)
		if err != nil {
			return err
		}
		return service.MariaDB.Execute(ctx, query)
	}, Undo: func(ctx context.Context) error {
		query, err := DropSQL(database.Name, database.User)
		if err != nil {
			return err
		}
		return service.MariaDB.Execute(ctx, query)
	}}, {Name: "import restored MariaDB database", Preview: "import database dump " + database.Name, Do: func(ctx context.Context) error {
		defer service.FS.Remove(raw)
		output, ok := service.Commands.(system.OutputFileCommander)
		if !ok {
			return fmt.Errorf("restore commander cannot write decompressed database dumps")
		}
		input, ok := service.Commands.(system.InputFileCommander)
		if !ok {
			return fmt.Errorf("restore commander cannot read database dumps")
		}
		if _, err := output.RunToFile(ctx, raw, 0o600, "/usr/bin/zstd", "-q", "-d", "-c", dump); err != nil {
			return fmt.Errorf("decompress database dump %q: %w", database.Name, err)
		}
		arguments := []string{}
		if service.Config.MariaDB.DefaultsFile != "" {
			arguments = append(arguments, "--defaults-extra-file="+service.Config.MariaDB.DefaultsFile)
		}
		arguments = append(arguments, database.Name)
		if _, err := input.RunWithFile(ctx, raw, "/usr/bin/mysql", arguments...); err != nil {
			return fmt.Errorf("import database %q: %w", database.Name, err)
		}
		return nil
	}}, {Name: "record restored MariaDB database", Preview: "insert database " + database.Name + " into SQLite", Do: func(ctx context.Context) error {
		restored, err := service.Store.SubscriptionByName(ctx, subscription.Name)
		if err != nil {
			return err
		}
		database.ID, database.SubscriptionID = 0, restored.ID
		return service.Store.CreateDatabase(ctx, database)
	}, Undo: func(ctx context.Context) error {
		restored, err := service.Store.SubscriptionByName(ctx, subscription.Name)
		if err != nil {
			return err
		}
		return service.Store.DeleteDatabase(ctx, restored.ID, database.Name)
	}}}
}

func (service BackupService) Create(ctx context.Context, name string) (backupID int64, returnErr error) {
	if err := domain.ValidateSubscriptionName(name); err != nil {
		return 0, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, name)
	if err != nil {
		return 0, err
	}
	backups, err := service.Store.ListBackups(ctx, subscription.ID)
	if err != nil {
		return 0, fmt.Errorf("list backups: %w", err)
	}
	if subscription.QuotaBackups > 0 && len(backups) >= subscription.QuotaBackups {
		return 0, fmt.Errorf("subscription %q has reached its backup quota of %d", name, subscription.QuotaBackups)
	}
	if service.FS == nil || service.Commands == nil || (service.Locker == nil && service.LockerFor == nil) {
		return 0, fmt.Errorf("backup filesystem, commander, and locker are required")
	}
	locker := service.Locker
	if service.LockerFor != nil {
		locker = service.LockerFor(name)
	}
	unlock, err := locker.Lock(ctx, "backup.create "+name)
	if err != nil {
		return 0, err
	}
	defer unlock()
	started := time.Now().UTC()
	directory := filepath.Join(service.Config.Paths.Backups, name, started.Format("2006-01-02T15-04-05Z"))
	if err := service.FS.MkdirAll(directory, 0o700); err != nil {
		return 0, fmt.Errorf("create backup directory: %w", err)
	}
	backupID, err = service.Store.CreateBackup(ctx, domain.Backup{SubscriptionID: subscription.ID, Path: directory, Status: "running", StartedAt: started})
	if err != nil {
		return 0, err
	}
	finished := false
	defer func() {
		if !finished {
			_ = service.Store.FinishBackup(context.Background(), backupID, 0, "failed")
		}
	}()
	archive := filepath.Join(directory, "files.tar.zst")
	_, err = service.Commands.Run(ctx, "/usr/bin/tar", "--numeric-owner", "--acls", "--xattrs", "-p", "--exclude=tmp", "--exclude=*/storage/framework/cache/*", "-I", "/usr/bin/zstd", "-cf", archive, "-C", subscription.Home, ".")
	if err != nil {
		return backupID, fmt.Errorf("archive subscription files: %w", err)
	}
	databases, err := service.Store.ListDatabases(ctx, subscription.ID)
	if err != nil {
		return backupID, fmt.Errorf("list databases: %w", err)
	}
	checksums := []string{"files.tar.zst"}
	if len(databases) > 0 {
		output, ok := service.Commands.(system.OutputFileCommander)
		if !ok {
			return backupID, fmt.Errorf("backup commander cannot write database dumps")
		}
		if err := service.FS.MkdirAll(filepath.Join(directory, "db"), 0o700); err != nil {
			return backupID, err
		}
		for _, database := range databases {
			raw := filepath.Join(directory, "db", database.Name+".sql")
			args := []string{"--single-transaction", "--quick", "--routines", "--triggers", "--events", "--no-create-db"}
			if service.Config.MariaDB.DefaultsFile != "" {
				args = append(args, "--defaults-extra-file="+service.Config.MariaDB.DefaultsFile)
			}
			args = append(args, database.Name)
			if _, err := output.RunToFile(ctx, raw, 0o600, "/usr/bin/mysqldump", args...); err != nil {
				return backupID, fmt.Errorf("dump database %q: %w", database.Name, err)
			}
			compressed := raw + ".zst"
			if _, err := output.RunToFile(ctx, compressed, 0o600, "/usr/bin/zstd", "-q", "-c", raw); err != nil {
				return backupID, fmt.Errorf("compress database dump %q: %w", database.Name, err)
			}
			if err := service.FS.Remove(raw); err != nil {
				return backupID, fmt.Errorf("remove uncompressed database dump: %w", err)
			}
			checksums = append(checksums, filepath.Join("db", database.Name+".sql.zst"))
		}
	}
	websites, err := service.Store.ListWebsites(ctx, subscription.ID)
	if err != nil {
		return backupID, fmt.Errorf("list websites: %w", err)
	}
	cronJobs, err := service.Store.ListCronJobs(ctx, subscription.ID)
	if err != nil {
		return backupID, fmt.Errorf("list cron jobs: %w", err)
	}
	sshKeys, err := service.Store.ListSSHKeys(ctx, subscription.ID)
	if err != nil {
		return backupID, fmt.Errorf("list SSH keys: %w", err)
	}
	certificates, err := service.Store.ListCertificates(ctx, subscription.ID)
	if err != nil {
		return backupID, fmt.Errorf("list certificates: %w", err)
	}
	metadata, err := json.MarshalIndent(domain.BackupMetadata{FormatVersion: 1, ProvctlVersion: meta.Version, CreatedAt: started, Subscription: subscription, Websites: websites, Databases: databases, CronJobs: cronJobs, SSHKeys: sshKeys, Certificates: certificates}, "", "  ")
	if err != nil {
		return backupID, fmt.Errorf("encode backup metadata: %w", err)
	}
	if err := service.FS.WriteFileAtomic(filepath.Join(directory, "metadata.json"), append(metadata, '\n'), 0o600); err != nil {
		return backupID, err
	}
	checksums = append([]string{"metadata.json"}, checksums...)
	if err := service.writeChecksums(directory, checksums); err != nil {
		return backupID, err
	}
	info, err := service.FS.Stat(archive)
	if err != nil {
		return backupID, fmt.Errorf("stat backup archive: %w", err)
	}
	if err := service.Store.FinishBackup(ctx, backupID, info.Size(), "complete"); err != nil {
		return backupID, err
	}
	finished = true
	return backupID, nil
}

func (service BackupService) writeChecksums(directory string, names []string) error {
	lines := make([]string, 0, len(names))
	for _, name := range names {
		contents, err := service.FS.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("%x  %s", sha256.Sum256(contents), name))
	}
	return service.FS.WriteFileAtomic(filepath.Join(directory, "SHA256SUMS"), []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

func (service BackupService) ListForSubscription(ctx context.Context, name string) ([]domain.Backup, error) {
	if err := domain.ValidateSubscriptionName(name); err != nil {
		return nil, err
	}
	if service.Store == nil {
		return nil, fmt.Errorf("backup store is required")
	}
	subscription, err := service.Store.SubscriptionByName(ctx, name)
	if err != nil {
		return nil, err
	}
	backups, err := service.Store.ListBackups(ctx, subscription.ID)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	return backups, nil
}

func (service BackupService) Inspect(ctx context.Context, name string, id int64) (domain.BackupMetadata, error) {
	if id < 1 {
		return domain.BackupMetadata{}, fmt.Errorf("backup ID must be positive")
	}
	if service.FS == nil {
		return domain.BackupMetadata{}, fmt.Errorf("filesystem is required")
	}
	if err := domain.ValidateSubscriptionName(name); err != nil {
		return domain.BackupMetadata{}, err
	}
	backup, err := service.Store.BackupByIDAny(ctx, id)
	if err != nil {
		return domain.BackupMetadata{}, err
	}
	expectedRoot := filepath.Join(service.Config.Paths.Backups, name)
	relative, err := filepath.Rel(expectedRoot, backup.Path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return domain.BackupMetadata{}, fmt.Errorf("backup path is outside subscription backup directory")
	}
	contents, err := service.FS.ReadFile(filepath.Join(backup.Path, "metadata.json"))
	if err != nil {
		return domain.BackupMetadata{}, fmt.Errorf("read backup metadata: %w", err)
	}
	var metadata domain.BackupMetadata
	if err := json.Unmarshal(contents, &metadata); err != nil {
		return domain.BackupMetadata{}, fmt.Errorf("decode backup metadata: %w", err)
	}
	if metadata.FormatVersion != 1 || metadata.Subscription.Name != name {
		return domain.BackupMetadata{}, fmt.Errorf("backup metadata does not match supported format and subscription")
	}
	if err := service.verifyChecksums(backup.Path); err != nil {
		return domain.BackupMetadata{}, err
	}
	return metadata, nil
}

// PrepareRestore validates an archive before any restore action is permitted.
// The mutating restore path must call this first.
func (service BackupService) PrepareRestore(ctx context.Context, name string, id int64) (domain.BackupMetadata, error) {
	metadata, err := service.Inspect(ctx, name, id)
	if err != nil {
		return domain.BackupMetadata{}, err
	}
	if metadata.Subscription.Home == "" || metadata.Subscription.UnixUser == "" {
		return domain.BackupMetadata{}, fmt.Errorf("backup metadata has no subscription identity")
	}
	return metadata, nil
}

func (service BackupService) extractArchive(ctx context.Context, archive, staging string) error {
	if service.Commands == nil || service.FS == nil {
		return fmt.Errorf("restore filesystem and commander are required")
	}
	if err := service.FS.MkdirAll(staging, 0o700); err != nil {
		return fmt.Errorf("create restore staging: %w", err)
	}
	if _, err := service.Commands.Run(ctx, "/usr/bin/tar", "--numeric-owner", "--acls", "--xattrs", "-p", "--zstd", "-xf", archive, "-C", staging); err != nil {
		_ = service.FS.RemoveAll(staging)
		return fmt.Errorf("extract backup archive: %w", err)
	}
	return nil
}

func (service BackupService) promoteStaging(staging, target string) error {
	if _, err := service.FS.Stat(target); err == nil {
		return fmt.Errorf("restore target %q already exists", target)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect restore target: %w", err)
	}
	mover, ok := service.FS.(system.FileMover)
	if !ok {
		return fmt.Errorf("restore filesystem cannot atomically move staged data")
	}
	if err := mover.Rename(staging, target); err != nil {
		return fmt.Errorf("promote restore staging: %w", err)
	}
	return nil
}

func (service BackupService) verifyChecksums(backupPath string) error {
	contents, err := service.FS.ReadFile(filepath.Join(backupPath, "SHA256SUMS"))
	if err != nil {
		return fmt.Errorf("read backup checksums: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(contents)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != 64 {
			return fmt.Errorf("invalid backup checksum entry")
		}
		name := filepath.Clean(fields[1])
		if filepath.IsAbs(name) || name == "." || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid backup checksum path")
		}
		file, err := service.FS.ReadFile(filepath.Join(backupPath, name))
		if err != nil {
			return fmt.Errorf("read backup file %q: %w", name, err)
		}
		actual := fmt.Sprintf("%x", sha256.Sum256(file))
		if actual != strings.ToLower(fields[0]) {
			return fmt.Errorf("backup checksum mismatch for %q", name)
		}
	}
	return nil
}

type BackupRuntime struct {
	Service    BackupService
	repository *sqlite.Repository
}

func NewReadOnlyBackupRuntime(ctx context.Context, cfg config.Config) (*BackupRuntime, error) {
	repository, err := sqlite.OpenReadOnly(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	return &BackupRuntime{Service: BackupService{Store: repository, FS: system.OSFS{}, Config: cfg}, repository: repository}, nil
}

func NewProductionBackupRuntime(ctx context.Context, cfg config.Config) (*BackupRuntime, error) {
	repository, err := sqlite.Open(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	return &BackupRuntime{Service: BackupService{Store: repository, FS: system.OSFS{}, Commands: commander, Users: system.CommandUsers{Commander: commander}, MariaDB: MariaDB{Commands: commander, Config: cfg.MariaDB}, Executor: productionExecutor(repository), LockerFor: func(name string) system.Locker {
		return system.FileLocker{Path: filepath.Join("/run", "provctl-"+name+".lock")}
	}, Config: cfg}, repository: repository}, nil
}

func (runtime *BackupRuntime) Close() error { return runtime.repository.Close() }
