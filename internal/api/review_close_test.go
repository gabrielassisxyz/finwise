package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/classrules"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

func init() {
	writeCases = append(writeCases, closeMonthWriteCases...)
}

// closeMonthWriteCases close a month that has no version of its own and one
// that has, each recorded and undone, and refuse a closed month, a month to
// come and a month with a review of February still waiting.
var closeMonthWriteCases = []writeCase{
	{method: closeMonthMethod, arguments: `["2026-02"]`, expects: expectsRecorded, state: closeExamplesBudget},
	{method: closeMonthMethod, arguments: `["2026-03"]`, expects: expectsRecorded, state: startFromTheInventedState},
	{method: closeMonthMethod, arguments: `["2026-01"]`, expects: expectsRefusal, reason: reasonMonthClosed, state: startFromTheInventedState},
	{method: closeMonthMethod, arguments: `["2026-04"]`, expects: expectsRefusal, reason: reasonFutureMonth, state: closeExamplesBudget},
	{method: closeMonthMethod, arguments: `["2026-03"]`, expects: expectsRefusal, reason: reasonPendingReviews, state: februaryWaitsForReview},
}

// closeExamplesBudget is the budget of the close's examples: one version from
// 2026-01 holding Mercado 400,00 and Delivery 200,00, which a fresh database
// numbers 2 and 3.
func closeExamplesBudget(t *testing.T, scratch *authoredtest.Scratch) {
	t.Helper()
	authoredtest.SaveBudget(t, scratch.Database, utc(2026, time.January, 2, 15, 0), "2026-01",
		authored.Document{authoredtest.Group("var", authoredtest.Line("Mercado", 40_000), authoredtest.Line("Delivery", 20_000))})
}

// februaryWaitsForReview leaves the full ledger's purchase of 28 February
// waiting, which stops the close of March as well as February's.
func februaryWaitsForReview(t *testing.T, scratch *authoredtest.Scratch) {
	t.Helper()
	closeExamplesBudget(t, scratch)
	rulesLeave(t, scratch, newFullLedger(t).Path, waitingReview{txMonthEnd, string(classrules.ReasonNewEstablishment), "", nil})
}

// The close's own ledger: one purchase on each side of every boundary the
// close looks at. Tf and Tm are the examples' transactions of February and
// March.
const (
	closeAccount       = "acc-close"
	txCloseJanuary     = "tx-close-january"
	txCloseTf          = "tx-close-tf"
	txCloseTwentySeven = "tx-close-27"
	txCloseLastHour    = "tx-close-last-hour"
	txCloseMidnight    = "tx-close-midnight"
	txCloseTm          = "tx-close-tm"
)

func newCloseLedger(t testing.TB, leftOut ...string) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-close", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddAccount(ledgertest.Account{ID: closeAccount, ItemID: "item-close", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Corrente", Currency: "BRL", Balance: 700, TransferNumber: "341/0001/40000-4"})
	purchase := func(id string, at time.Time, description string, amount float64) {
		if slices.Contains(leftOut, id) {
			return
		}
		f.AddTransaction(ledgertest.Transaction{ID: id, AccountID: closeAccount, Date: at, Description: description,
			Amount: amount, Currency: "BRL", Type: "DEBIT", Raw: map[string]any{"operationType": "CARTAO"}})
	}
	purchase(txCloseJanuary, utc(2026, time.January, 15, 15, 0), "LIVRARIA EXEMPLO", -30)
	purchase(txCloseTf, utc(2026, time.February, 10, 15, 0), "PIZZARIA EXEMPLO", -45)
	purchase(txCloseTwentySeven, utc(2026, time.February, 27, 15, 0), "FARMACIA EXEMPLO", -22)
	purchase(txCloseLastHour, utc(2026, time.March, 1, 2, 30), "PADARIA EXEMPLO", -12)
	purchase(txCloseMidnight, utc(2026, time.March, 1, 3, 0), "CAFETERIA EXEMPLO", -9)
	purchase(txCloseTm, utc(2026, time.March, 5, 15, 0), "LANCHONETE EXEMPLO", -38)
	return f
}

// closeEnv is a router over the close's ledger and a database of its own, the
// clock at 2026-03-20.
type closeEnv struct {
	*writeEnv
	ledgerPath string
}

