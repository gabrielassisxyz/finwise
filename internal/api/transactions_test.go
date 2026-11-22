package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

func listRaw(month, period string) string {
	return `[{"month":"` + month + `","period":"` + period + `"}]`
}

func transactionsOf(t *testing.T, env *readEnv, month, period string) []map[string]any {
	t.Helper()
	status, body := env.answer("transactions.list", listRaw(month, period))
	if status != http.StatusOK {
		t.Fatalf("transactions.list %s %s answered %d", month, period, status)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func idsOf(rows []map[string]any) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row["id"].(string))
	}
	return ids
}

func contains(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func TestTheMonthIsCutAtMidnightInBrazilNotInUTC(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	february := idsOf(transactionsOf(t, env, "2026-02", "month"))
	march := idsOf(transactionsOf(t, env, "2026-03", "month"))

	if !contains(february, txLastOfFebruary) || contains(march, txLastOfFebruary) {
		t.Errorf("the row at 2026-03-01T02:30:00Z belongs to February only: february %v, in march %t", february, contains(march, txLastOfFebruary))
	}
	if contains(february, txFirstOfMarch) || !contains(march, txFirstOfMarch) {
		t.Errorf("the row at 2026-03-01T03:00:00Z belongs to March only")
	}
}

func TestAPurchaseAndItsTaxAcrossAMonthEndAreOneLineInTheFirstMonth(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	february := transactionsOf(t, env, "2026-02", "month")
	march := transactionsOf(t, env, "2026-03", "month")

	var purchase map[string]any
	for _, row := range february {
		if row["id"] == txMonthEnd {
			purchase = row
		}
	}
	if purchase == nil || purchase["amount"] != -207.0 || purchase["iof"] != 7.0 {
		t.Fatalf("the purchase in February = %v, want -207.00 carrying a tax of 7.00", purchase)
	}
	for label, rows := range map[string][]map[string]any{"february": february, "march": march} {
		if contains(idsOf(rows), txMonthEndTax) {
			t.Errorf("the tax row is a line of its own in %s", label)
		}
	}
	if contains(idsOf(march), txMonthEnd) {
		t.Error("the purchase is in March as well")
	}
}

func TestWithoutWideningTheTaxWouldBeListedAloneInTheNextMonth(t *testing.T) {
	f := newFullLedger(t)
	env := newReadEnv(t, f)
	rows, err := viewOf(t, f.Path).CleanTransactions(t.Context(), brday.Period{First: "2026-03-01", Last: "2026-03-31"})
	withoutValues(t, "read", err)
	around, err := loadContext(t.Context(), viewOf(t, f.Path))
	withoutValues(t, "context", err)
	narrow, err := interpretRows(rows, around, brday.Period{First: "2026-03-01", Last: "2026-03-31"})
	withoutValues(t, "interpret", err)
	var alone bool
	for _, listed := range narrow.listed {
		alone = alone || listed.ID == txMonthEndTax
	}
	if !alone {
		t.Fatal("the control failed: a month read without its neighbours should leave the tax listed")
	}
	if contains(idsOf(transactionsOf(t, env, "2026-03", "month")), txMonthEndTax) {
		t.Error("the served month shows the tax alone")
	}
}

func TestAPurchaseWithItsTaxIsOneLineWhoseDetailCarriesTheTax(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	march := idsOf(transactionsOf(t, env, "2026-03", "month"))
	if !contains(march, txForeign) || contains(march, txForeignTax) {
		t.Errorf("march = %v: want the purchase and not its tax row", march)
	}
	_, body := env.answer("transactions.detail", `["tx-foreign"]`)
	detail := decodeAnswer(t, body)
	if detail["amount"] != -113.85 || detail["iof"] != 3.85 || detail["originalAmount"] != "US$ 20,00" {
		t.Errorf("detail amount %v iof %v original %v", detail["amount"], detail["iof"], detail["originalAmount"])
	}
	status, taxBody := env.answer("transactions.detail", `["tx-foreign-tax"]`)
	if status != http.StatusOK || string(taxBody) != "null" {
		t.Errorf("the tax row answered %d %s, want null", status, taxBody)
	}
}

func TestAPotMovementIsAbsentAndAnOwnTransferIsAMovement(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	march := transactionsOf(t, env, "2026-03", "month")
	if contains(idsOf(march), txPotTransfer) {
		t.Error("the pot movement is listed")
	}
	for _, row := range march {
		if row["id"] == txOwnTransfer && (row["movement"] != true || row["kind"] != "pix" || row["countsInTotals"] != false) {
			t.Errorf("own transfer = kind %v movement %v counts %v", row["kind"], row["movement"], row["countsInTotals"])
		}
	}
	if !contains(idsOf(march), txOwnTransfer) {
		t.Error("the transfer between own accounts is not listed")
	}
	if _, body := env.answer("transactions.detail", `["tx-pot-transfer"]`); string(body) != "null" {
		t.Errorf("the pot movement has a detail: %s", body)
	}
}

func TestAnUnconvertedForeignRowIsListedButKeptOutOfTotals(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	for _, row := range transactionsOf(t, env, "2026-03", "month") {
		if row["id"] != txUnconverted {
			continue
		}
		if row["countsInTotals"] != false || row["originalAmount"] != "€ 15,00" {
			t.Errorf("unconverted row = counts %v original %v", row["countsInTotals"], row["originalAmount"])
		}
		return
	}
	t.Error("the unconverted row is not listed")
}

func TestTheListIsNewestFirstAndStableWithinADay(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	rows := transactionsOf(t, env, "2026-03", "month")
	for index := 1; index < len(rows); index++ {
		if rows[index-1]["day"].(string) < rows[index]["day"].(string) {
			t.Errorf("row %d (%v) is newer than the one before it", index, rows[index]["day"])
		}
	}
	want := []string{txPending, txCardPayment, txBankBill}
	if got := idsOf(rows)[:3]; !reflect.DeepEqual(got, want) {
		t.Errorf("first three = %v, want %v (the ledger's order within a day)", got, want)
	}
}

func TestPeriodsCoverTheMonthTheLastThreeMonthsAndTheYear(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	quarter := idsOf(transactionsOf(t, env, "2026-03", "last-3-months"))
	year := idsOf(transactionsOf(t, env, "2026-03", "year"))
	if !contains(quarter, txStale) && contains(year, txStale) {
		// January is two months before March: the quarter reaches it.
		t.Errorf("last-3-months from March lacks a January row")
	}
	if len(transactionsOf(t, env, "2025-12", "month")) != 0 {
		t.Error("a month with no row is not an empty list")
	}
	if len(year) != len(quarter) {
		t.Errorf("the fixture has only Jan to Mar rows, so year (%d) should equal the quarter (%d)", len(year), len(quarter))
	}
	if len(quarter) <= len(transactionsOf(t, env, "2026-03", "month")) {
		t.Error("the quarter is not wider than the month")
	}
}

func TestEveryListedLineHasADetailThatAgreesWithIt(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	for _, month := range []string{"2026-01", "2026-02", "2026-03"} {
		for _, row := range transactionsOf(t, env, month, "month") {
			_, body := env.answer("transactions.detail", `["`+row["id"].(string)+`"]`)
			detail := decodeAnswer(t, body)
			for field, value := range row {
				if !reflect.DeepEqual(detail[field], value) {
					t.Errorf("%s: %s is %v in the list and %v in the detail", row["id"], field, value, detail[field])
				}
			}
		}
	}
}

func TestTransactionDetailForAnInstalmentCardPurchase(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	_, body := env.answer("transactions.detail", `["tx-instalment"]`)
	detail := decodeAnswer(t, body)
	want := map[string]any{
		"installment": "1/3", "bill": "Abr/26", "accountDescription": "Platinum Visa · final 1234",
		"accountLabel": "Platinum Visa", "note": "", "askReason": false, "justification": nil, "line": nil,
		"classifiedBy": nil, "confidence": nil, "pending": nil, "exclusion": nil, "splitParts": nil,
		"reimbursement": nil, "refundLine": nil, "splitRemaining": 0.0,
	}
	for field, value := range want {
		if detail[field] != value {
			t.Errorf("%s = %v, want %v", field, detail[field], value)
		}
	}
	for _, field := range []string{"history", "tags", "ownTags", "suggestedTags", "reimbursementCandidates"} {
		if list, ok := detail[field].([]any); !ok || len(list) != 0 {
			t.Errorf("%s = %v, want []", field, detail[field])
		}
	}
	sections := detail["sections"].(map[string]any)
	if sections["establishment"] != true || sections["person"] != false || sections["split"] != true || sections["refund"] != false {
		t.Errorf("sections = %v", sections)
	}
	if detail["counterparty"] == nil {
		t.Error("an establishment has no counterparty")
	}
}

func TestTransactionDetailOfABankMovementAndOfAPerson(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	_, body := env.answer("transactions.detail", `["tx-own-transfer"]`)
	movement := decodeAnswer(t, body)
	if movement["counterparty"] != nil || movement["accountDescription"] != "Itaú · Conta Corrente" ||
		movement["sections"].(map[string]any)["person"] != false {
		t.Errorf("own transfer: counterparty %v description %v", movement["counterparty"], movement["accountDescription"])
	}
	_, body = env.answer("transactions.detail", `["tx-pix-out"]`)
	person := decodeAnswer(t, body)
	if person["person"] != true || person["sections"].(map[string]any)["person"] != true {
		t.Errorf("the PIX to a person is not marked as one: %v", person["sections"])
	}
}

func TestTransactionsOverTheEmptyLedgerAreEmptyLists(t *testing.T) {
	env := newReadEnv(t, newEmptyLedger(t))
	for _, c := range readCases {
		if c.method != "transactions.list" {
			continue
		}
		if status, body := env.answer(c.method, c.arguments); status != http.StatusOK || string(body) != "[]" {
			t.Errorf("%s: %d %s", c.arguments, status, body)
		}
	}
	_, body := env.answer("transactions.filterOptions", `[]`)
	if string(body) != `{"accounts":[],"tags":[],"lines":[]}` {
		t.Errorf("filterOptions = %s", body)
	}
}

func TestTransactionDetailIsNullForAnUnknownId(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	if status, body := env.answer("transactions.detail", `["no-such-transaction"]`); status != http.StatusOK || string(body) != "null" {
		t.Errorf("status %d body %s, want 200 and null", status, body)
	}
}

func TestFilterOptionsOfferEveryAccountWithAReadableLabel(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	_, body := env.answer("transactions.filterOptions", `[]`)
	var options struct {
		Accounts []map[string]string `json:"accounts"`
		Tags     []string            `json:"tags"`
		Lines    []any               `json:"lines"`
	}
	if err := json.Unmarshal(body, &options); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, option := range options.Accounts {
		got = append(got, option["value"]+"="+option["label"])
	}
	want := []string{
		"acc-other=Banco não identificado", "acc-stale=C6", "acc-itau=Itaú", "acc-pots=Mercado Pago", "acc-nu=Nubank",
		"card-itau=Platinum Visa",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("accounts = %v, want %v", got, want)
	}
	if len(options.Tags) != 0 || len(options.Lines) != 0 {
		t.Errorf("tags %v lines %v, want none", options.Tags, options.Lines)
	}
}

func TestTwoAccountsThatWouldReadTheSameAreToldApart(t *testing.T) {
	accounts := []ledger.Account{
		{ID: "a", Name: "Conta Corrente", Type: ledger.AccountBank},
		{ID: "b", Name: "Conta Salário", Type: ledger.AccountBank},
		{ID: "c", Name: "Visa", Type: ledger.AccountCredit},
	}
	banks := map[string]string{"a": "Itaú", "b": "Itaú", "c": "Itaú"}
	labels := accountLabelsOf(accounts, banks)
	if labels["a"] != "Itaú · Conta Corrente" || labels["b"] != "Itaú · Conta Salário" || labels["c"] != "Visa" {
		t.Errorf("labels = %v", labels)
	}
}

func TestAQueryThatIsNotWellFormedIsABadRequest(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	for _, arguments := range []string{
		`[{"month":"2026-03","period":"week"}]`,
		`[{"month":"2026-13","period":"month"}]`,
		`[{"month":"2026-3","period":"month"}]`,
		`[{"month":"","period":"month"}]`,
		`[{"month":"2026-03"}]`,
		`[{"month":"2026-03","period":"month","extra":1}]`,
		`["2026-03"]`,
		`[null]`,
	} {
		expectError(t, arguments, env.call("/api/transactions/list", arguments, env.cookie), http.StatusBadRequest, "bad_request")
	}
}

func TestPeriodsOfAQuery(t *testing.T) {
	for _, c := range []struct {
		query       transactionQuery
		first, last brday.Day
	}{
		{transactionQuery{"2026-03", "month"}, "2026-03-01", "2026-03-31"},
		{transactionQuery{"2024-02", "month"}, "2024-02-01", "2024-02-29"},
		{transactionQuery{"2026-03", "last-3-months"}, "2026-01-01", "2026-03-31"},
		{transactionQuery{"2026-01", "last-3-months"}, "2025-11-01", "2026-01-31"},
		{transactionQuery{"2026-03", "year"}, "2026-01-01", "2026-12-31"},
	} {
		got, err := periodOf(c.query)
		if err != nil || got.First != c.first || got.Last != c.last {
			t.Errorf("%+v = %+v (%v), want %s to %s", c.query, got, err, c.first, c.last)
		}
	}
}

func TestWideningAddsTheDaysOnBothSides(t *testing.T) {
	got, err := widened(brday.Period{First: "2026-03-01", Last: "2026-03-31"}, 3)
	if err != nil || got.First != "2026-02-26" || got.Last != "2026-04-03" {
		t.Errorf("widened = %+v (%v)", got, err)
	}
}

func TestACutKeepsOnlyTheDaysOfThePeriod(t *testing.T) {
	listed := []txrules.Transaction{{ID: "before", Day: "2026-02-28"}, {ID: "first", Day: "2026-03-01"},
		{ID: "last", Day: "2026-03-31"}, {ID: "after", Day: "2026-04-01"}}
	kept := within(listed, brday.Period{First: "2026-03-01", Last: "2026-03-31"})
	if len(kept) != 2 || kept[0].ID != "first" || kept[1].ID != "last" {
		t.Errorf("kept %v", kept)
	}
}

func TestARowThatWentNowhereFailsTheRead(t *testing.T) {
	if checkPlaced(txrules.Tally{Input: 3, Listed: 2}, 3) == nil {
		t.Error("an unbalanced tally passed")
	}
	if checkPlaced(txrules.Tally{Input: 2, Listed: 2}, 3) == nil {
		t.Error("a tally of fewer rows than were read passed")
	}
	if checkPlaced(txrules.Tally{Input: 3, Listed: 1, AttachedAsTax: 1, PotMovements: 1}, 3) != nil {
		t.Error("a balanced tally failed")
	}
}

func TestMoneyOnTheWireIsAnExactDecimal(t *testing.T) {
	_, body := newReadEnv(t, newFullLedger(t)).answer("transactions.list", listRaw("2026-03", "month"))
	for _, literal := range []string{`"amount":-113.85`, `"effectiveAmount":-113.85`, `"iof":3.85`, `"amount":5000.00`, `"amount":-109.90`, `"iof":null`} {
		if !strings.Contains(string(body), literal) {
			t.Errorf("the answer lacks %s", literal)
		}
	}
}

func TestAForeignAmountReadsTheBrazilianWay(t *testing.T) {
	for _, c := range []struct {
		original txrules.Original
		want     string
	}{
		{txrules.Original{Amount: 2000, Currency: "USD"}, "US$ 20,00"},
		{txrules.Original{Amount: 123456789, Currency: "EUR"}, "€ 1.234.567,89"},
		{txrules.Original{Amount: 5, Currency: "JPY"}, "JPY 0,05"},
		{txrules.Original{Amount: 100000, Currency: "GBP"}, "£ 1.000,00"},
	} {
		if got := originalAmountOf(&c.original); got == nil || *got != c.want {
			t.Errorf("%+v read as %v, want %q", c.original, got, c.want)
		}
	}
	if originalAmountOf(nil) != nil {
		t.Error("a domestic purchase has an original amount")
	}
}

func TestNewestFirstOrdersByDayAndKeepsTheOrderOfTheDay(t *testing.T) {
	got := newestFirst([]txrules.Transaction{
		{ID: "old", Day: "2026-03-01"}, {ID: "late-a", Day: "2026-03-05"},
		{ID: "mid", Day: "2026-03-03"}, {ID: "late-b", Day: "2026-03-05"},
	})
	var order []string
	for _, listed := range got {
		order = append(order, listed.ID)
	}
	if strings.Join(order, ",") != "late-a,late-b,mid,old" {
		t.Errorf("order %v", order)
	}
}

// The interface opens the panel of every transaction a month lists, so each one needs a recorded answer.
func init() {
	readCases = append(readCases,
		readCase{"transactions.detail", `["tx-card-payment"]`},
		readCase{"transactions.detail", `["tx-bank-bill"]`},
		readCase{"transactions.detail", `["tx-refund"]`},
		readCase{"transactions.detail", `["tx-pix-out"]`},
		readCase{"transactions.detail", `["tx-boleto"]`},
		readCase{"transactions.detail", `["tx-fee"]`},
		readCase{"transactions.detail", `["tx-first-of-march"]`},
	)
}
