package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/contract"
)

// What a write case expects. Every write and every evaluation of the manifest is
// of exactly one mode, and each mode has its own check; a refusal is a
// further expectation any method may have.
type expectation int

const (
	// expectsRecorded: a change is recorded, answered with its id, and its effects
	// are the case's golden file. Undoing it puts every journaled table back.
	expectsRecorded expectation = iota + 1
	// expectsNothing: the answer is null and the journal gained no row.
	expectsNothing
	// expectsEvaluation: the answer conforms and no table of the file changed.
	expectsEvaluation
	// expectsExtension: the effects are appended to an earlier change, whose id is
	// the answer. Undoing that id puts every journaled table back to before the
	// earlier change.
	expectsExtension
	// expectsRefusal: 409 refused with a named reason and no table of the file
	// changed.
	expectsRefusal
	// exemptFromTheHarness is the mode of a method that has tests of its own.
	exemptFromTheHarness
)

func (e expectation) String() string {
	return [...]string{"", "a recorded change", "nothing to do", "an evaluation", "an extension", "a refusal", "exempt"}[e]
}

// exemptMethods are the only two methods outside the harness, named so that a
// third cannot hide beside them: the undo is the harness's own instrument, and
// the PIN is the session service's.
var exemptMethods = []string{"changes.undo", "session.changePin"}

// extensionMethod is the one method whose mode is an extension: "all purchases
// here" appends its effects to the change that set the line.
const extensionMethod = "transactions.applyLineToEstablishment"

// answerOnlyNull lists the methods that answer only null until what they act on
// has a producer; nothing to do is their mode. Each piece of work that gives one
// a producer removes its name.
var answerOnlyNull = []string{}

// writeCall is a call made to put a case's state in place.
type writeCall struct{ method, arguments string }

// writeCase is one call of a write or an evaluation and what it expects. Each
// case starts from a database of its own, so ids the database mints are the
// same whatever other cases exist and a golden file never moves.
type writeCase struct {
	method    string
	arguments string
	expects   expectation
	// reason is the refusal's reason, for a case that expects a refusal.
	reason refusalReason
	// state writes the invented authored content the case starts from.
	state func(t *testing.T, scratch *authoredtest.Scratch)
	// before are calls made, each answered 200, after the state and before the
	// case's own call. The last one's change is what an extension extends.
	before []writeCall
	// handlers registers handlers that exist only in tests, on methods of the
	// manifest borrowed for the purpose. A case without it runs against the
	// server's own registry.
	handlers func(registry *Registry, scratch *authoredtest.Scratch)
	// held is the golden content of a recorded change whose file is not in
	// testdata, which the harness's own proof cases use.
	held string
}

// writeCases is every real case, in every mode. Each slice that adds a write
// adds its cases here; the handlers that exist when this lands are the undo
// alone, which has tests of its own and only this refusal.
var writeCases = append([]writeCase{
	{method: "changes.undo", arguments: `["999"]`, expects: expectsRefusal, reason: reasonStaleChange},
	{method: "settings.setThreshold", arguments: `[72.6]`, expects: expectsRecorded, state: startFromTheInventedState},
	{method: "settings.setMinExamples", arguments: `[80]`, expects: expectsRecorded, state: startFromTheInventedState},
	{method: "settings.setMinExamples", arguments: `[5]`, expects: expectsRecorded},
	{method: "settings.setCardLabel", arguments: `["  Virtual "]`, expects: expectsRecorded, state: startFromTheInventedState},
	{method: "settings.setClosing", arguments: `["manual",31]`, expects: expectsRecorded},
}, append(budgetWriteCases, append(potSettingsWriteCases, tagAndNicknameWriteCases...)...)...)

// reasonsWithoutProducer are the reasons of the closed list that nothing
// produces yet, and so have no case. A piece of work that first produces one
// adds its case and removes the name.
var reasonsWithoutProducer = []refusalReason{
	reasonFutureMonth, reasonPendingReviews, reasonSplitWithReimbursement,
}

// writeObservation is everything one run of a case saw. The checks read it and
// nothing else, so each can be fed a broken one.
type writeObservation struct {
	status int
	body   []byte

	canonicalBeforeOrigin string
	canonicalBefore       string
	canonicalAfterUndo    string
	journalBefore         string
	journalAfter          string
	everythingBefore      string
	everythingAfter       string
	changesBefore         int
	changesAfter          int

	changeID      string
	effects       []authoredtest.Effect
	originID      string
	originEffects [2]int

	undoStatus int
	undoBody   string

	requestLog []byte
}

