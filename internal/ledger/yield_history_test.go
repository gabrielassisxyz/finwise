package ledger_test

import (
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

func sightingOn(t *testing.T, history ledger.YieldHistory, day string) ledger.YieldSighting {
	t.Helper()
	for _, sighting := range history.Sightings {
		if sighting.AccountID == "acc-1" && string(sighting.Day) == day {
			return sighting
		}
	}
	t.Fatalf("no sighting of acc-1 on %s among %d", day, len(history.Sightings))
	return ledger.YieldSighting{}
}

func TestYieldHistoryHoldsEverySightingWithItsEntriesAndTheCheckedAmounts(t *testing.T) {
	f := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}, {Name: "Loose", Amount: 7, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 150.30, CDIRatio: 1}})
	history, err := viewOf(t, f.Path).YieldHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	first := sightingOn(t, history, string(yieldFrom))
	if first.UpdatedAt != "2026-03-10 02:00:00" || first.RunID != 1 || len(first.Entries) != 2 {
		t.Fatalf("first sighting = %+v, want the provider's time as the statement compares it and two entries", first)
	}
	stable, loose := first.Entries[0], first.Entries[1]
	if !stable.Stable || !stable.BRL || !stable.AmountOK || stable.Cents == nil || *stable.Cents != 10000 {
		t.Errorf("the pot with an id reads as %+v", stable)
	}
	if loose.Stable || loose.ReserveID != nil || loose.Cents == nil || *loose.Cents != 700 {
		t.Errorf("the pot with no id reads as %+v, want unstable with its amount", loose)
	}
}

func TestYieldHistoryLeavesOutASightingThatReportsNoListOfReserves(t *testing.T) {
	f := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 101, CDIRatio: 1}})
	f.ObserveAccount("acc-1", "2026-03-12", map[string]any{"bankData": map[string]any{}})

	history, err := viewOf(t, f.Path).YieldHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Sightings) != 2 {
		t.Fatalf("%d sightings, want the two that list their reserves", len(history.Sightings))
	}
}

func TestYieldHistoryFlagsAnAmountTheStatementWouldNotTrust(t *testing.T) {
	f := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 101, CDIRatio: 1}})
	f.ObserveAccount("acc-1", "2026-03-12", map[string]any{
		"updatedAt": "2026-03-12T02:00:00.000Z",
		"bankData": map[string]any{"reservedBalances": []any{
			map[string]any{"identification": "pot-a", "name": "Trip", "availableAmounts": []any{
				map[string]any{"currencyCode": "USD", "amount": 10.005}}},
		}},
	})
	history, err := viewOf(t, f.Path).YieldHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	entry := sightingOn(t, history, "2026-03-12").Entries[0]
	if entry.BRL || entry.AmountOK {
		t.Errorf("a dollar amount of three decimals reads as %+v, want neither BRL nor readable", entry)
	}
}

func TestYieldHistoryHoldsEveryVersionOfATransferAndOnlyTransfers(t *testing.T) {
	f := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 150, CDIRatio: 1}})
	walkReachesBack(t, f)
	addReserveTransfer(t, f, "tx-dep", "Dinheiro reservado Trip", -50)
	f.ObserveTransaction("tx-dep", "2026-03-16", map[string]any{"amount": -40.0})
	f.AddTransaction(ledgertest.Transaction{ID: "tx-coffee", AccountID: "acc-1", Date: instant(t, transferMoment),
		Description: "Coffee", Amount: -3, Currency: "BRL", Type: "DEBIT"})
	f.RecordEvent("transactions", "tx-dep", 1, "removed", "2026-03-17")

	history, err := viewOf(t, f.Path).YieldHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Transfers) != 2 || len(history.Removals) != 1 || len(history.Walks) != 1 {
		t.Fatalf("%d transfer versions, %d removals, %d walks, want 2, 1 and 1", len(history.Transfers), len(history.Removals), len(history.Walks))
	}
	older, newer := history.Transfers[0], history.Transfers[1]
	if older.ObservationID >= newer.ObservationID || older.Cents == nil || *older.Cents != -5000 || *newer.Cents != -4000 {
		t.Errorf("versions read as %+v then %+v, want the amounts -5000 then -4000 oldest first", older, newer)
	}
	if !older.IsReserve || older.NotPosted || older.Fault != nil || older.Name == nil || *older.Name != "Trip" {
		t.Errorf("the transfer reads as %+v, want a posted one naming Trip with no fault", older)
	}
}

