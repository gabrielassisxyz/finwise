package authored

import (
	"context"
	"slices"
)

// The person's say in what a transaction counts for: the parts it is split
// into, whether it is left out of the totals, and whether a credit is a refund
// and of which line. Each write goes through the recorder; the three that a
// history step follows answer whether they recorded anything, since a step is
// written only for a change that changed the transaction.

// SetSplit makes the parts of the transaction at key the ones given, each at
// its own position, and removes a part held at a position they do not name. A
// part that keeps its line and amount records nothing. Fewer than two parts are
// no split: every part held is removed and none is kept.
func (recorder *Recorder) SetSplit(ctx context.Context, key TxKey, parts []SplitPart) error {
	held, err := recorder.splitOf(ctx, key)
	if err != nil {
		return err
	}
	if len(parts) < 2 {
		parts = nil
	}
	owner := Integer(int64(key))
	for _, part := range held {
		named := slices.ContainsFunc(parts, func(given SplitPart) bool { return given.Position == part.Position })
		if named {
			continue
		}
		if err := recorder.Delete(ctx, txSplitPartsTable, []Value{owner, Integer(part.Position)}); err != nil {
			return err
		}
	}
	for _, part := range parts {
		row := Row{"tx_key": owner, "position": Integer(part.Position), "line_id": lineValue(part.Line), "cents": Integer(int64(part.Cents))}
		if err := recorder.Put(ctx, txSplitPartsTable, row); err != nil {
			return err
		}
	}
	return nil
}

func (recorder *Recorder) splitOf(ctx context.Context, key TxKey) ([]SplitPart, error) {
	view := recorder.View()
	defer view.Close()
	found, err := view.Transactions(ctx, []TxKey{key})
	if err != nil {
		return nil, recorder.fail(err)
	}
	return found[key].Split, nil
}

// SetExclusion stores the exclusion of the transaction at key: nil while it
// counts, empty while it is switched off without a reason, which leaves it
// counting, and the reason otherwise. It answers whether anything changed.
func (recorder *Recorder) SetExclusion(ctx context.Context, key TxKey, exclusion *string) (bool, error) {
	value := Null
	if exclusion != nil {
		value = Text(*exclusion)
	}
	return recorder.setTransactionFact(ctx, key, "exclusion", value)
}

// SetRefund stores the person's word on whether the transaction at key is a
// refund, which overrides the ledger's rule from then on. It answers whether
// anything changed.
func (recorder *Recorder) SetRefund(ctx context.Context, key TxKey, on bool) (bool, error) {
	value := Integer(0)
	if on {
		value = Integer(1)
	}
	return recorder.setTransactionFact(ctx, key, "refund", value)
}

// SetRefundLine stores the line a refund takes off, whether or not the
// transaction is marked as one. It answers whether anything changed.
func (recorder *Recorder) SetRefundLine(ctx context.Context, key TxKey, line LineID) (bool, error) {
	return recorder.setTransactionFact(ctx, key, "refund_line_id", Integer(int64(line)))
}

// setTransactionFact sets one column of the transaction's facts and says
// whether the recorder recorded an effect for it.
func (recorder *Recorder) setTransactionFact(ctx context.Context, key TxKey, column string, value Value) (bool, error) {
	before := recorder.seq
	if err := recorder.Set(ctx, txStateTable, []Value{Integer(int64(key))}, Row{column: value}); err != nil {
		return false, err
	}
	return recorder.seq > before, nil
}

func lineValue(line *LineID) Value {
	if line == nil {
		return Null
	}
	return Integer(int64(*line))
}
