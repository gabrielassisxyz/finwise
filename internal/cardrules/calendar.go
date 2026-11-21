package cardrules

import (
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// A brday.Day is a calendar date, so the arithmetic here runs in UTC where no
// daylight-saving transition can move a date.

func parseDay(day brday.Day) (time.Time, bool) {
	parsed, err := time.Parse("2006-01-02", string(day))
	return parsed, err == nil
}

func formatDay(moment time.Time) brday.Day {
	return brday.Day(moment.Format("2006-01-02"))
}

func addDays(day brday.Day, days int) (brday.Day, bool) {
	parsed, ok := parseDay(day)
	if !ok {
		return "", false
	}
	return formatDay(parsed.AddDate(0, 0, days)), true
}

func daysBetween(earlier, later brday.Day) (int, bool) {
	first, firstOK := parseDay(earlier)
	last, lastOK := parseDay(later)
	if !firstOK || !lastOK {
		return 0, false
	}
	return int(last.Sub(first).Hours() / 24), true
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// dayOfMonth is the date with that day number in a month, held to the month's
// last day when the month is shorter.
func dayOfMonth(year int, month time.Month, number int) time.Time {
	return time.Date(year, month, min(number, daysInMonth(year, month)), 0, 0, 0, 0, time.UTC)
}

// dueInMonthsAfter keeps the day number of the first due date, so that a due on
// the 31st does not drift to the 28th after passing through February.
func dueInMonthsAfter(due brday.Day, months int) (brday.Day, bool) {
	parsed, ok := parseDay(due)
	if !ok {
		return "", false
	}
	target := time.Date(parsed.Year(), parsed.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	return formatDay(dayOfMonth(target.Year(), target.Month(), parsed.Day())), true
}
