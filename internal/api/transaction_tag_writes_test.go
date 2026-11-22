package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// tagAndNoteWriteCases are the four writes of a transaction's own words, each
// recorded and undone, and the refusals a target that is not listed produces.
// The purchase at the market of the full ledger is alone in its establishment;
// the pending purchase at the bakery shares its with another row.
var tagAndNoteWriteCases = []writeCase{
	{method: "transactions.addTag", arguments: `["tx-month-end","  viagem de trabalho "]`, expects: expectsRecorded},
	{method: "transactions.removeTag", arguments: `["tx-month-end","#viagem-de-trabalho"]`, expects: expectsRecorded,
		before: []writeCall{{"transactions.addTag", `["tx-month-end","viagem de trabalho"]`}}},
	{method: "transactions.addTagToMany", arguments: `[["tx-month-end","tx-pending","tx-month-end"],"viagem"]`, expects: expectsRecorded},
	{method: "transactions.setNote", arguments: `["tx-month-end","abastecer na sexta"]`, expects: expectsRecorded},
	{method: "transactions.setNote", arguments: `["tx-month-end",""]`, expects: expectsRecorded,
		before: []writeCall{{"transactions.setNote", `["tx-month-end","abastecer na sexta"]`}}},
	{method: "transactions.addTag", arguments: `["no-such-transaction","viagem"]`, expects: expectsRefusal, reason: reasonNotFound},
	{method: "transactions.removeTag", arguments: `["tx-foreign-tax","#viagem"]`, expects: expectsRefusal, reason: reasonNotFound},
	{method: "transactions.setNote", arguments: `["tx-pot-transfer","abastecer na sexta"]`, expects: expectsRefusal, reason: reasonNotFound},
	{method: "transactions.addTagToMany", arguments: `[["tx-month-end","no-such-transaction"],"viagem"]`, expects: expectsRefusal, reason: reasonNotFound},
}

