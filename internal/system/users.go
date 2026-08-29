package system

import (
	"context"
	"os/user"
)

type Users interface {
	Lookup(name string) (*user.User, error)
	LookupID(uid string) (*user.User, error)
	Create(ctx context.Context, opts CreateUserOptions) error
	SetShell(ctx context.Context, name, shell string) error
	LockPassword(ctx context.Context, name string) error
	SetPassword(ctx context.Context, name, password string) error
	Delete(ctx context.Context, name string, removeHome bool) error
}

type CreateUserOptions struct {
	Name         string
	UID          int
	GID          int
	Home         string
	Shell        string
	System       bool
	UserGroup    bool
	NoCreateHome bool
}
