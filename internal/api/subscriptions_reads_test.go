package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Ledgers built for these tests: one card and one bank account of an invented
// bank, and the rows each test adds. Everything is invented.

const (
	subscriptionsCard = "card-sub"
	subscriptionsBank = "acc-sub"
)

// subscriptionsNow is noon of 22 November 2026 in Brazil.
var subscriptionsNow = utc(2026, time.November, 22, 15, 0)

func newSubscriptionsLedger(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	fresh := utc(2026, time.November, 22, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-sub", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddAccount(ledgertest.Account{ID: subscriptionsCard, ItemID: "item-sub", Type: "CREDIT", Subtype: "CREDIT_CARD",
		Name: "Cartão Exemplo", Currency: "BRL"})
	f.AddAccount(ledgertest.Account{ID: subscriptionsBank, ItemID: "item-sub", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Exemplo", Currency: "BRL"})
	return f
}

// cardRow is a row of the invented card; raw adds to what the bank sent.
func cardRow(f *ledgertest.Fixture, id string, at time.Time, description string, amount float64, kind, operation string, raw map[string]any) {
	body := map[string]any{"operationType": operation, "creditCardMetadata": map[string]any{"cardNumber": "4444333322221111"}}
	for key, value := range raw {
		body[key] = value
	}
	f.AddTransaction(ledgertest.Transaction{ID: id, AccountID: subscriptionsCard, Date: at, Description: description,
		Amount: amount, Currency: "BRL", Type: kind, Raw: body})
}

func cardPurchase(f *ledgertest.Fixture, id string, at time.Time, description string, amount float64) {
	cardRow(f, id, at, description, amount, "DEBIT", "PAGAMENTO", nil)
}

func november(day, hour int) time.Time { return utc(2026, time.November, day, hour, 0) }

func newSubscriptionsEnv(t *testing.T, fixture *ledgertest.Fixture) *settingsEnv {
	t.Helper()
	return newSettingsEnv(t, fixture, subscriptionsNow)
}

// keepSubscription writes a subscription straight through the store, made
// from the purchase providerID names, for a state the writes cannot reach in
// one step, such as a status set on a chosen day.
func keepSubscription(t *testing.T, database *authored.Database, providerID string, subscription authored.Subscription) authored.SubscriptionID {
	t.Helper()
	var id authored.SubscriptionID
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "scratch.keepSubscription", At: subscriptionsNow}
	err := database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		seen := authoredtest.Seen(providerID, subscription.CreatedDay, -subscription.CreatedCents, subscription.EstablishmentKey)
		key, err := recorder.IndexTransaction(t.Context(), seen)
		if err != nil {
			return err
		}
		subscription.CreatedFrom = key
		id, err = recorder.CreateSubscription(t.Context(), subscription)
		return err
	})
	if err != nil {
		t.Fatalf("keep a subscription: %v", err)
	}
	return id
}

func keptSubscription(key, name string, day brday.Day, cents money.Cents, status authored.SubscriptionStatus) authored.Subscription {
	return authored.Subscription{EstablishmentKey: key, CreatedDay: day, CreatedCents: cents, Name: name, Status: status,
		StatusDay: day, ReviewOn: "2027-02-22"}
}

// exclude switches a listed row off, with a reason or, for an empty one,
// without one yet.
func exclude(t *testing.T, database *authored.Database, providerID string, day brday.Day, reason string) {
	t.Helper()
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "scratch.exclude", At: subscriptionsNow}
	err := database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		key, err := recorder.IndexTransaction(t.Context(), authoredtest.Seen(providerID, day, -1, "text:EXCLUIDA"))
		if err != nil {
			return err
		}
		return authoredtest.WriteState(t.Context(), recorder, key, authored.TransactionState{Exclusion: &reason})
	})
	if err != nil {
		t.Fatalf("exclude %s: %v", providerID, err)
	}
}

func candidatesOf(t *testing.T, env *readEnv) []subscriptionCandidate {
	t.Helper()
	status, body := env.answer("subscriptions.candidates", `[]`)
	var candidates []subscriptionCandidate
	if status != http.StatusOK || json.Unmarshal(body, &candidates) != nil {
		t.Fatalf("candidates answered %d %s", status, body)
	}
	if problem := conformanceProblem("subscriptions.candidates", body); problem != "" {
		t.Errorf("the candidates do not conform: %s", problem)
	}
	return candidates
}

func candidateIDs(candidates []subscriptionCandidate) []string {
	ids := make([]string, len(candidates))
	for index, candidate := range candidates {
		ids[index] = candidate.TransactionID
	}
	return ids
}

