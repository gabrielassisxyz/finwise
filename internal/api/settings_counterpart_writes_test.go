package api

import (
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// settingsCounterpartWriteCases are the settings' two writes about an
// establishment or a person, each recorded and undone, and the refusals only
// they produce: a key nothing lists, a key only a movement carries, a default
// line for a person, and a line the current month's budget lacks.
var settingsCounterpartWriteCases = []writeCase{
	{method: "settings.setNickname", arguments: `["text:PADARIA EXEMPLO","  Pão  "]`, expects: expectsRecorded},
	{method: "settings.setNickname", arguments: `["doc:11122233344","Maria"]`, expects: expectsRecorded},
	{method: "settings.setNickname", arguments: `["text:PADARIA EXEMPLO",""]`, expects: expectsRecorded,
		before: []writeCall{{"settings.setNickname", `["text:PADARIA EXEMPLO","Pão"]`}}},
	{method: "settings.setDefaultLine", arguments: `["text:MERCADO EXEMPLO","Remédios"]`, expects: expectsRecorded, state: budgetState()},
	{method: "settings.setDefaultLine", arguments: `["text:MERCADO EXEMPLO","Tarifas e juros"]`, expects: expectsRecorded, state: budgetState()},
	{method: "settings.setNickname", arguments: `["text:no-such-establishment","Pão"]`, expects: expectsRefusal, reason: reasonNotFound},
	{method: "settings.setNickname", arguments: `["doc:98765432100","Pão"]`, expects: expectsRefusal, reason: reasonNotFound},
	{method: "settings.setDefaultLine", arguments: `["text:no-such-establishment","Remédios"]`, expects: expectsRefusal, reason: reasonNotFound, state: budgetState()},
	{method: "settings.setDefaultLine", arguments: `["doc:11122233344","Remédios"]`, expects: expectsRefusal, reason: reasonNotFound, state: budgetState()},
	{method: "settings.setDefaultLine", arguments: `["doc:98765432100","Remédios"]`, expects: expectsRefusal, reason: reasonNotFound, state: budgetState()},
	{method: "settings.setDefaultLine", arguments: `["text:MERCADO EXEMPLO","Inexistente"]`, expects: expectsRefusal, reason: reasonUnknownLine, state: budgetState()},
	{method: "settings.setDefaultLine", arguments: `["text:MERCADO EXEMPLO","Tarifas e juros"]`, expects: expectsRefusal, reason: reasonUnknownLine},
}

func init() {
	writeCases = append(writeCases, settingsCounterpartWriteCases...)
	// Documents of the invented people, which the keys of these cases carry.
	secrets = append(secrets, "11122233344", "98765432100")
}

const keyOfTheOwnAccount = "doc:98765432100"

var bakeryRows = []string{txPending, txLastOfFebruary}

// overviewLists is the establishments and the people as the server wrote them.
func (env *settingsEnv) overviewLists() string {
	env.t.Helper()
	return env.overviewField("establishments") + env.overviewField("people")
}

func rowOfKey(t *testing.T, rows []map[string]any, key string) map[string]any {
	t.Helper()
	for _, row := range rows {
		if row["key"] == key {
			return row
		}
	}
	t.Fatalf("%s is not listed", key)
	return nil
}

func TestANicknameSetInTheSettingsRenamesEveryTransactionOfItsKeyAndABlankOneGivesTheNameBack(t *testing.T) {
	env := newAuthoringEnv(t)
	env.change("settings.setNickname", argumentsOf(keyOfTheBakery, "  Pão  "))
	for _, tx := range bakeryRows {
		if name, original := nameOf(t, env, tx); name != "Pão" || original != ledgerNameOfTheBakery {
			t.Errorf("%s is named %v (originally %v), want Pão over the ledger's %s", tx, name, original, ledgerNameOfTheBakery)
		}
		if got := detailOver(t, env.readEnv, tx)["name"]; got != "Pão" {
			t.Errorf("the detail of %s names it %v", tx, got)
		}
	}
	establishments, _ := listsOf(t, env.readEnv)
	if got := rowOfKey(t, establishments, keyOfTheBakery)["nickname"]; got != "Pão" {
		t.Errorf("the overview answers the nickname %v, want the trimmed Pão", got)
	}
	env.change("settings.setNickname", argumentsOf(keyOfTheBakery, "   "))
	for _, tx := range bakeryRows {
		if name, _ := nameOf(t, env, tx); name != ledgerNameOfTheBakery {
			t.Errorf("%s is named %v after the nickname was cleared, want the ledger's name", tx, name)
		}
	}
	establishments, _ = listsOf(t, env.readEnv)
	if got := rowOfKey(t, establishments, keyOfTheBakery)["nickname"]; got != nil {
		t.Errorf("the overview answers the nickname %v after it was cleared, want null", got)
	}
	if rows := tableRows(t, env.scratch, "establishments"); len(rows) != 0 {
		t.Errorf("a cleared nickname left %v", rows)
	}
}

func TestAPersonIsNamedFromTheSettingsAndThePanelShowsItAndTheFullNameStays(t *testing.T) {
	env := newAuthoringEnv(t)
	env.change("settings.setNickname", argumentsOf(keyOfTheFriend, "Maria"))
	_, people := listsOf(t, env.readEnv)
	friend := rowOfKey(t, people, keyOfTheFriend)
	if friend["nickname"] != "Maria" || friend["fullName"] != "Maria Exemplo" {
		t.Errorf("the person is %v, want the nickname beside the ledger's own name", friend)
	}
	if got := nicknameOf(t, env, txPixOut); got != "Maria" {
		t.Errorf("the detail of the PIX answers the nickname %v", got)
	}
}

// The settings and the transaction panel are two doors to one store: what either
// writes the other reads, and both write the same cell.
func TestTheNicknameIsOneStoreWhicheverDoorWroteIt(t *testing.T) {
	panel := newAuthoringEnv(t)
	panel.change("transactions.setNickname", argumentsOf(txPending, "  Pão  "))
	establishments, _ := listsOf(t, panel.readEnv)
	if got := rowOfKey(t, establishments, keyOfTheBakery)["nickname"]; got != "Pão" {
		t.Errorf("a nickname written from the panel is answered by the overview as %v", got)
	}

	settings := newAuthoringEnv(t)
	settings.change("settings.setNickname", argumentsOf(keyOfTheBakery, "  Pão  "))
	for _, tx := range bakeryRows {
		if got := nicknameOf(t, settings, tx); got != "Pão" {
			t.Errorf("a nickname written from the settings is answered by the detail of %s as %v", tx, got)
		}
	}
	if fromPanel, fromSettings := effectsOfTheLastChange(t, panel), effectsOfTheLastChange(t, settings); !slices.Equal(fromPanel, fromSettings) {
		t.Errorf("the two doors wrote different cells:\npanel    %v\nsettings %v", fromPanel, fromSettings)
	}
	if fromPanel, fromSettings := tableRows(t, panel.scratch, "establishments"), tableRows(t, settings.scratch, "establishments"); !slices.Equal(fromPanel, fromSettings) {
		t.Errorf("the two doors left %v and %v", fromPanel, fromSettings)
	}
}

// effectsOfTheLastChange are the table, key, column and values of what the
// newest change did, without the numbers the journal gave them.
func effectsOfTheLastChange(t *testing.T, env *settingsEnv) []authoredtest.Effect {
	t.Helper()
	changes := env.scratch.Changes(t)
	effects := env.scratch.EffectsOf(t, changes[len(changes)-1].ID)
	for index := range effects {
		effects[index].Seq = 0
	}
	return effects
}

func TestADefaultLineSetInTheSettingsIsTheOneThePanelShowsAndMovesNoTransaction(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	full := newFullLedger(t)
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	lines := lineIDsByName(t, scratch.Database)
	authorOver(t, scratch, viewOf(t, full.Path), lines, firstDecisions, "scratch.seedLine", func(a authoring) error {
		return a.state(txMonthEnd, authored.TransactionState{
			Line: a.line("Delivery"), ClassifiedBy: authored.AuthorPerson, PendingReason: "Confiança abaixo do limite",
			PendingLine: a.line("Mercado"), PendingConfidence: authoredtest.Some[int64](64),
		})
	})
	env := &settingsEnv{readEnv: newReadEnvOver(t, full, scratch.Database), scratch: scratch}
	before := detailOver(t, env.readEnv, txMonthEnd)

	id := env.change("settings.setDefaultLine", argumentsOf(keyOfTheMarket, "Remédios"))

	if got := objectAt(t, detailOver(t, env.readEnv, txMonthEnd), "counterparty")["defaultLine"]; got != "Remédios" {
		t.Errorf("the panel shows the default line %v, want Remédios", got)
	}
	establishments, _ := listsOf(t, env.readEnv)
	if got := rowOfKey(t, establishments, keyOfTheMarket)["defaultLine"]; got != "Remédios" {
		t.Errorf("the overview shows the default line %v, want Remédios", got)
	}
	after := detailOver(t, env.readEnv, txMonthEnd)
	for _, field := range []string{"line", "classifiedBy", "pending", "confidence"} {
		if !jsonEqual(before[field], after[field]) {
			t.Errorf("%s moved from %v to %v: a default line acts on transactions that arrive later", field, before[field], after[field])
		}
	}
	for _, effect := range scratch.EffectsOf(t, changeNumber(t, id)) {
		if effect.Table != "establishments" {
			t.Errorf("the change has an effect on %s, want the establishment's row alone", effect.Table)
		}
	}
	want := `key="text:MERCADO EXEMPLO" nickname=NULL default_line_id=` + strconv.Itoa(int(lines["Remédios"]))
	if rows := tableRows(t, scratch, "establishments"); !slices.Equal(rows, []string{want}) {
		t.Errorf("establishments hold %v, want %s", rows, want)
	}
}

func TestTheFeesLineIsADefaultLineOnlyWhileTheCurrentMonthsBudgetHasALine(t *testing.T) {
	env := newAuthoringEnv(t)
	expectRefused(t, "a budget with no line", env.call("/api/settings/setDefaultLine", argumentsOf(keyOfTheMarket, "Tarifas e juros"), env.cookie), "unknown_line")
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	env.change("settings.setDefaultLine", argumentsOf(keyOfTheMarket, "Tarifas e juros"))
	establishments, _ := listsOf(t, env.readEnv)
	if got := rowOfKey(t, establishments, keyOfTheMarket)["defaultLine"]; got != "Tarifas e juros" {
		t.Errorf("the default line is %v, want the fees line", got)
	}
}

// Two versions of the budget name one line differently: the line is resolved in
// the one in force for the current month, March, and in no other.
func TestTheDefaultLineIsResolvedInTheCurrentMonthsBudgetAndNoOtherMonths(t *testing.T) {
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
		for index := range group.Lines {
			if group.Lines[index].Name == "Remédios" {
				group.Lines[index].Name = "Farmácia"
			}
		}
	}
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-03", renamed)
	later := authoredtest.DocumentOf(february)
	for _, group := range later {
		for index := range group.Lines {
			if group.Lines[index].Name == "Remédios" {
				group.Lines[index].Name = "Boticário"
			}
		}
	}
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-04", later)

	for _, name := range []string{"Remédios", "Boticário"} {
		expectRefused(t, "a name only another month's budget has: "+name,
			env.call("/api/settings/setDefaultLine", argumentsOf(keyOfTheMarket, name), env.cookie), "unknown_line")
	}
	if rows := tableRows(t, env.scratch, "establishments"); len(rows) != 0 {
		t.Fatalf("a refused default line left %v", rows)
	}
	env.change("settings.setDefaultLine", argumentsOf(keyOfTheMarket, "Farmácia"))
	establishments, _ := listsOf(t, env.readEnv)
	if got := rowOfKey(t, establishments, keyOfTheMarket)["defaultLine"]; got != "Farmácia" {
		t.Errorf("the default line is %v, want Farmácia", got)
	}
}

