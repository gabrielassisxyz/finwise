package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/contract"
)

// fixtureClock is the clock the app's database of a case is created with, so its
// tracking start month is the fixture's.
func fixtureClock() time.Time { return fixtureNow }

// undoer takes a change back and answers as the undo does. A test hands the
// harness one that is wrong on purpose to see the check refuse it.
type undoer func(env *writeEnv, id string) *httptest.ResponseRecorder

func theRealUndo(env *writeEnv, id string) *httptest.ResponseRecorder { return env.undo(id) }

// newCaseEnv is a router over the full invented ledger and an app's database
// created by authoredtest, with a session open. A case that brings handlers
// of its own runs on a registry that holds those and the undo; any other runs on
// the server's own.
func newCaseEnv(t *testing.T, c writeCase) *writeEnv {
	t.Helper()
	scratch := authoredtest.Open(t, fixtureClock)
	options := []envOption{withLedger(newFullLedger(t).Path, fixtureNow), withDatabase(scratch.Database)}
	if c.handlers != nil {
		options = append(options, func(config *Config) {
			config.Reads = NewRegistry()
			registerChanges(config.Reads)
			c.handlers(config.Reads, scratch)
		})
	}
	env := newTestEnv(t, options...)
	return &writeEnv{testEnv: env, scratch: scratch, cookie: env.unlock()}
}

type fileState struct {
	canonical, journal, everything string
	changes                        int
}

func (env *writeEnv) state() fileState {
	env.t.Helper()
	return fileState{
		canonical:  env.scratch.Canonical(env.t),
		journal:    env.scratch.Journal(env.t),
		everything: env.scratch.Everything(env.t),
		changes:    len(env.scratch.Changes(env.t)),
	}
}

func changeNumber(t *testing.T, id string) int64 {
	t.Helper()
	number, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		t.Fatalf("%q is not a change id: %v", id, err)
	}
	return number
}

// runWriteCase makes a case's call over a database of its own and observes the
// file around it. A recorded change or an extension that answered an id is then
// taken back with undo, and the file observed again.
func runWriteCase(t *testing.T, c writeCase, undo undoer) writeObservation {
	t.Helper()
	env := newCaseEnv(t, c)
	if c.state != nil {
		c.state(t, env.scratch)
	}
	var o writeObservation
	o.canonicalBeforeOrigin = env.state().canonical
	for _, call := range c.before {
		resp := env.post(call.method, call.arguments)
		if resp.Code != http.StatusOK {
			t.Fatalf("before %s %s answered %d %s, want 200", call.method, call.arguments, resp.Code, resp.Body.String())
		}
		if id, ok := changeIDIn(resp.Body.Bytes()); ok {
			o.originID = id
		}
	}
	if o.originID != "" {
		o.originEffects[0] = len(env.scratch.EffectsOf(t, changeNumber(t, o.originID)))
	}
	before := env.state()
	response := env.post(c.method, c.arguments)
	after := env.state()

	o.status, o.body, o.requestLog = response.Code, response.Body.Bytes(), env.logs.Bytes()
	o.canonicalBefore, o.canonicalAfter = before.canonical, after.canonical
	o.journalBefore, o.everythingBefore, o.changesBefore = before.journal, before.everything, before.changes
	o.journalAfter, o.everythingAfter, o.changesAfter = after.journal, after.everything, after.changes
	if c.expects == expectsProposal {
		view := env.scratch.Database.View()
		proposals, err := view.Proposals(t.Context())
		view.Close()
		if err != nil {
			t.Fatal(err)
		}
		o.proposals = proposals
	}
	if o.originID != "" {
		o.originEffects[1] = len(env.scratch.EffectsOf(t, changeNumber(t, o.originID)))
	}
	id, answered := changeIDIn(o.body)
	if response.Code != http.StatusOK || !answered {
		return o
	}
	o.changeID = id
	if c.expects != expectsRecorded && c.expects != expectsExtension {
		return o
	}
	o.effects = env.scratch.EffectsOf(t, changeNumber(t, id))
	undone := undo(env, id)
	o.undoStatus, o.undoBody = undone.Code, undone.Body.String()
	o.canonicalAfterUndo = env.state().canonical
	return o
}

// checkWriteCase runs a case and reports every problem of it, the golden
// content of a recorded change included.
func checkWriteCase(t *testing.T, c writeCase, undo undoer) writeObservation {
	t.Helper()
	label := c.method + " " + c.arguments
	o := runWriteCase(t, c, undo)
	for _, problem := range problemsOf(c, o) {
		t.Errorf("%s: %s", label, problem)
	}
	if c.expects == expectsProposal && o.status == http.StatusOK {
		document, err := proposalDocument(o.body, o.proposals)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		path, err := writeGoldenFileName(c)
		if err != nil {
			t.Fatal(err)
		}
		compareWithGolden(t, path, document)
		return o
	}
	if c.expects != expectsRecorded || o.changeID == "" {
		return o
	}
	document, err := effectsDocument(o.body, o.effects)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	if c.held != "" {
		if problem := effectsProblem(document, []byte(c.held)); problem != "" {
			t.Errorf("%s: %s", label, problem)
		}
		return o
	}
	path, err := writeGoldenFileName(c)
	if err != nil {
		t.Fatal(err)
	}
	compareWithGolden(t, path, document)
	return o
}

func TestEveryWriteCaseConformsAndMatchesItsGoldenFile(t *testing.T) {
	if err := goldenUpdateRefusal(*updateGolden, os.Getenv(snapshotVariable)); err != nil {
		t.Fatal(err)
	}
	for _, c := range writeCases {
		checkWriteCase(t, c, theRealUndo)
	}
}

func handledWrites(t *testing.T) []string {
	t.Helper()
	changing := manifestChanging(t)
	return append(registeredChanges(t, changing), sessionServiceWrites...)
}

func TestEveryRegisteredWriteAndEvaluationHasACaseInItsMode(t *testing.T) {
	methods, err := contract.Load()
	if err != nil {
		t.Fatal(err)
	}
	modes, nullable := modesOfManifest(methods)
	for _, problem := range writeCoverageProblems(handledWrites(t), modes, nullable, writeCases) {
		t.Error(problem)
	}
}

func TestOnlyTheUndoAndThePINAreExemptFromTheHarness(t *testing.T) {
	methods, err := contract.Load()
	if err != nil {
		t.Fatal(err)
	}
	modes, nullable := modesOfManifest(methods)
	var exempt []string
	for name, mode := range modes {
		if mode == exemptFromTheHarness {
			exempt = append(exempt, name)
		}
	}
	slices.Sort(exempt)
	if !slices.Equal(exempt, []string{"changes.undo", "session.changePin"}) {
		t.Errorf("the methods exempt from the harness are %v", exempt)
	}
	if len(modes) != writesAndEvaluationsInTheManifest || modes["budget.evaluateScenario"] != expectsEvaluation ||
		modes[extensionMethod] != expectsExtension || modes["settings.setNickname"] != expectsRecorded {
		t.Errorf("modes of the manifest: %d methods, %v", len(modes), modes)
	}
	for _, name := range answerOnlyNull {
		if !nullable[name] {
			t.Errorf("%s answers only null and its result does not allow null", name)
		}
	}
}

func TestEveryReasonOfTheClosedListIsCasedOrNamedAsWithoutAProducer(t *testing.T) {
	for _, problem := range reasonProblems(writeCases, reasonsWithoutProducer) {
		t.Error(problem)
	}
}

func TestNoTwoRecordedCasesNameOneFile(t *testing.T) {
	for _, problem := range sharedFileProblems(writeCases) {
		t.Error(problem)
	}
}
