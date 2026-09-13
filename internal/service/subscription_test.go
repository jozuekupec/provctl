package service

import (
	"context"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/plan"
	"provctl/internal/system"
	"provctl/internal/system/fake"
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
func (fs *subscriptionFS) Rename(oldPath, newPath string) error {
	if !fs.directories[oldPath] {
		return os.ErrNotExist
	}
	delete(fs.directories, oldPath)
	fs.directories[newPath] = true
	return nil
}

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
	values       map[string]domain.Subscription
	domains      map[string]bool
	websites     []domain.Website
	certificates []domain.Certificate
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
	if subscription.ID == 0 {
		subscription.ID = int64(len(store.values) + 1)
	}
	store.values[subscription.Name] = subscription
	return nil
}
func (store *subscriptionStore) DomainExists(_ context.Context, name string) (bool, error) {
	return store.domains[name], nil
}
func (store *subscriptionStore) CreateWebsite(_ context.Context, website domain.Website) (int64, error) {
	website.ID = int64(len(store.websites) + 1)
	store.websites = append(store.websites, website)
	store.domains[website.PrimaryDomain] = true
	return website.ID, nil
}

func (store *subscriptionStore) CreateCertificate(_ context.Context, certificate domain.Certificate) (int64, error) {
	store.certificates = append(store.certificates, certificate)
	return 1, nil
}

func (store *subscriptionStore) DeleteCertificateByWebsite(context.Context, int64) error {
	return nil
}

func (store *subscriptionStore) AddWebsiteAlias(_ context.Context, _ int64, alias string) error {
	store.domains[alias] = true
	return nil
}
func (store *subscriptionStore) DeleteSubscription(_ context.Context, name string) error {
	delete(store.values, name)
	return nil
}
func (store *subscriptionStore) SetSubscriptionStatus(_ context.Context, id int64, status string) error {
	for name, subscription := range store.values {
		if subscription.ID == id {
			subscription.Status = status
			store.values[name] = subscription
			return nil
		}
	}
	return errors.New("not found")
}
func (*subscriptionStore) ListDatabases(context.Context, int64) ([]domain.Database, error) {
	return nil, nil
}
func (*subscriptionStore) DeleteDatabase(context.Context, int64, string) error { return nil }
func (store *subscriptionStore) ListWebsites(context.Context, int64) ([]domain.Website, error) {
	return store.websites, nil
}
func (*subscriptionStore) DeleteWebsite(context.Context, int64) error { return nil }
func (*subscriptionStore) ListCertificates(context.Context, int64) ([]domain.Certificate, error) {
	return nil, nil
}
func (*subscriptionStore) DeleteCertificatesBySubscription(context.Context, int64) error {
	return nil
}

type subscriptionJournal struct{ status plan.OperationStatus }

func (*subscriptionJournal) Start(context.Context, plan.Snapshot) (int64, error) { return 1, nil }
func (journal *subscriptionJournal) Update(_ context.Context, _ int64, status plan.OperationStatus, _ plan.Snapshot, _ string) error {
	journal.status = status
	return nil
}

type subscriptionRenewals struct {
	lineages     []RenewalLineage
	reconfigured []string
	verifyErr    error
	restored     bool
}

func (renewals *subscriptionRenewals) Snapshot(context.Context, string) (func(context.Context) error, error) {
	return func(context.Context) error { renewals.restored = true; return nil }, nil
}

func (renewals *subscriptionRenewals) Find(context.Context, string) ([]RenewalLineage, error) {
	return renewals.lineages, nil
}
func (renewals *subscriptionRenewals) Reconfigure(_ context.Context, lineage RenewalLineage, _ string) error {
	renewals.reconfigured = append(renewals.reconfigured, lineage.Name)
	return nil
}
func (renewals *subscriptionRenewals) Verify(context.Context, string) error {
	return renewals.verifyErr
}

type subscriptionLocker struct{}

func (subscriptionLocker) Lock(context.Context, string) (system.Unlock, error) {
	return func() error { return nil }, nil
}

