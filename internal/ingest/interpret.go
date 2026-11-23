package ingest

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// wholeLedger is the period that holds every row the clean list can have.
var wholeLedger = brday.Period{First: "1970-01-01", Last: "2999-12-31"}

// listedTransactions interprets the whole ledger in one pass, as the API does
// for anything that spans months, with the accounts and identities of the same
// view as the reference. A tally that does not place every row fails the run as
// it fails a read: taking in a list silently short of a row is worse than
// taking in nothing.
func listedTransactions(ctx context.Context, view *ledger.View) ([]txrules.Transaction, error) {
	accounts, err := view.Accounts(ctx)
	if err != nil {
		return nil, ledgerFailure(err)
	}
	identities, err := view.Identities(ctx)
	if err != nil {
		return nil, ledgerFailure(err)
	}
	rows, err := view.CleanTransactions(ctx, wholeLedger)
	if err != nil {
		return nil, ledgerFailure(err)
	}
	result := txrules.Interpret(rows, txrules.Reference{Accounts: accounts, Identities: identities})
	if result.Tally.Input != len(rows) || !result.Tally.Balanced() {
		return nil, &Error{Failure: FailureUnplaced}
	}
	return result.Listed, nil
}
