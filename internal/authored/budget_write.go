package authored

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// SaveBudget makes document the budget from month on, as one recorded change:
//
//   - a closed month is refused and nothing is written;
//   - a closed month after it keeps the budget it had, as a version of its own
//     that is written when none starts there, and the budget resumes the month
//     after it unless that month is closed too;
//   - every other later version is removed, and the document is the version
//     that starts at month.
//
// A line without an id gets one, minted once and carried by every version this
// save writes. Everything is read before anything is written, so what was in
// force for a closed month is what it was before this save. Once the versions
// are written, every indexed transaction loses its references to a line its
// month's budget no longer has, in the same change (ClearLinesTheirMonthLacks).
func (recorder *Recorder) SaveBudget(ctx context.Context, month brday.Month, document Document) error {
	if err := recorder.usable(); err != nil {
		return err
	}
	plan, err := planSave(ctx, recorder.tx, month, document)
	if err != nil {
		var invalid *InvalidBudgetError
		if errors.Is(err, ErrMonthClosed) || errors.As(err, &invalid) {
			return err
		}
		return recorder.fail(err)
	}
	if err := recorder.writeSave(ctx, plan); err != nil {
		return recorder.guard(err)
	}
	return recorder.guard(recorder.clearStrayLines(ctx, EveryIndexedTransaction()))
}

// savePlan is every version the save leaves holding something other than what
// it held, and the later versions it removes.
type savePlan struct {
	write  []plannedVersion
	remove []brday.Month
}

type plannedVersion struct {
	from  brday.Month
	lines []BudgetLine
}

func planSave(ctx context.Context, tx *sql.Tx, month brday.Month, document Document) (*savePlan, error) {
	closed, err := closedMonths(ctx, tx)
	if err != nil {
		return nil, err
	}
	if slices.Contains(closed, month) {
		return nil, ErrMonthClosed
	}
	resolved, invalid, err := resolveDocument(ctx, tx, document)
	if err != nil || invalid != nil {
		return nil, errors.Join(err, invalid)
	}
	later, err := versionsAfter(ctx, tx, month)
	if err != nil {
		return nil, err
	}
	closedAfter := laterThan(closed, month)
	kept, err := keptClosedVersions(ctx, tx, closedAfter, later)
	if err != nil {
		return nil, err
	}
	saved, err := mintAndNumber(ctx, tx, resolved)
	if err != nil {
		return nil, err
	}
	plan := &savePlan{write: []plannedVersion{{from: month, lines: saved}}}
	plan.write = append(plan.write, kept...)
	for _, c := range closedAfter {
		next, err := brday.MonthAfter(c)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(closed, next) {
			plan.write = append(plan.write, plannedVersion{from: next, lines: saved})
		}
	}
	for _, from := range later {
		if !slices.Contains(closedAfter, from) && !slices.ContainsFunc(plan.write, func(v plannedVersion) bool { return v.from == from }) {
			plan.remove = append(plan.remove, from)
		}
	}
	return plan, nil
}

func laterThan(months []brday.Month, month brday.Month) []brday.Month {
	return slices.DeleteFunc(slices.Clone(months), func(found brday.Month) bool { return found <= month })
}

// keptClosedVersions writes, for each closed month with no version of its own,
// what was in force for it. A version that starts at a closed month stays as it
// is and is not written.
func keptClosedVersions(ctx context.Context, tx *sql.Tx, closedAfter, versions []brday.Month) ([]plannedVersion, error) {
	var kept []plannedVersion
	for _, c := range closedAfter {
		if slices.Contains(versions, c) {
			continue
		}
		inForce, err := budgetInForce(ctx, tx, c)
		if err != nil {
			return nil, err
		}
		kept = append(kept, plannedVersion{from: c, lines: inForce.Lines})
	}
	return kept, nil
}

// mintAndNumber gives every new line its id, in the fixed group order, and
// returns the lines a version of this save holds.
func mintAndNumber(ctx context.Context, tx *sql.Tx, resolved []resolvedLine) ([]BudgetLine, error) {
	lines := make([]BudgetLine, 0, len(resolved))
	placed := map[string]int{}
	for _, line := range resolved {
		id := line.id
		if !line.known {
			minted, err := mintLine(ctx, tx)
			if err != nil {
				return nil, err
			}
			id = minted
		}
		lines = append(lines, BudgetLine{Group: line.group, Position: placed[line.group], ID: id, Name: line.Name, Cents: line.Cents, Flag: line.Flag})
		placed[line.group]++
	}
	return lines, nil
}

func (recorder *Recorder) writeSave(ctx context.Context, plan *savePlan) error {
	versions, err := recorder.registry.table("budget_versions")
	if err != nil {
		return err
	}
	lines, err := recorder.registry.table("budget_version_lines")
	if err != nil {
		return err
	}
	for _, from := range plan.remove {
		if err := recorder.delete(ctx, versions, []Value{Text(string(from))}); err != nil {
			return err
		}
	}
	for _, version := range plan.write {
		if err := recorder.writeVersion(ctx, versions, lines, version); err != nil {
			return err
		}
	}
	return nil
}

