// Package render produces deterministic configuration artifacts from domain state.
package render

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"text/template"

	projecttemplates "provctl/templates"
)

// ApacheHTTPVHost is the complete input for a generated PHP-FPM HTTP vhost.
type ApacheHTTPVHost struct {
	Subscription      string
	PrimaryDomain     string
	Aliases           []string
	DocumentRoot      string
	AcmeChallengeRoot string
	ForceHTTPS        bool
	FPMSocket         string
	ProxyTimeout      int
	LogDir            string
}

// ApacheTLSVHost describes the HTTPS half of a PHP-FPM vhost. It is rendered
// separately so issuance can keep the HTTP challenge vhost valid first.
type ApacheTLSVHost struct {
	Subscription    string
	PrimaryDomain   string
	Aliases         []string
	DocumentRoot    string
	CertificateFile string
	CertificateKey  string
	FPMSocket       string
	ProxyTimeout    int
	LogDir          string
}

// ApacheStaticVHost describes an HTTP vhost serving static content.
type ApacheStaticVHost struct {
	PrimaryDomain     string
	Aliases           []string
	DocumentRoot      string
	AcmeChallengeRoot string
	LogDir            string
}

// ApacheProxyVHost describes a loopback-only HTTP reverse proxy vhost.
type ApacheProxyVHost struct {
	PrimaryDomain     string
	Aliases           []string
	Target            string
	Scheme            string
	AcmeChallengeRoot string
	ProxyTimeout      int
	LogDir            string
}

// ApacheRedirectVHost describes an HTTP redirect vhost.
type ApacheRedirectVHost struct {
	PrimaryDomain     string
	Aliases           []string
	Target            string
	RedirectCode      int
	AcmeChallengeRoot string
	LogDir            string
}

func RenderApachePHPFPMHTTP(vhost ApacheHTTPVHost) ([]byte, error) {
	if vhost.Subscription == "" || vhost.PrimaryDomain == "" || vhost.DocumentRoot == "" || vhost.AcmeChallengeRoot == "" || vhost.FPMSocket == "" || vhost.ProxyTimeout <= 0 || vhost.LogDir == "" {
		return nil, fmt.Errorf("incomplete Apache HTTP vhost input")
	}
	return renderApacheTemplate("apache/php-fpm-http.conf.tmpl", vhost)
}

func RenderApachePHPFPMTLS(vhost ApacheTLSVHost) ([]byte, error) {
	if vhost.Subscription == "" || vhost.PrimaryDomain == "" || vhost.DocumentRoot == "" || vhost.CertificateFile == "" || vhost.CertificateKey == "" || vhost.FPMSocket == "" || vhost.ProxyTimeout <= 0 || vhost.LogDir == "" {
		return nil, fmt.Errorf("incomplete Apache TLS vhost input")
	}
	return renderApacheTemplate("apache/php-fpm-tls.conf.tmpl", vhost)
}

func RenderApacheStaticHTTP(vhost ApacheStaticVHost) ([]byte, error) {
	if vhost.PrimaryDomain == "" || vhost.DocumentRoot == "" || vhost.AcmeChallengeRoot == "" || vhost.LogDir == "" {
		return nil, fmt.Errorf("incomplete Apache static vhost input")
	}
	return renderApacheTemplate("apache/static-http.conf.tmpl", vhost)
}

func RenderApacheProxyHTTP(vhost ApacheProxyVHost, allowedHosts []string) ([]byte, error) {
	if vhost.PrimaryDomain == "" || vhost.Target == "" || vhost.AcmeChallengeRoot == "" || vhost.ProxyTimeout <= 0 || vhost.LogDir == "" {
		return nil, fmt.Errorf("incomplete Apache proxy vhost input")
	}
	if err := ValidateProxyTarget(vhost.Target, allowedHosts); err != nil {
		return nil, err
	}
	parsed, _ := url.Parse(vhost.Target)
	vhost.Target = trimTrailingSlash(vhost.Target)
	vhost.Scheme = parsed.Scheme
	return renderApacheTemplate("apache/proxy-http.conf.tmpl", vhost)
}

func RenderApacheRedirectHTTP(vhost ApacheRedirectVHost) ([]byte, error) {
	if vhost.PrimaryDomain == "" || vhost.Target == "" || vhost.AcmeChallengeRoot == "" || vhost.LogDir == "" {
		return nil, fmt.Errorf("incomplete Apache redirect vhost input")
	}
	if vhost.RedirectCode != 301 && vhost.RedirectCode != 302 {
		return nil, fmt.Errorf("redirect code must be 301 or 302")
	}
	parsed, err := url.ParseRequestURI(vhost.Target)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("redirect target must be an absolute HTTP(S) URL")
	}
	return renderApacheTemplate("apache/redirect-http.conf.tmpl", vhost)
}

// ValidateProxyTarget restricts reverse proxies to explicitly allowed local targets.
func ValidateProxyTarget(target string, allowedHosts []string) error {
	parsed, err := url.ParseRequestURI(target)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() == "" || parsed.Port() == "" || parsed.User != nil {
		return fmt.Errorf("proxy target must be an HTTP URL with host and port")
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1024 || port > 65535 {
		return fmt.Errorf("proxy target port must be between 1024 and 65535")
	}
	host := parsed.Hostname()
	if host == "localhost" || net.ParseIP(host).IsLoopback() {
		return nil
	}
	for _, allowed := range allowedHosts {
		if host == allowed {
			return nil
		}
	}
	return fmt.Errorf("proxy target host %q is not allowed", host)
}

func renderApacheTemplate(name string, value any) ([]byte, error) {
	templateContents, err := projecttemplates.Files.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read Apache template %q: %w", name, err)
	}
	parsed, err := template.New(name).Option("missingkey=error").Parse(string(templateContents))
	if err != nil {
		return nil, fmt.Errorf("parse Apache template %q: %w", name, err)
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, value); err != nil {
		return nil, fmt.Errorf("render Apache template %q: %w", name, err)
	}
	return output.Bytes(), nil
}

func trimTrailingSlash(value string) string {
	for len(value) > 0 && value[len(value)-1] == '/' {
		value = value[:len(value)-1]
	}
	return value
}