func TestThirteenEstablishmentsGiveTwelveCandidatesNewestFirst(t *testing.T) {
	fixture := newSubscriptionsLedger(t)
	var want []string
	for index, letter := range "ABCDEFGHIJKLM" {
		id := "tx-loja-" + string(letter)
		cardPurchase(fixture, id, november(index+1, 15), "LOJA "+string(letter), float64(10+index))
		want = append([]string{id}, want...)
	}
	got := candidateIDs(candidatesOf(t, newSubscriptionsEnv(t, fixture).readEnv))
	if !slices.Equal(got, want[:12]) {
		t.Errorf("candidates %v, want %v", got, want[:12])
	}
}

// One ledger holds every reason a purchase is or is not offered. Two
// candidates of one day keep the list's order, the later purchase first,
// whatever their ids.
func TestTheCandidatesLeaveOutWhatCannotBecomeASubscription(t *testing.T) {
	fixture := newSubscriptionsLedger(t)
	cardPurchase(fixture, "tx-gym-b", november(5, 18), "ACADEMIA", 99.9)
	cardPurchase(fixture, "tx-gym-a", november(5, 12), "ACADEMIA", 99.9)
	cardPurchase(fixture, "tx-stream-new", november(10, 15), "STREAMSVC", 39.9)
	cardPurchase(fixture, "tx-stream-old", november(3, 15), "STREAMSVC", 39.9)
	cardPurchase(fixture, "tx-bakery", november(12, 15), "PADARIA", 18.5)
	cardPurchase(fixture, "tx-a-pharmacy", november(12, 10), "FARMACIA", 32.4)
	cardPurchase(fixture, "tx-grocer-early", november(2, 15), "MERCADINHO", 61)
	cardPurchase(fixture, "tx-grocer-late", november(16, 15), "MERCADINHO", 58)
	cardPurchase(fixture, "tx-music", november(8, 15), "MUSICASVC", 21.9)
	cardPurchase(fixture, "tx-magazine", utc(2026, time.October, 30, 15, 0), "REVISTA", 15)
	cardRow(fixture, "tx-annual-fee", november(9, 15), "ANUIDADE", 30, "DEBIT", "TARIFA", nil)
	cardRow(fixture, "tx-paid-bill", november(11, 15), "PAGAMENTO COM SALDO", 200, "DEBIT", "PAGAMENTO", nil)
	cardRow(fixture, "tx-refund", november(13, 15), "ESTORNO LOJA", 40, "CREDIT", "ESTORNO", nil)
	fixture.AddTransaction(ledgertest.Transaction{ID: "tx-euro", AccountID: subscriptionsCard, Date: november(14, 15),
		Description: "HOTELSVC", Amount: 15, Currency: "EUR", Type: "DEBIT",
		Raw: map[string]any{"operationType": "PAGAMENTO", "creditCardMetadata": map[string]any{"cardNumber": "4444333322221111"}}})
	fixture.AddTransaction(ledgertest.Transaction{ID: "tx-bank-debit", AccountID: subscriptionsBank, Date: november(15, 15),
		Description: "LIVRARIA", Amount: -50, Currency: "BRL", Type: "DEBIT", Raw: map[string]any{"operationType": "CARTAO"}})
	env := newSubscriptionsEnv(t, fixture)
	exclude(t, env.scratch.Database, "tx-stream-new", "2026-11-10", "Cobrança em dobro")
	exclude(t, env.scratch.Database, "tx-bakery", "2026-11-12", "")
	keepSubscription(t, env.scratch.Database, "tx-music", keptSubscription("text:MUSICASVC", "MUSICASVC", "2026-11-08", 2_190, authored.StatusKeep))

	got := candidatesOf(t, env.readEnv)
	want := []subscriptionCandidate{
		{TransactionID: "tx-grocer-late", Name: "MERCADINHO", Amount: 5_800},
		{TransactionID: "tx-bakery", Name: "PADARIA", Amount: 1_850},
		{TransactionID: "tx-a-pharmacy", Name: "FARMACIA", Amount: 3_240},
		{TransactionID: "tx-gym-a", Name: "ACADEMIA", Amount: 9_990},
		{TransactionID: "tx-stream-old", Name: "STREAMSVC", Amount: 3_990},
	}
	if !slices.Equal(got, want) {
		t.Errorf("candidates %+v, want %+v", got, want)
	}
}

