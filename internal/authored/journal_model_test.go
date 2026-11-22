package authored_test

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// The property test's oracle: the scratch tables held as Go maps, and the
// journal as a list of changes, written from the rules as the bead states them
// rather than from the package's code. Undo here restores unit by unit in
// reverse order of each unit's first effect; the package plans whole rows.

// mSide is one side of an effect: a column's value, a whole row, or nothing.
type mSide struct {
	present bool
	value   authored.Value
	row     authored.Row
}

type mEffect struct {
	seq           int
	unit          authored.Unit
	before, after mSide
}

type mChange struct {
	id       authored.ChangeID
	method   string
	effects  []mEffect
	undone   bool
	extended bool
	// unordered marks a change whose effects the package records in its own
	// row order (the inverse of a part), compared as a set.
	unordered bool
}

type mCollapsed struct {
	unit              authored.Unit
	firstSeq, lastSeq int
	before, after     mSide
}

type model struct {
	tables  map[string]authored.Table
	state   map[string]map[string]authored.Row
	changes []*mChange
}

func newModel() *model {
	m := &model{tables: map[string]authored.Table{}, state: map[string]map[string]authored.Row{}}
	for _, table := range scratchTables() {
		m.tables[table.Name] = table
		m.state[table.Name] = map[string]authored.Row{}
	}
	return m
}

func keyText(table authored.Table, row authored.Row) string {
	values := make([]authored.Value, len(table.Key))
	for i, name := range table.Key {
		values[i] = row[name]
	}
	return authored.EncodeKey(values...)
}

func keyRow(table authored.Table, key []authored.Value) authored.Row {
	row := authored.Row{}
	for i, name := range table.Key {
		row[name] = key[i]
	}
	return row
}

func copyRow(row authored.Row) authored.Row {
	out := authored.Row{}
	for name, value := range row {
		out[name] = value
	}
	return out
}

func rowsEqual(a, b authored.Row) bool {
	if len(a) != len(b) {
		return false
	}
	for name, value := range a {
		if other, ok := b[name]; !ok || other != value {
			return false
		}
	}
	return true
}

func optional(table authored.Table) bool { return table.Kind == authored.KindOptionalFacts }
func whole(table authored.Table) bool    { return table.Kind == authored.KindCreatedWhole }

// content is a row as a reader sees it: a missing row of optional facts is its defaults.
func (m *model) content(table authored.Table, key string, keyValues authored.Row) (authored.Row, bool) {
	row, found := m.state[table.Name][key]
	if found {
		return copyRow(row), true
	}
	if !optional(table) {
		return nil, false
	}
	row = copyRow(keyValues)
	for _, column := range table.Columns {
		if _, isKey := keyValues[column.Name]; !isKey {
			row[column.Name] = column.Default
		}
	}
	return row, false
}

func (m *model) store(table authored.Table, row authored.Row) {
	key := keyText(table, row)
	if optional(table) && atDefaults(table, row) {
		delete(m.state[table.Name], key)
		return
	}
	m.state[table.Name][key] = copyRow(row)
}

func atDefaults(table authored.Table, row authored.Row) bool {
	for _, column := range table.Columns {
		if !contains(table.Key, column.Name) && row[column.Name] != column.Default {
			return false
		}
	}
	return true
}

// The recorder's writes, as the bead describes them.

func (m *model) set(name string, key []authored.Value, values authored.Row) []mEffect {
	table := m.tables[name]
	k := authored.EncodeKey(key...)
	current, _ := m.content(table, k, keyRow(table, key))
	if current == nil {
		panic("the model was asked to set a column of a thing that does not exist")
	}
	next := copyRow(current)
	var effects []mEffect
	for _, column := range table.Columns {
		value, given := values[column.Name]
		if !given || value == current[column.Name] {
			continue
		}
		next[column.Name] = value
		effects = append(effects, mEffect{
			unit:   authored.Unit{Table: name, Key: k, Column: column.Name},
			before: mSide{present: true, value: current[column.Name]}, after: mSide{present: true, value: value},
		})
	}
	if len(effects) > 0 {
		m.store(table, next)
	}
	return effects
}

func (m *model) create(name string, row authored.Row) []mEffect {
	table := m.tables[name]
	m.state[name][keyText(table, row)] = copyRow(row)
	return []mEffect{{unit: authored.Unit{Table: name, Key: keyText(table, row)}, after: mSide{present: true, row: copyRow(row)}}}
}

func (m *model) put(name string, row authored.Row) []mEffect {
	table := m.tables[name]
	k := keyText(table, row)
	current, found := m.state[name][k]
	if found && rowsEqual(current, row) {
		return nil
	}
	effect := mEffect{unit: authored.Unit{Table: name, Key: k}, after: mSide{present: true, row: copyRow(row)}}
	if found {
		effect.before = mSide{present: true, row: copyRow(current)}
	}
	m.state[name][k] = copyRow(row)
	return []mEffect{effect}
}

func (m *model) remove(name string, key []authored.Value) []mEffect {
	k := authored.EncodeKey(key...)
	current, found := m.state[name][k]
	if !found {
		return nil
	}
	var effects []mEffect
	for _, ownedKey := range m.ownedKeys(name, key) {
		effects = append(effects, m.remove("step_reasons", ownedKey)...)
	}
	delete(m.state[name], k)
	return append(effects, mEffect{unit: authored.Unit{Table: name, Key: k}, before: mSide{present: true, row: current}})
}

// ownedKeys is the scratch registry's one ownership: a step owns its reason.
func (m *model) ownedKeys(name string, key []authored.Value) [][]authored.Value {
	if name != "steps" {
		return nil
	}
	if _, found := m.state["step_reasons"][authored.EncodeKey(key[0])]; found {
		return [][]authored.Value{{key[0]}}
	}
	return nil
}

