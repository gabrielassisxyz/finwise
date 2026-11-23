package authored_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

var lineClock = time.Date(2026, time.March, 20, 15, 0, 0, 0, time.UTC)

// saveMethods are the two writes that save a budget; both go through
// SaveBudget, which is where the clearing is called.
var saveMethods = []string{"budget.save", "budget.applyScenario"}

// lineEnv is the line examples over the invented budget as the only version,
// from 2026-01, the clock at 2026-03-20.
type lineEnv struct {
	t        *testing.T
	scratch  *authoredtest.Scratch
	examples authoredtest.LineExamples
	tick     int
}

func newLineEnv(t *testing.T) *lineEnv {
	t.Helper()
	return newLineEnvOver(t, nil)
}

// newLineEnvOver lets versions write budgets after the first and before the
// examples, so that a fixture can leave an example on a line its month lacks.
func newLineEnvOver(t *testing.T, versions func(env *lineEnv)) *lineEnv {
	t.Helper()
	env := &lineEnv{t: t, scratch: authoredtest.Open(t, func() time.Time { return lineClock })}
	authoredtest.SaveBudget(t, env.scratch.Database, env.at(), "2026-01", authoredtest.InventedBudget())
	if versions != nil {
		versions(env)
	}
	env.change("scratch.lineExamples", func(recorder *authored.Recorder) error {
		var err error
		env.examples, err = authoredtest.WriteLineExamples(t.Context(), recorder)
		return err
	})
	return env
}

func (env *lineEnv) at() time.Time {
	env.tick++
	return lineClock.Add(time.Duration(env.tick) * time.Minute)
}

func (env *lineEnv) change(method string, write func(*authored.Recorder) error) authored.ChangeID {
	env.t.Helper()
	var id authored.ChangeID
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: method, At: env.at()}
	err := env.scratch.Database.Change(env.t.Context(), facts, func(recorder *authored.Recorder) error {
		if err := write(recorder); err != nil {
			return err
		}
		var err error
		id, err = recorder.ID(env.t.Context())
		return err
	})
	if err != nil {
		env.t.Fatalf("%s: %v", method, err)
	}
	return id
}

func (env *lineEnv) budget(month brday.Month) authored.Budget {
	env.t.Helper()
	view := env.scratch.Database.View()
	defer view.Close()
	budget, err := view.BudgetOf(env.t.Context(), month)
	if err != nil {
		env.t.Fatal(err)
	}
	return budget
}

func (env *lineEnv) hasLine(month brday.Month, name string) bool {
	return slices.ContainsFunc(env.budget(month).Lines, func(line authored.BudgetLine) bool { return line.Name == name })
}

// save saves, under method, the document in force for month as edit leaves it.
func (env *lineEnv) save(method string, month brday.Month, edit func(authored.Document) authored.Document) authored.ChangeID {
	env.t.Helper()
	document := edit(authoredtest.DocumentOf(env.budget(month)))
	return env.change(method, func(recorder *authored.Recorder) error {
		return recorder.SaveBudget(env.t.Context(), month, document)
	})
}

func (env *lineEnv) undo(id authored.ChangeID) error {
	return env.scratch.Database.Undo(env.t.Context(), id, env.at())
}

func (env *lineEnv) kept(name string) authored.AuthoredTransaction {
	env.t.Helper()
	key, found := env.examples.Keys[name]
	if !found {
		env.t.Fatalf("no example is called %s", name)
	}
	view := env.scratch.Database.View()
	defer view.Close()
	kept, err := view.Transactions(env.t.Context(), []authored.TxKey{key})
	if err != nil {
		env.t.Fatal(err)
	}
	return kept[key]
}

// setState is a later change of the person's that writes an example's state.
func (env *lineEnv) setState(name string, state authored.TransactionState) authored.ChangeID {
	env.t.Helper()
	return env.change("scratch.setLine", func(recorder *authored.Recorder) error {
		return authoredtest.WriteState(env.t.Context(), recorder, env.examples.Keys[name], state)
	})
}

func (env *lineEnv) names(keys []authored.TxKey) []string {
	names := []string{}
	for _, key := range keys {
		names = append(names, env.examples.Name(key))
	}
	return names
}

