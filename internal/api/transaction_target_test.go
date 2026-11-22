package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// inChange runs write in one change of the person's over the fixture ledger,
// as the router runs a write handler.
func inChange(t *testing.T, scratch *authoredtest.Scratch, fixture *ledgertest.Fixture, write func(Writes) error) error {
	t.Helper()
	snapshot := viewOf(t, fixture.Path)
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "transactions.setNote", At: fixtureNow}
	return scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		view := recorder.View()
		defer view.Close()
		return write(Writes{Ledger: snapshot, Authored: view, Recorder: recorder})
	})
}

// linesAdded is, per table, the rows of after that before lacks, in the format
// of authoredtest's Everything.
func linesAdded(before, after string) map[string][]string {
	held := map[string]bool{}
	table := ""
	for _, line := range strings.Split(before, "\n") {
		if name, found := strings.CutPrefix(line, "== "); found {
			table = name
			continue
		}
		held[table+"|"+line] = true
	}
	added := map[string][]string{}
	for _, line := range strings.Split(after, "\n") {
		if name, found := strings.CutPrefix(line, "== "); found {
			table = name
			continue
		}
		if line != "" && !held[table+"|"+line] {
			added[table] = append(added[table], line)
		}
	}
	return added
}

func TestTheLookupMintsAKeyAtFirstSightWithTheRowsFactsAndJournalsNothing(t *testing.T) {
	cases := []struct {
		id         string
		day        string
		amount     money.Cents
		ownCents   money.Cents
		name       string
		person     bool
		movement   txrules.MovementKind
		indexedRow string
	}{
		// A purchase of February whose tax was posted in March: the amount keeps
		// the tax, the index keeps the row's own.
		{txMonthEnd, "2026-02-28", -20_700, -20_000, "MERCADO EXEMPLO", false, txrules.NotAMovement,
			`account_id="card-itau" day="2026-02-28" cents=-20000 establishment_key="text:MERCADO EXEMPLO" bank_pending=0`},
		{txLastOfFebruary, "2026-02-28", -4_200, -4_200, "PADARIA EXEMPLO", false, txrules.NotAMovement,
			`account_id="acc-itau" day="2026-02-28" cents=-4200 establishment_key="text:PADARIA EXEMPLO" bank_pending=0`},
		{txStale, "2026-01-05", 5_000, 5_000, "Amigo Exemplo", true, txrules.NotAMovement,
			`account_id="acc-stale" day="2026-01-05" cents=5000 establishment_key="doc:44455566677" bank_pending=0`},
		{txOwnTransfer, "2026-03-08", -30_000, -30_000, "Pessoa Exemplo", true, txrules.OwnTransfer,
			`account_id="acc-itau" day="2026-03-08" cents=-30000 establishment_key="doc:98765432100" bank_pending=0`},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			scratch := authoredtest.Open(t, fixtureClock)
			before := scratch.Everything(t)
			var target transactionTarget
			err := inChange(t, scratch, newFullLedger(t), func(writes Writes) error {
				var found bool
				var err error
				if target, found, err = lookupTarget(t.Context(), writes, c.id); err == nil && !found {
					err = errors.New("not found")
				}
				return err
			})
			if err != nil {
				t.Fatalf("the lookup: %v", err)
			}
			listed := target.listed
			if target.key != 1 || listed.ID != c.id || string(listed.Day) != c.day || listed.Amount != c.amount ||
				listed.Amount-listed.Tax != c.ownCents || listed.Name != c.name || listed.Person != c.person || listed.Movement != c.movement {
				t.Errorf("the target is key %d %+v", target.key, listed)
			}
			added := linesAdded(before, scratch.Everything(t))
			want := map[string][]string{"tx_index": {"tx_key=1 " + c.indexedRow + " first_run=NULL last_run=NULL evaluated=0"}, "tx_ids": {`provider_id="` + c.id + `" tx_key=1`}}
			if len(added) != 2 || !slices.Equal(added["tx_index"], want["tx_index"]) || !slices.Equal(added["tx_ids"], want["tx_ids"]) {
				t.Errorf("the lookup added %v, want one index row and one id, and nothing in changes or change_effects:\n%v", added, want)
			}
		})
	}
}

