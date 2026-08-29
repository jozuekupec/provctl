package cli

import "testing"

func TestNewRootCommand_SilencesCobraErrors(t *testing.T) {
	command := NewRootCommand()
	if !command.SilenceErrors {
		t.Error("SilenceErrors = false, want true because main prints command errors")
	}
}
