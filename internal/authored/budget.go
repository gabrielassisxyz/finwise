package authored

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// FeesLineName is the built-in line that belongs to no group and no document.
const FeesLineName = "Tarifas e juros"

// GroupKeys are the five groups a budget document has, in the order every
// answer lists them.
var GroupKeys = []string{"rec", "fix", "var", "prov", "res"}

// LineID is the identity of a budget line, written on the wire as a decimal
// string.
type LineID int64

func (id LineID) String() string { return strconv.FormatInt(int64(id), 10) }

// ParseLineID reads the one spelling String writes; "07", "+7" and "abc" are
// the id of no line.
func ParseLineID(text string) (LineID, bool) {
	number, err := strconv.ParseInt(text, 10, 64)
	if err != nil || number < 1 {
		return 0, false
	}
	id := LineID(number)
	return id, id.String() == text
}

// BudgetLine is one line of a budget version, with the name that version gives
// it.
type BudgetLine struct {
	Group string
	// Position is the line's place within its group, from 0.
	Position int
	ID       LineID
	Name     string
	Cents    money.Cents
	// Flag is empty for a line with none.
	Flag string
}

// Budget is the budget in force for a month: the lines of one version, by
// group in the fixed order and within a group by position. Exists is false
// when the database has no version at all, which is a budget with no line.
type Budget struct {
	From   brday.Month
	Exists bool
	Lines  []BudgetLine
}

// HasLines says whether the month's budget has a line, which is when the fees
// line is among what the month has.
func (budget Budget) HasLines() bool { return len(budget.Lines) > 0 }

// DocumentLine is a line as a client sends and reads it. ID is nil for a line
// that is new; it is text because a client's id is whatever it was given, and
// one that names no line is a refusal and not a parse error.
type DocumentLine struct {
	ID    *string
	Name  string
	Cents money.Cents
	Flag  string
}

// DocumentGroup is one group of a document.
type DocumentGroup struct {
	Key   string
	Lines []DocumentLine
}

// Document is a budget as it is saved: groups with lines. The names and order
// of the groups are the server's, so a document carries only their keys.
type Document []DocumentGroup

// Scenario is a saved scenario, its lines' names as they are now.
type Scenario struct {
	Name     string
	Document Document
}

// ErrMonthClosed is a save for a month the person closed.
var ErrMonthClosed = errors.New("authored: the month is closed and its budget does not change")

// InvalidBudgetError is a document that cannot be stored: an empty, repeated
// or reserved name, an id that is not a line's or is repeated, an unknown or
// repeated group. It carries no text of the document.
type InvalidBudgetError struct{}

func (*InvalidBudgetError) Error() string { return "authored: the budget document is not valid" }

func groupIndex(key string) int { return slices.Index(GroupKeys, key) }

func compareBudgetLines(a, b BudgetLine) int {
	if byGroup := groupIndex(a.Group) - groupIndex(b.Group); byGroup != 0 {
		return byGroup
	}
	return a.Position - b.Position
}

// BudgetOf is the budget in force for a month.
func (view *View) BudgetOf(ctx context.Context, month brday.Month) (Budget, error) {
	var budget Budget
	_, err := view.read(ctx, func(tx *sql.Tx) (err error) {
		budget, err = budgetInForce(ctx, tx, month)
		return err
	})
	return budget, err
}

func budgetInForce(ctx context.Context, tx *sql.Tx, month brday.Month) (Budget, error) {
	var from sql.NullString
	if err := tx.QueryRowContext(ctx, statementBudgetVersionInForce, string(month)).Scan(&from); err != nil {
		return Budget{}, err
	}
	if !from.Valid {
		return Budget{}, nil
	}
	lines, err := linesOfVersion(ctx, tx, from.String)
	return Budget{From: brday.Month(from.String), Exists: true, Lines: lines}, err
}

func linesOfVersion(ctx context.Context, tx *sql.Tx, from string) ([]BudgetLine, error) {
	rows, err := tx.QueryContext(ctx, statementBudgetLinesOfVersion, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []BudgetLine
	for rows.Next() {
		var found BudgetLine
		var position, id, cents int64
		var flag sql.NullString
		if err := rows.Scan(&found.Group, &position, &id, &found.Name, &cents, &flag); err != nil {
			return nil, err
		}
		found.Position, found.ID, found.Cents, found.Flag = int(position), LineID(id), money.Cents(cents), flag.String
		lines = append(lines, found)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(lines, compareBudgetLines)
	return lines, nil
}

// Scenarios are the saved scenarios in list order. A line that has an id is
// named as the budget in force for month names it; a line the budget does not
// have, and a line with no id, keeps the name it was saved with.
func (view *View) Scenarios(ctx context.Context, month brday.Month) ([]Scenario, error) {
	var scenarios []Scenario
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		budget, err := budgetInForce(ctx, tx, month)
		if err != nil {
			return err
		}
		scenarios, err = storedScenarios(ctx, tx, budget)
		return err
	})
	return scenarios, err
}