// changeIDIn reads the handle out of an answer that has one.
func changeIDIn(body []byte) (string, bool) {
	var answer struct {
		ChangeID string `json:"changeId"`
	}
	if err := json.Unmarshal(body, &answer); err != nil || answer.ChangeID == "" {
		return "", false
	}
	return answer.ChangeID, true
}

// differences names what separates two texts, line by line: the lines only the
// first holds with "-" and the lines only the second holds with "+".
func differences(before, after string) string {
	counts := map[string]int{}
	for _, line := range strings.Split(before, "\n") {
		counts[line]++
	}
	var out []string
	for _, line := range strings.Split(after, "\n") {
		if counts[line] > 0 {
			counts[line]--
			continue
		}
		out = append(out, "+ "+line)
	}
	for _, line := range strings.Split(before, "\n") {
		if counts[line] > 0 {
			counts[line]--
			out = append(out, "- "+line)
		}
	}
	if len(out) == 0 && before != after {
		return "the same lines in another order"
	}
	return strings.Join(out, "\n")
}

// restoredProblem is empty when an undo put every journaled table back to what
// it was before the thing it takes back.
func restoredProblem(before, afterUndo string) string {
	if before == afterUndo {
		return ""
	}
	return "after the undo the journaled tables are not what they were before:\n" + differences(before, afterUndo)
}

func unchangedProblem(what, before, after string) string {
	if before == after {
		return ""
	}
	return what + " changed:\n" + differences(before, after)
}

// answerProblems is what every answer that carries a result is checked for.
func answerProblems(method string, o writeObservation) []string {
	if o.status != http.StatusOK {
		return []string{fmt.Sprintf("answered %d %s, want 200", o.status, o.body)}
	}
	if problem := conformanceProblem(method, o.body); problem != "" {
		return []string{"the answer does not conform: " + problem}
	}
	return nil
}

func collect(problems ...string) []string {
	var found []string
	for _, problem := range problems {
		if problem != "" {
			found = append(found, problem)
		}
	}
	return found
}

// recordedProblems checks a recorded change: its answer, the undo's answer and
// the tables the undo left. The effects are compared with the golden content by
// effectsProblem.
func recordedProblems(method string, o writeObservation) []string {
	if problems := answerProblems(method, o); problems != nil {
		return problems
	}
	var problems []string
	if o.changeID == "" {
		return []string{"the answer holds no change id"}
	}
	if len(o.effects) == 0 {
		problems = append(problems, "the change recorded no effect")
	}
	if o.undoStatus != http.StatusOK || o.undoBody != "null" {
		problems = append(problems, fmt.Sprintf("the undo answered %d %s, want 200 null", o.undoStatus, o.undoBody))
	}
	return append(problems, collect(restoredProblem(o.canonicalBefore, o.canonicalAfterUndo))...)
}

// extensionProblems checks an extension: it adds no change of its own, answers
// the origin's id, appends to the origin's effects, and one undo of that id
// returns the tables to before the origin.
func extensionProblems(method string, o writeObservation) []string {
	if problems := answerProblems(method, o); problems != nil {
		return problems
	}
	var problems []string
	if o.changeID != o.originID {
		problems = append(problems, fmt.Sprintf("the answer is change %q, want the origin's %q", o.changeID, o.originID))
	}
	if o.changesAfter != o.changesBefore {
		problems = append(problems, fmt.Sprintf("the extension wrote %d changes of its own", o.changesAfter-o.changesBefore))
	}
	if o.originEffects[1] <= o.originEffects[0] {
		problems = append(problems, fmt.Sprintf("the origin holds %d effects after the extension, were %d", o.originEffects[1], o.originEffects[0]))
	}
	if o.undoStatus != http.StatusOK || o.undoBody != "null" {
		problems = append(problems, fmt.Sprintf("the undo answered %d %s, want 200 null", o.undoStatus, o.undoBody))
	}
	return append(problems, collect(restoredProblem(o.canonicalBeforeOrigin, o.canonicalAfterUndo))...)
}

// nothingProblems checks an answer of nothing to do: null, and not a row more
// in the journal.
func nothingProblems(o writeObservation) []string {
	var problems []string
	if o.status != http.StatusOK || strings.TrimSpace(string(o.body)) != "null" {
		problems = append(problems, fmt.Sprintf("answered %d %s, want 200 null", o.status, o.body))
	}
	if o.changesAfter != o.changesBefore {
		problems = append(problems, fmt.Sprintf("an answer of null left %d changes in the journal", o.changesAfter-o.changesBefore))
	}
	return append(problems, collect(unchangedProblem("the journal", o.journalBefore, o.journalAfter))...)
}

