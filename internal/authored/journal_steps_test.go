package authored_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

var bg = context.Background()

func (r *run) step(name string) {
	r.t.Helper()
	switch name {
	case "facts.firstUnderSecond":
		k := key(text(fmt.Sprintf("f%d", r.fresh())))
		first := r.commit("facts.setRefund", true, wSet("facts", k, authored.Row{"refund": integer(1)}))
		r.commit("facts.setAmount", true, wSet("facts", k, authored.Row{"amount": integer(r.rng.Int63n(1000) + 1)}))
		r.undoWanting(first.id, 0)
	case "facts.existingRow":
		k := r.existingFacts()
		values := authored.Row{"amount": integer(r.rng.Int63n(5000))}
		if r.rng.Intn(2) == 0 {
			values = authored.Row{"refund": integer(r.rng.Int63n(2))}
		}
		r.commit("facts.setAmount", true, wSet("facts", k, values))
	case "facts.setLine":
		r.commit(setLine, true, wSet("facts", r.someFactsKey(), r.lineValues()))
	case "facts.bigAmount":
		r.commit("facts.setAmount", true, wSet("facts", r.someFactsKey(), authored.Row{"amount": integer(beyondFloat - r.rng.Int63n(2)*2)}))
	case "things.create":
		r.createThing()
	case "things.creationRefused":
		id := r.createThing()
		k := key(integer(id))
		creation := r.m.changes[len(r.m.changes)-1].id
		r.commit("things.setAmount", true, wSet("things", k, authored.Row{"amount": integer(777)}))
		if r.rng.Intn(2) == 0 {
			r.commit("things.setAmount", true, wSet("things", k, authored.Row{"amount": integer(id * 10)}))
		}
		r.undoWanting(creation, authored.CauseLaterChange)
	case "things.existingRow":
		k := key(integer(r.existingThing()))
		values := []authored.Row{
			{"amount": integer(r.rng.Int63n(9000))},
			{"note": text(fmt.Sprintf("note %d", r.fresh()))},
			{"note": null},
			{"name": text(fmt.Sprintf("n%d", r.fresh()))},
		}[r.rng.Intn(4)]
		r.commit("things.set", true, wSet("things", k, values))
	case "things.delete":
		r.commit("things.delete", true, wDelete("things", integer(r.existingThing())))
	case "set.insert":
		r.commit("tags.add", true, wPut("tags", tag(r.factsID(), fmt.Sprintf("#t%d", r.fresh()))))
	case "set.delete":
		row := r.existingRow("tags", func() { r.step("set.insert") })
		r.commit("tags.remove", true, wDelete("tags", row["tx"], row["tag"]))
	case "set.replace":
		row := r.existingRow("parts", func() {
			r.commit("parts.put", true, wPut("parts", part(r.factsID(), r.fresh(), text("Mercado"), 100)))
		})
		replaced := copyRow(row)
		if row["line"].IsNull() {
			replaced["line"] = text(fmt.Sprintf("line %d", r.fresh()))
		} else {
			replaced["line"] = null
		}
		replaced["amount"] = integer(r.rng.Int63n(500))
		r.commit("parts.put", true, wPut("parts", replaced))
	case "owned.write":
		row := r.existingRow("steps", func() { r.createStep() })
		id, _ := row["id"].Int64()
		r.commit("steps.setReason", true, wPut("step_reasons", reason(id, fmt.Sprintf("why %d", r.fresh()))))
	case "owned.ownerRemoved":
		id := r.createStep()
		r.commit("steps.setReason", true, wPut("step_reasons", reason(id, "because")))
		r.commit("steps.remove", true, wDelete("steps", integer(id)))
	case "family.parentFirst", "family.childrenFirst":
		r.createFamily(name == "family.parentFirst")
	case "family.removeParentFirst", "family.removeChildrenFirst":
		r.removeFamily(name == "family.removeParentFirst")
	case "nothing":
		r.commit("test.nothing", false)
	case "nothing.sameContent":
		k := r.existingFacts()
		current, _ := r.m.content(r.m.tables["facts"], authored.EncodeKey(k...), keyRow(r.m.tables["facts"], k))
		r.commit("facts.setAmount", false, wSet("facts", k, authored.Row{"amount": current["amount"]}))
	case "empty":
		r.commit("test.empty", true)
	case "extend":
		r.extend()
	case "part":
		r.part()
	case "interfere":
		r.interfere()
	case "interfere.thenUndo":
		k := key(text(fmt.Sprintf("f%d", r.fresh())))
		victim := r.commit("facts.setAmount", true, wSet("facts", k, authored.Row{"amount": integer(5)}))
		r.around(`UPDATE facts SET amount = amount + 1 WHERE id = ?`, arg(k[0]))
		r.undoWanting(victim.id, authored.CauseOverwritten)
	case "abcb":
		k := key(text(fmt.Sprintf("f%d", r.fresh())))
		r.commit("facts.setAmount", true, wSet("facts", k, authored.Row{"amount": integer(1)}))
		toB := r.commit("facts.setAmount", true, wSet("facts", k, authored.Row{"amount": integer(2)}))
		toC := r.commit("facts.setAmount", true, wSet("facts", k, authored.Row{"amount": integer(3)}))
		backToB := r.commit("facts.setAmount", true, wSet("facts", k, authored.Row{"amount": integer(2)}))
		r.undoWanting(toB.id, authored.CauseLaterChange)
		r.undoWanting(backToB.id, 0)
		r.undoWanting(toC.id, 0)
		r.undoWanting(toB.id, 0)
	case "undo":
		r.undo(authored.ChangeID(r.rng.Intn(len(r.m.changes) + 2)))
	case "undo.latest":
		for i := len(r.m.changes) - 1; i >= 0; i-- {
			if !r.m.changes[i].undone {
				r.undo(r.m.changes[i].id)
				return
			}
		}
		r.undo(authored.ChangeID(len(r.m.changes) + 1))
	default:
		r.failf("no step is called %s", name)
	}
}

