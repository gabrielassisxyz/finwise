package api

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/contract"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// The checks of the write series. Each reads what a run observed, held in
// memory, and answers what is wrong in sentences built from its own words,
// counts, statuses, error kinds and refusal reasons alone: never a month, a
// day, an id, a key, an amount, a body or an error's text. That is what makes
// them safe over a real ledger, and each is fed a break of its own over the
// invented ledger to show it can fail.

// seriesObservation is everything one run of the series saw.
type seriesObservation struct {
	ingest  ingestObservation
	fresh   []monthUnderCheck
	inForce []monthUnderCheck
	steps   stepTally
	// startLast is the newest change at the starting point; changes is the
	// journal with every change of the series in force.
	startLast            int64
	changes              []authoredtest.Change
	closedInForce        int
	undos                []undoOutcome
	startDump, endDump   string
	startIndex, endIndex indexSnapshot
}

// problemsOfSeries runs every check over one run.
func problemsOfSeries(o seriesObservation) []string {
	return slices.Concat(
		ingestProblems(o.ingest),
		invariantsProblems("on the fresh database", o.fresh),
		o.steps.failures,
		invariantsProblems("with every change of the series in force", o.inForce),
		ruleChangeProblems(o.changes, o.startLast),
		undoProblems(o.undos, o.steps.recorded),
		seriesRestoredProblems(o.startDump, o.endDump),
		indexProblems(o.startIndex, o.endIndex),
	)
}

// monthUnderCheck is one month as two paths answer it: the ids and amounts of
// transactions.list, and the dispositions of the month function.
type monthUnderCheck struct {
	ids      []string
	amounts  []money.Cents
	disposed monthDispositions
}

// invariantsProblems counts the months whose dispositions break either
// invariant. invariantProblems writes the ids and amounts it found into its
// sentences, so only how many months it complained about leaves here; it is
// given no month, so not even that is in what it builds.
func invariantsProblems(moment string, months []monthUnderCheck) []string {
	broken := 0
	for _, month := range months {
		if len(invariantProblems("", month.ids, month.amounts, month.disposed)) > 0 {
			broken++
		}
	}
	if broken == 0 {
		return nil
	}
	return []string{fmt.Sprintf("%s, %d of %d months break an invariant of their dispositions", moment, broken, len(months))}
}

// ingestObservation is what the ingest counted before and after the first
// budget, and what the reads answered right after it.
type ingestObservation struct {
	withoutBudget         ingest.Counts
	afterFirstBudget      ingest.Counts
	savesAfterFirstBudget int
	failedAfterASave      int
	overviewPending       int
	queueLength           int
	suggested             int
	beforeStart           int
	second                secondRun
}

// secondRun is what a run right after the first one left.
type secondRun struct {
	changesAdded, indexRowsAdded int
	dumpChanged                  bool
}

// ingestProblems holds the counts to what a database nobody has put a
// transaction on a line in leaves room for.
func ingestProblems(o ingestObservation) []string {
	var problems []string
	add := func(format string, counts ...any) { problems = append(problems, fmt.Sprintf(format, counts...)) }
	if o.withoutBudget != (ingest.Counts{}) {
		add("the run with no budget counted something")
	}
	if o.savesAfterFirstBudget != 1 || o.failedAfterASave != 0 {
		add("the first budget save ran the ingest %d times, and %d runs after a save failed", o.savesAfterFirstBudget, o.failedAfterASave)
	}
	counts := o.afterFirstBudget
	if counts.Applied != 0 || counts.PendingLowConfidence != 0 {
		add("the run after the first budget applied %d lines by rule and asked about %d of low confidence, want none", counts.Applied, counts.PendingLowConfidence)
	}
	if counts.Paired != 0 || counts.Unpaired != 0 {
		add("the run after the first budget paired %d and left %d unpaired, want none", counts.Paired, counts.Unpaired)
	}
	if o.suggested != 0 {
		add("%d items of the queue carry a suggestion, want none", o.suggested)
	}
	pending := counts.PendingLowConfidence + counts.PendingUnnamedPerson + counts.PendingNewEstablishment
	if pending != o.overviewPending || pending != o.queueLength {
		add("the run put %d transactions in the queue, shell.overview counts %d and review.weekly lists %d", pending, o.overviewPending, o.queueLength)
	}
	if o.beforeStart != 0 {
		add("%d items of the queue are dated before the tracking start month", o.beforeStart)
	}
	if second := o.second; second.changesAdded != 0 || second.indexRowsAdded != 0 || second.dumpChanged {
		add("a second run added %d changes and %d index rows, and changed the canonical dump: %t", second.changesAdded, second.indexRowsAdded, second.dumpChanged)
	}
	return problems
}

