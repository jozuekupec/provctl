package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

// DatabaseStore persists non-secret database metadata.
type DatabaseStore interface {
	SubscriptionByName(context.Context, string) (domain.Subscription, error)
	ListDatabases(context.Context, int64) ([]domain.Database, error)
	CreateDatabase(context.Context, domain.Database) error
	DeleteDatabase(context.Context, int64, string) error
}

type MariaDBExecutor interface {
	Execute(context.Context, string) error
	UserNameLimit(context.Context) (int, error)
}

// DatabaseService coordinates MariaDB changes with SQLite metadata.
type DatabaseService struct {
	FS       system.FS
	Store    DatabaseStore
	MariaDB  MariaDBExecutor
	Executor plan.Executor
	Config   config.Config
}

// DatabaseRuntime owns the SQLite connection used by database commands.
type DatabaseRuntime struct {
	Service    DatabaseService
	repository *sqlite.Repository
}

func NewProductionDatabaseRuntime(ctx context.Context, cfg config.Config) (*DatabaseRuntime, error) {
	repository, err := sqlite.Open(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	return &DatabaseRuntime{Service: DatabaseService{
		FS: system.OSFS{}, Store: repository, MariaDB: MariaDB{Commands: commander, Config: cfg.MariaDB},
		Executor: plan.Executor{Journal: sqlite.OperationJournal{DB: repository.DB}, Locker: system.FileLocker{Path: meta.LockFile}}, Config: cfg,
	}, repository: repository}, nil
}

func NewReadOnlyDatabaseRuntime(ctx context.Context, cfg config.Config) (*DatabaseRuntime, error) {
	repository, err := sqlite.OpenReadOnly(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	return &DatabaseRuntime{Service: DatabaseService{FS: system.OSFS{}, Store: repository, MariaDB: MariaDB{Commands: commander, Config: cfg.MariaDB}, Config: cfg}, repository: repository}, nil
}

func (runtime *DatabaseRuntime) Close() error { return runtime.repository.Close() }

func (service DatabaseService) ListForSubscription(ctx context.Context, subscriptionName string) ([]domain.Database, error) {
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return nil, err
	}
	if service.Store == nil {
		return nil, fmt.Errorf("database store is required")
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return nil, err
	}
	databases, err := service.Store.ListDatabases(ctx, subscription.ID)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	return databases, nil
}

// Create creates a database and returns its generated password exactly once.
func (service DatabaseService) Create(ctx context.Context, subscriptionName, localName string) (string, int64, error) {
	return service.CreateWithCredentials(ctx, subscriptionName, localName, "")
}

// CreateWithCredentials optionally writes a new client credentials file owned
// by the subscription. Existing files are never overwritten.
func (service DatabaseService) CreateWithCredentials(ctx context.Context, subscriptionName, localName, credentialsPath string) (string, int64, error) {
	operation, password, err := service.PrepareCreateWithCredentials(ctx, subscriptionName, localName, credentialsPath)
	if err != nil {
		return "", 0, err
	}
	id, err := service.Executor.Run(ctx, operation)
	if err != nil {
		return "", id, err
	}
	return password, id, nil
}

// PrepareCreate builds the reversible database creation operation. The returned
// password is intentionally unpersisted and must only be shown after success.
func (service DatabaseService) PrepareCreate(ctx context.Context, subscriptionName, localName string) (plan.Plan, string, error) {
	return service.PrepareCreateWithCredentials(ctx, subscriptionName, localName, "")
}

func (service DatabaseService) PrepareCreateWithCredentials(ctx context.Context, subscriptionName, localName, credentialsPath string) (plan.Plan, string, error) {
	subscription, database, err := service.prepareDatabase(ctx, subscriptionName, localName)
	if err != nil {
		return plan.Plan{}, "", err
	}
	password, err := GenerateDatabasePassword(24)
	if err != nil {
		return plan.Plan{}, "", err
	}
	query, err := CreateSQL(database.Name, database.User, password)
	if err != nil {
		return plan.Plan{}, "", err
	}
	dropQuery, err := DropSQL(database.Name, database.User)
	if err != nil {
		return plan.Plan{}, "", err
	}
	steps := []plan.Step{{Name: "create MariaDB database and user", Preview: "create database " + database.Name + " and local user", Do: func(ctx context.Context) error {
		if err := service.MariaDB.Execute(ctx, query); err != nil {
			// A multi-statement server error can happen after CREATE DATABASE.
			// Best-effort cleanup prevents a partial account from being stranded.
			_ = service.MariaDB.Execute(ctx, dropQuery)
			return err
		}
		return nil
	}, Undo: func(ctx context.Context) error { return service.MariaDB.Execute(ctx, dropQuery) }}, {Name: "record database in SQLite", Preview: "insert database " + database.Name + " into SQLite", Do: func(ctx context.Context) error {
		return service.Store.CreateDatabase(ctx, database)
	}, Undo: func(ctx context.Context) error {
		return service.Store.DeleteDatabase(ctx, subscription.ID, database.Name)
	}}}
	if credentialsPath != "" {
		path, err := service.validateCredentialsPath(subscription, credentialsPath)
		if err != nil {
			return plan.Plan{}, "", err
		}
		contents := []byte(fmt.Sprintf("# Generated by provctl. Do not edit.\n[client]\nuser=%s\npassword=%s\nhost=%s\ndatabase=%s\n", database.User, password, database.Host, database.Name))
		steps = append(steps, plan.Step{Name: "write database credentials", Preview: "write subscription-owned credentials file " + path, Do: func(context.Context) error {
			if err := service.FS.WriteFileAtomic(path, contents, 0o600); err != nil {
				return err
			}
			return service.FS.Chown(path, subscription.UnixUID, subscription.UnixUID)
		}, Undo: func(context.Context) error { return service.FS.Remove(path) }})
	}
	return plan.Plan{Action: "database.create", Target: subscription.Name + "/" + database.Name, Steps: steps}, password, nil
}

func (service DatabaseService) validateCredentialsPath(subscription domain.Subscription, candidate string) (string, error) {
	if service.FS == nil {
		return "", fmt.Errorf("filesystem is required to write credentials")
	}
	if !filepath.IsAbs(candidate) {
		return "", fmt.Errorf("credentials path must be absolute")
	}
	path := filepath.Clean(candidate)
	home, err := service.FS.EvalSymlinks(subscription.Home)
	if err != nil {
		return "", fmt.Errorf("resolve subscription home: %w", err)
	}
	directory, err := service.FS.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("resolve credentials directory: %w", err)
	}
	relative, err := filepath.Rel(home, directory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("credentials path must be inside subscription home %q", subscription.Home)
	}
	if _, err := service.FS.Stat(path); err == nil {
		return "", fmt.Errorf("credentials file %q already exists", path)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect credentials file %q: %w", path, err)
	}
	return path, nil
}

