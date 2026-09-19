// Package cron reads the five-field cron expressions a workflow is scheduled
// with, answers which minutes they are due, and works out the runs still to
// come so the editor can show them while the expression is being typed.
//
// The dialect is the familiar one: "minute hour day-of-month month
// day-of-week", with lists, ranges, steps, three-letter month and weekday
// names, and the @hourly family of macros. There is no seconds field: a
// schedule is never due more than once a minute.
package cron

import (
	"fmt"
	"strconv"
	"strings"
)

import "time"

// Schedule is a parsed expression: which minutes, hours, days, months and
// weekdays it is due on.
type Schedule struct {
	minute [60]bool
	hour   [24]bool
	day    [32]bool
	month  [13]bool
	// weekday is indexed by time.Weekday, Sunday first.
	weekday [7]bool
	// A day-of-month and a day-of-week that are both restricted match on
	// either, as cron has always done, so each field remembers whether it
	// was narrowed at all.
	dayRestricted     bool
	weekdayRestricted bool
	expression        string
}

// String is the expression the schedule was read from, tidied of the spaces
// around it.
func (schedule Schedule) String() string {
	return schedule.expression
}

// macros are the shorthands that stand for a whole expression.
var macros = map[string]string{
	"@hourly":   "0 * * * *",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@weekly":   "0 0 * * 0",
	"@monthly":  "0 0 1 * *",
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
}

var weekdayNames = map[string]int{
	"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
}

var monthNames = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

// Parse reads an expression, reporting why it cannot be run rather than
// guessing at what was meant.
func Parse(expression string) (Schedule, error) {
	tidy := strings.Join(strings.Fields(expression), " ")
	if tidy == "" {
		return Schedule{}, fmt.Errorf("a schedule needs an expression such as %q", "0 9 * * 1-5")
	}
	fields := strings.Fields(tidy)
	if strings.HasPrefix(fields[0], "@") {
		if len(fields) > 1 {
			return Schedule{}, fmt.Errorf("%s stands for a whole schedule; it takes no fields after it", fields[0])
		}
		expanded, ok := macros[strings.ToLower(fields[0])]
		if !ok {
			return Schedule{}, fmt.Errorf("%s is not a schedule; the shorthands are @hourly, @daily, @weekly, @monthly and @yearly", fields[0])
		}
		schedule, err := Parse(expanded)
		if err != nil {
			return Schedule{}, err
		}
		schedule.expression = tidy
		return schedule, nil
	}
	if len(fields) != 5 {
		return Schedule{}, fmt.Errorf(
			"a schedule has five fields, minute hour day-of-month month day-of-week, and this one has %d",
			len(fields),
		)
	}
	schedule := Schedule{expression: tidy}
	specs := []struct {
		label  string
		field  string
		low    int
		high   int
		names  map[string]int
		into   []bool
		narrow *bool
	}{
		{"minute", fields[0], 0, 59, nil, schedule.minute[:], nil},
		{"hour", fields[1], 0, 23, nil, schedule.hour[:], nil},
		{"day of month", fields[2], 1, 31, nil, schedule.day[:], &schedule.dayRestricted},
		{"month", fields[3], 1, 12, monthNames, schedule.month[:], nil},
		{"day of week", fields[4], 0, 7, weekdayNames, schedule.weekday[:], &schedule.weekdayRestricted},
	}
	for _, spec := range specs {
		values, err := parseField(spec.field, spec.low, spec.high, spec.names)
		if err != nil {
			return Schedule{}, fmt.Errorf("the %s field %q: %w", spec.label, spec.field, err)
		}
		for _, value := range values {
			// Sunday is both 0 and 7 in cron; the set is indexed the way
			// time.Weekday counts.
			spec.into[value%len(spec.into)] = true
		}
		if spec.narrow != nil {
			*spec.narrow = strings.TrimSpace(spec.field) != "*"
		}
	}
	return schedule, nil
}

// Valid reports whether the expression can be run, for callers that only
// need the answer.
func Valid(expression string) error {
	_, err := Parse(expression)
	return err
}

