package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"provctl/internal/domain"
)

func TestRepository_CertificateRoundTrip(t *testing.T) {
	repository, err := Open(context.Background(), filepath.Join(t.TempDir(), "provctl.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.CreateSubscription(context.Background(), domain.Subscription{Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme", PHPMaxChildren: 10, PHPMemoryLimit: "256M", PHPUploadMax: "64M", PHPMaxExecTime: 60, SSHAccess: "none"}); err != nil {
		t.Fatal(err)
	}
	subscription, err := repository.SubscriptionByName(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if _, err := repository.CreateCertificate(context.Background(), domain.Certificate{SubscriptionID: subscription.ID, Lineage: "provctl-acme-example.test", PrimaryDomain: "example.test", SANs: []string{"example.test", "www.example.test"}, NotAfter: want}); err != nil {
		t.Fatal(err)
	}
	certificate, err := repository.CertificateByLineage(context.Background(), "provctl-acme-example.test")
	if err != nil {
		t.Fatal(err)
	}
	if certificate.NotAfter != want || len(certificate.SANs) != 2 {
		t.Errorf("certificate = %#v", certificate)
	}
	updated := want.Add(24 * time.Hour)
	updatedRecord, err := repository.UpdateCertificateNotAfter(context.Background(), certificate.Lineage, updated)
	if err != nil || !updatedRecord {
		t.Fatalf("UpdateCertificateNotAfter() = %t, %v", updatedRecord, err)
	}
	certificate, err = repository.CertificateByLineage(context.Background(), certificate.Lineage)
	if err != nil {
		t.Fatal(err)
	}
	if certificate.NotAfter != updated || certificate.LastCheckedAt.IsZero() {
		t.Errorf("certificate after update = %#v", certificate)
	}
}
