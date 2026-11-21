package potrules

import (
	"sort"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Base says how a yield figure was reported. Nubank reports net of tax and Mercado
// Pago gross, and whether to convert one to the other is not decided, so every
// figure carries its base and figures of different bases are never added.
type Base string

const (
	BaseNet   Base = "net"
	BaseGross Base = "gross"
	// BaseUnknown is for a pot whose bank the caller could not say; it is a base of
	// its own and is not merged into either of the others.
	BaseUnknown Base = "unknown"
)

// DayYield is the ledger's answer for one pot between two consecutive sightings of
// its account, usually a single day. Yield is nil exactly when the ledger could
// not compute it, and a zero Yield is a computed zero: a deposit raises the
// balance and yields nothing.
type DayYield struct {
	AccountID string
	ReserveID string
	From, To  brday.Day
	Yield     *money.Cents
}

// MonthYield is a pot's yield for one month, with the rows behind it.
type MonthYield struct {
	AccountID string
	ReserveID string
	Month     brday.Month
	Base      Base
	// Known is false when no row reaches the month, or when any row of the month
	// could not be computed. Total is then not a figure to show, least of all as zero.
	Known bool
	Total money.Cents
	// Days are the rows behind the total, oldest first. A row belongs to the month
	// of its To day: that is the sighting where the yield was first seen.
	Days []DayYield
	// Since is the day the first row starts from. It precedes the month when the
	// account was not sighted on the days before, and the total then holds yield
	// earned before the month began.
	Since brday.Day
}

// YieldOfMonth sums the rows of one pot whose To day falls in the month. A pot
// with no stable identity has no rows to sum and gets an unknown yield.
func YieldOfMonth(pot Pot, base Base, month brday.Month, rows []DayYield) MonthYield {
	result := MonthYield{AccountID: pot.AccountID, ReserveID: pot.ReserveID, Month: month, Base: base}
	if !pot.Stable {
		return result
	}
	for _, row := range rows {
		if row.AccountID == pot.AccountID && row.ReserveID == pot.ReserveID &&
			strings.HasPrefix(string(row.To), string(month)+"-") {
			result.Days = append(result.Days, row)
		}
	}
	sort.SliceStable(result.Days, func(i, j int) bool { return result.Days[i].To < result.Days[j].To })
	if len(result.Days) > 0 {
		result.Since = result.Days[0].From
	}
	result.Known = len(result.Days) > 0
	for _, row := range result.Days {
		if row.Yield == nil {
			result.Known = false
			result.Total = 0
			return result
		}
		result.Total += *row.Yield
	}
	return result
}

// BaseTotal is what the pots of one base yielded. Total adds only the pots whose
// yield is known; Unknown counts the others, and a total with Unknown above zero
// is a floor, not the figure.
type BaseTotal struct {
	Base    Base
	Total   money.Cents
	Pots    int
	Unknown int
}

// TotalYieldsByBase returns one total per base, ordered by base. It has no
// single total across bases to return.
func TotalYieldsByBase(yields []MonthYield) []BaseTotal {
	byBase := map[Base]*BaseTotal{}
	for _, yield := range yields {
		total, seen := byBase[yield.Base]
		if !seen {
			total = &BaseTotal{Base: yield.Base}
			byBase[yield.Base] = total
		}
		total.Pots++
		if yield.Known {
			total.Total += yield.Total
		} else {
			total.Unknown++
		}
	}
	totals := make([]BaseTotal, 0, len(byBase))
	for _, total := range byBase {
		totals = append(totals, *total)
	}
	sort.Slice(totals, func(i, j int) bool { return totals[i].Base < totals[j].Base })
	return totals
}
