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
	result      system.Result
}

func (commander *mariaCommander) Run(context.Context, string, ...string) (system.Result, error) {
	return commander.result, nil
}
func (commander *mariaCommander) RunWithStdin(_ context.Context, input io.Reader, name string, args ...string) (system.Result, error) {
	contents, _ := io.ReadAll(input)
	commander.name, commander.stdin, commander.args = name, string(contents), append([]string(nil), args...)
	return commander.result, nil
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

func TestMariaDB_UserNameLimitReadsServerLimit(t *testing.T) {
	commander := &mariaCommander{result: system.Result{Stdout: "80\n"}}
	limit, err := (MariaDB{Commands: commander}).UserNameLimit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if limit != 80 {
		t.Errorf("limit = %d, want 80", limit)
	}
	if !strings.Contains(commander.stdin, "information_schema.COLUMNS") {
		t.Errorf("query = %q", commander.stdin)
	}
}

func TestMariaDB_UserNameLimitRejectsInvalidResult(t *testing.T) {
	commander := &mariaCommander{result: system.Result{Stdout: "not-a-number\n"}}
	if _, err := (MariaDB{Commands: commander}).UserNameLimit(context.Background()); err == nil {
		t.Fatal("UserNameLimit() error = nil")
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

func TestDropSQLAndPasswordSQLValidateAndEscape(t *testing.T) {
	drop, err := DropSQL("acme_main", "acme_main")
	if err != nil || !strings.Contains(drop, "DROP USER IF EXISTS 'acme_main'@'localhost'") {
		t.Fatalf("DropSQL() = %q, %v", drop, err)
	}
	password, err := PasswordSQL("acme_main", "a'b")
	if err != nil || !strings.Contains(password, "IDENTIFIED BY 'a''b'") {
		t.Fatalf("PasswordSQL() = %q, %v", password, err)
	}
}
