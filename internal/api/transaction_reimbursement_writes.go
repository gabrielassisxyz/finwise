package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// A purchase the person expects to be paid back for, and the received PIX that
// paid it. Each write is one change on the transaction the list shows for the
// id, found by the lookup every write uses; what the purchase and the credit
// then count for is Dispose's to say, from what these store.

func registerTransactionReimbursementWrites(registry *Registry) {
	registry.RegisterWrite("transactions.setReimbursable", setReimbursable)
	registry.RegisterWrite("transactions.setExpectedReimbursement", setExpectedReimbursement)
	registry.RegisterWrite("transactions.linkReimbursement", linkReimbursement)
	registry.RegisterWrite("transactions.unlinkReimbursement", unlinkReimbursement)
}

// setReimbursable marks a transaction as one the person expects half of back,
// whatever was kept before, so marking again starts over and drops the link;
// unmarking takes the amount expected and the link away, which frees the credit.
func setReimbursable(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, on, err := idAndFlag(args)
	if err != nil {
		return nil, err
	}
	target, err := countedTarget(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	if on {
		err = writes.Recorder.MarkReimbursable(ctx, target.key, halfOf(target.listed.Amount))
	} else {
		err = writes.Recorder.UnmarkReimbursable(ctx, target.key)
	}
	if err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// halfOf is half of what the list shows was paid, tax included, rounded up to
// the cent.
func halfOf(amount money.Cents) money.Cents {
	if amount < 0 {
		amount = -amount
	}
	return (amount + 1) / 2
}

// setExpectedReimbursement keeps what the person expects back, nothing when the
// field was emptied, while the transaction is marked; on one that is not it
// changes nothing. An amount below zero is not one the field can mean.
func setExpectedReimbursement(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	id, err := textArgument(args[0])
	if err != nil {
		return nil, err
	}
	var amount *Reais
	if err := decodeArgument(args[1], &amount); err != nil {
		return nil, err
	}
	if amount != nil && *amount < 0 {
		return nil, &badArgumentsError{}
	}
	target, err := countedTarget(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	kept, err := keptAbout(ctx, writes.Authored, target.key)
	if err != nil || !kept.State.Reimbursable {
		return changeResultOrFailure(ctx, writes, err)
	}
	var expected *money.Cents
	if amount != nil {
		cents := money.Cents(*amount)
		expected = &cents
	}
	if err := writes.Recorder.SetExpectedReimbursement(ctx, target.key, expected); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// linkReimbursement links a received PIX to a marked purchase as what paid it
// back, replacing the one it had. On a transaction that is not a marked
// purchase in reais it changes nothing. A split purchase is refused before the
// credit is looked at; a credit that is not among the purchase's candidates is
// not found.
func linkReimbursement(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, creditID, err := idAndText(args)
	if err != nil {
		return nil, err
	}
	target, err := countedTarget(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	kept, err := keptAbout(ctx, writes.Authored, target.key)
	if err != nil || !takesALink(target.listed) || !kept.State.Reimbursable {
		return changeResultOrFailure(ctx, writes, err)
	}
	if len(kept.Split) >= 2 {
		return nil, refuse(reasonSplitWithReimbursement)
	}
	candidates, err := readCandidates(ctx, Views{Ledger: writes.Ledger, Authored: writes.Authored}, target.key, true)
	if err != nil {
		return nil, err
	}
	credit, found := candidates.withID(creditID)
	if !found {
		return nil, refuse(reasonNotFound)
	}
	creditKey, err := indexListed(ctx, writes.Recorder, credit)
	if err != nil {
		return nil, err
	}
	linked, err := writes.Recorder.LinkReimbursement(ctx, target.key, creditKey, credit.Amount)
	if err != nil {
		return nil, err
	}
	return answerWithStep(ctx, writes, target.key, linkedReimbursementStep(candidates.nameOf(credit), credit.Amount), linked)
}

// takesALink is a purchase whose amount is in reais. An unconverted one counts
// for nothing until the bank states its value in reais, and a credit in reais
// taken off it would be a figure of two currencies.
func takesALink(listed txrules.Transaction) bool {
	return sectionsOf(listed).Reimbursement && !listed.Unconverted
}

// unlinkReimbursement drops the link and keeps the mark and the amount
// expected. The credit's review, settled by the link, is not brought back.
func unlinkReimbursement(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, err := stringArgument(args)
	if err != nil {
		return nil, err
	}
	target, err := countedTarget(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	unlinked, err := writes.Recorder.UnlinkReimbursement(ctx, target.key)
	if err != nil {
		return nil, err
	}
	return answerWithStep(ctx, writes, target.key, unlinkedReimbursementStep, unlinked)
}

// refuseSplitOfALinkedPurchase is setSplit's half of the rule that a purchase
// is either split or paid back by a linked credit: a stored link refuses the
// split even while its credit is not listed, since the link applies again once
// it is.
func refuseSplitOfALinkedPurchase(ctx context.Context, writes Writes, key authored.TxKey) error {
	kept, err := keptAbout(ctx, writes.Authored, key)
	if err != nil {
		return err
	}
	if kept.State.Credit != nil {
		return refuse(reasonSplitWithReimbursement)
	}
	return nil
}

func keptAbout(ctx context.Context, view *authored.View, key authored.TxKey) (authored.AuthoredTransaction, error) {
	found, err := view.Transactions(ctx, []authored.TxKey{key})
	if err != nil {
		return authored.AuthoredTransaction{}, err
	}
	return found[key], nil
}

// changeResultOrFailure answers err, or the change when there is none.
func changeResultOrFailure(ctx context.Context, writes Writes, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}