func newCloseEnvOver(t *testing.T, ledgerPath string, scratch *authoredtest.Scratch) *closeEnv {
	t.Helper()
	return &closeEnv{writeEnv: newReviewEnv(t, ledgerPath, scratch, fixtureNow), ledgerPath: ledgerPath}
}

// newCloseExamples is the state Z of the examples: the budget, and Tf and Tm
// on Delivery unless left out.
func newCloseExamples(t *testing.T, withTf bool) *closeEnv {
	t.Helper()
	f := newCloseLedger(t)
	scratch := authoredtest.Open(t, fixtureClock)
	closeExamplesBudget(t, scratch)
	if withTf {
		settledOn(t, scratch, f.Path, "2026-03-02", "transactions.setLine", txCloseTf, "Delivery")
	}
	settledOn(t, scratch, f.Path, "2026-03-06", "transactions.setLine", txCloseTm, "Delivery")
	return newCloseEnvOver(t, f.Path, scratch)
}

// leaveWaiting records a review of the rules on each transaction.
func (env *closeEnv) leaveWaiting(ids ...string) {
	env.t.Helper()
	reviews := make([]waitingReview, len(ids))
	for i, id := range ids {
		reviews[i] = waitingReview{id, string(classrules.ReasonNewEstablishment), "", nil}
	}
	rulesLeave(env.t, env.scratch, env.ledgerPath, reviews...)
}

func (env *closeEnv) closeMonth(month string) *httptest.ResponseRecorder {
	env.t.Helper()
	return env.post(closeMonthMethod, `["`+month+`"]`)
}

func (env *closeEnv) closed(month string) string {
	env.t.Helper()
	return changeIDOfAnswer(env.t, env.closeMonth(month))
}

// varDocument is a budget document of the variable group alone, each line
// named and given in reais, under its id when the line exists.
func (env *closeEnv) varDocument(namesAndAmounts ...string) string {
	env.t.Helper()
	ids := env.lineIDs()
	lines := make([]string, 0, len(namesAndAmounts)/2)
	for i := 0; i+1 < len(namesAndAmounts); i += 2 {
		name, amount := namesAndAmounts[i], namesAndAmounts[i+1]
		id := ""
		if known, found := ids[name]; found {
			id = `"id":"` + known + `",`
		}
		lines = append(lines, `{`+id+`"name":"`+name+`","amount":`+amount+`}`)
	}
	return `[{"key":"var","name":"Variáveis","lines":[` + strings.Join(lines, ",") + `]}]`
}

// lineIDs is every line of the examples by name, as January's document names
// them; a line it lacks was never minted.
func (env *closeEnv) lineIDs() map[string]string {
	env.t.Helper()
	ids := map[string]string{}
	var groups []struct {
		Lines []struct{ ID, Name string } `json:"lines"`
	}
	if err := json.Unmarshal([]byte(env.document("2026-01")), &groups); err != nil {
		env.t.Fatal(err)
	}
	for _, group := range groups {
		for _, line := range group.Lines {
			ids[line.Name] = line.ID
		}
	}
	return ids
}

func (env *closeEnv) save(month, document string) string {
	env.t.Helper()
	return changeIDOfAnswer(env.t, env.post("budget.save", `["`+month+`",`+document+`]`))
}

func (env *closeEnv) document(month string) string {
	env.t.Helper()
	response := env.post("budget.document", `["`+month+`"]`)
	if response.Code != http.StatusOK {
		env.t.Fatalf("budget.document %s answered %d %s", month, response.Code, response.Body.String())
	}
	return response.Body.String()
}

// mercado is the amount of Mercado in the budget of month, as its document
// writes it, or "none".
func (env *closeEnv) mercado(month string) string {
	env.t.Helper()
	var groups []struct {
		Lines []struct {
			Name   string          `json:"name"`
			Amount json.RawMessage `json:"amount"`
		} `json:"lines"`
	}
	if err := json.Unmarshal([]byte(env.document(month)), &groups); err != nil {
		env.t.Fatal(err)
	}
	for _, group := range groups {
		for _, line := range group.Lines {
			if line.Name == "Mercado" {
				return string(line.Amount)
			}
		}
	}
	return "none"
}

