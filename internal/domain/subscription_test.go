package domain

import "testing"

func TestValidateSubscriptionName(t *testing.T) {
	for _, name := range []string{"acme", "client-42", "root", "AcmE", "a", "contains_underscore"} {
		err := ValidateSubscriptionName(name)
		valid := name == "acme" || name == "client-42"
		if valid && err != nil {
			t.Errorf("ValidateSubscriptionName(%q) = %v", name, err)
		}
		if !valid && err == nil {
			t.Errorf("ValidateSubscriptionName(%q) = nil, want error", name)
		}
	}
}
