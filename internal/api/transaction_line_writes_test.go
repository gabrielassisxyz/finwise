package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// The line of the line writes' budgets: in no version from 2026-01, Mercado in
// the version from 2026-02 and Feira, under the same id, from 2026-03. The ids
// of the other lines are the ones a fresh database mints for the invented
// budget without Mercado: 2 Salário, 3 Renda extra, 4 Contas da casa, 5 Saúde,
// 6 Delivery, 7 Remédios, 8 Viagens.
const (
	marketLine   authored.LineID = 9
	deliveryLine authored.LineID = 6
	// stationeryLine is in the version from 2026-02 alone, when a test saves it.
	stationeryLine = "Papelaria"

	nicknameOfTheWarehouse = "Atacado do Bairro"
	keyOfTheWarehouse      = "text:ATACADO EXEMPLO"
)

// lineVersions saves the three budgets the line writes are checked against:
// from 2026-01 one without the line, from 2026-02 one with it as Mercado, and
// from 2026-03 the same line as Feira.
func lineVersions(t *testing.T, scratch *authoredtest.Scratch) {
	t.Helper()
	saveLineVersions(t, scratch, false)
}

// lineVersionsWithStationery is lineVersions with one more line in the version
// from 2026-02, which the version from 2026-03 drops.
func lineVersionsWithStationery(t *testing.T, scratch *authoredtest.Scratch) {
	t.Helper()
	saveLineVersions(t, scratch, true)
}

func saveLineVersions(t *testing.T, scratch *authoredtest.Scratch, stationery bool) {
	t.Helper()
	january := authoredtest.InventedBudget()
	group(january, "var").Lines = group(january, "var").Lines[1:]
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-01", january)
	february := authoredtest.DocumentOf(budgetIn(t, scratch.Database, "2026-01"))
	group(february, "var").Lines = append(group(february, "var").Lines, authoredtest.Line("Mercado", 40_000))
	if stationery {
		group(february, "var").Lines = append(group(february, "var").Lines, authoredtest.Line(stationeryLine, 5_000))
	}
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-02", february)
	march := authoredtest.DocumentOf(budgetIn(t, scratch.Database, "2026-02"))
	lines := group(march, "var").Lines[:0]
	for _, line := range group(march, "var").Lines {
		switch line.Name {
		case "Mercado":
			line.Name = "Feira"
		case stationeryLine:
			continue
		}
		lines = append(lines, line)
	}
	group(march, "var").Lines = lines
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-03", march)
}

func group(document authored.Document, key string) *authored.DocumentGroup {
	for i := range document {
		if document[i].Key == key {
			return &document[i]
		}
	}
	panic("no group " + key)
}

func budgetIn(t *testing.T, database *authored.Database, month string) authored.Budget {
	t.Helper()
	view := database.View()
	defer view.Close()
	budget, err := view.BudgetOf(t.Context(), brday.Month(month))
	if err != nil {
		t.Fatal(err)
	}
	return budget
}

// authoredOver writes, in one change over the ledger at fixture, what write
// says about its transactions.
func authoredOver(t *testing.T, scratch *authoredtest.Scratch, fixture *ledgertest.Fixture, write func(a authoring) error) {
	t.Helper()
	authorOver(t, scratch, viewOf(t, fixture.Path), lineIDsByName(t, scratch.Database), fixtureNow, "scratch.seedTransactions", write)
}

// overTheFullLedger is a case's state written over a full ledger of its own,
// which holds what the case's own ledger holds.
func overTheFullLedger(write func(a authoring) error) func(t *testing.T, scratch *authoredtest.Scratch) {
	return func(t *testing.T, scratch *authoredtest.Scratch) {
		t.Helper()
		authoredOver(t, scratch, newFullLedger(t), write)
	}
}

func states(all ...func(t *testing.T, scratch *authoredtest.Scratch)) func(t *testing.T, scratch *authoredtest.Scratch) {
	return func(t *testing.T, scratch *authoredtest.Scratch) {
		t.Helper()
		for _, state := range all {
			state(t, scratch)
		}
	}
}

// bakeryNickname names the bakery of the full ledger.
func bakeryNickname(a authoring) error {
	return authoredtest.WriteEstablishment(a.ctx, a.writes.Recorder, keyOfTheBakery, nicknameOfTheBakery, nil)
}