func (service DatabaseService) ChangePassword(ctx context.Context, subscriptionName, localName string) (string, int64, error) {
	database, err := service.findDatabase(ctx, subscriptionName, localName)
	if err != nil {
		return "", 0, err
	}
	password, err := GenerateDatabasePassword(24)
	if err != nil {
		return "", 0, err
	}
	query, err := PasswordSQL(database.User, password)
	if err != nil {
		return "", 0, err
	}
	id, err := service.Executor.Run(ctx, plan.Plan{Action: "database.password", Target: subscriptionName + "/" + database.Name, Steps: []plan.Step{{Name: "change MariaDB password", Preview: "change password for " + database.User, Do: func(ctx context.Context) error { return service.MariaDB.Execute(ctx, query) }}}})
	if err != nil {
		return "", id, err
	}
	return password, id, nil
}

func (service DatabaseService) Delete(ctx context.Context, subscriptionName, localName string) (int64, error) {
	database, err := service.findDatabase(ctx, subscriptionName, localName)
	if err != nil {
		return 0, err
	}
	dropQuery, err := DropSQL(database.Name, database.User)
	if err != nil {
		return 0, err
	}
	operation := plan.Plan{Action: "database.delete", Target: subscriptionName + "/" + database.Name, Steps: []plan.Step{{Name: "remove database from SQLite", Preview: "delete database " + database.Name + " from SQLite", Do: func(ctx context.Context) error {
		return service.Store.DeleteDatabase(ctx, database.SubscriptionID, database.Name)
	}, Undo: func(ctx context.Context) error { return service.Store.CreateDatabase(ctx, database) }}, {Name: "drop MariaDB database and user", Preview: "drop database " + database.Name + " and local user", Do: func(ctx context.Context) error { return service.MariaDB.Execute(ctx, dropQuery) }}}}
	return service.Executor.Run(ctx, operation)
}

