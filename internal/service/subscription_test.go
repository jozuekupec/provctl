package service

import (
	"context"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/plan"
	"provctl/internal/system"
)

type subscriptionFS struct {
	directories map[string]bool
	failPath    string
}

func (fs *subscriptionFS) Stat(path string) (os.FileInfo, error) {
	if fs.directories[path] {
		return subscriptionInfo{}, nil
	}
	return nil, os.ErrNotExist
}
func (fs *subscriptionFS) ReadFile(string) ([]byte, error)                   { return nil, os.ErrNotExist }
func (fs *subscriptionFS) WriteFileAtomic(string, []byte, os.FileMode) error { return nil }
func (fs *subscriptionFS) Remove(path string) error                          { delete(fs.directories, path); return nil }
func (fs *subscriptionFS) RemoveAll(path string) error                       { delete(fs.directories, path); return nil }
func (fs *subscriptionFS) MkdirAll(path string, _ os.FileMode) error {
	if path == fs.failPath {
		return errors.New("filesystem failure")
	}
	fs.directories[path] = true
	return nil
}
func (fs *subscriptionFS) Chown(string, int, int) error             { return nil }
func (fs *subscriptionFS) Chmod(string, os.FileMode) error          { return nil }
func (fs *subscriptionFS) Symlink(string, string) error             { return nil }
func (fs *subscriptionFS) ReadDir(string) ([]os.DirEntry, error)    { return nil, nil }
func (fs *subscriptionFS) EvalSymlinks(path string) (string, error) { return path, nil }

type subscriptionInfo struct{}

func (subscriptionInfo) Name() string       { return "directory" }
func (subscriptionInfo) Size() int64        { return 0 }
func (subscriptionInfo) Mode() os.FileMode  { return os.ModeDir | 0o751 }
func (subscriptionInfo) ModTime() time.Time { return time.Time{} }
func (subscriptionInfo) IsDir() bool        { return true }
func (subscriptionInfo) Sys() any           { return nil }

type subscriptionUsers struct {
	created, deleted bool
	account          *user.User
}

func (users *subscriptionUsers) Lookup(name string) (*user.User, error) {
	if users.account != nil && users.account.Username == name {
		return users.account, nil
	}
	return nil, user.UnknownUserError(name)
}
func (users *subscriptionUsers) LookupID(uid string) (*user.User, error) {
	id, _ := strconv.Atoi(uid)
	return nil, user.UnknownUserIdError(id)
}
func (users *subscriptionUsers) Create(context.Context, system.CreateUserOptions) error {
	users.created = true
	return nil
}
func (users *subscriptionUsers) SetShell(context.Context, string, string) error    { return nil }
func (users *subscriptionUsers) LockPassword(context.Context, string) error        { return nil }
func (users *subscriptionUsers) SetPassword(context.Context, string, string) error { return nil }
func (users *subscriptionUsers) Delete(context.Context, string, bool) error {
	users.deleted = true
	return nil
}

type subscriptionStore struct {
	values map[string]domain.Subscription
}

func (store *subscriptionStore) SubscriptionExists(_ context.Context, name string) (bool, error) {
	_, exists := store.values[name]
	return exists, nil
}
func (store *subscriptionStore) SubscriptionUIDExists(_ context.Context, uid int) (bool, error) {
	for _, subscription := range store.values {
		if subscription.UnixUID == uid {
			return true, nil
		}
	}
	return false, nil
}
func (store *subscriptionStore) ListSubscriptions(_ context.Context) ([]domain.Subscription, error) {
	var subscriptions []domain.Subscription
	for _, subscription := range store.values {
		subscriptions = append(subscriptions, subscription)
	}
	return subscriptions, nil
}
func (store *subscriptionStore) SubscriptionByName(_ context.Context, name string) (domain.Subscription, error) {
	subscription, exists := store.values[name]
	if !exists {
		return domain.Subscription{}, errors.New("not found")
	}
	return subscription, nil
}
func (store *subscriptionStore) CreateSubscription(_ context.Context, subscription domain.Subscription) error {
	store.values[subscription.Name] = subscription
	return nil
}
func (store *subscriptionStore) DeleteSubscription(_ context.Context, name string) error {
	delete(store.values, name)
	return nil
}

type subscriptionJournal struct{ status plan.OperationStatus }

func (*subscriptionJournal) Start(context.Context, plan.Snapshot) (int64, error) { return 1, nil }
func (journal *subscriptionJournal) Update(_ context.Context, _ int64, status plan.OperationStatus, _ plan.Snapshot, _ string) error {
	journal.status = status
	return nil
}

type subscriptionLocker struct{}

func (subscriptionLocker) Lock(context.Context, string) (system.Unlock, error) {
	return func() error { return nil }, nil
}

func newSubscriptionService(fs *subscriptionFS, users *subscriptionUsers, store *subscriptionStore, journal *subscriptionJournal) SubscriptionService {
	cfg := config.Config{Paths: config.Paths{VHosts: "/vhosts"}, PHP: config.PHP{MaxChildren: 10, MemoryLimit: "256M", UploadMax: "64M", MaxExecTime: 60}, Users: config.Users{UIDMin: 5000, UIDMax: 5001, Shell: "/bin/bash"}}
	return SubscriptionService{FS: fs, Users: users, Store: store, Executor: plan.Executor{Journal: journal, Locker: subscriptionLocker{}}, Config: cfg}
}