// reviewsOfTheFullLedger leaves the boleto waiting for a review that suggests
// Feira, and the dollar purchase on Delivery by the model.
func reviewsOfTheFullLedger(a authoring) error {
	market, delivery := marketLine, deliveryLine
	return each(
		func() error {
			return a.state(txBoleto, authored.TransactionState{
				PendingReason: "Confiança abaixo do limite", PendingLine: &market, PendingConfidence: authoredtest.Some[int64](64),
			})
		},
		func() error {
			return a.state(txForeign, authored.TransactionState{Line: &delivery, ClassifiedBy: authored.AuthorModel, Confidence: authoredtest.Some[int64](91)})
		},
	)
}

// transactionLineWriteCases are the line writes in the harness: each recorded
// and undone, the extension of a line change, the answers of nothing to do, and
// the refusals.
var transactionLineWriteCases = []writeCase{
	{method: "transactions.setLine", arguments: `["tx-pending","Feira"]`, expects: expectsRecorded,
		state: states(lineVersions, overTheFullLedger(bakeryNickname))},
	{method: "transactions.setLine", arguments: `["tx-month-end","Mercado"]`, expects: expectsRecorded, state: lineVersions},
	{method: "transactions.setLine", arguments: `["tx-boleto","Feira"]`, expects: expectsRecorded,
		state: states(lineVersions, overTheFullLedger(reviewsOfTheFullLedger))},
	{method: "transactions.confirmLine", arguments: `["tx-boleto"]`, expects: expectsRecorded,
		state: states(lineVersions, overTheFullLedger(reviewsOfTheFullLedger))},
	{method: "transactions.confirmLine", arguments: `["tx-foreign"]`, expects: expectsRecorded,
		state: states(lineVersions, overTheFullLedger(reviewsOfTheFullLedger))},
	{method: "transactions.setLineForMany", arguments: `[["tx-boleto","tx-instalment","tx-boleto"],"Feira"]`, expects: expectsRecorded,
		state: states(lineVersions, overTheFullLedger(reviewsOfTheFullLedger))},
	// The three budgets are the first three changes, so the line change is the fourth.
	{method: "transactions.applyLineToEstablishment", arguments: `["4"]`, expects: expectsExtension, state: lineVersions,
		before: []writeCall{{"transactions.setLine", `["tx-pending","Feira"]`}}},
	{method: "transactions.setLine", arguments: `["no-such-tx","Feira"]`, expects: expectsNothing, state: lineVersions},
	{method: "transactions.setLine", arguments: `["tx-foreign-tax","Feira"]`, expects: expectsNothing, state: lineVersions},
	{method: "transactions.setLine", arguments: `["tx-pot-transfer","Feira"]`, expects: expectsNothing, state: lineVersions},
	{method: "transactions.setLine", arguments: `["tx-pending","Feira"]`, expects: expectsNothing, state: lineVersions,
		before: []writeCall{{"transactions.setLine", `["tx-pending","Feira"]`}}},
	{method: "transactions.confirmLine", arguments: `["tx-salary"]`, expects: expectsNothing, state: lineVersions},
	{method: "transactions.confirmLine", arguments: `["no-such-tx"]`, expects: expectsNothing, state: lineVersions},
	{method: "transactions.setLine", arguments: `["tx-pending","Mercado"]`, expects: expectsRefusal, reason: reasonUnknownLine, state: lineVersions},
	{method: "transactions.setLineForMany", arguments: `[["tx-pending","no-such-tx"],"Feira"]`, expects: expectsRefusal,
		reason: reasonNotFound, state: lineVersions},
	{method: "transactions.setLineForMany", arguments: `[["tx-pending","tx-month-end"],"Feira"]`, expects: expectsRefusal,
		reason: reasonUnknownLine, state: lineVersions},
	{method: "transactions.applyLineToEstablishment", arguments: `["999"]`, expects: expectsRefusal, reason: reasonStaleChange, state: lineVersions},
	{method: "transactions.applyLineToEstablishment", arguments: `["4"]`, expects: expectsRefusal, reason: reasonNotFound, state: lineVersions,
		before: []writeCall{{"transactions.setLine", `["tx-pix-out","Feira"]`}}},
}

// newLineEnv is a router with every handler of the server over fixture and an
// app's database holding what states write, with a session open.
func newLineEnv(t *testing.T, fixture *ledgertest.Fixture, all ...func(t *testing.T, scratch *authoredtest.Scratch)) *writeEnv {
	t.Helper()
	scratch := authoredtest.Open(t, fixtureClock)
	env := newTestEnv(t, withLedger(fixture.Path, fixtureNow), withDatabase(scratch.Database))
	states(all...)(t, scratch)
	return &writeEnv{testEnv: env, scratch: scratch, cookie: env.unlock()}
}