func (env *closeEnv) column(query string, args ...any) []string {
	env.t.Helper()
	rows, err := env.scratch.Inspect(env.t).Query(query, args...)
	if err != nil {
		env.t.Fatal(err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			env.t.Fatal(err)
		}
		values = append(values, value)
	}
	return values
}

func (env *closeEnv) versions() []string {
	return env.column(`SELECT from_month FROM budget_versions ORDER BY from_month`)
}

// versionLines is every column of a version's lines but its month.
func (env *closeEnv) versionLines(month string) []string {
	return env.column(`SELECT group_key || ' ' || position || ' ' || line_id || ' ' || name || ' ' || cents || ' ' || coalesce(flag, '-')
FROM budget_version_lines WHERE from_month = ? ORDER BY group_key, position`, month)
}

func (env *closeEnv) closedMonths() []string {
	return env.column(`SELECT month || ' ' || closed_on FROM closed_months ORDER BY month`)
}

func (env *closeEnv) undone(id string) bool {
	return len(env.column(`SELECT id FROM changes WHERE id = ? AND undone_at IS NOT NULL`, id)) == 1
}

// lineOf is the name of the line a transaction is on, as its panel shows it,
// or "none".
func (env *closeEnv) lineOf(id string) string {
	env.t.Helper()
	response := env.post("transactions.detail", `["`+id+`"]`)
	if response.Code != http.StatusOK {
		env.t.Fatalf("transactions.detail %s answered %d", id, response.Code)
	}
	if line, ok := decodeAnswer(env.t, response.Body.Bytes())["line"].(string); ok {
		return line
	}
	return "none"
}

func (env *closeEnv) mustUndo(label, id string) {
	env.t.Helper()
	expectUndone(env.t, label, env.undo(id))
}

// expectRefusedUndo wants the undo of id refused as stale with nothing changed
// and the change not undone.
func (env *closeEnv) expectRefusedUndo(label, id string) {
	env.t.Helper()
	before, counts := env.scratch.Canonical(env.t), journalCounts(env.t, env.scratch)
	expectRefused(env.t, label, env.undo(id), string(reasonStaleChange))
	if env.scratch.Canonical(env.t) != before || journalCounts(env.t, env.scratch) != counts {
		env.t.Errorf("%s: the refused undo changed the database or the journal", label)
	}
	if env.undone(id) {
		env.t.Errorf("%s: the refused change is marked undone", label)
	}
}

// expectCloseRefused wants the close of month refused for reason, with every
// journaled table and the journal's two tables as they were.
func (env *closeEnv) expectCloseRefused(label, month string, reason refusalReason) {
	env.t.Helper()
	before, counts := env.scratch.Canonical(env.t), journalCounts(env.t, env.scratch)
	expectRefused(env.t, label, env.closeMonth(month), string(reason))
	if env.scratch.Canonical(env.t) != before || journalCounts(env.t, env.scratch) != counts {
		env.t.Errorf("%s: the refused close changed the database or the journal", label)
	}
}

func TestAReviewOfTheMonthOrAnEarlierOneStopsTheCloseAndOneOfTheNextMonthDoesNot(t *testing.T) {
	for _, id := range []string{txCloseTwentySeven, txCloseLastHour, txCloseJanuary} {
		env := newCloseExamples(t, true)
		env.leaveWaiting(id)
		env.expectCloseRefused(id+" waiting", "2026-02", reasonPendingReviews)
	}
	env := newCloseExamples(t, true)
	env.leaveWaiting(txCloseMidnight)
	env.closed("2026-02")
	if got := env.closedMonths(); !slices.Equal(got, []string{"2026-02 2026-03-20"}) {
		t.Errorf("with only the purchase of 1 March waiting, closed_months = %v", got)
	}
}

func TestAReviewTheSnapshotDoesNotListOrOfAMonthWithNoLineDoesNotStopTheClose(t *testing.T) {
	env := newCloseExamples(t, true)
	env.leaveWaiting(txCloseTwentySeven)
	without := newCloseEnvOver(t, newCloseLedger(t, txCloseTwentySeven).Path, env.scratch)
	closing := without.closed("2026-02")
	without.mustUndo("the close over the snapshot without the row", closing)
	env.expectCloseRefused("the row listed again", "2026-02", reasonPendingReviews)

	f := newCloseLedger(t)
	scratch := authoredtest.Open(t, fixtureClock)
	authoredtest.SaveBudget(t, scratch.Database, utc(2026, time.January, 2, 15, 0), "2026-01", authoredtest.Emptied(nil))
	lineless := newCloseEnvOver(t, f.Path, scratch)
	lineless.leaveWaiting(txCloseTwentySeven)
	lineless.closed("2026-02")
}