// stepTally is what the steps of the series did. failures are fixed sentences.
type stepTally struct {
	recorded, extensions, nulls, evaluations, proposals int
	skipped                                             map[string]int
	failures                                            []string
}

func (tally stepTally) skippedCount() int {
	count := 0
	for _, skipped := range tally.skipped {
		count += skipped
	}
	return count
}

// stepObservation is one step's answer, reduced to what its check reads: the
// status, the kind and reason of a refusal from their closed lists, and what
// the journal and the file did around it. The body itself is never kept.
type stepObservation struct {
	status           int
	kind, reason     string
	null             bool
	changeID         string
	proposalID       string
	origin           string
	changesAdded     int
	fileChanged      bool
	canonicalChanged bool
	conforms         bool
}

func proposalIDIn(body []byte) string {
	var answer struct {
		ProposalID string `json:"proposalId"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return ""
	}
	return answer.ProposalID
}

func stepObservationOf(method string, response *httptest.ResponseRecorder) stepObservation {
	o := stepObservation{status: response.Code}
	body := response.Body.Bytes()
	if response.Code != http.StatusOK {
		o.kind, o.reason = refusalOf(body)
		return o
	}
	o.null = strings.TrimSpace(string(body)) == "null"
	o.changeID, _ = changeIDIn(body)
	o.proposalID = proposalIDIn(body)
	o.conforms = conformanceProblem(method, body) == ""
	return o
}

// refusalOf is the kind and reason of an error envelope, each only when it is
// one of the contract's closed list.
func refusalOf(body []byte) (kind, reason string) {
	var envelope errorEnvelope
	if json.Unmarshal(body, &envelope) != nil {
		return "with no error envelope", ""
	}
	kind = "of another kind"
	if _, known := statusOfKind[envelope.Error.Kind]; known {
		kind = string(envelope.Error.Kind)
	}
	if slices.Contains(refusalReasons, envelope.Error.Reason) {
		reason = string(envelope.Error.Reason)
	}
	return kind, reason
}

// stepProblem is what is wrong with one step's answer, named by the step's
// number and method.
func stepProblem(number int, step seriesStep, o stepObservation) string {
	label := fmt.Sprintf("step %d (%s)", number, step.method)
	if o.status != http.StatusOK {
		return fmt.Sprintf("%s answered %d %s %s", label, o.status, o.kind, o.reason)
	}
	switch step.mode {
	case stepRecorded:
		if o.changeID == "" {
			return label + " answered without the id of a change"
		}
	case stepExtension:
		if o.changeID == "" || o.changeID != o.origin {
			return label + " answered another change than the line change it extends"
		}
	case stepNull:
		if !o.null || o.changesAdded != 0 {
			return fmt.Sprintf("%s answered null: %t, and added %d changes to the journal", label, o.null, o.changesAdded)
		}
	case stepEvaluation:
		if !o.conforms || o.fileChanged {
			return fmt.Sprintf("%s conforms: %t, and changed a table of the file: %t (the answer is withheld)", label, o.conforms, o.fileChanged)
		}
	case stepProposal:
		if o.null || !o.conforms || o.changesAdded != 0 || o.canonicalChanged {
			return fmt.Sprintf("%s conforms: %t, null: %t, added %d changes, changed journaled tables: %t", label, o.conforms, o.null, o.changesAdded, o.canonicalChanged)
		}
	}
	return ""
}

// ruleChangeProblems counts the changes by the rule since the starting point:
// every transaction the rules evaluate was evaluated before the series.
func ruleChangeProblems(changes []authoredtest.Change, startLast int64) []string {
	count := 0
	for _, change := range changes {
		if change.ID > startLast && change.Author == string(authored.AuthorRule) {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	return []string{fmt.Sprintf("the journal gained %d changes by the rule during the series, want none", count)}
}

// undoOutcome is one undo's answer: 200 null, or the kind and reason it was
// refused with.
type undoOutcome struct {
	status       int
	undone       bool
	kind, reason string
}

func undoOutcomeOf(response *httptest.ResponseRecorder) undoOutcome {
	outcome := undoOutcome{status: response.Code}
	if response.Code == http.StatusOK {
		outcome.undone = strings.TrimSpace(response.Body.String()) == "null"
		return outcome
	}
	outcome.kind, outcome.reason = refusalOf(response.Body.Bytes())
	return outcome
}

// undoProblems requires every change the series recorded to be undone. One the
// engine refuses by its own rules, as stale, is counted apart: newest first, no
// later change stands in the way, so that refusal too is a defect.
func undoProblems(outcomes []undoOutcome, recorded int) []string {
	var failed, stale int
	var firstFailure undoOutcome
	for _, outcome := range outcomes {
		if outcome.status == http.StatusOK && outcome.undone {
			continue
		}
		if failed == 0 {
			firstFailure = outcome
		}
		failed++
		if outcome.reason == string(reasonStaleChange) {
			stale++
		}
	}
	var problems []string
	if len(outcomes) != recorded {
		problems = append(problems, fmt.Sprintf("%d undos were asked for %d recorded changes", len(outcomes), recorded))
	}
	if failed > 0 {
		problems = append(problems, fmt.Sprintf("%d of %d undos did not answer 200 null, %d of them refused as stale by the engine's own rules; the first answered %d %s %s",
			failed, len(outcomes), stale, firstFailure.status, firstFailure.kind, firstFailure.reason))
	}
	return problems
}

// seriesRestoredProblems compares the two dumps whole and says only whether
// they differ: a difference of two dumps is real rows.
func seriesRestoredProblems(before, after string) []string {
	if before == after {
		return nil
	}
	return []string{"once every change of the series is undone, the journaled tables are not what they were before the first step"}
}

// indexSnapshot is the index as sets of its keys and provider ids, held in
// memory, with the row count of each table.
type indexSnapshot struct {
	keys         map[authored.TxKey]bool
	providerIDs  map[string]bool
	rows, idRows int
}

func indexSnapshotOf(rows []authored.IndexedTransaction) indexSnapshot {
	snapshot := indexSnapshot{keys: map[authored.TxKey]bool{}, providerIDs: map[string]bool{}, rows: len(rows)}
	for _, row := range rows {
		snapshot.keys[row.Key] = true
		for _, id := range row.ProviderIDs {
			snapshot.providerIDs[id] = true
			snapshot.idRows++
		}
	}
	return snapshot
}

// indexProblems requires the index only to have grown: an undo leaves the index
// rows a write created or refreshed.
func indexProblems(before, after indexSnapshot) []string {
	var lostKeys, lostIDs int
	for key := range before.keys {
		if !after.keys[key] {
			lostKeys++
		}
	}
	for id := range before.providerIDs {
		if !after.providerIDs[id] {
			lostIDs++
		}
	}
	if lostKeys == 0 && lostIDs == 0 && after.rows >= before.rows && after.idRows >= before.idRows {
		return nil
	}
	return []string{fmt.Sprintf("the index lost %d keys and %d provider ids; tx_index went from %d rows to %d and tx_ids from %d to %d",
		lostKeys, lostIDs, before.rows, after.rows, before.idRows, after.idRows)}
}

// messagesUnderEveryBreak runs the series over the invented ledger once, with
// the oldest change kept out of the undos for a moment, and feeds every check a
// break of its own. It answers what each check said, by break.
func messagesUnderEveryBreak(t *testing.T) map[string][]string {
	t.Helper()
	run := startSeries(t, newFullLedger(t).Path)
	run.performSeries(seriesSteps)
	run.observeInForce()
	run.undoChanges(run.ids[1:])
	leftOut := run.canonical()
	run.undoChanges(run.ids[:1])
	run.observeEnd()
	o := run.observed
	if problems := problemsOfSeries(o); len(problems) != 0 {
		t.Fatalf("the control failed: %v", problems)
	}
	pendingOffByOne := o.ingest
	pendingOffByOne.overviewPending++
	recorded := seriesStep{method: "transactions.setSplit", mode: stepRecorded}
	return map[string][]string{
		"one change left out of the undos":                    seriesRestoredProblems(o.startDump, leftOut),
		"one transaction dropped from a month's dispositions": invariantsProblems("with every change of the series in force", withADispositionDropped(t, o.inForce)),
		"one split part left out of a debit's sum":            invariantsProblems("with every change of the series in force", withASplitPartLost(t, o.inForce)),
		"one key removed from the index":                      indexProblems(o.startIndex, withAKeyRemoved(t, o.startIndex, o.endIndex)),
		"one pending count off by one":                        ingestProblems(pendingOffByOne),
		"one change by the rule added during the series":      ruleChangeProblems(withAChangeByTheRule(t, run), o.startLast),
		"one undo refused as stale":                           undoProblems(withAStaleUndo(o.undos), o.steps.recorded),
		"a step that expected a change refused":               collect(stepProblem(7, recorded, stepObservation{status: http.StatusConflict, kind: "refused", reason: "not_found"})),
		"a step that answered null and added a change":        collect(stepProblem(8, seriesStep{method: "review.approve", mode: stepNull}, stepObservation{status: http.StatusOK, null: true, changesAdded: 1})),
		"an evaluation that changed the file":                 collect(stepProblem(9, seriesStep{method: "budget.evaluateScenario", mode: stepEvaluation}, stepObservation{status: http.StatusOK, conforms: true, fileChanged: true})),
	}
}

func withADispositionDropped(t *testing.T, months []monthUnderCheck) []monthUnderCheck {
	t.Helper()
	broken := slices.Clone(months)
	for i, month := range broken {
		if len(month.disposed.listed) > 0 {
			broken[i].disposed = monthDispositions{listed: month.disposed.listed[1:], outcomes: month.disposed.outcomes[1:]}
			return broken
		}
	}
	t.Fatal("no month holds a transaction to drop")
	return nil
}

func withASplitPartLost(t *testing.T, months []monthUnderCheck) []monthUnderCheck {
	t.Helper()
	broken := slices.Clone(months)
	for i, month := range broken {
		for j, outcome := range month.disposed.outcomes {
			if outcome.Disposition != budgetrules.Split || len(outcome.Lines) == 0 {
				continue
			}
			outcomes := slices.Clone(month.disposed.outcomes)
			outcomes[j].Lines = outcome.Lines[1:]
			broken[i].disposed = monthDispositions{listed: month.disposed.listed, outcomes: outcomes}
			return broken
		}
	}
	t.Fatal("no split is in force to lose a part of")
	return nil
}

// withAKeyRemoved takes out of the index the oldest key it held at the starting
// point, with its row.
func withAKeyRemoved(t *testing.T, start, index indexSnapshot) indexSnapshot {
	t.Helper()
	keys := slices.Sorted(maps.Keys(start.keys))
	if len(keys) == 0 || !index.keys[keys[0]] {
		t.Fatal("the index holds no key of the starting point to remove")
	}
	broken := index
	broken.keys = maps.Clone(index.keys)
	delete(broken.keys, keys[0])
	broken.rows--
	return broken
}

// withAChangeByTheRule records a change by the rule in the scratch database,
// as an ingest that evaluated a transaction a second time would, and answers
// the journal.
func withAChangeByTheRule(t *testing.T, run *seriesRun) []authoredtest.Change {
	t.Helper()
	ctx := t.Context()
	facts := authored.ChangeFacts{Author: authored.AuthorRule, Method: ingest.Method, At: run.clock}
	err := run.env.scratch.Database.Change(ctx, facts, func(recorder *authored.Recorder) error {
		return recorder.Set(ctx, authoredtest.Facts, []authored.Value{authored.Text("fact-by-the-rule")}, authored.Row{"amount": authored.Integer(1)})
	})
	if err != nil {
		t.Fatal(err)
	}
	return run.changes()
}

func withAStaleUndo(outcomes []undoOutcome) []undoOutcome {
	broken := slices.Clone(outcomes)
	broken[len(broken)-1] = undoOutcome{status: http.StatusConflict, kind: "refused", reason: string(reasonStaleChange)}
	return broken
}

func TestEachCheckOfTheSeriesComplainsAboutItsOwnBreak(t *testing.T) {
	breaks := messagesUnderEveryBreak(t)
	for label, messages := range breaks {
		if len(messages) == 0 {
			t.Errorf("%s went unnoticed", label)
		}
	}
	// A refusal is reported by the method, the status and the reason.
	refused := strings.Join(breaks["a step that expected a change refused"], " ")
	for _, part := range []string{"transactions.setSplit", "409", "refused", "not_found"} {
		if !strings.Contains(refused, part) {
			t.Errorf("the refused step is reported without %q", part)
		}
	}
}

// The months and the latest day of the invented ledger: a month and a day are
// values too.
var inventedLedgerDates = []string{"2026-01", "2026-03", "2026-03-19"}

func TestNoMessageOfTheChecksHoldsAStringOfTheFixturesOrOfTheSeries(t *testing.T) {
	forbidden := slices.Concat(secrets, seriesArguments, inventedLedgerDates)
	breaks := messagesUnderEveryBreak(t)
	read := 0
	for label, messages := range breaks {
		for _, message := range messages {
			read++
			for _, value := range forbidden {
				if strings.Contains(message, value) {
					t.Errorf("under %q a message holds %q", label, value)
				}
			}
		}
	}
	if read < len(breaks) {
		t.Errorf("%d breaks gave %d messages, so a check said nothing to read", len(breaks), read)
	}
}

// seriesCoverageProblems holds the series to the server's registry: a
// registered write has a step of its mode, a method the list allows to have no
// handler has none, and no step names a method nobody answers.
func seriesCoverageProblems(handled []string, modes map[string]expectation, steps []seriesStep, allowed []string) []string {
	stepped := map[string][]stepMode{}
	for _, step := range steps {
		stepped[step.method] = append(stepped[step.method], step.mode)
	}
	wanted := map[expectation]stepMode{
		expectsRecorded: stepRecorded, expectsExtension: stepExtension, expectsNothing: stepNull, expectsEvaluation: stepEvaluation, expectsProposal: stepProposal,
	}
	var problems []string
	for _, name := range handled {
		if mode, needed := wanted[modes[name]]; needed && !slices.Contains(stepped[name], mode) {
			problems = append(problems, name+": registered and has no step of its mode; add one to seriesSteps in realsnapshot_writes_series_test.go")
		}
	}
	for _, name := range allowed {
		if slices.Contains(handled, name) {
			problems = append(problems, name+": has a handler; remove it from stepsAllowedWithoutHandler in realsnapshot_writes_series_test.go")
		}
	}
	for name := range stepped {
		if !slices.Contains(handled, name) && !slices.Contains(allowed, name) {
			problems = append(problems, name+": a step names a method with no handler, and stepsAllowedWithoutHandler does not list it")
		}
	}
	slices.Sort(problems)
	return problems
}

func seriesCoverageInputs(t *testing.T) ([]string, map[string]expectation) {
	t.Helper()
	methods, err := contract.Load()
	if err != nil {
		t.Fatal(err)
	}
	modes, _ := modesOfManifest(methods)
	return handledWrites(t), modes
}

// recordedMethodsOfTheSeries are the methods the series records a change with.
func recordedMethodsOfTheSeries() []string {
	var names []string
	for _, step := range seriesSteps {
		if (step.mode == stepRecorded || step.mode == stepExtension) && !slices.Contains(names, step.method) {
			names = append(names, step.method)
		}
	}
	return names
}

// recordedWritesOfTheContract is how many writes record a change: the 56 writes
// but the undo, the PIN, the two proposal-table writes and the four that answer
// only null.
const recordedWritesOfTheContract = 48

func TestEveryRecordedWriteHasAStepInTheSeries(t *testing.T) {
	handled, modes := seriesCoverageInputs(t)
	for _, problem := range seriesCoverageProblems(handled, modes, seriesSteps, stepsAllowedWithoutHandler) {
		t.Error(problem)
	}
	for _, name := range stepsAllowedWithoutHandler {
		if !slices.Contains([]string{"review.dismissWarning", "settings.setNickname", "settings.setDefaultLine"}, name) {
			t.Errorf("%s may not be allowed to find no handler", name)
		}
	}
	if got := len(recordedMethodsOfTheSeries()); got != recordedWritesOfTheContract {
		t.Errorf("the series records changes through %d methods, want %d", got, recordedWritesOfTheContract)
	}
}

func TestTheSeriesCoverageFailsForAMissingStepAndForAListedMethodWithAHandler(t *testing.T) {
	handled, modes := seriesCoverageInputs(t)
	if problems := seriesCoverageProblems(handled, modes, seriesSteps, nil); len(problems) != 0 {
		t.Fatalf("the control failed: %v", problems)
	}
	without := func(method string) []seriesStep {
		return slices.DeleteFunc(slices.Clone(seriesSteps), func(step seriesStep) bool { return step.method == method })
	}
	broken := map[string][]seriesStep{
		"a recorded write with no step": without(closeMonthMethod),
		"the extension with no step":    without(extensionMethod),
		"a write answering only null":   without("review.propose"),
		"a proposal write with no step": without("review.refuse"),
		"the evaluation with no step":   without("budget.evaluateScenario"),
		"a step of a method nobody has": append(slices.Clone(seriesSteps), seriesStep{method: "a.write", mode: stepRecorded}),
	}
	for label, steps := range broken {
		if len(seriesCoverageProblems(handled, modes, steps, nil)) == 0 {
			t.Errorf("%s went unnoticed", label)
		}
	}
	if len(seriesCoverageProblems(handled, modes, seriesSteps, []string{"settings.setNickname"})) == 0 {
		t.Error("a listed method that has a handler went unnoticed")
	}
}
