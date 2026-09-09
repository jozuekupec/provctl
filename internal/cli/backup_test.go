package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestBackupRestoreCommand_ForceRequiresDoubleConfirmation(t *testing.T) {
	command := newBackupRestoreCommand()
	command.SetArgs([]string{"acme", "1", "--force"})
	command.SetOut(&bytes.Buffer{})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "--confirm-name") {
		t.Fatalf("Execute() error = %v, want confirm-name rejection", err)
	}

	command = newBackupRestoreCommand()
	command.SetArgs([]string{"acme", "1", "--force", "--confirm-name", "acme"})
	command.SetOut(&bytes.Buffer{})
	err = command.Execute()
	if err == nil || !strings.Contains(err.Error(), "--yes-i-am-sure") {
		t.Fatalf("Execute() error = %v, want explicit-sure rejection", err)
	}
}
