package api

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// The ledgers of these tests have one bank account and the rows a test names,
// each dated a day of March 2026; every name and document is invented.

const (
	counterpartAccount = "acc-lists"
	// ownDocument is the person's own, so a PIX to it is a transfer between
	// their accounts.
	ownDocument = "98765432100"
)

type counterpartRow struct {
	id          string
	description string
	day         int
	kind        string
	raw         map[string]any
}

func (row counterpartRow) transaction() ledgertest.Transaction {
	amount := -10.0
	if row.kind == "CREDIT" {
		amount = 10
	}
	return ledgertest.Transaction{
		ID: row.id, AccountID: counterpartAccount, Date: utc(2026, time.March, row.day, 15, 0),
		Description: row.description, Amount: amount, Currency: "BRL", Type: row.kind, Raw: row.raw,
	}
}

func shop(day int, description string) counterpartRow {
	return counterpartRow{
		id: fmt.Sprintf("tx-shop-%d-%s", day, description), description: description, day: day, kind: "DEBIT",
		raw: map[string]any{"operationType": "CARTAO"},
	}
}

func pixTo(day int, name, document string) counterpartRow {
	return counterpartRow{
		id: fmt.Sprintf("tx-pix-%d-%s", day, document), description: "PIX ENVIADO " + name, day: day, kind: "DEBIT",
		raw: map[string]any{"operationType": "PIX", "paymentData": payment("PIX", nil, party(name, document))},
	}
}

func boletoTo(day int, name, document string) counterpartRow {
	row := pixTo(day, name, document)
	row.id, row.description = fmt.Sprintf("tx-boleto-%d-%s", day, document), name
	row.raw = map[string]any{"operationType": "BOLETO", "paymentData": payment("BOLETO", nil, party(name, document))}
	return row
}

func ledgerOfRows(t *testing.T, rows ...counterpartRow) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-lists", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddIdentity(ledgertest.Identity{ID: "identity-lists", ItemID: "item-lists", Document: ownDocument, FullName: "Pessoa Exemplo"})
	f.AddAccount(ledgertest.Account{ID: counterpartAccount, ItemID: "item-lists", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Corrente", Currency: "BRL", Balance: 100})
	for _, row := range rows {
		f.AddTransaction(row.transaction())
	}
	return f
}

// listsOf are the establishments and the people the overview answers, each as
// the object the interface receives.
func listsOf(t *testing.T, env *readEnv) (establishments, people []map[string]any) {
	t.Helper()
	overview := settingsOf(t, env)
	return rowsOf(t, overview["establishments"]), rowsOf(t, overview["people"])
}

func rowsOf(t *testing.T, value any) []map[string]any {
	t.Helper()
	list, ok := value.([]any)
	if !ok {
		t.Fatalf("%v is not a list", value)
	}
	rows := make([]map[string]any, 0, len(list))
	for _, entry := range list {
		rows = append(rows, entry.(map[string]any))
	}
	return rows
}

func fieldsOf(rows []map[string]any, field string) []string {
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		values = append(values, fmt.Sprint(row[field]))
	}
	return values
}

// twelveNames are twelve establishments whose names differ in letters, which the
// key keeps, and not in an instalment marker, which it folds away.
var twelveNames = []string{
	"LOJA ALFA", "LOJA BETA", "LOJA GAMA", "LOJA DELTA", "LOJA EPSILON", "LOJA ZETA",
	"LOJA ETA", "LOJA TETA", "LOJA IOTA", "LOJA CAPA", "LOJA LAMBDA", "LOJA MI",
}

func twelvePeople() []counterpartRow {
	var rows []counterpartRow
	for index := range 12 {
		rows = append(rows, pixTo(index+1, fmt.Sprintf("Pessoa %c Exemplo", 'A'+index), fmt.Sprintf("111222333%02d", index)))
	}
	return rows
}

func twelveShops() []counterpartRow {
	var rows []counterpartRow
	for index, name := range twelveNames {
		rows = append(rows, shop(index+1, name))
	}
	return rows
}