// argumentsOf is the JSON array a client posts for these values.
func argumentsOf(values ...any) string {
	encoded, err := json.Marshal(values)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// newAuthoringEnv is a router over the full invented ledger and an empty app's
// database, at the fixture's instant.
func newAuthoringEnv(t *testing.T) *settingsEnv {
	t.Helper()
	return newSettingsEnv(t, newFullLedger(t), fixtureNow)
}

// tableRows are the rows of one table of the file, in the format of Everything.
func tableRows(t *testing.T, scratch *authoredtest.Scratch, table string) []string {
	t.Helper()
	var rows []string
	inside := false
	for _, line := range strings.Split(scratch.Everything(t), "\n") {
		if name, found := strings.CutPrefix(line, "== "); found {
			inside = name == table
			continue
		}
		if inside && line != "" {
			rows = append(rows, line)
		}
	}
	return rows
}

func ownTagsOf(t *testing.T, env *settingsEnv, id string) []string {
	t.Helper()
	return texts(t, detailOver(t, env.readEnv, id)["ownTags"])
}

func fixedTagsOf(t *testing.T, env *settingsEnv, id string) []string {
	t.Helper()
	return texts(t, objectAt(t, detailOver(t, env.readEnv, id), "counterparty")["fixedTags"])
}

func TestEveryWayATagEntersSpellsItTheSameAndStoresItOnce(t *testing.T) {
	rows := []struct {
		typed string
		kept  []string
	}{
		{" viagem de trabalho ", []string{"#viagem-de-trabalho"}},
		{"#viagem-de-trabalho", []string{"#viagem-de-trabalho"}},
		{"#", []string{"#viagem-de-trabalho"}},
		{"   ", []string{"#viagem-de-trabalho"}},
		{"##a", []string{"#viagem-de-trabalho", "#a"}},
		{"# a b", []string{"#viagem-de-trabalho", "#a", "#a-b"}},
		{"a  b", []string{"#viagem-de-trabalho", "#a", "#a-b"}},
	}
	ways := map[string]struct {
		add  func(env *settingsEnv, typed string)
		read func(env *settingsEnv) []string
	}{
		"addTag": {
			add: func(env *settingsEnv, typed string) {
				env.change("transactions.addTag", argumentsOf(txMonthEnd, typed))
			},
			read: func(env *settingsEnv) []string { return ownTagsOf(t, env, txMonthEnd) },
		},
		"addTagToMany": {
			add: func(env *settingsEnv, typed string) {
				env.change("transactions.addTagToMany", argumentsOf([]string{txMonthEnd}, typed))
			},
			read: func(env *settingsEnv) []string { return ownTagsOf(t, env, txMonthEnd) },
		},
		"addFixedTag": {
			add: func(env *settingsEnv, typed string) {
				env.change("transactions.addFixedTag", argumentsOf(txMonthEnd, typed))
			},
			read: func(env *settingsEnv) []string { return fixedTagsOf(t, env, txMonthEnd) },
		},
	}
	for name, way := range ways {
		env := newAuthoringEnv(t)
		for _, row := range rows {
			way.add(env, row.typed)
			if got := way.read(env); !slices.Equal(got, row.kept) {
				t.Errorf("%s of %q: the tags are %v, want %v", name, row.typed, got, row.kept)
			}
		}
	}
}

func TestATagKeepsItsPlaceAcrossARemovalAndItsUndo(t *testing.T) {
	env := newAuthoringEnv(t)
	for _, tag := range []string{"#a", "#b", "#c"} {
		env.change("transactions.addTag", argumentsOf(txPending, tag))
	}
	removal := env.change("transactions.removeTag", argumentsOf(txPending, "#b"))
	if got, want := ownTagsOf(t, env, txPending), []string{"#a", "#c"}; !slices.Equal(got, want) {
		t.Errorf("after the removal: %v, want %v", got, want)
	}
	env.change("transactions.addTag", argumentsOf(txPending, "#d"))
	env.undo(removal)
	if got, want := ownTagsOf(t, env, txPending), []string{"#a", "#b", "#c", "#d"}; !slices.Equal(got, want) {
		t.Errorf("after the undo of the removal: %v, want every tag where it was, %v", got, want)
	}
}

func TestOwnTagsComeBeforeTheEstablishmentsFixedOnesInTheListAndInTheDetail(t *testing.T) {
	env := newAuthoringEnv(t)
	for _, tag := range []string{"#b", "#a"} {
		env.change("transactions.addTag", argumentsOf(txPending, tag))
	}
	for _, tag := range []string{"#a", "#c"} {
		env.change("transactions.addFixedTag", argumentsOf(txPending, tag))
	}
	want := []string{"#b", "#a", "#c"}
	detail := detailOver(t, env.readEnv, txPending)
	if got := texts(t, detail["tags"]); !slices.Equal(got, want) {
		t.Errorf("the detail's tags are %v, want %v", got, want)
	}
	if got := texts(t, detail["ownTags"]); !slices.Equal(got, []string{"#b", "#a"}) {
		t.Errorf("ownTags are %v, want [#b #a]", got)
	}
	rows := transactionsOf(t, env.readEnv, "2026-03", "last-3-months")
	listed := rowWithIDIn(t, rows, txPending)
	if got := texts(t, listed["tags"]); !slices.Equal(got, want) {
		t.Errorf("the list's tags are %v, want %v", got, want)
	}
	sibling := rowWithIDIn(t, rows, txLastOfFebruary)
	if got := texts(t, sibling["tags"]); !slices.Equal(got, []string{"#a", "#c"}) {
		t.Errorf("the bakery's other purchase carries %v, want the fixed tags alone", got)
	}
}

func TestTheKnownTagsAreEveryTagInUseOnceAndTheSuggestedOnesLackTheTransactionsOwn(t *testing.T) {
	env := newAuthoringEnv(t)
	for _, tag := range []string{"#Zebra", "#água", "#casa"} {
		env.change("transactions.addTag", argumentsOf(txPending, tag))
	}
	env.change("transactions.addFixedTag", argumentsOf(txMonthEnd, "#Casa"))
	env.change("transactions.addTag", argumentsOf(txMonthEnd, "#casa"))
	known := []string{"#água", "#Casa", "#casa", "#Zebra"}
	status, body := env.answer("transactions.filterOptions", `["2026-03"]`)
	if got := texts(t, decodeAnswer(t, body)["tags"]); status != http.StatusOK || !slices.Equal(got, known) {
		t.Errorf("the known tags are %v, want every tag in use once, %v", got, known)
	}
	if got, want := texts(t, detailOver(t, env.readEnv, txMonthEnd)["suggestedTags"]), []string{"#água", "#Casa", "#Zebra"}; !slices.Equal(got, want) {
		t.Errorf("suggested for the market's purchase: %v, want the known tags it lacks, %v", got, want)
	}
	env.change("transactions.removeTag", argumentsOf(txPending, "#Zebra"))
	_, body = env.answer("transactions.filterOptions", `["2026-03"]`)
	if got := texts(t, decodeAnswer(t, body)["tags"]); slices.Contains(got, "#Zebra") {
		t.Errorf("a tag whose last use was removed is still known: %v", got)
	}
}

func TestABulkTagTagsEachTransactionOnceAndItsUndoRemovesOnlyWhatItAdded(t *testing.T) {
	env := newAuthoringEnv(t)
	env.change("transactions.addTag", argumentsOf(txMonthEnd, "#viagem"))
	before := env.scratch.Canonical(t)
	id := env.change("transactions.addTagToMany", argumentsOf([]string{txMonthEnd, txPending, txMonthEnd, txPending}, "viagem"))
	for _, tx := range []string{txMonthEnd, txPending} {
		if got := ownTagsOf(t, env, tx); !slices.Equal(got, []string{"#viagem"}) {
			t.Errorf("%s holds %v, want #viagem once", tx, got)
		}
	}
	env.undo(id)
	if got := ownTagsOf(t, env, txMonthEnd); !slices.Equal(got, []string{"#viagem"}) {
		t.Errorf("the undo took the tag off the purchase that already had it: %v", got)
	}
	if got := ownTagsOf(t, env, txPending); len(got) != 0 {
		t.Errorf("the undo left %v on the purchase it was added to", got)
	}
	if after := env.scratch.Canonical(t); after != before {
		t.Errorf("the undo did not restore the tables:\n%s\nwas\n%s", after, before)
	}
}

func TestABulkTagWithAnUnknownIDRefusesTheWholeCallAndTouchesNothing(t *testing.T) {
	env := newAuthoringEnv(t)
	before := env.scratch.Everything(t)
	response := env.call("/api/transactions/addTagToMany", argumentsOf([]string{txMonthEnd, "no-such-transaction"}, "viagem"), env.cookie)
	expectRefused(t, "a selection with an unknown transaction", response, "not_found")
	if after := env.scratch.Everything(t); after != before {
		t.Errorf("a refused selection changed the file: %v", linesAdded(before, after))
	}
}

// The router rolls a refused write back, so a handler that wrote before it
// refused would still leave nothing there. The handler is called here under a
// caller that keeps the change whatever it answered, which is what makes
// finding every id first a property of the handler and not of the router.
func TestABulkWriteLeavesNothingEvenWhenItsCallerKeepsTheChange(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	full := newFullLedger(t)
	before := scratch.Everything(t)
	var refusal error
	err := inChange(t, scratch, full, func(writes Writes) error {
		_, refusal = addTagToTransactions(t.Context(), rawArguments(t, []string{txMonthEnd, "no-such-transaction"}, "viagem"), writes)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if kind, reason := classify(refusal); kind != kindRefused || reason != reasonNotFound {
		t.Fatalf("the handler answered %v, want the refusal not_found", refusal)
	}
	if after := scratch.Everything(t); after != before {
		t.Errorf("the handler wrote %v before it refused", linesAdded(before, after))
	}
}

func rawArguments(t *testing.T, values ...any) []json.RawMessage {
	t.Helper()
	raw := make([]json.RawMessage, len(values))
	for i, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		raw[i] = encoded
	}
	return raw
}

func TestANoteIsStoredAsWrittenAndAnEmptyOneLeavesNoRowForATransactionWithNothingElse(t *testing.T) {
	env := newAuthoringEnv(t)
	env.change("transactions.setNote", argumentsOf(txMonthEnd, " abastecer na sexta "))
	if got := detailOver(t, env.readEnv, txMonthEnd)["note"]; got != " abastecer na sexta " {
		t.Errorf("the note is %q, want it as written", got)
	}
	if rows := tableRows(t, env.scratch, "tx_state"); len(rows) != 1 {
		t.Errorf("tx_state holds %v, want the one row of the note", rows)
	}
	env.change("transactions.setNote", argumentsOf(txMonthEnd, ""))
	if rows := tableRows(t, env.scratch, "tx_state"); len(rows) != 0 {
		t.Errorf("an emptied note left %v", rows)
	}
	if got := detailOver(t, env.readEnv, txMonthEnd)["note"]; got != "" {
		t.Errorf("the note is %q after it was emptied", got)
	}
}

// singleTransactionWrites are the seven methods that name one transaction, with
// arguments that are well formed whatever the id is.
var singleTransactionWrites = map[string]string{
	"transactions.addTag":         `["%s","viagem"]`,
	"transactions.removeTag":      `["%s","#viagem"]`,
	"transactions.setNote":        `["%s","abastecer"]`,
	"transactions.setNickname":    `["%s","Pão"]`,
	"transactions.setDefaultLine": `["%s","Remédios"]`,
	"transactions.addFixedTag":    `["%s","viagem"]`,
	"transactions.removeFixedTag": `["%s","#viagem"]`,
}

func TestEverySingleTransactionWriteRefusesWhatTheListDoesNotShowAndChangesNothing(t *testing.T) {
	env := newAuthoringEnv(t)
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	before := env.scratch.Everything(t)
	for method, template := range singleTransactionWrites {
		for _, id := range []string{"no-such-transaction", txForeignTax, txPotTransfer} {
			domain, name := splitMethod(t, method)
			arguments := strings.Replace(template, "%s", id, 1)
			expectRefused(t, method+" "+id, env.call("/api/"+domain+"/"+name, arguments, env.cookie), "not_found")
		}
	}
	if after := env.scratch.Everything(t); after != before {
		t.Errorf("a refused write changed the file: %v", linesAdded(before, after))
	}
}

// A write that is refused must be refused, not answered as one that changed
// nothing: the second is a 200 with a change id.
func TestAnUnknownTargetIsARefusalAndNotAnEmptyChange(t *testing.T) {
	env := newAuthoringEnv(t)
	changes := len(env.scratch.Changes(t))
	response := env.call("/api/transactions/setNote", argumentsOf("no-such-transaction", ""), env.cookie)
	expectRefused(t, "an empty note on an unknown transaction", response, "not_found")
	if len(env.scratch.Changes(t)) != changes {
		t.Error("the refused write left a change in the journal")
	}
}

func TestAWriteThatChangesNothingAnswersAChangeWithNoEffectAndItsUndoDoesNothing(t *testing.T) {
	setup := []writeCall{
		{"transactions.addTag", argumentsOf(txPending, "#a")},
		{"transactions.addFixedTag", argumentsOf(txPending, "#f")},
		{"transactions.setNote", argumentsOf(txPending, "nota")},
		{"transactions.setNickname", argumentsOf(txPending, "Pão")},
	}
	asked := []writeCall{
		{"transactions.addTag", argumentsOf(txPending, "#a")},
		{"transactions.addTag", argumentsOf(txPending, "#")},
		{"transactions.addTag", argumentsOf(txPending, "   ")},
		{"transactions.removeTag", argumentsOf(txPending, "#nunca")},
		{"transactions.addTagToMany", argumentsOf([]string{txPending}, "a")},
		{"transactions.addTagToMany", argumentsOf([]string{}, "a")},
		{"transactions.addFixedTag", argumentsOf(txPending, "f")},
		{"transactions.removeFixedTag", argumentsOf(txPending, "#nunca")},
		{"transactions.setNote", argumentsOf(txPending, "nota")},
		{"transactions.setNote", argumentsOf(txMonthEnd, "")},
		{"transactions.setNickname", argumentsOf(txPending, " Pão ")},
		{"transactions.setNickname", argumentsOf(txMonthEnd, "   ")},
	}
	env := newAuthoringEnv(t)
	for _, call := range setup {
		env.change(call.method, call.arguments)
	}
	for _, call := range asked {
		canonical, effects := env.scratch.Canonical(t), env.scratch.EffectCount(t)
		id := env.change(call.method, call.arguments)
		if got := env.scratch.EffectCount(t); got != effects {
			t.Errorf("%s %s recorded %d effects, want an empty change", call.method, call.arguments, got-effects)
		}
		env.undo(id)
		if after := env.scratch.Canonical(t); after != canonical {
			t.Errorf("%s %s: its undo changed the tables:\n%s\nwas\n%s", call.method, call.arguments, after, canonical)
		}
	}
}

func TestOverAMissingSnapshotEveryWriteAnswersLedgerUnavailableAndRecordsNothing(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	missing := t.TempDir() + "/no-snapshot.db"
	env := newTestEnv(t, withLedger(missing, fixtureNow), withDatabase(scratch.Database))
	cookie := env.unlock()
	before := scratch.Everything(t)
	calls := map[string]string{"transactions.addTagToMany": argumentsOf([]string{txMonthEnd}, "viagem")}
	for method, template := range singleTransactionWrites {
		calls[method] = strings.Replace(template, "%s", txMonthEnd, 1)
	}
	for method, arguments := range calls {
		domain, name := splitMethod(t, method)
		expectError(t, method, env.call("/api/"+domain+"/"+name, arguments, cookie), http.StatusServiceUnavailable, "ledger_unavailable")
	}
	if after := scratch.Everything(t); after != before {
		t.Errorf("a write over a missing snapshot changed the file: %v", linesAdded(before, after))
	}
}

// The selection is resolved against one reading of the whole ledger, and a
// single write against the month's own: both must find the same transaction.
func TestTheSelectionFindsTheTransactionsTheSingleLookupFinds(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	full := newFullLedger(t)
	var ids []string
	for _, listed := range []string{"2026-01", "2026-02", "2026-03"} {
		env := newReadEnvOver(t, full, scratch.Database)
		for _, row := range transactionsOf(t, env, listed, "month") {
			ids = append(ids, row["id"].(string))
		}
	}
	for _, id := range append(ids, txForeignTax, txPotTransfer, "no-such-transaction") {
		var single, bulk bool
		var singleKey, bulkKey authored.TxKey
		err := inChange(t, scratch, full, func(writes Writes) error {
			target, found, err := lookupTarget(t.Context(), writes, id)
			single, singleKey = found, target.key
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		err = inChange(t, scratch, full, func(writes Writes) error {
			targets, err := targetsNamedBy(t.Context(), writes, []string{id})
			if err == nil {
				bulk, bulkKey = len(targets) == 1, targets[0].key
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if single != bulk || singleKey != bulkKey {
			t.Errorf("%s: the single lookup answers %v key %d, the selection %v key %d", id, single, singleKey, bulk, bulkKey)
		}
	}
}