// parseField reads one comma-separated field into the values it allows.
func parseField(field string, low int, high int, names map[string]int) ([]int, error) {
	var values []int
	for _, entry := range strings.Split(field, ",") {
		if strings.TrimSpace(entry) == "" {
			return nil, fmt.Errorf("has an empty entry")
		}
		part, stepText, hasStep := strings.Cut(entry, "/")
		step := 1
		if hasStep {
			parsed, err := strconv.Atoi(strings.TrimSpace(stepText))
			if err != nil || parsed < 1 {
				return nil, fmt.Errorf("has the step %q, which must be a whole number above zero", stepText)
			}
			step = parsed
		}
		from, to, err := bounds(part, low, high, names, hasStep)
		if err != nil {
			return nil, err
		}
		for value := from; value <= to; value += step {
			values = append(values, value)
		}
	}
	return values, nil
}

// bounds reads the range an entry covers, before its step is applied. A
// single value with a step counts from that value to the end of the field,
// as cron reads "9/2".
func bounds(part string, low int, high int, names map[string]int, hasStep bool) (int, int, error) {
	part = strings.TrimSpace(part)
	if part == "*" {
		return low, high, nil
	}
	first, second, isRange := strings.Cut(part, "-")
	from, err := value(first, low, high, names)
	if err != nil {
		return 0, 0, err
	}
	if !isRange {
		if hasStep {
			return from, high, nil
		}
		return from, from, nil
	}
	to, err := value(second, low, high, names)
	if err != nil {
		return 0, 0, err
	}
	if to < from {
		return 0, 0, fmt.Errorf("has the range %q, which ends before it starts", part)
	}
	return from, to, nil
}

func value(text string, low int, high int, names map[string]int) (int, error) {
	text = strings.TrimSpace(text)
	if named, ok := names[strings.ToLower(text)]; ok {
		return named, nil
	}
	number, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("does not understand %q", text)
	}
	if number < low || number > high {
		return 0, fmt.Errorf("has %d, which is outside %d-%d", number, low, high)
	}
	return number, nil
}

// Matches reports whether the schedule is due in the minute the moment falls
// in, whichever second of it the scheduler happens to look.
func (schedule Schedule) Matches(moment time.Time) bool {
	return schedule.minute[moment.Minute()] &&
		schedule.hour[moment.Hour()] &&
		schedule.matchesDay(moment)
}

func (schedule Schedule) matchesDay(moment time.Time) bool {
	if !schedule.month[int(moment.Month())] {
		return false
	}
	day := schedule.day[moment.Day()]
	weekday := schedule.weekday[int(moment.Weekday())]
	if schedule.dayRestricted && schedule.weekdayRestricted {
		return day || weekday
	}
	return day && weekday
}

// searchDays bounds the search for the next run, so an expression naming a
// date that never comes, such as 30 February, gives up instead of looping.
const searchDays = 5 * 366

// NextRuns returns the next count runs after the moment, in its own time
// zone: a schedule is read in the zone it is asked about, which is the
// server's own when the scheduler asks. A moment that is itself due is not
// one of them; the next run comes after it. Fewer runs come back when the
// expression names a date that never comes round.
func (schedule Schedule) NextRuns(after time.Time, count int) []time.Time {
	runs := make([]time.Time, 0, max(count, 0))
	from := after
	for len(runs) < count {
		next, ok := schedule.next(from)
		if !ok {
			break
		}
		runs = append(runs, next)
		from = next
	}
	return runs
}

// Next is the first run after the moment, and whether there is one at all.
func (schedule Schedule) Next(after time.Time) (time.Time, bool) {
	return schedule.next(after)
}

func (schedule Schedule) next(after time.Time) (time.Time, bool) {
	start := after.Truncate(time.Minute).Add(time.Minute)
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	for searched := 0; searched < searchDays; searched++ {
		if schedule.matchesDay(day) {
			for hour := range 24 {
				if !schedule.hour[hour] {
					continue
				}
				for minute := range 60 {
					if !schedule.minute[minute] {
						continue
					}
					candidate := time.Date(
						day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, day.Location(),
					)
					if !candidate.Before(start) {
						return candidate, true
					}
				}
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	return time.Time{}, false
}
