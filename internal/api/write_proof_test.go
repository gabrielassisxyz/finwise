package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// No table of the app is journaled yet, so the harness is proved over handlers
// registered only here, on methods of the manifest borrowed for the purpose,
// writing through the recorder to the scratch tables. Each is a case of one mode
// that must pass, and each check is then fed what is wrong on purpose.

const (
	compoundMethod   = "transactions.setReason"
	nothingMethod    = "transactions.confirmLine"
	evaluationMethod = "budget.evaluateScenario"
	refusingMethod   = "review.closeMonth"
	leavingMethod    = "settings.setCardLabel"
	loggingMethod    = "settings.setThreshold"
)

// seedScratch is the content every proof case starts from, written in one change.
func seedScratch(t *testing.T, scratch *authoredtest.Scratch) {
	t.Helper()
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "scratch.seed", At: fixtureNow}
	err := scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		if err := recorder.Set(t.Context(), authoredtest.Facts, key(text("fact-seed")), authored.Row{"amount": integer(3)}); err != nil {
			return err
		}
		return recorder.Put(t.Context(), authoredtest.Tags, authored.Row{"tx": text("fact-seed"), "tag": text("tag-seed")})
	})
	if err != nil {
		t.Fatal(err)
	}
}

// compoundWrite changes both shapes of table in one change: two facts, a tag, a
// parent and the child that refers to it.
func compoundWrite(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	fact, note, err := twoStrings(args)
	if err != nil {
		return nil, err
	}
	recorder := writes.Recorder
	steps := []func() error{
		func() error {
			return recorder.Set(ctx, authoredtest.Facts, key(text(fact)), authored.Row{"amount": integer(11)})
		},
		func() error {
			return recorder.Set(ctx, authoredtest.Facts, key(text("fact-b")), authored.Row{"amount": integer(12)})
		},
		func() error {
			return recorder.Put(ctx, authoredtest.Tags, authored.Row{"tx": text(fact), "tag": text("tag-compound")})
		},
		func() error {
			return recorder.Put(ctx, authoredtest.Parents, authored.Row{"id": integer(7), "name": text(note)})
		},
		func() error {
			return recorder.Put(ctx, authoredtest.Children, authored.Row{"id": integer(7), "parent_id": integer(7), "label": text("nota-child")})
		},
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	return answerWithChange(ctx, writes)
}

// extendTheOrigin is the handler of an extension: it appends a fact to the
// change whose id it is given, and answers that change's id.
func extendTheOrigin(ctx context.Context, args []json.RawMessage, journal Journal, _ *ledger.View) (any, error) {
	given, err := stringArgument(args)
	if err != nil {
		return nil, err
	}
	origin, ok := changeIDOf(given)
	if !ok {
		return nil, refuse(reasonStaleChange)
	}
	var handed authored.ChangeID
	err = journal.Extend(ctx, origin, setFactMethod, func(recorder *authored.Recorder) error {
		if err := recorder.Set(ctx, authoredtest.Facts, key(text("fact-b")), authored.Row{"amount": integer(41)}); err != nil {
			return err
		}
		var err error
		handed, err = recorder.ID(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"changed": 1, "changeId": handed.String()}, nil
}

func scratchEvaluation(effect func()) ReadHandler {
	return func(context.Context, []json.RawMessage, Views) (any, error) {
		if effect != nil {
			effect()
		}
		figures := map[string]any{"investment": 0, "rate": nil}
		return map[string]any{"current": figures, "scenario": figures, "difference": 0, "changed": [][]bool{}}, nil
	}
}

// proofHandlers are the handlers that a case of the harness's own is answered by.
func proofHandlers(registry *Registry, scratch *authoredtest.Scratch) {
	withScratchWriters(&Config{Reads: registry})
	registry.RegisterWrite(compoundMethod, compoundWrite)
	registry.RegisterWrite(nothingMethod, func(context.Context, []json.RawMessage, Writes) (any, error) { return nil, nil })
	registry.Register(evaluationMethod, scratchEvaluation(nil))
	registry.RegisterJournal(extensionMethod, extendTheOrigin)
	registry.RegisterWrite(refusingMethod, func(ctx context.Context, _ []json.RawMessage, writes Writes) (any, error) {
		if err := writes.Recorder.Put(ctx, authoredtest.Tags, authored.Row{"tx": text("tx-refused"), "tag": text("tag-refused")}); err != nil {
			return nil, err
		}
		return nil, refuse(reasonMonthClosed)
	})
}

// withProofHandlers is the proof handlers with the ones change registers in
// their place, which a case uses to make one handler wrong.
func withProofHandlers(change func(registry *Registry, scratch *authoredtest.Scratch)) func(*Registry, *authoredtest.Scratch) {
	return func(registry *Registry, scratch *authoredtest.Scratch) {
		replaced := NewRegistry()
		change(replaced, scratch)
		full := NewRegistry()
		proofHandlers(full, scratch)
		for _, name := range full.Names() {
			if _, taken := replaced.lookup(name); !taken {
				moveHandler(full, registry, name)
			}
		}
		change(registry, scratch)
	}
}

func moveHandler(from, to *Registry, name string) {
	found, _ := from.lookup(name)
	switch {
	case found.read != nil:
		to.Register(name, found.read)
	case found.write != nil:
		to.RegisterWrite(name, found.write)
	case found.journal != nil:
		to.RegisterJournal(name, found.journal)
	}
}

var (
	singleCase = writeCase{
		method: setFactMethod, arguments: `["fact-a","40"]`, expects: expectsRecorded,
		state: seedScratch, handlers: proofHandlers, held: `{
  "answer": {
    "changeId": "2"
  },
  "effects": [
    {
      "seq": 1,
      "table": "scratch_facts",
      "key": [
        "fact-a"
      ],
      "column": "amount",
      "before": 0,
      "after": 40
    }
  ]
}
`,
	}
	compoundCase = writeCase{
		method: compoundMethod, arguments: `["fact-a","nota-lazer"]`, expects: expectsRecorded,
		state: seedScratch, handlers: proofHandlers, held: `{
  "answer": {
    "changeId": "2"
  },
  "effects": [
    {
      "seq": 1,
      "table": "scratch_facts",
      "key": [
        "fact-a"
      ],
      "column": "amount",
      "before": 0,
      "after": 11
    },
    {
      "seq": 2,
      "table": "scratch_facts",
      "key": [
        "fact-b"
      ],
      "column": "amount",
      "before": 0,
      "after": 12
    },
    {
      "seq": 3,
      "table": "scratch_tags",
      "key": [
        "fact-a",
        "tag-compound"
      ],
      "column": null,
      "before": null,
      "after": {
        "tx": "fact-a",
        "tag": "tag-compound"
      }
    },
    {
      "seq": 4,
      "table": "scratch_parents",
      "key": [
        7
      ],
      "column": null,
      "before": null,
      "after": {
        "id": 7,
        "name": "nota-lazer"
      }
    },
    {
      "seq": 5,
      "table": "scratch_children",
      "key": [
        7
      ],
      "column": null,
      "before": null,
      "after": {
        "id": 7,
        "parent_id": 7,
        "label": "nota-child"
      }
    }
  ]
}
`,
	}
	nothingCase    = writeCase{method: nothingMethod, arguments: `["tx-1"]`, expects: expectsNothing, state: seedScratch, handlers: proofHandlers}
	evaluationCase = writeCase{method: evaluationMethod, arguments: `["2026-03",[]]`, expects: expectsEvaluation, state: seedScratch, handlers: proofHandlers}
	// The origin is the second change of the database: the seed is the first.
	extensionCase = writeCase{
		method: extensionMethod, arguments: `["2"]`, expects: expectsExtension, state: seedScratch, handlers: proofHandlers,
		before: []writeCall{{setFactMethod, `["fact-a","40"]`}},
	}
	refusalCase = writeCase{
		method: refusingMethod, arguments: `["2026-03"]`, expects: expectsRefusal, reason: reasonMonthClosed,
		state: seedScratch, handlers: proofHandlers,
	}
)

// proofWriteCases are the harness's own cases, one of each mode and a refusal.
var proofWriteCases = []writeCase{singleCase, compoundCase, nothingCase, evaluationCase, extensionCase, refusalCase}

func allWriteCases() []writeCase {
	return append(append([]writeCase(nil), writeCases...), proofWriteCases...)
}

func TestEveryProofCaseOfTheHarnessPasses(t *testing.T) {
	for _, c := range proofWriteCases {
		checkWriteCase(t, c, theRealUndo)
	}
}

// brokenUndo is the real undo followed by what a faulty one would have left.
func brokenUndo(leave func(scratch *authoredtest.Scratch, t *testing.T)) undoer {
	return func(env *writeEnv, id string) *httptest.ResponseRecorder {
		response := env.undo(id)
		leave(env.scratch, env.t)
		return response
	}
}

func TestTheRecordedCheckRefusesAnUndoThatIsWrongInThreeWays(t *testing.T) {
	broken := map[string]struct {
		undo   undoer
		naming string
	}{
		"one unit not restored":                  {brokenUndo(func(s *authoredtest.Scratch, t *testing.T) { s.ForceFact(t, "fact-b", 12) }), `"id":"fact-b"`},
		"one unit restored to a different value": {brokenUndo(func(s *authoredtest.Scratch, t *testing.T) { s.ForceFact(t, "fact-b", 99) }), `"amount":99`},
		"a child row left behind":                {brokenUndo(func(s *authoredtest.Scratch, t *testing.T) { s.ForceChild(t, 7, 7, "nota-child") }), `"id":7`},
	}
	for label, wrong := range broken {
		problems := problemsOf(compoundCase, runWriteCase(t, compoundCase, wrong.undo))
		if len(problems) == 0 {
			t.Errorf("%s went unnoticed", label)
			continue
		}
		if !strings.Contains(strings.Join(problems, "\n"), wrong.naming) {
			t.Errorf("%s was reported without naming %s: %v", label, wrong.naming, problems)
		}
	}
}

func TestTheExtensionCheckRefusesAnUndoThatReturnsToTheStateAfterTheOrigin(t *testing.T) {
	afterTheOrigin := brokenUndo(func(s *authoredtest.Scratch, t *testing.T) { s.ForceFact(t, "fact-a", 40) })

	problems := problemsOf(extensionCase, runWriteCase(t, extensionCase, afterTheOrigin))

	if len(problems) == 0 || !strings.Contains(strings.Join(problems, "\n"), `"id":"fact-a"`) {
		t.Errorf("an undo that stops at the origin's state was reported as %v", problems)
	}
}

func TestTheRecordedCheckRefusesAnAnswerThatDoesNotValidateAndEffectsThatDiffer(t *testing.T) {
	o := runWriteCase(t, singleCase, theRealUndo)
	if problems := recordedProblems(singleCase.method, o); len(problems) != 0 {
		t.Fatalf("the control was refused: %v", problems)
	}
	invalid := o
	invalid.body = []byte(`{"changeId":2}`)
	if len(recordedProblems(singleCase.method, invalid)) == 0 {
		t.Error("an answer that does not validate went unnoticed")
	}

	document, err := effectsDocument(o.body, o.effects)
	if err != nil {
		t.Fatal(err)
	}
	if problem := effectsProblem(document, []byte(singleCase.held)); problem != "" {
		t.Fatalf("the control differs from its golden content: %s", problem)
	}
	byAColumn := strings.Replace(singleCase.held, `"amount"`, `"note"`, 1)
	if effectsProblem(document, []byte(byAColumn)) == "" {
		t.Error("effects that differ by one column went unnoticed")
	}
	reordered := o
	reordered.effects = []authoredtest.Effect{{Seq: 2, Table: "scratch_tags", Key: `["a","b"]`, After: `{"tx":"a","tag":"b"}`}, o.effects[0]}
	original := append([]authoredtest.Effect{o.effects[0]}, reordered.effects[0])
	first, _ := effectsDocument(o.body, original)
	second, _ := effectsDocument(o.body, reordered.effects)
	if effectsProblem(second, first) == "" {
		t.Error("effects in another order went unnoticed")
	}
}

func TestTheNothingToDoCheckRefusesANullAfterAJournalRow(t *testing.T) {
	if problems := problemsOf(nothingCase, runWriteCase(t, nothingCase, theRealUndo)); len(problems) != 0 {
		t.Fatalf("the control was refused: %v", problems)
	}
	recordsThenAnswersNull := nothingCase
	recordsThenAnswersNull.handlers = withProofHandlers(func(registry *Registry, _ *authoredtest.Scratch) {
		registry.RegisterWrite(nothingMethod, func(ctx context.Context, _ []json.RawMessage, writes Writes) (any, error) {
			_, err := writes.Recorder.ID(ctx)
			return nil, err
		})
	})

	problems := problemsOf(recordsThenAnswersNull, runWriteCase(t, recordsThenAnswersNull, theRealUndo))

	if len(problems) == 0 || !strings.Contains(strings.Join(problems, "\n"), "changes in the journal") {
		t.Errorf("a null that left a row in the journal was reported as %v", problems)
	}
}

func TestTheEvaluationCheckRefusesAnyTableThatChanged(t *testing.T) {
	if problems := problemsOf(evaluationCase, runWriteCase(t, evaluationCase, theRealUndo)); len(problems) != 0 {
		t.Fatalf("the control was refused: %v", problems)
	}
	writes := map[string]func(scratch *authoredtest.Scratch, t *testing.T){
		"a journaled table":             func(s *authoredtest.Scratch, t *testing.T) { s.ForceFact(t, "fact-a", 5) },
		"a table that is not journaled": func(s *authoredtest.Scratch, t *testing.T) { s.TouchMeta(t, "observed", "x") },
	}
	for label, write := range writes {
		storing := evaluationCase
		storing.handlers = withProofHandlers(func(registry *Registry, scratch *authoredtest.Scratch) {
			registry.Register(evaluationMethod, scratchEvaluation(func() { write(scratch, t) }))
		})
		if len(problemsOf(storing, runWriteCase(t, storing, theRealUndo))) == 0 {
			t.Errorf("an evaluation that changed %s went unnoticed", label)
		}
	}
}

func TestTheRefusalCheckRefusesARefusalThatLeftARowBehind(t *testing.T) {
	if problems := problemsOf(refusalCase, runWriteCase(t, refusalCase, theRealUndo)); len(problems) != 0 {
		t.Fatalf("the control was refused: %v", problems)
	}
	leaving := refusalCase
	leaving.handlers = withProofHandlers(func(registry *Registry, _ *authoredtest.Scratch) {
		registry.RegisterJournal(refusingMethod, func(ctx context.Context, _ []json.RawMessage, journal Journal, _ *ledger.View) (any, error) {
			facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: refusingMethod, At: fixtureNow}
			err := journal.(*authored.Database).Change(ctx, facts, func(recorder *authored.Recorder) error {
				return recorder.Put(ctx, authoredtest.Tags, authored.Row{"tx": text("tx-left"), "tag": text("tag-left")})
			})
			if err != nil {
				return nil, err
			}
			return nil, refuse(reasonMonthClosed)
		})
	})

	problems := problemsOf(leaving, runWriteCase(t, leaving, theRealUndo))

	if len(problems) == 0 || !strings.Contains(strings.Join(problems, "\n"), "tx-left") {
		t.Errorf("a refusal that left a row behind was reported as %v", problems)
	}
	wrongReason := refusalCase
	wrongReason.reason = reasonFutureMonth
	if len(problemsOf(wrongReason, runWriteCase(t, wrongReason, theRealUndo))) == 0 {
		t.Error("a refusal for another reason went unnoticed")
	}
}

func TestTheWriteCoverageCheckFailsForAMissingCaseOrAnInventedOne(t *testing.T) {
	modes := map[string]expectation{
		"a.recorded": expectsRecorded, "a.evaluation": expectsEvaluation, "a.extension": expectsExtension,
		"a.exempt": exemptFromTheHarness, "a.refusing": expectsRecorded, "a.unhandled": expectsRecorded,
	}
	nullable := map[string]bool{"a.recorded": true}
	handled := []string{"a.recorded", "a.evaluation", "a.extension", "a.exempt", "a.refusing"}
	complete := []writeCase{
		{method: "a.recorded", expects: expectsRecorded}, {method: "a.evaluation", expects: expectsEvaluation},
		{method: "a.extension", expects: expectsExtension}, {method: "a.refusing", expects: expectsRecorded},
		{method: "a.exempt", expects: expectsRefusal, reason: reasonStaleChange},
		{method: "a.recorded", expects: expectsNothing},
	}
	if problems := writeCoverageProblems(handled, modes, nullable, complete); len(problems) != 0 {
		t.Fatalf("a complete arrangement was reported: %v", problems)
	}
	cases := map[string][]writeCase{
		"a method with no case":                  complete[1:],
		"a case of another mode":                 append(append([]writeCase(nil), complete[:3]...), writeCase{method: "a.refusing", expects: expectsEvaluation}),
		"a case for a method with no handler":    append(append([]writeCase(nil), complete...), writeCase{method: "a.unhandled", expects: expectsRecorded}),
		"a case for a method of no manifest":     append(append([]writeCase(nil), complete...), writeCase{method: "a.invented", expects: expectsRecorded}),
		"a case for an exempt method":            append(append([]writeCase(nil), complete...), writeCase{method: "a.exempt", expects: expectsRecorded}),
		"a null where the result does not allow": append(append([]writeCase(nil), complete...), writeCase{method: "a.evaluation", expects: expectsNothing}),
		"only a null for a recorded method":      {{method: "a.recorded", expects: expectsNothing}, complete[1], complete[2], complete[3]},
	}
	for label, given := range cases {
		if len(writeCoverageProblems(handled, modes, nullable, given)) == 0 {
			t.Errorf("%s went unnoticed", label)
		}
	}
}

func TestTheReasonsCheckFailsForAReasonInNeitherPlaceOrInBoth(t *testing.T) {
	cased := []writeCase{{method: "changes.undo", expects: expectsRefusal, reason: reasonStaleChange}}
	without := []refusalReason{
		reasonMonthClosed, reasonFutureMonth, reasonPendingReviews, reasonNotFound,
		reasonUnknownLine, reasonInvalidBudget, reasonUnstablePot, reasonSplitWithReimbursement,
	}
	if problems := reasonProblems(cased, without); len(problems) != 0 {
		t.Fatalf("an accounted list was reported: %v", problems)
	}
	if len(reasonProblems(cased, without[1:])) == 0 {
		t.Error("a reason in neither place went unnoticed")
	}
	if len(reasonProblems(cased, append(append([]refusalReason(nil), without...), reasonStaleChange))) == 0 {
		t.Error("a reason with a case and in the list went unnoticed")
	}
	if len(reasonProblems(cased, append(append([]refusalReason(nil), without...), "made_up"))) == 0 {
		t.Error("a name that is not a reason went unnoticed")
	}
	if len(reasonProblems(append(cased, writeCase{expects: expectsRefusal, reason: "made_up"}), without)) == 0 {
		t.Error("a case with a reason outside the list went unnoticed")
	}
}

func TestTheSharedFileCheckFailsForTwoCasesOfOneFile(t *testing.T) {
	one := writeCase{method: setFactMethod, arguments: `["a","1"]`, expects: expectsRecorded}
	other := writeCase{method: setFactMethod, arguments: `["a","2"]`, expects: expectsRecorded}
	if problems := sharedFileProblems([]writeCase{one, other}); len(problems) != 0 {
		t.Fatalf("two files were reported: %v", problems)
	}
	if len(sharedFileProblems([]writeCase{one, one})) == 0 {
		t.Error("two cases of one file went unnoticed")
	}
	spelledAlike := writeCase{method: setFactMethod, arguments: `[ "a" , "1" ]`, expects: expectsRecorded}
	if len(sharedFileProblems([]writeCase{one, spelledAlike})) == 0 {
		t.Error("two spellings of one call went unnoticed")
	}
}

func TestEveryRecordedCaseOfOneMethodDiffersInItsArguments(t *testing.T) {
	for _, problem := range sharedFileProblems(allWriteCases()) {
		t.Error(problem)
	}
}

func TestADifferenceIsNamedLineByLine(t *testing.T) {
	got := differences("a\nb\nc", "a\nc\nd")
	if got != "+ d\n- b" {
		t.Errorf("differences = %q", got)
	}
	if differences("a\nb", "b\na") == "" {
		t.Error("lines in another order were not a difference")
	}
}

func TestTheExtensionCheckRefusesAChangeOfItsOwnAndAnotherChangesId(t *testing.T) {
	control := runWriteCase(t, extensionCase, theRealUndo)
	if problems := problemsOf(extensionCase, control); len(problems) != 0 {
		t.Fatalf("the control was refused: %v", problems)
	}
	own := control
	own.changesAfter++
	otherID := control
	otherID.changeID = "1"
	appendedNothing := control
	appendedNothing.originEffects[1] = appendedNothing.originEffects[0]
	for label, observed := range map[string]writeObservation{"a change of its own": own, "another change's id": otherID, "no effect appended": appendedNothing} {
		if len(problemsOf(extensionCase, observed)) == 0 {
			t.Errorf("an extension with %s went unnoticed", label)
		}
	}
}

func TestTheRefusalCheckValidatesTheBodyAgainstTheManifestsErrorSchema(t *testing.T) {
	outsideTheList := refusalReason("made_up")
	observed := writeObservation{status: 409, body: []byte(`{"error":{"kind":"refused","reason":"made_up"}}`)}
	if problems := refusalProblems(outsideTheList, observed); len(problems) == 0 {
		t.Error("a body that is not an envelope of the manifest went unnoticed because it was the one expected")
	}
}

// Each check of a mode is fed an observation that only it can refuse: the
// control observation with one thing made wrong, which must be reported under
// the name of that thing and not merely reported.
func TestEveryCheckOfEveryModeNamesTheProblemOnlyItCanSee(t *testing.T) {
	type mutation struct {
		label   string
		c       writeCase
		mutate  func(o *writeObservation)
		problem string
	}
	mutations := []mutation{
		{"recorded: a change with no effect", singleCase, func(o *writeObservation) { o.effects = nil }, "recorded no effect"},
		{"recorded: an undo that answers another status", singleCase, func(o *writeObservation) { o.undoStatus = 409 }, "the undo answered"},
		{"recorded: an undo that answers a body that is not null", singleCase, func(o *writeObservation) { o.undoBody = `{}` }, "the undo answered"},
		{"recorded: no change id", singleCase, func(o *writeObservation) { o.changeID = "" }, "holds no change id"},
		{"recorded: another status", singleCase, func(o *writeObservation) { o.status = 500 }, "want 200"},
		{"extension: an undo that answers another status", extensionCase, func(o *writeObservation) { o.undoStatus = 409 }, "the undo answered"},
		{"extension: an undo that answers a body that is not null", extensionCase, func(o *writeObservation) { o.undoBody = `{}` }, "the undo answered"},
		{"nothing: an answer that is not null", nothingCase, func(o *writeObservation) { o.body = []byte(`{"changeId":"1"}`) }, "want 200 null"},
		{"nothing: another status", nothingCase, func(o *writeObservation) { o.status = 500 }, "want 200 null"},
		{"nothing: a journal that differs with the same count", nothingCase, func(o *writeObservation) { o.journalAfter += "\nchange 9" }, "the journal changed"},
		{"evaluation: another status", evaluationCase, func(o *writeObservation) { o.status = 500 }, "want 200"},
		{"evaluation: an answer that does not conform", evaluationCase, func(o *writeObservation) { o.body = []byte(`{}`) }, "does not conform"},
		{"refusal: another status", refusalCase, func(o *writeObservation) { o.status = 500 }, "want 409"},
		{"refusal: another body", refusalCase, func(o *writeObservation) { o.body = []byte(`{"error":{"kind":"read_only"}}`) }, "want 409"},
	}
	for _, m := range mutations {
		control := runWriteCase(t, m.c, theRealUndo)
		if problems := problemsOf(m.c, control); len(problems) != 0 {
			t.Fatalf("%s: the control was refused: %v", m.label, problems)
		}
		observed := control
		m.mutate(&observed)
		if joined := strings.Join(problemsOf(m.c, observed), "\n"); !strings.Contains(joined, m.problem) {
			t.Errorf("%s: reported as %q, want a problem naming %q", m.label, joined, m.problem)
		}
	}
}

// A handler that asks for the change's id and writes nothing leaves an empty
// change that undoes to the same tables; only the effects count refuses it.
func TestARecordedCaseWhoseHandlerWritesNothingIsRefused(t *testing.T) {
	empty := singleCase
	empty.handlers = withProofHandlers(func(registry *Registry, _ *authoredtest.Scratch) {
		registry.RegisterWrite(setFactMethod, func(ctx context.Context, _ []json.RawMessage, writes Writes) (any, error) {
			return answerWithChange(ctx, writes)
		})
	})
	empty.held = ""

	o := runWriteCase(t, empty, theRealUndo)
	if restoredProblem(o.canonicalBefore, o.canonicalAfterUndo) != "" || o.undoStatus != 200 {
		t.Fatalf("the empty change was expected to undo cleanly")
	}
	if joined := strings.Join(recordedProblems(empty.method, o), "\n"); !strings.Contains(joined, "recorded no effect") {
		t.Errorf("an empty recorded change was reported as %q", joined)
	}
}