func TestAKeyNoListedTransactionHasAndAPersonsDefaultLineAreRefusedAndNothingIsRecorded(t *testing.T) {
	env := newAuthoringEnv(t)
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	before := env.scratch.Everything(t)
	for _, call := range []writeCall{
		{"settings.setNickname", argumentsOf("text:no-such-establishment", "Pão")},
		{"settings.setDefaultLine", argumentsOf("text:no-such-establishment", "Remédios")},
		{"settings.setDefaultLine", argumentsOf(keyOfTheFriend, "Remédios")},
	} {
		domain, name := splitMethod(t, call.method)
		expectRefused(t, call.method+" "+call.arguments, env.call("/api/"+domain+"/"+name, call.arguments, env.cookie), "not_found")
	}
	if after := env.scratch.Everything(t); after != before {
		t.Errorf("a refused write changed the file: %v", linesAdded(before, after))
	}
}

// The key is checked before the line, as the panel checks the transaction before
// the line: what a key names does not depend on a budget.
func TestTheKeyIsCheckedBeforeTheLine(t *testing.T) {
	env := newAuthoringEnv(t)
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	for _, key := range []string{"text:no-such-establishment", keyOfTheFriend, keyOfTheOwnAccount} {
		expectRefused(t, key, env.call("/api/settings/setDefaultLine", argumentsOf(key, "Inexistente"), env.cookie), "not_found")
	}
	expectRefused(t, "a listed establishment", env.call("/api/settings/setDefaultLine", argumentsOf(keyOfTheMarket, "Inexistente"), env.cookie), "unknown_line")
}