func TestOnlyTheTenNewestEstablishmentsAreListedNewestFirst(t *testing.T) {
	establishments, people := listsOf(t, newReadEnv(t, ledgerOfRows(t, twelveShops()...)))
	var want []string
	for index := 11; index >= 2; index-- {
		want = append(want, "text:"+twelveNames[index])
	}
	if got := fieldsOf(establishments, "key"); !slices.Equal(got, want) {
		t.Errorf("the establishments are %v, want the ten newest, newest first: %v", got, want)
	}
	if len(people) != 0 {
		t.Errorf("the people are %v, want none", people)
	}
}

func TestEveryPersonIsListedNewestFirstAndPeopleDoNotUseUpTheEstablishmentsCap(t *testing.T) {
	people := twelvePeople()
	rows := append(twelveShops(), people...)
	for index := range rows[:12] {
		rows[index].day = index + 13
	}
	establishments, listed := listsOf(t, newReadEnv(t, ledgerOfRows(t, rows...)))
	var want []string
	for index := 11; index >= 0; index-- {
		want = append(want, fmt.Sprintf("doc:111222333%02d", index))
	}
	if got := fieldsOf(listed, "key"); !slices.Equal(got, want) {
		t.Errorf("the people are %v, want all twelve, newest first: %v", got, want)
	}
	if len(establishments) != maxEstablishmentsListed {
		t.Errorf("%d establishments are listed, want %d beside twelve people", len(establishments), maxEstablishmentsListed)
	}
	if got := fieldsOf(establishments, "key"); got[0] != "text:LOJA MI" {
		t.Errorf("the newest establishment is %s, want text:LOJA MI", got[0])
	}
}

func TestAKeySeenTwiceIsListedOnceAtItsNewestTransaction(t *testing.T) {
	establishments, _ := listsOf(t, newReadEnv(t, ledgerOfRows(t,
		shop(2, "loja  alfa"), shop(5, "LOJA BETA"), shop(9, "LOJA ALFA"), shop(7, "LOJA GAMA"),
	)))
	want := []string{"text:LOJA ALFA", "text:LOJA GAMA", "text:LOJA BETA"}
	if got := fieldsOf(establishments, "key"); !slices.Equal(got, want) {
		t.Errorf("the establishments are %v, want %v: each once, at the place of its newest transaction", got, want)
	}
	if got := fieldsOf(establishments, "rawText"); got[0] != "LOJA ALFA" {
		t.Errorf("the first one is worded %q, want the newest transaction's wording", got[0])
	}
}

// A movement is neither an establishment nor a person, and one that is the
// newest of its key must not hide the older transactions of that key.
func TestAMovementIsNeverListedAndDoesNotMarkItsKeySeen(t *testing.T) {
	billPaid := shop(11, "PAGAMENTO DE FATURA CARTAO")
	earlierCredit := shop(3, "PAGAMENTO DE FATURA CARTAO")
	earlierCredit.kind = "CREDIT"
	establishments, people := listsOf(t, newReadEnv(t, ledgerOfRows(t,
		billPaid, earlierCredit, shop(6, "LOJA ALFA"),
		pixTo(10, "Pessoa Exemplo", ownDocument), boletoTo(4, "Pessoa Exemplo", ownDocument),
		pixTo(8, "Maria Exemplo", "11122233344"), pixTo(12, "Pessoa Exemplo", ownDocument),
	)))
	if got, want := fieldsOf(establishments, "key"), []string{"text:LOJA ALFA", "text:PAGAMENTO DE FATURA CARTAO"}; !slices.Equal(got, want) {
		t.Errorf("the establishments are %v, want %v: the older transaction of a key whose newest is a movement is listed at its own place", got, want)
	}
	if got, want := fieldsOf(people, "key"), []string{"doc:11122233344", "doc:" + ownDocument}; !slices.Equal(got, want) {
		t.Errorf("the people are %v, want %v", got, want)
	}
}

func TestAKeyOnlyMovementsCarryIsNotListed(t *testing.T) {
	establishments, people := listsOf(t, newReadEnv(t, ledgerOfRows(t,
		shop(11, "PAGAMENTO DE FATURA CARTAO"), pixTo(10, "Pessoa Exemplo", ownDocument), shop(6, "LOJA ALFA"),
	)))
	if got := fieldsOf(establishments, "key"); !slices.Equal(got, []string{"text:LOJA ALFA"}) {
		t.Errorf("the establishments are %v", got)
	}
	if len(people) != 0 {
		t.Errorf("the people are %v, want none: a transfer between own accounts is no person", people)
	}
}

