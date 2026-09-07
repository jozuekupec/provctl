package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Certificate is the persisted cache of one Certbot lineage. The certificate
// files remain authoritative because Certbot can renew them independently.
type Certificate struct {
	ID             int64
	SubscriptionID int64
	WebsiteID      int64
	Lineage        string
	PrimaryDomain  string
	SANs           []string
	Issuer         string
	NotBefore      time.Time
	NotAfter       time.Time
	LastCheckedAt  time.Time
}

var certificateName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)

// ValidateCertificateName accepts a safe Certbot name, including legacy names.
// It does not establish ownership or authorize deletion of the certificate.
func ValidateCertificateName(lineage string) error {
	if !certificateName.MatchString(lineage) {
		return fmt.Errorf("invalid certificate lineage %q", lineage)
	}
	return nil
}

func ValidateCertificateLineage(lineage string) error {
	if err := ValidateCertificateName(lineage); err != nil {
		return err
	}
	if !strings.HasPrefix(lineage, "provctl-") {
		return fmt.Errorf("certificate lineage %q is not managed by provctl", lineage)
	}
	return nil
}
