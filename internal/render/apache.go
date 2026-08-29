// Package render produces deterministic configuration artifacts from domain state.
package render

import (
	"bytes"
	"fmt"
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

func RenderApachePHPFPMHTTP(vhost ApacheHTTPVHost) ([]byte, error) {
	if vhost.Subscription == "" || vhost.PrimaryDomain == "" || vhost.DocumentRoot == "" || vhost.AcmeChallengeRoot == "" || vhost.FPMSocket == "" || vhost.ProxyTimeout <= 0 || vhost.LogDir == "" {
		return nil, fmt.Errorf("incomplete Apache HTTP vhost input")
	}
	templateContents, err := projecttemplates.Files.ReadFile("apache/php-fpm-http.conf.tmpl")
	if err != nil {
		return nil, fmt.Errorf("read Apache HTTP template: %w", err)
	}
	parsed, err := template.New("php-fpm-http.conf.tmpl").Option("missingkey=error").Parse(string(templateContents))
	if err != nil {
		return nil, fmt.Errorf("parse Apache HTTP template: %w", err)
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, vhost); err != nil {
		return nil, fmt.Errorf("render Apache HTTP template: %w", err)
	}
	return output.Bytes(), nil
}
