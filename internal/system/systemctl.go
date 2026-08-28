package system

import (
	"context"
	"fmt"
)

// CommandSystemd manages systemd only through the allowlisted systemctl binary.
type CommandSystemd struct {
	Commander Commander
}

func (systemd CommandSystemd) Reload(ctx context.Context, unit string) error {
	return systemd.run(ctx, "reload", unit)
}

func (systemd CommandSystemd) Restart(ctx context.Context, unit string) error {
	return systemd.run(ctx, "restart", unit)
}

func (systemd CommandSystemd) Start(ctx context.Context, unit string) error {
	return systemd.run(ctx, "start", unit)
}

func (systemd CommandSystemd) Stop(ctx context.Context, unit string) error {
	return systemd.run(ctx, "stop", unit)
}

func (systemd CommandSystemd) Enable(ctx context.Context, unit string) error {
	return systemd.run(ctx, "enable", unit)
}

func (systemd CommandSystemd) Disable(ctx context.Context, unit string) error {
	return systemd.run(ctx, "disable", unit)
}

func (systemd CommandSystemd) IsActive(ctx context.Context, unit string) (bool, error) {
	result, err := systemd.Commander.Run(ctx, "/usr/bin/systemctl", "is-active", "--quiet", unit)
	if err != nil && result.ExitCode != 3 {
		return false, fmt.Errorf("check systemd unit %q: %w", unit, err)
	}
	return result.ExitCode == 0, nil
}

func (systemd CommandSystemd) run(ctx context.Context, action, unit string) error {
	if _, err := systemd.Commander.Run(ctx, "/usr/bin/systemctl", action, unit); err != nil {
		return fmt.Errorf("systemctl %s %q: %w", action, unit, err)
	}
	return nil
}