// txEffects is each effect of change on the transactions' tables as
// "<example> <table> <column>", in the order of the journal.
func (env *lineEnv) txEffects(change authored.ChangeID) []string {
	env.t.Helper()
	effects := []string{}
	for _, effect := range env.scratch.EffectsOf(env.t, int64(change)) {
		if effect.Table != "tx_state" && effect.Table != "tx_split_parts" {
			continue
		}
		var key []int64
		if err := json.Unmarshal([]byte(effect.Key), &key); err != nil || len(key) == 0 {
			env.t.Fatalf("the key %s of an effect: %v", effect.Key, err)
		}
		effects = append(effects, fmt.Sprintf("%s %s %s", env.examples.Name(authored.TxKey(key[0])), effect.Table, effect.Column))
	}
	return effects
}

// expectOnlyTheSavesTables checks that a save's effects name the budget's
// versions and the transactions' two tables and nothing else, the budget's
// first.
func (env *lineEnv) expectOnlyTheSavesTables(change authored.ChangeID) {
	env.t.Helper()
	lastBudget, firstTx := int64(0), int64(math.MaxInt64)
	for _, effect := range env.scratch.EffectsOf(env.t, int64(change)) {
		switch effect.Table {
		case "budget_versions", "budget_version_lines":
			lastBudget = max(lastBudget, effect.Seq)
		case "tx_state", "tx_split_parts":
			firstTx = min(firstTx, effect.Seq)
		default:
			env.t.Errorf("the save has an effect on %s: %+v", effect.Table, effect)
		}
	}
	if firstTx <= lastBudget {
		env.t.Errorf("an effect on a transaction (seq %d) comes before one on the budget (seq %d)", firstTx, lastBudget)
	}
}

// expectMonthlessLinesKept checks the three references to Delivery that belong
// to no month: the establishment's default line, the pot's link and the
// subscription's line.
func (env *lineEnv) expectMonthlessLinesKept() {
	env.t.Helper()
	view := env.scratch.Database.View()
	defer view.Close()
	establishments, err := view.Establishments(env.t.Context(), []string{authoredtest.LineExampleEstablishment})
	if err != nil {
		env.t.Fatal(err)
	}
	if line := establishments[authoredtest.LineExampleEstablishment].DefaultLine; line == nil || *line != env.examples.Delivery {
		env.t.Errorf("the establishment's default line is %v, want Delivery's %d", line, env.examples.Delivery)
	}
	linked, err := view.LinesOfPot(env.t.Context(), authoredtest.LineExamplePot)
	if err != nil {
		env.t.Fatal(err)
	}
	if !slices.Equal(linked, []authored.LineID{env.examples.Delivery}) {
		env.t.Errorf("the pot's lines are %v, want Delivery's", linked)
	}
	subscription, found, err := view.Subscription(env.t.Context(), env.examples.Subscription)
	if err != nil {
		env.t.Fatal(err)
	}
	if !found || subscription.Line == nil || *subscription.Line != env.examples.Delivery {
		line := "no line"
		if subscription.Line != nil {
			line = fmt.Sprint(*subscription.Line)
		}
		env.t.Errorf("the subscription (found %t) is on %s, want Delivery's %d", found, line, env.examples.Delivery)
	}
}

func TestEverySaveCaseClearsWhatTheMonthsThatLoseALineHoldAndOneUndoGivesItBack(t *testing.T) {
	for _, method := range saveMethods {
		for _, c := range authoredtest.LineSaveCases() {
			t.Run(method+"/"+c.Name, func(t *testing.T) {
				env := newLineEnv(t)
				if c.Prepare != nil {
					c.Prepare(t, env.scratch.Database, env.at())
				}
				before, canonical := env.scratch.LineReferences(t), env.scratch.Canonical(t)
				change := env.save(method, c.Month, c.Edit)
				after := env.scratch.LineReferences(t)
				for _, problem := range authoredtest.ClearedProblems(before, after) {
					t.Error(problem)
				}
				if got := env.names(authoredtest.Changed(before, after)); !slices.Equal(got, append([]string{}, c.Changed...)) {
					t.Errorf("the save changed %v, want %v", got, c.Changed)
				}
				if effects := env.txEffects(change); len(c.Changed) == 0 && len(effects) != 0 {
					t.Errorf("a save that removes no line in use has effects on the transactions: %v", effects)
				}
				env.expectOnlyTheSavesTables(change)
				env.expectMonthlessLinesKept()
				if err := env.undo(change); err != nil {
					t.Fatalf("undo: %v", err)
				}
				if got := env.scratch.Canonical(t); got != canonical {
					t.Errorf("the undo left another state:\n%s\nwant\n%s", got, canonical)
				}
			})
		}
	}
}

