package domain

import "testing"

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
