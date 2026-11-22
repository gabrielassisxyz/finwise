package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

func detailOver(t *testing.T, env *readEnv, id string) map[string]any {
	t.Helper()
	status, body := env.answer("transactions.detail", `["`+id+`"]`)
	if status != http.StatusOK {
		t.Fatalf("transactions.detail %s answered %d", id, status)
	}
	return decodeAnswer(t, body)
}

func rowWithIDIn(t *testing.T, rows []map[string]any, id string) map[string]any {
	t.Helper()
	for _, row := range rows {
		if row["id"] == id {
			return row
		}
	}
	t.Fatalf("%s is not listed", id)
	return nil
}

// texts reads a JSON list of strings; null reads as nil.
func texts(t *testing.T, value any) []string {
	t.Helper()
	list, ok := value.([]any)
	if !ok {
		t.Fatalf("%v is not a list", value)
	}
	out := []string{}
	for _, item := range list {
		out = append(out, item.(string))
	}
	return out
}

func TestATransactionsTagsAreItsOwnInOrderThenItsEstablishmentsFixedOnesOnce(t *testing.T) {
	env := newAuthoredReadEnv(t)
	detail := detailOver(t, env, txPending)
	if got, want := texts(t, detail["tags"]), []string{"#manhã", "#lanche", "#Trabalho"}; !slices.Equal(got, want) {
		t.Errorf("tags = %v, want %v", got, want)
	}
	if got, want := texts(t, detail["ownTags"]), []string{"#manhã", "#lanche"}; !slices.Equal(got, want) {
		t.Errorf("ownTags = %v, want %v", got, want)
	}
	counterparty := objectAt(t, detail, "counterparty")
	if got, want := texts(t, counterparty["fixedTags"]), []string{"#lanche", "#Trabalho"}; !slices.Equal(got, want) {
		t.Errorf("fixedTags = %v, want %v", got, want)
	}
	listed := rowWithIDIn(t, transactionsOf(t, env, "2026-03", "month"), txPending)
	if !reflect.DeepEqual(listed["tags"], detail["tags"]) {
		t.Errorf("the list's tags %v are not the detail's %v", listed["tags"], detail["tags"])
	}
}

func TestANicknameIsTheNameOfEveryListedTransactionOfItsKeyIndexedOrNot(t *testing.T) {
	env := newAuthoredReadEnv(t)
	nicknamed := 0
	for _, row := range transactionsOf(t, env, "2026-03", "last-3-months") {
		ledgerName := row["originalName"]
		if row["establishmentKey"] != keyOfTheBakery {
			if row["name"] != ledgerName {
				t.Errorf("%s is named %v, though nothing is kept about its key", row["id"], row["name"])
			}
			continue
		}
		nicknamed++
		if row["name"] != nicknameOfTheBakery || ledgerName != "PADARIA EXEMPLO" {
			t.Errorf("%s is named %v with original %v, want %q and the ledger's name", row["id"], row["name"], ledgerName, nicknameOfTheBakery)
		}
	}
	if nicknamed != 2 {
		t.Errorf("%d rows of the bakery are listed, want tx-pending, which is indexed, and tx-last-of-february, which is not", nicknamed)
	}
}

func TestALineIsNamedByTheBudgetOfTheTransactionsOwnMonth(t *testing.T) {
	env := newAuthoredReadEnv(t)
	quarter := transactionsOf(t, env, "2026-03", "last-3-months")
	for id, want := range map[string]string{txMonthEnd: "Mercado", txFirstOfMarch: "Feira", txSalary: "Salário", txFee: authored.FeesLineName} {
		if got := rowWithIDIn(t, quarter, id)["line"]; got != want {
			t.Errorf("%s is on %v, want %q", id, got, want)
		}
	}
	if got := rowWithIDIn(t, transactionsOf(t, env, "2026-02", "month"), txMonthEnd)["line"]; got != "Mercado" {
		t.Errorf("in February's own list the purchase is on %v, want Mercado", got)
	}
}

