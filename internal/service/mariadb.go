package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/system"
)

const databasePasswordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

// GenerateDatabasePassword creates a URL- and shell-friendly secret that is
// never stored by provctl. Callers show it once or write it to an explicit file.
func GenerateDatabasePassword(length int) (string, error) {
	if length < 24 {
		return "", fmt.Errorf("database password length must be at least 24")
	}
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
	if database.Commands == nil {
		return fmt.Errorf("MariaDB commander is required")
	}
	arguments := []string{"--batch", "--skip-column-names"}
	if database.Config.DefaultsFile != "" {
		arguments = append([]string{"--defaults-extra-file=" + database.Config.DefaultsFile}, arguments...)
	}
	result, err := database.Commands.RunWithStdin(ctx, strings.NewReader(query), "/usr/bin/mysql", arguments...)
	if err == nil {
		return nil
	}
	output := strings.TrimSpace(strings.Join([]string{result.Stdout, result.Stderr}, "\n"))
	if output == "" {
		return fmt.Errorf("execute MariaDB query: %w", err)
	}
	return fmt.Errorf("execute MariaDB query: %w: %s", err, output)
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
