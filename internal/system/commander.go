package system

import (
	"context"
	"io"
	"os"
	"time"
)

// Commander runs explicitly allowlisted programs with explicit arguments.
// Implementations must never invoke a shell.
type Commander interface {
	Run(ctx context.Context, name string, args ...string) (Result, error)
	RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) (Result, error)
}

// OutputFileCommander streams a command's stdout into a caller-selected file.
// It is deliberately separate from Commander so existing read-only seams stay
// minimal; implementations must still enforce the binary allowlist.
type OutputFileCommander interface {
	Commander
	RunToFile(ctx context.Context, path string, mode os.FileMode, name string, args ...string) (Result, error)
}

// InputFileCommander streams a caller-selected file to a command's stdin.
type InputFileCommander interface {
	Commander
	RunWithFile(ctx context.Context, path string, name string, args ...string) (Result, error)
}

type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}
