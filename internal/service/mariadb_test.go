package service

import "testing"

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
