package authored_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

func seenRow(providerID string, day brday.Day, cents money.Cents, establishment string) authored.SeenTransaction {
	return authored.SeenTransaction{ProviderID: providerID, AccountID: "acc-1", Day: day, Cents: cents, EstablishmentKey: establishment}
}

// indexed records a change that only indexes seen, and answers the key.
func indexed(t *testing.T, database *authored.Database, seen authored.SeenTransaction) authored.TxKey {
	t.Helper()
	var key authored.TxKey
	err := database.Change(t.Context(), facts("transactions.setNote"), func(recorder *authored.Recorder) error {
		var err error
		key, err = recorder.IndexTransaction(t.Context(), seen)
		return err
	})
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	return key
}

// theIndex is every row of both index tables, every column.
func theIndex(t *testing.T, database *authored.Database) string {
	t.Helper()
	rows := columnValues(t, database.Pool(), `
SELECT 'index ' || tx_key || ' ' || account_id || ' ' || day || ' ' || cents || ' ' || establishment_key || ' ' || bank_pending
  || ' ' || coalesce(first_run, '-') || ' ' || coalesce(last_run, '-') || ' ' || evaluated
FROM tx_index
UNION ALL
SELECT 'id ' || provider_id || ' ' || tx_key FROM tx_ids
ORDER BY 1`)
	return strings.Join(rows, "\n")
}

func journalRows(t *testing.T, database *authored.Database) int {
	t.Helper()
	var changes, effects int
	if err := database.Pool().QueryRow("SELECT (SELECT count(*) FROM changes), (SELECT count(*) FROM change_effects)").Scan(&changes, &effects); err != nil {
		t.Fatal(err)
	}
	return changes + effects
}

func TestIndexingAProviderIDNeverSeenMintsOneKeyAndRecordsNothing(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -4590, "name:padaria"))
	if key != 1 {
		t.Errorf("the first key is %d, want 1", key)
	}
	want := "id p-1 1\nindex 1 acc-1 2026-11-28 -4590 name:padaria 0 - - 0"
	if got := theIndex(t, database); got != want {
		t.Errorf("index:\n%s\nwant:\n%s", got, want)
	}
	if rows := journalRows(t, database); rows != 0 {
		t.Errorf("indexing wrote %d rows of the journal, want none", rows)
	}

	again := indexed(t, database, seenRow("p-1", "2026-11-28", -4590, "name:padaria"))
	if again != key || theIndex(t, database) != want {
		t.Errorf("indexing again answered %d and left:\n%s", again, theIndex(t, database))
	}
	var minted int
	if err := database.Pool().QueryRow("SELECT seq FROM sqlite_sequence WHERE name = 'tx_index'").Scan(&minted); err != nil || minted != 1 {
		t.Errorf("tx_index has minted %d keys (%v), want 1", minted, err)
	}
}

func TestIndexingAKnownProviderIDRefreshesItsFactsOnTheSameKey(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -4590, "name:padaria"))
	around(t, database, "UPDATE tx_index SET evaluated = 1, first_run = 3, last_run = 4 WHERE tx_key = ?", int64(key))
	change(t, database, func(recorder *authored.Recorder) error {
		return recorder.Set(context.Background(), "establishments", key1(text("name:padaria")), authored.Row{"nickname": text("Padaria")})
	})
	authoredBefore := dump(t, database)

	reworded := seenRow("p-1", "2026-11-29", -4600, "text:padaria do bairro")
	reworded.BankPending = true
	if got := indexed(t, database, reworded); got != key {
		t.Fatalf("the reworded row got key %d, want %d", got, key)
	}
	want := "id p-1 1\nindex 1 acc-1 2026-11-29 -4600 text:padaria do bairro 1 3 4 1"
	if got := theIndex(t, database); got != want {
		t.Errorf("index:\n%s\nwant:\n%s", got, want)
	}
	if got := dump(t, database); got != authoredBefore {
		t.Errorf("a refresh moved what was authored:\n%s\nwant:\n%s", got, authoredBefore)
	}
}

func key1(value authored.Value) []authored.Value { return []authored.Value{value} }

func TestAPostedRowTakesTheKeyOfThePendingRowItReplaces(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	pending := seenRow("p-pending", "2026-11-28", -4590, "name:padaria")
	pending.BankPending = true
	key := indexed(t, database, pending)

	posted := seenRow("p-posted", "2026-11-29", -4590, "name:padaria")
	posted.Replaces = key
	if got := indexed(t, database, posted); got != key {
		t.Fatalf("the posted row got key %d, want %d", got, key)
	}
	want := "id p-pending 1\nid p-posted 1\nindex 1 acc-1 2026-11-29 -4590 name:padaria 0 - - 0"
	if got := theIndex(t, database); got != want {
		t.Errorf("index:\n%s\nwant:\n%s", got, want)
	}
	for _, seen := range []authored.SeenTransaction{pending, posted} {
		if got := indexed(t, database, seen); got != key {
			t.Errorf("%s answers %d after the mapping, want %d", seen.ProviderID, got, key)
		}
	}
}

