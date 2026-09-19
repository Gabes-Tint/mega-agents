package cron_test

import (
	"testing"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/cron"
)

func at(t *testing.T, value string) time.Time {
	t.Helper()
	moment, err := time.ParseInLocation("2006-01-02 15:04", value, time.UTC)
	if err != nil {
		t.Fatalf("cannot read the time %q: %v", value, err)
	}
	return moment
}

func TestNextRunsEveryHourOnTheHalfHour(t *testing.T) {
	schedule, err := cron.Parse("30 * * * *")
	if err != nil {
		t.Fatalf("parsing the expression: %v", err)
	}
	next := schedule.NextRuns(at(t, "2026-09-18 09:45"), 3)
	want := []string{"2026-09-18 10:30", "2026-09-18 11:30", "2026-09-18 12:30"}
	assertRuns(t, next, want)
}

// A moment that matches is not its own next run: the next one comes after it.
func TestNextRunsStartsAfterAMatchingMinute(t *testing.T) {
	schedule, err := cron.Parse("*/15 * * * *")
	if err != nil {
		t.Fatalf("parsing the expression: %v", err)
	}
	assertRuns(t, schedule.NextRuns(at(t, "2026-09-18 09:15"), 2), []string{
		"2026-09-18 09:30", "2026-09-18 09:45",
	})
}

func TestNextRunsCombinesListsRangesAndSteps(t *testing.T) {
	schedule, err := cron.Parse("0 9-17/4 1,15 * *")
	if err != nil {
		t.Fatalf("parsing the expression: %v", err)
	}
	assertRuns(t, schedule.NextRuns(at(t, "2026-09-01 12:00"), 4), []string{
		"2026-09-01 13:00", "2026-09-01 17:00", "2026-09-15 09:00", "2026-09-15 13:00",
	})
}

// A day-of-month and a day-of-week that are both restricted match on either,
// as cron has always done.
func TestNextRunsMatchesEitherDayFieldWhenBothAreRestricted(t *testing.T) {
	schedule, err := cron.Parse("0 0 13 * FRI")
	if err != nil {
		t.Fatalf("parsing the expression: %v", err)
	}
	assertRuns(t, schedule.NextRuns(at(t, "2026-11-01 00:00"), 3), []string{
		"2026-11-06 00:00", "2026-11-13 00:00", "2026-11-20 00:00",
	})
}

func TestNextRunsAcceptsSundayAsSevenAndNames(t *testing.T) {
	seven, err := cron.Parse("0 6 * * 7")
	if err != nil {
		t.Fatalf("parsing the expression: %v", err)
	}
	named, err := cron.Parse("0 6 * * sun")
	if err != nil {
		t.Fatalf("parsing the named expression: %v", err)
	}
	from := at(t, "2026-09-18 00:00")
	assertRuns(t, seven.NextRuns(from, 2), []string{"2026-09-20 06:00", "2026-09-27 06:00"})
	assertRuns(t, named.NextRuns(from, 2), []string{"2026-09-20 06:00", "2026-09-27 06:00"})
}

func TestNextRunsAcceptsMonthNames(t *testing.T) {
	schedule, err := cron.Parse("0 0 1 JAN,jul *")
	if err != nil {
		t.Fatalf("parsing the expression: %v", err)
	}
	assertRuns(t, schedule.NextRuns(at(t, "2026-09-18 00:00"), 2), []string{
		"2027-01-01 00:00", "2027-07-01 00:00",
	})
}

