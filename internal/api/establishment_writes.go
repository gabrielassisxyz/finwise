package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// What the person says about the establishment or the person behind a
// transaction: a name they recognise, the line its purchases are put on from now
// on, and the tags every transaction of it carries. All of it is kept by the
// establishment's key, so one write reaches every transaction of it, and the
// handlers only find the key of the transaction they were given.

func registerEstablishmentWrites(registry *Registry) {
	registry.RegisterWrite("transactions.setNickname", setTransactionNickname)
	registry.RegisterWrite("transactions.setDefaultLine", setTransactionDefaultLine)
	registry.RegisterWrite("transactions.addFixedTag", addEstablishmentFixedTag)
	registry.RegisterWrite("transactions.removeFixedTag", removeEstablishmentFixedTag)
}

// setTransactionNickname names every transaction of the establishment or the
// person; a blank nickname takes the name away.
func setTransactionNickname(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, nickname, err := idAndText(args)
	if err != nil {
		return nil, err
	}
	key, err := counterpartKeyNamedBy(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	if err := writes.Recorder.SetNickname(ctx, key, nickname); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// setTransactionDefaultLine stores the line by id, resolved in the budget in
// force for the current month: a default line belongs to no month, and the
// month of the transaction it was chosen on is no reason to look there. It
// touches no transaction, and only an establishment has one.
func setTransactionDefaultLine(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, name, err := idAndText(args)
	if err != nil {
		return nil, err
	}
	target, err := targetNamedBy(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	if !sectionsOf(target.listed).Establishment {
		return nil, refuse(reasonNotFound)
	}
	if err := storeDefaultLine(ctx, writes, target.listed.EstablishmentKey, name); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// storeDefaultLine is the one way a default line is kept, whichever door the
// person came through: the name is resolved in the budget in force for the
// current month, whatever month a transaction they were looking at is in, and
// stored by id. A name that budget lacks is refused.
func storeDefaultLine(ctx context.Context, writes Writes, establishmentKey, name string) error {
	lines, err := linesOfMonth(ctx, writes.Authored, brday.MonthOf(nowOf(ctx)))
	if err != nil {
		return err
	}
	line, found := lines.idOf(name)
	if !found {
		return refuse(reasonUnknownLine)
	}
	return writes.Recorder.SetDefaultLine(ctx, establishmentKey, line)
}

// addEstablishmentFixedTag gives the establishment a tag that every
// transaction of it carries; the store spells it as it does an own tag.
func addEstablishmentFixedTag(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, tag, err := idAndText(args)
	if err != nil {
		return nil, err
	}
	key, err := counterpartKeyNamedBy(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	if err := writes.Recorder.AddFixedTag(ctx, key, tag); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

func removeEstablishmentFixedTag(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, tag, err := idAndText(args)
	if err != nil {
		return nil, err
	}
	key, err := counterpartKeyNamedBy(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	if err := writes.Recorder.RemoveFixedTag(ctx, key, tag); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// counterpartKeyNamedBy is the key of the establishment or the person behind
// the transaction a write names. A movement has neither: the panel offers it no
// such section, and a nickname or a tag stored under its key could not be taken
// away from there while the list kept showing it on every row of that key, so a
// write for one is refused as for a transaction that does not exist.
func counterpartKeyNamedBy(ctx context.Context, writes Writes, id string) (string, error) {
	target, err := targetNamedBy(ctx, writes, id)
	if err != nil {
		return "", err
	}
	if sections := sectionsOf(target.listed); !sections.Establishment && !sections.Person {
		return "", refuse(reasonNotFound)
	}
	return target.listed.EstablishmentKey, nil
}
