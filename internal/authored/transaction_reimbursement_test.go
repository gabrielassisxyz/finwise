package authored_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// A purchase, two credits and a second purchase, indexed, the first credit
// waiting for a review with a suggestion.
type reimbursementKeys struct {
	purchase, credit, otherCredit, otherPurchase authored.TxKey
}

const linkedStepText = "Você ligou ao PIX de Ana Exemplo · R$\u00a0150,00"

func reimbursementDatabase(t *testing.T) (*authored.Database, reimbursementKeys) {
	t.Helper()
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	keys := reimbursementKeys{
		purchase:      indexed(t, database, seenRow("p-dinner", "2026-11-10", -18_000, "name:restaurante")),
		credit:        indexed(t, database, seenRow("p-pix", "2026-11-12", 15_000, "doc:11122233344")),
		otherCredit:   indexed(t, database, seenRow("p-pix-2", "2026-11-13", 20_000, "doc:55566677788")),
		otherPurchase: indexed(t, database, seenRow("p-lunch", "2026-11-14", -6_000, "name:restaurante")),
	}
	changeBy(t, database, "scratch.seed", func(recorder *authored.Recorder) error {
		review := authored.TransactionState{PendingReason: "Estabelecimento novo", PendingLine: lineID(1), PendingConfidence: authoredtest.Some[int64](55)}
		return authoredtest.WriteState(context.Background(), recorder, keys.credit, review)
	})
	return database, keys
}

func mark(t *testing.T, database *authored.Database, key authored.TxKey, expected money.Cents) authored.ChangeID {
	t.Helper()
	return changeBy(t, database, "transactions.setReimbursable", func(recorder *authored.Recorder) error {
		return recorder.MarkReimbursable(context.Background(), key, expected)
	})
}

// link links credit to purchase and writes the step when the link changed, as
// the write does.
func link(t *testing.T, database *authored.Database, purchase, credit authored.TxKey, cents money.Cents) authored.ChangeID {
	t.Helper()
	return changeBy(t, database, "transactions.linkReimbursement", func(recorder *authored.Recorder) error {
		ctx := context.Background()
		linked, err := recorder.LinkReimbursement(ctx, purchase, credit, cents)
		if err != nil || !linked {
			return err
		}
		_, err = recorder.AddHistoryStep(ctx, purchase, authored.StepEffect, linkedStepText)
		return err
	})
}

func unlink(t *testing.T, database *authored.Database, purchase authored.TxKey) authored.ChangeID {
	t.Helper()
	return changeBy(t, database, "transactions.unlinkReimbursement", func(recorder *authored.Recorder) error {
		ctx := context.Background()
		unlinked, err := recorder.UnlinkReimbursement(ctx, purchase)
		if err != nil || !unlinked {
			return err
		}
		_, err = recorder.AddHistoryStep(ctx, purchase, authored.StepEffect, "Você desfez a ligação com o PIX")
		return err
	})
}

func creditOf(state authored.TransactionState) (authored.TxKey, money.Cents, bool) {
	if state.Credit == nil || state.CreditCents == nil {
		return 0, 0, false
	}
	return *state.Credit, *state.CreditCents, true
}

func TestMarkingAgainStartsOverAndUnmarkingTakesTheLinkAway(t *testing.T) {
	database, keys := reimbursementDatabase(t)
	mark(t, database, keys.purchase, 9_000)
	link(t, database, keys.purchase, keys.credit, 15_000)
	mark(t, database, keys.purchase, 9_000)
	state := heldOf(t, database, keys.purchase).State
	if _, _, linked := creditOf(state); linked || !state.Reimbursable || state.ExpectedCents == nil || *state.ExpectedCents != 9_000 {
		t.Errorf("marking a linked purchase again left %+v, want marked, 9000 expected and no link", state)
	}
	link(t, database, keys.purchase, keys.credit, 15_000)
	changeBy(t, database, "transactions.setReimbursable", func(recorder *authored.Recorder) error {
		return recorder.UnmarkReimbursable(context.Background(), keys.purchase)
	})
	if state := heldOf(t, database, keys.purchase).State; !reflect.DeepEqual(state, authored.TransactionState{}) {
		t.Errorf("unmarking left %+v, want nothing kept", state)
	}
}

func TestLinkingSettlesTheCreditsReviewAndItsUndoBringsTheReviewBack(t *testing.T) {
	database, keys := reimbursementDatabase(t)
	mark(t, database, keys.purchase, 9_000)
	before := canonical(t, database)
	linked := link(t, database, keys.purchase, keys.credit, 15_000)
	if credit, cents, ok := creditOf(heldOf(t, database, keys.purchase).State); !ok || credit != keys.credit || cents != 15_000 {
		t.Errorf("the purchase links %d of %d (%v), want the credit and 15000", credit, cents, ok)
	}
	if state := heldOf(t, database, keys.credit).State; state.PendingReason != "" || state.PendingLine != nil || state.PendingConfidence != nil {
		t.Errorf("the linked credit still waits: %+v", state)
	}
	undo(t, database, linked)
	if after := canonical(t, database); after != before {
		t.Errorf("the undo of the link left\n%s\nwant\n%s", after, before)
	}
}

