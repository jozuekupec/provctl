package cli

import "testing"

func TestRootCommand_ExposesMigrate(t *testing.T) {
	root := NewRootCommand()
	for _, command := range root.Commands() {
		if command.Name() == "migrate" {
			return
		}
	}
	t.Fatal("migrate command is not registered")
}
