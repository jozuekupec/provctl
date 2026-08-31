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

func TestValidateSSHAccess(t *testing.T) {
	for _, access := range []string{"none", "key", "password", "key+password"} {
		if err := ValidateSSHAccess(access); err != nil {
			t.Errorf("ValidateSSHAccess(%q) = %v", access, err)
		}
	}
	if err := ValidateSSHAccess("all"); err == nil {
		t.Error("ValidateSSHAccess(\"all\") = nil")
	}
}
