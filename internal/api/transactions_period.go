package api

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// taxPairingDays is how far a tax row can fall from its purchase, which is the
// window the rules pair them in. A period is read this much wider on each side,
// so a purchase on the last day of a month and its tax on the first of the next
// are interpreted together, and only then is the result cut back.
const taxPairingDays = 3

const (
	periodMonth       = "month"
	periodLastQuarter = "last-3-months"
	periodYear        = "year"
)

type transactionQuery struct {
	Month  string `json:"month"`
	Period string `json:"period"`
}

// periodOf is the run of Brazil days a query covers. A malformed month or an
// unknown period is a bad request, not an empty answer.
func periodOf(query transactionQuery) (brday.Period, error) {
	month, err := brday.ParseMonth(query.Month)
	if err != nil {
		return brday.Period{}, &badArgumentsError{}
	}
	thisMonth, err := brday.MonthPeriod(month)
	if err != nil {
		return brday.Period{}, &badArgumentsError{}
	}
	start, _ := time.Parse("2006-01", string(month))
	switch query.Period {
	case periodMonth:
		return thisMonth, nil
	case periodLastQuarter:
		return brday.Period{First: dayOf(start.AddDate(0, -2, 0)), Last: thisMonth.Last}, nil
	case periodYear:
		return brday.Period{
			First: dayOf(time.Date(start.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)),
			Last:  dayOf(time.Date(start.Year(), time.December, 31, 0, 0, 0, 0, time.UTC)),
		}, nil
	}
	return brday.Period{}, &badArgumentsError{}
}

func dayOf(instant time.Time) brday.Day { return brday.Day(instant.Format("2006-01-02")) }

// widened extends a period by days on each side.
func widened(period brday.Period, days int) (brday.Period, error) {
	first, err := time.Parse("2006-01-02", string(period.First))
	if err != nil {
		return brday.Period{}, err
	}
	last, err := time.Parse("2006-01-02", string(period.Last))
	if err != nil {
		return brday.Period{}, err
	}
	return brday.Period{First: dayOf(first.AddDate(0, 0, -days)), Last: dayOf(last.AddDate(0, 0, days))}, nil
}

// ledgerContext is what interpreting rows needs from the rest of the ledger.
type ledgerContext struct {
	accounts   []ledger.Account
	reference  txrules.Reference
	banks      map[string]string
	labels     map[string]string
	billLabels map[string]string
}

func loadContext(ctx context.Context, reader *ledger.Reader) (ledgerContext, error) {
	accounts, err := reader.Accounts(ctx)
	if err != nil {
		return ledgerContext{}, err
	}
	identities, err := reader.Identities(ctx)
	if err != nil {
		return ledgerContext{}, err
	}
	bills, err := reader.Bills(ctx)
	if err != nil {
		return ledgerContext{}, err
	}
	banks := bankLabels(accounts)
	return ledgerContext{
		accounts:   accounts,
		reference:  txrules.Reference{Accounts: accounts, Identities: identities},
		banks:      banks,
		labels:     accountLabelsOf(accounts, banks),
		billLabels: billLabelsOf(bills),
	}, nil
}

// interpreted is a period's transactions as the list shows them.
type interpreted struct {
	listed []txrules.Transaction
	// tally is the widened read's: it is the one that must place every row.
	tally txrules.Tally
	rows  map[string]ledger.Transaction
}

// interpretPeriod reads the period widened by the pairing window, interprets it
// and keeps the transactions whose day is inside the period, newest first.
func interpretPeriod(ctx context.Context, reader *ledger.Reader, around ledgerContext, period brday.Period) (interpreted, error) {
	wide, err := widened(period, taxPairingDays)
	if err != nil {
		return interpreted{}, err
	}
	rows, err := reader.CleanTransactions(ctx, wide)
	if err != nil {
		return interpreted{}, err
	}
	return interpretRows(rows, around, period)
}

func interpretRows(rows []ledger.Transaction, around ledgerContext, period brday.Period) (interpreted, error) {
	result := txrules.Interpret(rows, around.reference)
	if err := checkPlaced(result.Tally, len(rows)); err != nil {
		return interpreted{}, err
	}
	byID := make(map[string]ledger.Transaction, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	return interpreted{listed: newestFirst(within(result.Listed, period)), tally: result.Tally, rows: byID}, nil
}

// checkPlaced fails a read in which a row went nowhere: showing a month that is
// silently short of a transaction is worse than showing an error.
func checkPlaced(tally txrules.Tally, input int) error {
	if tally.Input != input || !tally.Balanced() {
		return errors.New("api: the rules did not place every row")
	}
	return nil
}

func within(listed []txrules.Transaction, period brday.Period) []txrules.Transaction {
	kept := make([]txrules.Transaction, 0, len(listed))
	for _, transaction := range listed {
		if transaction.Day >= period.First && transaction.Day <= period.Last {
			kept = append(kept, transaction)
		}
	}
	return kept
}

// newestFirst keeps the ledger's order within a day, which is the later instant
// first and then the id.
func newestFirst(listed []txrules.Transaction) []txrules.Transaction {
	sort.SliceStable(listed, func(a, b int) bool { return listed[a].Day > listed[b].Day })
	return listed
}

// rowsAround selects the rows of a whole-ledger read that fall in the days of a period.
func rowsAround(rows []ledger.Transaction, period brday.Period) []ledger.Transaction {
	var selected []ledger.Transaction
	for _, row := range rows {
		if row.Day >= period.First && row.Day <= period.Last {
			selected = append(selected, row)
		}
	}
	return selected
}
