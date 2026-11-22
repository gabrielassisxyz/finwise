package authoredtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// What a budget save does to the transactions that name a line their month's
// budget no longer has, as fixtures and as the rule a test judges a save by.
// The rule is stated here again, apart from the store's, so that a store that
// clears too much or too little is caught by a test and not agreed with.

// FeesLine is the built-in fees line, which the budget's migration seeds under
// this id.
const FeesLine authored.LineID = 1

// LineExampleEstablishment is the establishment of t4, whose default line is
// Delivery; LineExamplePot is the pot Delivery is linked to. Both name a line
// without belonging to a month, so no save may clear them.
const (
	LineExampleEstablishment = "name:delivery-exemplo"
	LineExamplePot           = "reserve:acc-invented/pot-delivery"
)

// LineExamples are the invented transactions of WriteLineExamples, by name, and
// the two lines they are on.
type LineExamples struct {
	Keys     map[string]authored.TxKey
	Delivery authored.LineID
	Mercado  authored.LineID
}

// Name is the example's name of key, or the key itself for a transaction that
// is not one of them.
func (examples LineExamples) Name(key authored.TxKey) string {
	for name, held := range examples.Keys {
		if held == key {
			return name
		}
	}
	return fmt.Sprint(key)
}

// WriteLineExamples indexes, inside recorder's change, ten invented
// transactions that hold every kind of reference to a line, on the lines of the
// budget in force for 2026-01, which the caller has written: t1 (2025-12-20)
// on Delivery by the person; t2 (2026-01-10) on Delivery by the model at 88;
// t3 (2026-02-14) split into Delivery 30.00 and Mercado 20.00; t4 (2026-03-05)
// on Delivery by the person with the question of why open; t5 (2026-03-08)
// waiting for review with Delivery suggested at 62; t6 (2026-03-09) a credit
// marked as a refund of Delivery; t7 (2026-03-11) on Mercado by the person; t8
// (2026-04-02) on Delivery by the person; t9 (2026-03-12) on the fees line by
// the person; t10 (2026-03-13) waiting for review with no suggestion. t4's
// establishment has Delivery as its default line and Delivery is linked to a
// pot. Nothing of it is real.
func WriteLineExamples(ctx context.Context, recorder *authored.Recorder) (LineExamples, error) {
	view := recorder.View()
	defer view.Close()
	budget, err := view.BudgetOf(ctx, "2026-01")
	if err != nil {
		return LineExamples{}, err
	}
	examples := LineExamples{Keys: map[string]authored.TxKey{}}
	for _, line := range budget.Lines {
		switch line.Name {
		case "Delivery":
			examples.Delivery = line.ID
		case "Mercado":
			examples.Mercado = line.ID
		}
	}
	if examples.Delivery == 0 || examples.Mercado == 0 {
		return LineExamples{}, errors.New("authoredtest: the line examples need Delivery and Mercado in force for 2026-01")
	}
	for _, example := range lineExamples(examples.Delivery, examples.Mercado) {
		key, err := writeLineExample(ctx, recorder, example)
		if err != nil {
			return LineExamples{}, err
		}
		examples.Keys[example.name] = key
	}
	if err := WriteEstablishment(ctx, recorder, LineExampleEstablishment, "", Some(examples.Delivery)); err != nil {
		return LineExamples{}, err
	}
	return examples, recorder.ToggleLink(ctx, examples.Delivery, LineExamplePot)
}

type lineExample struct {
	name  string
	day   brday.Day
	cents money.Cents
	state authored.TransactionState
	split []authored.SplitPart
}