// A key only movements carry is known to the ledger and listed by nobody: a
// default line is refused, and a nickname is refused as the panel refuses it for
// the transfer itself, so the two doors to a nickname agree.
func TestAKeyOnlyAMovementCarriesIsRefusedAsThePanelRefusesIt(t *testing.T) {
	env := newAuthoringEnv(t)
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	before := env.scratch.Everything(t)
	panel := env.call("/api/transactions/setNickname", argumentsOf(txOwnTransfer, "Minha conta"), env.cookie)
	settings := env.call("/api/settings/setNickname", argumentsOf(keyOfTheOwnAccount, "Minha conta"), env.cookie)
	expectRefused(t, "the panel", panel, "not_found")
	if panel.Code != settings.Code || panel.Body.String() != settings.Body.String() {
		t.Errorf("the settings answered %d %s where the panel answered %d %s", settings.Code, settings.Body, panel.Code, panel.Body)
	}
	expectRefused(t, "the default line", env.call("/api/settings/setDefaultLine", argumentsOf(keyOfTheOwnAccount, "Remédios"), env.cookie), "not_found")
	if after := env.scratch.Everything(t); after != before {
		t.Errorf("a refused write changed the file: %v", linesAdded(before, after))
	}
}

func TestSettingWhatTheKeyAlreadyHasRecordsAnEmptyChangeAndItsUndoDoesNothing(t *testing.T) {
	for _, call := range []writeCall{
		{"settings.setNickname", argumentsOf(keyOfTheBakery, "Pão")},
		{"settings.setDefaultLine", argumentsOf(keyOfTheMarket, "Remédios")},
	} {
		env := newAuthoringEnv(t)
		authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
		env.change(call.method, call.arguments)
		kept := env.scratch.Canonical(t)
		again := env.change(call.method, call.arguments)
		if effects := env.scratch.EffectsOf(t, changeNumber(t, again)); len(effects) != 0 {
			t.Errorf("%s again has the effects %v, want none", call.method, effects)
		}
		if env.scratch.Canonical(t) != kept {
			t.Errorf("%s again changed a journaled table", call.method)
		}
		env.undo(again)
		if env.scratch.Canonical(t) != kept {
			t.Errorf("the undo of the empty change of %s changed a journaled table", call.method)
		}
	}
}