func (m *model) record(method string, effects []mEffect, askedForID bool) *mChange {
	if len(effects) == 0 && !askedForID {
		return nil
	}
	recorded := &mChange{id: authored.ChangeID(len(m.changes) + 1), method: method}
	m.appendEffects(recorded, effects)
	m.changes = append(m.changes, recorded)
	return recorded
}

func (m *model) appendEffects(target *mChange, effects []mEffect) {
	for _, effect := range effects {
		effect.seq = len(target.effects) + 1
		target.effects = append(target.effects, effect)
	}
}

func (m *model) change(id authored.ChangeID) *mChange {
	if id < 1 || int(id) > len(m.changes) {
		return nil
	}
	return m.changes[id-1]
}

func collapseModel(effects []mEffect) []mCollapsed {
	var units []mCollapsed
	for _, effect := range effects {
		at := slices.IndexFunc(units, func(unit mCollapsed) bool { return unit.unit == effect.unit })
		if at < 0 {
			units = append(units, mCollapsed{unit: effect.unit, firstSeq: effect.seq, lastSeq: effect.seq, before: effect.before, after: effect.after})
			continue
		}
		units[at].lastSeq, units[at].after = effect.seq, effect.after
	}
	return units
}

func (m *model) overlaps(unit, other authored.Unit) bool {
	if unit.Table != other.Table || unit.Key != other.Key {
		return false
	}
	if whole(m.tables[unit.Table]) && (unit.Column == "" || other.Column == "") {
		return true
	}
	return unit.Column == other.Column
}

func (m *model) laterOverlap(id authored.ChangeID, units []mCollapsed) bool {
	for _, later := range m.changes[id:] {
		if later.undone {
			continue
		}
		for _, effect := range later.effects {
			for _, unit := range units {
				if m.overlaps(unit.unit, effect.unit) {
					return true
				}
			}
		}
	}
	return false
}

// holds is the guard against a write that went around the journal.
func (m *model) holds(units []mCollapsed) bool {
	for _, unit := range units {
		table := m.tables[unit.unit.Table]
		current, found := m.content(table, unit.unit.Key, m.keyValues(table, unit.unit.Key))
		if unit.unit.Column != "" {
			if current == nil || current[unit.unit.Column] != unit.after.value {
				return false
			}
			continue
		}
		if !unit.after.present {
			if found {
				return false
			}
			continue
		}
		expected := copyRow(unit.after.row)
		for _, other := range units {
			if other.unit.Table == unit.unit.Table && other.unit.Key == unit.unit.Key && other.unit.Column != "" && other.lastSeq > unit.lastSeq {
				expected[other.unit.Column] = other.after.value
			}
		}
		if !found || !rowsEqual(current, expected) {
			return false
		}
	}
	return true
}

func (m *model) keyValues(table authored.Table, key string) authored.Row {
	return keyRow(table, parseScratchKey(key))
}

func keyValuesOf(table authored.Table, row authored.Row) []authored.Value {
	values := make([]authored.Value, len(table.Key))
	for i, name := range table.Key {
		values[i] = row[name]
	}
	return values
}

// parseScratchKey reads a key the model wrote, which in the scratch tables
// is one text or one integer, or a text and an integer.
func parseScratchKey(key string) []authored.Value {
	parts := strings.Split(strings.Trim(key, "[]"), ",")
	values := make([]authored.Value, len(parts))
	for i, part := range parts {
		if strings.HasPrefix(part, `"`) {
			values[i] = text(strings.Trim(part, `"`))
			continue
		}
		var n int64
		fmt.Sscan(part, &n)
		values[i] = integer(n)
	}
	return values
}

// predict is what an undo or a part's inverse over units of id must answer.
func (m *model) predict(id authored.ChangeID, selected func(authored.Unit) bool) (authored.StaleCause, []mCollapsed) {
	target := m.change(id)
	if target == nil {
		return authored.CauseUnknown, nil
	}
	if target.undone {
		return authored.CauseUndone, nil
	}
	var units []mCollapsed
	for _, unit := range collapseModel(target.effects) {
		if selected == nil || selected(unit.unit) {
			units = append(units, unit)
		}
	}
	if m.laterOverlap(id, units) {
		return authored.CauseLaterChange, nil
	}
	if !m.holds(units) {
		return authored.CauseOverwritten, nil
	}
	return 0, units
}

// restore puts each unit back to its first before, units first touched last
// first, recording the restores as effects when recording is asked for.
func (m *model) restore(units []mCollapsed, recording bool) []mEffect {
	ordered := slices.Clone(units)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].firstSeq > ordered[j].firstSeq })
	var effects []mEffect
	for _, unit := range ordered {
		table := m.tables[unit.unit.Table]
		key := m.keyValues(table, unit.unit.Key)
		switch {
		case unit.unit.Column != "" && recording:
			effects = append(effects, m.set(table.Name, keyValuesOf(table, key), authored.Row{unit.unit.Column: unit.before.value})...)
		case unit.unit.Column != "":
			current, _ := m.content(table, unit.unit.Key, key)
			current[unit.unit.Column] = unit.before.value
			m.store(table, current)
		case unit.before.present:
			m.state[table.Name][unit.unit.Key] = copyRow(unit.before.row)
		default:
			for _, owned := range m.ownedKeys(table.Name, keyValuesOf(table, key)) {
				delete(m.state["step_reasons"], authored.EncodeKey(owned...))
			}
			delete(m.state[table.Name], unit.unit.Key)
		}
	}
	return effects
}
