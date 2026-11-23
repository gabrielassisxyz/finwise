package api

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// What the person says a transaction counts for: the parts a purchase is split
// into, leaving a transaction out of the totals, and whether a credit is a
// refund and of which line. Each write is one change on the transaction the
// list shows for the id, found by the lookup every write uses. What each of
// them then counts for is Dispose's to say, from what these store.

func registerTransactionCountWrites(registry *Registry) {
	registry.RegisterWrite("transactions.setSplit", setSplit)
	registry.RegisterWrite("transactions.setExcluded", setExcluded)
	registry.RegisterWrite("transactions.setExclusionReason", setExclusionReason)
	registry.RegisterWrite("transactions.excludeMany", excludeMany)
	registry.RegisterWrite("transactions.setRefund", setRefund)
	registry.RegisterWrite("transactions.setRefundLine", setRefundLine)
}

// splitPartArgument is the contract's SplitPart. Amount is a pointer so that an
// amount left out is refused instead of reading as zero.
type splitPartArgument struct {
	Line   *string `json:"line"`
	Amount *Reais  `json:"amount"`
}

// setSplit replaces the parts of a split whole, in the order sent. A part may
// be on no line and may be zero, as the panel starts a new one, and nothing
// makes the parts add up while the person types them; a negative one is not
// an amount the panel can mean. Each part's line is the name it has in the
// budget of the transaction's own month.
func setSplit(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	id, err := textArgument(args[0])
	if err != nil {
		return nil, err
	}
	var given []splitPartArgument
	if err := decodeArgument(args[1], &given); err != nil {
		return nil, err
	}
	for _, part := range given {
		if part.Amount == nil || *part.Amount < 0 {
			return nil, &badArgumentsError{}
		}
	}
	target, err := targetNamedBy(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	if len(given) >= 2 {
		if err := refuseSplitOfALinkedPurchase(ctx, writes, target.key); err != nil {
			return nil, err
		}
	}
	lines, err := linesOfMonth(ctx, writes.Authored, monthOfDay(target.listed.Day))
	if err != nil {
		return nil, err
	}
	parts := make([]authored.SplitPart, 0, len(given))
	for position, part := range given {
		line, err := lineOfPart(lines, part.Line)
		if err != nil {
			return nil, err
		}
		parts = append(parts, authored.SplitPart{Position: int64(position), Line: line, Cents: money.Cents(*part.Amount)})
	}
	if err := writes.Recorder.SetSplit(ctx, target.key, parts); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

func lineOfPart(lines monthLines, name *string) (*authored.LineID, error) {
	if name == nil {
		return nil, nil
	}
	id, found := lines.idOf(*name)
	if !found {
		return nil, refuse(reasonUnknownLine)
	}
	return &id, nil
}

// setExcluded switches the exclusion on without a reason, which keeps the
// transaction counting until one is given, whatever reason it had; switching
// it off counts the transaction again and says so in its history.
func setExcluded(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, on, err := idAndFlag(args)
	if err != nil {
		return nil, err
	}
	target, err := targetNamedBy(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	if on {
		noReason := ""
		if _, err := writes.Recorder.SetExclusion(ctx, target.key, &noReason); err != nil {
			return nil, err
		}
		return changeResultOf(ctx, writes.Recorder)
	}
	changed, err := writes.Recorder.SetExclusion(ctx, target.key, nil)
	if err != nil {
		return nil, err
	}
	return answerWithStep(ctx, writes, target.key, countedAgainStep, changed)
}

// setExclusionReason keeps the reason trimmed, and so switches the exclusion on
// when it was off; a reason of spaces alone is no reason, and the transaction
// keeps counting.
func setExclusionReason(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, reason, err := idAndText(args)
	if err != nil {
		return nil, err
	}
	target, err := targetNamedBy(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(reason)
	if _, err := writes.Recorder.SetExclusion(ctx, target.key, &trimmed); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// excludeMany keeps the reason as sent on every transaction of the selection,
// all of them or none: the whole list is found before anything is written, so
// one id the list does not show refuses the call and indexes nothing.
func excludeMany(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	var ids []string
	if err := decodeArgument(args[0], &ids); err != nil {
		return nil, err
	}
	reason, err := textArgument(args[1])
	if err != nil {
		return nil, err
	}
	rows, err := listedSelection(ctx, writes.Ledger, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		key, err := indexListed(ctx, writes.Ledger, writes.Recorder, row.listed)
		if err != nil {
			return nil, err
		}
		if _, err := writes.Recorder.SetExclusion(ctx, key, &reason); err != nil {
			return nil, err
		}
	}
	return changeResultOf(ctx, writes.Recorder)
}

// setRefund keeps the person's word on whether a credit is a refund, which
// then decides over the ledger's rule. Unmarking it keeps the line chosen for
// it.
func setRefund(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, on, err := idAndFlag(args)
	if err != nil {
		return nil, err
	}
	target, err := targetNamedBy(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	changed, err := writes.Recorder.SetRefund(ctx, target.key, on)
	if err != nil {
		return nil, err
	}
	step := refundUnmarkedStep
	if on {
		step = refundMarkedStep
	}
	return answerWithStep(ctx, writes, target.key, step, changed)
}

// setRefundLine keeps the line a refund takes off, named in the budget of the
// transaction's own month, whether the mark is on or not.
func setRefundLine(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, name, err := idAndText(args)
	if err != nil {
		return nil, err
	}
	target, err := targetNamedBy(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	lines, err := linesOfMonth(ctx, writes.Authored, monthOfDay(target.listed.Day))
	if err != nil {
		return nil, err
	}
	line, found := lines.idOf(name)
	if !found {
		return nil, refuse(reasonUnknownLine)
	}
	changed, err := writes.Recorder.SetRefundLine(ctx, target.key, line)
	if err != nil {
		return nil, err
	}
	return answerWithStep(ctx, writes, target.key, refundLineStep(name), changed)
}

// answerWithStep writes the step that says what the change did, when it did
// something to the transaction, and answers the change.
func answerWithStep(ctx context.Context, writes Writes, key authored.TxKey, text string, changed bool) (any, error) {
	if changed {
		if _, err := writes.Recorder.AddHistoryStep(ctx, key, authored.StepEffect, text); err != nil {
			return nil, err
		}
	}
	return changeResultOf(ctx, writes.Recorder)
}

func idAndFlag(args []json.RawMessage) (string, bool, error) {
	if len(args) != 2 {
		return "", false, &badArgumentsError{}
	}
	id, err := textArgument(args[0])
	if err != nil {
		return "", false, err
	}
	var on *bool
	if err := decodeArgument(args[1], &on); err != nil || on == nil {
		return "", false, &badArgumentsError{}
	}
	return id, *on, nil
}