// transferHistoryLedger holds a row of each kind the transfer history reads: a
// transfer seen in two versions and later removed, a transfer the bank has not
// settled beside its posted twin, which is orphaned with its parent, and a
// purchase pending beside its posted twin.
func transferHistoryLedger(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 150, CDIRatio: 1}})
	walkReachesBack(t, f)
	addReserveTransfer(t, f, "tx-dep", "Dinheiro reservado Trip", -50)
	f.ObserveTransaction("tx-dep", "2026-03-16", map[string]any{"amount": -40.0})
	f.RecordEvent("transactions", "tx-dep", 1, "removed", "2026-03-17")
	for _, twin := range []struct{ id, status, description, operation string }{
		{"tx-twin-pending", "PENDING", "Dinheiro reservado Trip", "TRANSFERENCIA_SALDO_RESERVADO"},
		{"tx-twin-posted", "POSTED", "Dinheiro reservado Trip", "TRANSFERENCIA_SALDO_RESERVADO"},
		{"tx-coffee-pending", "PENDING", "Coffee", "CARTAO"},
		{"tx-coffee-posted", "POSTED", "Coffee", "CARTAO"},
	} {
		f.AddTransaction(ledgertest.Transaction{ID: twin.id, AccountID: "acc-1", Date: instant(t, transferMoment),
			Description: twin.description, Amount: -7, Currency: "BRL", Type: "DEBIT", Status: twin.status,
			Raw: map[string]any{"operationType": twin.operation}})
	}
	f.RecordEvent("transactions", "tx-twin-posted", 1, "parent_gone", "2026-03-18")
	return f
}

func TestYieldHistoryHoldsEveryRowOfTheThreeKindsTheTransfersNeed(t *testing.T) {
	history, err := viewOf(t, transferHistoryLedger(t).Path).YieldHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tx-coffee-pending", "tx-coffee-posted", "tx-twin-pending", "tx-twin-posted"}
	if got := slices.Sorted(maps.Keys(history.Unresolved)); !slices.Equal(got, want) {
		t.Errorf("unresolved ids %v, want both of every pending pair, transfer or not: %v", got, want)
	}
	if len(history.Transfers) != 4 {
		t.Errorf("%d transfer versions, want the two of tx-dep and one of each twin", len(history.Transfers))
	}
	if len(history.Removals) != 2 || history.Removals[0].Day != "2026-03-17" || history.Removals[1].Day != "2026-03-18" {
		t.Errorf("removals %+v, want tx-dep's removal and the twin's lost parent", history.Removals)
	}
}

func TestOneYieldHistoryReadsTheLedgersTransactionsOnce(t *testing.T) {
	f := transferHistoryLedger(t)
	var readings atomic.Int64
	view := ledger.New(f.Path).ObservingYieldReadings(func() { readings.Add(1) }).View()
	defer view.Close()

	history, err := view.YieldHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Transfers) == 0 || len(history.Removals) == 0 || len(history.Unresolved) == 0 {
		t.Fatalf("the history holds %d versions, %d removals and %d unresolved ids, want some of each",
			len(history.Transfers), len(history.Removals), len(history.Unresolved))
	}
	if got := readings.Load(); got != 1 {
		t.Errorf("one yield history read the ledger's transactions %d times, want once", got)
	}
}

// planRow is a row of EXPLAIN QUERY PLAN.
type planRow struct {
	parent int
	detail string
}