func storedScenarios(ctx context.Context, tx *sql.Tx, current Budget) ([]Scenario, error) {
	rows, err := tx.QueryContext(ctx, statementScenariosInOrder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := map[string]string{}
	for _, line := range current.Lines {
		names[line.ID.String()] = line.Name
	}
	var scenarios []Scenario
	for rows.Next() {
		var name, doc string
		if err := rows.Scan(&name, &doc); err != nil {
			return nil, err
		}
		document, err := decodeScenario(doc)
		if err != nil {
			return nil, err
		}
		for _, group := range document {
			for i, line := range group.Lines {
				if line.ID == nil {
					continue
				}
				if now, found := names[*line.ID]; found {
					group.Lines[i].Name = now
				}
			}
		}
		scenarios = append(scenarios, Scenario{Name: name, Document: document})
	}
	return scenarios, rows.Err()
}

func closedMonths(ctx context.Context, tx *sql.Tx) ([]brday.Month, error) {
	rows, err := tx.QueryContext(ctx, statementClosedMonthsInOrder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var months []brday.Month
	for rows.Next() {
		var month string
		if err := rows.Scan(&month); err != nil {
			return nil, err
		}
		months = append(months, brday.Month(month))
	}
	return months, rows.Err()
}

func versionsAfter(ctx context.Context, tx *sql.Tx, month brday.Month) ([]brday.Month, error) {
	rows, err := tx.QueryContext(ctx, statementBudgetVersionsAfter, string(month))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var months []brday.Month
	for rows.Next() {
		var from string
		if err := rows.Scan(&from); err != nil {
			return nil, err
		}
		months = append(months, brday.Month(from))
	}
	return months, rows.Err()
}

// ValidateDocument refuses a document the three budget writes would not store.
// It reads the lines an id may name and writes nothing.
func (view *View) ValidateDocument(ctx context.Context, document Document) error {
	var invalid error
	_, err := view.read(ctx, func(tx *sql.Tx) (err error) {
		_, invalid, err = resolveDocument(ctx, tx, document)
		return err
	})
	if err != nil {
		return err
	}
	return invalid
}

// resolvedLine is a document line whose id, where it has one, is a line's.
type resolvedLine struct {
	group string
	id    LineID
	known bool
	DocumentLine
}

// resolveDocument checks a document and lists its lines in the fixed group
// order. invalid is the refusal; err is a failure of the database.
func resolveDocument(ctx context.Context, tx *sql.Tx, document Document) (lines []resolvedLine, invalid, err error) {
	byGroup, ok := groupsOf(document)
	if !ok {
		return nil, &InvalidBudgetError{}, nil
	}
	names, ids := map[string]bool{}, map[LineID]bool{}
	for _, key := range GroupKeys {
		for _, line := range byGroup[key] {
			resolved := resolvedLine{group: key, DocumentLine: line}
			if strings.TrimSpace(line.Name) == "" || line.Name == FeesLineName || names[line.Name] {
				return nil, &InvalidBudgetError{}, nil
			}
			names[line.Name] = true
			if line.ID != nil {
				id, exists, err := lineIDNamed(ctx, tx, *line.ID)
				if err != nil {
					return nil, nil, err
				}
				if !exists || ids[id] {
					return nil, &InvalidBudgetError{}, nil
				}
				ids[id] = true
				resolved.id, resolved.known = id, true
			}
			lines = append(lines, resolved)
		}
	}
	return lines, nil, nil
}

// groupsOf indexes a document by group key. A key outside the five, or one
// that appears twice, is not a document: a line is never dropped or merged
// silently.
func groupsOf(document Document) (map[string][]DocumentLine, bool) {
	byGroup := map[string][]DocumentLine{}
	for _, group := range document {
		if groupIndex(group.Key) < 0 {
			return nil, false
		}
		if _, repeated := byGroup[group.Key]; repeated {
			return nil, false
		}
		byGroup[group.Key] = group.Lines
	}
	return byGroup, true
}

// lineIDNamed says whether text is the id of a line a document may carry: one
// that exists and is not the built-in one.
func lineIDNamed(ctx context.Context, tx *sql.Tx, text string) (LineID, bool, error) {
	id, ok := ParseLineID(text)
	if !ok {
		return 0, false, nil
	}
	var builtin int64
	err := tx.QueryRowContext(ctx, statementLineBuiltin, int64(id)).Scan(&builtin)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil && builtin == 0, err
}

func mintLine(ctx context.Context, tx *sql.Tx) (LineID, error) {
	result, err := tx.ExecContext(ctx, statementMintLine)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return LineID(id), err
}