func TestAMonthToComeIsRefusedFirstAndTheCurrentMonthCanBeClosed(t *testing.T) {
	env := newCloseExamples(t, true)
	env.leaveWaiting(txCloseTm)
	env.expectCloseRefused("April with a purchase of March waiting", "2026-04", reasonFutureMonth)

	current := newCloseExamples(t, true)
	current.closed("2026-03")
	if got := current.closedMonths(); !slices.Equal(got, []string{"2026-03 2026-03-20"}) {
		t.Errorf("closed_months = %v", got)
	}
}

func TestAClosedMonthIsRefusedBeforeAnythingElseAndAMissingSnapshotIsUnavailable(t *testing.T) {
	env := newCloseExamples(t, true)
	env.closed("2026-02")
	env.leaveWaiting(txCloseTwentySeven)
	env.expectCloseRefused("a closed month with a review of its own waiting", "2026-02", reasonMonthClosed)

	gone := filepath.Join(t.TempDir(), "pluggy-snapshot.db")
	missing := newCloseEnvOver(t, gone, env.scratch)
	missing.expectCloseRefused("a closed month with the snapshot gone", "2026-02", reasonMonthClosed)
	before, counts := env.scratch.Canonical(t), journalCounts(t, env.scratch)
	expectError(t, "an open month with the snapshot gone", missing.closeMonth("2026-03"), http.StatusServiceUnavailable, "ledger_unavailable")
	expectError(t, "2026-13", env.closeMonth("2026-13"), http.StatusBadRequest, "bad_request")
	if env.scratch.Canonical(t) != before || journalCounts(t, env.scratch) != counts {
		t.Error("an unavailable snapshot or a bad month changed the database or the journal")
	}
	// With no review kept, the queue itself would ask the ledger nothing.
	quiet := newCloseExamples(t, true)
	quietBefore, quietCounts := quiet.scratch.Canonical(t), journalCounts(t, quiet.scratch)
	unread := newCloseEnvOver(t, gone, quiet.scratch)
	expectError(t, "an open month with no review kept and the snapshot gone", unread.closeMonth("2026-02"), http.StatusServiceUnavailable, "ledger_unavailable")
	if quiet.scratch.Canonical(t) != quietBefore || journalCounts(t, quiet.scratch) != quietCounts {
		t.Error("the close over a missing snapshot changed the database or the journal")
	}

	without := newTestEnv(t, withLedger(newCloseLedger(t).Path, fixtureNow))
	if reads, status := readsOfBody(without, "/api/review/closeMonth", without.unlock()); reads != 0 || status != http.StatusForbidden {
		t.Errorf("without a database the close read its body %d times and answered %d, want 0 and 403", reads, status)
	}
}

// Example A: a month with no version of its own gets one, a copy row for row
// of the version in force, and no month's budget moves.
func TestClosingAMonthWithoutItsOwnVersionCopiesItAndTheUndoGivesBackZ(t *testing.T) {
	env := newCloseExamples(t, true)
	z := env.scratch.Canonical(t)
	months := []string{"2025-12", "2026-01", "2026-02", "2026-03"}
	documents := map[string]string{}
	for _, month := range months {
		documents[month] = env.document(month)
	}
	closing := env.closed("2026-02")
	if got := env.closedMonths(); !slices.Equal(got, []string{"2026-02 2026-03-20"}) {
		t.Errorf("closed_months = %v", got)
	}
	if got := env.versions(); !slices.Equal(got, []string{"2026-01", "2026-02"}) {
		t.Errorf("budget_versions = %v", got)
	}
	if copied, first := env.versionLines("2026-02"), env.versionLines("2026-01"); len(first) != 2 || !slices.Equal(copied, first) {
		t.Errorf("the lines of 2026-02 are %v, want those of 2026-01 %v", copied, first)
	}
	for _, month := range months {
		if got := env.document(month); got != documents[month] {
			t.Errorf("%s answers %s after the close, %s before", month, got, documents[month])
		}
	}
	env.mustUndo("the undo of the close", closing)
	if env.scratch.Canonical(t) != z {
		t.Errorf("the undo did not give back Z:\n%s", differences(z, env.scratch.Canonical(t)))
	}
}

