package fake

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCommander_FailsConfiguredInvocation(t *testing.T) {
	want := errors.New("command failed")
	commander := Commander{FailAt: 2, Err: want}
	if _, err := commander.Run(context.Background(), "/usr/bin/first"); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if _, err := commander.RunWithStdin(context.Background(), strings.NewReader("secret"), "/usr/bin/second"); !errors.Is(err, want) {
		t.Fatalf("second RunWithStdin() error = %v, want %v", err, want)
	}
	if commander.Calls[1].HasStdin != true {
		t.Error("second command should record stdin use")
	}
}
