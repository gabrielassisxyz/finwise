package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
)

const markTransferDoneMethod = "review.markTransferDone"

func registerPaydayWrites(registry *Registry) {
	registry.RegisterWrite(markTransferDoneMethod, markTransferDone)
}

// markTransferDone marks one of a month's transfers made, dated today. The
// transfer is found among those the month derives now, as the payday shows
// them, by its key and never by its name; a key the month does not list and a
// transfer already marked answer null and leave nothing in the journal. A
// closed month or another month than the current one is marked like any other:
// closing freezes a budget, not its transfers. A transfer is not a
// transaction, so the change has no history step.
func markTransferDone(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	month, err := monthArgument(args[0])
	if err != nil {
		return nil, err
	}
	key, err := textArgument(args[1])
	if err != nil {
		return nil, err
	}
	// The month's transfers read the ledger only for its pots, so a missing
	// snapshot is asked about first, as every read of the payday asks.
	if err := requireReadableLedger(ctx, writes.Ledger); err != nil {
		return nil, err
	}
	planned, err := transfersOfMonth(ctx, Views{Ledger: writes.Ledger, Authored: writes.Authored}, month)
	if err != nil {
		return nil, err
	}
	if transfer, listed := budgetrules.TransferByKey(planned.transfers, key); !listed || transfer.Done() {
		return nil, nil
	}
	recorded, err := writes.Recorder.MarkTransferDone(ctx, month, key, brday.DayOf(nowOf(ctx)))
	if err != nil || !recorded {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}
