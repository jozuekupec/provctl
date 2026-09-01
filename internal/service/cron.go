package service

import (
	"context"
	"fmt"
	"strings"

	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

// CronStore persists cron-job metadata, which is the source of truth for the
// generated crontab.
type CronStore interface {
	SubscriptionByName(context.Context, string) (domain.Subscription, error)
	ListCronJobs(context.Context, int64) ([]domain.CronJob, error)
	CreateCronJob(context.Context, domain.CronJob) (int64, error)
	DeleteCronJob(context.Context, int64, int64) error
}

// CronService applies the cron_jobs database state through crontab(1).
type CronService struct {
	Commands system.Commander
	Store    CronStore
	Executor plan.Executor
}

// CronRuntime owns the SQLite connection used by cron commands.
type CronRuntime struct {
	Service    CronService
	repository *sqlite.Repository
}

func NewProductionCronRuntime(ctx context.Context) (*CronRuntime, error) {
	repository, err := sqlite.Open(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	return &CronRuntime{Service: CronService{Commands: system.ExecCommander{}, Store: repository, Executor: productionExecutor(repository)}, repository: repository}, nil
}

func NewReadOnlyCronRuntime(ctx context.Context) (*CronRuntime, error) {
	repository, err := sqlite.OpenReadOnly(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	return &CronRuntime{Service: CronService{Store: repository}, repository: repository}, nil
}

func (runtime *CronRuntime) Close() error { return runtime.repository.Close() }

func (service CronService) List(ctx context.Context, subscriptionName string) ([]domain.CronJob, error) {
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return nil, err
	}
	if service.Store == nil {
		return nil, fmt.Errorf("cron store is required")
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return nil, err
	}
	return service.Store.ListCronJobs(ctx, subscription.ID)
}

func (service CronService) Add(ctx context.Context, subscriptionName, schedule, command, comment string) (int64, error) {
	operation, err := service.PrepareAdd(ctx, subscriptionName, schedule, command, comment)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service CronService) PrepareAdd(ctx context.Context, subscriptionName, schedule, command, comment string) (plan.Plan, error) {
	if err := domain.ValidateCronSchedule(schedule); err != nil {
		return plan.Plan{}, err
	}
	if err := domain.ValidateCronCommand(command); err != nil {
		return plan.Plan{}, err
	}
	if err := domain.ValidateCronComment(comment); err != nil {
		return plan.Plan{}, err
	}
	subscription, jobs, err := service.subscriptionJobs(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	job := domain.CronJob{SubscriptionID: subscription.ID, Schedule: schedule, Command: command, Enabled: true, Comment: comment}
	desired := append(append([]domain.CronJob(nil), jobs...), job)
	var undoCrontab func(context.Context) error
	steps := []plan.Step{{Name: "write generated crontab", Preview: "write crontab for " + subscription.UnixUser, Do: func(ctx context.Context) error {
		var writeErr error
		undoCrontab, writeErr = service.writeCrontab(ctx, subscription, desired)
		return writeErr
	}, Undo: func(ctx context.Context) error {
		if undoCrontab == nil {
			return nil
		}
		return undoCrontab(ctx)
	}}, {Name: "record cron job in SQLite", Preview: "insert cron job for " + subscription.Name, Do: func(ctx context.Context) error {
		id, err := service.Store.CreateCronJob(ctx, job)
		job.ID = id
		return err
	}, Undo: func(ctx context.Context) error { return service.Store.DeleteCronJob(ctx, subscription.ID, job.ID) }}}
	return plan.Plan{Action: "cron.add", Target: subscription.Name, Steps: steps}, nil
}

func (service CronService) Remove(ctx context.Context, subscriptionName string, jobID int64) (int64, error) {
	operation, err := service.PrepareRemove(ctx, subscriptionName, jobID)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service CronService) PrepareRemove(ctx context.Context, subscriptionName string, jobID int64) (plan.Plan, error) {
	subscription, jobs, err := service.subscriptionJobs(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	if jobID < 1 {
		return plan.Plan{}, fmt.Errorf("cron job ID must be positive")
	}
	var removed domain.CronJob
	desired := make([]domain.CronJob, 0, len(jobs)-1)
	for _, job := range jobs {
		if job.ID == jobID {
			removed = job
			continue
		}
		desired = append(desired, job)
	}
	if removed.ID == 0 {
		return plan.Plan{}, fmt.Errorf("cron job %d not found", jobID)
	}
	var undoCrontab func(context.Context) error
	steps := []plan.Step{{Name: "write generated crontab", Preview: "write crontab for " + subscription.UnixUser, Do: func(ctx context.Context) error {
		var writeErr error
		undoCrontab, writeErr = service.writeCrontab(ctx, subscription, desired)
		return writeErr
	}, Undo: func(ctx context.Context) error {
		if undoCrontab == nil {
			return nil
		}
		return undoCrontab(ctx)
	}}, {Name: "remove cron job from SQLite", Preview: fmt.Sprintf("delete cron job %d", jobID), Do: func(ctx context.Context) error {
		return service.Store.DeleteCronJob(ctx, subscription.ID, jobID)
	}, Undo: func(ctx context.Context) error {
		_, err := service.Store.CreateCronJob(ctx, removed)
		return err
	}}}
	return plan.Plan{Action: "cron.remove", Target: fmt.Sprintf("%s/%d", subscription.Name, jobID), Steps: steps}, nil
}

func (service CronService) subscriptionJobs(ctx context.Context, subscriptionName string) (domain.Subscription, []domain.CronJob, error) {
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return domain.Subscription{}, nil, err
	}
	if service.Store == nil || service.Commands == nil {
		return domain.Subscription{}, nil, fmt.Errorf("cron store and commander are required")
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return domain.Subscription{}, nil, err
	}
	if subscription.Status != "active" {
		return domain.Subscription{}, nil, fmt.Errorf("subscription %q is %s", subscriptionName, subscription.Status)
	}
	jobs, err := service.Store.ListCronJobs(ctx, subscription.ID)
	if err != nil {
		return domain.Subscription{}, nil, fmt.Errorf("list cron jobs: %w", err)
	}
	return subscription, jobs, nil
}

func (service CronService) writeCrontab(ctx context.Context, subscription domain.Subscription, jobs []domain.CronJob) (func(context.Context) error, error) {
	previous, missing, err := service.readCrontab(ctx, subscription.UnixUser)
	if err != nil {
		return nil, err
	}
	if _, err := service.Commands.RunWithStdin(ctx, strings.NewReader(renderCrontab(jobs)), "/usr/bin/crontab", "-u", subscription.UnixUser, "-"); err != nil {
		return nil, fmt.Errorf("write crontab for %q: %w", subscription.UnixUser, err)
	}
	return func(ctx context.Context) error {
		if missing {
			_, err := service.Commands.Run(ctx, "/usr/bin/crontab", "-u", subscription.UnixUser, "-r")
			return err
		}
		_, err := service.Commands.RunWithStdin(ctx, strings.NewReader(previous), "/usr/bin/crontab", "-u", subscription.UnixUser, "-")
		return err
	}, nil
}

func (service CronService) readCrontab(ctx context.Context, user string) (string, bool, error) {
	result, err := service.Commands.Run(ctx, "/usr/bin/crontab", "-u", user, "-l")
	if err == nil {
		return result.Stdout, false, nil
	}
	if result.ExitCode == 1 && strings.TrimSpace(result.Stdout) == "" {
		return "", true, nil
	}
	return "", false, fmt.Errorf("read crontab for %q: %w", user, err)
}

func renderCrontab(jobs []domain.CronJob) string {
	var output strings.Builder
	output.WriteString("# GENERATED BY PROVCTL - DO NOT EDIT\n")
	for _, job := range jobs {
		if !job.Enabled {
			continue
		}
		output.WriteString(job.Schedule)
		output.WriteByte(' ')
		output.WriteString(job.Command)
		if job.Comment != "" {
			output.WriteString(" # ")
			output.WriteString(job.Comment)
		}
		output.WriteByte('\n')
	}
	return output.String()
}
