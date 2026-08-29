package system

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"
)

// FileLocker uses an advisory Linux flock and records the current owner for diagnostics.
type FileLocker struct {
	Path          string
	RetryInterval time.Duration
}

func (locker FileLocker) Lock(ctx context.Context, action string) (Unlock, error) {
	if locker.Path == "" {
		return nil, fmt.Errorf("lock path is required")
	}
	interval := locker.RetryInterval
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	file, err := os.OpenFile(locker.Path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open operation lock: %w", err)
	}
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			_ = file.Close()
			return nil, fmt.Errorf("acquire operation lock: %w", err)
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			holder, readErr := os.ReadFile(locker.Path)
			if readErr == nil && len(holder) > 0 {
				return nil, fmt.Errorf("wait for operation lock held by %s: %w", string(holder), ctx.Err())
			}
			return nil, fmt.Errorf("wait for operation lock: %w", ctx.Err())
		case <-time.After(interval):
		}
	}
	if err := file.Truncate(0); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("clear operation lock: %w", err)
	}
	if _, err := file.WriteString(strconv.Itoa(os.Getpid()) + " " + action + "\n"); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("write operation lock: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("sync operation lock: %w", err)
	}
	return func() error {
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
			_ = file.Close()
			return fmt.Errorf("release operation lock: %w", err)
		}
		return file.Close()
	}, nil
}