func lineExamples(delivery, mercado authored.LineID) []lineExample {
	person, model := authored.AuthorPerson, authored.AuthorModel
	return []lineExample{
		{name: "t1", day: "2025-12-20", cents: -4_500, state: authored.TransactionState{Line: Some(delivery), ClassifiedBy: person}},
		{name: "t2", day: "2026-01-10", cents: -3_800, state: authored.TransactionState{Line: Some(delivery), ClassifiedBy: model, Confidence: Some[int64](88)}},
		{name: "t3", day: "2026-02-14", cents: -5_000, split: []authored.SplitPart{
			{Position: 0, Line: Some(delivery), Cents: 3_000}, {Position: 1, Line: Some(mercado), Cents: 2_000},
		}},
		{name: "t4", day: "2026-03-05", cents: -6_200, state: authored.TransactionState{Line: Some(delivery), ClassifiedBy: person, AskReason: true}},
		{name: "t5", day: "2026-03-08", cents: -2_990, state: authored.TransactionState{
			PendingReason: "Baixa confiança", PendingLine: Some(delivery), PendingConfidence: Some[int64](62),
		}},
		{name: "t6", day: "2026-03-09", cents: 2_990, state: authored.TransactionState{Refund: Some(true), RefundLine: Some(delivery)}},
		{name: "t7", day: "2026-03-11", cents: -15_000, state: authored.TransactionState{Line: Some(mercado), ClassifiedBy: person}},
		{name: "t8", day: "2026-04-02", cents: -4_100, state: authored.TransactionState{Line: Some(delivery), ClassifiedBy: person}},
		{name: "t9", day: "2026-03-12", cents: -1_290, state: authored.TransactionState{Line: Some(FeesLine), ClassifiedBy: person}},
		{name: "t10", day: "2026-03-13", cents: -7_700, state: authored.TransactionState{PendingReason: "Estabelecimento novo"}},
	}
}

func writeLineExample(ctx context.Context, recorder *authored.Recorder, example lineExample) (authored.TxKey, error) {
	establishment := "name:loja-" + example.name
	if example.name == "t4" {
		establishment = LineExampleEstablishment
	}
	key, err := recorder.IndexTransaction(ctx, Seen("line-"+example.name, example.day, example.cents, establishment))
	if err != nil {
		return 0, err
	}
	if err := WriteState(ctx, recorder, key, example.state); err != nil {
		return 0, err
	}
	if example.split == nil {
		return key, nil
	}
	return key, WriteSplit(ctx, recorder, key, example.split)
}

// LineReferences is what the rule is judged on: the month of every indexed
// transaction, what is kept about each, and the lines the budget in force for
// each of those months has, the fees line among them exactly when it has
// another.
type LineReferences struct {
	Months map[authored.TxKey]brday.Month
	Kept   map[authored.TxKey]authored.AuthoredTransaction
	Lines  map[brday.Month]map[authored.LineID]bool
}

const selectIndexDays = `SELECT tx_key, day FROM tx_index ORDER BY tx_key`

// LineReferences reads them as the database holds them now.
func (scratch *Scratch) LineReferences(t testing.TB) LineReferences {
	t.Helper()
	rows, err := scratch.Inspect(t).QueryContext(t.Context(), selectIndexDays)
	if err != nil {
		t.Fatalf("authoredtest: %v", err)
	}
	defer rows.Close()
	refs := LineReferences{Months: map[authored.TxKey]brday.Month{}, Lines: map[brday.Month]map[authored.LineID]bool{}}
	var keys []authored.TxKey
	for rows.Next() {
		var key int64
		var day string
		if err := rows.Scan(&key, &day); err != nil {
			t.Fatalf("authoredtest: %v", err)
		}
		keys = append(keys, authored.TxKey(key))
		refs.Months[authored.TxKey(key)] = brday.Month(day[:7])
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("authoredtest: %v", err)
	}
	view := scratch.Database.View()
	defer view.Close()
	if refs.Kept, err = view.Transactions(t.Context(), keys); err != nil {
		t.Fatalf("authoredtest: %v", err)
	}
	for _, month := range refs.Months {
		if _, done := refs.Lines[month]; !done {
			refs.Lines[month] = linesOfMonth(t, view, month)
		}
	}
	return refs
}

func linesOfMonth(t testing.TB, view *authored.View, month brday.Month) map[authored.LineID]bool {
	t.Helper()
	budget, err := view.BudgetOf(t.Context(), month)
	if err != nil {
		t.Fatalf("authoredtest: %v", err)
	}
	has := map[authored.LineID]bool{}
	for _, line := range budget.Lines {
		has[line.ID] = true
	}
	if len(has) > 0 {
		has[FeesLine] = true
	}
	return has
}