// Example B: the close finds February's own version and writes only its row.
// The save's undo would answer the closed February from January's version.
func TestASaveForTheMonthThenItsCloseUndoneInEitherOrder(t *testing.T) {
	env := newCloseExamples(t, true)
	z := env.scratch.Canonical(t)
	save := env.save("2026-02", env.varDocument("Mercado", "500", "Delivery", "200"))
	closing := env.closed("2026-02")
	if effects := env.scratch.EffectsOf(t, changeNumber(t, closing)); len(effects) != 1 || effects[0].Table != "closed_months" {
		t.Errorf("the close's effects are %+v, want the closed row alone", effects)
	}
	env.expectRefusedUndo("the undo of the save while the close stands", save)
	if got := env.mercado("2026-02"); got != "500.00" {
		t.Errorf("after the refusal February's Mercado is %s, want 500.00", got)
	}
	env.mustUndo("the undo of the close", closing)
	if got := env.mercado("2026-02"); got != "500.00" {
		t.Errorf("the reopened February's Mercado is %s, want its own version's 500.00", got)
	}
	env.mustUndo("the undo of the save", save)
	if env.scratch.Canonical(t) != z {
		t.Errorf("undoing the close then the save did not give back Z:\n%s", differences(z, env.scratch.Canonical(t)))
	}
}

// Example C: a save for January, then the close of February, which copies
// what the save left in force.
func TestASaveForAnEarlierMonthThenACloseUndoneInBothOrders(t *testing.T) {
	for _, closeFirst := range []bool{true, false} {
		env := newCloseExamples(t, true)
		z := env.scratch.Canonical(t)
		save := env.save("2026-01", env.varDocument("Mercado", "450", "Delivery", "200"))
		closing := env.closed("2026-02")
		if got := env.versionLines("2026-02"); len(got) != 2 || !strings.Contains(got[0], "Mercado 45000") {
			t.Errorf("the close copied %v, want Mercado at 450,00", got)
		}
		if closeFirst {
			env.mustUndo("the undo of the close", closing)
			if got := env.mercado("2026-02"); got != "450.00" {
				t.Errorf("the reopened February reads Mercado %s, want January's 450.00", got)
			}
			env.mustUndo("then the save", save)
		} else {
			env.mustUndo("the undo of the save", save)
			for month, want := range map[string]string{"2026-01": "400.00", "2026-02": "450.00", "2026-03": "450.00"} {
				if got := env.mercado(month); got != want {
					t.Errorf("after the save's undo %s reads Mercado %s, want %s", month, got, want)
				}
			}
			env.mustUndo("then the close", closing)
		}
		if env.scratch.Canonical(t) != z {
			t.Errorf("close undone first %v: the two undos did not give back Z:\n%s", closeFirst, differences(z, env.scratch.Canonical(t)))
		}
	}
}

