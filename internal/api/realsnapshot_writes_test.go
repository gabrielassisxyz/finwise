package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/contract"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// The write side over a real snapshot, opt-in, and the same series over the
// invented ledger in every run. The snapshot is opened read-only. The app's
// database is a scratch finwise.db in the test's temporary directory, which
// holds real values from the first ingest on: account ids, days, amounts and
// establishment keys. Go removes that directory when the test ends, pass or
// fail; if the test binary is killed it survives under the system's temporary
// directory, in a directory named after the test, and is the person's to
// delete. Every comparison happens in memory. Success logs counts; failure says
// fixed sentences built from the test's own constants, counts, statuses, error
// kinds, refusal reasons, table names and Go types, never from a body, a row,
// an error's text, a month, a day, an id or a key.

func TestTheWriteSideOverARealSnapshot(t *testing.T) {
	observed := runWriteSeries(t, realSnapshotPath(t))
	for _, problem := range problemsOfSeries(observed) {
		t.Error(problem)
	}
	logSeries(t, observed)
}

// skippedOverTheInventedLedger is how many steps find no target over the full
// invented ledger: no warning holds for its subscription, so none is dismissed.
const skippedOverTheInventedLedger = 1

func TestTheWriteSeriesHoldsOverTheInventedLedger(t *testing.T) {
	observed := runWriteSeries(t, newFullLedger(t).Path)
	for _, problem := range problemsOfSeries(observed) {
		t.Error(problem)
	}
	if skipped := observed.steps.skippedCount(); skipped != skippedOverTheInventedLedger {
		t.Errorf("%d steps found no target over the invented ledger, want %d: %v", skipped, skippedOverTheInventedLedger, observed.steps.skipped)
	}
	// The state that matters holds a split, a linked reimbursement, an exclusion,
	// a refund and a closed month at once. The month is too small to keep a
	// review waiting or a fee alone once the series has acted on it.
	shown := dispositionsOf(observed.inForce)
	for _, disposition := range []budgetrules.Disposition{
		budgetrules.Movement, budgetrules.Excluded, budgetrules.Split, budgetrules.Reimbursement,
		budgetrules.Refund, budgetrules.OnALine, budgetrules.Uncategorised,
	} {
		if shown[disposition.String()] == 0 {
			t.Errorf("with the series in force no transaction is %v", disposition)
		}
	}
	if observed.closedInForce != 1 {
		t.Errorf("with the series in force %d months are closed, want one", observed.closedInForce)
	}
	steps := observed.steps
	if steps.recorded != 47 || steps.extensions != 1 || steps.nulls != 7 || steps.evaluations != 1 || len(observed.inForce) != len(fullLedgerMonths) {
		t.Errorf("%d changes recorded, %d extended, %d answers of null and %d evaluations over %d months, want 47, 1, 7 and 1 over %d",
			steps.recorded, steps.extensions, steps.nulls, steps.evaluations, len(observed.inForce), len(fullLedgerMonths))
	}
	logSeries(t, observed)
}

// runWriteSeries is the order of the run: the database without a budget, the
// first budget, the series, the state with all of it in force, every change
// undone newest first, and what is left.
func runWriteSeries(t *testing.T, ledgerPath string) seriesObservation {
	t.Helper()
	run := startSeries(t, ledgerPath)
	run.performSeries(seriesSteps)
	run.observeInForce()
	run.undoChanges(run.ids)
	run.observeEnd()
	return run.observed
}

// seriesRun is one run of the series: the router over the snapshot and the
// scratch database, and what it has seen so far. month is the current month
// and is never printed.
type seriesRun struct {
	t          *testing.T
	env        *writeEnv
	ledgerPath string
	clock      time.Time
	month      brday.Month
	previous   brday.Month
	months     []brday.Month
	saves      []ingestAfterSave
	// ids are the changes the series recorded, oldest first; lineOrigin is the
	// last line change, which "all purchases here" extends.
	ids        []string
	lineOrigin string
	observed   seriesObservation
}

// ingestAfterSave is what one run of the ingest after a budget save gave.
type ingestAfterSave struct {
	counts ingest.Counts
	failed bool
}

// startSeries creates the scratch database at the instant of the ledger's
// latest listed day, takes the snapshot in without a budget, checks every month,
// saves the first budget and takes the starting point.
func startSeries(t *testing.T, ledgerPath string) *seriesRun {
	t.Helper()
	run := &seriesRun{t: t, ledgerPath: ledgerPath}
	run.readLedger()
	scratch := authoredtest.Open(withheld{T: t, step: "creating the scratch database"}, func() time.Time { return run.clock })
	run.env = newSeriesServer(t, ledgerPath, scratch, run.clock, run.ingestAfterSave)
	run.observed.steps.skipped = map[string]int{}
	run.observed.ingest.withoutBudget = run.ingest("the run with no budget", ingest.TriggerStart)
	run.observed.fresh = run.monthsUnderCheck()
	run.setUp()
	return run
}

