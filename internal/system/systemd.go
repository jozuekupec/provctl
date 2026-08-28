package system

import "context"

type Systemd interface {
	Reload(ctx context.Context, unit string) error
	Restart(ctx context.Context, unit string) error
	Start(ctx context.Context, unit string) error
	Stop(ctx context.Context, unit string) error
	IsActive(ctx context.Context, unit string) (bool, error)
	Enable(ctx context.Context, unit string) error
	Disable(ctx context.Context, unit string) error
}
