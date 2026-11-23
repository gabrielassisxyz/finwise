package subrules

import (
	"fmt"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// A brday.Day is a calendar date, so the arithmetic here runs in UTC, where no
// daylight-saving transition can move a date.

const dayLayout = "2006-01-02"

const oneDay = 24 * time.Hour

func parseDay(day brday.Day) (time.Time, error) {
	parsed, err := time.Parse(dayLayout, string(day))
	if err != nil {
		return time.Time{}, fmt.Errorf("subrules: %q is not a YYYY-MM-DD day", day)
	}
	return parsed, nil
}

// AddMonths moves a day by whole months, keeping its day number and holding
// it to the last day of a month that is shorter: 2027-01-31 plus one month is
// 2027-02-28, not 2027-03-03.
func AddMonths(day brday.Day, months int) (brday.Day, error) {
	parsed, err := parseDay(day)
	if err != nil {
		return "", err
	}
	first := time.Date(parsed.Year(), parsed.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	moved := time.Date(first.Year(), first.Month(), min(parsed.Day(), last), 0, 0, 0, 0, time.UTC)
	return brday.Day(moved.Format(dayLayout)), nil
}

// DaysBetween is the whole days from one day to another, negative when the
// second is earlier.
func DaysBetween(from, to brday.Day) (int, error) {
	first, err := parseDay(from)
	if err != nil {
		return 0, err
	}
	last, err := parseDay(to)
	if err != nil {
		return 0, err
	}
	return int(last.Sub(first) / oneDay), nil
}

// FirstReview is the day a subscription made today is first up for review.
func FirstReview(today brday.Day) (brday.Day, error) {
	return AddMonths(today, ReviewAfterMonths)
}

// compareDays orders two days of the YYYY-MM-DD form, which sort as they read.
func compareDays(a, b brday.Day) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
