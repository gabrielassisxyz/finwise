package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// monthArgument reads one argument that is a calendar month, `YYYY-MM`.
func monthArgument(raw json.RawMessage) (brday.Month, error) {
	var text *string
	if err := decodeArgument(raw, &text); err != nil || text == nil {
		return "", &badArgumentsError{}
	}
	month, err := brday.ParseMonth(*text)
	if err != nil {
		return "", &badArgumentsError{}
	}
	return month, nil
}

// onlyMonthArgument is for a method whose one argument is the month.
func onlyMonthArgument(args []json.RawMessage) (brday.Month, error) {
	if len(args) != 1 {
		return "", &badArgumentsError{}
	}
	return monthArgument(args[0])
}

// requireReadableLedger keeps a method that answers a fixed value from
// answering it over a ledger that is gone: an empty budget would read as the
// person's own, when it is the snapshot that cannot be opened.
func requireReadableLedger(ctx context.Context, reader *ledger.Reader) error {
	_, err := reader.Accounts(ctx)
	return err
}
