package ingest

import (
	"context"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// RunWithSeam is Run calling afterRead once the ledger is read and before the
// lock is taken.
func RunWithSeam(ctx context.Context, sources Sources, trigger Trigger, now time.Time, afterRead func()) (Counts, error) {
	return run(ctx, sources, trigger, now, afterRead)
}

// ListedTransactions is the interpretation of the whole ledger a run takes in.
func ListedTransactions(ctx context.Context, view *ledger.View) ([]txrules.Transaction, error) {
	return listedTransactions(ctx, view)
}

// RuleStepPrefix begins the history step of a line the rule applied.
const RuleStepPrefix = ruleStepPrefix
