package budgetrules

import (
	"fmt"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// MonthLimit is the variable group's cap in a month and what was spent on its
// lines.
type MonthLimit struct {
	Cap   money.Cents
	Spent money.Cents
}

// VariableLimit is the variable group as View weighs it, reduced to its
// subtotal and what was spent against it, so the home's limit and the budget
// screen's variable group are one figure.
func VariableLimit(plan Plan, figures map[LineID]money.Cents) MonthLimit {
	group := MonthFacts{Figures: figures}.editableGroup(GroupVariable, plan.linesOf(GroupVariable))
	return MonthLimit{Cap: group.Subtotal, Spent: *group.SubtotalSpent}
}

// Remaining is the cap less what was spent, negative once the cap is passed.
func (limit MonthLimit) Remaining() money.Cents { return limit.Cap - limit.Spent }

func (limit MonthLimit) Over() bool { return limit.Remaining() < 0 }

// Fill is how much of the cap is used, a whole percent from 0 to 100.
func (limit MonthLimit) Fill() int64 { return Share(limit.Spent, limit.Cap) }

// DatedOutcome is a transaction's outcome with the Brazil day it is listed on.
type DatedOutcome struct {
	Day     brday.Day
	Outcome Outcome
}

// VariableSpendingByDay is what the outcomes add to the plan's variable lines,
// day by day, weighed as VariableLimit weighs the whole month: a day's figure
// can be negative when a refund outweighs that day's purchases.
func VariableSpendingByDay(plan Plan, dated []DatedOutcome) map[brday.Day]money.Cents {
	byDay := map[brday.Day][]Outcome{}
	for _, one := range dated {
		byDay[one.Day] = append(byDay[one.Day], one.Outcome)
	}
	spending := make(map[brday.Day]money.Cents, len(byDay))
	for day, outcomes := range byDay {
		spending[day] = VariableLimit(plan, LineFigures(outcomes)).Spent
	}
	return spending
}

// WeekGuide is one week of a month and how much of the variable cap it can
// spend. Spent is nil for a week that has not begun.
type WeekGuide struct {
	Label   string
	Guide   money.Cents
	Spent   *money.Cents
	Current bool
}

// Fill is how much of its guide the week has spent, 0 to 100.
func (week WeekGuide) Fill() int64 {
	var spent money.Cents
	if week.Spent != nil {
		spent = *week.Spent
	}
	return Share(spent, week.Guide)
}

// WeekGuides shares what is left of the month's variable cap over the days
// still to come, one guide for each week of brday.Weeks: the cap less what was
// spent before the week, never below zero, times the week's days over the
// days from its first to the month's end, rounded half up to the cent. Equal
// shares per week would over-fund the short block at the month's end. A month
// with no cap has no week to guide. today is the Brazil day of the request; a
// week has begun once its first day is past in the current month, or in any
// earlier month.
func WeekGuides(month brday.Month, today brday.Day, variableCap money.Cents, spending map[brday.Day]money.Cents) ([]WeekGuide, error) {
	weeks, err := brday.Weeks(month)
	if err != nil {
		return nil, err
	}
	guides := []WeekGuide{}
	if variableCap <= 0 {
		return guides, nil
	}
	daily := spendingOfMonth(month, spending)
	monthEnd := dayNumber(weeks[len(weeks)-1].Last)
	current := brday.Month(today[:len("2006-01")])
	for _, week := range weeks {
		first, last := dayNumber(week.First), dayNumber(week.Last)
		base := max(0, variableCap-daily.sum(1, first-1))
		guide := WeekGuide{
			Label:   weekLabel(first, last),
			Guide:   roundedShareOfDays(base, last-first+1, monthEnd-first+1),
			Current: month == current && week.First <= today && today <= week.Last,
		}
		if month < current || (month == current && week.First <= today) {
			spent := daily.sum(first, last)
			guide.Spent = &spent
		}
		guides = append(guides, guide)
	}
	return guides, nil
}

// roundedShareOfDays is base × days / remaining rounded half up, for
// non-negative terms and remaining above zero.
func roundedShareOfDays(base money.Cents, days, remaining int) money.Cents {
	return money.Cents(floorDivision(2*int64(base)*int64(days)+int64(remaining), 2*int64(remaining)))
}

// daysOfMonth holds a month's figures by day number, index 0 unused.
type daysOfMonth [32]money.Cents

func spendingOfMonth(month brday.Month, spending map[brday.Day]money.Cents) daysOfMonth {
	var daily daysOfMonth
	for day, cents := range spending {
		if brday.Month(day[:len("2006-01")]) == month {
			daily[dayNumber(day)] += cents
		}
	}
	return daily
}

// sum is the figures of the days from first to last, both included.
func (daily daysOfMonth) sum(first, last int) money.Cents {
	var total money.Cents
	for day := first; day <= last; day++ {
		total += daily[day]
	}
	return total
}

// dayNumber is the day of the month of a YYYY-MM-DD day, which brday has
// already validated.
func dayNumber(day brday.Day) int {
	return int(day[8]-'0')*10 + int(day[9]-'0')
}

// weekLabel is the design's label: the first and the last day as two digits
// joined by an en dash, or the one day of a week of one day.
func weekLabel(first, last int) string {
	if first == last {
		return fmt.Sprintf("%02d", first)
	}
	return fmt.Sprintf("%02d–%02d", first, last)
}
