package system_test

import (
	"context"
	"testing"

	"provctl/internal/system"
	"provctl/internal/system/fake"
)

func TestCommandUsers_SetPasswordUsesStdin(t *testing.T) {
	commander := &fake.Commander{}
	users := system.CommandUsers{Commander: commander}
	if err := users.SetPassword(context.Background(), "alice", "secret"); err != nil {
		t.Fatalf("SetPassword() error = %v", err)
	}
	if len(commander.Calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(commander.Calls))
	}
	call := commander.Calls[0]
	if call.Name != "/usr/sbin/chpasswd" || !call.HasStdin {
		t.Errorf("password command = %#v, want chpasswd with stdin", call)
	}
}
