package service

import (
	"context"
	"io"
	"strings"
	"testing"

	"provctl/internal/config"
	"provctl/internal/system"
)

func TestGenerateDatabasePassword(t *testing.T) {
	password, err := GenerateDatabasePassword(24)
	if err != nil {
		t.Fatalf("GenerateDatabasePassword() error = %v", err)
	}
	if len(password) != 24 {
		t.Errorf("length = %d, want 24", len(password))
	}
	for _, character := range password {
		if !containsDatabasePasswordCharacter(byte(character)) {
			t.Errorf("unexpected character %q", character)
		}
	}
}

func TestGenerateDatabasePasswordRejectsShortLength(t *testing.T) {
	if _, err := GenerateDatabasePassword(23); err == nil {
		t.Fatal("GenerateDatabasePassword() error = nil")
	}
}

func containsDatabasePasswordCharacter(character byte) bool {
	for index := range databasePasswordAlphabet {
		if databasePasswordAlphabet[index] == character {
			return true
		}
	}
	return false
}

type mariaCommander struct {
	name, stdin string
	args        []string
}

func (*mariaCommander) Run(context.Context, string, ...string) (system.Result, error) {
	return system.Result{}, nil
}
func (commander *mariaCommander) RunWithStdin(_ context.Context, input io.Reader, name string, args ...string) (system.Result, error) {
	contents, _ := io.ReadAll(input)
	commander.name, commander.stdin, commander.args = name, string(contents), append([]string(nil), args...)
	return system.Result{}, nil
}

func TestMariaDB_ExecuteUsesStdinAndDefaultsFile(t *testing.T) {
	commander := &mariaCommander{}
	if err := (MariaDB{Commands: commander, Config: config.MariaDB{DefaultsFile: "/etc/provctl/mysql.cnf"}}).Execute(context.Background(), "SELECT 1;"); err != nil {
		t.Fatal(err)
	}
	if commander.name != "/usr/bin/mysql" || commander.stdin != "SELECT 1;" {
		t.Errorf("call = %#v", commander)
	}
	if strings.Join(commander.args, " ") != "--defaults-extra-file=/etc/provctl/mysql.cnf --batch --skip-column-names" {
		t.Errorf("args = %#v", commander.args)
	}
}

func TestCreateSQLEscapesPassword(t *testing.T) {
	query, err := CreateSQL("main", "acme_main", "a'b")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "IDENTIFIED BY 'a''b'") {
		t.Errorf("query = %q", query)
	}
}
