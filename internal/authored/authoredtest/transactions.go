package authoredtest

import (
	"context"
	"errors"
	"slices"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Builders of what is said about transactions. Each writes inside a recorder's
// change, through the recorder and the package's own functions, so a test can
// put several in one change or each in its own.

// InventedAccount is the account every invented row belongs to.
const InventedAccount = "acc-invented"

// Seen is an invented ledger row as the index takes it in, posted.
func Seen(providerID string, day brday.Day, cents money.Cents, establishmentKey string) authored.SeenTransaction {
	return authored.SeenTransaction{
		ProviderID: providerID, AccountID: InventedAccount, Day: day, Cents: cents, EstablishmentKey: establishmentKey,
	}
}

// WriteState makes the transaction at key hold state; the recorder records one
// effect for each column that changes.
func WriteState(ctx context.Context, recorder *authored.Recorder, key authored.TxKey, state authored.TransactionState) error {
	return recorder.Set(ctx, "tx_state", []authored.Value{authored.Integer(int64(key))}, authored.Row{
		"line_id": optional(state.Line), "classified_by": textOrNull(string(state.ClassifiedBy)),
		"confidence": optional(state.Confidence), "pending_reason": textOrNull(state.PendingReason),
		"pending_line_id": optional(state.PendingLine), "pending_confidence": optional(state.PendingConfidence),
		"ask_reason": zeroOrOne(state.AskReason), "note": authored.Text(state.Note), "exclusion": present(state.Exclusion),
		"refund": optionalFlag(state.Refund), "refund_line_id": optional(state.RefundLine),
		"reimbursable": zeroOrOne(state.Reimbursable), "expected_cents": optional(state.ExpectedCents),
		"credit_tx_key": optional(state.Credit), "credit_cents": optional(state.CreditCents),
	})
}

// AddTag gives the transaction at key a tag, at the place after its last one.
// A tag it already has stays where it is.
func AddTag(ctx context.Context, recorder *authored.Recorder, key authored.TxKey, tag string) error {
	view := recorder.View()
	defer view.Close()
	found, err := view.Transactions(ctx, []authored.TxKey{key})
	if err != nil {
		return err
	}
	position, has := nextPosition(found[key].Tags, tag)
	if has {
		return nil
	}
	return recorder.Put(ctx, "tx_tags", authored.Row{
		"tx_key": authored.Integer(int64(key)), "tag": authored.Text(tag), "position": authored.Integer(position),
	})
}

// RemoveTag takes a tag off the transaction at key and moves no other.
func RemoveTag(ctx context.Context, recorder *authored.Recorder, key authored.TxKey, tag string) error {
	return recorder.Delete(ctx, "tx_tags", []authored.Value{authored.Integer(int64(key)), authored.Text(tag)})
}

// AddFixedTag gives an establishment a tag every transaction of it carries, at
// the place after its last one. A tag it already has stays where it is.
func AddFixedTag(ctx context.Context, recorder *authored.Recorder, establishment, tag string) error {
	view := recorder.View()
	defer view.Close()
	found, err := view.Establishments(ctx, []string{establishment})
	if err != nil {
		return err
	}
	position, has := nextPosition(found[establishment].FixedTags, tag)
	if has {
		return nil
	}
	return recorder.Put(ctx, "establishment_tags", authored.Row{
		"key": authored.Text(establishment), "tag": authored.Text(tag), "position": authored.Integer(position),
	})
}

// RemoveFixedTag takes a fixed tag off an establishment and moves no other.
func RemoveFixedTag(ctx context.Context, recorder *authored.Recorder, establishment, tag string) error {
	return recorder.Delete(ctx, "establishment_tags", []authored.Value{authored.Text(establishment), authored.Text(tag)})
}

// nextPosition is the place after the last of tags, and whether tag is among
// them already. A place freed by a removal is not handed out again unless it
// was the last.
func nextPosition(tags []authored.Tag, tag string) (int64, bool) {
	if slices.ContainsFunc(tags, func(held authored.Tag) bool { return held.Name == tag }) {
		return 0, true
	}
	if len(tags) == 0 {
		return 0, false
	}
	return tags[len(tags)-1].Position + 1, false
}

// WriteEstablishment sets an establishment's nickname, empty for none, and its
// default line, nil for none.
func WriteEstablishment(ctx context.Context, recorder *authored.Recorder, establishment, nickname string, line *authored.LineID) error {
	return recorder.Set(ctx, "establishments", []authored.Value{authored.Text(establishment)}, authored.Row{
		"nickname": textOrNull(nickname), "default_line_id": optional(line),
	})
}

// WriteSplit makes the transaction at key split into parts, each at its own
// position; a part held at a position parts does not name is removed.
func WriteSplit(ctx context.Context, recorder *authored.Recorder, key authored.TxKey, parts []authored.SplitPart) error {
	view := recorder.View()
	defer view.Close()
	found, err := view.Transactions(ctx, []authored.TxKey{key})
	if err != nil {
		return err
	}
	for _, held := range found[key].Split {
		named := slices.ContainsFunc(parts, func(part authored.SplitPart) bool { return part.Position == held.Position })
		if named {
			continue
		}
		if err := recorder.Delete(ctx, "tx_split_parts", []authored.Value{authored.Integer(int64(key)), authored.Integer(held.Position)}); err != nil {
			return err
		}
	}
	for _, part := range parts {
		err := recorder.Put(ctx, "tx_split_parts", authored.Row{
			"tx_key": authored.Integer(int64(key)), "position": authored.Integer(part.Position),
			"line_id": optional(part.Line), "cents": authored.Integer(int64(part.Cents)),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// GiveReason keeps the person's reason for a step beside it, replacing the one
// it had.
func GiveReason(ctx context.Context, recorder *authored.Recorder, step authored.StepID, reason string) error {
	return recorder.Put(ctx, "history_step_reasons", authored.Row{
		"step_id": authored.Integer(int64(step)), "reason": authored.Text(reason),
	})
}

func optional[T ~int64](value *T) authored.Value {
	if value == nil {
		return authored.Null
	}
	return authored.Integer(int64(*value))
}

func textOrNull(text string) authored.Value {
	if text == "" {
		return authored.Null
	}
	return authored.Text(text)
}

func present(text *string) authored.Value {
	if text == nil {
		return authored.Null
	}
	return authored.Text(*text)
}

func optionalFlag(on *bool) authored.Value {
	if on == nil {
		return authored.Null
	}
	return zeroOrOne(*on)
}

func zeroOrOne(on bool) authored.Value {
	if on {
		return authored.Integer(1)
	}
	return authored.Integer(0)
}

// The invented transactions SeedTransactions writes, by provider id. Nothing of
// them is real.
const (
	SeedPendingPurchase = "seed-mercado-pendente"
	SeedPurchase        = "seed-mercado"
	SeedRuled           = "seed-padaria"
	SeedSuggested       = "seed-farmacia"
	SeedUnsuggested     = "seed-loja"
	SeedSplit           = "seed-atacado"
	SeedExcluded        = "seed-transferencia"
	SeedSwitchedOff     = "seed-saque"
	SeedRefund          = "seed-estorno"
	SeedReimbursable    = "seed-jantar"
	SeedCredit          = "seed-pix"
	SeedEstablishment   = "name:mercado-exemplo"
)

// SeedTransactions writes into recorder's change what a person who has used
// the app for a while would have said about transactions, on the first two
// lines of the budget in force for 2026-02, which SeedBudget writes: a purchase
// seen pending and then posted under another id, put on a line by the person
// with a note, own tags and a step with a reason; one put by a rule, its step
// without a reason; a pending review with a suggestion and one without; a
// split with a part on a line and one on none; an exclusion with a reason and
// one without; a refund override with its line; a reimbursable purchase with
// an expected amount, its linked credit and a note step; and an establishment
// with a nickname, a default line and fixed tags.
func SeedTransactions(recorder *authored.Recorder) error {
	ctx := context.Background()
	view := recorder.View()
	defer view.Close()
	budget, err := view.BudgetOf(ctx, "2026-02")
	if err != nil {
		return err
	}
	if len(budget.Lines) < 2 {
		return errors.New("authoredtest: the transactions are seeded on a budget with two lines")
	}
	seed := &seeder{ctx: ctx, recorder: recorder, first: budget.Lines[0], second: budget.Lines[1]}
	for _, write := range []func() error{seed.purchases, seed.reviews, seed.splitAndExclusions, seed.refundAndReimbursement, seed.establishment} {
		if err := write(); err != nil {
			return err
		}
	}
	return nil
}

type seeder struct {
	ctx           context.Context
	recorder      *authored.Recorder
	first, second authored.BudgetLine
}

func (seed *seeder) index(providerID string, day brday.Day, cents money.Cents, pending bool, replaces authored.TxKey) (authored.TxKey, error) {
	seen := Seen(providerID, day, cents, SeedEstablishment)
	seen.BankPending, seen.Replaces = pending, replaces
	return seed.recorder.IndexTransaction(seed.ctx, seen)
}

func (seed *seeder) indexed(providerID string, day brday.Day, cents money.Cents, state authored.TransactionState) (authored.TxKey, error) {
	key, err := seed.index(providerID, day, cents, false, 0)
	if err != nil {
		return 0, err
	}
	return key, WriteState(seed.ctx, seed.recorder, key, state)
}

func (seed *seeder) purchases() error {
	pending, err := seed.index(SeedPendingPurchase, "2026-02-09", 12_000, true, 0)
	if err != nil {
		return err
	}
	if _, err := seed.index(SeedPurchase, "2026-02-10", 12_000, false, pending); err != nil {
		return err
	}
	state := authored.TransactionState{Line: Some(seed.first.ID), ClassifiedBy: authored.AuthorPerson, Note: "Compra do mês"}
	if err := WriteState(seed.ctx, seed.recorder, pending, state); err != nil {
		return err
	}
	for _, tag := range []string{"#casa", "#mensal"} {
		if err := AddTag(seed.ctx, seed.recorder, pending, tag); err != nil {
			return err
		}
	}
	step, err := seed.recorder.AddHistoryStep(seed.ctx, pending, authored.StepEffect, "Você colocou em "+seed.first.Name)
	if err != nil {
		return err
	}
	if err := GiveReason(seed.ctx, seed.recorder, step, "É compra de casa"); err != nil {
		return err
	}
	ruled, err := seed.indexed(SeedRuled, "2026-02-11", 1_500, authored.TransactionState{Line: Some(seed.second.ID), ClassifiedBy: authored.AuthorRule})
	if err != nil {
		return err
	}
	_, err = seed.recorder.AddHistoryStep(seed.ctx, ruled, authored.StepEffect, "Regra colocou em "+seed.second.Name)
	return err
}

func (seed *seeder) reviews() error {
	suggested := authored.TransactionState{
		PendingReason: "Confiança abaixo do limite", PendingLine: Some(seed.first.ID), PendingConfidence: Some[int64](70),
	}
	if _, err := seed.indexed(SeedSuggested, "2026-02-12", 4_590, suggested); err != nil {
		return err
	}
	_, err := seed.indexed(SeedUnsuggested, "2026-02-13", 9_900, authored.TransactionState{PendingReason: "Estabelecimento novo"})
	return err
}

func (seed *seeder) splitAndExclusions() error {
	split, err := seed.index(SeedSplit, "2026-02-14", 8_000, false, 0)
	if err != nil {
		return err
	}
	parts := []authored.SplitPart{{Position: 0, Line: Some(seed.first.ID), Cents: 5_000}, {Position: 1, Cents: 3_000}}
	if err := WriteSplit(seed.ctx, seed.recorder, split, parts); err != nil {
		return err
	}
	if _, err := seed.indexed(SeedExcluded, "2026-02-15", 50_000, authored.TransactionState{Exclusion: Some("Transferência entre contas")}); err != nil {
		return err
	}
	_, err = seed.indexed(SeedSwitchedOff, "2026-02-16", 20_000, authored.TransactionState{Exclusion: Some("")})
	return err
}

func (seed *seeder) refundAndReimbursement() error {
	refund := authored.TransactionState{Refund: Some(true), RefundLine: Some(seed.second.ID)}
	if _, err := seed.indexed(SeedRefund, "2026-02-17", 2_500, refund); err != nil {
		return err
	}
	credit, err := seed.index(SeedCredit, "2026-02-19", 4_000, false, 0)
	if err != nil {
		return err
	}
	reimbursable := authored.TransactionState{
		Reimbursable: true, ExpectedCents: Some[money.Cents](4_000), Credit: Some(credit), CreditCents: Some[money.Cents](4_000),
	}
	purchase, err := seed.indexed(SeedReimbursable, "2026-02-18", 8_000, reimbursable)
	if err != nil {
		return err
	}
	_, err = seed.recorder.AddHistoryStep(seed.ctx, purchase, authored.StepNote, "O banco confirmou a compra")
	return err
}

func (seed *seeder) establishment() error {
	if err := WriteEstablishment(seed.ctx, seed.recorder, SeedEstablishment, "Mercado da esquina", Some(seed.first.ID)); err != nil {
		return err
	}
	for _, tag := range []string{"#mercado", "#essencial"} {
		if err := AddFixedTag(seed.ctx, seed.recorder, SeedEstablishment, tag); err != nil {
			return err
		}
	}
	return nil
}