// line and asking go together, as the scratch table's check demands.
func (r *run) lineValues() authored.Row {
	if r.rng.Intn(4) == 0 {
		return authored.Row{"line": null, "asking": integer(0)}
	}
	return authored.Row{"line": text(fmt.Sprintf("line %d", r.rng.Intn(5))), "asking": integer(1)}
}

func (r *run) factsID() string {
	return fmt.Sprintf("f%d", r.rng.Int63n(r.counter+1))
}

func (r *run) someFactsKey() []authored.Value {
	if r.rng.Intn(3) == 0 {
		return key(text(fmt.Sprintf("f%d", r.fresh())))
	}
	return key(text(r.factsID()))
}

func (r *run) existingFacts() []authored.Value {
	row := r.existingRow("facts", func() {
		r.commit("facts.setAmount", true, wSet("facts", key(text(fmt.Sprintf("f%d", r.fresh()))), authored.Row{"amount": integer(10)}))
	})
	return key(row["id"])
}

func (r *run) createThing() int64 {
	id := r.fresh()
	note := null
	if r.rng.Intn(2) == 0 {
		note = text("monthly")
	}
	r.commit("things.create", true, wCreate(thing(id, fmt.Sprintf("n%d", r.fresh()), id*10, note)))
	return id
}

func (r *run) existingThing() int64 {
	row := r.existingRow("things", func() { r.createThing() })
	id, _ := row["id"].Int64()
	return id
}

func (r *run) createStep() int64 {
	id := r.fresh()
	r.commit("steps.add", true, wPut("steps", step(id, r.factsID(), "line")))
	return id
}

// existingRow is a random row of table, made first by create when there is none.
func (r *run) existingRow(table string, create func()) authored.Row {
	if len(r.m.state[table]) == 0 {
		create()
	}
	keys := slices.Sorted(maps.Keys(r.m.state[table]))
	if len(keys) == 0 {
		r.failf("no row of %s to write", table)
	}
	return copyRow(r.m.state[table][keys[r.rng.Intn(len(keys))]])
}

func (r *run) createFamily(parentFirst bool) {
	id := r.fresh()
	children := []pwrite{wPut("children", child(r.fresh(), id, "a")), wPut("children", child(r.fresh(), id, "b"))}
	writes := append([]pwrite{wPut("parents", parent(id, "p"))}, children...)
	if !parentFirst {
		writes = append(children, wPut("parents", parent(id, "p")))
	}
	r.commit("family.add", true, writes...)
}

func (r *run) removeFamily(parentFirst bool) {
	row := r.existingRow("parents", func() { r.createFamily(true) })
	var children []pwrite
	for _, childRow := range r.m.state["children"] {
		if childRow["parent_id"] == row["id"] {
			children = append(children, wDelete("children", childRow["id"]))
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].describe < children[j].describe })
	writes := append([]pwrite{wDelete("parents", row["id"])}, children...)
	if !parentFirst {
		writes = append(children, wDelete("parents", row["id"]))
	}
	r.commit("family.remove", true, writes...)
}

// around is a write that escapes the journal: the model's state follows it,
// its journal does not.
func (r *run) around(statement string, args ...any) {
	r.t.Helper()
	r.logf("around the journal: %s %v", statement, args)
	if _, err := r.db.Pool().ExecContext(bg, statement, args...); err != nil {
		r.failf("%s: %v", statement, err)
	}
	r.spoilAfter(0)
	r.m.state = r.readState()
	r.compare()
}