// subscriptionAnswer is a read's answer as the server wrote it, checked
// against the manifest.
func subscriptionAnswer(t *testing.T, env *readEnv, method, arguments string, into any) {
	t.Helper()
	status, body := env.answer(method, arguments)
	if status != http.StatusOK {
		t.Fatalf("%s %s answered %d %s", method, arguments, status, body)
	}
	if problem := conformanceProblem(method, body); problem != "" {
		t.Errorf("%s %s does not conform: %s", method, arguments, problem)
	}
	if err := json.Unmarshal(body, into); err != nil {
		t.Fatal(err)
	}
}

type listAnswer struct {
	MonthlyTotal json.Number `json:"monthlyTotal"`
	AnnualTotal  json.Number `json:"annualTotal"`
	Groups       []struct {
		Status        *string     `json:"status"`
		MonthlyTotal  json.Number `json:"monthlyTotal"`
		Subscriptions []rowAnswer `json:"subscriptions"`
	} `json:"groups"`
	Suggestion json.RawMessage `json:"suggestion"`
}

type rowAnswer struct {
	ID                string          `json:"id"`
	Amount            json.Number     `json:"amount"`
	Cycle             string          `json:"cycle"`
	NextCharge        string          `json:"nextCharge"`
	MonthlyEquivalent json.Number     `json:"monthlyEquivalent"`
	Line              *string         `json:"line"`
	Status            string          `json:"status"`
	ReviewOverdue     bool            `json:"reviewOverdue"`
	Marks             json.RawMessage `json:"marks"`
}

type detailAnswer struct {
	rowAnswer
	AnnualEquivalent json.Number     `json:"annualEquivalent"`
	Note             string          `json:"note"`
	Charges          json.RawMessage `json:"charges"`
	Alternatives     json.RawMessage `json:"alternatives"`
}

func TestASubscriptionWithSeveralChargeDaysIsWorkedOutFromTheCardAlone(t *testing.T) {
	fixture := newSubscriptionsLedger(t)
	cardPurchase(fixture, "tx-s-aug", utc(2026, time.August, 15, 15, 0), "STREAMSVC", 39.9)
	cardPurchase(fixture, "tx-s-sep", utc(2026, time.September, 15, 15, 0), "STREAMSVC", 39.9)
	fixture.AddTransaction(ledgertest.Transaction{ID: "tx-s-oct", AccountID: subscriptionsCard, Date: utc(2026, time.October, 15, 15, 0),
		Description: "STREAMSVC", Amount: 8, Currency: "USD", Type: "DEBIT",
		Raw: map[string]any{"operationType": "PAGAMENTO", "amountInAccountCurrency": 43.3, "creditCardMetadata": map[string]any{"cardNumber": "4444333322221111"}}})
	cardRow(fixture, "tx-s-oct-tax", utc(2026, time.October, 15, 15, 0), "IOF INTERNACIONAL", 1.6, "DEBIT", "OUTROS", nil)
	cardPurchase(fixture, "tx-s-nov", november(14, 12), "STREAMSVC", 44.9)
	cardPurchase(fixture, "tx-s-nov-extra", november(14, 13), "STREAMSVC", 5)
	cardPurchase(fixture, "tx-s-excluded", november(1, 15), "STREAMSVC", 300)
	fixture.AddTransaction(ledgertest.Transaction{ID: "tx-s-bank", AccountID: subscriptionsBank, Date: november(20, 15),
		Description: "STREAMSVC", Amount: -39.9, Currency: "BRL", Type: "DEBIT", Raw: map[string]any{"operationType": "CARTAO"}})
	env := newSubscriptionsEnv(t, fixture)
	exclude(t, env.scratch.Database, "tx-s-excluded", "2026-11-01", "Compra duplicada")
	cancelled := keptSubscription("text:STREAMSVC", "STREAMSVC", "2026-11-14", 4_490, authored.StatusCancel)
	cancelled.StatusDay = "2026-11-10"
	id := keepSubscription(t, env.scratch.Database, "tx-s-nov", cancelled)

	var detail detailAnswer
	subscriptionAnswer(t, env.readEnv, "subscriptions.detail", `["`+id.String()+`"]`, &detail)
	wantCharges := `[{"day":"2026-11-14","amount":49.90,"foreign":null},` +
		`{"day":"2026-10-15","amount":44.90,"foreign":{"originalAmount":"US$ 8,00","iof":1.60}},` +
		`{"day":"2026-09-15","amount":39.90,"foreign":null}]`
	wantMarks := `[{"text":"Preço subiu","tone":"warning"},{"text":"Cobrou após cancelar","tone":"bad"}]`
	if detail.Cycle != "Mensal" || detail.Amount != "49.90" || detail.NextCharge != "2026-12-14" ||
		detail.MonthlyEquivalent != "49.90" || detail.AnnualEquivalent != "598.80" ||
		string(detail.Charges) != wantCharges || string(detail.Marks) != wantMarks || string(detail.Alternatives) != `[]` {
		t.Errorf("detail %+v\ncharges %s\nmarks %s", detail, detail.Charges, detail.Marks)
	}
	var list listAnswer
	subscriptionAnswer(t, env.readEnv, "subscriptions.list", `[{"grouped":false}]`, &list)
	if len(list.Groups) != 1 || len(list.Groups[0].Subscriptions) != 1 || list.MonthlyTotal != "49.90" || list.AnnualTotal != "598.80" {
		t.Fatalf("list %+v", list)
	}
	row := list.Groups[0].Subscriptions[0]
	if row.Amount != "49.90" || row.NextCharge != "2026-12-14" || string(row.Marks) != wantMarks || string(list.Suggestion) != "null" {
		t.Errorf("row %+v, marks %s", row, row.Marks)
	}
}

