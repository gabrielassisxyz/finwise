package authored_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

const (
	potReserve    = "reserve:acc-pots/pot-1"
	potInvestment = "investment:inv-1"
)

var potsStart = time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC)

// potsDatabase is a created database with four lines minted besides the fees
// line, which are ids 2 to 5.
func potsDatabase(t *testing.T) *authored.Database {
	t.Helper()
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	for range 4 {
		mustExec(t, database.Pool(), `INSERT INTO lines (builtin) VALUES (0)`)
	}
	return database
}

func changePots(ctx context.Context, database *authored.Database, at time.Time, write func(*authored.Recorder) error) error {
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "pots.test", At: at}
	return database.Change(ctx, facts, write)
}

// recordPots makes one change and returns its id, which exists even when the
// write recorded nothing.
func recordPots(t *testing.T, database *authored.Database, at time.Time, write func(ctx context.Context, recorder *authored.Recorder) error) authored.ChangeID {
	t.Helper()
	var id authored.ChangeID
	err := changePots(t.Context(), database, at, func(recorder *authored.Recorder) error {
		var err error
		if id, err = recorder.ID(t.Context()); err != nil {
			return err
		}
		return write(t.Context(), recorder)
	})
	if err != nil {
		t.Fatalf("change: %v", err)
	}
	return id
}

func setBucket(pot string, bucket authored.PotBucket) func(context.Context, *authored.Recorder) error {
	return func(ctx context.Context, recorder *authored.Recorder) error {
		return recorder.SetPotBucket(ctx, pot, bucket)
	}
}

func clearBucket(pot string) func(context.Context, *authored.Recorder) error {
	return func(ctx context.Context, recorder *authored.Recorder) error {
		return recorder.ClearPotBucket(ctx, pot)
	}
}

func toggleLink(line authored.LineID, pot string) func(context.Context, *authored.Recorder) error {
	return func(ctx context.Context, recorder *authored.Recorder) error {
		return recorder.ToggleLink(ctx, line, pot)
	}
}

func undoAt(t *testing.T, database *authored.Database, id authored.ChangeID, minutes int) error {
	t.Helper()
	return database.Undo(t.Context(), id, potsStart.Add(time.Duration(minutes)*time.Minute))
}

func bucketOf(t *testing.T, view *authored.View, pot string) (authored.PotBucket, bool) {
	t.Helper()
	bucket, found, err := view.PotBucket(t.Context(), pot)
	if err != nil {
		t.Fatal(err)
	}
	return bucket, found
}

func potOfLine(t *testing.T, view *authored.View, line authored.LineID) (string, bool) {
	t.Helper()
	pot, found, err := view.PotOfLine(t.Context(), line)
	if err != nil {
		t.Fatal(err)
	}
	return pot, found
}

func linesOfPot(t *testing.T, view *authored.View, pot string) []authored.LineID {
	t.Helper()
	lines, err := view.LinesOfPot(t.Context(), pot)
	if err != nil {
		t.Fatal(err)
	}
	return lines
}

// nowView is a view made after every change so far, closed when the test ends.
func nowView(t *testing.T, database *authored.Database) *authored.View {
	t.Helper()
	view := database.View()
	t.Cleanup(func() { view.Close() })
	return view
}

