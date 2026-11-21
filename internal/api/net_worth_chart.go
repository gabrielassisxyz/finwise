package api

import (
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// chart has a point for each month the ledger has a reading in, from the first
// such month on. A month before the first reading is not in the chart at all,
// and a month with no reading is a gap: a zero would say the money was gone.
func (view *potsView) chart() (netWorthChartWire, brday.Month) {
	chart := netWorthChartWire{Months: []string{}, Points: []netWorthPointWire{}}
	first := view.firstReadingMonth()
	if first == "" {
		return chart, view.month
	}
	for month := first; month <= view.month; month = nextMonth(month) {
		chart.Months = append(chart.Months, string(month))
		if point, recorded := view.pointOf(month); recorded {
			chart.Points = append(chart.Points, point)
		}
	}
	chart.Max = Reais(chartMaxOf(chart.Points))
	return chart, first
}

func (view *potsView) firstReadingMonth() brday.Month {
	var first brday.Day
	for _, days := range view.sightings {
		if len(days) > 0 && (first == "" || days[0] < first) {
			first = days[0]
		}
	}
	return monthOfDay(first)
}

func nextMonth(month brday.Month) brday.Month {
	parsed, err := time.Parse("2006-01", string(month))
	if err != nil {
		return "9999-12"
	}
	return brday.Month(parsed.AddDate(0, 1, 0).Format("2006-01"))
}

// pointOf is the net worth at the end of a month, each account at its latest
// sighting up to then, and recorded only when some account was sighted in the
// month. A reading that cannot be read leaves the month out.
func (view *potsView) pointOf(month brday.Month) (netWorthPointWire, bool) {
	end := lastDayOf(month)
	var parts netWorthParts
	recorded := false
	for accountID, days := range view.balances {
		latest, found := latestUpTo(days, end)
		if !found || latest.Balance == nil {
			if found {
				return netWorthPointWire{}, false
			}
			continue
		}
		recorded = recorded || monthOfDay(latest.Day) == month
		parts.available += *latest.Balance
		reserved, readable := view.reservedOn(accountID, latest.Day)
		if !readable {
			return netWorthPointWire{}, false
		}
		parts.reserved += reserved
	}
	invested, readable := view.investedUpTo(end)
	if !recorded || !readable {
		return netWorthPointWire{}, false
	}
	parts.invested = invested
	return netWorthPointWire{
		Month: string(month), Available: Reais(parts.available), Reserved: Reais(parts.reserved),
		Invested: Reais(parts.invested), Total: Reais(parts.total()),
	}, true
}

func latestUpTo(days []ledger.AccountDay, end brday.Day) (ledger.AccountDay, bool) {
	var latest ledger.AccountDay
	found := false
	for _, day := range days {
		if day.Day <= end {
			latest, found = day, true
		}
	}
	return latest, found
}

// reservedOn adds the reserve entries of one sighting; false when one of them
// has no readable amount.
func (view *potsView) reservedOn(accountID string, day brday.Day) (money.Cents, bool) {
	var total money.Cents
	for _, row := range view.reserveHistory {
		if row.AccountID != accountID || row.Day != day {
			continue
		}
		if row.Amount == nil || row.AmountStatus != "ok" {
			return 0, false
		}
		total += *row.Amount
	}
	return total, true
}

// investedUpTo adds each investment at its latest day up to the end; an
// investment not yet in the ledger counts for nothing.
func (view *potsView) investedUpTo(end brday.Day) (money.Cents, bool) {
	var total money.Cents
	for _, days := range view.investmentDays {
		var net *money.Cents
		for _, day := range days {
			if day.Day <= end {
				net = day.AmountNet
				if net == nil {
					return 0, false
				}
			}
		}
		if net != nil {
			total += *net
		}
	}
	return total, true
}

// chartMaxOf is the smallest 1, 2 or 5 followed by zeros, in reais, that holds
// the highest total of the points; zero when nothing is above zero.
func chartMaxOf(points []netWorthPointWire) money.Cents {
	var top money.Cents
	for _, point := range points {
		top = max(top, money.Cents(point.Total))
	}
	if top <= 0 {
		return 0
	}
	for unit := money.Cents(100); ; unit *= 10 {
		for _, step := range []money.Cents{1, 2, 5} {
			if step*unit >= top {
				return step * unit
			}
		}
	}
}
