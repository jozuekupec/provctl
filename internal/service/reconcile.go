package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

// ReconcileStore is the read-only state required to regenerate HTTP vhosts.
type ReconcileStore interface {
	ListSubscriptions(context.Context) ([]domain.Subscription, error)
	ListWebsites(context.Context, int64) ([]domain.Website, error)
}

// Drift describes one generated Apache artifact that differs from SQLite state.
type Drift struct {
	Path     string
	Current  []byte
	Expected []byte
}

// DriftError makes a dry-run report configuration drift with the conventional
// diff exit status while retaining the report already written to stdout.
type DriftError struct{}

func (DriftError) Error() string { return "generated configuration differs from SQLite state" }
func (DriftError) ExitCode() int { return 2 }

// ReconcileService restores generated HTTP vhosts from the SQLite source of truth.
type ReconcileService struct {
	FS       system.FS
	Store    ReconcileStore
	Apache   ApacheVHostApplier
	Executor plan.Executor
	Config   config.Config
}

// ReconcileRuntime owns the database connection used by reconcile commands.
type ReconcileRuntime struct {
	Service    ReconcileService
	repository *sqlite.Repository
}

func NewProductionReconcileRuntime(ctx context.Context, cfg config.Config) (*ReconcileRuntime, error) {
	repository, err := sqlite.Open(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	systemd := system.CommandSystemd{Commander: commander}
	return &ReconcileRuntime{Service: ReconcileService{
		FS:       system.OSFS{},
		Store:    repository,
		Apache:   Apache{FS: system.OSFS{}, Commands: commander, Systemd: systemd, Service: cfg.Apache.Service},
		Executor: plan.Executor{Journal: sqlite.OperationJournal{DB: repository.DB}, Locker: system.FileLocker{Path: meta.LockFile}},
		Config:   cfg,
	}, repository: repository}, nil
}

func NewReadOnlyReconcileRuntime(ctx context.Context, cfg config.Config) (*ReconcileRuntime, error) {
	repository, err := sqlite.OpenReadOnly(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	return &ReconcileRuntime{Service: ReconcileService{FS: system.OSFS{}, Store: repository, Config: cfg}, repository: repository}, nil
}

func (runtime *ReconcileRuntime) Close() error { return runtime.repository.Close() }

// Inspect reports generated HTTP vhost drift without changing the system.
func (service ReconcileService) Inspect(ctx context.Context, subscriptionName string) ([]Drift, error) {
	if service.FS == nil || service.Store == nil {
		return nil, errors.New("reconcile requires filesystem and store")
	}
	subscriptions, err := service.Store.ListSubscriptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	if subscriptionName != "" {
		if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
			return nil, err
		}
		found := false
		for _, subscription := range subscriptions {
			if subscription.Name == subscriptionName {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("subscription %q not found", subscriptionName)
		}
	}

	var drifts []Drift
	for _, subscription := range subscriptions {
		if subscriptionName != "" && subscription.Name != subscriptionName {
			continue
		}
		websites, err := service.Store.ListWebsites(ctx, subscription.ID)
		if err != nil {
			return nil, fmt.Errorf("list websites for subscription %q: %w", subscription.Name, err)
		}
		for _, website := range websites {
			expected, err := service.renderHTTPVHost(subscription.Name, website)
			if err != nil {
				return nil, err
			}
			path := service.vhostPath(subscription.Name, website.PrimaryDomain)
			current, err := service.readFileOrEmpty(path)
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(current, expected) {
				drifts = append(drifts, Drift{Path: path, Current: current, Expected: expected})
			}
			enabledPath := filepath.Join(service.Config.Apache.SitesEnabled, filepath.Base(path))
			enabled, err := service.pathExists(enabledPath)
			if err != nil {
				return nil, err
			}
			if enabled != website.Enabled {
				currentState := []byte("disabled\n")
				if enabled {
					currentState = []byte("enabled\n")
				}
				expectedState := []byte("disabled\n")
				if website.Enabled {
					expectedState = []byte("enabled\n")
				}
				drifts = append(drifts, Drift{Path: enabledPath, Current: currentState, Expected: expectedState})
			}
		}
	}
	return drifts, nil
}

// Prepare builds the reversible changes needed to remove detected drift.
func (service ReconcileService) Prepare(ctx context.Context, subscriptionName string) (plan.Plan, []Drift, error) {
	if service.Apache == nil {
		return plan.Plan{}, nil, errors.New("reconcile requires an Apache vhost applier")
	}
	drifts, err := service.Inspect(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, nil, err
	}
	steps := make([]plan.Step, 0, len(drifts))
	for _, drift := range drifts {
		drift := drift
		if filepath.Dir(drift.Path) == service.Config.Apache.SitesEnabled {
			enabled := bytes.Equal(drift.Expected, []byte("enabled\n"))
			vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, filepath.Base(drift.Path))
			var undo func(context.Context) error
			steps = append(steps, plan.Step{Name: "restore Apache vhost enabled state", Preview: fmt.Sprintf("set %s enabled=%t", drift.Path, enabled), Do: func(ctx context.Context) error {
				var err error
				undo, err = service.Apache.SetVHostEnabled(ctx, vhostPath, drift.Path, enabled)
				return err
			}, Undo: func(ctx context.Context) error {
				if undo == nil {
					return nil
				}
				return undo(ctx)
			}})
			continue
		}
		var undo func(context.Context) error
		steps = append(steps, plan.Step{Name: "restore generated Apache vhost", Preview: "write " + drift.Path, Do: func(ctx context.Context) error {
			var err error
			undo, err = service.Apache.Apply(ctx, drift.Path, drift.Expected)
			return err
		}, Undo: func(ctx context.Context) error {
			if undo == nil {
				return nil
			}
			return undo(ctx)
		}})
	}
	return plan.Plan{Action: "reconcile", Target: reconcileTarget(subscriptionName), Steps: steps}, drifts, nil
}

// Reconcile applies only detected drift; an already reconciled system is a no-op.
func (service ReconcileService) Reconcile(ctx context.Context, subscriptionName string) (int64, error) {
	operation, drifts, err := service.Prepare(ctx, subscriptionName)
	if err != nil {
		return 0, err
	}
	if len(drifts) == 0 {
		return 0, nil
	}
	return service.Executor.Run(ctx, operation)
}

func (service ReconcileService) renderHTTPVHost(subscriptionName string, website domain.Website) ([]byte, error) {
	return WebsiteService{Config: service.Config}.RenderVHost(subscriptionName, website)
}

func (service ReconcileService) vhostPath(subscriptionName, domain string) string {
	return filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscriptionName+"-"+domain+".conf")
}