func TestRemovingALineFromAMonthOnClearsEachReferenceOnItsOwnAndKeepsTheFeesLine(t *testing.T) {
	env := newLineEnv(t)
	change := env.save("budget.save", "2026-03", authoredtest.Without("Delivery"))
	person := authored.AuthorPerson
	for name, want := range map[string]authored.TransactionState{
		"t4":  {},
		"t5":  {PendingReason: "Baixa confiança"},
		"t6":  {Refund: authoredtest.Some(true)},
		"t8":  {},
		"t9":  {Line: authoredtest.Some(authoredtest.FeesLine), ClassifiedBy: person},
		"t7":  {Line: authoredtest.Some(env.examples.Mercado), ClassifiedBy: person},
		"t10": {PendingReason: "Estabelecimento novo"},
	} {
		if got := env.kept(name).State; !reflect.DeepEqual(got, want) {
			t.Errorf("%s holds %+v, want %+v", name, got, want)
		}
	}
	want := []string{
		"t4 tx_state line_id", "t4 tx_state classified_by", "t4 tx_state ask_reason",
		"t5 tx_state pending_line_id", "t5 tx_state pending_confidence",
		"t6 tx_state refund_line_id",
		"t8 tx_state line_id", "t8 tx_state classified_by",
	}
	if got := env.txEffects(change); !slices.Equal(got, want) {
		t.Errorf("the save's effects on the transactions are\n%v\nwant\n%v", got, want)
	}
	env.expectOnlyTheSavesTables(change)
}

func TestRemovingALineAtTheFirstVersionsMonthClearsEarlierMonthsAndKeepsASplitPartInPlace(t *testing.T) {
	env := newLineEnv(t)
	env.save("budget.applyScenario", "2026-01", authoredtest.Without("Delivery"))
	for _, name := range []string{"t1", "t2", "t4", "t8"} {
		if got := env.kept(name).State; !reflect.DeepEqual(got, authored.TransactionState{}) {
			t.Errorf("%s holds %+v, want nothing", name, got)
		}
	}
	split := env.kept("t3").Split
	want := []authored.SplitPart{{Position: 0, Cents: 3_000}, {Position: 1, Line: authoredtest.Some(env.examples.Mercado), Cents: 2_000}}
	if !reflect.DeepEqual(split, want) {
		t.Errorf("t3's split is %+v, want the first part in place with no line", split)
	}
}

func TestAClosedMonthAfterTheSaveKeepsItsLineAndTheMonthAfterItLosesIt(t *testing.T) {
	env := newLineEnv(t)
	authoredtest.CloseWithItsOwnVersion("2026-02")(t, env.scratch.Database, env.at())
	env.save("budget.save", "2026-01", authoredtest.Without("Delivery"))
	if !env.hasLine("2026-02", "Delivery") || env.hasLine("2026-03", "Delivery") {
		t.Errorf("2026-02 has Delivery: %v, 2026-03 has it: %v; want true and false",
			env.hasLine("2026-02", "Delivery"), env.hasLine("2026-03", "Delivery"))
	}
	if part := env.kept("t3").Split[0]; part.Line == nil || *part.Line != env.examples.Delivery {
		t.Errorf("t3's first part is %+v, want it on Delivery", part)
	}
}