func TestThePixWordsTheBankPutsInFrontAreTakenOffTheWording(t *testing.T) {
	for _, row := range []struct{ raw, want string }{
		{"PIX ENVIADO CAFETERIA EXEMPLO", "CAFETERIA EXEMPLO"},
		{"PIX RECEBIDO EMPRESA EXEMPLO", "EMPRESA EXEMPLO"},
		{"PIX ENVIADO PIX RECEBIDO LOJA", "PIX RECEBIDO LOJA"},
		{"LOJA PIX ENVIADO ALFA", "LOJA PIX ENVIADO ALFA"},
		{"PIX ENVIADOS LOJA", "PIX ENVIADOS LOJA"},
		{"pix enviado loja", "pix enviado loja"},
		{"PIX ENVIADO", "PIX ENVIADO"},
		{"PADARIA EXEMPLO", "PADARIA EXEMPLO"},
	} {
		if got := withoutPixPrefix(row.raw); got != row.want {
			t.Errorf("%q is worded %q, want %q", row.raw, got, row.want)
		}
	}
}

func TestThePixWordsAreTakenOffTheEstablishmentsOfTheFullLedger(t *testing.T) {
	establishments, _ := listsOf(t, newReadEnv(t, newFullLedger(t)))
	wordings := map[string]string{}
	for _, row := range establishments {
		wordings[row["key"].(string)] = row["rawText"].(string)
	}
	for key, want := range map[string]string{"doc:22233344000155": "CAFETERIA EXEMPLO", "doc:12345678000199": "EMPRESA EXEMPLO"} {
		if wordings[key] != want {
			t.Errorf("%s is worded %q, want %q", key, wordings[key], want)
		}
	}
}

// The establishments of the full ledger are the keys of its newest transactions
// that are not movements, once each: the card's payments and the bank's are
// movements, the own transfer is one, and the older transaction of the bakery
// is not a place of its own.
func TestTheFullLedgersEstablishmentsAndPeopleAreInTheOrderOfTheirNewestTransaction(t *testing.T) {
	establishments, people := listsOf(t, newReadEnv(t, newFullLedger(t)))
	wantEstablishments := []string{
		"text:PADARIA EXEMPLO", "text:HOTELSVC EXEMPLO", "text:ESTORNO LIVRARIA EXEMPLO", "text:CLOUDSVC SUBSCRIPTION",
		"doc:55566677000188", "text:LIVRARIA EXEMPLO", "doc:12345678000199", "text:TAR PACOTE SERVICOS",
		"doc:22233344000155", "text:MERCADO EXEMPLO",
	}
	if got := fieldsOf(establishments, "key"); !slices.Equal(got, wantEstablishments) {
		t.Errorf("the establishments are %v, want %v", got, wantEstablishments)
	}
	if got, want := fieldsOf(people, "key"), []string{"doc:11122233344", "doc:44455566677"}; !slices.Equal(got, want) {
		t.Errorf("the people are %v, want %v", got, want)
	}
	if got, want := fieldsOf(people, "fullName"), []string{"Maria Exemplo", "Amigo Exemplo"}; !slices.Equal(got, want) {
		t.Errorf("the people are named %v, want %v", got, want)
	}
	for _, key := range append(fieldsOf(establishments, "key"), fieldsOf(people, "key")...) {
		for _, movement := range []string{"doc:" + ownDocument, "text:PAGAMENTO DE FATURA CARTAO", "text:PAGAMENTO COM SALDO"} {
			if key == movement {
				t.Errorf("%s is the key of a movement and is listed", key)
			}
		}
	}
}

func TestTheListsAreTheSameOnEveryVisit(t *testing.T) {
	env := newReadEnv(t, ledgerOfRows(t, append(twelveShops(), twelvePeople()...)...))
	_, first := env.answer("settings.overview", `[]`)
	for visit := range 8 {
		if _, again := env.answer("settings.overview", `[]`); string(again) != string(first) {
			t.Fatalf("visit %d answered otherwise than the first", visit+2)
		}
	}
}

