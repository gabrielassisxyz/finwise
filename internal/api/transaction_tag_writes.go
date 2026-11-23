package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
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
	rows, err := listedSelection(ctx, writes.Ledger, ids)
	if err != nil {
		return nil, err
	}
	keys := make([]authored.TxKey, 0, len(rows))
	for _, row := range rows {
		key, err := indexListed(ctx, writes.Ledger, writes.Recorder, row.listed)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	for _, key := range keys {
		if err := writes.Recorder.AddTag(ctx, key, tag); err != nil {
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

// listedSelection is the listed transaction of each id of a selection, each
// once, in the order first named, found as findListed finds one. An id the list
// does not show refuses the whole selection. Nothing is indexed: every id is
// found before the caller indexes or writes any of them, and the index is not
// journaled, so a row indexed before a later id failed would outlive the
// refusal.
func listedSelection(ctx context.Context, snapshot *ledger.View, ids []string) ([]listedRow, error) {
	return selectionAmong(ctx, snapshot, ids, func() ([]ledger.Transaction, error) {
		return snapshot.CleanTransactions(ctx, wholeLedger)
	})
}

// selectionAmong is listedSelection over the rows readRows answers. It reads
// them once and interprets each month once, however many ids the selection
// holds.
func selectionAmong(ctx context.Context, snapshot *ledger.View, ids []string, readRows func() ([]ledger.Transaction, error)) ([]listedRow, error) {
	all, err := readRows()
	if err != nil {
		return nil, err
	}
	around, err := loadContext(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	months := monthListings{all: all, around: around, read: map[brday.Month]map[string]txrules.Transaction{}}
	rows := make([]listedRow, 0, len(ids))
	named := map[string]bool{}
	for _, id := range ids {
		if named[id] {
			continue
		}
		named[id] = true
		row, found, err := months.find(id)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, refuse(reasonNotFound)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// monthListings is one reading of the ledger's rows with the months already
// interpreted from it.
type monthListings struct {
	all    []ledger.Transaction
	around ledgerContext
	read   map[brday.Month]map[string]txrules.Transaction
}

func (months monthListings) find(id string) (listedRow, bool, error) {
	source, found := rowWithID(months.all, id)
	if !found {
		return listedRow{}, false, nil
	}
	month := brday.MonthOf(source.Date)
	listed, read := months.read[month]
	if !read {
		view, err := interpretMonthOf(months.all, source, months.around)
		if err != nil {
			return listedRow{}, false, err
		}
		listed = make(map[string]txrules.Transaction, len(view.listed))
		for _, transaction := range view.listed {
			listed[transaction.ID] = transaction
		}
		months.read[month] = listed
	}
	transaction, found := listed[id]
	if !found {
		return listedRow{}, false, nil
	}
	return listedRow{listed: transaction, source: source, around: months.around}, true, nil
}