func queryPlan(t *testing.T, path, query string) map[int]planRow {
	t.Helper()
	database, err := ledger.OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	rows, err := database.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	plan := map[int]planRow{}
	for rows.Next() {
		var id, unused int
		var row planRow
		if err := rows.Scan(&id, &row.parent, &unused, &row.detail); err != nil {
			t.Fatal(err)
		}
		plan[id] = row
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return plan
}

// materializedAround is the nearest materialization the plan row sits in, or
// the empty text when it is in none.
func materializedAround(plan map[int]planRow, id int) string {
	for row, found := plan[id]; found; row, found = plan[row.parent] {
		if strings.HasPrefix(row.detail, "MATERIALIZE ") {
			return row.detail
		}
	}
	return ""
}

// The statement reads only transaction documents, and every walk over all of
// them goes through the documents' index by kind alone: one for the transfer
// documents and one for the pending pairs, each inside a CTE evaluated once.
// Any other walk would be another pass over the ledger's transactions.
func TestTheTransferHistoryWalksTheTransactionsOnceForTheDocumentsAndOnceForThePairs(t *testing.T) {
	plan := queryPlan(t, transferHistoryLedger(t).Path, ledger.AllQueries()["queryYieldTransferHistory"])
	walks := map[string]int{}
	materializations := map[string]int{}
	for id, row := range plan {
		if strings.Contains(row.detail, "sqlite_autoindex_documents_1 (kind=?)") {
			walks[materializedAround(plan, id)]++
		}
		if strings.HasPrefix(row.detail, "MATERIALIZE ") {
			materializations[row.detail]++
		}
	}
	want := map[string]int{"MATERIALIZE reserve_transfer_documents": 1, "MATERIALIZE pairs": 1}
	if !reflect.DeepEqual(walks, want) {
		t.Errorf("the walks over the transaction documents sit in %v, want one in each CTE: %v", walks, want)
	}
	for name := range want {
		if materializations[name] != 1 {
			t.Errorf("%q appears %d times in the plan, want once", name, materializations[name])
		}
	}
}

// readsTheLedgersTransactions says whether a statement reads the ledger's
// transactions: the current-state table, the documents of that kind with their
// versions, or one of the collector's views over them.
var readsTheLedgersTransactions = regexp.MustCompile(
	`\b(FROM|JOIN) transactions\b|documents \w+ ON [^\n]*kind = 'transactions'|pending_posted_candidates|transaction_versions|document_coverage`)

// Every statement of the package is a registered constant, and the yield
// history's are the queryYield ones: one of them reads the ledger's
// transactions, and a second would be a second pass over them that the count
// of readings does not see.
func TestOneStatementOfTheYieldHistoryReadsTheLedgersTransactions(t *testing.T) {
	var reading []string
	for name, query := range ledger.AllQueries() {
		if strings.HasPrefix(name, "queryYield") && readsTheLedgersTransactions.MatchString(query) {
			reading = append(reading, name)
		}
	}
	slices.Sort(reading)
	if want := []string{"queryYieldTransferHistory"}; !slices.Equal(reading, want) {
		t.Errorf("the yield history's statements that read the ledger's transactions are %v, want %v", reading, want)
	}
}

// The collector renames a new snapshot over the file: a view opened before
// keeps its own, and a view opened after reads the new one, so nothing of the
// history outlives the view that read it.
func TestAViewOpenedAfterTheFileWasReplacedReadsTheNewTransferHistory(t *testing.T) {
	f := transferHistoryLedger(t)
	reader := ledger.New(f.Path)
	first := reader.View()
	defer first.Close()
	before, err := first.YieldHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	replacement := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 120, CDIRatio: 1}})
	walkReachesBack(t, replacement)
	addReserveTransfer(t, replacement, "tx-new", "Dinheiro reservado Trip", -20)
	renameOver(t, replacement, f.Path)

	if again, err := first.YieldHistory(t.Context()); err != nil || !reflect.DeepEqual(again, before) {
		t.Errorf("the first view answered another history after the rename (%v)", err)
	}
	later := reader.View()
	defer later.Close()
	after, err := later.YieldHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Transfers) != 1 || after.Transfers[0].ProviderID != "tx-new" || len(after.Removals) != 0 || len(after.Unresolved) != 0 {
		t.Errorf("a view opened after the rename holds %d versions, %d removals and %d unresolved ids, want tx-new alone",
			len(after.Transfers), len(after.Removals), len(after.Unresolved))
	}
}
