package authored_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// heldOf is everything kept about the transaction at key, as a read sees it.
func heldOf(t *testing.T, database *authored.Database, key authored.TxKey) authored.AuthoredTransaction {
	t.Helper()
	view := database.View()
	defer view.Close()
	found, err := view.Transactions(t.Context(), []authored.TxKey{key})
	if err != nil {
		t.Fatal(err)
	}
	return found[key]
}

func partAt(position int64, line *authored.LineID, cents money.Cents) authored.SplitPart {
	return authored.SplitPart{Position: position, Line: line, Cents: cents}
}

func lineID(id authored.LineID) *authored.LineID { return &id }

func splitInChange(t *testing.T, database *authored.Database, key authored.TxKey, parts ...authored.SplitPart) authored.ChangeID {
	t.Helper()
	return changeBy(t, database, "transactions.setSplit", func(recorder *authored.Recorder) error {
		return recorder.SetSplit(context.Background(), key, parts)
	})
}

func effectUnitsOf(t *testing.T, database *authored.Database, id authored.ChangeID) []string {
	t.Helper()
	return columnValues(t, database.Pool(), `SELECT tbl || ' ' || key FROM change_effects WHERE change_id = `+id.String()+` ORDER BY seq`)
}

func TestASplitIsReplacedWholeAndAPartThatKeptItsLineAndAmountIsNotRewritten(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -18_000, "name:a"))
	fees := lineID(1)
	first := []authored.SplitPart{partAt(0, fees, 10_000), partAt(1, nil, 5_000), partAt(2, fees, 3_000)}
	splitInChange(t, database, key, first...)
	afterFirst := canonical(t, database)

	replaced := splitInChange(t, database, key, partAt(0, fees, 10_000), partAt(1, fees, 8_000))
	if got := heldOf(t, database, key).Split; !reflect.DeepEqual(got, []authored.SplitPart{partAt(0, fees, 10_000), partAt(1, fees, 8_000)}) {
		t.Errorf("the parts after the replacement are %+v", got)
	}
	if got := effectUnitsOf(t, database, replaced); len(got) != 2 {
		t.Errorf("the replacement recorded %v, want the removal of the third part and the change of the second alone", got)
	}
	undo(t, database, replaced)
	if got := canonical(t, database); got != afterFirst {
		t.Errorf("the undo of the replacement left\n%s\nwant\n%s", got, afterFirst)
	}
	if got := heldOf(t, database, key).Split; !reflect.DeepEqual(got, first) {
		t.Errorf("the undo brought back %+v, want %+v in their order", got, first)
	}
}

func TestFewerThanTwoPartsLeaveNoPartOfTheSplit(t *testing.T) {
	for label, parts := range map[string][]authored.SplitPart{
		"none":     nil,
		"empty":    {},
		"one part": {partAt(0, lineID(1), 18_000)},
	} {
		database := openDatabase(t, databasePath(t), creationInstant, nil)
		key := indexed(t, database, seenRow("p-1", "2026-11-28", -18_000, "name:a"))
		splitInChange(t, database, key, partAt(0, lineID(1), 10_000), partAt(1, nil, 8_000))
		splitInChange(t, database, key, parts...)
		if got := heldOf(t, database, key).Split; len(got) != 0 {
			t.Errorf("%s: the transaction keeps the parts %+v", label, got)
		}
	}
}

// fact runs one of the writes that a history step follows in its own change and
// answers whether it said it changed something.
func fact(t *testing.T, database *authored.Database, method string, write func(context.Context, *authored.Recorder) (bool, error)) bool {
	t.Helper()
	var changed bool
	changeBy(t, database, method, func(recorder *authored.Recorder) (err error) {
		changed, err = write(context.Background(), recorder)
		return err
	})
	return changed
}

func TestEachFactWriteSaysWhetherItChangedTheTransaction(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", 3_000, "name:a"))
	empty, reason := "", "Compra da empresa"
	exclude := func(exclusion *string) func(context.Context, *authored.Recorder) (bool, error) {
		return func(ctx context.Context, recorder *authored.Recorder) (bool, error) {
			return recorder.SetExclusion(ctx, key, exclusion)
		}
	}
	refund := func(on bool) func(context.Context, *authored.Recorder) (bool, error) {
		return func(ctx context.Context, recorder *authored.Recorder) (bool, error) {
			return recorder.SetRefund(ctx, key, on)
		}
	}
	refundLine := func(ctx context.Context, recorder *authored.Recorder) (bool, error) {
		return recorder.SetRefundLine(ctx, key, 1)
	}
	steps := []struct {
		label string
		write func(context.Context, *authored.Recorder) (bool, error)
		want  bool
	}{
		{"counting again a transaction that counts", exclude(nil), false},
		{"switching the exclusion on", exclude(&empty), true},
		{"switching it on again", exclude(&empty), false},
		{"giving it a reason", exclude(&reason), true},
		{"giving the same reason", exclude(&reason), false},
		{"counting it again", exclude(nil), true},
		{"marking a refund over the ledger's rule", refund(true), true},
		{"marking it again", refund(true), false},
		{"unmarking it", refund(false), true},
		{"choosing its line", refundLine, true},
		{"choosing the same line", refundLine, false},
	}
	for _, step := range steps {
		if got := fact(t, database, "transactions.test", step.write); got != step.want {
			t.Errorf("%s answered changed %v, want %v", step.label, got, step.want)
		}
	}
	state := heldOf(t, database, key).State
	if state.Exclusion != nil || state.Refund == nil || *state.Refund || state.RefundLine == nil || *state.RefundLine != 1 {
		t.Errorf("the transaction holds %+v", state)
	}
}