func TestASaveForALaterMonthClearsThatMonthsTransactionsOnly(t *testing.T) {
	env := newLineEnv(t)
	change := env.save("budget.save", "2026-04", authoredtest.Without("Delivery"))
	if got, want := env.txEffects(change), []string{"t8 tx_state line_id", "t8 tx_state classified_by"}; !slices.Equal(got, want) {
		t.Errorf("the effects on the transactions are %v, want %v", got, want)
	}
	if !env.hasLine("2026-03", "Delivery") {
		t.Error("the current month lost Delivery")
	}
}

func TestAMonthLeftWithNoLineLosesTheFeesLineAndItsSuggestionsAndTheUndoBringsThemBack(t *testing.T) {
	env := newLineEnv(t)
	before := map[string]authored.TransactionState{"t5": env.kept("t5").State, "t9": env.kept("t9").State}
	change := env.save("budget.save", "2026-03", authoredtest.Emptied)
	for name, want := range map[string]authored.TransactionState{
		"t9": {}, "t7": {}, "t5": {PendingReason: "Baixa confiança"}, "t10": {PendingReason: "Estabelecimento novo"},
		"t6": {Refund: authoredtest.Some(true)},
	} {
		if got := env.kept(name).State; !reflect.DeepEqual(got, want) {
			t.Errorf("%s holds %+v, want %+v", name, got, want)
		}
	}
	if err := env.undo(change); err != nil {
		t.Fatalf("undo: %v", err)
	}
	for name, want := range before {
		if got := env.kept(name).State; !reflect.DeepEqual(got, want) {
			t.Errorf("after the undo %s holds %+v, want %+v", name, got, want)
		}
	}
}

func TestARefusedSaveClearsNothing(t *testing.T) {
	env := newLineEnv(t)
	authoredtest.CloseMonth(t, env.scratch.Database, env.at(), "2026-03", "2026-03-20")
	refusals := []struct {
		month brday.Month
		edit  func(authored.Document) authored.Document
		is    func(error) bool
	}{
		{"2026-03", authoredtest.Without("Delivery"), func(err error) bool { return errors.Is(err, authored.ErrMonthClosed) }},
		{"2026-04", authoredtest.Renamed("Mercado", "Delivery"), func(err error) bool {
			var invalid *authored.InvalidBudgetError
			return errors.As(err, &invalid)
		}},
	}
	for _, refusal := range refusals {
		canonical, changes, effects := env.scratch.Canonical(t), len(env.scratch.Changes(t)), env.scratch.EffectCount(t)
		document := refusal.edit(authoredtest.DocumentOf(env.budget(refusal.month)))
		facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "budget.save", At: env.at()}
		err := env.scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
			return recorder.SaveBudget(t.Context(), refusal.month, document)
		})
		if !refusal.is(err) {
			t.Errorf("a save for %s answered %v", refusal.month, err)
		}
		if env.scratch.Canonical(t) != canonical || len(env.scratch.Changes(t)) != changes || env.scratch.EffectCount(t) != effects {
			t.Errorf("a refused save for %s changed the database or the journal", refusal.month)
		}
	}
}

func TestUndoingTheSaveGivesEveryClearedReferenceBackWithTheLine(t *testing.T) {
	env := newLineEnv(t)
	before := map[string]authored.AuthoredTransaction{}
	for _, name := range []string{"t4", "t5", "t6", "t7", "t8"} {
		before[name] = env.kept(name)
	}
	change := env.save("budget.save", "2026-03", authoredtest.Without("Delivery"))
	if err := env.undo(change); err != nil {
		t.Fatalf("undo: %v", err)
	}
	for name, want := range before {
		if got := env.kept(name); !reflect.DeepEqual(got, want) {
			t.Errorf("after the undo %s holds %+v, want %+v", name, got, want)
		}
	}
	if !env.hasLine("2026-03", "Delivery") {
		t.Error("after the undo 2026-03 has no Delivery")
	}
}

