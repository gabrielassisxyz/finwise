package authored_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

var (
	journalSeed = flag.Int64("journal.seed", 0, "run the journal's property test on this seed alone")
	journalRuns = flag.Int("journal.runs", 24, "how many seeds the journal's property test runs")
)

const fillerSteps = 36

// Every run takes each of these at least once, in a random order, between
// random filler steps.
var mandatorySteps = []string{
	"facts.firstUnderSecond", "facts.existingRow", "facts.setLine", "facts.bigAmount",
	"things.creationRefused", "things.existingRow", "things.delete",
	"set.insert", "set.delete", "set.replace",
	"owned.write", "owned.ownerRemoved",
	"family.parentFirst", "family.childrenFirst", "family.removeParentFirst", "family.removeChildrenFirst",
	"things.rewrite.setDelete", "things.rewrite.createSet", "things.rewrite.setDeleteCreate", "things.rewrite.deleteCreateSet",
	"nothing", "nothing.sameContent", "empty",
	"extend", "part", "interfere.thenUndo", "abcb", "undo", "undo.latest",
}

var fillerChoices = []string{
	"facts.existingRow", "facts.setLine", "things.existingRow", "things.delete", "things.create",
	"set.insert", "set.delete", "set.replace", "owned.write", "owned.ownerRemoved",
	"family.parentFirst", "family.childrenFirst", "family.removeParentFirst", "family.removeChildrenFirst",
	"extend", "part", "interfere", "undo", "undo", "undo", "undo.latest", "undo.latest",
	"things.rewrite.random", "things.rewrite.random",
}

// outcomes counts, over every run, how undos, extensions and parts ended, so
// the test can fail when a run of seeds never reached an ending it must prove.
type outcomes map[string]int

func TestTheJournalProperty(t *testing.T) {
	seeds := make([]int64, 0, *journalRuns)
	if *journalSeed != 0 {
		seeds = append(seeds, *journalSeed)
	} else {
		for i := range *journalRuns {
			seeds = append(seeds, int64(i+1))
		}
	}
	seen := outcomes{}
	for _, seed := range seeds {
		r := &run{t: t, seed: seed, rng: rand.New(rand.NewSource(seed)), m: newModel(), seen: seen, ran: map[string]int{}}
		r.db = openScratch(t, "")
		r.sequence()
	}
	t.Logf("endings over %d seeds: %v", len(seeds), seen)
	if *journalSeed != 0 {
		return
	}
	for _, ending := range []string{
		"undo ok", "undo refused 1", "undo refused 2", "undo refused 3", "undo refused 4",
		"extend ok", "extend refused", "part ok", "part refused",
	} {
		if seen[ending] == 0 {
			t.Errorf("no run reached %q; the generator no longer covers it", ending)
		}
	}
}

type run struct {
	t       *testing.T
	seed    int64
	rng     *rand.Rand
	db      *authored.Database
	m       *model
	steps   []string
	seen    outcomes
	ran     map[string]int
	counter int64
	// dumps is the package's dump before each change. It stops being the state
	// to return to once anything but a later change and its undo has happened
	// since: an earlier change undone or extended, or a write around the journal.
	dumps   map[authored.ChangeID]string
	spoiled map[authored.ChangeID]bool
}

// spoilAfter marks the snapshot of every change after id.
func (r *run) spoilAfter(id authored.ChangeID) {
	for _, recorded := range r.m.changes {
		if recorded.id > id {
			r.spoiled[recorded.id] = true
		}
	}
}

func (r *run) failf(format string, args ...any) {
	r.t.Helper()
	r.t.Fatalf("seed %d: %s\nsteps:\n  %s\nreproduce with: go test ./internal/authored -run TestTheJournalProperty -args -journal.seed=%d",
		r.seed, fmt.Sprintf(format, args...), strings.Join(r.steps, "\n  "), r.seed)
}

func (r *run) logf(format string, args ...any) {
	r.steps = append(r.steps, fmt.Sprintf(format, args...))
}

func (r *run) sequence() {
	r.dumps, r.spoiled = map[authored.ChangeID]string{}, map[authored.ChangeID]bool{}
	plan := append([]string{}, mandatorySteps...)
	for range fillerSteps {
		plan = append(plan, fillerChoices[r.rng.Intn(len(fillerChoices))])
	}
	r.rng.Shuffle(len(plan), func(i, j int) { plan[i], plan[j] = plan[j], plan[i] })
	for _, name := range plan {
		r.logf("# %s", name)
		r.step(name)
		r.ran[name]++
	}
	for _, name := range mandatorySteps {
		if r.ran[name] == 0 {
			r.failf("the step %s never ran", name)
		}
	}
}