func TestTheUndoOfEachWriteBringsBackTheTablesAndTheOverview(t *testing.T) {
	for _, call := range []writeCall{
		{"settings.setNickname", argumentsOf(keyOfTheBakery, "Pão")},
		{"settings.setNickname", argumentsOf(keyOfTheFriend, "Maria")},
		{"settings.setDefaultLine", argumentsOf(keyOfTheMarket, "Remédios")},
	} {
		env := newAuthoringEnv(t)
		authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
		canonical, lists := env.scratch.Canonical(t), env.overviewLists()
		id := env.change(call.method, call.arguments)
		if env.overviewLists() == lists {
			t.Errorf("%s %s left the overview as it was", call.method, call.arguments)
		}
		env.undo(id)
		if got := env.scratch.Canonical(t); got != canonical {
			t.Errorf("the undo of %s %s left the journaled tables otherwise:\n%s", call.method, call.arguments, differences(canonical, got))
		}
		if got := env.overviewLists(); got != lists {
			t.Errorf("the undo of %s %s left the overview as %s, want %s", call.method, call.arguments, got, lists)
		}
	}
}

func TestWithTheSnapshotMissingBothWritesAnswerLedgerUnavailableAndRecordNothing(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	env := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "absent.db"), fixtureNow), withDatabase(scratch.Database))
	cookie := env.unlock()
	before := scratch.Everything(t)
	for route, arguments := range map[string]string{
		"/api/settings/setNickname":    argumentsOf(keyOfTheBakery, "Pão"),
		"/api/settings/setDefaultLine": argumentsOf(keyOfTheMarket, "Remédios"),
	} {
		expectError(t, route, env.call(route, arguments, cookie), http.StatusServiceUnavailable, "ledger_unavailable")
	}
	if after := scratch.Everything(t); after != before {
		t.Errorf("a write without a ledger changed the file: %v", linesAdded(before, after))
	}
}

func TestWithoutTheAppsDatabaseBothWritesAnswerReadOnlyBeforeTheirBodyIsRead(t *testing.T) {
	env := newTestEnv(t, withLedger(newFullLedger(t).Path, fixtureNow))
	cookie := env.unlock()
	for _, route := range []string{"/api/settings/setNickname", "/api/settings/setDefaultLine"} {
		expectError(t, route, env.call(route, "this is not JSON", cookie), http.StatusForbidden, "read_only")
	}
}

func TestAnArgumentThatIsNotTwoTextsIsABadRequestAndRecordsNothing(t *testing.T) {
	env := newAuthoringEnv(t)
	before := env.scratch.Everything(t)
	for _, route := range []string{"/api/settings/setNickname", "/api/settings/setDefaultLine"} {
		for _, arguments := range []string{`[]`, `["text:PADARIA EXEMPLO"]`, `["text:PADARIA EXEMPLO",5]`, `[null,"Pão"]`} {
			expectError(t, route+" "+arguments, env.call(route, arguments, env.cookie), http.StatusBadRequest, "bad_request")
		}
	}
	if after := env.scratch.Everything(t); after != before {
		t.Errorf("a bad request changed the file: %v", linesAdded(before, after))
	}
}