// readLedger reads, in memory, the months the ledger holds and the latest day
// it lists, which places today.
func (run *seriesRun) readLedger() {
	snapshot := viewOf(run.t, run.ledgerPath)
	ctx := run.t.Context()
	rows, err := snapshot.CleanTransactions(ctx, wholeLedger)
	withoutValues(run.t, "reading the ledger", err)
	whole, _, err := listedLedger(ctx, snapshot)
	withoutValues(run.t, "interpreting the ledger", err)
	if len(whole.listed) == 0 {
		run.t.Fatal("the ledger lists no transaction")
	}
	var latest brday.Day
	for _, listed := range whole.listed {
		latest = max(latest, listed.Day)
	}
	day, err := time.Parse(time.DateOnly, string(latest))
	withoutValues(run.t, "reading the latest listed day", err)
	run.clock = day.Add(15 * time.Hour)
	run.month = brday.MonthOf(run.clock)
	run.months = monthsOf(rows)
	for _, month := range run.months {
		if month < run.month {
			run.previous = month
		}
	}
}

// setUp saves the first budget, which runs the ingest as the server does, and
// observes what that run left before the series starts.
func (run *seriesRun) setUp() {
	response := run.env.post("budget.save", jsonArguments(run.t, string(run.month), seriesBudget()))
	if _, recorded := changeIDIn(response.Body.Bytes()); response.Code != http.StatusOK || !recorded {
		run.t.Fatalf("the first budget save answered %d", response.Code)
	}
	observed := &run.observed.ingest
	observed.savesAfterFirstBudget = len(run.saves)
	if len(run.saves) > 0 {
		observed.afterFirstBudget = run.saves[0].counts
	}
	observed.overviewPending = run.shell().PendingCount
	start, err := run.env.scratch.Database.TrackingStartMonth(run.t.Context())
	withoutValues(run.t, "reading the tracking start month", err)
	for _, item := range run.queue() {
		observed.queueLength++
		if item.Pending != nil && item.Pending.Suggestion != nil {
			observed.suggested++
		}
		if monthOfDay(brday.Day(item.Day)) < start {
			observed.beforeStart++
		}
	}
	changes, dump, index := len(run.changes()), run.canonical(), run.index()
	run.ingest("the second run", ingest.TriggerCommand)
	observed.second = secondRun{
		changesAdded:   len(run.changes()) - changes,
		dumpChanged:    run.canonical() != dump,
		indexRowsAdded: run.index().rows - index.rows,
	}
	run.observed.startDump, run.observed.startIndex = run.canonical(), run.index()
	run.observed.startLast = newestChangeOf(run.changes())
}

// observeInForce checks every month with every change of the series in force,
// and reads the journal's authors since the starting point.
func (run *seriesRun) observeInForce() {
	run.observed.inForce = run.monthsUnderCheck()
	run.observed.changes = run.changes()
	run.observed.closedInForce = len(run.shell().ClosedMonths)
	for _, save := range run.saves {
		if save.failed {
			run.observed.ingest.failedAfterASave++
		}
	}
}

// monthsUnderCheck answers every month the ledger holds by both paths: the
// month function's dispositions and the list's ids and amounts.
func (run *seriesRun) monthsUnderCheck() []monthUnderCheck {
	ctx := withNow(run.t.Context(), func() time.Time { return run.clock })
	views := Views{Ledger: viewOf(run.t, run.ledgerPath), Authored: run.env.scratch.Database.View()}
	defer views.Authored.Close()
	months := make([]monthUnderCheck, 0, len(run.months))
	for _, month := range run.months {
		disposed, err := disposeMonth(ctx, views, month)
		withoutValues(run.t, "disposing a month", err)
		checked := monthUnderCheck{ids: []string{}, amounts: []money.Cents{}, disposed: disposed}
		for _, row := range run.listOf(month) {
			checked.ids, checked.amounts = append(checked.ids, row.ID), append(checked.amounts, row.cents)
		}
		months = append(months, checked)
	}
	return months
}

// undoChanges takes back the changes given, newest first.
func (run *seriesRun) undoChanges(ids []string) {
	for i := len(ids) - 1; i >= 0; i-- {
		run.observed.undos = append(run.observed.undos, undoOutcomeOf(run.env.undo(ids[i])))
	}
}

func (run *seriesRun) observeEnd() {
	run.observed.endDump, run.observed.endIndex = run.canonical(), run.index()
}

