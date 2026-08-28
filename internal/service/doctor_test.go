package service

import "testing"

func TestHasFailure(t *testing.T) {
	if HasFailure([]Check{{Status: CheckOK}, {Status: CheckWarn}}) {
		t.Error("HasFailure() = true without failed checks")
	}
	if !HasFailure([]Check{{Status: CheckFail}}) {
		t.Error("HasFailure() = false with a failed check")
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("Apache 2.4\nsecond line"); got != "Apache 2.4" {
		t.Errorf("firstLine() = %q", got)
	}
}