func TestBothListsAreEmptyArraysOverAnEmptyLedger(t *testing.T) {
	_, body := newReadEnv(t, newEmptyLedger(t)).answer("settings.overview", `[]`)
	var overview struct {
		Establishments []any `json:"establishments"`
		People         []any `json:"people"`
	}
	if err := json.Unmarshal(body, &overview); err != nil || overview.Establishments == nil || overview.People == nil ||
		len(overview.Establishments) != 0 || len(overview.People) != 0 {
		t.Errorf("the lists are %v and %v (%v), want [] and []", overview.Establishments, overview.People, err)
	}
}

// What the person authored is joined to the rows by key, with each default line
// named by the current month's budget.
func TestWhatThePersonSetIsAnsweredOnTheRowsOfItsKeys(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	full := newFullLedger(t)
	lines := lineIDsByName(t, scratch.Database)
	authorOver(t, scratch, viewOf(t, full.Path), lines, firstDecisions, "scratch.seedCounterparts", func(a authoring) error {
		recorder := a.writes.Recorder
		return each(
			func() error {
				return authoredtest.WriteEstablishment(a.ctx, recorder, keyOfTheBakery, "Padaria", a.line("Mercado"))
			},
			func() error { return authoredtest.AddFixedTag(a.ctx, recorder, keyOfTheBakery, "#casa") },
			func() error { return authoredtest.AddFixedTag(a.ctx, recorder, keyOfTheBakery, "#cafe") },
			func() error { return authoredtest.WriteEstablishment(a.ctx, recorder, keyOfTheFriend, "Maria", nil) },
			func() error {
				return authoredtest.WriteEstablishment(a.ctx, recorder, "text:NO LONGER LISTED", "Fantasma", nil)
			},
		)
	})
	env := &settingsEnv{readEnv: newReadEnvOver(t, full, scratch.Database), scratch: scratch}

	establishments, people := listsOf(t, env.readEnv)
	bakery, friend := establishments[0], people[0]
	if bakery["key"] != keyOfTheBakery || bakery["nickname"] != "Padaria" || bakery["defaultLine"] != "Mercado" ||
		!slices.Equal(texts(t, bakery["fixedTags"]), []string{"#casa", "#cafe"}) {
		t.Errorf("the bakery is %v, want its nickname, its line and its tags in the order they were added", bakery)
	}
	if friend["key"] != keyOfTheFriend || friend["nickname"] != "Maria" || friend["fullName"] != "Maria Exemplo" {
		t.Errorf("the person is %v, want the nickname beside the ledger's own name", friend)
	}
	for _, row := range append(establishments, people...) {
		if row["key"] == "text:NO LONGER LISTED" {
			t.Errorf("a key the ledger no longer lists is shown: %v", row)
		}
	}
	if rows := tableRows(t, scratch, "establishments"); len(rows) != 3 {
		t.Errorf("establishments hold %v, want the three rows kept", rows)
	}

	removeLineFromBudget(t, scratch, "Mercado")
	establishments, _ = listsOf(t, env.readEnv)
	if establishments[0]["defaultLine"] != nil {
		t.Errorf("the default line is %v, want null where the current month's budget lacks it", establishments[0]["defaultLine"])
	}
	if rows := tableRows(t, scratch, "establishments"); !slices.ContainsFunc(rows, func(row string) bool {
		return row == fmt.Sprintf("key=%q nickname=%q default_line_id=%d", keyOfTheBakery, "Padaria", lines["Mercado"])
	}) {
		t.Errorf("establishments hold %v, want the stored line id kept", rows)
	}
}

// removeLineFromBudget saves the current month's budget without the line.
func removeLineFromBudget(t *testing.T, scratch *authoredtest.Scratch, name string) {
	t.Helper()
	view := scratch.Database.View()
	current, err := view.BudgetOf(t.Context(), "2026-03")
	view.Close()
	if err != nil {
		t.Fatal(err)
	}
	without := authoredtest.DocumentOf(current)
	for index := range without {
		without[index].Lines = slices.DeleteFunc(without[index].Lines, func(line authored.DocumentLine) bool { return line.Name == name })
	}
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-03", without)
}
