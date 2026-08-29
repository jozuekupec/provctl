package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRenderApachePHPFPMHTTP_Golden(t *testing.T) {
	vhost := ApacheHTTPVHost{
		Subscription: "acme", PrimaryDomain: "example.test", Aliases: []string{"www.example.test"},
		DocumentRoot: "/var/www/vhosts/acme/sites/example.test/public", AcmeChallengeRoot: "/var/lib/provctl/acme-challenge",
		FPMSocket: "/run/php/provctl-acme.sock", ProxyTimeout: 60, LogDir: "/var/log/provctl/acme/example.test",
	}
	got, err := RenderApachePHPFPMHTTP(vhost)
	if err != nil {
		t.Fatalf("RenderApachePHPFPMHTTP() error = %v", err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "php-fpm-http.golden"))
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}
	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Errorf("render mismatch (-want +got):\n%s", diff)
	}
}

func TestRenderApachePHPFPMHTTP_ForceHTTPSKeepsACME(t *testing.T) {
	vhost := ApacheHTTPVHost{Subscription: "acme", PrimaryDomain: "example.test", DocumentRoot: "/srv/public", AcmeChallengeRoot: "/srv/acme", ForceHTTPS: true, FPMSocket: "/run/php/acme.sock", ProxyTimeout: 60, LogDir: "/var/log/provctl/acme/example.test"}
	got, err := RenderApachePHPFPMHTTP(vhost)
	if err != nil {
		t.Fatalf("RenderApachePHPFPMHTTP() error = %v", err)
	}
	contents := string(got)
	if !containsAll(contents, "Alias /.well-known/acme-challenge/ /srv/acme/", "RewriteRule ^ https://%{SERVER_NAME}%{REQUEST_URI}", "!^/\\.well-known/acme-challenge/") {
		t.Errorf("forced HTTPS vhost does not preserve the ACME exception:\n%s", contents)
	}
}

func containsAll(contents string, values ...string) bool {
	for _, value := range values {
		if !strings.Contains(contents, value) {
			return false
		}
	}
	return true
}
