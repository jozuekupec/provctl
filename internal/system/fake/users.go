package fake

import (
	"context"
	"os/user"

	"provctl/internal/system"
)

// Users delegates account operations to callbacks supplied by a test.
type Users struct {
	LookupFunc       func(string) (*user.User, error)
	LookupIDFunc     func(string) (*user.User, error)
	CreateFunc       func(context.Context, system.CreateUserOptions) error
	SetShellFunc     func(context.Context, string, string) error
	LockPasswordFunc func(context.Context, string) error
	SetPasswordFunc  func(context.Context, string, string) error
	DeleteFunc       func(context.Context, string, bool) error
}

func (f *Users) Lookup(name string) (*user.User, error)  { return f.LookupFunc(name) }
func (f *Users) LookupID(uid string) (*user.User, error) { return f.LookupIDFunc(uid) }
func (f *Users) Create(ctx context.Context, opts system.CreateUserOptions) error {
	return f.CreateFunc(ctx, opts)
}
func (f *Users) SetShell(ctx context.Context, name, shell string) error {
	return f.SetShellFunc(ctx, name, shell)
}
func (f *Users) LockPassword(ctx context.Context, name string) error {
	return f.LockPasswordFunc(ctx, name)
}
func (f *Users) SetPassword(ctx context.Context, name, password string) error {
	return f.SetPasswordFunc(ctx, name, password)
}
func (f *Users) Delete(ctx context.Context, name string, removeHome bool) error {
	return f.DeleteFunc(ctx, name, removeHome)
}
