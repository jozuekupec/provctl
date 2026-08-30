package domain

import "testing"

func TestValidateDatabaseName(t *testing.T) {
	for _, name := range []string{"main", "app_01", "a"} {
		if err := ValidateDatabaseName(name); err != nil {
			t.Errorf("ValidateDatabaseName(%q) error = %v", name, err)
		}
	}
	for _, name := range []string{"Main", "1main", "drop-table", "a;drop"} {
		if err := ValidateDatabaseName(name); err == nil {
			t.Errorf("ValidateDatabaseName(%q) error = nil", name)
		}
	}
}