func (r *run) interfere() {
	switch r.rng.Intn(3) {
	case 0:
		r.around(`UPDATE facts SET amount = amount + 1 WHERE id = ?`, arg(r.existingFacts()[0]))
	case 1:
		r.around(`UPDATE things SET note = 'around' WHERE id = ?`, r.existingThing())
	default:
		row := r.existingRow("tags", func() { r.step("set.insert") })
		r.around(`DELETE FROM tags WHERE tx = ? AND tag = ?`, arg(row["tx"]), arg(row["tag"]))
	}
}

func (r *run) extend() {
	var candidates []authored.ChangeID
	for _, recorded := range r.m.changes {
		if recorded.method == setLine {
			candidates = append(candidates, recorded.id)
		}
	}
	origin := authored.ChangeID(r.rng.Intn(len(r.m.changes) + 2))
	if len(candidates) > 0 && r.rng.Intn(4) > 0 {
		origin = candidates[r.rng.Intn(len(candidates))]
	}
	write := wSet("facts", r.someFactsKey(), r.lineValues())
	r.logf("extend %s with %s", origin, write.describe)

	saved := cloneState(r.m.state)
	cause, appended := r.predictExtension(origin, write)
	before := everything(r.t, r.db)
	err := r.db.Extend(bg, origin, setLine, write.db)
	if cause != 0 {
		r.m.state = saved
		var refused *authored.StaleChangeError
		if !errors.As(err, &refused) || refused.Cause != cause {
			r.failf("extend %s answered %v, the model wants cause %d", origin, err, cause)
		}
		if after := everything(r.t, r.db); after != before {
			r.failf("a refused extension changed something")
		}
		r.seen["extend refused"]++
		return
	}
	if err != nil {
		r.failf("extend %s failed where the model succeeds: %v", origin, err)
	}
	target := r.m.change(origin)
	r.m.appendEffects(target, appended)
	target.extended = true
	r.spoilAfter(origin)
	r.compare()
	r.seen["extend ok"]++
}

func (r *run) predictExtension(origin authored.ChangeID, write pwrite) (authored.StaleCause, []mEffect) {
	target := r.m.change(origin)
	switch {
	case target == nil:
		return authored.CauseUnknown, nil
	case target.undone:
		return authored.CauseUndone, nil
	case target.method != setLine:
		return authored.CauseOtherMethod, nil
	case target.extended:
		return authored.CauseExtended, nil
	}
	if cause, _ := r.m.predict(origin, nil); cause != 0 {
		return cause, nil
	}
	appended := write.model(r.m)
	units := collapseModel(appended)
	if r.m.laterOverlap(origin, units) {
		return authored.CauseLaterChange, nil
	}
	return 0, appended
}

// part takes back one facts row's units of a change, inside a new change, and
// wants the read-only check to have said so beforehand.
func (r *run) part() {
	var candidates []authored.Unit
	for _, recorded := range r.m.changes {
		for _, effect := range recorded.effects {
			if effect.unit.Table == "facts" {
				candidates = append(candidates, authored.Unit{Table: "facts", Key: fmt.Sprint(recorded.id) + " " + effect.unit.Key})
			}
		}
	}
	if len(candidates) == 0 {
		r.step("facts.setLine")
		r.part()
		return
	}
	chosen := strings.SplitN(candidates[r.rng.Intn(len(candidates))].Key, " ", 2)
	var source int64
	fmt.Sscan(chosen[0], &source)
	selected := selectKey("facts", chosen[1])
	r.logf("part of %d, facts %s", source, chosen[1])

	can, err := r.db.CanRestorePart(bg, authored.ChangeID(source), selected)
	if err != nil {
		r.failf("the read-only check failed: %v", err)
	}
	cause, units := r.m.predict(authored.ChangeID(source), selected)
	if can != (cause == 0) {
		r.failf("the read-only check says %v for part of %d, the model's cause is %d", can, source, cause)
	}
	before := everything(r.t, r.db)
	var got authored.ChangeID
	err = r.db.Change(bg, facts("facts.undoHistoryStep"), func(rec *authored.Recorder) error {
		if err := rec.RestorePart(bg, authored.ChangeID(source), selected); err != nil {
			return err
		}
		var err error
		got, err = rec.ID(bg)
		return err
	})
	if cause != 0 {
		var refused *authored.StaleChangeError
		if !errors.As(err, &refused) || refused.Cause != cause {
			r.failf("part of %d answered %v, the model wants cause %d", source, err, cause)
		}
		if after := everything(r.t, r.db); after != before {
			r.failf("a refused part changed something")
		}
		r.seen["part refused"]++
		return
	}
	if err != nil {
		r.failf("part of %d failed where the read-only check and the model succeed: %v", source, err)
	}
	recorded := r.m.record("facts.undoHistoryStep", r.m.restore(units, true), true)
	recorded.unordered = true
	if got != recorded.id {
		r.failf("the part's change is %s, the model's %s", got, recorded.id)
	}
	r.dumps[recorded.id] = dumpOf(before)
	r.compare()
	r.seen["part ok"]++
}