func rowCount(t *testing.T, database *authored.Database, table string) int {
	t.Helper()
	var count int
	query := map[string]string{"pot_settings": `SELECT count(*) FROM pot_settings`, "pot_lines": `SELECT count(*) FROM pot_lines`}[table]
	if err := database.Pool().QueryRow(query).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func requireRefusal(t *testing.T, err error, cause authored.StaleCause) {
	t.Helper()
	var stale *authored.StaleChangeError
	if !errors.As(err, &stale) || stale.Cause != cause {
		t.Errorf("err = %v, want a refusal with cause %d", err, cause)
	}
}

func TestABucketIsReadBackAndUndoingItReadsNoneAndRestoresTheDump(t *testing.T) {
	database := potsDatabase(t)
	empty := canonicalOf(t, database)
	id := recordPots(t, database, potsStart, setBucket(potReserve, authored.BucketAvailable))

	if bucket, found := bucketOf(t, nowView(t, database), potReserve); !found || bucket != "Disponível" {
		t.Errorf("read %q (%v), want Disponível", bucket, found)
	}
	if _, found := bucketOf(t, nowView(t, database), potInvestment); found {
		t.Error("a pot nobody gave a bucket has one")
	}
	if canonicalOf(t, database) == empty {
		t.Fatal("the write left the dump as it was; the instrument is not seeing the table")
	}
	if err := undoAt(t, database, id, 1); err != nil {
		t.Fatal(err)
	}
	if bucket, found := bucketOf(t, nowView(t, database), potReserve); found {
		t.Errorf("after the undo the pot still has %q", bucket)
	}
	if got := canonicalOf(t, database); got != empty {
		t.Errorf("the dump after the undo differs from the one before:\n%s\n%s", got, empty)
	}
}

func TestEachOfTheThreeBucketsIsStoredAndReadBack(t *testing.T) {
	for _, bucket := range []authored.PotBucket{authored.BucketAvailable, authored.BucketReserved, authored.BucketInvested} {
		database := potsDatabase(t)
		recordPots(t, database, potsStart, setBucket(potInvestment, bucket))
		if got, found := bucketOf(t, nowView(t, database), potInvestment); !found || got != bucket {
			t.Errorf("stored %q, read %q (%v)", bucket, got, found)
		}
	}
}

func TestSettingTheBucketAlreadyStoredRecordsNoEffect(t *testing.T) {
	database := potsDatabase(t)
	recordPots(t, database, potsStart, setBucket(potReserve, authored.BucketReserved))
	effects, dump := effectCount(t, database), canonicalOf(t, database)

	recordPots(t, database, potsStart.Add(time.Hour), setBucket(potReserve, authored.BucketReserved))
	if got := effectCount(t, database); got != effects {
		t.Errorf("setting the stored bucket again took the effects from %d to %d", effects, got)
	}
	if canonicalOf(t, database) != dump {
		t.Error("setting the stored bucket again changed the dump")
	}

	recordPots(t, database, potsStart.Add(2*time.Hour), setBucket(potReserve, authored.BucketInvested))
	if got := effectCount(t, database); got != effects+1 {
		t.Errorf("setting another bucket took the effects from %d to %d, want one more; the count is not seeing the write", effects, got)
	}
}

func TestClearingABucketRemovesTheRowAndUndoingItBringsTheChoiceBack(t *testing.T) {
	database := potsDatabase(t)
	empty := canonicalOf(t, database)
	recordPots(t, database, potsStart, setBucket(potReserve, authored.BucketAvailable))
	withBucket := canonicalOf(t, database)

	cleared := recordPots(t, database, potsStart.Add(time.Hour), clearBucket(potReserve))
	if _, found := bucketOf(t, nowView(t, database), potReserve); found {
		t.Error("a cleared pot still has a bucket")
	}
	if rows := rowCount(t, database, "pot_settings"); rows != 0 {
		t.Errorf("%d rows after clearing, want none: a cleared pot is the same as one nobody chose for", rows)
	}
	if got := canonicalOf(t, database); got != empty {
		t.Errorf("the dump after clearing is not the empty one:\n%s", got)
	}
	if err := undoAt(t, database, cleared, 90); err != nil {
		t.Fatal(err)
	}
	if bucket, found := bucketOf(t, nowView(t, database), potReserve); !found || bucket != authored.BucketAvailable {
		t.Errorf("after undoing the clear: %q (%v), want Disponível", bucket, found)
	}
	if got := canonicalOf(t, database); got != withBucket {
		t.Error("undoing the clear did not restore the dump")
	}
}

func TestClearingAPotWithNoBucketRecordsNoEffect(t *testing.T) {
	database := potsDatabase(t)
	recordPots(t, database, potsStart, clearBucket(potReserve))
	if got := effectCount(t, database); got != 0 {
		t.Errorf("%d effects, want none", got)
	}
}

func TestUndoingASecondBucketAnswersTheFirstAndTheFirstCannotBeUndoneBeforeIt(t *testing.T) {
	database := potsDatabase(t)
	first := recordPots(t, database, potsStart, setBucket(potReserve, authored.BucketAvailable))
	afterFirst := canonicalOf(t, database)
	second := recordPots(t, database, potsStart.Add(time.Hour), setBucket(potReserve, authored.BucketInvested))

	requireRefusal(t, undoAt(t, database, first, 90), authored.CauseLaterChange)
	if bucket, _ := bucketOf(t, nowView(t, database), potReserve); bucket != authored.BucketInvested {
		t.Errorf("a refused undo left %q", bucket)
	}
	if err := undoAt(t, database, second, 100); err != nil {
		t.Fatal(err)
	}
	if bucket, found := bucketOf(t, nowView(t, database), potReserve); !found || bucket != authored.BucketAvailable {
		t.Errorf("after the undo: %q (%v), want the first bucket", bucket, found)
	}
	if canonicalOf(t, database) != afterFirst {
		t.Error("the dump after the undo is not the one after the first write")
	}
}

func TestABucketOutsideTheThreeAndAPotWithoutAnIdAreRefusedAndWriteNothing(t *testing.T) {
	cases := []struct {
		name, pot string
		bucket    authored.PotBucket
	}{
		{"the bucket Outro", potReserve, "Outro"},
		{"an empty bucket", potReserve, ""},
		{"a slot", "slot:acc-pots/1", authored.BucketAvailable},
		{"no pot", "", authored.BucketAvailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			database := potsDatabase(t)
			before := canonicalOf(t, database)
			err := changePots(t.Context(), database, potsStart, func(recorder *authored.Recorder) error {
				return recorder.SetPotBucket(t.Context(), c.pot, c.bucket)
			})
			var failed *authored.TransactionError
			if !errors.As(err, &failed) {
				t.Fatalf("err = %v, want the table's refusal as a failure of the change", err)
			}
			if effects := effectCount(t, database); effects != 0 || canonicalOf(t, database) != before {
				t.Errorf("a refused write left %d effects or changed the dump", effects)
			}
		})
	}
}

