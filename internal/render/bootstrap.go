package render

import (
	"bytes"
	"fmt"
	"text/template"

	projecttemplates "provctl/templates"
)

// DefaultApacheVHost contains the certificate paths for the generated catch-all vhost.
type DefaultApacheVHost struct {
	CertificateFile string
	KeyFile         string
}

func RenderDefaultApacheVHost(vhost DefaultApacheVHost) ([]byte, error) {
	if vhost.CertificateFile == "" || vhost.KeyFile == "" {
		return nil, fmt.Errorf("default Apache vhost requires certificate and key paths")
	}
	templateContents, err := projecttemplates.Files.ReadFile("bootstrap/default-vhost.conf.tmpl")
	if err != nil {
		return nil, fmt.Errorf("read default Apache vhost template: %w", err)
	}
	parsed, err := template.New("default-vhost.conf.tmpl").Option("missingkey=error").Parse(string(templateContents))
	if err != nil {
		return nil, fmt.Errorf("parse default Apache vhost template: %w", err)
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, vhost); err != nil {
		return nil, fmt.Errorf("render default Apache vhost template: %w", err)
	}
	return output.Bytes(), nil
}