func TestMacrosName(t *testing.T) {
	cases := []struct {
		expression string
		from       string
		want       []string
	}{
		{"@hourly", "2026-09-18 09:45", []string{"2026-09-18 10:00", "2026-09-18 11:00"}},
		{"@daily", "2026-09-18 09:45", []string{"2026-09-19 00:00", "2026-09-20 00:00"}},
		{"@midnight", "2026-09-18 09:45", []string{"2026-09-19 00:00", "2026-09-20 00:00"}},
		{"@weekly", "2026-09-18 09:45", []string{"2026-09-20 00:00", "2026-09-27 00:00"}},
		{"@monthly", "2026-09-18 09:45", []string{"2026-10-01 00:00", "2026-11-01 00:00"}},
		{"@yearly", "2026-09-18 09:45", []string{"2027-01-01 00:00", "2028-01-01 00:00"}},
		{"@annually", "2026-09-18 09:45", []string{"2027-01-01 00:00", "2028-01-01 00:00"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.expression, func(t *testing.T) {
			schedule, err := cron.Parse(testCase.expression)
			if err != nil {
				t.Fatalf("parsing %s: %v", testCase.expression, err)
			}
			assertRuns(t, schedule.NextRuns(at(t, testCase.from), len(testCase.want)), testCase.want)
		})
	}
}

// February 30 never comes round; the search gives up rather than looping.
func TestNextRunsGivesUpOnADateThatNeverComes(t *testing.T) {
	schedule, err := cron.Parse("0 0 30 2 *")
	if err != nil {
		t.Fatalf("parsing the expression: %v", err)
	}
	if runs := schedule.NextRuns(at(t, "2026-09-18 00:00"), 1); len(runs) != 0 {
		t.Fatalf("want no runs, got %v", runs)
	}
}

func TestNextRunsKeepsTheZoneItIsAskedAbout(t *testing.T) {
	zone := time.FixedZone("test", -3*60*60)
	schedule, err := cron.Parse("0 9 * * *")
	if err != nil {
		t.Fatalf("parsing the expression: %v", err)
	}
	next := schedule.NextRuns(time.Date(2026, 9, 18, 8, 0, 0, 0, zone), 1)
	if len(next) != 1 {
		t.Fatalf("want one run, got %v", next)
	}
	if got := next[0].Format("2006-01-02 15:04 -0700"); got != "2026-09-18 09:00 -0300" {
		t.Fatalf("next run is %s, want it in the zone it was asked about", got)
	}
}

func TestMatchesReportsTheMinutesTheScheduleIsDue(t *testing.T) {
	schedule, err := cron.Parse("*/30 9 * * *")
	if err != nil {
		t.Fatalf("parsing the expression: %v", err)
	}
	for _, due := range []string{"2026-09-18 09:00", "2026-09-18 09:30"} {
		if !schedule.Matches(at(t, due)) {
			t.Errorf("%s should be due", due)
		}
	}
	for _, quiet := range []string{"2026-09-18 09:15", "2026-09-18 10:00"} {
		if schedule.Matches(at(t, quiet)) {
			t.Errorf("%s should not be due", quiet)
		}
	}
}

// A minute is due whatever second of it the scheduler happens to look.
func TestMatchesIgnoresSecondsWithinTheMinute(t *testing.T) {
	schedule, err := cron.Parse("0 * * * *")
	if err != nil {
		t.Fatalf("parsing the expression: %v", err)
	}
	if !schedule.Matches(time.Date(2026, 9, 18, 10, 0, 42, 0, time.UTC)) {
		t.Fatal("the minute is due whichever second the scheduler looks at it")
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]string{
		"too few fields":      "* * * *",
		"too many fields":     "* * * * * *",
		"minute out of range": "60 * * * *",
		"hour out of range":   "* 24 * * *",
		"day out of range":    "* * 32 * *",
		"month out of range":  "* * * 13 *",
		"weekday out of rang": "* * * * 8",
		"backwards range":     "30-10 * * * *",
		"zero step":           "*/0 * * * *",
		"negative step":       "*/-1 * * * *",
		"unknown macro":       "@fortnightly",
		"unknown name":        "* * * * funday",
		"empty":               "   ",
		"empty list entry":    "1,,2 * * * *",
	}
	for name, expression := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := cron.Parse(expression); err == nil {
				t.Fatalf("%q should not parse", expression)
			}
		})
	}
}

func TestParseAcceptsSurroundingSpace(t *testing.T) {
	if _, err := cron.Parse("  0   9  *  *  *  "); err != nil {
		t.Fatalf("extra space should not matter: %v", err)
	}
}

func assertRuns(t *testing.T, got []time.Time, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d runs %v, want %d", len(got), got, len(want))
	}
	for i, moment := range got {
		if formatted := moment.Format("2006-01-02 15:04"); formatted != want[i] {
			t.Errorf("run %d is %s, want %s", i+1, formatted, want[i])
		}
	}
}