func TestTogglingALinkLinksItUnlinksItAndLinksItAgain(t *testing.T) {
	database := potsDatabase(t)
	empty := canonicalOf(t, database)

	linked := recordPots(t, database, potsStart, toggleLink(2, potReserve))
	view := nowView(t, database)
	if pot, found := potOfLine(t, view, 2); !found || pot != potReserve {
		t.Errorf("the line is on %q (%v), want %s", pot, found, potReserve)
	}
	if lines := linesOfPot(t, view, potReserve); !slices.Equal(lines, []authored.LineID{2}) {
		t.Errorf("the pot has the lines %v, want [2]", lines)
	}
	withLink := canonicalOf(t, database)
	if withLink == empty {
		t.Fatal("linking left the dump as it was; the instrument is not seeing the table")
	}

	unlinked := recordPots(t, database, potsStart.Add(time.Hour), toggleLink(2, potReserve))
	view = nowView(t, database)
	if pot, found := potOfLine(t, view, 2); found {
		t.Errorf("the unlinked line is still on %q", pot)
	}
	if lines := linesOfPot(t, view, potReserve); len(lines) != 0 {
		t.Errorf("the pot still has the lines %v", lines)
	}
	if got := canonicalOf(t, database); got != empty {
		t.Errorf("the dump after unlinking is not the empty one:\n%s", got)
	}

	if err := undoAt(t, database, unlinked, 90); err != nil {
		t.Fatal(err)
	}
	if got := canonicalOf(t, database); got != withLink {
		t.Error("undoing the unlink did not put the link back")
	}
	if err := undoAt(t, database, linked, 100); err != nil {
		t.Fatal(err)
	}
	if got := canonicalOf(t, database); got != empty {
		t.Error("undoing the link did not restore the dump")
	}
}

func TestTogglingALineOnAnotherPotMovesItInOneChangeAndUndoPutsItBack(t *testing.T) {
	database := potsDatabase(t)
	recordPots(t, database, potsStart, toggleLink(2, potReserve))
	beforeMove := canonicalOf(t, database)
	effectsBefore := effectCount(t, database)

	moved := recordPots(t, database, potsStart.Add(time.Hour), toggleLink(2, potInvestment))
	view := nowView(t, database)
	if lines := linesOfPot(t, view, potReserve); len(lines) != 0 {
		t.Errorf("the first pot keeps the lines %v after the move", lines)
	}
	if lines := linesOfPot(t, view, potInvestment); !slices.Equal(lines, []authored.LineID{2}) {
		t.Errorf("the second pot has the lines %v, want [2]", lines)
	}
	if pot, _ := potOfLine(t, view, 2); pot != potInvestment {
		t.Errorf("the line is on %q", pot)
	}
	if rows := rowCount(t, database, "pot_lines"); rows != 1 {
		t.Errorf("%d links after the move, want 1: a line is on one pot", rows)
	}
	if got := effectCount(t, database); got != effectsBefore+1 {
		t.Errorf("the move recorded %d effects, want 1", got-effectsBefore)
	}

	if err := undoAt(t, database, moved, 90); err != nil {
		t.Fatal(err)
	}
	view = nowView(t, database)
	if pot, found := potOfLine(t, view, 2); !found || pot != potReserve {
		t.Errorf("after the undo the line is on %q (%v), want %s", pot, found, potReserve)
	}
	if lines := linesOfPot(t, view, potInvestment); len(lines) != 0 {
		t.Errorf("after the undo the second pot keeps %v", lines)
	}
	if got := canonicalOf(t, database); got != beforeMove {
		t.Errorf("the dump after the undo is not the one before the move:\n%s\n%s", got, beforeMove)
	}
}

