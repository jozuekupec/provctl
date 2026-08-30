package domain

import (
	"fmt"
	"regexp"
)

// Database is the persisted, non-secret description of one MariaDB database.
type Database struct {
	ID             int64
	SubscriptionID int64
	Name           string
	User           string
	Host           string
	Charset        string
	Collation      string
}

var databaseName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)

// ValidateDatabaseName accepts only identifiers safe for quoted SQL assembly.
func ValidateDatabaseName(name string) error {
	if !databaseName.MatchString(name) {
		return fmt.Errorf("database name %q must match %s", name, databaseName)
	}
	return nil
}
