package cli

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"

	"provctl/internal/domain"
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

func TestWriteSubscriptionList_FormatsRows(t *testing.T) {
	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)
	subscriptions := []domain.Subscription{{Name: "acme", UnixUID: 5000, Status: "active", Home: "/var/www/vhosts/acme"}}
	if err := writeSubscriptionList(command, subscriptions); err != nil {
		t.Fatalf("writeSubscriptionList() error = %v", err)
	}
	want := "NAME\tUID\tSTATUS\tHOME\nacme\t5000\tactive\t/var/www/vhosts/acme\n"
	if diff := cmp.Diff(want, output.String()); diff != "" {
		t.Errorf("writeSubscriptionList() mismatch (-want +got):\n%s", diff)
	}
}

func TestWriteWebsite_FormatsProxyFields(t *testing.T) {
	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)
	website := domain.Website{PrimaryDomain: "proxy.example.test", Type: domain.WebsiteProxy, Enabled: true, Target: "http://127.0.0.1:8080"}
	if err := writeWebsite(command, website); err != nil {
		t.Fatalf("writeWebsite() error = %v", err)
	}
	want := "Domain: proxy.example.test\nType: proxy\nEnabled: true\nSSL enabled: false\nDocument root: \nTarget: http://127.0.0.1:8080\nRedirect code: 0\nPHP version: \n"
	if diff := cmp.Diff(want, output.String()); diff != "" {
		t.Errorf("writeWebsite() mismatch (-want +got):\n%s", diff)
	}
}
