package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"testing"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/plan"
	"provctl/internal/system"
	"provctl/internal/system/fake"
)

type restoreFS struct {
	*fake.FS
	moves [][2]string
}

func (fs *restoreFS) Rename(oldPath, newPath string) error {
	fs.moves = append(fs.moves, [2]string{oldPath, newPath})
	return nil
}

type backupStore struct {
	subscription domain.Subscription
	backups      []domain.Backup
}

type restoredDatabaseStore struct {
	backupStore
	restored  domain.Subscription
	databases []domain.Database
}

func (store *restoredDatabaseStore) SubscriptionByName(context.Context, string) (domain.Subscription, error) {
	return store.restored, nil
}
func (store *restoredDatabaseStore) CreateDatabase(_ context.Context, database domain.Database) error {
	store.databases = append(store.databases, database)
	return nil
}
func (store *restoredDatabaseStore) DeleteDatabase(_ context.Context, _ int64, name string) error {
	for index, database := range store.databases {
		if database.Name == name {
			store.databases = append(store.databases[:index], store.databases[index+1:]...)
		}
	}
	return nil
}

type restoringMariaDB struct{ queries []string }

func (database *restoringMariaDB) Execute(_ context.Context, query string) error {
	database.queries = append(database.queries, query)
	return nil
}
func (*restoringMariaDB) UserNameLimit(context.Context) (int, error) { return 64, nil }

func (store backupStore) SubscriptionExists(_ context.Context, name string) (bool, error) {
	return store.subscription.Name == name, nil
}
func (backupStore) SubscriptionUIDExists(context.Context, int) (bool, error) { return false, nil }
func (store backupStore) ListSubscriptions(context.Context) ([]domain.Subscription, error) {
	if store.subscription.Name == "" {
		return nil, nil
	}
	return []domain.Subscription{store.subscription}, nil
}

func (store backupStore) SubscriptionByName(context.Context, string) (domain.Subscription, error) {
	return store.subscription, nil
}
func (store backupStore) ListBackups(context.Context, int64) ([]domain.Backup, error) {
	return store.backups, nil
}
func (store backupStore) BackupByID(_ context.Context, _ int64, id int64) (domain.Backup, error) {
	for _, backup := range store.backups {
		if backup.ID == id {
			return backup, nil
		}
	}
	return domain.Backup{}, context.Canceled
}
func (store backupStore) BackupByIDAny(_ context.Context, id int64) (domain.Backup, error) {
	for _, backup := range store.backups {
		if backup.ID == id {
			return backup, nil
		}
	}
	return domain.Backup{}, context.Canceled
}
func (backupStore) CreateBackup(context.Context, domain.Backup) (int64, error)      { return 1, nil }
func (backupStore) FinishBackup(context.Context, int64, int64, string) error        { return nil }
func (backupStore) ListDatabases(context.Context, int64) ([]domain.Database, error) { return nil, nil }
func (backupStore) ListWebsites(context.Context, int64) ([]domain.Website, error)   { return nil, nil }
func (backupStore) ListCronJobs(context.Context, int64) ([]domain.CronJob, error)   { return nil, nil }
func (backupStore) ListSSHKeys(context.Context, int64) ([]domain.SSHKey, error)     { return nil, nil }
func (backupStore) ListCertificates(context.Context, int64) ([]domain.Certificate, error) {
	return nil, nil
}
func (backupStore) CreateDatabase(context.Context, domain.Database) error         { return nil }
func (backupStore) DeleteDatabase(context.Context, int64, string) error           { return nil }
func (backupStore) CreateSubscription(context.Context, domain.Subscription) error { return nil }
func (backupStore) DeleteSubscription(context.Context, string) error              { return nil }
func (backupStore) SetSubscriptionStatus(context.Context, int64, string) error    { return nil }

func TestBackupService_ListForSubscriptionRejectsInvalidName(t *testing.T) {
	_, err := (BackupService{Store: backupStore{}}).ListForSubscription(context.Background(), "BAD")
	if err == nil || !strings.Contains(err.Error(), "subscription name") {
		t.Fatalf("error = %v", err)
	}
}