// panelSeen is what the line writes are checked by in a transaction's panel.
type panelSeen struct {
	Line         *string             `json:"line"`
	ClassifiedBy *string             `json:"classifiedBy"`
	Confidence   *int                `json:"confidence"`
	Pending      *pendingReview      `json:"pending"`
	AskReason    bool                `json:"askReason"`
	Reason       string              `json:"reason"`
	Counterparty *counterpartyDetail `json:"counterparty"`
	History      []historyEntry      `json:"history"`
}

func (env *writeEnv) panelOf(id string) panelSeen {
	env.t.Helper()
	response := env.post("transactions.detail", `["`+id+`"]`)
	var seen panelSeen
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &seen) != nil {
		env.t.Fatalf("the panel of %s answered %d %s", id, response.Code, response.Body.String())
	}
	return seen
}

// storedAbout is what the database keeps about the transaction a provider id
// names, nothing for one the index never met.
func (env *writeEnv) storedAbout(id string) authored.AuthoredTransaction {
	env.t.Helper()
	view := env.scratch.Database.View()
	defer view.Close()
	keys, err := view.TransactionKeys(env.t.Context(), []string{id})
	if err != nil {
		env.t.Fatal(err)
	}
	key, indexed := keys[id]
	if !indexed {
		return authored.AuthoredTransaction{}
	}
	kept, err := view.Transactions(env.t.Context(), []authored.TxKey{key})
	if err != nil {
		env.t.Fatal(err)
	}
	return kept[key]
}

func (env *writeEnv) lineChange(id, line string) lineChangeResult {
	env.t.Helper()
	response := env.post("transactions.setLine", `["`+id+`","`+line+`"]`)
	var answer lineChangeResult
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &answer) != nil || answer.ChangeID == "" {
		env.t.Fatalf("setLine %s %s answered %d %s", id, line, response.Code, response.Body.String())
	}
	return answer
}

func (env *writeEnv) applyToEstablishment(id string) establishmentLineResult {
	env.t.Helper()
	response := env.post("transactions.applyLineToEstablishment", `["`+id+`"]`)
	var answer establishmentLineResult
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &answer) != nil {
		env.t.Fatalf("applyLineToEstablishment %s answered %d %s", id, response.Code, response.Body.String())
	}
	if problem := conformanceProblem("transactions.applyLineToEstablishment", response.Body.Bytes()); problem != "" {
		env.t.Errorf("the answer does not conform: %s", problem)
	}
	return answer
}

func lineIs(state authored.TransactionState, line authored.LineID) bool {
	return state.Line != nil && *state.Line == line
}

func textIs(text *string, want string) bool { return text != nil && *text == want }

func TestSetLineNamesTheLineInTheTransactionsOwnMonth(t *testing.T) {
	env := newLineEnv(t, newFullLedger(t), lineVersions)
	authoredOver(t, env.scratch, newFullLedger(t), bakeryNickname)
	answer := env.lineChange(txPending, "Feira")
	if answer.Establishment == nil || *answer.Establishment != (establishmentOfChange{Key: keyOfTheBakery, Name: nicknameOfTheBakery}) {
		t.Errorf("the establishment of the change is %+v, want the bakery under its nickname", answer.Establishment)
	}
	if stored := env.storedAbout(txPending).State; !lineIs(stored, marketLine) || stored.ClassifiedBy != authored.AuthorPerson {
		t.Errorf("the stored state is %+v, want line %d by the person", stored, marketLine)
	}
	panel := env.panelOf(txPending)
	if !textIs(panel.Line, "Feira") || !textIs(panel.ClassifiedBy, classifiedByPerson) || panel.Confidence != nil || panel.Pending != nil {
		t.Errorf("the panel reads %+v", panel)
	}
	if len(panel.History) != 1 || panel.History[0].Text != "Você alterou para Feira" || panel.History[0].Day != "2026-03-20" || !panel.History[0].Undoable {
		t.Errorf("the history is %+v", panel.History)
	}
	expectRefused(t, "Mercado in 2026-03", env.post("transactions.setLine", `["tx-pending","Mercado"]`), "unknown_line")
	env.lineChange(txMonthEnd, "Mercado")
	if stored := env.storedAbout(txMonthEnd).State; !lineIs(stored, marketLine) {
		t.Errorf("Mercado in 2026-02 stored %+v", stored)
	}
	expectRefused(t, "Feira in 2026-02", env.post("transactions.setLine", `["tx-month-end","Feira"]`), "unknown_line")
}