// A line is on the pot its latest change left it on, so an earlier change to it
// cannot be taken back first without putting it somewhere nobody chose.
func TestAnEarlierChangeToALineIsRefusedWhileALaterOneStands(t *testing.T) {
	database := potsDatabase(t)
	empty := canonicalOf(t, database)
	linked := recordPots(t, database, potsStart, toggleLink(2, potReserve))
	moved := recordPots(t, database, potsStart.Add(time.Hour), toggleLink(2, potInvestment))

	requireRefusal(t, undoAt(t, database, linked, 90), authored.CauseLaterChange)
	unlinked := recordPots(t, database, potsStart.Add(2*time.Hour), toggleLink(2, potInvestment))
	requireRefusal(t, undoAt(t, database, moved, 100), authored.CauseLaterChange)
	if rows := rowCount(t, database, "pot_lines"); rows != 0 {
		t.Errorf("refused undos left %d links, want the unlinked state", rows)
	}

	for step, id := range []authored.ChangeID{unlinked, moved, linked} {
		if err := undoAt(t, database, id, 110+step); err != nil {
			t.Fatalf("undo %d: %v", step, err)
		}
		pot, found := potOfLine(t, nowView(t, database), 2)
		want := []struct {
			pot   string
			found bool
		}{{potInvestment, true}, {potReserve, true}, {"", false}}[step]
		if pot != want.pot || found != want.found {
			t.Errorf("after undo %d the line is on %q (%v), want %q (%v)", step, pot, found, want.pot, want.found)
		}
	}
	if got := canonicalOf(t, database); got != empty {
		t.Error("undoing every change did not restore the empty dump")
	}
}

func TestEveryLinkOfAPotIsReadInTheSameOrderEveryTime(t *testing.T) {
	database := potsDatabase(t)
	for step, line := range []authored.LineID{5, 3, 4} {
		recordPots(t, database, potsStart.Add(time.Duration(step)*time.Minute), toggleLink(line, potReserve))
	}
	recordPots(t, database, potsStart.Add(time.Hour), toggleLink(2, potInvestment))

	want := []authored.LineID{3, 4, 5}
	for read := range 5 {
		view := database.View()
		if got := linesOfPot(t, view, potReserve); !slices.Equal(got, want) {
			t.Errorf("read %d answers %v, want %v whatever order they were linked in", read, got, want)
		}
		view.Close()
	}
	if got := linesOfPot(t, nowView(t, database), potInvestment); !slices.Equal(got, []authored.LineID{2}) {
		t.Errorf("the other pot has %v, want [2]", got)
	}
	if got := linesOfPot(t, nowView(t, database), "reserve:acc-pots/nobody"); len(got) != 0 {
		t.Errorf("a pot nobody linked has %v", got)
	}
}

func TestALinkToALineThatDoesNotExistOrToASlotIsRefusedAndWritesNothing(t *testing.T) {
	cases := []struct {
		name string
		line authored.LineID
		pot  string
	}{
		{"a line that is not in lines", 99, potReserve},
		{"a slot", 2, "slot:acc-pots/1"},
		{"no pot", 2, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			database := potsDatabase(t)
			before := canonicalOf(t, database)
			err := changePots(t.Context(), database, potsStart, func(recorder *authored.Recorder) error {
				return recorder.ToggleLink(t.Context(), c.line, c.pot)
			})
			var failed *authored.TransactionError
			if !errors.As(err, &failed) {
				t.Fatalf("err = %v, want the table's refusal as a failure of the change", err)
			}
			if effects := effectCount(t, database); effects != 0 || canonicalOf(t, database) != before {
				t.Errorf("a refused link left %d effects or changed the dump", effects)
			}
		})
	}
}

// The toggle reads on the change's own transaction, so a line linked earlier in
// the same change is seen as linked.
func TestTogglingTwiceInOneChangeSeesTheFirstToggle(t *testing.T) {
	cases := []struct {
		name       string
		second     string
		wantPot    string
		wantLinked bool
	}{
		{"the same pot unlinks", potReserve, "", false},
		{"another pot moves", potInvestment, potInvestment, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			database := potsDatabase(t)
			empty := canonicalOf(t, database)
			id := recordPots(t, database, potsStart, func(ctx context.Context, recorder *authored.Recorder) error {
				if err := recorder.ToggleLink(ctx, 2, potReserve); err != nil {
					return err
				}
				return recorder.ToggleLink(ctx, 2, c.second)
			})
			if pot, linked := potOfLine(t, nowView(t, database), 2); pot != c.wantPot || linked != c.wantLinked {
				t.Errorf("the line is on %q (%v), want %q (%v)", pot, linked, c.wantPot, c.wantLinked)
			}
			if err := undoAt(t, database, id, 1); err != nil {
				t.Fatal(err)
			}
			if got := canonicalOf(t, database); got != empty {
				t.Error("undoing the change did not restore the empty dump")
			}
		})
	}
}

