package system

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFileLocker_ReportsHolderOnTimeout(t *testing.T) {
	locker := FileLocker{Path: t.TempDir() + "/provctl.lock", RetryInterval: time.Millisecond}
	unlock, err := locker.Lock(context.Background(), "subscription.create acme")
	if err != nil {
		t.Fatalf("Lock() error = %v", err)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = locker.Lock(ctx, "subscription.create beta")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Lock() error = %v, want deadline exceeded", err)
	}
	if !strings.Contains(err.Error(), "subscription.create acme") {
		t.Errorf("Lock() error = %v, want holder action", err)
	}
}
