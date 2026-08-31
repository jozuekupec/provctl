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

func TestRenderApachePHPFPMTLS_UsesLiveLineage(t *testing.T) {
	contents, err := RenderApachePHPFPMTLS(ApacheTLSVHost{Subscription: "acme", PrimaryDomain: "example.test", Aliases: []string{"www.example.test"}, DocumentRoot: "/srv/public", CertificateFile: "/etc/letsencrypt/live/provctl-acme-example.test/fullchain.pem", CertificateKey: "/etc/letsencrypt/live/provctl-acme-example.test/privkey.pem", FPMSocket: "/run/php/provctl-acme.sock", ProxyTimeout: 60, LogDir: "/var/log/provctl/acme/example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(string(contents), "<VirtualHost *:443>", "SSLEngine on", "SSLCertificateFile /etc/letsencrypt/live/provctl-acme-example.test/fullchain.pem", "ServerAlias www.example.test") {
		t.Errorf("TLS vhost is incomplete:\n%s", contents)
	}
}

func TestRenderApacheStaticHTTP_ContainsIsolatedDocumentRootAndACME(t *testing.T) {
	contents, err := RenderApacheStaticHTTP(ApacheStaticVHost{PrimaryDomain: "static.example.test", Aliases: []string{"www.static.example.test"}, DocumentRoot: "/vhosts/acme/sites/static.example.test/public", AcmeChallengeRoot: "/state/acme", LogDir: "/logs/acme/static.example.test"})
	if err != nil {
		t.Fatalf("RenderApacheStaticHTTP() error = %v", err)
	}
	if !containsAll(string(contents), "DocumentRoot /vhosts/acme/sites/static.example.test/public", "AllowOverride None", "Alias /.well-known/acme-challenge/ /state/acme/") {
		t.Errorf("static vhost is missing required directives:\n%s", contents)
	}
	assertApacheGolden(t, "static-http.golden", contents)
}

func TestRenderApacheProxyHTTP_RestrictsTargetAndSetsHeaders(t *testing.T) {
	contents, err := RenderApacheProxyHTTP(ApacheProxyVHost{PrimaryDomain: "proxy.example.test", Target: "http://127.0.0.1:8080/", AcmeChallengeRoot: "/state/acme", ProxyTimeout: 60, LogDir: "/logs/acme/proxy.example.test"}, nil)
	if err != nil {
		t.Fatalf("RenderApacheProxyHTTP() error = %v", err)
	}
	if !containsAll(string(contents), "ProxyRequests Off", "ProxyPass / http://127.0.0.1:8080/ timeout=60", "RequestHeader set X-Forwarded-Proto \"http\"") {
		t.Errorf("proxy vhost is missing required directives:\n%s", contents)
	}
	assertApacheGolden(t, "proxy-http.golden", contents)
	if _, err := RenderApacheProxyHTTP(ApacheProxyVHost{PrimaryDomain: "proxy.example.test", Target: "http://10.0.0.1:8080", AcmeChallengeRoot: "/state/acme", ProxyTimeout: 60, LogDir: "/logs/acme/proxy.example.test"}, nil); err == nil {
		t.Error("RenderApacheProxyHTTP() accepted a non-allowlisted target")
	}
}

func TestRenderApacheRedirectHTTP_ValidatesTargetAndStatus(t *testing.T) {
	contents, err := RenderApacheRedirectHTTP(ApacheRedirectVHost{PrimaryDomain: "old.example.test", Target: "https://new.example.test/path", RedirectCode: 301, AcmeChallengeRoot: "/state/acme", LogDir: "/logs/acme/old.example.test"})
	if err != nil {
		t.Fatalf("RenderApacheRedirectHTTP() error = %v", err)
	}
	if !containsAll(string(contents), "Redirect 301 / https://new.example.test/path", "Alias /.well-known/acme-challenge/ /state/acme/") {
		t.Errorf("redirect vhost is missing required directives:\n%s", contents)
	}
	assertApacheGolden(t, "redirect-http.golden", contents)
	if _, err := RenderApacheRedirectHTTP(ApacheRedirectVHost{PrimaryDomain: "old.example.test", Target: "mailto:test@example.test", RedirectCode: 301, AcmeChallengeRoot: "/state/acme", LogDir: "/logs/acme/old.example.test"}); err == nil {
		t.Error("RenderApacheRedirectHTTP() accepted a non-HTTP target")
	}
}

func assertApacheGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}
	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Errorf("render mismatch (-want +got):\n%s", diff)
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
