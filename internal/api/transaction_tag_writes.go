package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// The person's tags and notes on their transactions. Each write is one change
// through the store, and the transaction it names is found as the detail finds
// it, in the request's snapshot, so a tag can never end up on a row the list
// does not show. No month is closed to these: closing freezes a budget, not the
// words a person keeps beside a purchase.

func registerTransactionTagWrites(registry *Registry) {
	registry.RegisterWrite("transactions.addTag", addTransactionTag)
	registry.RegisterWrite("transactions.removeTag", removeTransactionTag)
	registry.RegisterWrite("transactions.addTagToMany", addTagToTransactions)
	registry.RegisterWrite("transactions.setNote", setTransactionNote)
}

// addTransactionTag gives the transaction a tag; the store spells it.
func addTransactionTag(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, tag, err := idAndText(args)
	if err != nil {
		return nil, err
	}
	target, err := targetNamedBy(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	if err := writes.Recorder.AddTag(ctx, target.key, tag); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// removeTransactionTag takes a tag off by the text the person sees on its chip.
func removeTransactionTag(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, tag, err := idAndText(args)
	if err != nil {
		return nil, err
	}
	target, err := targetNamedBy(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	if err := writes.Recorder.RemoveTag(ctx, target.key, tag); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// addTagToTransactions tags a selection. Every id is found before anything is
// written, so one the list does not show refuses the whole call with nothing
// recorded, not even in the index; an id named twice counts once.
func addTagToTransactions(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	var ids []string
	if err := decodeArgument(args[0], &ids); err != nil || ids == nil {
		return nil, &badArgumentsError{}
	}
	tag, err := textArgument(args[1])
	if err != nil {
		return nil, err
	}
	targets, err := targetsNamedBy(ctx, writes, ids)
	if err != nil {
		return nil, err
	}
	for _, target := range targets {
		if err := writes.Recorder.AddTag(ctx, target.key, tag); err != nil {
			return nil, err
		}
	}
	return changeResultOf(ctx, writes.Recorder)
}

// setTransactionNote keeps the note exactly as it was written.
func setTransactionNote(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, note, err := idAndText(args)
	if err != nil {
		return nil, err
	}
	target, err := targetNamedBy(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	if err := writes.Recorder.SetNote(ctx, target.key, note); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// idAndText reads the two texts of a write of one transaction: its id, and what
// the person typed.
func idAndText(args []json.RawMessage) (id, text string, err error) {
	if len(args) != 2 {
		return "", "", &badArgumentsError{}
	}
	if id, err = textArgument(args[0]); err != nil {
		return "", "", err
	}
	if text, err = textArgument(args[1]); err != nil {
		return "", "", err
	}
	return id, text, nil
}

// targetNamedBy is the transaction a write names, indexed inside the write's
// change. An id the list does not show (an unknown one, a tax row folded into
// its purchase, a pot movement) is no transaction: the write is refused and
// records nothing.
func targetNamedBy(ctx context.Context, writes Writes, id string) (transactionTarget, error) {
	target, found, err := lookupTarget(ctx, writes, id)
	if err != nil {
		return transactionTarget{}, err
	}
	if !found {
		return transactionTarget{}, refuse(reasonNotFound)
	}
	return target, nil
}

// targetsNamedBy is targetNamedBy for a selection, resolved against one reading
// of the whole ledger. The ids are all found before the first is indexed, each
// once, in the order they were named.
func targetsNamedBy(ctx context.Context, writes Writes, ids []string) ([]transactionTarget, error) {
	view, around, err := listedLedger(ctx, writes.Ledger)
	if err != nil {
		return nil, err
	}
	shown := make(map[string]txrules.Transaction, len(view.listed))
	for _, listed := range view.listed {
		shown[listed.ID] = listed
	}
	var rows []listedRow
	named := map[string]bool{}
	for _, id := range ids {
		if named[id] {
			continue
		}
		named[id] = true
		listed, found := shown[id]
		if !found {
			return nil, refuse(reasonNotFound)
		}
		rows = append(rows, listedRow{listed: listed, source: view.rows[id], around: around})
	}
	targets := make([]transactionTarget, 0, len(rows))
	for _, row := range rows {
		key, err := indexListed(ctx, writes.Recorder, row.listed)
		if err != nil {
			return nil, err
		}
		targets = append(targets, transactionTarget{listedRow: row, key: key})
	}
	return targets, nil
}