// writeVersion makes the version at from hold exactly lines. The rows that do
// not stay go first, so a name that changes hands is free by the time its new
// holder is written, and an undo, which walks the effects backwards, frees it
// again before the old holder returns.
func (recorder *Recorder) writeVersion(ctx context.Context, versions, lines *Table, version plannedVersion) error {
	key := []Value{Text(string(version.from))}
	existing, err := readOwned(ctx, recorder.tx, lines, key)
	if err != nil {
		return err
	}
	stays := map[string]bool{}
	for _, line := range version.lines {
		stays[EncodeKey(versionLineKey(version.from, line)...)] = true
	}
	for _, row := range existing {
		rowKey := lines.keyOf(row)
		if !stays[EncodeKey(rowKey...)] {
			if err := recorder.delete(ctx, lines, rowKey); err != nil {
				return err
			}
		}
	}
	if err := recorder.put(ctx, versions, Row{"from_month": key[0]}); err != nil {
		return err
	}
	for _, line := range version.lines {
		if err := recorder.put(ctx, lines, versionLineRow(version.from, line)); err != nil {
			return err
		}
	}
	return nil
}

func versionLineKey(from brday.Month, line BudgetLine) []Value {
	return []Value{Text(string(from)), Integer(int64(line.ID)), Text(line.Name)}
}

func versionLineRow(from brday.Month, line BudgetLine) Row {
	flag := Null
	if line.Flag != "" {
		flag = Text(line.Flag)
	}
	return Row{
		"from_month": Text(string(from)), "group_key": Text(line.Group), "position": Integer(int64(line.Position)),
		"line_id": Integer(int64(line.ID)), "name": Text(line.Name), "cents": Integer(int64(line.Cents)), "flag": flag,
	}
}

type scenarioLineJSON struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name"`
	Cents int64  `json:"cents"`
	Flag  string `json:"flag,omitempty"`
}

type scenarioGroupJSON struct {
	Key   string             `json:"key"`
	Lines []scenarioLineJSON `json:"lines"`
}

// encodeScenario is the bytes a scenario's document is stored as: the five
// groups in the fixed order, amounts as integer cents, ids as decimal text.
func encodeScenario(resolved []resolvedLine) (string, error) {
	groups := make([]scenarioGroupJSON, len(GroupKeys))
	for i, key := range GroupKeys {
		groups[i] = scenarioGroupJSON{Key: key, Lines: []scenarioLineJSON{}}
	}
	for _, line := range resolved {
		entry := scenarioLineJSON{Name: line.Name, Cents: int64(line.Cents), Flag: line.Flag}
		if line.known {
			entry.ID = line.id.String()
		}
		at := groupIndex(line.group)
		groups[at].Lines = append(groups[at].Lines, entry)
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(groups); err != nil {
		return "", err
	}
	return strings.TrimSuffix(encoded.String(), "\n"), nil
}

func decodeScenario(doc string) (Document, error) {
	var groups []scenarioGroupJSON
	if err := json.Unmarshal([]byte(doc), &groups); err != nil {
		return nil, err
	}
	document := make(Document, len(groups))
	for i, group := range groups {
		document[i] = DocumentGroup{Key: group.Key, Lines: make([]DocumentLine, len(group.Lines))}
		for j, line := range group.Lines {
			found := DocumentLine{Name: line.Name, Cents: money.Cents(line.Cents), Flag: line.Flag}
			if line.ID != "" {
				id := line.ID
				found.ID = &id
			}
			document[i].Lines[j] = found
		}
	}
	return document, nil
}

// SaveScenario keeps document under name as one recorded change. A scenario
// saved under a name that exists replaces it and goes to the end of the list.
// It mints nothing: a line without an id is stored without one. name is stored
// as given; the caller has trimmed it and chosen the name of a blank one.
func (recorder *Recorder) SaveScenario(ctx context.Context, name string, document Document) error {
	if err := recorder.usable(); err != nil {
		return err
	}
	if name == "" {
		return errors.New("authored: a scenario has a name")
	}
	resolved, invalid, err := resolveDocument(ctx, recorder.tx, document)
	if err != nil {
		return recorder.fail(err)
	}
	if invalid != nil {
		return invalid
	}
	doc, err := encodeScenario(resolved)
	if err != nil {
		return recorder.fail(err)
	}
	var last int64
	if err := recorder.tx.QueryRowContext(ctx, statementLastScenarioPlace).Scan(&last); err != nil {
		return recorder.fail(err)
	}
	table, err := recorder.registry.table("scenarios")
	if err != nil {
		return recorder.fail(err)
	}
	return recorder.guard(recorder.put(ctx, table, Row{
		"name": Text(name), "position": Integer(last + 1), "doc": Text(doc),
	}))
}
