package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"provctl/internal/domain"
)

func TestRepository_CronJobsLifecycle(t *testing.T) {
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
	id, err := repository.CreateCronJob(ctx, domain.CronJob{SubscriptionID: subscription.ID, Schedule: "@daily", Command: "/usr/bin/true", Enabled: true, Comment: "health"})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := repository.ListCronJobs(ctx, subscription.ID)
	if err != nil || len(jobs) != 1 || jobs[0].ID != id || jobs[0].Comment != "health" {
		t.Fatalf("ListCronJobs() = %#v, %v", jobs, err)
	}
	if err := repository.DeleteCronJob(ctx, subscription.ID, id); err != nil {
		t.Fatal(err)
	}
}