func TestALaterChangeOnAClearedReferenceMakesTheSavesUndoStaleAndAnotherTransactionsDoesNot(t *testing.T) {
	env := newLineEnv(t)
	canonical := env.scratch.Canonical(t)
	save := env.save("budget.save", "2026-03", authoredtest.Without("Delivery"))
	onMercado := env.setState("t4", authored.TransactionState{Line: authoredtest.Some(env.examples.Mercado), ClassifiedBy: authored.AuthorPerson})
	held := env.scratch.State(t)
	var stale *authored.StaleChangeError
	if err := env.undo(save); !errors.As(err, &stale) || stale.Cause != authored.CauseLaterChange {
		t.Fatalf("the undo of the save answered %v, want stale for a later change", err)
	}
	if env.scratch.State(t) != held {
		t.Error("the refused undo changed the database or the journal")
	}
	if err := env.undo(onMercado); err != nil {
		t.Fatalf("undo the later change: %v", err)
	}
	if err := env.undo(save); err != nil {
		t.Fatalf("undo the save once the later change is undone: %v", err)
	}
	if got := env.scratch.Canonical(t); got != canonical {
		t.Errorf("the state is not the one before both changes:\n%s\nwant\n%s", got, canonical)
	}

	other := newLineEnv(t)
	save = other.save("budget.save", "2026-03", authoredtest.Without("Delivery"))
	other.setState("t7", authored.TransactionState{Line: authoredtest.Some(other.examples.Mercado), ClassifiedBy: authored.AuthorRule})
	if err := other.undo(save); err != nil {
		t.Fatalf("the undo of the save after a change to t7 alone: %v", err)
	}
	if t7 := other.kept("t7").State; t7.ClassifiedBy != authored.AuthorRule {
		t.Errorf("t7 lost its later change: %+v", t7)
	}
}

// withAprilWithoutDelivery is B0 without Delivery from 2026-04, written before
// the examples, so t8 is left on a line its month lacks.
func withAprilWithoutDelivery(env *lineEnv) {
	env.t.Helper()
	document := authoredtest.Without("Delivery")(authoredtest.DocumentOf(env.budget("2026-04")))
	authoredtest.SaveBudget(env.t, env.scratch.Database, env.at(), "2026-04", document)
}

func TestTheScopedClearingJudgesItsKeysOnlyAgainstTheMonthTheirIndexRowNowReads(t *testing.T) {
	env := newLineEnvOver(t, withAprilWithoutDelivery)
	var t11 authored.TxKey
	env.change("scratch.indexEndOfMarch", func(recorder *authored.Recorder) error {
		var err error
		seen := authoredtest.Seen("line-t11", "2026-03-31", -3_300, "name:loja-t11")
		if t11, err = recorder.IndexTransaction(t.Context(), seen); err != nil {
			return err
		}
		return authoredtest.WriteState(t.Context(), recorder, t11, authored.TransactionState{Line: authoredtest.Some(env.examples.Delivery), ClassifiedBy: authored.AuthorPerson})
	})
	env.examples.Keys["t11"] = t11
	before := env.scratch.LineReferences(t)
	moved := env.change("scratch.ingest", func(recorder *authored.Recorder) error {
		if _, err := recorder.IndexTransaction(t.Context(), authoredtest.Seen("line-t11", "2026-04-01", -3_300, "name:loja-t11")); err != nil {
			return err
		}
		return recorder.ClearLinesTheirMonthLacks(t.Context(), authored.OnlyTransactions(t11))
	})
	after := env.scratch.LineReferences(t)
	if got := env.names(authoredtest.Changed(before, after)); !slices.Equal(got, []string{"t11"}) {
		t.Errorf("the scoped clearing changed %v, want t11 alone", got)
	}
	if got, want := env.txEffects(moved), []string{"t11 tx_state line_id", "t11 tx_state classified_by"}; !slices.Equal(got, want) {
		t.Errorf("the open change's effects are %v, want %v", got, want)
	}
	env.change("scratch.ingest", func(recorder *authored.Recorder) error {
		return recorder.ClearLinesTheirMonthLacks(t.Context(), authored.EveryIndexedTransaction())
	})
	if got := env.names(authoredtest.Changed(after, env.scratch.LineReferences(t))); !slices.Equal(got, []string{"t8"}) {
		t.Errorf("the clearing of every transaction changed %v, want t8", got)
	}
}

