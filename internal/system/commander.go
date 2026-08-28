package system

import (
	"context"
	"io"
	"time"
)

// Commander runs explicitly allowlisted programs with explicit arguments.
// Implementations must never invoke a shell.
type Commander interface {
	Run(ctx context.Context, name string, args ...string) (Result, error)
	RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) (Result, error)
}

type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}