func (r *run) fresh() int64 {
	r.counter++
	return r.counter
}

// A recorded write, on the database and on the model alike.
type pwrite struct {
	describe string
	db       func(*authored.Recorder) error
	model    func(*model) []mEffect
}

func wSet(table string, key []authored.Value, values authored.Row) pwrite {
	return pwrite{
		describe: fmt.Sprintf("set %s %s %v", table, authored.EncodeKey(key...), describeRow(values)),
		db:       func(rec *authored.Recorder) error { return rec.Set(bg, table, key, values) },
		model:    func(m *model) []mEffect { return m.set(table, key, values) },
	}
}

func wCreate(row authored.Row) pwrite {
	return pwrite{
		describe: "create things " + describeRow(row),
		db:       func(rec *authored.Recorder) error { return rec.Create(bg, "things", row) },
		model:    func(m *model) []mEffect { return m.create("things", row) },
	}
}

func wPut(table string, row authored.Row) pwrite {
	return pwrite{
		describe: "put " + table + " " + describeRow(row),
		db:       func(rec *authored.Recorder) error { return rec.Put(bg, table, row) },
		model:    func(m *model) []mEffect { return m.put(table, row) },
	}
}

func wDelete(table string, key ...authored.Value) pwrite {
	return pwrite{
		describe: "delete " + table + " " + authored.EncodeKey(key...),
		db:       func(rec *authored.Recorder) error { return rec.Delete(bg, table, key) },
		model:    func(m *model) []mEffect { return m.remove(table, key) },
	}
}

func describeRow(row authored.Row) string {
	names := make([]string, 0, len(row))
	for name := range row {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = name + "=" + valueText(row[name])
	}
	return "{" + strings.Join(parts, " ") + "}"
}

// valueText is a value's canonical JSON, which the package writes as the one
// element of a key.
func valueText(value authored.Value) string {
	return strings.TrimSuffix(strings.TrimPrefix(authored.EncodeKey(value), "["), "]")
}

// commit records writes as one change and checks the database against the model.
func (r *run) commit(method string, askID bool, writes ...pwrite) *mChange {
	r.t.Helper()
	descriptions := make([]string, len(writes))
	for i, write := range writes {
		descriptions[i] = write.describe
	}
	r.logf("change %s ask-id=%v: %s", method, askID, strings.Join(descriptions, "; "))
	before := dump(r.t, r.db)
	var got authored.ChangeID
	err := r.db.Change(bg, facts(method), func(rec *authored.Recorder) error {
		for _, write := range writes {
			if err := write.db(rec); err != nil {
				return err
			}
		}
		if !askID {
			return nil
		}
		var err error
		got, err = rec.ID(bg)
		return err
	})
	if err != nil {
		r.failf("the change failed: %v", err)
	}
	var effects []mEffect
	for _, write := range writes {
		effects = append(effects, write.model(r.m)...)
	}
	recorded := r.m.record(method, effects, askID)
	if recorded != nil && askID && got != recorded.id {
		r.failf("the change answered id %s, the model %s", got, recorded.id)
	}
	if recorded != nil {
		r.dumps[recorded.id] = before
	}
	r.compare()
	return recorded
}

// undo asks the package to undo id and wants what the model predicts.
func (r *run) undo(id authored.ChangeID) authored.StaleCause {
	r.t.Helper()
	cause, units := r.m.predict(id, nil)
	r.logf("undo %s, model says %s", id, describeCause(cause))
	before := everything(r.t, r.db)
	err := r.db.Undo(bg, id, undoInstant)
	if cause != 0 {
		var refused *authored.StaleChangeError
		if !errors.As(err, &refused) || refused.Cause != cause {
			r.failf("undo %s answered %v, the model wants a refusal of cause %d", id, err, cause)
		}
		if after := everything(r.t, r.db); after != before {
			r.failf("a refused undo changed something:\nbefore:\n%s\nafter:\n%s", before, after)
		}
		r.seen[fmt.Sprintf("undo refused %d", cause)]++
		return cause
	}
	if err != nil {
		r.failf("undo %s failed where the model succeeds: %v", id, err)
	}
	r.m.restore(units, false)
	r.m.change(id).undone = true
	r.compare()
	r.checkSnapshot(id)
	r.spoilAfter(id)
	r.seen["undo ok"]++
	return 0
}