func TestSubscriptionService_CreateCreatesSystemAndDatabaseState(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{}}
	users, store, journal := &subscriptionUsers{}, &subscriptionStore{values: map[string]domain.Subscription{}}, &subscriptionJournal{}
	_, err := newSubscriptionService(fs, users, store, journal).Create(context.Background(), "acme")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !users.created || users.deleted {
		t.Errorf("user state = created %t, deleted %t", users.created, users.deleted)
	}
	if _, exists := store.values["acme"]; !exists {
		t.Error("subscription was not stored")
	}
	if !fs.directories[filepath.Join("/vhosts", "acme", "tmp", "sessions")] {
		t.Error("session directory was not created")
	}
	if journal.status != plan.OperationDone {
		t.Errorf("journal status = %s, want done", journal.status)
	}
}

func TestSubscriptionService_PrepareCreateUsesDiscoveredPHPVersion(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{}}
	users, store, journal := &subscriptionUsers{}, &subscriptionStore{values: map[string]domain.Subscription{}}, &subscriptionJournal{}
	service := newSubscriptionService(fs, users, store, journal)
	service.PHPVersion = "7.9"
	operation, err := service.PrepareCreate(context.Background(), "acme")
	if err != nil {
		t.Fatalf("PrepareCreate() error = %v", err)
	}
	for _, step := range operation.Steps {
		if step.Name == "record subscription" {
			if err := step.Do(context.Background()); err != nil {
				t.Fatalf("record subscription: %v", err)
			}
			break
		}
	}
	if got, want := store.values["acme"].PHPVersion, "7.9"; got != want {
		t.Errorf("PHP version = %q, want %q", got, want)
	}
}

func TestSubscriptionService_PrepareCreateDoesNotChangeState(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{}}
	users, store, journal := &subscriptionUsers{}, &subscriptionStore{values: map[string]domain.Subscription{}}, &subscriptionJournal{}
	operation, err := newSubscriptionService(fs, users, store, journal).PrepareCreate(context.Background(), "acme")
	if err != nil {
		t.Fatalf("PrepareCreate() error = %v", err)
	}
	if users.created || users.deleted || len(fs.directories) != 0 || len(store.values) != 0 {
		t.Error("PrepareCreate() changed system or database state")
	}
	if got, want := len(operation.Steps), 9; got != want {
		t.Errorf("plan step count = %d, want %d", got, want)
	}
	if operation.Steps[0].Preview == "" {
		t.Error("user-creation preview is empty")
	}
}

func TestSubscriptionService_ShowReturnsStoredSubscription(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{}}
	users, journal := &subscriptionUsers{}, &subscriptionJournal{}
	stored := domain.Subscription{Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme", Status: "active"}
	store := &subscriptionStore{values: map[string]domain.Subscription{"acme": stored}}
	got, err := newSubscriptionService(fs, users, store, journal).Show(context.Background(), "acme")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if got != stored {
		t.Errorf("Show() = %#v, want %#v", got, stored)
	}
}

func TestSubscriptionService_DeleteRemovesArchivedSubscription(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{"/vhosts/acme": true}}
	users := &subscriptionUsers{account: &user.User{Username: "acme", Uid: "5000", HomeDir: "/vhosts/acme"}}
	journal := &subscriptionJournal{}
	stored := domain.Subscription{Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme", Status: "archived"}
	store := &subscriptionStore{values: map[string]domain.Subscription{"acme": stored}}
	_, err := newSubscriptionService(fs, users, store, journal).Delete(context.Background(), "acme", false)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !users.deleted {
		t.Error("Unix user was not deleted")
	}
	if fs.directories["/vhosts/acme"] {
		t.Error("subscription home was not deleted")
	}
	if _, exists := store.values["acme"]; exists {
		t.Error("subscription database record was not deleted")
	}
	if journal.status != plan.OperationDone {
		t.Errorf("journal status = %s, want done", journal.status)
	}
}

func TestSubscriptionService_PrepareDeleteRejectsUnexpectedHome(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{"/vhosts": true, "/tmp/acme": true}}
	users := &subscriptionUsers{account: &user.User{Username: "acme", Uid: "5000", HomeDir: "/tmp/acme"}}
	journal := &subscriptionJournal{}
	stored := domain.Subscription{Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/tmp/acme", Status: "archived"}
	store := &subscriptionStore{values: map[string]domain.Subscription{"acme": stored}}
	_, err := newSubscriptionService(fs, users, store, journal).PrepareDelete(context.Background(), "acme", false)
	if err == nil {
		t.Fatal("PrepareDelete() error = nil, want rejection")
	}
}

func TestSubscriptionService_CreateRollsBackOnFilesystemFailure(t *testing.T) {
	failure := filepath.Join("/vhosts", "acme", ".ssh")
	fs := &subscriptionFS{directories: map[string]bool{}, failPath: failure}
	users, store, journal := &subscriptionUsers{}, &subscriptionStore{values: map[string]domain.Subscription{}}, &subscriptionJournal{}
	_, err := newSubscriptionService(fs, users, store, journal).Create(context.Background(), "acme")
	if err == nil {
		t.Fatal("Create() error = nil, want failure")
	}
	if !users.deleted {
		t.Error("Unix user was not rolled back")
	}
	if len(store.values) != 0 {
		t.Errorf("stored subscriptions = %#v, want none", store.values)
	}
	if len(fs.directories) != 0 {
		t.Errorf("directories = %#v, want rollback", fs.directories)
	}
	if journal.status != plan.OperationRolledBack {
		t.Errorf("journal status = %s, want rolled_back", journal.status)
	}
}

var _ system.FS = (*subscriptionFS)(nil)
var _ system.Users = (*subscriptionUsers)(nil)
