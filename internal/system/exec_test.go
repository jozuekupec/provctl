package system

import (
	"context"
	"errors"
	"testing"
)

func TestIsAllowedBinary(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "/usr/bin/systemctl", want: true},
		{name: "/usr/sbin/php-fpm8.3", want: true},
		{name: "/usr/bin/ssh-keygen", want: true},
		{name: "systemctl", want: false},
		{name: "/bin/sh", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsAllowedBinary(test.name); got != test.want {
				t.Errorf("IsAllowedBinary(%q) = %t, want %t", test.name, got, test.want)
			}
		})
	}
}

func TestExecCommander_RejectsUnallowlistedBinary(t *testing.T) {
	_, err := (ExecCommander{}).Run(context.Background(), "/bin/sh")
	if !errors.Is(err, ErrBinaryNotAllowed) {
		t.Fatalf("Run() error = %v, want ErrBinaryNotAllowed", err)
	}
}
