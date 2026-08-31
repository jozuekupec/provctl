package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/system"
)

const databasePasswordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

// GenerateSSHPassword creates a one-time account password that is never
// persisted by provctl.
func GenerateSSHPassword(length int) (string, error) {
	if length < 20 {
		return "", fmt.Errorf("SSH password length must be at least 20")
	}
	return generatePassword(length)
}

// GenerateDatabasePassword creates a URL- and shell-friendly secret that is
// never stored by provctl. Callers show it once or write it to an explicit file.
func GenerateDatabasePassword(length int) (string, error) {
	if length < 24 {
		return "", fmt.Errorf("database password length must be at least 24")
	}
	return generatePassword(length)
}

func generatePassword(length int) (string, error) {
	password := make([]byte, length)
	limit := big.NewInt(int64(len(databasePasswordAlphabet)))
	for index := range password {
		value, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("generate database password: %w", err)
		}
		password[index] = databasePasswordAlphabet[value.Int64()]
	}
	return string(password), nil
}

// MariaDB executes SQL only through stdin, never through process arguments.
type MariaDB struct {
	Commands system.Commander
	Config   config.MariaDB
}

func (database MariaDB) Execute(ctx context.Context, query string) error {
	_, err := database.Query(ctx, query)
	return err
}

// Query executes SQL through stdin and returns only bounded mysql output.
func (database MariaDB) Query(ctx context.Context, query string) (string, error) {
	if database.Commands == nil {
		return "", fmt.Errorf("MariaDB commander is required")
	}
	arguments := []string{"--batch", "--skip-column-names"}
	if database.Config.DefaultsFile != "" {
		arguments = append([]string{"--defaults-extra-file=" + database.Config.DefaultsFile}, arguments...)
	}
	result, err := database.Commands.RunWithStdin(ctx, strings.NewReader(query), "/usr/bin/mysql", arguments...)
	if err == nil {
		return strings.TrimSpace(result.Stdout), nil
	}
	output := strings.TrimSpace(strings.Join([]string{result.Stdout, result.Stderr}, "\n"))
	if output == "" {
		return "", fmt.Errorf("execute MariaDB query: %w", err)
	}
	return "", fmt.Errorf("execute MariaDB query: %w: %s", err, output)
}

func (database MariaDB) UserNameLimit(ctx context.Context) (int, error) {
	output, err := database.Query(ctx, "SELECT CHARACTER_MAXIMUM_LENGTH FROM information_schema.COLUMNS WHERE TABLE_SCHEMA='mysql' AND TABLE_NAME='global_priv' AND COLUMN_NAME='User';\n")
	if err != nil {
		return 0, err
	}
	limit, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil || limit < 1 {
		return 0, fmt.Errorf("read MariaDB user-name limit: %q", output)
	}
	return limit, nil
}

// CreateSQL returns validated SQL for a database and local login. The password
// must only be supplied through MariaDB.Execute stdin.
func CreateSQL(name, user, password string) (string, error) {
	if err := domain.ValidateDatabaseName(name); err != nil {
		return "", err
	}
	if err := domain.ValidateDatabaseName(user); err != nil {
		return "", err
	}
	if password == "" {
		return "", fmt.Errorf("database password is required")
	}
	escapedPassword := strings.ReplaceAll(password, "'", "''")
	return fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;\nCREATE USER '%s'@'localhost' IDENTIFIED BY '%s';\nGRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost';\nFLUSH PRIVILEGES;\n", name, user, escapedPassword, name, user), nil
}

// DropSQL removes a database and its explicitly local user.
func DropSQL(name, user string) (string, error) {
	if err := domain.ValidateDatabaseName(name); err != nil {
		return "", err
	}
	if err := domain.ValidateDatabaseName(user); err != nil {
		return "", err
	}
	return fmt.Sprintf("DROP DATABASE IF EXISTS `%s`;\nDROP USER IF EXISTS '%s'@'localhost';\nFLUSH PRIVILEGES;\n", name, user), nil
}

// PasswordSQL changes a local database user's password without exposing it in
// process arguments.
func PasswordSQL(user, password string) (string, error) {
	if err := domain.ValidateDatabaseName(user); err != nil {
		return "", err
	}
	if password == "" {
		return "", fmt.Errorf("database password is required")
	}
	escapedPassword := strings.ReplaceAll(password, "'", "''")
	return fmt.Sprintf("ALTER USER '%s'@'localhost' IDENTIFIED BY '%s';\n", user, escapedPassword), nil
}
