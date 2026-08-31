package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"provctl/internal/domain"
)

func TestRepository_SSHKeysLifecycle(t *testing.T) {
	repository, err := Open(context.Background(), filepath.Join(t.TempDir(), "provctl.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	ctx := context.Background()
	if err := repository.CreateSubscription(ctx, domain.Subscription{Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme", PHPMaxChildren: 10, PHPMemoryLimit: "256M", PHPUploadMax: "64M", PHPMaxExecTime: 60, SSHAccess: "none"}); err != nil {
		t.Fatal(err)
	}
	subscription, err := repository.SubscriptionByName(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.CreateSSHKey(ctx, domain.SSHKey{SubscriptionID: subscription.ID, Comment: "laptop", Fingerprint: "SHA256:abc", PublicKey: "ssh-ed25519 AAAA test"})
	if err != nil {
		t.Fatal(err)
	}
	keys, err := repository.ListSSHKeys(ctx, subscription.ID)
	if err != nil || len(keys) != 1 || keys[0].Fingerprint != "SHA256:abc" {
		t.Fatalf("ListSSHKeys() = %#v, %v", keys, err)
	}
	if err := repository.UpdateSSHAccess(ctx, subscription.ID, "key"); err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteSSHKey(ctx, subscription.ID, "SHA256:abc"); err != nil {
		t.Fatal(err)
	}
}