func (service DatabaseService) prepareDatabase(ctx context.Context, subscriptionName, localName string) (domain.Subscription, domain.Database, error) {
	if !service.Config.MariaDB.Enabled {
		return domain.Subscription{}, domain.Database{}, fmt.Errorf("MariaDB is disabled in configuration")
	}
	if service.Store == nil || service.MariaDB == nil {
		return domain.Subscription{}, domain.Database{}, fmt.Errorf("database store and MariaDB executor are required")
	}
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return domain.Subscription{}, domain.Database{}, err
	}
	if err := domain.ValidateDatabaseName(localName); err != nil {
		return domain.Subscription{}, domain.Database{}, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return domain.Subscription{}, domain.Database{}, err
	}
	if subscription.Status != "active" {
		return domain.Subscription{}, domain.Database{}, fmt.Errorf("subscription %q is %s", subscriptionName, subscription.Status)
	}
	name := subscription.Name + "_" + localName
	if err := domain.ValidateDatabaseName(name); err != nil {
		return domain.Subscription{}, domain.Database{}, fmt.Errorf("database name with subscription prefix: %w", err)
	}
	limit, err := service.MariaDB.UserNameLimit(ctx)
	if err != nil {
		return domain.Subscription{}, domain.Database{}, fmt.Errorf("read MariaDB user-name limit: %w", err)
	}
	if len(name) > limit {
		return domain.Subscription{}, domain.Database{}, fmt.Errorf("database user %q is %d characters, exceeding server limit %d", name, len(name), limit)
	}
	databases, err := service.Store.ListDatabases(ctx, subscription.ID)
	if err != nil {
		return domain.Subscription{}, domain.Database{}, fmt.Errorf("list databases: %w", err)
	}
	for _, database := range databases {
		if database.Name == name {
			return domain.Subscription{}, domain.Database{}, fmt.Errorf("database %q already exists", name)
		}
	}
	host := strings.TrimSpace(service.Config.MariaDB.Host)
	if host == "" {
		host = "localhost"
	}
	return subscription, domain.Database{SubscriptionID: subscription.ID, Name: name, User: name, Host: host, Charset: "utf8mb4", Collation: "utf8mb4_unicode_ci"}, nil
}

func (service DatabaseService) findDatabase(ctx context.Context, subscriptionName, localName string) (domain.Database, error) {
	if !service.Config.MariaDB.Enabled {
		return domain.Database{}, fmt.Errorf("MariaDB is disabled in configuration")
	}
	if service.Store == nil || service.MariaDB == nil {
		return domain.Database{}, fmt.Errorf("database store and MariaDB executor are required")
	}
	if err := domain.ValidateDatabaseName(localName); err != nil {
		return domain.Database{}, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return domain.Database{}, err
	}
	name := subscription.Name + "_" + localName
	databases, err := service.Store.ListDatabases(ctx, subscription.ID)
	if err != nil {
		return domain.Database{}, fmt.Errorf("list databases: %w", err)
	}
	for _, database := range databases {
		if database.Name == name {
			return database, nil
		}
	}
	return domain.Database{}, fmt.Errorf("database %q not found in subscription %q", name, subscriptionName)
}
