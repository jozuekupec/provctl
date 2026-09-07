package domain

import (
	"strings"
	"testing"
)

func TestValidateCertificateName_LegacyAndUnsafeNames(t *testing.T) {
	for _, name := range []string{"example.test", "example.test-0001", "provctl-site-7"} {
		if err := ValidateCertificateName(name); err != nil {
			t.Errorf("valid name %q: %v", name, err)
		}
	}
	for _, name := range []string{"", ".", "..", "../escape", "a/b", "a\\b", "-option", "a\x00b", "a\nb", strings.Repeat("a", 256)} {
		if err := ValidateCertificateName(name); err == nil {
			t.Errorf("accepted unsafe name %q", name)
		}
	}
	if err := ValidateCertificateLineage("example.test"); err == nil {
		t.Fatal("legacy name passed the managed-lineage guard")
	}
}
