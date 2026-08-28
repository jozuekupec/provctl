// Package fake provides deterministic system implementations for unit tests.
package fake

import (
	"context"
	"io"
	"sync"

	"provctl/internal/system"
)

type CommandCall struct {
	Name     string
	Args     []string
	HasStdin bool
}

// Commander records calls and can fail a selected 1-based invocation.
// It never retains stdin, so tests cannot accidentally expose credentials.
type Commander struct {
	mu     sync.Mutex
	Calls  []CommandCall
	FailAt int
	Err    error
	Result system.Result
}

func (fake *Commander) Run(ctx context.Context, name string, args ...string) (system.Result, error) {
	return fake.record(ctx, name, args, false)
}

func (fake *Commander) RunWithStdin(ctx context.Context, _ io.Reader, name string, args ...string) (system.Result, error) {
	return fake.record(ctx, name, args, true)
}

func (fake *Commander) record(ctx context.Context, name string, args []string, hasStdin bool) (system.Result, error) {
	if err := ctx.Err(); err != nil {
		return system.Result{}, err
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.Calls = append(fake.Calls, CommandCall{Name: name, Args: append([]string(nil), args...), HasStdin: hasStdin})
	if fake.FailAt > 0 && len(fake.Calls) == fake.FailAt {
		return fake.Result, fake.Err
	}
	return fake.Result, nil
}