// evaluationProblems checks an evaluation: it conforms and it stores nothing,
// in any table of the file.
func evaluationProblems(method string, o writeObservation) []string {
	return append(answerProblems(method, o), collect(unchangedProblem("the file", o.everythingBefore, o.everythingAfter))...)
}

// refusalProblems checks a refusal: 409, the named reason, the manifest's error
// envelope, and no table of the file changed.
func refusalProblems(reason refusalReason, o writeObservation) []string {
	var problems []string
	want := `{"error":{"kind":"refused","reason":"` + string(reason) + `"}}`
	if o.status != http.StatusConflict || string(o.body) != want {
		problems = append(problems, fmt.Sprintf("answered %d %s, want 409 %s", o.status, o.body, want))
	}
	problems = append(problems, collect(errorEnvelopeProblem(o.body))...)
	return append(problems, collect(unchangedProblem("the file", o.everythingBefore, o.everythingAfter))...)
}

// problemsOf runs the check of the mode the case expects.
func problemsOf(c writeCase, o writeObservation) []string {
	switch c.expects {
	case expectsRecorded:
		return recordedProblems(c.method, o)
	case expectsNothing:
		return nothingProblems(o)
	case expectsEvaluation:
		return evaluationProblems(c.method, o)
	case expectsExtension:
		return extensionProblems(c.method, o)
	case expectsRefusal:
		return refusalProblems(c.reason, o)
	}
	return []string{"the case expects " + c.expects.String()}
}

type effectOfChange struct {
	Seq    int64           `json:"seq"`
	Table  string          `json:"table"`
	Key    json.RawMessage `json:"key"`
	Column *string         `json:"column"`
	Before json.RawMessage `json:"before"`
	After  json.RawMessage `json:"after"`
}

type changeDocument struct {
	Answer  json.RawMessage  `json:"answer"`
	Effects []effectOfChange `json:"effects"`
}

func rawOrNull(text string) json.RawMessage {
	if text == "" {
		return json.RawMessage("null")
	}
	return json.RawMessage(text)
}

