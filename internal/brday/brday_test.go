package brday_test

import (
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

func mustInstant(t *testing.T, text string) time.Time {
	t.Helper()
	instant, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		t.Fatal(err)
	}
	return instant
}

func TestMonthBoundsAreThreeHoursPastUTCMidnight(t *testing.T) {
	cases := []struct {
		month       brday.Month
		start, end  string
		first, end2 brday.Day
	}{
		{"2026-03", "2026-03-01T03:00:00Z", "2026-04-01T03:00:00Z", "2026-03-01", "2026-03-31"},
		{"2026-02", "2026-02-01T03:00:00Z", "2026-03-01T03:00:00Z", "2026-02-01", "2026-02-28"},
		{"2026-12", "2026-12-01T03:00:00Z", "2027-01-01T03:00:00Z", "2026-12-01", "2026-12-31"},
		{"2028-02", "2028-02-01T03:00:00Z", "2028-03-01T03:00:00Z", "2028-02-01", "2028-02-29"},
	}
	for _, c := range cases {
		period, err := brday.MonthPeriod(c.month)
		if err != nil {
			t.Fatal(err)
		}
		if period.First != c.first || period.Last != c.end2 {
			t.Errorf("%s: period %v..%v, want %v..%v", c.month, period.First, period.Last, c.first, c.end2)
		}
		start, end, err := period.Bounds()
		if err != nil {
			t.Fatal(err)
		}
		if !start.Equal(mustInstant(t, c.start)) || !end.Equal(mustInstant(t, c.end)) {
			t.Errorf("%s: bounds %v..%v, want %s..%s", c.month, start, end, c.start, c.end)
		}
	}
}

func TestDayOfCrossesTheMonthLateInUTC(t *testing.T) {
	cases := []struct {
		instant string
		day     brday.Day
		month   brday.Month
	}{
		{"2026-03-01T02:30:00Z", "2026-02-28", "2026-02"},
		{"2026-03-01T03:00:00Z", "2026-03-01", "2026-03"},
		{"2027-01-01T02:59:59.999Z", "2026-12-31", "2026-12"},
		{"2027-01-01T03:00:00Z", "2027-01-01", "2027-01"},
	}
	for _, c := range cases {
		instant := mustInstant(t, c.instant)
		if got := brday.DayOf(instant); got != c.day {
			t.Errorf("DayOf(%s) = %s, want %s", c.instant, got, c.day)
		}
		if got := brday.MonthOf(instant); got != c.month {
			t.Errorf("MonthOf(%s) = %s, want %s", c.instant, got, c.month)
		}
	}
}

func TestPeriodOfOneDayCoversTwentyFourHours(t *testing.T) {
	start, end, err := brday.Period{First: "2026-06-10", Last: "2026-06-10"}.Bounds()
	if err != nil {
		t.Fatal(err)
	}
	if end.Sub(start) != 24*time.Hour {
		t.Fatalf("one day spans %v", end.Sub(start))
	}
}

func TestInvalidInputIsRefused(t *testing.T) {
	if _, err := brday.ParseDay("2026-02-30"); err == nil {
		t.Error("2026-02-30 accepted as a day")
	}
	if _, err := brday.ParseDay("2026-2-3"); err == nil {
		t.Error("2026-2-3 accepted as a day")
	}
	if _, err := brday.ParseMonth("2026-13"); err == nil {
		t.Error("2026-13 accepted as a month")
	}
	if _, _, err := (brday.Period{First: "2026-03-05", Last: "2026-03-04"}).Bounds(); err == nil {
		t.Error("a period that ends before it starts was accepted")
	}
	if _, err := brday.MonthPeriod("March"); err == nil {
		t.Error("a month name was accepted")
	}
}

func TestMonthAfterCrossesTheEndOfAYear(t *testing.T) {
	for month, want := range map[brday.Month]brday.Month{"2026-03": "2026-04", "2026-12": "2027-01", "2027-01": "2027-02", "2028-01": "2028-02"} {
		got, err := brday.MonthAfter(month)
		if err != nil || got != want {
			t.Errorf("MonthAfter(%s) = %s, %v; want %s", month, got, err, want)
		}
	}
	if _, err := brday.MonthAfter("March"); err == nil {
		t.Error("a month name was accepted")
	}
}

func TestTheWeeksOfAMonthAreSevenDaysFromTheFirstAndThenWhatIsLeft(t *testing.T) {
	cases := map[brday.Month][]brday.Period{
		"2026-03": {
			{First: "2026-03-01", Last: "2026-03-07"}, {First: "2026-03-08", Last: "2026-03-14"},
			{First: "2026-03-15", Last: "2026-03-21"}, {First: "2026-03-22", Last: "2026-03-28"},
			{First: "2026-03-29", Last: "2026-03-31"},
		},
		"2026-02": {
			{First: "2026-02-01", Last: "2026-02-07"}, {First: "2026-02-08", Last: "2026-02-14"},
			{First: "2026-02-15", Last: "2026-02-21"}, {First: "2026-02-22", Last: "2026-02-28"},
		},
		"2028-02": {
			{First: "2028-02-01", Last: "2028-02-07"}, {First: "2028-02-08", Last: "2028-02-14"},
			{First: "2028-02-15", Last: "2028-02-21"}, {First: "2028-02-22", Last: "2028-02-28"},
			{First: "2028-02-29", Last: "2028-02-29"},
		},
	}
	for month, want := range cases {
		got, err := brday.Weeks(month)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("Weeks(%s) = %v, %v; want %v", month, got, err, want)
		}
	}
	if _, err := brday.Weeks("March"); err == nil {
		t.Error("a month name was accepted")
	}
}

func TestTheWeekOfADayIsTheWeekOfItsMonthThatHoldsIt(t *testing.T) {
	cases := map[brday.Day]brday.Period{
		"2026-03-21": {First: "2026-03-15", Last: "2026-03-21"},
		"2026-03-15": {First: "2026-03-15", Last: "2026-03-21"},
		"2026-03-14": {First: "2026-03-08", Last: "2026-03-14"},
		"2026-03-31": {First: "2026-03-29", Last: "2026-03-31"},
		"2026-02-28": {First: "2026-02-22", Last: "2026-02-28"},
		"2028-02-29": {First: "2028-02-29", Last: "2028-02-29"},
		"2026-12-01": {First: "2026-12-01", Last: "2026-12-07"},
	}
	for day, want := range cases {
		if got, err := brday.WeekOf(day); err != nil || got != want {
			t.Errorf("WeekOf(%s) = %v, %v; want %v", day, got, err, want)
		}
	}
	for _, day := range []brday.Day{"2026-02-30", "2026-03", ""} {
		if _, err := brday.WeekOf(day); err == nil {
			t.Errorf("WeekOf(%q) was answered", day)
		}
	}
}
