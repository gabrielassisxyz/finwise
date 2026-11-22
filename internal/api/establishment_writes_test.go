package api

import (
	"slices"
	"strconv"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// establishmentWriteCases are the four writes about the establishment or the
// person behind a transaction, each recorded and undone, and the refusals only
// they produce: a default line for a person or a movement, a line the current
// month's budget lacks, and any of them for a movement.
//
// The pending purchase at the bakery, the PIX to a person and the transfer
// between the person's own accounts are the establishment, the person and the
// movement of the full ledger.
var establishmentWriteCases = []writeCase{
	{method: "transactions.setNickname", arguments: `["tx-pending","  Pão  "]`, expects: expectsRecorded},
	{method: "transactions.setNickname", arguments: `["tx-pix-out","Tia Maria"]`, expects: expectsRecorded},
	{method: "transactions.setNickname", arguments: `["tx-pending",""]`, expects: expectsRecorded,
		before: []writeCall{{"transactions.setNickname", `["tx-pending","Pão"]`}}},
	{method: "transactions.setDefaultLine", arguments: `["tx-pending","Remédios"]`, expects: expectsRecorded, state: budgetState()},
	{method: "transactions.setDefaultLine", arguments: `["tx-pending","Tarifas e juros"]`, expects: expectsRecorded, state: budgetState()},
	{method: "transactions.addFixedTag", arguments: `["tx-pending","  viagem de trabalho "]`, expects: expectsRecorded},
	{method: "transactions.removeFixedTag", arguments: `["tx-pending","#viagem-de-trabalho"]`, expects: expectsRecorded,
		before: []writeCall{{"transactions.addFixedTag", `["tx-pending","viagem de trabalho"]`}}},
	{method: "transactions.setDefaultLine", arguments: `["tx-pix-out","Remédios"]`, expects: expectsRefusal, reason: reasonNotFound, state: budgetState()},
	{method: "transactions.setDefaultLine", arguments: `["tx-own-transfer","Remédios"]`, expects: expectsRefusal, reason: reasonNotFound, state: budgetState()},
	{method: "transactions.setDefaultLine", arguments: `["tx-pending","Inexistente"]`, expects: expectsRefusal, reason: reasonUnknownLine, state: budgetState()},
	{method: "transactions.setDefaultLine", arguments: `["tx-pending","Tarifas e juros"]`, expects: expectsRefusal, reason: reasonUnknownLine},
	{method: "transactions.setNickname", arguments: `["tx-own-transfer","Pão"]`, expects: expectsRefusal, reason: reasonNotFound},
	{method: "transactions.addFixedTag", arguments: `["tx-own-transfer","viagem"]`, expects: expectsRefusal, reason: reasonNotFound},
	{method: "transactions.removeFixedTag", arguments: `["tx-own-transfer","#viagem"]`, expects: expectsRefusal, reason: reasonNotFound},
}

// tagAndNicknameWriteCases are the cases of the eight writes, for the shared
// list.
var tagAndNicknameWriteCases = append(append([]writeCase(nil), tagAndNoteWriteCases...), establishmentWriteCases...)

const (
	ledgerNameOfTheBakery = "PADARIA EXEMPLO"
	keyOfTheFriend        = "doc:11122233344"
)

func nameOf(t *testing.T, env *settingsEnv, id string) (name, originalName any) {
	t.Helper()
	row := rowWithIDIn(t, transactionsOf(t, env.readEnv, "2026-03", "last-3-months"), id)
	return row["name"], row["originalName"]
}

func nicknameOf(t *testing.T, env *settingsEnv, id string) any {
	t.Helper()
	return objectAt(t, detailOver(t, env.readEnv, id), "counterparty")["nickname"]
}

func TestANicknameRenamesEveryTransactionOfItsKeyAndABlankOneBringsTheLedgersNameBack(t *testing.T) {
	env := newAuthoringEnv(t)
	bakery := []string{txPending, txLastOfFebruary}
	env.change("transactions.setNickname", argumentsOf(txPending, "  Pão  "))
	for _, tx := range bakery {
		if name, original := nameOf(t, env, tx); name != "Pão" || original != ledgerNameOfTheBakery {
			t.Errorf("%s is named %v (originally %v), want Pão over the ledger's %s", tx, name, original, ledgerNameOfTheBakery)
		}
		if got := detailOver(t, env.readEnv, tx)["name"]; got != "Pão" {
			t.Errorf("the detail of %s names it %v", tx, got)
		}
		if got := nicknameOf(t, env, tx); got != "Pão" {
			t.Errorf("the detail of %s answers the nickname %v", tx, got)
		}
	}
	if name, _ := nameOf(t, env, txMonthEnd); name == "Pão" {
		t.Error("a purchase of another establishment was renamed")
	}
	env.change("transactions.setNickname", argumentsOf(txLastOfFebruary, "   "))
	for _, tx := range bakery {
		if name, _ := nameOf(t, env, tx); name != ledgerNameOfTheBakery {
			t.Errorf("%s is named %v after the nickname was cleared, want the ledger's name", tx, name)
		}
		if got := nicknameOf(t, env, tx); got != nil {
			t.Errorf("the detail of %s answers the nickname %v after it was cleared, want null", tx, got)
		}
	}
	if rows := tableRows(t, env.scratch, "establishments"); len(rows) != 0 {
		t.Errorf("a cleared nickname left %v", rows)
	}
}

func TestAPersonIsNamedOnEveryPixWithThem(t *testing.T) {
	env := newAuthoringEnv(t)
	env.change("transactions.setNickname", argumentsOf(txPixOut, "Tia Maria"))
	if name, original := nameOf(t, env, txPixOut); name != "Tia Maria" || original == "Tia Maria" {
		t.Errorf("the PIX is named %v (originally %v)", name, original)
	}
	if got := nicknameOf(t, env, txPixOut); got != "Tia Maria" {
		t.Errorf("the person's nickname is %v", got)
	}
	env.change("transactions.setNickname", argumentsOf(txPixOut, " "))
	if got := nicknameOf(t, env, txPixOut); got != nil {
		t.Errorf("the nickname is %v after it was cleared", got)
	}
	if rows := tableRows(t, env.scratch, "establishments"); len(rows) != 0 {
		t.Errorf("a cleared nickname left %v", rows)
	}
}

func TestADefaultLineIsStoredByIDAndMovesNoTransaction(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	full := newFullLedger(t)
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	lines := lineIDsByName(t, scratch.Database)
	snapshot := viewOf(t, full.Path)
	authorOver(t, scratch, snapshot, lines, firstDecisions, "scratch.seedLine", func(a authoring) error {
		return a.state(txMonthEnd, authored.TransactionState{
			Line: a.line("Delivery"), ClassifiedBy: authored.AuthorPerson, PendingReason: "Confiança abaixo do limite",
			PendingLine: a.line("Mercado"), PendingConfidence: authoredtest.Some[int64](64),
		})
	})
	env := &settingsEnv{readEnv: newReadEnvOver(t, full, scratch.Database), scratch: scratch}
	before := detailOver(t, env.readEnv, txMonthEnd)

	id := env.change("transactions.setDefaultLine", argumentsOf(txMonthEnd, "Remédios"))

	after := detailOver(t, env.readEnv, txMonthEnd)
	if got := objectAt(t, after, "counterparty")["defaultLine"]; got != "Remédios" {
		t.Errorf("the default line is %v, want Remédios", got)
	}
	for _, field := range []string{"line", "classifiedBy", "pending", "confidence"} {
		if !jsonEqual(before[field], after[field]) {
			t.Errorf("%s moved from %v to %v: a default line acts on transactions that arrive later", field, before[field], after[field])
		}
	}
	rows := tableRows(t, scratch, "establishments")
	want := `key="text:MERCADO EXEMPLO" nickname=NULL default_line_id=` + strconv.Itoa(int(lines["Remédios"]))
	if !slices.Equal(rows, []string{want}) {
		t.Errorf("establishments hold %v, want %s", rows, want)
	}
	changeNumber := changeNumber(t, id)
	for _, effect := range scratch.EffectsOf(t, changeNumber) {
		if effect.Table != "establishments" {
			t.Errorf("the change has an effect on %s, want the establishment's row alone", effect.Table)
		}
	}
}

func jsonEqual(a, b any) bool {
	return argumentsOf(a) == argumentsOf(b)
}

// The default line is resolved in the budget in force for the current month,
// whatever month the transaction it was chosen on is in.
func TestADefaultLineIsResolvedInTheCurrentMonthsBudgetAndNotTheTransactionsOwn(t *testing.T) {
	env := newAuthoringEnv(t)
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	view := env.scratch.Database.View()
	february, err := view.BudgetOf(t.Context(), "2026-02")
	view.Close()
	if err != nil {
		t.Fatal(err)
	}
	renamed := authoredtest.DocumentOf(february)
	for _, group := range renamed {
		for i := range group.Lines {
			if group.Lines[i].Name == "Remédios" {
				group.Lines[i].Name = "Farmácia"
			}
		}
	}
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-03", renamed)

	// txMonthEnd is of 28 February, whose budget calls the line Remédios; the
	// current month is March, whose budget calls it Farmácia.
	expectRefused(t, "the name of the transaction's own month",
		env.call("/api/transactions/setDefaultLine", argumentsOf(txMonthEnd, "Remédios"), env.cookie), "unknown_line")
	if rows := tableRows(t, env.scratch, "establishments"); len(rows) != 0 {
		t.Fatalf("a refused default line left %v", rows)
	}
	env.change("transactions.setDefaultLine", argumentsOf(txMonthEnd, "Farmácia"))
	counterparty := objectAt(t, detailOver(t, env.readEnv, txMonthEnd), "counterparty")
	if counterparty["defaultLine"] != "Farmácia" {
		t.Errorf("the default line is %v, want Farmácia", counterparty["defaultLine"])
	}
}

func TestTheFeesLineCanBeADefaultLineOnlyWhileTheCurrentMonthsBudgetHasALine(t *testing.T) {
	env := newAuthoringEnv(t)
	expectRefused(t, "a budget with no line", env.call("/api/transactions/setDefaultLine", argumentsOf(txPending, "Tarifas e juros"), env.cookie), "unknown_line")
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	env.change("transactions.setDefaultLine", argumentsOf(txPending, "Tarifas e juros"))
	if got := objectAt(t, detailOver(t, env.readEnv, txPending), "counterparty")["defaultLine"]; got != "Tarifas e juros" {
		t.Errorf("the default line is %v, want the fees line", got)
	}
}

func TestADefaultLineTheCurrentMonthNoLongerHasIsNullWhileItsRowKeepsTheLineID(t *testing.T) {
	env := newAuthoringEnv(t)
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	env.change("transactions.setDefaultLine", argumentsOf(txPending, "Delivery"))
	stored := tableRows(t, env.scratch, "establishments")

	view := env.scratch.Database.View()
	current, err := view.BudgetOf(t.Context(), "2026-03")
	view.Close()
	if err != nil {
		t.Fatal(err)
	}
	without := authoredtest.DocumentOf(current)
	for i := range without {
		without[i].Lines = slices.DeleteFunc(without[i].Lines, func(line authored.DocumentLine) bool { return line.Name == "Delivery" })
	}
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-03", without)

	if got := objectAt(t, detailOver(t, env.readEnv, txPending), "counterparty")["defaultLine"]; got != nil {
		t.Errorf("the default line is %v, want null where the current month lacks it", got)
	}
	if rows := tableRows(t, env.scratch, "establishments"); !slices.Equal(rows, stored) || len(rows) != 1 {
		t.Errorf("the establishment's row is %v, want %v kept with its line id", rows, stored)
	}
}

func TestAFixedTagShowsOnEveryTransactionOfTheKeyOnceAndComesOffThemAll(t *testing.T) {
	env := newAuthoringEnv(t)
	env.change("transactions.addTag", argumentsOf(txPending, "#viagem"))
	env.change("transactions.addFixedTag", argumentsOf(txPending, "viagem"))
	env.change("transactions.addFixedTag", argumentsOf(txLastOfFebruary, "#viagem"))
	if rows := tableRows(t, env.scratch, "establishment_tags"); len(rows) != 1 {
		t.Errorf("the same fixed tag twice left %v, want one row", rows)
	}
	for _, tx := range []string{txPending, txLastOfFebruary} {
		if got := fixedTagsOf(t, env, tx); !slices.Equal(got, []string{"#viagem"}) {
			t.Errorf("%s answers the fixed tags %v", tx, got)
		}
		if got := texts(t, detailOver(t, env.readEnv, tx)["tags"]); !slices.Equal(got, []string{"#viagem"}) {
			t.Errorf("%s carries %v, want #viagem once even where it is also an own tag", tx, got)
		}
	}
	for _, row := range transactionsOf(t, env.readEnv, "2026-03", "last-3-months") {
		if row["establishmentKey"] == keyOfTheBakery {
			if got := texts(t, row["tags"]); !slices.Equal(got, []string{"#viagem"}) {
				t.Errorf("the list shows %v on %v", got, row["id"])
			}
		}
	}
	env.change("transactions.removeFixedTag", argumentsOf(txLastOfFebruary, "#viagem"))
	if got := fixedTagsOf(t, env, txPending); len(got) != 0 {
		t.Errorf("the fixed tag is still on the bakery: %v", got)
	}
	if got := ownTagsOf(t, env, txPending); !slices.Equal(got, []string{"#viagem"}) {
		t.Errorf("removing the fixed tag took the own tag too: %v", got)
	}
}

// A movement is neither an establishment nor a person: the panel offers it no
// section for either, so a nickname or a fixed tag stored under its key could
// not be taken away from there while the list kept showing it. The stand-in
// stores them; the server refuses.
func TestAMovementHasNoNicknameNoDefaultLineAndNoFixedTags(t *testing.T) {
	env := newAuthoringEnv(t)
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	before := env.scratch.Everything(t)
	for _, call := range []writeCall{
		{"transactions.setNickname", argumentsOf(txOwnTransfer, "Minha conta")},
		{"transactions.setDefaultLine", argumentsOf(txOwnTransfer, "Remédios")},
		{"transactions.addFixedTag", argumentsOf(txOwnTransfer, "viagem")},
		{"transactions.removeFixedTag", argumentsOf(txOwnTransfer, "#viagem")},
	} {
		domain, name := splitMethod(t, call.method)
		expectRefused(t, call.method, env.call("/api/"+domain+"/"+name, call.arguments, env.cookie), "not_found")
	}
	if after := env.scratch.Everything(t); after != before {
		t.Errorf("a refused write about a movement changed the file: %v", linesAdded(before, after))
	}
	if got := detailOver(t, env.readEnv, txOwnTransfer)["counterparty"]; got != nil {
		t.Errorf("the transfer's panel has a counterparty section: %v", got)
	}
}
