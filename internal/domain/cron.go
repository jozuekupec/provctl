package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
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

// NextCronRun returns the next calendar execution after after. The result
// follows Vixie cron's day-of-month/day-of-week OR rule. @reboot has no
// calendar occurrence and returns an explanatory error.
func NextCronRun(schedule string, after time.Time) (time.Time, error) {
	if err := ValidateCronSchedule(schedule); err != nil {
		return time.Time{}, err
	}
	if schedule == "@reboot" {
		return time.Time{}, fmt.Errorf("@reboot runs only when cron starts")
	}
	if strings.HasPrefix(schedule, "@") {
		schedule = map[string]string{
			"@yearly": "0 0 1 1 *", "@annually": "0 0 1 1 *", "@monthly": "0 0 1 * *",
			"@weekly": "0 0 * * 0", "@daily": "0 0 * * *", "@midnight": "0 0 * * *", "@hourly": "0 * * * *",
		}[schedule]
	}
	fields := strings.Fields(schedule)
	candidate := after.In(after.Location()).Truncate(time.Minute).Add(time.Minute)
	const previewMinutes = 366 * 24 * 60
	for count := 0; count < previewMinutes; count, candidate = count+1, candidate.Add(time.Minute) {
		if cronScheduleMatches(fields, candidate) {
			return candidate, nil
		}
	}
	return time.Time{}, fmt.Errorf("no run within the next 366 days")
}

func cronScheduleMatches(fields []string, value time.Time) bool {
	minute, hour := cronFieldMatches(fields[0], value.Minute(), 0, 59, 0), cronFieldMatches(fields[1], value.Hour(), 0, 23, 1)
	month := cronFieldMatches(fields[3], int(value.Month()), 1, 12, 3)
	dayOfMonth := cronFieldMatches(fields[2], value.Day(), 1, 31, 2)
	dayOfWeek := cronFieldMatches(fields[4], int(value.Weekday()), 0, 7, 4)
	day := dayOfMonth && dayOfWeek
	if fields[2] != "*" && fields[4] != "*" {
		day = dayOfMonth || dayOfWeek
	}
	return minute && hour && month && day
}

func cronFieldMatches(expression string, value, minimum, maximum, index int) bool {
	for _, part := range strings.Split(expression, ",") {
		base, stepText, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			step, _ = strconv.Atoi(stepText)
		}
		start, end := minimum, maximum
		if base != "*" {
			first, last, ranged := strings.Cut(base, "-")
			start = cronValueOrder(first, index)
			end = start
			if ranged {
				end = cronValueOrder(last, index)
			}
		}
		candidate := value
		if index == 4 && candidate == 0 && end == 7 {
			candidate = 7
		}
		if candidate >= start && candidate <= end && (candidate-start)%step == 0 {
			return true
		}
	}
	return false
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
