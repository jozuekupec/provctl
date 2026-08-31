package domain

import (
	"fmt"
	"strings"
	"time"
)

// Certificate is the persisted cache of one Certbot lineage. The certificate
// files remain authoritative because Certbot can renew them independently.
type Certificate struct {
	ID             int64
	SubscriptionID int64
	Lineage        string
	PrimaryDomain  string
	SANs           []string
	Issuer         string
	NotBefore      time.Time
	NotAfter       time.Time
	LastCheckedAt  time.Time
}

func ValidateCertificateLineage(lineage string) error {
	if !strings.HasPrefix(lineage, "provctl-") || strings.ContainsAny(lineage, "/\\\x00") || len(lineage) > 255 {
		return fmt.Errorf("invalid certificate lineage %q", lineage)
	}
	return nil
}