func TestSetLineAnswersNullWhenThereIsNothingToDo(t *testing.T) {
	env := newLineEnv(t, newFullLedger(t), lineVersions)
	env.lineChange(txPending, "Feira")
	env.lineChange(txMonthEnd, "Mercado")
	authoredOver(t, env.scratch, newFullLedger(t), reviewsOfTheFullLedger)
	before := env.scratch.Journal(t)
	for _, arguments := range []string{`["no-such-tx","Feira"]`, `["tx-foreign-tax","Feira"]`, `["tx-pot-transfer","Feira"]`, `["tx-pending","Feira"]`} {
		if response := env.post("transactions.setLine", arguments); response.Code != http.StatusOK || response.Body.String() != "null" {
			t.Errorf("setLine %s answered %d %s, want null", arguments, response.Code, response.Body.String())
		}
	}
	if after := env.scratch.Journal(t); after != before {
		t.Errorf("an answer of null wrote to the journal:\n%s", differences(before, after))
	}
	// The line the month-end purchase is on is called Feira now, and Mercado in
	// its own month: the name is resolved before it is compared.
	expectRefused(t, "the current month's name of the line it is on", env.post("transactions.setLine", `["tx-month-end","Feira"]`), "unknown_line")

	market := marketLine
	authoredOver(t, env.scratch, newFullLedger(t), func(a authoring) error {
		return a.state(txBoleto, authored.TransactionState{Line: &market, ClassifiedBy: authored.AuthorPerson, PendingReason: "Estabelecimento novo"})
	})
	env.lineChange(txBoleto, "Feira")
	if stored := env.storedAbout(txBoleto).State; stored.PendingReason != "" || !lineIs(stored, marketLine) || !stored.AskReason {
		t.Errorf("the same line with a review waiting left %+v", stored)
	}
}

func TestSetLineAsksWhyOnlyWhenItCorrectsTheModelOrSettlesAReview(t *testing.T) {
	env := newLineEnv(t, newFullLedger(t), lineVersions)
	delivery := deliveryLine
	authoredOver(t, env.scratch, newFullLedger(t), func(a authoring) error {
		return each(
			func() error { return reviewsOfTheFullLedger(a) },
			func() error {
				return a.state(txInstalment, authored.TransactionState{Line: &delivery, ClassifiedBy: authored.AuthorPerson})
			},
			func() error {
				return a.state(txFee, authored.TransactionState{Line: &delivery, ClassifiedBy: authored.AuthorRule})
			},
		)
	})
	for id, want := range map[string]bool{txBoleto: true, txForeign: true, txInstalment: false, txFee: false, txSalary: false} {
		env.lineChange(id, "Feira")
		if asks := env.panelOf(id).AskReason; asks != want {
			t.Errorf("%s: askReason is %v, want %v", id, asks, want)
		}
	}
	for _, id := range []string{txPixOut, txOwnTransfer} {
		if answer := env.lineChange(id, "Feira"); answer.Establishment != nil {
			t.Errorf("%s: the change offers %+v to the whole establishment", id, answer.Establishment)
		}
		if stored := env.storedAbout(id).State; !lineIs(stored, marketLine) {
			t.Errorf("%s was not put on the line: %+v", id, stored)
		}
	}
}

// A month before the first version is read with the first, so a month whose
// budget has no line is one whose version holds none, or one of a database
// with no budget at all.
func TestTheFeesLineNeedsAMonthWhoseBudgetHasALine(t *testing.T) {
	env := newLineEnv(t, newFullLedger(t), budgetState(), func(t *testing.T, scratch *authoredtest.Scratch) {
		authoredtest.PutVersion(t, scratch.Database, fixtureNow, "2026-01")
	})
	env.lineChange(txPending, authored.FeesLineName)
	if stored := env.storedAbout(txPending).State; !lineIs(stored, feesLineID) {
		t.Errorf("the fees line stored %+v", stored)
	}
	for _, line := range []string{authored.FeesLineName, "Salário"} {
		expectRefused(t, line+" in a month whose budget has no line", env.post("transactions.setLine", `["tx-stale","`+line+`"]`), "unknown_line")
	}
	bare := newLineEnv(t, newFullLedger(t))
	expectRefused(t, "the fees line without a budget", bare.post("transactions.setLine", `["tx-pending","Tarifas e juros"]`), "unknown_line")
}

