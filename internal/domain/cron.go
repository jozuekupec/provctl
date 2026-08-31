package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// CronJob is one persisted entry in a subscription's generated crontab.
type CronJob struct {
	ID             int64
	SubscriptionID int64
	Schedule       string
	Command        string
	Enabled        bool
	Comment        string
}

var cronMacros = map[string]struct{}{
	"@reboot": {}, "@yearly": {}, "@annually": {}, "@monthly": {},
	"@weekly": {}, "@daily": {}, "@midnight": {}, "@hourly": {},
}

// ValidateCronSchedule accepts the standard five-field crontab syntax and
// its named schedule shortcuts. It intentionally validates only scheduling,
// never the command to execute.
func ValidateCronSchedule(schedule string) error {
	if strings.TrimSpace(schedule) != schedule || strings.ContainsAny(schedule, "\r\n") || schedule == "" {
		return fmt.Errorf("cron schedule must be one non-empty line")
	}
	if strings.HasPrefix(schedule, "@") {
		if _, ok := cronMacros[schedule]; !ok {
			return fmt.Errorf("unsupported cron schedule %q", schedule)
		}
		return nil
	}
	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return fmt.Errorf("cron schedule must have five fields")
	}
	limits := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}}
	for index, field := range fields {
		if err := validateCronField(field, limits[index][0], limits[index][1], index); err != nil {
			return fmt.Errorf("cron field %d: %w", index+1, err)
		}
	}
	return nil
}

// ValidateCronCommand permits any one-line command. It becomes part of the
// user's own crontab and must not be able to create another entry.
func ValidateCronCommand(command string) error {
	if command == "" || strings.ContainsAny(command, "\r\n") {
		return fmt.Errorf("cron command must be one non-empty line")
	}
	return nil
}

// ValidateCronComment ensures a rendered comment cannot alter the crontab.
func ValidateCronComment(comment string) error {
	if strings.ContainsAny(comment, "\r\n") {
		return fmt.Errorf("cron comment must be one line")
	}
	return nil
}

func validateCronField(field string, minimum, maximum, index int) error {
	if field == "" {
		return fmt.Errorf("empty field")
	}
	for _, part := range strings.Split(field, ",") {
		if err := validateCronPart(part, minimum, maximum, index); err != nil {
			return err
		}
	}
	return nil
}

func validateCronPart(part string, minimum, maximum, index int) error {
	base, step, hasStep := strings.Cut(part, "/")
	if strings.Contains(step, "/") || base == "" || (hasStep && !validCronNumber(step, 1, maximum-minimum+1)) {
		return fmt.Errorf("invalid expression %q", part)
	}
	if base == "*" {
		return nil
	}
	start, end, hasRange := strings.Cut(base, "-")
	if strings.Contains(end, "-") || !validCronValue(start, minimum, maximum, index) {
		return fmt.Errorf("invalid expression %q", part)
	}
	if hasRange {
		if !validCronValue(end, minimum, maximum, index) || cronValueOrder(start, index) > cronValueOrder(end, index) {
			return fmt.Errorf("invalid range %q", base)
		}
	}
	return nil
}

func validCronNumber(value string, minimum, maximum int) bool {
	number, err := strconv.Atoi(value)
	return err == nil && number >= minimum && number <= maximum
}

func validCronValue(value string, minimum, maximum, index int) bool {
	if validCronNumber(value, minimum, maximum) {
		return true
	}
	if index == 3 {
		_, ok := map[string]int{"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6, "JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12}[strings.ToUpper(value)]
		return ok
	}
	if index == 4 {
		_, ok := map[string]int{"SUN": 0, "MON": 1, "TUE": 2, "WED": 3, "THU": 4, "FRI": 5, "SAT": 6}[strings.ToUpper(value)]
		return ok
	}
	return false
}

func cronValueOrder(value string, index int) int {
	if number, err := strconv.Atoi(value); err == nil {
		return number
	}
	if index == 3 {
		return map[string]int{"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6, "JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12}[strings.ToUpper(value)]
	}
	return map[string]int{"SUN": 0, "MON": 1, "TUE": 2, "WED": 3, "THU": 4, "FRI": 5, "SAT": 6}[strings.ToUpper(value)]
}