// ClearedByTheRule is kept as a save leaves it in a month whose budget has the
// lines of has: its own line goes with who put it there, the confidence and the
// open question of why; a suggestion goes with its confidence and the review
// stays; a refund's line goes and the refund stays; a split part keeps its
// place and amount and loses its line. A reference to a line has holds, and
// everything else, stays as it was.
func ClearedByTheRule(kept authored.AuthoredTransaction, has map[authored.LineID]bool) authored.AuthoredTransaction {
	lacks := func(line *authored.LineID) bool { return line != nil && !has[*line] }
	state := kept.State
	if lacks(state.Line) {
		state.Line, state.ClassifiedBy, state.Confidence, state.AskReason = nil, "", nil, false
	}
	if lacks(state.PendingLine) {
		state.PendingLine, state.PendingConfidence = nil, nil
	}
	if lacks(state.RefundLine) {
		state.RefundLine = nil
	}
	split := slices.Clone(kept.Split)
	for i := range split {
		if lacks(split[i].Line) {
			split[i].Line = nil
		}
	}
	return authored.AuthoredTransaction{State: state, Tags: kept.Tags, Split: split, History: kept.History}
}

// ClearedProblems names every indexed transaction that after does not hold as
// the rule clears before against the budgets after has: a reference cleared in
// a month that still has the line, one left in a month that lost it, and
// anything else of the transaction that moved.
func ClearedProblems(before, after LineReferences) []string {
	var problems []string
	for _, key := range sortedKeys(after.Months) {
		month := after.Months[key]
		want := normalized(ClearedByTheRule(before.Kept[key], after.Lines[month]))
		if got := normalized(after.Kept[key]); !reflect.DeepEqual(got, want) {
			problems = append(problems, fmt.Sprintf("transaction %d of %s holds %s, want %s", key, month, describeKept(got), describeKept(want)))
		}
	}
	return problems
}

// StrayProblems names every reference to a line its transaction's month's
// budget lacks.
func (refs LineReferences) StrayProblems() []string {
	var problems []string
	for _, key := range sortedKeys(refs.Months) {
		has := refs.Lines[refs.Months[key]]
		if kept := normalized(refs.Kept[key]); !reflect.DeepEqual(kept, normalized(ClearedByTheRule(kept, has))) {
			problems = append(problems, fmt.Sprintf("transaction %d of %s points at a line its month lacks: %s", key, refs.Months[key], describeKept(kept)))
		}
	}
	return problems
}

// Changed is every transaction whose kept rows differ between before and after.
func Changed(before, after LineReferences) []authored.TxKey {
	var changed []authored.TxKey
	for _, key := range sortedKeys(after.Months) {
		if !reflect.DeepEqual(normalized(before.Kept[key]), normalized(after.Kept[key])) {
			changed = append(changed, key)
		}
	}
	return changed
}

func normalized(kept authored.AuthoredTransaction) authored.AuthoredTransaction {
	if len(kept.Tags) == 0 {
		kept.Tags = nil
	}
	if len(kept.Split) == 0 {
		kept.Split = nil
	}
	if len(kept.History) == 0 {
		kept.History = nil
	}
	return kept
}

func describeKept(kept authored.AuthoredTransaction) string {
	described, err := json.Marshal(kept)
	if err != nil {
		return fmt.Sprintf("%v", err)
	}
	return string(described)
}