func TestASaveJudgesEveryIndexedTransactionAndNotOnlyTheMonthsItChanges(t *testing.T) {
	env := newLineEnvOver(t, func(env *lineEnv) {
		withAprilWithoutDelivery(env)
		authoredtest.CloseMonth(env.t, env.scratch.Database, env.at(), "2026-04", "2026-05-02")
	})
	april := env.budget("2026-04")
	before := env.scratch.LineReferences(t)
	env.save("budget.save", "2026-01", func(document authored.Document) authored.Document { return document })
	if !reflect.DeepEqual(env.budget("2026-04"), april) {
		t.Fatal("the save changed what 2026-04 has")
	}
	after := env.scratch.LineReferences(t)
	if got := env.names(authoredtest.Changed(before, after)); !slices.Equal(got, []string{"t8"}) {
		t.Errorf("the save changed %v, want t8 alone", got)
	}
	for _, problem := range authoredtest.ClearedProblems(before, after) {
		t.Error(problem)
	}
}

func (env *lineEnv) setSplit(name string, parts []authored.SplitPart) authored.ChangeID {
	env.t.Helper()
	return env.change("scratch.setSplit", func(recorder *authored.Recorder) error {
		return authoredtest.WriteSplit(env.t.Context(), recorder, env.examples.Keys[name], parts)
	})
}

// addLine saves, for month on, the budget in force with one more line, and
// answers the change and the line's id.
func (env *lineEnv) addLine(month brday.Month, name string) (authored.ChangeID, authored.LineID) {
	env.t.Helper()
	change := env.save("budget.save", month, func(document authored.Document) authored.Document {
		document[2].Lines = append(document[2].Lines, authoredtest.Line(name, 9_000))
		return document
	})
	for _, line := range env.budget(month).Lines {
		if line.Name == name {
			return change, line.ID
		}
	}
	env.t.Fatalf("%s has no line %s after the save", month, name)
	return 0, 0
}

// expectRefusedByTheCheck undoes change and checks that the line check refused
// it and that nothing changed, the change itself still not undone.
func (env *lineEnv) expectRefusedByTheCheck(label string, change authored.ChangeID) {
	env.t.Helper()
	state, changes, effects := env.scratch.State(env.t), len(env.scratch.Changes(env.t)), env.scratch.EffectCount(env.t)
	var stale *authored.StaleChangeError
	if err := env.undo(change); !errors.As(err, &stale) || stale.Cause != authored.CauseCheck {
		env.t.Errorf("%s: the undo answered %v, want refused by a check", label, err)
	}
	if env.scratch.State(env.t) != state || len(env.scratch.Changes(env.t)) != changes || env.scratch.EffectCount(env.t) != effects {
		env.t.Errorf("%s: the refused undo changed the database or the journal", label)
	}
	for _, recorded := range env.scratch.Changes(env.t) {
		if recorded.ID == int64(change) && recorded.UndoneAt != "" {
			env.t.Errorf("%s: change %d is marked undone", label, change)
		}
	}
}

func (env *lineEnv) mustUndo(label string, change authored.ChangeID) {
	env.t.Helper()
	if err := env.undo(change); err != nil {
		env.t.Fatalf("%s: %v", label, err)
	}
}

// lineReferenceKinds are the four references a transaction of March holds, each
// written on a line in a change of its own.
var lineReferenceKinds = []struct {
	name, example string
	on            func(env *lineEnv, line authored.LineID) authored.ChangeID
}{
	{"own line", "t4", func(env *lineEnv, line authored.LineID) authored.ChangeID {
		return env.setState("t4", authored.TransactionState{Line: authoredtest.Some(line), ClassifiedBy: authored.AuthorPerson})
	}},
	{"suggestion", "t5", func(env *lineEnv, line authored.LineID) authored.ChangeID {
		return env.setState("t5", authored.TransactionState{PendingReason: "Baixa confiança", PendingLine: authoredtest.Some(line), PendingConfidence: authoredtest.Some[int64](62)})
	}},
	{"refund line", "t6", func(env *lineEnv, line authored.LineID) authored.ChangeID {
		return env.setState("t6", authored.TransactionState{Refund: authoredtest.Some(true), RefundLine: authoredtest.Some(line)})
	}},
	{"split part", "t10", func(env *lineEnv, line authored.LineID) authored.ChangeID {
		return env.setSplit("t10", []authored.SplitPart{{Position: 0, Line: authoredtest.Some(line), Cents: 1_000}, {Position: 1, Cents: 6_700}})
	}},
}

