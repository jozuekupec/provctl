package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

// BackupStore provides the read-only state needed before archive operations.
type BackupStore interface {
	SubscriptionByName(context.Context, string) (domain.Subscription, error)
	ListBackups(context.Context, int64) ([]domain.Backup, error)
	BackupByID(context.Context, int64, int64) (domain.Backup, error)
	CreateBackup(context.Context, domain.Backup) (int64, error)
	FinishBackup(context.Context, int64, int64, string) error
	ListDatabases(context.Context, int64) ([]domain.Database, error)
}

type BackupService struct {
	Store     BackupStore
	FS        system.FS
	Config    config.Config
	Commands  system.Commander
	Locker    system.Locker
	LockerFor func(string) system.Locker
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
			args := []string{"--single-transaction", "--quick", "--routines", "--triggers", "--events"}
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
	metadata, err := json.MarshalIndent(domain.BackupMetadata{FormatVersion: 1, ProvctlVersion: meta.Version, CreatedAt: started, Subscription: subscription, Databases: databases}, "", "  ")
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
	subscription, err := service.Store.SubscriptionByName(ctx, name)
	if err != nil {
		return domain.BackupMetadata{}, err
	}
	backup, err := service.Store.BackupByID(ctx, subscription.ID, id)
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
	return &BackupRuntime{Service: BackupService{Store: repository, FS: system.OSFS{}, Commands: system.ExecCommander{}, LockerFor: func(name string) system.Locker {
		return system.FileLocker{Path: filepath.Join("/run", "provctl-"+name+".lock")}
	}, Config: cfg}, repository: repository}, nil
}

func (runtime *BackupRuntime) Close() error { return runtime.repository.Close() }
