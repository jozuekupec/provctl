package domain

import "testing"

func TestValidateDomain(t *testing.T) {
	for _, name := range []string{"example.test", "www.example.test", "a-b.example.test"} {
		if err := ValidateDomain(name); err != nil {
			t.Errorf("ValidateDomain(%q) error = %v", name, err)
		}
	}
	for _, name := range []string{"Example.test", "localhost", "example", "-bad.example"} {
		if err := ValidateDomain(name); err == nil {
			t.Errorf("ValidateDomain(%q) error = nil, want failure", name)
		}
	}
}
