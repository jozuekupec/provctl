package system_test

import (
	"context"
	"testing"

	"provctl/internal/system"
	"provctl/internal/system/fake"
)

func TestCommandSystemd_ReloadUsesSystemctl(t *testing.T) {
	commander := &fake.Commander{}
	manager := system.CommandSystemd{Commander: commander}
	if err := manager.Reload(context.Background(), "apache2"); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	call := commander.Calls[0]
	if call.Name != "/usr/bin/systemctl" || len(call.Args) != 2 || call.Args[0] != "reload" || call.Args[1] != "apache2" {
		t.Errorf("call = %#v, want systemctl reload apache2", call)
	}
}