func sortedKeys(months map[authored.TxKey]brday.Month) []authored.TxKey {
	keys := make([]authored.TxKey, 0, len(months))
	for key := range months {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// LineSaveCase is one save over the line examples, which start from the
// invented budget as the only version, from 2026-01.
type LineSaveCase struct {
	Name string
	// Prepare writes what the case starts from besides that, or is nil.
	Prepare func(t testing.TB, database *authored.Database, at time.Time)
	Month   brday.Month
	// Edit makes the document saved out of the budget in force for Month, read
	// back with its ids.
	Edit func(authored.Document) authored.Document
	// Changed names the examples whose kept rows the save changes; every other
	// one is unchanged.
	Changed []string
}

// LineSaveCases are the worked examples of the rule, the saves that clear
// nothing among them.
func LineSaveCases() []LineSaveCase {
	return []LineSaveCase{
		{Name: "a line removed from 2026-03 on", Month: "2026-03", Edit: Without("Delivery"), Changed: []string{"t4", "t5", "t6", "t8"}},
		{Name: "a line removed at the first version's month", Month: "2026-01", Edit: Without("Delivery"),
			Changed: []string{"t1", "t2", "t3", "t4", "t5", "t6", "t8"}},
		{Name: "a closed later month keeps the line", Prepare: CloseWithItsOwnVersion("2026-02"), Month: "2026-01", Edit: Without("Delivery"),
			Changed: []string{"t1", "t2", "t4", "t5", "t6", "t8"}},
		{Name: "a later month's save leaves the current month alone", Month: "2026-04", Edit: Without("Delivery"), Changed: []string{"t8"}},
		{Name: "a month left with no line", Month: "2026-03", Edit: Emptied, Changed: []string{"t4", "t5", "t6", "t7", "t8", "t9"}},
		{Name: "a renamed line", Month: "2026-03", Edit: Renamed("Delivery", "Comida")},
		{Name: "a line moved to another group", Month: "2026-03", Edit: Moved("Delivery", "fix")},
		{Name: "a line with another amount", Month: "2026-03", Edit: WithCents("Delivery", 25_000)},
		{Name: "a line no transaction uses removed", Month: "2026-03", Edit: Without("Viagens")},
	}
}

// CloseWithItsOwnVersion closes month with a version of its own holding what
// is in force for it, as closing a month will.
func CloseWithItsOwnVersion(month brday.Month) func(t testing.TB, database *authored.Database, at time.Time) {
	return func(t testing.TB, database *authored.Database, at time.Time) {
		t.Helper()
		view := database.View()
		budget, err := view.BudgetOf(t.Context(), month)
		view.Close()
		if err != nil {
			t.Fatal(err)
		}
		PutVersion(t, database, at, month, budget.Lines...)
		CloseMonth(t, database, at.Add(time.Second), month, "2026-03-20")
	}
}

// Without is the document with no line called name.
func Without(name string) func(authored.Document) authored.Document {
	return editLines(func(group string, lines []authored.DocumentLine) []authored.DocumentLine {
		return slices.DeleteFunc(lines, func(line authored.DocumentLine) bool { return line.Name == name })
	})
}

// Renamed is the document with the line called from called to, under its id.
func Renamed(from, to string) func(authored.Document) authored.Document {
	return eachLine(func(line *authored.DocumentLine) {
		if line.Name == from {
			line.Name = to
		}
	})
}

// WithCents is the document with the line called name at cents.
func WithCents(name string, cents money.Cents) func(authored.Document) authored.Document {
	return eachLine(func(line *authored.DocumentLine) {
		if line.Name == name {
			line.Cents = cents
		}
	})
}

// Moved is the document with the line called name at the end of group.
func Moved(name, group string) func(authored.Document) authored.Document {
	return func(document authored.Document) authored.Document {
		var moved []authored.DocumentLine
		out := Without(name)(eachLine(func(line *authored.DocumentLine) {
			if line.Name == name {
				moved = append(moved, *line)
			}
		})(document))
		for i := range out {
			if out[i].Key == group {
				out[i].Lines = append(out[i].Lines, moved...)
			}
		}
		return out
	}
}

// Emptied is the five groups with no line.
func Emptied(authored.Document) authored.Document {
	document := make(authored.Document, len(authored.GroupKeys))
	for i, key := range authored.GroupKeys {
		document[i] = authored.DocumentGroup{Key: key, Lines: []authored.DocumentLine{}}
	}
	return document
}

func editLines(edit func(group string, lines []authored.DocumentLine) []authored.DocumentLine) func(authored.Document) authored.Document {
	return func(document authored.Document) authored.Document {
		out := make(authored.Document, len(document))
		for i, group := range document {
			out[i] = authored.DocumentGroup{Key: group.Key, Lines: edit(group.Key, slices.Clone(group.Lines))}
		}
		return out
	}
}

func eachLine(change func(*authored.DocumentLine)) func(authored.Document) authored.Document {
	return editLines(func(_ string, lines []authored.DocumentLine) []authored.DocumentLine {
		for i := range lines {
			change(&lines[i])
		}
		return lines
	})
}
