// Package subrules works out a subscription from the card's own charges: how
// often it is charged, what that costs per month and per year, when the next
// charge is due, what the last charges were and what is worth a warning. It
// takes plain values and returns plain values, in integer cents; it opens
// nothing, reads no clock, and knows neither HTTP nor SQL.
package subrules

import (
	"slices"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Row is one card purchase of the subscription's establishment, as the
// caller found it eligible.
type Row struct {
	Day brday.Day
	// Cents is the purchase's magnitude, with the tax folded into it.
	Cents money.Cents
	// Foreign is the amount the card stated the purchase in, nil for one in reais.
	Foreign *Foreign
	// Tax is the magnitude of the tax folded into the purchase.
	Tax money.Cents
}

// Foreign is an amount in another currency, kept for display.
type Foreign struct {
	Amount   money.Cents
	Currency string
}

// Charge is everything the card charged on one day.
type Charge struct {
	Day   brday.Day
	Cents money.Cents
	// Foreign and Tax are set only for a charge made of exactly one row that
	// carries both a foreign amount and a folded tax: the figures of several
	// rows cannot be told apart once they are added.
	Foreign *Foreign
	Tax     money.Cents
	// stored is the creation's purchase standing in for charges the ledger no
	// longer lists.
	stored bool
}

// Subscription is what one subscription is worked out from.
type Subscription struct {
	Rows []Row
	// CreatedDay and CreatedCents are the purchase it was made from, as it was
	// then; they stand in for a charge when no row is left.
	CreatedDay   brday.Day
	CreatedCents money.Cents
	// Cancelling is set while the person has marked it to cancel, since
	// StatusDay.
	Cancelling bool
	StatusDay  brday.Day
	Today      brday.Day
}

// Derivation is what the rules answer for one subscription.
type Derivation struct {
	Cycle       Cycle
	Amount      money.Cents
	Previous    money.Cents
	HasPrevious bool
	NextCharge  brday.Day
	Monthly     money.Cents
	Yearly      money.Cents
	Marks       Marks
	// History is the latest charges, newest first.
	History []Charge
}

// Marks are the warnings that hold. The sentences are the caller's.
type Marks struct {
	PriceRose bool
	// Renews is set for a cycle of a year or more whose next charge is
	// RenewsIn whole days away, from 0 to RenewalNoticeDays.
	Renews   bool
	RenewsIn int
	// ChargedAfterCancel is a charge after the day it was marked to cancel.
	ChargedAfterCancel bool
}

const (
	// RenewalNoticeDays is how far ahead a renewal of a yearly cycle or
	// longer is worth a warning.
	RenewalNoticeDays = 30
	// ChargesShown is how many charges the history keeps, the page's design.
	ChargesShown = 3
	// ReviewAfterMonths is when a new subscription is first up for review.
	ReviewAfterMonths = 3
)

// Derive works a subscription out from its rows, in whatever order they come.
func Derive(subscription Subscription) (Derivation, error) {
	charges := ChargesOf(subscription.Rows)
	if len(charges) == 0 {
		charges = []Charge{{Day: subscription.CreatedDay, Cents: subscription.CreatedCents, stored: true}}
	}
	cycle, err := cycleOf(charges)
	if err != nil {
		return Derivation{}, err
	}
	latest := charges[len(charges)-1]
	next, err := AddMonths(latest.Day, cycle.Months())
	if err != nil {
		return Derivation{}, err
	}
	derivation := Derivation{
		Cycle: cycle, Amount: latest.Cents, NextCharge: next,
		Monthly: MonthlyEquivalent(latest.Cents, cycle), Yearly: YearlyEquivalent(latest.Cents, cycle),
		History: historyOf(charges),
	}
	if len(charges) >= 2 {
		derivation.Previous, derivation.HasPrevious = charges[len(charges)-2].Cents, true
	}
	derivation.Marks, err = marksOf(subscription, derivation, latest)
	return derivation, err
}

// ChargesOf adds the rows of each day into one charge, oldest first.
func ChargesOf(rows []Row) []Charge {
	byDay := map[brday.Day][]Row{}
	for _, row := range rows {
		byDay[row.Day] = append(byDay[row.Day], row)
	}
	charges := make([]Charge, 0, len(byDay))
	for day, ofDay := range byDay {
		charge := Charge{Day: day}
		for _, row := range ofDay {
			charge.Cents += row.Cents
		}
		if len(ofDay) == 1 && ofDay[0].Foreign != nil && ofDay[0].Tax > 0 {
			foreign := *ofDay[0].Foreign
			charge.Foreign, charge.Tax = &foreign, ofDay[0].Tax
		}
		charges = append(charges, charge)
	}
	slices.SortFunc(charges, func(a, b Charge) int { return compareDays(a.Day, b.Day) })
	return charges
}

func historyOf(charges []Charge) []Charge {
	history := make([]Charge, 0, ChargesShown)
	for index := len(charges) - 1; index >= 0 && len(history) < ChargesShown; index-- {
		history = append(history, charges[index])
	}
	return history
}

func marksOf(subscription Subscription, derivation Derivation, latest Charge) (Marks, error) {
	marks := Marks{PriceRose: derivation.HasPrevious && derivation.Previous < derivation.Amount}
	if derivation.Cycle.Months() >= 12 {
		days, err := DaysBetween(subscription.Today, derivation.NextCharge)
		if err != nil {
			return Marks{}, err
		}
		if days >= 0 && days <= RenewalNoticeDays {
			marks.Renews, marks.RenewsIn = true, days
		}
	}
	if subscription.Cancelling && !latest.stored {
		after, err := DaysBetween(subscription.StatusDay, latest.Day)
		if err != nil {
			return Marks{}, err
		}
		marks.ChargedAfterCancel = after > 0
	}
	return marks, nil
}

// MonthlyEquivalent is the amount spread over the months of its cycle,
// rounded half up to the cent.
func MonthlyEquivalent(amount money.Cents, cycle Cycle) money.Cents {
	return dividedHalfUp(amount, cycle.Months())
}

// YearlyEquivalent is twelve times the amount spread over the months of its
// cycle, rounded half up to the cent.
func YearlyEquivalent(amount money.Cents, cycle Cycle) money.Cents {
	return dividedHalfUp(amount*12, cycle.Months())
}

// Totals adds monthly equivalents as they are shown, already rounded, and
// answers twelve times that sum for the year, so the two figures of a header
// agree with each other and with the rows under it.
func Totals(monthly []money.Cents) (month, year money.Cents) {
	for _, cents := range monthly {
		month += cents
	}
	return month, month * 12
}

// dividedHalfUp divides a magnitude; a half rounds up. Amounts here are never
// negative, so there is no negative half to decide.
func dividedHalfUp(amount money.Cents, by int) money.Cents {
	divisor := money.Cents(by)
	return (2*amount + divisor) / (2 * divisor)
}
