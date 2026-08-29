package fake

import "context"

// Systemd delegates unit operations to callbacks supplied by a test.
type Systemd struct {
	ReloadFunc   func(context.Context, string) error
	RestartFunc  func(context.Context, string) error
	StartFunc    func(context.Context, string) error
	StopFunc     func(context.Context, string) error
	IsActiveFunc func(context.Context, string) (bool, error)
	EnableFunc   func(context.Context, string) error
	DisableFunc  func(context.Context, string) error
}

func (f *Systemd) Reload(ctx context.Context, unit string) error  { return f.ReloadFunc(ctx, unit) }
func (f *Systemd) Restart(ctx context.Context, unit string) error { return f.RestartFunc(ctx, unit) }
func (f *Systemd) Start(ctx context.Context, unit string) error   { return f.StartFunc(ctx, unit) }
func (f *Systemd) Stop(ctx context.Context, unit string) error    { return f.StopFunc(ctx, unit) }
func (f *Systemd) IsActive(ctx context.Context, unit string) (bool, error) {
	return f.IsActiveFunc(ctx, unit)
}
func (f *Systemd) Enable(ctx context.Context, unit string) error  { return f.EnableFunc(ctx, unit) }
func (f *Systemd) Disable(ctx context.Context, unit string) error { return f.DisableFunc(ctx, unit) }