// effectsDocument is the golden content of a recorded change: the answer and the
// change's effects in the journal's order, each as the recorder wrote it.
func effectsDocument(answer []byte, effects []authoredtest.Effect) ([]byte, error) {
	document := changeDocument{Answer: answer, Effects: []effectOfChange{}}
	for _, effect := range effects {
		listed := effectOfChange{Seq: effect.Seq, Table: effect.Table, Key: rawOrNull(effect.Key),
			Before: rawOrNull(effect.Before), After: rawOrNull(effect.After)}
		if effect.Column != "" {
			column := effect.Column
			listed.Column = &column
		}
		document.Effects = append(document.Effects, listed)
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// effectsProblem is empty when the effects document equals the golden content.
func effectsProblem(document, golden []byte) string {
	if bytes.Equal(document, golden) {
		return ""
	}
	return "the answer and effects differ from the golden content:\n" + differences(string(golden), string(document))
}

// writeGoldenFileName is the file of a recorded case under testdata/golden/writes.
func writeGoldenFileName(c writeCase) (string, error) {
	name, err := goldenName(c.method, c.arguments)
	if err != nil {
		return "", err
	}
	return filepath.Join("testdata", "golden", "writes", name), nil
}

// sharedFileProblems names the recorded cases that would write one file: two
// calls of a method whose arguments slug alike.
func sharedFileProblems(cases []writeCase) []string {
	owner := map[string]writeCase{}
	var problems []string
	for _, c := range cases {
		if c.expects != expectsRecorded {
			continue
		}
		name, err := goldenName(c.method, c.arguments)
		if err != nil {
			problems = append(problems, c.method+" "+c.arguments+": "+err.Error())
			continue
		}
		if first, taken := owner[name]; taken {
			problems = append(problems, fmt.Sprintf("%s %s and %s %s both name %s", first.method, first.arguments, c.method, c.arguments, name))
			continue
		}
		owner[name] = c
	}
	return problems
}

// modesOfManifest is the mode of every write and evaluation, and which of them
// may answer null.
func modesOfManifest(methods []contract.Method) (modes map[string]expectation, nullable map[string]bool) {
	modes, nullable = map[string]expectation{}, map[string]bool{}
	for _, method := range methods {
		if method.Class != contract.ClassWrite && method.Class != contract.ClassEvaluate {
			continue
		}
		name := method.Qualified()
		switch {
		case slices.Contains(exemptMethods, name):
			modes[name] = exemptFromTheHarness
		case method.Class == contract.ClassEvaluate:
			modes[name] = expectsEvaluation
		case slices.Contains(answerOnlyNull, name):
			modes[name] = expectsNothing
		case name == extensionMethod:
			modes[name] = expectsExtension
		default:
			modes[name] = expectsRecorded
		}
		nullable[name] = conformanceProblem(name, []byte("null")) == ""
	}
	return modes, nullable
}

// writeCoverageProblems is what the coverage test requires: a case in its mode
// for every method that has a handler and is not exempt, and no case for a
// method that has no handler, an exempt one (but for a refusal), or one of
// another mode than the method's.
func writeCoverageProblems(handled []string, modes map[string]expectation, nullable map[string]bool, cases []writeCase) []string {
	covered := map[string]bool{}
	var problems []string
	for _, c := range cases {
		mode, known := modes[c.method]
		switch {
		case !slices.Contains(handled, c.method):
			problems = append(problems, c.method+": a case names a method with no handler")
		case !known:
			problems = append(problems, c.method+": a case names a method that is not a write or an evaluation")
		case c.expects == expectsRefusal:
		case mode == exemptFromTheHarness:
			problems = append(problems, c.method+": exempt from the harness, and a case expects "+c.expects.String())
		case c.expects == mode:
			covered[c.method] = true
		case c.expects == expectsNothing && nullable[c.method]:
		default:
			problems = append(problems, fmt.Sprintf("%s: a method of mode %s has a case that expects %s", c.method, mode, c.expects))
		}
	}
	for _, name := range handled {
		if mode, known := modes[name]; known && mode != exemptFromTheHarness && !covered[name] {
			problems = append(problems, name+": registered and has no case that expects "+mode.String())
		}
	}
	sort.Strings(problems)
	return problems
}

// reasonProblems accounts for the nine reasons: each has a case that expects a
// refusal for it, or is named as one nothing produces yet, and never both.
func reasonProblems(cases []writeCase, without []refusalReason) []string {
	var problems []string
	cased := map[refusalReason]bool{}
	for _, c := range cases {
		if c.expects == expectsRefusal {
			cased[c.reason] = true
		}
	}
	for _, reason := range without {
		if !slices.Contains(refusalReasons, reason) {
			problems = append(problems, string(reason)+": listed as without a producer, and it is not a reason of the list")
		}
	}
	for _, reason := range refusalReasons {
		listed := slices.Contains(without, reason)
		switch {
		case cased[reason] && listed:
			problems = append(problems, string(reason)+": has a case and is also listed as without a producer")
		case !cased[reason] && !listed:
			problems = append(problems, string(reason)+": has no case and is not listed as without a producer")
		}
	}
	for reason := range cased {
		if !slices.Contains(refusalReasons, reason) {
			problems = append(problems, string(reason)+": a case refuses with a reason that is not of the list")
		}
	}
	sort.Strings(problems)
	return problems
}

var monthOrNumber = regexp.MustCompile(`^(\d+|\d{4}-\d{2})?$`)

// budgetGroupKey is a key or a name of the budget's five groups: a place in a
// document, not text a person typed.
var budgetGroupKey = regexp.MustCompile(`^(rec|fix|var|prov|res|Receita|Fixos e essenciais|Variáveis|Provisões|Reservas de desejo)$`)

// unlistedArguments names every argument of a case that is text a person types
// or that names a row, which is any string but a number or a month, and holds no
// string of the list: such a text would not be seen in a log by the test that
// looks.
func unlistedArguments(secrets []string, calls []string) []string {
	var found []string
	var visit func(value any)
	visit = func(value any) {
		switch typed := value.(type) {
		case string:
			if monthOrNumber.MatchString(typed) || budgetGroupKey.MatchString(typed) {
				return
			}
			for _, secret := range secrets {
				if strings.Contains(typed, secret) {
					return
				}
			}
			found = append(found, typed)
		case []any:
			for _, item := range typed {
				visit(item)
			}
		case map[string]any:
			for _, item := range typed {
				visit(item)
			}
		}
	}
	for _, arguments := range calls {
		var decoded any
		if err := json.Unmarshal([]byte(arguments), &decoded); err != nil {
			found = append(found, arguments)
			continue
		}
		visit(decoded)
	}
	sort.Strings(found)
	return found
}