// The cycles of the six, the bead's own vector: 202.37 a month and twelve
// times that a year, one group per status in the statuses' order.
func TestTheGroupsFollowTheStatusesOrderAndTheTotalsAddTheRowsAsShown(t *testing.T) {
	fixture := newSubscriptionsLedger(t)
	cardPurchase(fixture, "tx-yearly-old", utc(2025, time.March, 25, 15, 0), "ANTIVIRUS", 80)
	cardPurchase(fixture, "tx-yearly", utc(2026, time.March, 25, 15, 0), "ANTIVIRUS", 80)
	env := newSubscriptionsEnv(t, fixture)
	database := env.scratch.Database
	kept := []struct {
		key    string
		cents  money.Cents
		status authored.SubscriptionStatus
	}{
		{"name:cancelada", 11_000, authored.StatusCancel},
		{"name:mantida", 2_190, authored.StatusKeep},
		{"name:backup", 1_390, authored.StatusBackUpAndCancel},
		{"name:analise", 2_000, authored.StatusReview},
		{"name:mantida-2", 2_990, authored.StatusKeep},
	}
	for _, subscription := range kept {
		keepSubscription(t, database, "tx-"+strings.TrimPrefix(subscription.key, "name:"),
			keptSubscription(subscription.key, subscription.key, "2026-11-01", subscription.cents, subscription.status))
	}
	keepSubscription(t, database, "tx-yearly", keptSubscription("text:ANTIVIRUS", "ANTIVIRUS", "2026-03-25", 8_000, authored.StatusReview))

	var list listAnswer
	subscriptionAnswer(t, env.readEnv, "subscriptions.list", `[{"grouped":true}]`, &list)
	if list.MonthlyTotal != "202.37" || list.AnnualTotal != "2428.44" {
		t.Errorf("totals %s and %s, want 202.37 and 2428.44", list.MonthlyTotal, list.AnnualTotal)
	}
	type group struct {
		status string
		total  json.Number
		ids    []string
	}
	want := []group{{"Manter", "51.80", []string{"2", "5"}}, {"Analisar", "26.67", []string{"4", "6"}},
		{"Backup e cancelar", "13.90", []string{"3"}}, {"Cancelar", "110.00", []string{"1"}}}
	var got []group
	for _, found := range list.Groups {
		var ids []string
		for _, row := range found.Subscriptions {
			ids = append(ids, row.ID)
		}
		got = append(got, group{*found.Status, found.MonthlyTotal, ids})
	}
	if len(got) != len(want) {
		t.Fatalf("groups %+v, want %+v", got, want)
	}
	for index := range want {
		if got[index].status != want[index].status || got[index].total != want[index].total || !slices.Equal(got[index].ids, want[index].ids) {
			t.Errorf("group %d is %+v, want %+v", index, got[index], want[index])
		}
	}
	var flat listAnswer
	subscriptionAnswer(t, env.readEnv, "subscriptions.list", `[{"grouped":false}]`, &flat)
	if len(flat.Groups) != 1 || flat.Groups[0].Status != nil || flat.Groups[0].MonthlyTotal != "202.37" || len(flat.Groups[0].Subscriptions) != 6 {
		t.Errorf("the list not grouped: %+v", flat)
	}
}

