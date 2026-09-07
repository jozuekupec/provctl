package cli

import "testing"

func TestNewRootCommand_SilencesCobraErrors(t *testing.T) {
	command := NewRootCommand()
	if !command.SilenceErrors {
		t.Error("SilenceErrors = false, want true because main prints command errors")
	}
}

func TestNewRootCommand_DefaultActionStartsTUI(t *testing.T) {
	command := NewRootCommand()
	if command.RunE == nil {
		t.Fatal("root command has no default action; want the TUI launcher")
	}
}