func TestConfirmLineSettlesOnTheLineOrTheSuggestion(t *testing.T) {
	env := newLineEnv(t, newFullLedger(t), lineVersions)
	authoredOver(t, env.scratch, newFullLedger(t), func(a authoring) error {
		return each(
			func() error { return reviewsOfTheFullLedger(a) },
			func() error {
				return a.state(txBoleto, authored.TransactionState{
					PendingReason: "Confiança abaixo do limite", PendingLine: authoredtest.Some(marketLine), AskReason: true,
				})
			},
		)
	})
	for id, want := range map[string]string{txBoleto: "Feira", txForeign: "Delivery"} {
		changeIDOfAnswer(t, env.post("transactions.confirmLine", `["`+id+`"]`))
		panel := env.panelOf(id)
		if !textIs(panel.Line, want) || !textIs(panel.ClassifiedBy, classifiedByPerson) || panel.Pending != nil || panel.Confidence != nil {
			t.Errorf("%s after confirming reads %+v", id, panel)
		}
		if len(panel.History) != 1 || panel.History[0].Text != "Você confirmou "+want {
			t.Errorf("%s: the history is %+v", id, panel.History)
		}
	}
	if !env.panelOf(txBoleto).AskReason {
		t.Error("confirming closed the question of why")
	}
	again := changeIDOfAnswer(t, env.post("transactions.confirmLine", `["tx-boleto"]`))
	if effects := env.scratch.EffectsOf(t, changeNumber(t, again)); len(effects) != 1 || effects[0].Table != "history_steps" {
		t.Errorf("confirming what is already settled recorded %+v, want the step alone", effects)
	}
	before := env.scratch.Journal(t)
	for _, id := range []string{txSalary, "no-such-tx"} {
		if response := env.post("transactions.confirmLine", `["`+id+`"]`); response.Code != http.StatusOK || response.Body.String() != "null" {
			t.Errorf("confirmLine %s answered %d %s, want null", id, response.Code, response.Body.String())
		}
	}
	if after := env.scratch.Journal(t); after != before {
		t.Errorf("an answer of null wrote to the journal:\n%s", differences(before, after))
	}
}

func TestSetLineForManyIsAllOrNothing(t *testing.T) {
	env := newLineEnv(t, newFullLedger(t), lineVersions)
	authoredOver(t, env.scratch, newFullLedger(t), reviewsOfTheFullLedger)
	before := env.scratch.Canonical(t)
	historyBefore := len(env.panelOf(txBoleto).History) + len(env.panelOf(txForeign).History)
	id := changeIDOfAnswer(t, env.post("transactions.setLineForMany", `[["tx-boleto","tx-foreign","tx-boleto"],"Feira"]`))
	columns := map[string]int{}
	for _, effect := range env.scratch.EffectsOf(t, changeNumber(t, id)) {
		columns[effect.Key+" "+effect.Column]++
	}
	for unit, count := range columns {
		if count != 1 {
			t.Errorf("%s was written %d times", unit, count)
		}
	}
	for _, tx := range []string{txBoleto, txForeign} {
		stored := env.storedAbout(tx).State
		if !lineIs(stored, marketLine) || stored.ClassifiedBy != authored.AuthorPerson || stored.PendingReason != "" || stored.Confidence != nil || stored.AskReason {
			t.Errorf("%s after the bulk write: %+v", tx, stored)
		}
	}
	if history := len(env.panelOf(txBoleto).History) + len(env.panelOf(txForeign).History); history != historyBefore {
		t.Errorf("the bulk write added %d steps", history-historyBefore)
	}
	expectUndone(t, "the bulk write", env.undo(id))
	if after := env.scratch.Canonical(t); after != before {
		t.Errorf("undoing the bulk write left:\n%s", differences(before, after))
	}

	everything := env.scratch.Everything(t)
	for arguments, reason := range map[string]string{
		`[["tx-boleto","no-such-tx"],"Feira"]`:   "not_found",
		`[["tx-boleto","tx-month-end"],"Feira"]`: "unknown_line",
		// Every id is resolved before any line: the unknown one is found first.
		`[["tx-month-end","no-such-tx"],"Feira"]`: "not_found",
	} {
		expectRefused(t, arguments, env.post("transactions.setLineForMany", arguments), reason)
	}
	if after := env.scratch.Everything(t); after != everything {
		t.Errorf("a refused bulk write changed the file:\n%s", differences(everything, after))
	}
	empty := changeIDOfAnswer(t, env.post("transactions.setLineForMany", `[[],"Feira"]`))
	if effects := env.scratch.EffectsOf(t, changeNumber(t, empty)); len(effects) != 0 {
		t.Errorf("an empty list recorded %+v", effects)
	}
}