// ingest takes the snapshot in, failing with the kind of a failure alone.
func (run *seriesRun) ingest(step string, trigger ingest.Trigger) ingest.Counts {
	run.t.Helper()
	sources := ingest.Sources{Ledger: ledger.New(run.ledgerPath), Database: run.env.scratch.Database}
	counts, err := ingest.Run(run.t.Context(), sources, trigger, run.clock)
	if err != nil {
		failedWithItsKind(run.t, step, err)
	}
	return counts
}

// ingestAfterSave is the server's hook after a committed budget save: it runs
// the ingest at the request's instant and keeps what it counted.
func (run *seriesRun) ingestAfterSave(ctx context.Context, at time.Time) {
	sources := ingest.Sources{Ledger: ledger.New(run.ledgerPath), Database: run.env.scratch.Database}
	counts, err := ingest.Run(ctx, sources, ingest.TriggerBudgetChange, at)
	run.saves = append(run.saves, ingestAfterSave{counts: counts, failed: err != nil})
}

func failedWithItsKind(t *testing.T, step string, err error) {
	t.Helper()
	if kind, classified := ingest.FailureOf(err); classified {
		t.Fatalf("%s failed with the kind %s", step, kind)
	}
	t.Fatalf("%s failed (%T); the message is withheld because it may name a real value", step, err)
}

func (run *seriesRun) canonical() string {
	dump, err := run.env.scratch.Database.CanonicalDump(run.t.Context())
	withoutValues(run.t, "the canonical dump", err)
	return dump
}

func (run *seriesRun) changes() []authoredtest.Change {
	return run.env.scratch.Changes(withheld{T: run.t, step: "reading the journal"})
}

func (run *seriesRun) everything() string {
	return run.env.scratch.Everything(withheld{T: run.t, step: "reading every table"})
}

func (run *seriesRun) index() indexSnapshot {
	view := run.env.scratch.Database.View()
	defer view.Close()
	rows, err := view.IndexedTransactions(run.t.Context())
	withoutValues(run.t, "reading the index", err)
	return indexSnapshotOf(rows)
}

func newestChangeOf(changes []authoredtest.Change) int64 {
	var last int64
	for _, change := range changes {
		last = max(last, change.ID)
	}
	return last
}

// read answers a read through the router and requires 200.
func (run *seriesRun) read(method, arguments string) []byte {
	run.t.Helper()
	response := run.env.post(method, arguments)
	if response.Code != http.StatusOK {
		run.t.Fatalf("%s answered %d", method, response.Code)
	}
	return response.Body.Bytes()
}

func (run *seriesRun) decode(method string, body []byte, into any) {
	run.t.Helper()
	if err := json.Unmarshal(body, into); err != nil {
		run.t.Fatalf("%s gave an answer the series cannot read", method)
	}
}

func (run *seriesRun) args(values ...any) string {
	return jsonArguments(run.t, values...)
}