func (service ReconcileService) readFileOrEmpty(path string) ([]byte, error) {
	contents, err := service.FS.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read generated Apache vhost %q: %w", path, err)
	}
	return contents, nil
}

func (service ReconcileService) pathExists(path string) (bool, error) {
	_, err := service.FS.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect Apache vhost state %q: %w", path, err)
	}
	return true, nil
}

func reconcileTarget(subscriptionName string) string {
	if subscriptionName == "" {
		return "all subscriptions"
	}
	return subscriptionName
}

// UnifiedDiff returns a small line-oriented unified diff for dry-run output.
func UnifiedDiff(path string, current, expected []byte) string {
	from, to := splitLines(current), splitLines(expected)
	var builder strings.Builder
	path = strings.TrimPrefix(path, "/")
	fmt.Fprintf(&builder, "--- current/%s\n+++ generated/%s\n@@ -1,%d +1,%d @@\n", path, path, len(from), len(to))
	for _, line := range lineDiff(from, to) {
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func splitLines(contents []byte) []string {
	trimmed := strings.TrimSuffix(string(contents), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func lineDiff(from, to []string) []string {
	lengths := make([][]int, len(from)+1)
	for index := range lengths {
		lengths[index] = make([]int, len(to)+1)
	}
	for i := len(from) - 1; i >= 0; i-- {
		for j := len(to) - 1; j >= 0; j-- {
			if from[i] == to[j] {
				lengths[i][j] = lengths[i+1][j+1] + 1
			} else if lengths[i+1][j] >= lengths[i][j+1] {
				lengths[i][j] = lengths[i+1][j]
			} else {
				lengths[i][j] = lengths[i][j+1]
			}
		}
	}
	var lines []string
	for i, j := 0, 0; i < len(from) || j < len(to); {
		if i < len(from) && j < len(to) && from[i] == to[j] {
			lines = append(lines, " "+from[i])
			i++
			j++
		} else if j < len(to) && (i == len(from) || lengths[i][j+1] >= lengths[i+1][j]) {
			lines = append(lines, "+"+to[j])
			j++
		} else {
			lines = append(lines, "-"+from[i])
			i++
		}
	}
	return lines
}