// newWarehouseLedger holds one establishment with many card rows, every one
// invented: two purchases in 2026-03 besides the one a test starts from, one in
// 2026-02 and one in 2026-01, two ordinary credits, a refund, a bill paid under
// its name, a purchase excluded, one split, one in euros with no value in
// reais and one already on the line.
func newWarehouseLedger(t testing.TB) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-atacado", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddAccount(ledgertest.Account{ID: "card-atacado", ItemID: "item-atacado", Type: "CREDIT", Subtype: "CREDIT_CARD",
		Name: "Cartão Exemplo", Currency: "BRL", Balance: 900, Credit: map[string]any{"creditLimit": 5000, "availableCreditLimit": 4100}})
	for _, row := range warehouseRows {
		f.AddTransaction(ledgertest.Transaction{ID: row.id, AccountID: "card-atacado", Date: utc(2026, row.month, row.day, 15, 0),
			Description: "ATACADO EXEMPLO", Amount: row.amount, Currency: row.currency, Type: row.kind,
			Raw: map[string]any{"operationType": row.operation, "creditCardMetadata": map[string]any{"cardNumber": "5555666677778888"}}})
	}
	return f
}

const (
	warehouseOrigin    = "tx-atacado-origem"
	warehouseMarch     = "tx-atacado-marco"
	warehouseFebruary  = "tx-atacado-fevereiro"
	warehouseJanuary   = "tx-atacado-janeiro"
	warehouseCredit    = "tx-atacado-credito"
	warehouseCreditToo = "tx-atacado-credito-2"
	warehouseRefund    = "tx-atacado-estorno"
	warehouseBill      = "tx-atacado-fatura"
	warehouseExcluded  = "tx-atacado-excluida"
	warehouseSplit     = "tx-atacado-dividida"
	warehouseForeign   = "tx-atacado-exterior"
	warehouseOnLine    = "tx-atacado-na-linha"
)

var warehouseRows = []struct {
	id                        string
	month                     time.Month
	day                       int
	amount                    float64
	kind, operation, currency string
}{
	{warehouseOrigin, time.March, 5, 120, "DEBIT", "PAGAMENTO", "BRL"},
	{warehouseMarch, time.March, 10, 80, "DEBIT", "PAGAMENTO", "BRL"},
	{warehouseFebruary, time.February, 10, 95, "DEBIT", "PAGAMENTO", "BRL"},
	{warehouseJanuary, time.January, 10, 60, "DEBIT", "PAGAMENTO", "BRL"},
	{warehouseCredit, time.March, 12, 40, "CREDIT", "PAGAMENTO", "BRL"},
	{warehouseCreditToo, time.March, 13, 25, "CREDIT", "PAGAMENTO", "BRL"},
	{warehouseRefund, time.March, 14, 30, "CREDIT", "ESTORNO", "BRL"},
	{warehouseBill, time.March, 15, 500, "CREDIT", "PAGAMENTO_FATURA", "BRL"},
	{warehouseExcluded, time.March, 16, 70, "DEBIT", "PAGAMENTO", "BRL"},
	{warehouseSplit, time.March, 17, 150, "DEBIT", "PAGAMENTO", "BRL"},
	{warehouseForeign, time.March, 18, 15, "DEBIT", "PAGAMENTO", "EUR"},
	{warehouseOnLine, time.March, 19, 45, "DEBIT", "PAGAMENTO", "BRL"},
}

// warehouseAuthorship is what was said about the warehouse: its nickname, the
// March purchase waiting for a review, the February one on Delivery by the
// model, one purchase excluded with a reason, one split in two, and one on the
// line already.
func warehouseAuthorship(a authoring) error {
	market, delivery := marketLine, deliveryLine
	splitIn := func() error {
		key, err := a.key(warehouseSplit)
		if err != nil {
			return err
		}
		return authoredtest.WriteSplit(a.ctx, a.writes.Recorder, key, []authored.SplitPart{
			{Position: 0, Line: &delivery, Cents: 10_000}, {Position: 1, Cents: 5_000},
		})
	}
	return each(
		func() error {
			return authoredtest.WriteEstablishment(a.ctx, a.writes.Recorder, keyOfTheWarehouse, nicknameOfTheWarehouse, nil)
		},
		func() error {
			return a.state(warehouseMarch, authored.TransactionState{
				PendingReason: "Confiança abaixo do limite", PendingLine: &delivery, PendingConfidence: authoredtest.Some[int64](60),
			})
		},
		func() error {
			return a.state(warehouseFebruary, authored.TransactionState{Line: &delivery, ClassifiedBy: authored.AuthorModel, Confidence: authoredtest.Some[int64](80)})
		},
		func() error {
			return a.state(warehouseExcluded, authored.TransactionState{Exclusion: authoredtest.Some("Presente")})
		},
		splitIn,
		func() error {
			return a.state(warehouseOnLine, authored.TransactionState{Line: &market, ClassifiedBy: authored.AuthorPerson})
		},
	)
}

