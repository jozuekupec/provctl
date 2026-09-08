package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"provctl/internal/system"
)

func TestCertbotRenewals_FindUsesLiveCertificateSANs(t *testing.T) {
	root := t.TempDir()
	renewal, live := filepath.Join(root, "renewal"), filepath.Join(root, "live")
	for _, directory := range []string{renewal, filepath.Join(live, "legacy-0001")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(renewal, "legacy-0001.conf"), []byte("version = 4.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"example.test", "www.example.test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(live, "legacy-0001", "cert.pem")
	keyDER, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "legacy-0001", "privkey.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "legacy-0001", "fullchain.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := CertbotRenewals{FS: system.OSFS{}, Directory: renewal, LiveDirectory: live}
	lineages, err := manager.Find(context.Background(), "www.example.test")
	if err != nil || len(lineages) != 1 || lineages[0].Name != "legacy-0001" || len(lineages[0].Domains) != 2 {
		t.Fatalf("Find() = %#v, %v", lineages, err)
	}
	lineages, err = manager.Find(context.Background(), "other.test")
	if err != nil || len(lineages) != 0 {
		t.Fatalf("unrelated domain: %#v, %v", lineages, err)
	}
	if err := os.WriteFile(certPath, []byte("invalid certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Find(context.Background(), "example.test"); err == nil {
		t.Fatal("malformed certificate was silently ignored")
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "legacy-0001", "privkey.pem"), []byte("invalid key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Find(context.Background(), "example.test"); err == nil {
		t.Fatal("invalid private key was silently ignored")
	}
}