func TestTheLookupOfAnIndexedTransactionRefreshesItsRowAndMintsNothing(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	full := newFullLedger(t)
	var stale authored.TxKey
	err := inChange(t, scratch, full, func(writes Writes) (err error) {
		stale, err = writes.Recorder.IndexTransaction(t.Context(), authoredtest.Seen(txPending, "2026-03-01", -1, "text:OUTRO NOME"))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	before := scratch.Everything(t)
	var target transactionTarget
	err = inChange(t, scratch, full, func(writes Writes) (err error) {
		target, _, err = lookupTarget(t.Context(), writes, txPending)
		return err
	})
	if err != nil || target.key != stale {
		t.Fatalf("the lookup answered key %d (%v), want the one already given, %d", target.key, err, stale)
	}
	added := linesAdded(before, scratch.Everything(t))
	refreshed := `tx_key=1 account_id="card-itau" day="2026-03-19" cents=-1850 establishment_key="text:PADARIA EXEMPLO" bank_pending=1 first_run=NULL last_run=NULL evaluated=0`
	if len(added) != 1 || !slices.Equal(added["tx_index"], []string{refreshed}) {
		t.Errorf("the lookup changed %v, want only the index row refreshed to %s", added, refreshed)
	}
	if strings.Count(scratch.Everything(t), "tx_key=") != 2 {
		t.Error("the index holds more than one row and one id")
	}
}

func TestTheLookupFindsNothingAndWritesNothingForAnIDTheListDoesNotShow(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	full := newFullLedger(t)
	before := scratch.Everything(t)
	for _, id := range []string{"no-such-transaction", txForeignTax, txPotTransfer} {
		err := inChange(t, scratch, full, func(writes Writes) error {
			_, found, err := lookupTarget(t.Context(), writes, id)
			if err == nil && found {
				err = errors.New("found")
			}
			return err
		})
		if err != nil {
			t.Errorf("%s: %v, want unknown", id, err)
		}
	}
	if after := scratch.Everything(t); after != before {
		t.Errorf("looking up what the list does not show wrote %v", linesAdded(before, after))
	}
}

// A method of the manifest that takes two strings and has no handler of its own,
// which these tests lend a handler of theirs over the real registry.
const (
	unhandledMethod = "settings.setNickname"
	unhandledRoute  = "/api/settings/setNickname"
)

// lookingUpThenFailing is a write handler that finds the transaction its first
// argument names, and then fails as a write can after its lookup.
func lookingUpThenFailing(looked *bool) WriteHandler {
	return func(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
		id, _, err := twoStrings(args)
		if err != nil {
			return nil, err
		}
		if _, found, err := lookupTarget(ctx, writes, id); err != nil || !found {
			return nil, err
		}
		*looked = true
		return nil, errors.New("the write fails after its lookup")
	}
}

func TestAWriteThatFailsAfterItsLookupLeavesTheIndexAsItWas(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	var looked bool
	env := newTestEnv(t, withLedger(newFullLedger(t).Path, fixtureNow), withDatabase(scratch.Database),
		withWrite(unhandledMethod, lookingUpThenFailing(&looked)))
	before := scratch.Everything(t)
	response := env.call(unhandledRoute, `["tx-month-end","nota"]`, env.unlock())
	if !looked || response.Code != http.StatusInternalServerError {
		t.Fatalf("the write looked up its target %v and answered %d, want true and 500", looked, response.Code)
	}
	if after := scratch.Everything(t); after != before {
		t.Errorf("the failed write kept %v", linesAdded(before, after))
	}
}

func TestOverASnapshotThatCannotBeReadTheLookupAndTheReadsAnswerLedgerUnavailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pluggy-snapshot.db")
	if err := os.WriteFile(path, []byte("not a SQLite database, only text long enough to fill a header"), 0o600); err != nil {
		t.Fatal(err)
	}
	scratch := authoredtest.Open(t, fixtureClock)
	inventedAuthoredState(t, scratch)
	var looked bool
	env := newTestEnv(t, withLedger(path, fixtureNow), withDatabase(scratch.Database),
		withWrite(unhandledMethod, lookingUpThenFailing(&looked)))
	cookie := env.unlock()
	before := scratch.Everything(t)
	expectError(t, "the lookup", env.call(unhandledRoute, `["tx-month-end","nota"]`, cookie), http.StatusServiceUnavailable, "ledger_unavailable")
	if after := scratch.Everything(t); after != before || looked {
		t.Errorf("a lookup over an unreadable snapshot found its target %v or wrote %v", looked, linesAdded(before, after))
	}
	for path, arguments := range map[string]string{
		"/api/transactions/list":          listRaw("2026-03", "month"),
		"/api/transactions/detail":        `["tx-month-end"]`,
		"/api/transactions/filterOptions": `["2026-03"]`,
	} {
		expectError(t, path, env.call(path, arguments, cookie), http.StatusServiceUnavailable, "ledger_unavailable")
	}
}

func TestTheWholeLedgerIsListedInOnePassAndWhatAWriteTouchesIsIndexedUnderTheKeyTheLookupFinds(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	full := newFullLedger(t)
	keys := map[string]authored.TxKey{}
	err := inChange(t, scratch, full, func(writes Writes) error {
		view, _, err := listedLedger(t.Context(), writes.Ledger)
		if err != nil {
			return err
		}
		for _, listed := range view.listed {
			if listed.ID == txMonthEnd || listed.ID == txPending {
				if keys[listed.ID], err = indexListed(t.Context(), writes.Recorder, listed); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil || len(keys) != 2 || keys[txMonthEnd] == keys[txPending] {
		t.Fatalf("indexed %v (%v), want two transactions of two months under two keys", keys, err)
	}
	for id, key := range keys {
		var target transactionTarget
		err := inChange(t, scratch, full, func(writes Writes) (err error) {
			target, _, err = lookupTarget(t.Context(), writes, id)
			return err
		})
		if err != nil || target.key != key {
			t.Errorf("%s: the lookup answered %d (%v), want the key the whole-ledger write gave, %d", id, target.key, err, key)
		}
	}
}
