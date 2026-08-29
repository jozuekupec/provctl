package cli

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"

	"provctl/internal/plan"
)

func TestWritePlan_FormatsSteps(t *testing.T) {
	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)
	operation := plan.Plan{Action: "subscription.create", Target: "acme", Steps: []plan.Step{{Name: "create Unix user", Preview: "useradd acme"}}}
	if err := writePlan(command, operation); err != nil {
		t.Fatalf("writePlan() error = %v", err)
	}
	want := "Plan: subscription.create acme\n1. create Unix user\n   useradd acme\n"
	if diff := cmp.Diff(want, output.String()); diff != "" {
		t.Errorf("writePlan() mismatch (-want +got):\n%s", diff)
	}
}