func TestBackupService_InspectReadsMatchingManifest(t *testing.T) {
	manifest := []byte(`{"format_version":1,"created_at":"2026-01-01T00:00:00Z","subscription":{"name":"acme"}}`)
	service := BackupService{
		Store: backupStore{subscription: domain.Subscription{ID: 1, Name: "acme"}, backups: []domain.Backup{{ID: 4, Path: "/backups/acme/2026-01-01"}}},
		FS: &fake.FS{ReadFileFunc: func(path string) ([]byte, error) {
			switch path {
			case "/backups/acme/2026-01-01/metadata.json":
				return manifest, nil
			case "/backups/acme/2026-01-01/SHA256SUMS":
				return []byte(fmt.Sprintf("%x  metadata.json\n", sha256.Sum256(manifest))), nil
			default:
				t.Fatalf("path = %q", path)
			}
			return nil, nil
		}},
		Config: config.Config{Paths: config.Paths{Backups: "/backups"}},
	}
	metadata, err := service.Inspect(context.Background(), "acme", 4)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.FormatVersion != 1 || metadata.Subscription.Name != "acme" {
		t.Errorf("metadata = %#v", metadata)
	}
}

func TestBackupService_InspectDoesNotRequireSourceSubscription(t *testing.T) {
	manifest := []byte(`{"format_version":1,"created_at":"2026-01-01T00:00:00Z","subscription":{"name":"acme"}}`)
	service := BackupService{
		Store: backupStore{backups: []domain.Backup{{ID: 4, Path: "/backups/acme/2026-01-01"}}},
		FS: &fake.FS{ReadFileFunc: func(path string) ([]byte, error) {
			switch path {
			case "/backups/acme/2026-01-01/metadata.json":
				return manifest, nil
			case "/backups/acme/2026-01-01/SHA256SUMS":
				return []byte(fmt.Sprintf("%x  metadata.json\n", sha256.Sum256(manifest))), nil
			default:
				t.Fatalf("path = %q", path)
			}
			return nil, nil
		}},
		Config: config.Config{Paths: config.Paths{Backups: "/backups"}},
	}
	if _, err := service.Inspect(context.Background(), "acme", 4); err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
}