func newWarehouseEnv(t *testing.T, budgets func(t *testing.T, scratch *authoredtest.Scratch)) (*writeEnv, *ledgertest.Fixture) {
	t.Helper()
	fixture := newWarehouseLedger(t)
	env := newLineEnv(t, fixture, budgets)
	authoredOver(t, env.scratch, fixture, warehouseAuthorship)
	return env, fixture
}

func TestAllPurchasesHereMovesWhatCanGoOnTheLine(t *testing.T) {
	env, _ := newWarehouseEnv(t, lineVersions)
	untouched := map[string]authored.AuthoredTransaction{}
	for _, id := range []string{warehouseJanuary, warehouseCredit, warehouseCreditToo, warehouseRefund, warehouseBill, warehouseExcluded, warehouseSplit, warehouseForeign, warehouseOnLine} {
		untouched[id] = env.storedAbout(id)
	}
	before := env.scratch.Canonical(t)
	origin := env.lineChange(warehouseOrigin, "Feira")
	if origin.Establishment == nil || *origin.Establishment != (establishmentOfChange{Key: keyOfTheWarehouse, Name: nicknameOfTheWarehouse}) {
		t.Fatalf("the line change offers %+v", origin.Establishment)
	}
	answer := env.applyToEstablishment(origin.ChangeID)
	if answer != (establishmentLineResult{ChangeID: origin.ChangeID, Changed: 2}) {
		t.Errorf("all purchases here answered %+v, want the origin's id and 2", answer)
	}
	for id, line := range map[string]string{warehouseMarch: "Feira", warehouseFebruary: "Mercado"} {
		panel := env.panelOf(id)
		want := "Regra aplicou " + line + " (você pediu para todas em " + nicknameOfTheWarehouse + ")"
		if !textIs(panel.ClassifiedBy, classifiedByRule) || panel.Pending != nil || panel.Confidence != nil || !textIs(panel.Line, line) {
			t.Errorf("%s after all purchases here reads %+v", id, panel)
		}
		if len(panel.History) != 1 || panel.History[0].Text != want || !panel.History[0].Undoable || panel.History[0].Day != "2026-03-20" {
			t.Errorf("%s: the history is %+v, want one undoable step %q", id, panel.History, want)
		}
	}
	for id, was := range untouched {
		if now := env.storedAbout(id); !sameKept(now, was) {
			t.Errorf("%s moved: %+v, was %+v", id, now, was)
		}
	}
	if panel := env.panelOf(warehouseOrigin); panel.Counterparty == nil || !textIs(panel.Counterparty.DefaultLine, "Feira") {
		t.Errorf("the default line reads %+v", panel.Counterparty)
	}
	expectUndone(t, "the extended change", env.undo(origin.ChangeID))
	if after := env.scratch.Canonical(t); after != before {
		t.Errorf("one undo of the extended change left:\n%s", differences(before, after))
	}
}

func sameKept(a, b authored.AuthoredTransaction) bool {
	first, _ := json.Marshal(a)
	second, _ := json.Marshal(b)
	return string(first) == string(second)
}

func TestAllPurchasesHereOverACreditMovesOnlyCredits(t *testing.T) {
	env, _ := newWarehouseEnv(t, lineVersions)
	origin := env.lineChange(warehouseCredit, "Feira")
	if answer := env.applyToEstablishment(origin.ChangeID); answer.Changed != 1 || !lineIs(env.storedAbout(warehouseCreditToo).State, marketLine) {
		t.Errorf("over a credit, all purchases here answered %+v and left the other credit at %+v", answer, env.storedAbout(warehouseCreditToo).State)
	}
	for _, id := range []string{warehouseRefund, warehouseBill, warehouseMarch, warehouseOrigin} {
		if lineIs(env.storedAbout(id).State, marketLine) {
			t.Errorf("%s was moved by a credit", id)
		}
	}

	again, _ := newWarehouseEnv(t, lineVersions)
	again.lineChange(warehouseCreditToo, "Feira")
	origin = again.lineChange(warehouseCredit, "Feira")
	if answer := again.applyToEstablishment(origin.ChangeID); answer.Changed != 0 {
		t.Errorf("with every other credit on the line, all purchases here answered %+v", answer)
	}
	if panel := again.panelOf(warehouseCredit); panel.Counterparty == nil || !textIs(panel.Counterparty.DefaultLine, "Feira") {
		t.Errorf("moving nothing did not store the default line: %+v", panel.Counterparty)
	}
}

