// Package domain contains pure business types and validation.
package domain

import (
	"fmt"
	"regexp"
)

var subscriptionName = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}$`)

var reservedSubscriptions = map[string]struct{}{
	"root": {}, "www-data": {}, "mysql": {}, "admin": {}, "daemon": {}, "backup": {}, "provctl": {},
}

type Subscription struct {
	ID             int64
	Name           string
	UnixUser       string
	UnixUID        int
	Home           string
	PHPVersion     string
	PHPMaxChildren int
	PHPMemoryLimit string
	PHPUploadMax   string
	PHPMaxExecTime int
	SSHAccess      string
	Status         string
}

func ValidateSubscriptionName(name string) error {
	if !subscriptionName.MatchString(name) {
		return fmt.Errorf("subscription name %q must match %s", name, subscriptionName)
	}
	if _, reserved := reservedSubscriptions[name]; reserved {
		return fmt.Errorf("subscription name %q is reserved", name)
	}
	return nil
}