func dumpOf(everythingText string) string {
	return strings.SplitN(everythingText, "\n--\n", 2)[0]
}

func cloneState(state map[string]map[string]authored.Row) map[string]map[string]authored.Row {
	out := map[string]map[string]authored.Row{}
	for table, rows := range state {
		out[table] = map[string]authored.Row{}
		for k, row := range rows {
			out[table][k] = copyRow(row)
		}
	}
	return out
}

// compare holds the database against the model: every scratch table, every
// change's row and every change's effects.
func (r *run) compare() {
	r.t.Helper()
	got := r.readState()
	for name, rows := range r.m.state {
		if len(got[name]) != len(rows) {
			r.failf("%s holds %d rows, the model %d:\n%v\nmodel:\n%v", name, len(got[name]), len(rows), got[name], rows)
		}
		for k, row := range rows {
			if !rowsEqual(got[name][k], row) {
				r.failf("%s %s holds %v, the model %v", name, k, describeRow(got[name][k]), describeRow(row))
			}
		}
	}
	changes := columnValues(r.t, r.db.Pool(), "SELECT id || ' ' || method || ' ' || extended || ' ' || (undone_at IS NOT NULL) FROM changes ORDER BY id")
	if len(changes) != len(r.m.changes) {
		r.failf("the journal holds %d changes, the model %d", len(changes), len(r.m.changes))
	}
	for i, recorded := range r.m.changes {
		want := fmt.Sprintf("%s %s %d %d", recorded.id, recorded.method, boolInt(recorded.extended), boolInt(recorded.undone))
		if changes[i] != want {
			r.failf("change row %q, the model %q", changes[i], want)
		}
		r.compareEffects(recorded)
	}
}

func (r *run) compareEffects(recorded *mChange) {
	var got, want []string
	for _, effect := range effectsOf(r.t, r.db, recorded.id) {
		column := effect.column
		if effect.columnNull {
			column = "-"
		}
		got = append(got, fmt.Sprintf("%d %s %s %s %s %s", effect.seq, effect.table, effect.key, column,
			nullText(effect.before), nullText(effect.after)))
	}
	for _, effect := range recorded.effects {
		table := r.m.tables[effect.unit.Table]
		column := effect.unit.Column
		if column == "" {
			column = "-"
		}
		want = append(want, fmt.Sprintf("%d %s %s %s %s %s", effect.seq, effect.unit.Table, effect.unit.Key, column,
			sideText(table, effect.before), sideText(table, effect.after)))
	}
	if recorded.unordered {
		got, want = withoutSeq(got), withoutSeq(want)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		r.failf("change %s holds effects\n%s\nthe model\n%s", recorded.id, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func withoutSeq(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = strings.SplitN(line, " ", 2)[1]
	}
	sort.Strings(out)
	return out
}

func nullText(value sql.NullString) string {
	if !value.Valid {
		return "-"
	}
	return value.String
}

// sideText writes a side as the bead's canonical form: a value, or a row as
// an object of the table's columns in their order.
func sideText(table authored.Table, side mSide) string {
	switch {
	case !side.present:
		return "-"
	case side.row == nil:
		return valueText(side.value)
	}
	parts := make([]string, len(table.Columns))
	for i, column := range table.Columns {
		parts[i] = `"` + column.Name + `":` + valueText(side.row[column.Name])
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// readState reads every scratch table with the test's own SQL, leaving out a
// row of optional facts at its defaults.
func (r *run) readState() map[string]map[string]authored.Row {
	state := map[string]map[string]authored.Row{}
	for name, table := range r.m.tables {
		state[name] = map[string]authored.Row{}
		rows, err := r.db.Pool().QueryContext(bg, "SELECT * FROM "+name)
		if err != nil {
			r.failf("read %s: %v", name, err)
		}
		columns, _ := rows.Columns()
		for rows.Next() {
			scanned := make([]any, len(columns))
			targets := make([]any, len(columns))
			for i := range scanned {
				targets[i] = &scanned[i]
			}
			if err := rows.Scan(targets...); err != nil {
				r.failf("scan %s: %v", name, err)
			}
			row := authored.Row{}
			for i, column := range columns {
				row[column] = valueFromScan(scanned[i])
			}
			if optional(table) && atDefaults(table, row) {
				continue
			}
			state[name][keyText(table, row)] = row
		}
		rows.Close()
	}
	return state
}

func valueFromScan(scanned any) authored.Value {
	switch typed := scanned.(type) {
	case int64:
		return integer(typed)
	case string:
		return text(typed)
	}
	return null
}

func arg(value authored.Value) any {
	if n, ok := value.Int64(); ok {
		return n
	}
	if s, ok := value.Str(); ok {
		return s
	}
	return nil
}
