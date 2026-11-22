package authored_test

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

func TestTheUnitsOfATransactionAreItsFactsTagsPartsAndStepsAndNothingElse(t *testing.T) {
	selected := authored.TransactionUnits(5, []authored.StepID{12})
	cases := []struct {
		unit authored.Unit
		want bool
	}{
		{authored.Unit{Table: "tx_state", Key: authored.EncodeKey(integer(5)), Column: "line_id"}, true},
		{authored.Unit{Table: "tx_tags", Key: authored.EncodeKey(integer(5), text("#a"))}, true},
		{authored.Unit{Table: "tx_split_parts", Key: authored.EncodeKey(integer(5), integer(0))}, true},
		{authored.Unit{Table: "history_steps", Key: authored.EncodeKey(integer(12))}, true},
		{authored.Unit{Table: "history_step_reasons", Key: authored.EncodeKey(integer(12))}, true},
		{authored.Unit{Table: "tx_state", Key: authored.EncodeKey(integer(6)), Column: "line_id"}, false},
		{authored.Unit{Table: "tx_tags", Key: authored.EncodeKey(integer(6), text("#a"))}, false},
		{authored.Unit{Table: "tx_split_parts", Key: authored.EncodeKey(integer(50), integer(5))}, false},
		{authored.Unit{Table: "history_steps", Key: authored.EncodeKey(integer(5))}, false},
		{authored.Unit{Table: "history_step_reasons", Key: authored.EncodeKey(integer(13))}, false},
		{authored.Unit{Table: "establishments", Key: authored.EncodeKey(text("name:a")), Column: "nickname"}, false},
		{authored.Unit{Table: "establishment_tags", Key: authored.EncodeKey(text("name:a"), text("#a"))}, false},
		{authored.Unit{Table: "budget_versions", Key: authored.EncodeKey(integer(5))}, false},
	}
	for _, c := range cases {
		if got := selected(c.unit); got != c.want {
			t.Errorf("%s %s %s picked %v, want %v", c.unit.Table, c.unit.Key, c.unit.Column, got, c.want)
		}
	}
}

// twoTransactionsInOneChange records one change that puts the first
// transaction on a line, with a tag and a step, and gives the second a note
// and a step; it answers the change and the two steps.
func twoTransactionsInOneChange(t *testing.T, database *authored.Database, first, second authored.TxKey) (authored.ChangeID, authored.StepID, authored.StepID) {
	t.Helper()
	var firstStep, secondStep authored.StepID
	id := changeBy(t, database, "transactions.setLineForMany", func(recorder *authored.Recorder) error {
		ctx := context.Background()
		if err := recorder.Set(ctx, "tx_state", key1(integer(int64(first))), authored.Row{"line_id": integer(1), "classified_by": text("person")}); err != nil {
			return err
		}
		if err := recorder.Put(ctx, "tx_tags", authored.Row{"tx_key": integer(int64(first)), "tag": text("#a"), "position": integer(0)}); err != nil {
			return err
		}
		if err := recorder.Set(ctx, "tx_state", key1(integer(int64(second))), authored.Row{"note": text("nota")}); err != nil {
			return err
		}
		var err error
		if firstStep, err = recorder.AddHistoryStep(ctx, first, authored.StepEffect, "Você colocou em Tarifas e juros"); err != nil {
			return err
		}
		secondStep, err = recorder.AddHistoryStep(ctx, second, authored.StepEffect, "Você escreveu uma nota")
		return err
	})
	return id, firstStep, secondStep
}

func canRestoreOver(t *testing.T, view *authored.View, id authored.ChangeID, selected func(authored.Unit) bool) bool {
	t.Helper()
	can, err := view.CanRestorePart(t.Context(), id, selected)
	if err != nil {
		t.Fatalf("CanRestorePart: %v", err)
	}
	return can
}

func TestAViewTellsAStepOverwrittenSinceFromOneWhoseTransactionNobodyTouchedAgain(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	first := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	second := indexed(t, database, seenRow("p-2", "2026-11-28", -200, "name:b"))
	id, firstStep, secondStep := twoTransactionsInOneChange(t, database, first, second)
	before := database.View()
	defer before.Close()
	if !canRestoreOver(t, before, id, authored.TransactionUnits(first, []authored.StepID{firstStep})) {
		t.Fatal("the control failed: nothing touched the first transaction since, and its step cannot be taken back")
	}
	changeBy(t, database, "transactions.setLine", func(recorder *authored.Recorder) error {
		return recorder.Set(context.Background(), "tx_state", key1(integer(int64(first))), authored.Row{"classified_by": text("rule")})
	})

	now := database.View()
	defer now.Close()
	if canRestoreOver(t, now, id, authored.TransactionUnits(first, []authored.StepID{firstStep})) {
		t.Error("the first transaction's step can be taken back after a later change touched what it did")
	}
	if !canRestoreOver(t, now, id, authored.TransactionUnits(second, []authored.StepID{secondStep})) {
		t.Error("the second transaction's step cannot be taken back, though nothing touched that transaction since")
	}
	if canRestoreOver(t, now, id, func(authored.Unit) bool { return true }) {
		t.Error("the whole change can be taken back after a later change touched part of it")
	}
	if !canRestoreOver(t, before, id, authored.TransactionUnits(first, []authored.StepID{firstStep})) {
		t.Error("a view that began before the later change answered from a state it did not begin in")
	}
}

func TestTheEmptyViewCanRestoreNothing(t *testing.T) {
	view := authored.EmptyView()
	defer view.Close()
	can, err := view.CanRestorePart(t.Context(), 1, func(authored.Unit) bool { return true })
	if err != nil || can {
		t.Errorf("the empty view answered %v, %v; want false and no error", can, err)
	}
}