// checkSnapshot: once every later change is undone, undoing a change returns
// the dump to what it was just before it, unless a write went around the
// journal since.
func (r *run) checkSnapshot(id authored.ChangeID) {
	for _, later := range r.m.changes[id:] {
		if !later.undone {
			return
		}
	}
	if r.spoiled[id] {
		return
	}
	if got := dump(r.t, r.db); got != r.dumps[id] {
		r.failf("after undoing %s and every later change, the dump is\n%s\nnot the one before it:\n%s", id, got, r.dumps[id])
	}
}

func (r *run) undoWanting(id authored.ChangeID, want authored.StaleCause) {
	r.t.Helper()
	if got := r.undo(id); got != want {
		r.failf("undo %s ended with cause %d, the step needs %d", id, got, want)
	}
}

func describeCause(cause authored.StaleCause) string {
	if cause == 0 {
		return "it succeeds"
	}
	return fmt.Sprintf("refused, cause %d", cause)
}

// Random budget histories over the app's own tables: saves over a few months,
// many of which add one line to a version another save created, and undos of
// any change in force. A refused undo changes nothing, and after every step
// each row is as the last change in force that wrote it left it.

var budgetHistoryMonths = []brday.Month{"2026-02", "2026-03", "2026-04"}

const (
	budgetHistorySeeds = 40
	budgetHistorySteps = 24
)

type budgetHistory struct {
	*budgetEnv
	seed   int64
	random *rand.Rand
	names  int
	steps  []string
}

func TestEveryChangeInForceKeepsTheRowsItLeftThroughRandomBudgetSavesAndUndos(t *testing.T) {
	endings := map[string]int{}
	for seed := int64(1); seed <= budgetHistorySeeds; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			history := &budgetHistory{budgetEnv: newBudgetEnvWithNothing(t), seed: seed, random: rand.New(rand.NewSource(seed))}
			for range budgetHistorySteps {
				if history.random.Intn(5) < 2 {
					endings[history.undoAny()]++
				} else {
					history.saveRandom()
				}
				history.expectEveryChangeInForceWhole()
			}
		})
	}
	t.Logf("undos over %d seeds: %v", budgetHistorySeeds, endings)
	for _, ending := range []string{"accepted", refusedFor(authored.CauseOwnedRowHeld), refusedFor(authored.CauseLaterChange)} {
		if endings[ending] == 0 {
			t.Errorf("no undo ended %q; the histories no longer reach it", ending)
		}
	}
}

func refusedFor(cause authored.StaleCause) string { return fmt.Sprintf("refused %d", cause) }

func (history *budgetHistory) record(format string, args ...any) {
	history.steps = append(history.steps, fmt.Sprintf(format, args...))
}

func (history *budgetHistory) fail(format string, args ...any) {
	history.t.Helper()
	history.t.Fatalf("seed %d, after:\n  %s\n%s\nreproduce with: go test ./internal/authored -run 'TestEveryChangeInForceKeepsTheRowsItLeftThroughRandomBudgetSavesAndUndos/^%d$'",
		history.seed, strings.Join(history.steps, "\n  "), fmt.Sprintf(format, args...), history.seed)
}

func (history *budgetHistory) saveRandom() {
	history.t.Helper()
	month := budgetHistoryMonths[history.random.Intn(len(budgetHistoryMonths))]
	document, edit := history.edited(month)
	history.record("save %s with %s", month, edit)
	history.record("  change %d", history.save(month, document))
}

// edited is the invented budget, or the budget in force for month with one
// edit, most often a line added at the end of a group, which the save records
// as that line alone.
func (history *budgetHistory) edited(month brday.Month) (authored.Document, string) {
	budget := history.budget(month)
	if !budget.HasLines() || history.random.Intn(8) == 0 {
		return authoredtest.InventedBudget(), "the invented budget"
	}
	document := authoredtest.DocumentOf(budget)
	history.names++
	group := &document[history.random.Intn(len(document))]
	switch roll := history.random.Intn(6); {
	case roll < 3 || len(group.Lines) == 0:
		group.Lines = append(group.Lines, authoredtest.Line(fmt.Sprintf("Linha %d", history.names), 5_000))
		return document, "a line added to " + group.Key
	case roll == 3:
		group.Lines[0].Cents += 100
		return document, group.Lines[0].Name + " given another amount"
	case roll == 4:
		removed := group.Lines[0].Name
		group.Lines = group.Lines[1:]
		return document, removed + " removed"
	default:
		renamed := group.Lines[0].Name
		group.Lines[0].Name = fmt.Sprintf("Nome %d", history.names)
		return document, renamed + " renamed"
	}
}