func TestAllPurchasesHereStoresTheDefaultLineEvenWhenTheCurrentMonthLacksIt(t *testing.T) {
	env, _ := newWarehouseEnv(t, lineVersionsWithStationery)
	origin := env.lineChange(warehouseFebruary, stationeryLine)
	if answer := env.applyToEstablishment(origin.ChangeID); answer.Changed != 0 {
		t.Errorf("a line only February has moved %d transactions", answer.Changed)
	}
	view := env.scratch.Database.View()
	defer view.Close()
	kept, err := view.Establishments(t.Context(), []string{keyOfTheWarehouse})
	stationery := budgetIn(t, env.scratch.Database, "2026-02").Lines
	if err != nil || kept[keyOfTheWarehouse].DefaultLine == nil || !hasLine(stationery, *kept[keyOfTheWarehouse].DefaultLine, stationeryLine) {
		t.Errorf("the default line stored is %+v, %v", kept[keyOfTheWarehouse], err)
	}
	if panel := env.panelOf(warehouseFebruary); panel.Counterparty == nil || panel.Counterparty.DefaultLine != nil {
		t.Errorf("the panel names a default line the current month lacks: %+v", panel.Counterparty)
	}
}

func hasLine(lines []authored.BudgetLine, id authored.LineID, name string) bool {
	for _, line := range lines {
		if line.ID == id && line.Name == name {
			return true
		}
	}
	return false
}

func TestAllPurchasesHereRefusesAnOriginThatIsNotALiveLineChange(t *testing.T) {
	env := newLineEnv(t, newFullLedger(t), lineVersions)
	undone := env.lineChange(txInstalment, "Feira").ChangeID
	expectUndone(t, "the line change", env.undo(undone))
	bulk := changeIDOfAnswer(t, env.post("transactions.setLineForMany", `[["tx-boleto"],"Feira"]`))
	extended := env.lineChange(txPending, "Feira").ChangeID
	env.applyToEstablishment(extended)
	overwritten := env.lineChange(txMonthEnd, "Mercado").ChangeID
	env.lineChange(txMonthEnd, "Delivery")
	person := env.lineChange(txPixOut, "Feira").ChangeID
	movement := env.lineChange(txOwnTransfer, "Feira").ChangeID

	everything := env.scratch.Everything(t)
	for label, refusal := range map[string]struct{ id, reason string }{
		"an unknown change":               {"999", "stale_change"},
		"a change that is not a number":   {"nope", "stale_change"},
		"an undone line change":           {undone, "stale_change"},
		"a bulk change":                   {bulk, "stale_change"},
		"a change already extended":       {extended, "stale_change"},
		"a line change overwritten since": {overwritten, "stale_change"},
		"a line change of a person":       {person, "not_found"},
		"a line change of a movement":     {movement, "not_found"},
	} {
		expectRefused(t, label, env.post("transactions.applyLineToEstablishment", `["`+refusal.id+`"]`), refusal.reason)
	}
	if after := env.scratch.Everything(t); after != everything {
		t.Errorf("a refused extension changed the file:\n%s", differences(everything, after))
	}

	noted := env.lineChange(txInstalment, "Feira").ChangeID
	setNoteOf(t, env, txInstalment, "Livro da escola")
	env.applyToEstablishment(noted)
}

// setNoteOf writes a transaction's note in a change of its own, as the note's
// write will.
func setNoteOf(t *testing.T, env *writeEnv, id, note string) {
	t.Helper()
	authoredOver(t, env.scratch, newFullLedger(t), func(a authoring) error {
		key, err := a.key(id)
		if err != nil {
			return err
		}
		return a.writes.Recorder.Set(a.ctx, "tx_state", []authored.Value{authored.Integer(int64(key))}, authored.Row{"note": authored.Text(note)})
	})
}

func TestALineWriteOverAMissingSnapshotAnswersLedgerUnavailable(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	lineVersions(t, scratch)
	env := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "missing.db"), fixtureNow), withDatabase(scratch.Database))
	cookie := env.unlock()
	everything := scratch.Everything(t)
	for method, arguments := range map[string]string{
		"setLine": `["tx-pending","Feira"]`, "confirmLine": `["tx-pending"]`, "setLineForMany": `[["tx-pending"],"Feira"]`,
	} {
		expectError(t, method, env.call("/api/transactions/"+method, arguments, cookie), http.StatusServiceUnavailable, "ledger_unavailable")
	}
	if after := scratch.Everything(t); after != everything {
		t.Errorf("a write over a missing snapshot changed the file:\n%s", differences(everything, after))
	}
}