// A closed pot, a pot gone from the ledger and a line the budget no longer
// holds are all things that happen elsewhere; none of them is the person's
// toggle, so none may take a link away.
func TestALinkStaysWhenItsLineLeavesTheBudget(t *testing.T) {
	database := potsDatabase(t)
	recordPots(t, database, potsStart, func(ctx context.Context, recorder *authored.Recorder) error {
		if err := recorder.SaveBudget(ctx, "2026-03", authored.Document{
			{Key: "prov", Lines: []authored.DocumentLine{{Name: "Remédios", Cents: 15_000}}},
		}); err != nil {
			return err
		}
		view := recorder.View()
		defer view.Close()
		budget, err := view.BudgetOf(ctx, "2026-03")
		if err != nil || len(budget.Lines) == 0 {
			return errors.Join(err, errors.New("the saved budget has no line"))
		}
		return recorder.ToggleLink(ctx, budget.Lines[0].ID, potReserve)
	})
	linked := linesOfPot(t, nowView(t, database), potReserve)
	if len(linked) != 1 {
		t.Fatalf("the pot has the lines %v, want the one", linked)
	}

	recordPots(t, database, potsStart.Add(time.Hour), func(ctx context.Context, recorder *authored.Recorder) error {
		return recorder.SaveBudget(ctx, "2026-04", authored.Document{
			{Key: "prov", Lines: []authored.DocumentLine{{Name: "Outra", Cents: 1_000}}},
		})
	})
	budget, err := nowView(t, database).BudgetOf(t.Context(), "2026-04")
	if err != nil || len(budget.Lines) != 1 || budget.Lines[0].ID == linked[0] {
		t.Fatalf("the linked line is still in the budget of 2026-04 (%v): the test would prove nothing", err)
	}
	if got := linesOfPot(t, nowView(t, database), potReserve); !slices.Equal(got, linked) {
		t.Errorf("after the line left the budget the pot has %v, want %v", got, linked)
	}
}

func TestTheEmptyViewAnswersNoBucketAndNoLink(t *testing.T) {
	for name, view := range map[string]*authored.View{"the empty view": authored.EmptyView(), "a created database": potsDatabase(t).View()} {
		t.Run(name, func(t *testing.T) {
			defer view.Close()
			if bucket, found := bucketOf(t, view, potReserve); found || bucket != "" {
				t.Errorf("a bucket %q (%v)", bucket, found)
			}
			if pot, found := potOfLine(t, view, 2); found || pot != "" {
				t.Errorf("a line on %q (%v)", pot, found)
			}
			if lines := linesOfPot(t, view, potReserve); len(lines) != 0 {
				t.Errorf("a pot with the lines %v", lines)
			}
		})
	}
}

func TestTheJournalRecordsALinkAsOneEffectOnTheLineWhateverTheOperation(t *testing.T) {
	database := potsDatabase(t)
	added := recordPots(t, database, potsStart, toggleLink(2, potReserve))
	moved := recordPots(t, database, potsStart.Add(time.Minute), toggleLink(2, potInvestment))
	removed := recordPots(t, database, potsStart.Add(2*time.Minute), toggleLink(2, potInvestment))

	got := map[authored.ChangeID][]string{}
	rows, err := database.Pool().Query(`SELECT change_id, tbl || ' ' || key || ' ' || coalesce(col, '-') || ' ' || coalesce(before, '-') || ' -> ' || coalesce(after, '-') FROM change_effects ORDER BY change_id, seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id authored.ChangeID
		var effect string
		if err := rows.Scan(&id, &effect); err != nil {
			t.Fatal(err)
		}
		got[id] = append(got[id], effect)
	}
	want := map[authored.ChangeID][]string{
		added:   {`pot_lines [2] - - -> {"line_id":2,"pot_id":"reserve:acc-pots/pot-1"}`},
		moved:   {`pot_lines [2] pot_id "reserve:acc-pots/pot-1" -> "investment:inv-1"`},
		removed: {`pot_lines [2] - {"line_id":2,"pot_id":"investment:inv-1"} -> -`},
	}
	for id, effects := range want {
		if !slices.Equal(got[id], effects) {
			t.Errorf("change %d recorded %q, want %q", id, got[id], effects)
		}
	}
}