// Example D: a save across the closed February that removes Delivery.
func TestACloseThenASaveAcrossItThatRemovesALineTheMonthsTransactionIsOn(t *testing.T) {
	env := newCloseExamples(t, true)
	z := env.scratch.Canonical(t)
	closing := env.closed("2026-02")
	save := env.save("2026-01", env.varDocument("Mercado", "450"))
	if tf, tm := env.lineOf(txCloseTf), env.lineOf(txCloseTm); tf != "Delivery" || tm != "none" {
		t.Errorf("after the save Tf is on %s and Tm on %s, want Delivery and none", tf, tm)
	}
	if got := env.versions(); !slices.Equal(got, []string{"2026-01", "2026-02", "2026-03"}) {
		t.Errorf("budget_versions = %v", got)
	}
	env.expectRefusedUndo("the undo of the close while the save stands", closing)
	if got := env.closedMonths(); !slices.Equal(got, []string{"2026-02 2026-03-20"}) {
		t.Errorf("after the refusal closed_months = %v", got)
	}
	env.mustUndo("the undo of the save", save)
	if tm := env.lineOf(txCloseTm); tm != "Delivery" || !slices.Equal(env.versions(), []string{"2026-01", "2026-02"}) {
		t.Errorf("after the save's undo Tm is on %s and the versions are %v", tm, env.versions())
	}
	env.mustUndo("then the close", closing)
	if env.scratch.Canonical(t) != z {
		t.Errorf("undoing the save then the close did not give back Z:\n%s", differences(z, env.scratch.Canonical(t)))
	}

	alone := newCloseExamples(t, false)
	z = alone.scratch.Canonical(t)
	closing = alone.closed("2026-02")
	save = alone.save("2026-01", alone.varDocument("Mercado", "450"))
	alone.mustUndo("the undo of the close with no transaction of February on Delivery", closing)
	if got := alone.document("2026-02"); got != alone.document("2026-01") || alone.mercado("2026-02") != "450.00" || strings.Contains(got, "Delivery") {
		t.Errorf("the reopened February answers %s, want January's Mercado 450.00 alone", got)
	}
	alone.mustUndo("then the save", save)
	if alone.scratch.Canonical(t) != z {
		t.Errorf("without Tf the two undos did not give back Z:\n%s", differences(z, alone.scratch.Canonical(t)))
	}
}

// Example E: a close and a save for a later month share no unit and move no
// closed budget, so they come back in either order.
func TestACloseThenASaveForALaterMonthUndoneInEitherOrder(t *testing.T) {
	for _, closeFirst := range []bool{true, false} {
		env := newCloseExamples(t, true)
		z := env.scratch.Canonical(t)
		closing := env.closed("2026-02")
		save := env.save("2026-03", env.varDocument("Mercado", "600", "Delivery", "200"))
		first, second := save, closing
		if closeFirst {
			first, second = closing, save
		}
		env.mustUndo("the first undo", first)
		if closeFirst && (env.mercado("2026-02") != "400.00" || env.mercado("2026-03") != "600.00" || env.lineOf(txCloseTf) != "Delivery") {
			t.Errorf("after the close's undo February reads %s, March %s and Tf is on %s", env.mercado("2026-02"), env.mercado("2026-03"), env.lineOf(txCloseTf))
		}
		env.mustUndo("the second undo", second)
		if env.scratch.Canonical(t) != z {
			t.Errorf("close undone first %v: the two undos did not give back Z:\n%s", closeFirst, differences(z, env.scratch.Canonical(t)))
		}
	}
}

// Example F: with no budget the close keeps five empty groups, which a later
// save then leaves January following; before the first version it keeps a
// copy of the first.
func TestClosingAMonthWithNoBudgetOrBeforeTheFirstVersion(t *testing.T) {
	f := newCloseLedger(t)
	nothing := newCloseEnvOver(t, f.Path, authoredtest.Open(t, fixtureClock))
	empty := nothing.document("2026-02")
	nothing.closed("2026-02")
	if got := nothing.versions(); !slices.Equal(got, []string{"2026-02"}) || len(nothing.versionLines("2026-02")) != 0 {
		t.Errorf("with no budget the close wrote versions %v with lines %v, want one with none", got, nothing.versionLines("2026-02"))
	}
	nothing.post("budget.save", `["2026-03",[{"key":"var","name":"Variáveis","lines":[{"name":"Mercado","amount":400}]}]]`)
	if got := nothing.document("2026-01"); got != empty {
		t.Errorf("after a save for March, January answers %s, want the empty budget %s", got, empty)
	}

	later := newCloseEnvOver(t, f.Path, authoredtest.Open(t, fixtureClock))
	later.save("2026-03", `[{"key":"var","name":"Variáveis","lines":[{"name":"Mercado","amount":400}]}]`)
	january := later.document("2026-01")
	later.closed("2026-02")
	if got, want := later.versionLines("2026-02"), later.versionLines("2026-03"); len(want) != 1 || !slices.Equal(got, want) {
		t.Errorf("before the first version the close copied %v, want %v", got, want)
	}
	if got := later.document("2026-01"); got != january {
		t.Errorf("January answers %s after the close, %s before", got, january)
	}
}

