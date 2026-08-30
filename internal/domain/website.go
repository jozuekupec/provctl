package domain

import (
	"fmt"
	"regexp"
	"strings"
)

type WebsiteType string

const (
	WebsitePHPFPM   WebsiteType = "php-fpm"
	WebsiteStatic   WebsiteType = "static"
	WebsiteProxy    WebsiteType = "proxy"
	WebsiteRedirect WebsiteType = "redirect"
)

type Website struct {
	ID             int64
	SubscriptionID int64
	Type           WebsiteType
	PrimaryDomain  string
	DocumentRoot   string
	Target         string
	RedirectCode   int
	PHPVersion     string
	Enabled        bool
	SSLEnabled     bool
	ForceHTTPS     bool
	HSTS           bool
}

var domainName = regexp.MustCompile(`(?i)^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

func ValidateDomain(name string) error {
	if name != strings.ToLower(name) || len(name) > 253 || !domainName.MatchString(name) {
		return fmt.Errorf("domain %q must be a lowercase ASCII domain name", name)
	}
	return nil
}

func ValidateWebsiteType(kind WebsiteType) error {
	switch kind {
	case WebsitePHPFPM, WebsiteStatic, WebsiteProxy, WebsiteRedirect:
		return nil
	default:
		return fmt.Errorf("unsupported website type %q", kind)
	}
}
