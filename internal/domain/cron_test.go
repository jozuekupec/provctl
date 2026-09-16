package domain

import (
	"testing"
	"time"
)

func TestValidateCronSchedule_AcceptsStandardSchedules(t *testing.T) {
	for _, schedule := range []string{"*/5 * * * *", "0 4 * * MON-FRI", "@daily", "15 2 1 JAN,MAR *"} {
		if err := ValidateCronSchedule(schedule); err != nil {
			t.Errorf("ValidateCronSchedule(%q) = %v", schedule, err)
		}
	}
}

func TestValidateCronSchedule_RejectsInvalidSchedules(t *testing.T) {
	for _, schedule := range []string{"* * * *", "60 * * * *", "@sometimes", "*/0 * * * *", "* * * * *\n* * * * *"} {
		if err := ValidateCronSchedule(schedule); err == nil {
			t.Errorf("ValidateCronSchedule(%q) accepted invalid schedule", schedule)
		}
	}
}

func TestValidateCronCommand_RejectsMultilineCommand(t *testing.T) {
	if err := ValidateCronCommand("/usr/bin/true\n/bin/false"); err == nil {
		t.Fatal("ValidateCronCommand() accepted a multiline command")
	}
}

func TestNextCronRun_ReportsNextCalendarOccurrence(t *testing.T) {
	from := time.Date(2026, time.September, 16, 10, 15, 42, 0, time.UTC)
	got, err := NextCronRun("*/10 * * * *", from)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.September, 16, 10, 20, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("next cron run = %s, want %s", got, want)
	}
}

func TestNextCronRun_UsesDayOfMonthOrWeekday(t *testing.T) {
	from := time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC) // Wednesday
	got, err := NextCronRun("0 11 1 * 3", from)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.September, 16, 11, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("next cron run = %s, want %s", got, want)
	}
}

func TestNextCronRun_RebootIsNotCalendarSchedule(t *testing.T) {
	if _, err := NextCronRun("@reboot", time.Now()); err == nil {
		t.Fatal("@reboot preview unexpectedly has a calendar time")
	}
}
