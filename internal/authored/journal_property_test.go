package authored_test

import (
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
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
	"nothing", "nothing.sameContent", "empty",
	"extend", "part", "interfere.thenUndo", "abcb", "undo", "undo.latest",
}

var fillerChoices = []string{
	"facts.existingRow", "facts.setLine", "things.existingRow", "things.delete", "things.create",
	"set.insert", "set.delete", "set.replace", "owned.write", "owned.ownerRemoved",
	"family.parentFirst", "family.childrenFirst", "family.removeParentFirst", "family.removeChildrenFirst",
	"extend", "part", "interfere", "undo", "undo", "undo", "undo.latest", "undo.latest",
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
