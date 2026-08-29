package system

import "context"

// Locker serializes mutating provctl operations across processes.
type Locker interface {
	Lock(ctx context.Context, action string) (Unlock, error)
}

// Unlock releases a previously acquired operation lock.
type Unlock func() error