func TestAnUndoThatWouldTakeALineFromUnderATransactionIsRefusedUntilTheLaterChangeIsUndone(t *testing.T) {
	for _, kind := range lineReferenceKinds {
		t.Run(kind.name, func(t *testing.T) {
			env := newLineEnv(t)
			canonical := env.scratch.Canonical(t)
			added, pharmacy := env.addLine("2026-03", "Farmácia")
			later := kind.on(env, pharmacy)
			env.expectRefusedByTheCheck("the undo of the save that added the line", added)
			env.mustUndo("the undo of the later change", later)
			env.mustUndo("the undo of the save once the later change is undone", added)
			if got := env.scratch.Canonical(t); got != canonical {
				t.Errorf("the state is not the one before the save:\n%s\nwant\n%s", got, canonical)
			}
		})
	}
}

func TestAnUndoThatWouldPutATransactionBackOnARemovedLineIsRefusedWholeOrByPart(t *testing.T) {
	for _, kind := range lineReferenceKinds {
		t.Run(kind.name, func(t *testing.T) {
			env := newLineEnv(t)
			if kind.name == "split part" {
				kind.on(env, env.examples.Delivery)
			}
			moved := kind.on(env, env.examples.Mercado)
			removed := env.save("budget.save", "2026-03", authoredtest.Without("Delivery"))
			env.expectRefusedByTheCheck("the undo of the move", moved)

			state := env.scratch.State(t)
			facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "scratch.restorePart", At: env.at()}
			err := env.scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
				var stale *authored.StaleChangeError
				err := recorder.RestorePart(t.Context(), moved, authored.TransactionUnits(env.examples.Keys[kind.example], nil))
				if !errors.As(err, &stale) || stale.Cause != authored.CauseCheck {
					t.Errorf("the restore of the move's part answered %v, want refused by a check", err)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("the open change: %v", err)
			}
			if got := env.scratch.State(t); got != state {
				t.Errorf("the open change recorded something of the refused restore:\n%s", strings.TrimPrefix(got, state))
			}

			env.mustUndo("the undo of the save", removed)
			facts.At = env.at()
			err = env.scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
				if err := recorder.RestorePart(t.Context(), moved, authored.TransactionUnits(env.examples.Keys[kind.example], nil)); err != nil {
					t.Errorf("the restore of the move's part once the save is undone: %v", err)
				}
				return errTakenBack
			})
			if !errors.Is(err, errTakenBack) {
				t.Fatalf("the open change answered %v", err)
			}
			env.mustUndo("the undo of the move once the save is undone", moved)
			if line := env.lineOf(kind.example); line == nil || *line != env.examples.Delivery {
				t.Errorf("after the undo %s is on %v, want Delivery", kind.example, line)
			}
		})
	}
}

// errTakenBack ends a change whose writes a test only needed to see succeed.
var errTakenBack = errors.New("taken back on purpose")

// lineOf is the line of an example's reference of its kind.
func (env *lineEnv) lineOf(name string) *authored.LineID {
	kept := env.kept(name)
	switch name {
	case "t5":
		return kept.State.PendingLine
	case "t6":
		return kept.State.RefundLine
	case "t10":
		if len(kept.Split) == 0 {
			return nil
		}
		return kept.Split[0].Line
	}
	return kept.State.Line
}

func TestTheCheckJudgesEachTransactionByItsOwnMonth(t *testing.T) {
	env := newLineEnv(t)
	added, pharmacy := env.addLine("2026-04", "Farmácia")
	env.setState("t8", authored.TransactionState{Line: authoredtest.Some(pharmacy), ClassifiedBy: authored.AuthorPerson})
	note := env.setState("t7", authored.TransactionState{Line: authoredtest.Some(env.examples.Mercado), ClassifiedBy: authored.AuthorPerson, Note: "feira"})
	env.mustUndo("the undo of a change that leaves an April transaction on April's own line", note)
	env.expectRefusedByTheCheck("the undo of the save that gave April the line", added)
}