// jsonArguments is the JSON array a client posts.
func jsonArguments(t *testing.T, values ...any) string {
	t.Helper()
	if values == nil {
		values = []any{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		t.Fatal("the arguments of a step do not encode")
	}
	return string(encoded)
}

// performSeries makes every step in order and keeps the id of each change.
func (run *seriesRun) performSeries(steps []seriesStep) {
	handled := NewReadRegistry().Names()
	for i, step := range steps {
		run.perform(i+1, step, slices.Contains(handled, step.method))
	}
}

func (run *seriesRun) perform(number int, step seriesStep, handled bool) {
	tally := &run.observed.steps
	if !handled {
		tally.skipped[missingHandler]++
		return
	}
	arguments, found := step.target(run)
	if !found {
		tally.skipped[step.missing]++
		return
	}
	changes, file := 0, ""
	switch step.mode {
	case stepNull:
		changes = len(run.changes())
	case stepEvaluation:
		file = run.everything()
	}
	o := stepObservationOf(step.method, run.env.post(step.method, arguments))
	o.origin = run.lineOrigin
	switch step.mode {
	case stepNull:
		o.changesAdded = len(run.changes()) - changes
	case stepEvaluation:
		o.fileChanged = run.everything() != file
	}
	// Taking one step back is refused, by the undo's own rules, when anything it
	// would restore changed since; that is counted, not failed.
	if step.method == "transactions.undoHistoryStep" && o.status == http.StatusConflict && o.reason == string(reasonStaleChange) {
		tally.skipped[missingStale]++
		return
	}
	if problem := stepProblem(number, step, o); problem != "" {
		tally.failures = append(tally.failures, problem)
		return
	}
	run.count(step, o.changeID)
}

func (run *seriesRun) count(step seriesStep, changeID string) {
	tally := &run.observed.steps
	switch step.mode {
	case stepRecorded:
		run.ids = append(run.ids, changeID)
		tally.recorded++
		if step.method == lineChangeOrigin {
			run.lineOrigin = changeID
		}
	case stepExtension:
		tally.extensions++
	case stepNull:
		tally.nulls++
	case stepEvaluation:
		tally.evaluations++
	}
}

// newSeriesServer is the server's own registry over the snapshot and the
// scratch database, its clock fixed and its request log kept in memory, with a
// session open. It is built here rather than by newTestEnv, whose failure would
// print an error's text.
func newSeriesServer(t *testing.T, ledgerPath string, scratch *authoredtest.Scratch, now time.Time, afterSave func(context.Context, time.Time)) *writeEnv {
	t.Helper()
	methods, err := contract.Load()
	withoutValues(t, "loading the manifest", err)
	env := &testEnv{t: t, clock: newFakeClock(), reads: NewReadRegistry(), logs: &bytes.Buffer{}, methods: methods}
	env.handler, err = NewHandler(Config{
		Methods: methods, Reads: env.reads, Ledger: ledger.New(ledgerPath), Database: scratch.Database,
		Now: func() time.Time { return now }, Logger: slog.New(slog.NewTextHandler(env.logs, nil)),
		VerifyPIN: func(attempt string) bool { return attempt == testPIN }, AfterBudgetSaved: afterSave,
	})
	withoutValues(t, "building the router", err)
	return &writeEnv{testEnv: env, scratch: scratch, cookie: env.unlock()}
}

// withheld is the test's handle for a helper of authoredtest, whose failures
// quote an error's text: whatever such a helper fails or logs with, the test
// fails with the step's name alone and logs nothing.
type withheld struct {
	*testing.T
	step string
}

func (handle withheld) withheldSentence() string {
	return handle.step + " failed; the message is withheld because it may name a real value"
}

func (handle withheld) Fatalf(string, ...any) {
	handle.T.Helper()
	handle.T.Fatal(handle.withheldSentence())
}

func (handle withheld) Fatal(...any) {
	handle.T.Helper()
	handle.T.Fatal(handle.withheldSentence())
}

func (handle withheld) Errorf(string, ...any) {
	handle.T.Helper()
	handle.T.Error(handle.withheldSentence())
}

func (handle withheld) Error(...any) {
	handle.T.Helper()
	handle.T.Error(handle.withheldSentence())
}

func (handle withheld) Skipf(string, ...any) {
	handle.T.Helper()
	handle.T.Skip(handle.step + " was skipped")
}

func (handle withheld) Skip(...any) {
	handle.T.Helper()
	handle.T.Skip(handle.step + " was skipped")
}

func (handle withheld) Logf(string, ...any) {}

func (handle withheld) Log(...any) {}

func (handle withheld) Output() io.Writer { return io.Discard }

// logSeries logs what the run did, in counts.
func logSeries(t *testing.T, observed seriesObservation) {
	t.Helper()
	counts := observed.ingest.afterFirstBudget
	t.Logf("months=%d listed_with_the_series_in_force=%d steps=%d recorded=%d answered_null=%d evaluated=%d skipped=%d undos=%d",
		len(observed.inForce), listedIn(observed.inForce), len(seriesSteps), observed.steps.recorded, observed.steps.nulls,
		observed.steps.evaluations, observed.steps.skippedCount(), len(observed.undos))
	t.Logf("skipped_for_lack_of_a_target: %s", render(observed.steps.skipped))
	t.Logf("in_force: closed_months=%d dispositions: %s", observed.closedInForce, render(dispositionsOf(observed.inForce)))
	t.Logf("first_ingest: indexed=%d pending_unnamed_person=%d pending_new_establishment=%d left=%d nothing_to_store=%d already_decided=%d queue=%d",
		counts.Indexed, counts.PendingUnnamedPerson, counts.PendingNewEstablishment, counts.Left, counts.NothingToStore,
		counts.AlreadyDecided, observed.ingest.queueLength)
	t.Logf("index_rows_before=%d after=%d provider_ids_before=%d after=%d changes_in_the_journal=%d",
		observed.startIndex.rows, observed.endIndex.rows, observed.startIndex.idRows, observed.endIndex.idRows, len(observed.changes))
}

// dispositionsOf counts the transactions of every month by disposition.
func dispositionsOf(months []monthUnderCheck) map[string]int {
	counts := map[string]int{}
	for _, month := range months {
		for _, outcome := range month.disposed.outcomes {
			counts[outcome.Disposition.String()]++
		}
	}
	return counts
}

func listedIn(months []monthUnderCheck) int {
	count := 0
	for _, month := range months {
		count += len(month.ids)
	}
	return count
}