func TestAClosedMonthsBudgetRefusesItsSavesAndItsTransactionsStayEditable(t *testing.T) {
	env := newCloseExamples(t, true)
	env.closed("2026-02")
	before, counts := env.scratch.Canonical(t), journalCounts(t, env.scratch)
	for _, method := range []string{"budget.save", "budget.applyScenario"} {
		expectRefused(t, method, env.post(method, `["2026-02",`+env.varDocument("Mercado", "1")+`]`), string(reasonMonthClosed))
	}
	if env.scratch.Canonical(t) != before || journalCounts(t, env.scratch) != counts {
		t.Error("a refused save changed the database or the journal")
	}
	earlier := env.save("2026-01", env.varDocument("Mercado", "450", "Delivery", "200"))
	for _, effect := range env.scratch.EffectsOf(t, changeNumber(t, earlier)) {
		if strings.Contains(effect.Key, `"2026-02"`) {
			t.Errorf("the save for January has an effect on the closed month's version: %+v", effect)
		}
	}
	closing := closingIn(t, &readEnv{testEnv: env.testEnv, cookie: env.cookie}, "2026-02")
	if !closing.Closed || closing.ClosedOn == nil || *closing.ClosedOn != "2026-03-20" {
		t.Errorf("review.closing reads closed %v on %v", closing.Closed, closing.ClosedOn)
	}
	if response := env.post(markStepDoneMethod, `["2026-02","leftover"]`); response.Code != http.StatusOK || response.Body.String() != "null" {
		t.Errorf("a step of the closed month answered %d %s, want null", response.Code, response.Body.String())
	}
	changeIDOfAnswer(t, env.post("transactions.setLine", `["`+txCloseTf+`","Mercado"]`))
	if got := env.lineOf(txCloseTf); got != "Mercado" {
		t.Errorf("Tf of the closed February is on %s, want Mercado", got)
	}
}

func TestASecondUndoOfACloseIsStaleAndTheMonthClosesAgainUnderANewChange(t *testing.T) {
	env := newCloseExamples(t, true)
	first := env.closed("2026-02")
	env.mustUndo("the undo of the close", first)
	before, counts := env.scratch.Canonical(t), journalCounts(t, env.scratch)
	expectRefused(t, "a second undo of the close", env.undo(first), string(reasonStaleChange))
	if env.scratch.Canonical(t) != before || journalCounts(t, env.scratch) != counts {
		t.Error("the second undo changed the database or the journal")
	}
	if again := env.closed("2026-02"); again == first {
		t.Errorf("closing again answered the undone change %s", first)
	}
}

// The server runs the ingest after a budget save alone: the close, which
// changes no month's budget, records no change of the rules.
func TestTheCloseRunsNoIngest(t *testing.T) {
	f := newCloseLedger(t)
	scratch := authoredtest.Open(t, fixtureClock)
	sources := ingest.Sources{Ledger: ledger.New(f.Path), Database: scratch.Database}
	runs := 0
	server := newTestEnv(t, withLedger(f.Path, fixtureNow), withDatabase(scratch.Database), func(config *Config) {
		config.AfterBudgetSaved = func(ctx context.Context, at time.Time) {
			runs++
			if _, err := ingest.Run(ctx, sources, ingest.TriggerBudgetChange, at); err != nil {
				t.Errorf("the ingest after the save: %v", err)
			}
		}
	})
	env := &closeEnv{writeEnv: &writeEnv{testEnv: server, scratch: scratch, cookie: server.unlock()}, ledgerPath: f.Path}
	byTheRules := func() int {
		count := 0
		for _, change := range scratch.Changes(t) {
			if change.Author == string(authored.AuthorRule) {
				count++
			}
		}
		return count
	}
	env.save("2026-01", `[{"key":"var","name":"Variáveis","lines":[{"name":"Mercado","amount":400}]}]`)
	if runs != 1 || byTheRules() == 0 {
		t.Fatalf("the save ran the ingest %d times and left %d changes of the rules, want once and some", runs, byTheRules())
	}
	before := byTheRules()
	env.closed("2026-02")
	if runs != 1 || byTheRules() != before {
		t.Errorf("the close ran the ingest %d more times and the rules' changes went from %d to %d", runs-1, before, byTheRules())
	}
}