func TestADefaultLineIsNamedByTheCurrentMonthsBudgetAndIsNullWhereItLacksIt(t *testing.T) {
	env := newAuthoredReadEnv(t)
	for id, want := range map[string]any{txMonthEnd: "Feira", txBoleto: "Contas da casa", txPending: nil, txLastOfFebruary: nil} {
		counterparty := objectAt(t, detailOver(t, env, id), "counterparty")
		if counterparty["defaultLine"] != want {
			t.Errorf("%s: defaultLine = %v, want %v", id, counterparty["defaultLine"], want)
		}
	}
}

func historyOfDetail(t *testing.T, detail map[string]any) []map[string]any {
	t.Helper()
	var steps []map[string]any
	for _, entry := range detail["history"].([]any) {
		steps = append(steps, entry.(map[string]any))
	}
	return steps
}

func TestAStepIsUndoableWhileNothingChangedWhatItDidToItsTransactionAndANoteNever(t *testing.T) {
	env := newAuthoredReadEnv(t)
	detail := detailOver(t, env, txMonthEnd)
	type step struct {
		text     string
		reason   any
		undoable bool
	}
	var got []step
	for _, entry := range historyOfDetail(t, detail) {
		got = append(got, step{entry["text"].(string), entry["reason"], entry["undoable"].(bool)})
	}
	want := []step{
		{"O banco juntou o IOF à compra", nil, false},
		{"Você alterou para Mercado", reasonOfTheSecondStep, true},
		{"Você colocou em Delivery", reasonOfTheFirstStep, false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("history =\n%v\nwant\n%v", got, want)
	}
	if detail["reason"] != reasonOfTheSecondStep {
		t.Errorf("reason = %v, want the latest step's, %q", detail["reason"], reasonOfTheSecondStep)
	}
	salary := historyOfDetail(t, detailOver(t, env, txSalary))
	if len(salary) != 1 || salary[0]["undoable"] != true {
		t.Errorf("the salary's step, whose change a later one touched only on another transaction, = %v; want undoable", salary)
	}
}

func TestAnExclusionCountsOnlyOnceItHasAReason(t *testing.T) {
	env := newAuthoredReadEnv(t)
	march := transactionsOf(t, env, "2026-03", "month")
	for id, want := range map[string]struct {
		reason          string
		excluded, count bool
	}{
		txForeign: {"", false, true},
		txPixOut:  {"Empréstimo, ela devolve", true, false},
	} {
		detail := detailOver(t, env, id)
		exclusion := objectAt(t, detail, "exclusion")
		listed := rowWithIDIn(t, march, id)
		if exclusion["reason"] != want.reason || listed["excluded"] != want.excluded || listed["countsInTotals"] != want.count {
			t.Errorf("%s: exclusion %v excluded %v counts %v, want reason %q excluded %v counts %v",
				id, exclusion, listed["excluded"], listed["countsInTotals"], want.reason, want.excluded, want.count)
		}
	}
}

// The order that would come out of the bytes alone puts the upper-case tag
// first and the accented one last.
var knownTagsInOrder = []string{"#água", "#casa", "#essencial", "#lanche", "#manhã", "#mensal", "#mercado", "#Trabalho"}

func TestTheKnownTagsAreEveryTagInUseOnceInThePinnedOrder(t *testing.T) {
	env := newAuthoredReadEnv(t)
	_, body := env.answer("transactions.filterOptions", `["2026-03"]`)
	var options struct {
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &options); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(options.Tags, knownTagsInOrder) {
		t.Errorf("tags = %v, want %v", options.Tags, knownTagsInOrder)
	}
	own := []string{"#manhã", "#lanche"}
	var suggested []string
	for _, tag := range knownTagsInOrder {
		if !slices.Contains(own, tag) {
			suggested = append(suggested, tag)
		}
	}
	if got := texts(t, detailOver(t, env, txPending)["suggestedTags"]); !slices.Equal(got, suggested) {
		t.Errorf("suggestedTags = %v, want the known tags without the transaction's own, %v", got, suggested)
	}
}

func TestTagsSortAsTheyReadWithTheirBytesOnlyBreakingATie(t *testing.T) {
	got := sortedTags([]string{"#Zebra", "#água", "#casa", "#Casa", "#azul", "#Ônibus", "#onda"})
	want := []string{"#água", "#azul", "#Casa", "#casa", "#onda", "#Ônibus", "#Zebra"}
	if !slices.Equal(got, want) {
		t.Errorf("sorted = %v, want %v", got, want)
	}
}

func TestEveryListedTransactionAgreesWithItsDetailOverWhatWasAuthored(t *testing.T) {
	env := newAuthoredReadEnv(t)
	for _, month := range []string{"2026-01", "2026-02", "2026-03"} {
		for _, row := range transactionsOf(t, env, month, "month") {
			detail := detailOver(t, env, row["id"].(string))
			for field, value := range row {
				if !reflect.DeepEqual(detail[field], value) {
					t.Errorf("%s: %s is %v in the list and %v in the detail", row["id"], field, value, detail[field])
				}
			}
		}
	}
}

// reimbursedPurchase is a purchase of 300.00 on no line, whose state links a
// credit of the given cents, as the reimbursement writes will keep it.
func reimbursedPurchase(credit money.Cents) (txrules.Transaction, authoredRows) {
	purchase := txrules.Transaction{ID: "tx-purchase", AccountID: accountItau, Day: "2026-03-10", Amount: -30_000, Posted: true}
	creditKey := authored.TxKey(2)
	rows := authoredRows{
		keys: map[string]authored.TxKey{"tx-purchase": 1, "tx-credit": creditKey},
		kept: map[authored.TxKey]authored.AuthoredTransaction{
			1: {State: authored.TransactionState{Reimbursable: true, Credit: &creditKey, CreditCents: &credit}},
		},
		linkedCredits: map[authored.TxKey]bool{creditKey: true},
		months:        map[brday.Month]transactionLines{"2026-03": transactionLinesOf(authored.Budget{})},
	}
	return purchase, rows
}

func TestALinkedCreditIsTakenOffItsPurchaseUpToWhatWasPaid(t *testing.T) {
	for credit, want := range map[money.Cents]Reais{15_000: -15_000, 30_000: 0, 40_000: 0} {
		purchase, rows := reimbursedPurchase(credit)
		wire := rows.transactionOf(purchase, ledger.Transaction{}, ledgerContext{})
		if wire.EffectiveAmount != want || !wire.Reimbursed || wire.Amount != -30_000 {
			t.Errorf("a credit of %d: effective %d reimbursed %v amount %d, want %d, true, -30000", credit, wire.EffectiveAmount, wire.Reimbursed, wire.Amount, want)
		}
	}
	_, rows := reimbursedPurchase(15_000)
	credit := txrules.Transaction{ID: "tx-credit", AccountID: accountItau, Day: "2026-03-12", Amount: 15_000, Posted: true}
	if wire := rows.transactionOf(credit, ledger.Transaction{}, ledgerContext{}); !wire.ReimbursementIncome || wire.EffectiveAmount != 15_000 {
		t.Errorf("the linked credit: reimbursementIncome %v effective %d, want true and its own amount", wire.ReimbursementIncome, wire.EffectiveAmount)
	}
}

func TestTheRefundMarksAndTheLinkedCreditShowOnTheListAndInThePanel(t *testing.T) {
	env := newAuthoredReadEnv(t)
	quarter := transactionsOf(t, env, "2026-03", "last-3-months")
	want := map[string]map[string]any{
		txSalary:       {"refund": true, "line": "Salário", "reimbursementIncome": false, "effectiveAmount": 5000.0},
		txRefund:       {"refund": false, "line": nil, "reimbursementIncome": false},
		txFirstOfMarch: {"reimbursed": true, "amount": -10.0, "effectiveAmount": 0.0},
		txStale:        {"refund": false, "reimbursementIncome": true, "effectiveAmount": 50.0},
	}
	for id, fields := range want {
		row := rowWithIDIn(t, quarter, id)
		for field, value := range fields {
			if row[field] != value {
				t.Errorf("%s: %s = %v, want %v", id, field, row[field], value)
			}
		}
	}
	for id, refundLine := range map[string]any{txSalary: "Viagens", txRefund: nil, txStale: nil} {
		if got := detailOver(t, env, id)["refundLine"]; got != refundLine {
			t.Errorf("%s: refundLine = %v, want %v", id, got, refundLine)
		}
	}
}

// Each fact and each decision that Dispose reads takes two values over these
// rows, and the two line ids differ, so a field dropped, fixed or taken from
// its neighbour comes out wrong in one of them.
func TestDisposableHandsDisposeEveryFactOfTheRowAndEveryDecisionAboutIt(t *testing.T) {
	line, refundLine, partLine := authored.LineID(4), authored.LineID(9), authored.LineID(5)
	creditKey, creditCents := authored.TxKey(7), money.Cents(1_200)
	rows := authoredRows{
		keys: map[string]authored.TxKey{"tx-purchase": 3, "tx-credit": creditKey},
		kept: map[authored.TxKey]authored.AuthoredTransaction{
			3: {
				State: authored.TransactionState{
					Line: &line, ClassifiedBy: authored.AuthorRule, PendingReason: "Confiança abaixo do limite",
					Exclusion: authoredtest.Some("Presente"), Refund: authoredtest.Some(false), RefundLine: &refundLine,
					Credit: &creditKey, CreditCents: &creditCents,
				},
				Split: []authored.SplitPart{{Position: 0, Line: &partLine, Cents: 1_000}, {Position: 1, Cents: 3_000}},
			},
			creditKey: {State: authored.TransactionState{ClassifiedBy: authored.AuthorPerson, Exclusion: authoredtest.Some(""), Refund: authoredtest.Some(true)}},
		},
		linkedCredits: map[authored.TxKey]bool{3: false, creditKey: true},
	}
	cases := []struct {
		listed txrules.Transaction
		want   budgetrules.Transaction
	}{
		{
			txrules.Transaction{ID: "tx-purchase", Amount: -4_000, Unconverted: true, Movement: txrules.OwnTransfer, Fee: true, Posted: true, Refund: true},
			budgetrules.Transaction{
				Amount: -4_000, Unconverted: true, Movement: true, Fee: true, Posted: true, LedgerRefund: true,
				Line: budgetrules.OnLine(4), LineSetBy: budgetrules.SetByRule, ReviewPending: true,
				Exclusion: budgetrules.ExcludedWithReason, RefundOverride: budgetrules.RefundNo, RefundLine: budgetrules.OnLine(9),
				Split:        []budgetrules.SplitPart{{Line: budgetrules.OnLine(5), Amount: 1_000}, {Amount: 3_000}},
				LinkedCredit: 1_200,
			},
		},
		{
			txrules.Transaction{ID: "tx-credit", Amount: 1_200},
			budgetrules.Transaction{
				Amount: 1_200, LineSetBy: budgetrules.SetByPerson, Exclusion: budgetrules.ExcludedWithoutReason,
				RefundOverride: budgetrules.RefundYes, LinkedToPurchase: true,
			},
		},
		{
			txrules.Transaction{ID: "tx-never-indexed", Amount: -500, Posted: true},
			budgetrules.Transaction{Amount: -500, Posted: true},
		},
	}
	for _, c := range cases {
		if got := rows.disposable(c.listed); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n%+v\nwant\n%+v", c.listed.ID, got, c.want)
		}
	}
}