func TestBackupService_CreateRejectsBackupQuotaBeforeSystemChanges(t *testing.T) {
	service := BackupService{Store: backupStore{subscription: domain.Subscription{ID: 1, Name: "acme", QuotaBackups: 1}, backups: []domain.Backup{{ID: 1}}}}
	_, err := service.Create(context.Background(), "acme")
	if err == nil || !strings.Contains(err.Error(), "backup quota") {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestBackupService_ExtractArchiveUsesExplicitTarArguments(t *testing.T) {
	fs := &fake.FS{MkdirAllFunc: func(string, os.FileMode) error { return nil }, RemoveAllFunc: func(string) error { return nil }}
	commands := &fake.Commander{}
	service := BackupService{FS: fs, Commands: commands}
	if err := service.extractArchive(context.Background(), "/backups/acme/files.tar.zst", "/vhosts/.restore-acme"); err != nil {
		t.Fatal(err)
	}
	if len(commands.Calls) != 1 || commands.Calls[0].Name != "/usr/bin/tar" || strings.Contains(strings.Join(commands.Calls[0].Args, " "), "sh -c") {
		t.Errorf("calls = %#v", commands.Calls)
	}
}

func TestBackupService_PromoteStagingRejectsExistingTarget(t *testing.T) {
	fs := &fake.FS{StatFunc: func(string) (os.FileInfo, error) { return subscriptionInfo{}, nil }}
	service := BackupService{FS: fs}
	err := service.promoteStaging("/vhosts/.restore-acme", "/vhosts/acme")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("promoteStaging() error = %v", err)
	}
}

func TestBackupService_RestoreFilesExtractsThenPromotesAndRecords(t *testing.T) {
	fs := &restoreFS{FS: &fake.FS{
		StatFunc:      func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		MkdirAllFunc:  func(string, os.FileMode) error { return nil },
		RemoveAllFunc: func(string) error { return nil },
		ChownFunc:     func(string, int, int) error { return nil },
		ChmodFunc:     func(string, os.FileMode) error { return nil },
	}}
	usersCreated := false
	users := &fake.Users{
		CreateFunc:       func(context.Context, system.CreateUserOptions) error { usersCreated = true; return nil },
		LockPasswordFunc: func(context.Context, string) error { return nil },
		DeleteFunc:       func(context.Context, string, bool) error { return nil },
	}
	journal := &subscriptionJournal{}
	commands := &fake.Commander{}
	service := BackupService{
		Store:    backupStore{},
		FS:       fs,
		Commands: commands,
		Users:    users,
		Executor: plan.Executor{Journal: journal, Locker: subscriptionLocker{}},
	}
	subscription := domain.Subscription{Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme"}
	if _, err := service.restoreFiles(context.Background(), "/backups/acme/one", subscription, "/vhosts/.restore-acme", nil, nil); err != nil {
		t.Fatal(err)
	}
	if !usersCreated || journal.status != plan.OperationDone {
		t.Errorf("usersCreated=%t status=%s", usersCreated, journal.status)
	}
	if len(fs.moves) != 1 || fs.moves[0] != [2]string{"/vhosts/.restore-acme", "/vhosts/acme"} {
		t.Errorf("moves = %#v", fs.moves)
	}
	if len(commands.Calls) != 2 || commands.Calls[0].Name != "/usr/bin/tar" || commands.Calls[1].Name != "/usr/bin/chown" {
		t.Errorf("commands = %#v", commands.Calls)
	}
}

func TestBackupService_NextRestoreUIDSkipsReservedUID(t *testing.T) {
	users := &fake.Users{
		LookupFunc: func(string) (*user.User, error) { return nil, user.UnknownUserError("acme") },
		LookupIDFunc: func(id string) (*user.User, error) {
			if id == "5000" {
				return &user.User{Uid: id}, nil
			}
			value, _ := strconv.Atoi(id)
			return nil, user.UnknownUserIdError(value)
		},
	}
	service := BackupService{Store: backupStore{}, Users: users, Config: config.Config{Users: config.Users{UIDMin: 5000, UIDMax: 5001}}}
	uid, err := service.nextRestoreUID(context.Background(), "acme")
	if err != nil || uid != 5001 {
		t.Fatalf("nextRestoreUID() = %d, %v", uid, err)
	}
}

func TestBackupService_RestoreFilesImportsDatabaseWithNewCredentials(t *testing.T) {
	fs := &restoreFS{FS: &fake.FS{
		StatFunc:      func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		MkdirAllFunc:  func(string, os.FileMode) error { return nil },
		RemoveFunc:    func(string) error { return nil },
		RemoveAllFunc: func(string) error { return nil },
		ChownFunc:     func(string, int, int) error { return nil },
		ChmodFunc:     func(string, os.FileMode) error { return nil },
	}}
	store := &restoredDatabaseStore{restored: domain.Subscription{ID: 8, Name: "acme"}}
	users := &fake.Users{
		CreateFunc:       func(context.Context, system.CreateUserOptions) error { return nil },
		LockPasswordFunc: func(context.Context, string) error { return nil },
		DeleteFunc:       func(context.Context, string, bool) error { return nil },
	}
	commands, mariadb := &fake.Commander{}, &restoringMariaDB{}
	service := BackupService{
		Store: store, FS: fs, Commands: commands, Users: users, MariaDB: mariadb,
		Executor: plan.Executor{Journal: &subscriptionJournal{}, Locker: subscriptionLocker{}},
		Config:   config.Config{MariaDB: config.MariaDB{Enabled: true}},
	}
	subscription := domain.Subscription{Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme"}
	database := domain.Database{Name: "acme_main", User: "acme_main", Host: "localhost"}
	if _, err := service.restoreFiles(context.Background(), "/backups/acme/one", subscription, "/vhosts/.restore-acme", []domain.Database{database}, map[string]string{database.Name: "FreshPassword234567890123"}); err != nil {
		t.Fatal(err)
	}
	if len(store.databases) != 1 || store.databases[0].SubscriptionID != 8 {
		t.Fatalf("restored database metadata = %#v", store.databases)
	}
	if len(mariadb.queries) != 1 || !strings.Contains(mariadb.queries[0], "CREATE DATABASE `acme_main`") {
		t.Errorf("MariaDB queries = %#v", mariadb.queries)
	}
	if len(commands.Calls) != 4 || commands.Calls[2].Name != "/usr/bin/zstd" || commands.Calls[3].Name != "/usr/bin/mysql" || !commands.Calls[3].HasStdin {
		t.Errorf("commands = %#v", commands.Calls)
	}
}
