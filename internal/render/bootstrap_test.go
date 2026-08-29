package render

import (
	"strings"
	"testing"
)

func TestRenderDefaultApacheVHost_DeniesBothProtocols(t *testing.T) {
	contents, err := RenderDefaultApacheVHost(DefaultApacheVHost{CertificateFile: "/state/default.crt", KeyFile: "/state/default.key"})
	if err != nil {
		t.Fatalf("RenderDefaultApacheVHost() error = %v", err)
	}
	for _, expected := range []string{"<VirtualHost *:80>", "<VirtualHost *:443>", "Require all denied", "SSLCertificateFile /state/default.crt", "SSLCertificateKeyFile /state/default.key"} {
		if !strings.Contains(string(contents), expected) {
			t.Errorf("rendered vhost does not contain %q:\n%s", expected, contents)
		}
	}
	if strings.Contains(string(contents), "DocumentRoot") {
		t.Errorf("default vhost must not expose a document root:\n%s", contents)
	}
}
