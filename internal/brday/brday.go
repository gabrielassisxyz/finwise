// Package brday names a day and a month in Brazil time. The ledger stores UTC
// instants; a row at 02:30 UTC on the 1st is still the last day of the previous
// month in America/Sao_Paulo, so period bounds are computed here and nowhere else.
package brday

import (
	"fmt"
	"time"
	// The zone rules travel inside the binary so the answer does not depend on
	// the host having a tz database.
	_ "time/tzdata"
)

const (
	dayLayout   = "2006-01-02"
	monthLayout = "2006-01"
	zoneName    = "America/Sao_Paulo"
)

// Day is a calendar day as YYYY-MM-DD in Brazil time.
type Day string

// Month is a calendar month as YYYY-MM.
type Month string

// Period is a run of whole days, both ends included.
type Period struct {
	First Day
	Last  Day
}

var zone = mustLoadZone()

func mustLoadZone() *time.Location {
	location, err := time.LoadLocation(zoneName)
	if err != nil {
		panic(fmt.Sprintf("brday: %s: %v", zoneName, err))
	}
	return location
}

// ParseDay validates a YYYY-MM-DD string.
func ParseDay(text string) (Day, error) {
	if _, err := time.ParseInLocation(dayLayout, text, zone); err != nil {
		return "", fmt.Errorf("brday: %q is not a YYYY-MM-DD day", text)
	}
	return Day(text), nil
}

// DayOf is the Brazil day an instant falls on.
func DayOf(instant time.Time) Day {
	return Day(instant.In(zone).Format(dayLayout))
}

// ParseMonth validates a YYYY-MM string.
func ParseMonth(text string) (Month, error) {
	if _, err := time.ParseInLocation(monthLayout, text, zone); err != nil {
		return "", fmt.Errorf("brday: %q is not a YYYY-MM month", text)
	}
	return Month(text), nil
}

// MonthOf is the Brazil month an instant falls in.
func MonthOf(instant time.Time) Month {
	return Month(instant.In(zone).Format(monthLayout))
}

// MonthAfter is the month that follows one, across the end of a year.
func MonthAfter(month Month) (Month, error) {
	first, err := time.ParseInLocation(monthLayout, string(month), zone)
	if err != nil {
		return "", fmt.Errorf("brday: %q is not a YYYY-MM month", month)
	}
	return Month(first.AddDate(0, 1, 0).Format(monthLayout)), nil
}

// MonthPeriod is every day of a month.
func MonthPeriod(month Month) (Period, error) {
	first, err := time.ParseInLocation(monthLayout, string(month), zone)
	if err != nil {
		return Period{}, fmt.Errorf("brday: %q is not a YYYY-MM month", month)
	}
	last := first.AddDate(0, 1, -1)
	return Period{First: Day(first.Format(dayLayout)), Last: Day(last.Format(dayLayout))}, nil
}

// weekLength is how many days a week of a month holds, every week but the days
// left after the fourth.
const weekLength = 7

// Weeks are the weeks of a month as the review and the home screen count them:
// seven days from the 1st, four times, and then the days left, if any. They are
// dates alone, the same whether or not the month has a budget.
func Weeks(month Month) ([]Period, error) {
	whole, err := MonthPeriod(month)
	if err != nil {
		return nil, err
	}
	first, _ := time.ParseInLocation(dayLayout, string(whole.First), zone)
	last, _ := time.ParseInLocation(dayLayout, string(whole.Last), zone)
	var weeks []Period
	for start := first; !start.After(last); start = start.AddDate(0, 0, weekLength) {
		end := start.AddDate(0, 0, weekLength-1)
		if end.After(last) {
			end = last
		}
		weeks = append(weeks, Period{First: Day(start.Format(dayLayout)), Last: Day(end.Format(dayLayout))})
	}
	return weeks, nil
}

// WeekOf is the week of its month that holds day.
func WeekOf(day Day) (Period, error) {
	if _, err := ParseDay(string(day)); err != nil {
		return Period{}, err
	}
	weeks, err := Weeks(Month(day[:len(monthLayout)]))
	if err != nil {
		return Period{}, err
	}
	for _, week := range weeks {
		if day >= week.First && day <= week.Last {
			return week, nil
		}
	}
	return Period{}, fmt.Errorf("brday: no week of its month holds %q", day)
}

// Bounds returns the UTC instants a period covers: start is the first
// instant of First, end is the first instant after Last, so a row belongs
// to the period when start <= instant < end.
func (period Period) Bounds() (start, end time.Time, err error) {
	first, err := time.ParseInLocation(dayLayout, string(period.First), zone)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("brday: first day %q is not YYYY-MM-DD", period.First)
	}
	last, err := time.ParseInLocation(dayLayout, string(period.Last), zone)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("brday: last day %q is not YYYY-MM-DD", period.Last)
	}
	if last.Before(first) {
		return time.Time{}, time.Time{}, fmt.Errorf("brday: period ends (%s) before it starts (%s)", period.Last, period.First)
	}
	return first.UTC(), last.AddDate(0, 0, 1).UTC(), nil
}