func newSubscriptionService(fs *subscriptionFS, users *subscriptionUsers, store *subscriptionStore, journal *subscriptionJournal) SubscriptionService {
	cfg := config.Config{Paths: config.Paths{VHosts: "/vhosts"}, PHP: config.PHP{MaxChildren: 10, MemoryLimit: "256M", UploadMax: "64M", MaxExecTime: 60}, Users: config.Users{UIDMin: 5000, UIDMax: 5001, Shell: "/bin/bash"}}
	cfg.Paths.ACMEChallenge = "/var/lib/provctl-acme-challenge"
	cfg.Apache.ProxyTimeout = 60
	return SubscriptionService{FS: fs, Users: users, Store: store, Executor: plan.Executor{Journal: journal, Locker: subscriptionLocker{}}, Config: cfg}
}

func TestSubscriptionService_ListUsageCountsWebsitesAndMeasuresDisk(t *testing.T) {
	store := &subscriptionStore{values: map[string]domain.Subscription{
		"acme": {ID: 7, Name: "acme", Home: "/vhosts/acme"},
	}, websites: []domain.Website{{SubscriptionID: 7, Enabled: true}, {SubscriptionID: 7, Enabled: false}, {SubscriptionID: 7, Enabled: true}}}
	commands := &fake.Commander{Result: system.Result{Stdout: "12345\t/vhosts/acme\n"}}
	service := SubscriptionService{Store: store, Commands: commands}
	usage, err := service.ListUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := usage[7], (SubscriptionUsage{ActiveWebsites: 2, DisabledWebsites: 1, DiskUsedBytes: 12345, DiskUsageKnown: true}); !cmp.Equal(got, want) {
		t.Fatalf("usage = %#v, want %#v", got, want)
	}
	if got, want := commands.Calls, []fake.CommandCall{{Name: "/usr/bin/du", Args: []string{"-sb", "/vhosts/acme"}}}; !cmp.Equal(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
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

func TestSubscriptionService_SetStatusArchivesSubscription(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{}}
	users := &subscriptionUsers{}
	store := &subscriptionStore{values: map[string]domain.Subscription{"acme": {ID: 1, Name: "acme", Status: "active"}}}
	journal := &subscriptionJournal{}
	if _, err := newSubscriptionService(fs, users, store, journal).SetStatus(context.Background(), "acme", "archived"); err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	if got := store.values["acme"].Status; got != "archived" {
		t.Errorf("status = %q, want archived", got)
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

func TestSubscriptionService_PrepareDeleteRemovesPHPFPMBeforeUnixUser(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{"/vhosts/acme": true}}
	users := &subscriptionUsers{account: &user.User{Username: "acme", Uid: "5000", HomeDir: "/vhosts/acme"}}
	store := &subscriptionStore{values: map[string]domain.Subscription{"acme": {ID: 1, Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme", Status: "archived", PHPVersion: "8.4"}}, websites: []domain.Website{{Type: domain.WebsitePHPFPM}}}
	service := newSubscriptionService(fs, users, store, &subscriptionJournal{})
	service.PHPFPM, service.Apache = websitePHPFPM{}, websiteApache{}
	operation, err := service.PrepareDelete(context.Background(), "acme", false)
	if err != nil {
		t.Fatal(err)
	}
	p, u := -1, -1
	for index, step := range operation.Steps {
		if step.Name == "remove PHP-FPM pool" {
			p = index
		}
		if step.Name == "delete Unix user" {
			u = index
		}
	}
	if p < 0 || u < 0 || p >= u {
		t.Errorf("delete steps do not remove PHP-FPM before user: %#v", operation.Steps)
	}
}

func TestSubscriptionService_PrepareDeleteSkipsAbsentPHPFPMPoolForStaticOnlySubscription(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{"/vhosts/acme": true}}
	users := &subscriptionUsers{account: &user.User{Username: "acme", Uid: "5000", HomeDir: "/vhosts/acme"}}
	store := &subscriptionStore{values: map[string]domain.Subscription{"acme": {ID: 1, Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme", Status: "archived", PHPVersion: "8.4"}}, websites: []domain.Website{{Type: domain.WebsiteStatic}}}
	service := newSubscriptionService(fs, users, store, &subscriptionJournal{})
	service.PHPFPM, service.Apache = websitePHPFPM{}, websiteApache{}
	operation, err := service.PrepareDelete(context.Background(), "acme", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range operation.Steps {
		if step.Name == "remove PHP-FPM pool" {
			t.Fatal("static-only subscription unexpectedly removes a PHP-FPM pool")
		}
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

func TestSubscriptionService_PrepareAdoptBuildsOnePlanWithSQLiteLast(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{"/legacy/example.test": true}}
	users := &subscriptionUsers{}
	store := &subscriptionStore{values: map[string]domain.Subscription{}, domains: map[string]bool{}}
	service := newSubscriptionService(fs, users, store, &subscriptionJournal{})
	service.Commands = &fake.Commander{}
	service.Apache = websiteApache{}
	service.PHPFPM = websitePHPFPM{}
	service.PHPVersion = "8.4"
	operation, err := service.PrepareAdopt(context.Background(), "acme", SubscriptionAdoptOptions{Source: "/legacy/example.test", Domain: "example.test", Backup: true})
	if err != nil {
		t.Fatalf("PrepareAdopt() error = %v", err)
	}
	if got, want := operation.Action, "subscription.adopt"; got != want {
		t.Errorf("action = %q, want %q", got, want)
	}
	if operation.Steps[len(operation.Steps)-2].Name != "record subscription" || operation.Steps[len(operation.Steps)-1].Name != "record website" {
		t.Errorf("final steps = %q, %q, want SQLite records", operation.Steps[len(operation.Steps)-2].Name, operation.Steps[len(operation.Steps)-1].Name)
	}
	foundMove := false
	for _, step := range operation.Steps {
		if step.Name == "move legacy document root" {
			foundMove = true
			if !strings.Contains(step.Preview, "/vhosts/acme/sites/example.test/public") {
				t.Errorf("move target = %q", step.Preview)
			}
		}
	}
	if !foundMove {
		t.Error("move step is missing")
	}
}

func TestSubscriptionService_PrepareAdoptRejectsSourceInsideVHosts(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{"/vhosts/legacy": true}}
	store := &subscriptionStore{values: map[string]domain.Subscription{}, domains: map[string]bool{}}
	service := newSubscriptionService(fs, &subscriptionUsers{}, store, &subscriptionJournal{})
	service.Commands, service.Apache, service.PHPFPM = &fake.Commander{}, websiteApache{}, websitePHPFPM{}
	_, err := service.PrepareAdopt(context.Background(), "acme", SubscriptionAdoptOptions{Source: "/vhosts/legacy", Domain: "example.test"})
	if err == nil || !strings.Contains(err.Error(), "inside vhosts root") {
		t.Fatalf("PrepareAdopt() error = %v, want source-root rejection", err)
	}
}

func TestSubscriptionService_AdoptMovesDataAndRecordsWebsite(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{"/legacy/example.test": true}}
	users := &subscriptionUsers{}
	store := &subscriptionStore{values: map[string]domain.Subscription{}, domains: map[string]bool{}}
	service := newSubscriptionService(fs, users, store, &subscriptionJournal{})
	commands := &fake.Commander{}
	service.Commands, service.Apache, service.PHPFPM, service.PHPVersion = commands, websiteApache{}, websitePHPFPM{}, "8.4"
	if _, err := service.Adopt(context.Background(), "acme", SubscriptionAdoptOptions{Source: "/legacy/example.test", Domain: "example.test", Backup: false}); err != nil {
		t.Fatalf("Adopt() error = %v", err)
	}
	target := "/vhosts/acme/sites/example.test/public"
	if fs.directories["/legacy/example.test"] || !fs.directories[target] {
		t.Errorf("document-root move state = %#v", fs.directories)
	}
	if !users.created || len(store.websites) != 1 || store.websites[0].SubscriptionID == 0 {
		t.Errorf("adopted state: user=%t websites=%#v", users.created, store.websites)
	}
	if len(commands.Calls) != 2 || commands.Calls[0].Name != "/usr/sbin/apache2ctl" || commands.Calls[1].Name != "/usr/bin/chown" {
		t.Errorf("ownership command = %#v", commands.Calls)
	}
}

func TestSubscriptionService_TransferDocumentRootRejectsMissingAtomicMover(t *testing.T) {
	service := SubscriptionService{FS: &fake.FS{}, Commands: &fake.Commander{}}
	err := service.transferDocumentRoot("/legacy", "/vhosts/acme/sites/example.test/public", false)(context.Background())
	if err == nil || !strings.Contains(err.Error(), "atomic rename") {
		t.Fatalf("transferDocumentRoot() error = %v, want atomic mover rejection", err)
	}
}

func TestSubscriptionService_AdoptMarksRenewalFailureInconsistent(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{"/legacy/example.test": true}}
	journal := &subscriptionJournal{}
	store := &subscriptionStore{values: map[string]domain.Subscription{}, domains: map[string]bool{}}
	service := newSubscriptionService(fs, &subscriptionUsers{}, store, journal)
	service.Commands, service.Apache, service.PHPFPM, service.PHPVersion = &fake.Commander{}, websiteApache{}, websitePHPFPM{}, "8.4"
	renewals := &subscriptionRenewals{lineages: []RenewalLineage{{Name: "legacy", Domains: []string{"example.test"}}}, verifyErr: errors.New("renewal failed")}
	service.Renewals = renewals
	_, err := service.Adopt(context.Background(), "acme", SubscriptionAdoptOptions{Source: "/legacy/example.test", Domain: "example.test", Backup: false})
	if err == nil {
		t.Fatal("Adopt() error = nil, want renewal failure")
	}
	if journal.status != plan.OperationInconsistent {
		t.Errorf("journal status = %q, want inconsistent", journal.status)
	}
	if !cmp.Equal(renewals.reconfigured, []string{"legacy"}) {
		t.Errorf("reconfigured lineages = %#v", renewals.reconfigured)
	}
	if !renewals.restored {
		t.Error("renewal verification failure did not restore original configuration")
	}
	if len(store.values) != 1 || len(store.websites) != 1 {
		t.Errorf("adopted artifacts were rolled back after renewal failure: subscriptions=%#v websites=%#v", store.values, store.websites)
	}
}

func TestSubscriptionService_AdoptPreservesTLSLineage(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{"/legacy/example.test": true}}
	store := &subscriptionStore{values: map[string]domain.Subscription{}, domains: map[string]bool{}}
	service := newSubscriptionService(fs, &subscriptionUsers{}, store, &subscriptionJournal{})
	service.Commands, service.Apache, service.PHPFPM, service.PHPVersion = &fake.Commander{}, websiteApache{}, websitePHPFPM{}, "8.4"
	service.Renewals = &subscriptionRenewals{lineages: []RenewalLineage{{Name: "example.test-0001", Domains: []string{"example.test", "www.example.test"}}}}
	if _, err := service.Adopt(context.Background(), "acme", SubscriptionAdoptOptions{Source: "/legacy/example.test", Domain: "example.test"}); err != nil {
		t.Fatal(err)
	}
	if len(store.websites) != 1 || !store.websites[0].SSLEnabled || store.websites[0].CertificateName != "example.test-0001" {
		t.Fatalf("adopted website: %#v", store.websites)
	}
	if len(store.certificates) != 1 || store.certificates[0].WebsiteID != store.websites[0].ID || store.certificates[0].Lineage != "example.test-0001" {
		t.Fatalf("adopted certificate: %#v", store.certificates)
	}
	if store.certificates[0].Managed {
		t.Fatal("adopted certificate is incorrectly marked as provctl-managed")
	}
	if !store.domains["www.example.test"] || !cmp.Equal(store.websites[0].Aliases, []string{"www.example.test"}) {
		t.Fatal("certificate SAN alias was not adopted")
	}
}

var _ system.FS = (*subscriptionFS)(nil)
var _ system.FileMover = (*subscriptionFS)(nil)
var _ system.Users = (*subscriptionUsers)(nil)