func TestASubscriptionWithNoLineOrALineTheMonthLacksShowsNone(t *testing.T) {
	env := newSubscriptionsEnv(t, newSubscriptionsLedger(t))
	database := env.scratch.Database
	authoredtest.SaveBudget(t, database, subscriptionsNow, "2026-11", authoredtest.InventedBudget())
	december := authoredtest.DocumentOf(budgetOf(t, database, "2026-11"))
	december[2].Lines = append(december[2].Lines, authoredtest.Line("Natal", 50_000))
	authoredtest.SaveBudget(t, database, subscriptionsNow, "2026-12", december)
	lines := linesByName(t, database, "2026-12")
	if lines["Delivery"] != linesByName(t, database, "2026-11")["Delivery"] {
		t.Fatal("December's budget does not keep November's lines")
	}

	withLine := keptSubscription("name:com-linha", "Com linha", "2026-11-01", 1_000, authored.StatusKeep)
	withLine.Line = authoredtest.Some(lines["Delivery"])
	lacking := keptSubscription("name:linha-de-dezembro", "Linha de dezembro", "2026-11-01", 1_000, authored.StatusKeep)
	lacking.Line = authoredtest.Some(lines["Natal"])
	none := keptSubscription("name:sem-linha", "Sem linha", "2026-11-01", 1_000, authored.StatusKeep)
	ids := []authored.SubscriptionID{
		keepSubscription(t, database, "tx-com", withLine), keepSubscription(t, database, "tx-dez", lacking),
		keepSubscription(t, database, "tx-sem", none),
	}
	want := []*string{authoredtest.Some("Delivery"), nil, nil}

	var list listAnswer
	subscriptionAnswer(t, env.readEnv, "subscriptions.list", `[{"grouped":false}]`, &list)
	for index, id := range ids {
		var detail detailAnswer
		subscriptionAnswer(t, env.readEnv, "subscriptions.detail", `["`+id.String()+`"]`, &detail)
		row := list.Groups[0].Subscriptions[index]
		if !sameLine(detail.Line, want[index]) || !sameLine(row.Line, want[index]) {
			t.Errorf("subscription %s shows the line %v in the detail and %v in the row, want %v", id, detail.Line, row.Line, want[index])
		}
	}
}

func sameLine(got, want *string) bool {
	return (got == nil) == (want == nil) && (got == nil || *got == *want)
}

func budgetOf(t *testing.T, database *authored.Database, month brday.Month) authored.Budget {
	t.Helper()
	view := database.View()
	defer view.Close()
	budget, err := view.BudgetOf(t.Context(), month)
	if err != nil {
		t.Fatal(err)
	}
	return budget
}

func linesByName(t *testing.T, database *authored.Database, month brday.Month) map[string]authored.LineID {
	t.Helper()
	ids := map[string]authored.LineID{}
	for _, line := range budgetOf(t, database, month).Lines {
		ids[line.Name] = line.ID
	}
	return ids
}

func TestTheDetailOfAnIdThatIsNotASubscriptionsIsNull(t *testing.T) {
	env := newSubscriptionsEnv(t, newSubscriptionsLedger(t))
	keepSubscription(t, env.scratch.Database, "tx-um", keptSubscription("name:um", "Um", "2026-11-01", 1_000, authored.StatusKeep))
	expectAnswer(t, env.readEnv, "subscriptions.recurring", `[]`, `[]`)
	for _, id := range []string{"sub-1", "", "01", "+1", "2", "0", "-1", "1 "} {
		expectAnswer(t, env.readEnv, "subscriptions.detail", `["`+id+`"]`, `null`)
	}
	if status, body := env.answer("subscriptions.detail", `["1"]`); status != http.StatusOK || string(body) == "null" {
		t.Errorf("subscription 1 answered %d %s", status, body)
	}
}

func TestWithTheSnapshotMissingTheSubscriptionReadsAreUnavailable(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	keepSubscription(t, scratch.Database, "tx-um", keptSubscription("name:um", "Um", "2026-11-01", 1_000, authored.StatusKeep))
	missing := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "pluggy-snapshot.db"), subscriptionsNow), withDatabase(scratch.Database))
	cookie := missing.unlock()
	for method, arguments := range map[string]string{
		"list": `[{"grouped":true}]`, "detail": `["1"]`, "candidates": `[]`, "recurring": `[]`,
	} {
		expectError(t, method, missing.call("/api/subscriptions/"+method, arguments, cookie), http.StatusServiceUnavailable, "ledger_unavailable")
	}
}