// undoAny undoes a random change in force and answers how it ended.
func (history *budgetHistory) undoAny() string {
	history.t.Helper()
	var open []authoredtest.Change
	for _, change := range history.scratch.Changes(history.t) {
		if change.UndoneAt == "" {
			open = append(open, change)
		}
	}
	if len(open) == 0 {
		return "nothing to undo"
	}
	picked := open[history.random.Intn(len(open))]
	history.record("undo %d", picked.ID)
	state := history.scratch.State(history.t)
	err := history.undo(authored.ChangeID(picked.ID))
	var stale *authored.StaleChangeError
	switch {
	case errors.As(err, &stale):
		history.record("  refused, cause %d", stale.Cause)
		if history.scratch.State(history.t) != state {
			history.fail("the refused undo of %d changed the tables or the journal", picked.ID)
		}
		return refusedFor(stale.Cause)
	case err != nil:
		history.fail("the undo of %d failed: %v", picked.ID, err)
	}
	return "accepted"
}

const selectEffectsInForce = `
SELECT e.change_id, e.tbl, e.key, coalesce(e.col, ''), e.after
FROM change_effects AS e JOIN changes AS c ON c.id = e.change_id
WHERE c.undone_at IS NULL ORDER BY e.change_id, e.seq`

// leftBy is what the last change in force that wrote a row left in it, absent
// for a row it removed.
type leftBy struct {
	change int64
	after  sql.NullString
}

func (history *budgetHistory) expectEveryChangeInForceWhole() {
	history.t.Helper()
	rows := history.rowsByUnit()
	for unit, left := range history.leftInForce() {
		got, found := rows[unit]
		switch {
		case left.after.Valid && got != left.after.String:
			history.fail("change %d, in force, left %s %s as %s; it now holds %q", left.change, unit.Table, unit.Key, left.after.String, got)
		case !left.after.Valid && found:
			history.fail("change %d, in force, removed %s %s; it now holds %s", left.change, unit.Table, unit.Key, got)
		}
	}
}

func (history *budgetHistory) leftInForce() map[authored.Unit]leftBy {
	history.t.Helper()
	rows, err := history.scratch.Inspect(history.t).QueryContext(history.t.Context(), selectEffectsInForce)
	if err != nil {
		history.t.Fatal(err)
	}
	defer rows.Close()
	left := map[authored.Unit]leftBy{}
	for rows.Next() {
		var unit authored.Unit
		var by leftBy
		if err := rows.Scan(&by.change, &unit.Table, &unit.Key, &unit.Column, &by.after); err != nil {
			history.t.Fatal(err)
		}
		if unit.Column != "" {
			history.fail("change %d wrote the column %s of %s, which this check does not read", by.change, unit.Column, unit.Table)
		}
		left[unit] = by
	}
	if err := rows.Err(); err != nil {
		history.t.Fatal(err)
	}
	return left
}

// rowsByUnit is every row of the app's journaled tables as the dump encodes it,
// which is how an effect encodes a row, under the unit of its key.
func (history *budgetHistory) rowsByUnit() map[authored.Unit]string {
	history.t.Helper()
	keys := map[string][]string{}
	for _, table := range authored.AppTables() {
		keys[table.Name] = table.Key
	}
	rows := map[authored.Unit]string{}
	for _, line := range strings.Split(strings.TrimSuffix(dump(history.t, history.scratch.Database), "\n"), "\n") {
		if line == "" {
			continue
		}
		table, encoded, _ := strings.Cut(line, " ")
		key, ours := keys[table]
		if !ours {
			history.fail("the dump holds a row of %s, which is not a table of the app", table)
		}
		rows[authored.Unit{Table: table, Key: keyOfEncoded(history.t, key, encoded)}] = encoded
	}
	return rows
}

func keyOfEncoded(t testing.TB, key []string, encoded string) string {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.UseNumber()
	var row map[string]any
	if err := decoder.Decode(&row); err != nil {
		t.Fatalf("%s: %v", encoded, err)
	}
	values := make([]authored.Value, len(key))
	for i, name := range key {
		switch typed := row[name].(type) {
		case string:
			values[i] = text(typed)
		case json.Number:
			number, err := typed.Int64()
			if err != nil {
				t.Fatalf("%s: %v", encoded, err)
			}
			values[i] = integer(number)
		default:
			values[i] = null
		}
	}
	return authored.EncodeKey(values...)
}