func TestTakingBackTheLinksStepRemovesTheLinkAndLeavesTheCreditsReviewSettled(t *testing.T) {
	database, keys := reimbursementDatabase(t)
	mark(t, database, keys.purchase, 9_000)
	link(t, database, keys.purchase, keys.credit, 15_000)
	history := heldOf(t, database, keys.purchase).History
	if len(history) != 1 || history[0].Text != linkedStepText {
		t.Fatalf("the purchase's history is %v, want the link's step", stepTexts(history))
	}
	if _, err := takeBack(t, database, keys.purchase, history[0]); err != nil {
		t.Fatalf("the link's step was refused: %v", err)
	}
	held := heldOf(t, database, keys.purchase)
	if _, _, linked := creditOf(held.State); linked || !held.State.Reimbursable || *held.State.ExpectedCents != 9_000 || len(held.History) != 0 {
		t.Errorf("after taking the step back the purchase is %+v with history %v", held.State, stepTexts(held.History))
	}
	if state := heldOf(t, database, keys.credit).State; state.PendingReason != "" {
		t.Errorf("taking the link's step back brought the credit's review back: %+v", state)
	}
}

// The ways of bringing a link back after its credit was linked to another
// purchase: the undo of the unlinking, of the unmarking and of a link that
// replaced it, and taking the unlinking's step back.
func TestBringingALinkBackOnceItsCreditPaysAnotherPurchaseIsRefusedAndChangesNothing(t *testing.T) {
	away := map[string]func(*testing.T, *authored.Database, reimbursementKeys) authored.ChangeID{
		"unlinking": func(t *testing.T, database *authored.Database, keys reimbursementKeys) authored.ChangeID {
			return unlink(t, database, keys.purchase)
		},
		"unmarking": func(t *testing.T, database *authored.Database, keys reimbursementKeys) authored.ChangeID {
			return changeBy(t, database, "transactions.setReimbursable", func(recorder *authored.Recorder) error {
				return recorder.UnmarkReimbursable(context.Background(), keys.purchase)
			})
		},
		"linking another credit": func(t *testing.T, database *authored.Database, keys reimbursementKeys) authored.ChangeID {
			return link(t, database, keys.purchase, keys.otherCredit, 20_000)
		},
	}
	for label, takeAway := range away {
		database, keys := reimbursementDatabase(t)
		mark(t, database, keys.purchase, 9_000)
		link(t, database, keys.purchase, keys.credit, 15_000)
		taken := takeAway(t, database, keys)
		mark(t, database, keys.otherPurchase, 3_000)
		link(t, database, keys.otherPurchase, keys.credit, 15_000)
		before := canonical(t, database)
		assertStale(t, database.Undo(t.Context(), taken, undoInstant), authored.CauseConstraint)
		if after := canonical(t, database); after != before {
			t.Errorf("%s: the refused undo changed the database:\n%s", label, after)
		}
	}
	database, keys := reimbursementDatabase(t)
	mark(t, database, keys.purchase, 9_000)
	link(t, database, keys.purchase, keys.credit, 15_000)
	unlink(t, database, keys.purchase)
	mark(t, database, keys.otherPurchase, 3_000)
	link(t, database, keys.otherPurchase, keys.credit, 15_000)
	history := heldOf(t, database, keys.purchase).History
	before := canonical(t, database)
	step := history[len(history)-1]
	err := database.Change(t.Context(), facts("transactions.undoHistoryStep"), func(recorder *authored.Recorder) error {
		return recorder.RestorePart(context.Background(), step.Change, stepUnits(keys.purchase, step))
	})
	var refused *authored.StaleChangeError
	if !errors.As(err, &refused) || refused.Cause != authored.CauseConstraint {
		t.Errorf("taking the unlinking's step back answered %v, want a refusal by the database", err)
	}
	if after := canonical(t, database); after != before {
		t.Errorf("the refused step changed the database:\n%s", after)
	}
}

func TestTheLinkWritesSayWhetherTheyChangedTheLink(t *testing.T) {
	database, keys := reimbursementDatabase(t)
	mark(t, database, keys.purchase, 9_000)
	answer := func(write func(context.Context, *authored.Recorder) (bool, error)) bool {
		return fact(t, database, "transactions.linkReimbursement", write)
	}
	linkTo := func(credit authored.TxKey, cents money.Cents) func(context.Context, *authored.Recorder) (bool, error) {
		return func(ctx context.Context, recorder *authored.Recorder) (bool, error) {
			return recorder.LinkReimbursement(ctx, keys.purchase, credit, cents)
		}
	}
	unlinkIt := func(ctx context.Context, recorder *authored.Recorder) (bool, error) {
		return recorder.UnlinkReimbursement(ctx, keys.purchase)
	}
	steps := []struct {
		label string
		write func(context.Context, *authored.Recorder) (bool, error)
		want  bool
	}{
		{"unlinking a purchase with no link", unlinkIt, false},
		{"linking a credit", linkTo(keys.credit, 15_000), true},
		{"linking it again", linkTo(keys.credit, 15_000), false},
		{"linking another", linkTo(keys.otherCredit, 20_000), true},
		{"unlinking", unlinkIt, true},
	}
	for _, step := range steps {
		if got := answer(step.write); got != step.want {
			t.Errorf("%s answered %v, want %v", step.label, got, step.want)
		}
	}
}

func TestCreditLinksNamesEachLinkedCreditWithItsPurchase(t *testing.T) {
	database, keys := reimbursementDatabase(t)
	mark(t, database, keys.purchase, 9_000)
	mark(t, database, keys.otherPurchase, 3_000)
	link(t, database, keys.purchase, keys.credit, 15_000)
	link(t, database, keys.otherPurchase, keys.otherCredit, 20_000)
	view := database.View()
	defer view.Close()
	links, err := view.CreditLinks(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := map[authored.TxKey]authored.TxKey{keys.credit: keys.purchase, keys.otherCredit: keys.otherPurchase}
	if !reflect.DeepEqual(links, want) {
		t.Errorf("the links are %v, want %v", links, want)
	}
	empty, err := authored.EmptyView().CreditLinks(t.Context())
	if err != nil || len(empty) != 0 {
		t.Errorf("the empty view answers %v, %v", empty, err)
	}
}