func TestAMappingThatWouldPutOneIDUnderTwoKeysIsRefusedAndChangesNothing(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	first := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	second := indexed(t, database, seenRow("p-2", "2026-11-28", -200, "name:b"))
	before := theIndex(t, database)

	elsewhere := seenRow("p-1", "2026-11-29", -100, "name:a")
	elsewhere.Replaces = second
	nowhere := seenRow("p-3", "2026-11-29", -300, "name:c")
	nowhere.Replaces = second + 10
	for name, c := range map[string]struct {
		seen authored.SeenTransaction
		want error
	}{
		"an id mapped to another key": {elsewhere, authored.ErrProviderIDElsewhere},
		"a key never given":           {nowhere, authored.ErrNoSuchTxKey},
	} {
		err := database.Change(t.Context(), facts("transactions.setNote"), func(recorder *authored.Recorder) error {
			_, err := recorder.IndexTransaction(t.Context(), c.seen)
			return err
		})
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	if got := theIndex(t, database); got != before {
		t.Errorf("a refused mapping changed the index:\n%s\nwant:\n%s", got, before)
	}
	if first == second {
		t.Error("two provider ids never mapped together share a key")
	}
}

func TestAChangeThatFailsAfterIndexingTakesTheIndexRowsBack(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	indexed(t, database, seenRow("p-0", "2026-11-27", -50, "name:a"))
	before := theIndex(t, database)
	refused := errors.New("the write refused")
	failures := map[string]func(*authored.Recorder, authored.TxKey) error{
		"the write's own refusal": func(*authored.Recorder, authored.TxKey) error { return refused },
		"a line no budget has, found at the commit": func(recorder *authored.Recorder, key authored.TxKey) error {
			return recorder.Set(context.Background(), "tx_state", key1(integer(int64(key))), authored.Row{"line_id": integer(99)})
		},
	}
	for name, fail := range failures {
		err := database.Change(t.Context(), facts("transactions.setLine"), func(recorder *authored.Recorder) error {
			key, err := recorder.IndexTransaction(t.Context(), seenRow("p-1", "2026-11-28", -100, "name:b"))
			if err != nil {
				return err
			}
			if _, err := recorder.IndexTransaction(t.Context(), seenRow("p-0", "2026-12-01", -60, "name:c")); err != nil {
				return err
			}
			return fail(recorder, key)
		})
		if err == nil {
			t.Fatalf("%s: the change committed", name)
		}
		if got := theIndex(t, database); got != before {
			t.Errorf("%s: the index kept what the failed change wrote:\n%s\nwant:\n%s", name, got, before)
		}
	}
}

func TestAnUndoLeavesTheIndexRowsInPlace(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	empty := dump(t, database)
	id := changeBy(t, database, "transactions.setLine", func(recorder *authored.Recorder) error {
		key, err := recorder.IndexTransaction(context.Background(), seenRow("p-1", "2026-11-28", -100, "name:b"))
		if err != nil {
			return err
		}
		return recorder.Set(context.Background(), "tx_state", key1(integer(int64(key))), authored.Row{
			"line_id": integer(1), "classified_by": text("person"),
		})
	})
	indexedOnly := changeBy(t, database, "transactions.setNote", func(recorder *authored.Recorder) error {
		_, err := recorder.IndexTransaction(context.Background(), seenRow("p-2", "2026-11-28", -200, "name:c"))
		return err
	})
	kept := theIndex(t, database)
	if !strings.Contains(kept, "id p-1 1") || !strings.Contains(kept, "id p-2 2") {
		t.Fatalf("the index before the undos:\n%s", kept)
	}

	undo(t, database, id)
	undo(t, database, indexedOnly)
	if got := theIndex(t, database); got != kept {
		t.Errorf("an undo changed the index:\n%s\nwant:\n%s", got, kept)
	}
	if got := dump(t, database); got != empty {
		t.Errorf("the undo left what the change authored:\n%s", got)
	}
}

func TestTheIndexIsWrittenOnlyThroughItsOwnStatements(t *testing.T) {
	writers := map[string]bool{}
	for name, statement := range authored.AllStatements() {
		for _, match := range writeTarget.FindAllStringSubmatch(statement, -1) {
			if match[1] == "tx_index" || match[1] == "tx_ids" {
				writers[name] = true
			}
		}
	}
	want := map[string]bool{"statementMintTxKey": true, "statementMapProviderID": true, "statementRefreshTxKey": true}
	if len(writers) != len(want) {
		t.Errorf("the statements that write the index are %v, want %v", writers, want)
	}
	for name := range want {
		if !writers[name] {
			t.Errorf("%s does not write the index", name)
		}
	}
}
