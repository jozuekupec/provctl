package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/plan"
)

type databaseStore struct {
	subscription domain.Subscription
	databases    []domain.Database
	created      []domain.Database
	deleted      []string
}

func (store *databaseStore) SubscriptionByName(context.Context, string) (domain.Subscription, error) {
	return store.subscription, nil
}
func (store *databaseStore) ListDatabases(context.Context, int64) ([]domain.Database, error) {
	return append([]domain.Database(nil), store.databases...), nil
}
func (store *databaseStore) CreateDatabase(_ context.Context, database domain.Database) error {
	store.created = append(store.created, database)
	store.databases = append(store.databases, database)
	return nil
}
func (store *databaseStore) DeleteDatabase(_ context.Context, _ int64, name string) error {
	store.deleted = append(store.deleted, name)
	for index, database := range store.databases {
		if database.Name == name {
			store.databases = append(store.databases[:index], store.databases[index+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

type databaseMariaDB struct {
	limit   int
	queries []string
	failAt  int
}

func (database *databaseMariaDB) Execute(_ context.Context, query string) error {
	database.queries = append(database.queries, query)
	if database.failAt == len(database.queries) {
		return errors.New("MariaDB failure")
	}
	return nil
}
func (database *databaseMariaDB) UserNameLimit(context.Context) (int, error) {
	return database.limit, nil
}

func testDatabaseService(store *databaseStore, mariadb *databaseMariaDB) DatabaseService {
	return DatabaseService{Store: store, MariaDB: mariadb, Executor: plan.Executor{Journal: &subscriptionJournal{}, Locker: subscriptionLocker{}}, Config: config.Config{MariaDB: config.MariaDB{Enabled: true, Host: "localhost"}}}
}

func TestDatabaseService_PrepareCreateUsesPrefixedNameAndServerLimit(t *testing.T) {
	store := &databaseStore{subscription: domain.Subscription{ID: 4, Name: "acme", Status: "active"}}
	mariadb := &databaseMariaDB{limit: 32}
	operation, password, err := testDatabaseService(store, mariadb).PrepareCreate(context.Background(), "acme", "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(password) != 24 || strings.Contains(operation.Steps[0].Preview, password) {
		t.Errorf("password = %q, preview = %q", password, operation.Steps[0].Preview)
	}
	if operation.Target != "acme/acme_main" || len(operation.Steps) != 2 {
		t.Errorf("operation = %#v", operation)
	}
}

func TestDatabaseService_PrepareCreateRejectsServerUserLimit(t *testing.T) {
	store := &databaseStore{subscription: domain.Subscription{ID: 4, Name: "acme", Status: "active"}}
	_, _, err := testDatabaseService(store, &databaseMariaDB{limit: 8}).PrepareCreate(context.Background(), "acme", "main")
	if err == nil || !strings.Contains(err.Error(), "server limit") {
		t.Fatalf("PrepareCreate() error = %v", err)
	}
}

func TestDatabaseService_PrepareCreateCredentialsRequiresSafeNewPath(t *testing.T) {
	store := &databaseStore{subscription: domain.Subscription{ID: 4, Name: "acme", UnixUID: 5000, Home: "/vhosts/acme", Status: "active"}}
	service := testDatabaseService(store, &databaseMariaDB{limit: 32})
	service.FS = &subscriptionFS{directories: map[string]bool{"/vhosts/acme": true, "/vhosts/acme/private": true}}
	operation, password, err := service.PrepareCreateWithCredentials(context.Background(), "acme", "main", "/vhosts/acme/private/mysql.cnf")
	if err != nil {
		t.Fatal(err)
	}
	if len(operation.Steps) != 3 || strings.Contains(operation.Steps[2].Preview, password) {
		t.Errorf("operation = %#v", operation)
	}
	if _, _, err := service.PrepareCreateWithCredentials(context.Background(), "acme", "other", "/etc/mysql.cnf"); err == nil {
		t.Fatal("PrepareCreateWithCredentials() accepted path outside subscription home")
	}
}

func TestDatabaseService_CreateRollsBackMariaDBWhenSQLiteFails(t *testing.T) {
	store := &databaseStore{subscription: domain.Subscription{ID: 4, Name: "acme", Status: "active"}}
	mariadb := &databaseMariaDB{limit: 32}
	service := testDatabaseService(store, mariadb)
	service.Store = failingDatabaseStore{DatabaseStore: store}
	_, _, err := service.Create(context.Background(), "acme", "main")
	if err == nil {
		t.Fatal("Create() error = nil")
	}
	if len(mariadb.queries) != 2 || !strings.Contains(mariadb.queries[1], "DROP DATABASE") {
		t.Errorf("queries = %#v", mariadb.queries)
	}
}

type failingDatabaseStore struct{ DatabaseStore }

func (failingDatabaseStore) CreateDatabase(context.Context, domain.Database) error {
	return errors.New("SQLite failure")
}

func TestDatabaseService_DeleteRestoresSQLiteWhenMariaDBFails(t *testing.T) {
	database := domain.Database{ID: 1, SubscriptionID: 4, Name: "acme_main", User: "acme_main", Host: "localhost", Charset: "utf8mb4", Collation: "utf8mb4_unicode_ci"}
	store := &databaseStore{subscription: domain.Subscription{ID: 4, Name: "acme", Status: "active"}, databases: []domain.Database{database}}
	mariadb := &databaseMariaDB{limit: 32, failAt: 1}
	_, err := testDatabaseService(store, mariadb).Delete(context.Background(), "acme", "main")
	if err == nil {
		t.Fatal("Delete() error = nil")
	}
	if len(store.databases) != 1 || store.databases[0].Name != "acme_main" {
		t.Errorf("databases after rollback = %#v", store.databases)
	}
}
