package authored_test

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

const selectVersionLineRows = `
SELECT group_key, position, line_id, name, cents, coalesce(flag, '<none>')
FROM budget_version_lines WHERE from_month = ? ORDER BY group_key, position, line_id`

// closeOn closes month on day through the store and returns the change.
func (env *budgetEnv) closeOn(month brday.Month, day brday.Day) (authored.ChangeID, error) {
	var id authored.ChangeID
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "review.closeMonth", At: env.at()}
	err := env.scratch.Database.Change(env.t.Context(), facts, func(recorder *authored.Recorder) error {
		if err := recorder.CloseMonth(env.t.Context(), month, day); err != nil {
			return err
		}
		var err error
		id, err = recorder.ID(env.t.Context())
		return err
	})
	return id, err
}

// versionRows is every column of the lines of the version that starts at
// month, one line of text per row.
func (env *budgetEnv) versionRows(month brday.Month) []string {
	env.t.Helper()
	rows, err := env.scratch.Inspect(env.t).Query(selectVersionLineRows, string(month))
	if err != nil {
		env.t.Fatal(err)
	}
	defer rows.Close()
	var found []string
	for rows.Next() {
		var group, name, flag string
		var position, id, cents int64
		if err := rows.Scan(&group, &position, &id, &name, &cents, &flag); err != nil {
			env.t.Fatal(err)
		}
		found = append(found, strings.Join([]string{group, itoa(position), itoa(id), name, itoa(cents), flag}, " "))
	}
	return found
}

func (env *budgetEnv) closedRows() []string {
	env.t.Helper()
	return columnValues(env.t, env.scratch.Inspect(env.t), "SELECT month || ' ' || closed_on FROM closed_months ORDER BY month")
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestClosingAMonthWithoutAVersionOfItsOwnCopiesTheVersionInForceRowForRow(t *testing.T) {
	env := newBudgetEnv(t)
	before := env.scratch.Canonical(t)
	budgets := map[brday.Month]authored.Budget{}
	for _, month := range []brday.Month{"2026-01", "2026-02", "2026-03", "2026-04"} {
		budgets[month] = env.budget(month)
	}
	change, err := env.closeOn("2026-03", "2026-03-20")
	if err != nil {
		t.Fatal(err)
	}
	if got := env.closedRows(); !slices.Equal(got, []string{"2026-03 2026-03-20"}) {
		t.Errorf("closed_months = %v", got)
	}
	if got := env.versions(); got != "2026-02 2026-03" {
		t.Errorf("versions = %q, want 2026-02 2026-03", got)
	}
	copied, original := env.versionRows("2026-03"), env.versionRows("2026-02")
	if len(original) == 0 || !slices.Equal(copied, original) || !slices.ContainsFunc(copied, func(row string) bool { return strings.HasSuffix(row, "Não garantida") }) {
		t.Errorf("the closed month's lines are\n%v\nwant the flagged lines of the version in force\n%v", copied, original)
	}
	for month, want := range budgets {
		if got := env.budget(month); !slices.Equal(got.Lines, want.Lines) {
			t.Errorf("%s: the close moved the budget to %+v", month, got.Lines)
		}
	}
	for _, effect := range env.scratch.EffectsOf(t, int64(change)) {
		if effect.Table != "closed_months" && effect.Table != "budget_versions" && effect.Table != "budget_version_lines" {
			t.Errorf("the close has an effect on %s: %+v", effect.Table, effect)
		}
	}
	if err := env.undo(change); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if after := env.scratch.Canonical(t); after != before {
		t.Errorf("the undo left another state:\n%s", after)
	}
}

func TestClosingAMonthWithAVersionOfItsOwnWritesOnlyTheClosedRow(t *testing.T) {
	env := newBudgetEnv(t)
	env.save("2026-03", env.withHealth("2026-03", 90_000))
	own := env.versionRows("2026-03")
	change, err := env.closeOn("2026-03", "2026-03-20")
	if err != nil {
		t.Fatal(err)
	}
	effects := env.scratch.EffectsOf(t, int64(change))
	if len(effects) != 1 || effects[0].Table != "closed_months" {
		t.Errorf("the close's effects are %+v, want the closed row alone", effects)
	}
	if got := env.versionRows("2026-03"); !slices.Equal(got, own) {
		t.Errorf("the month's own version became %v", got)
	}
}

func TestClosingAMonthBeforeTheFirstVersionOrWithNoBudgetCopiesWhatItHad(t *testing.T) {
	first := newBudgetEnv(t)
	if _, err := first.closeOn("2026-01", "2026-03-20"); err != nil {
		t.Fatal(err)
	}
	if got, want := first.versionRows("2026-01"), first.versionRows("2026-02"); first.versions() != "2026-01 2026-02" || !slices.Equal(got, want) {
		t.Errorf("before the first version the close wrote versions %q with %v, want a copy of the first", first.versions(), got)
	}
	nothing := newBudgetEnvWithNothing(t)
	change, err := nothing.closeOn("2026-02", "2026-03-20")
	if err != nil {
		t.Fatal(err)
	}
	if nothing.versions() != "2026-02" || len(nothing.versionRows("2026-02")) != 0 {
		t.Errorf("with no budget the close wrote versions %q with lines %v, want one version with none", nothing.versions(), nothing.versionRows("2026-02"))
	}
	if budget := nothing.budget("2026-02"); budget.HasLines() {
		t.Errorf("the closed month has lines: %+v", budget)
	}
	if effects := nothing.scratch.EffectsOf(t, int64(change)); len(effects) != 2 {
		t.Errorf("the close's effects are %+v, want the closed row and the version", effects)
	}
}

func TestClosingAClosedMonthIsRefusedAndChangesNothing(t *testing.T) {
	env := newBudgetEnv(t)
	if _, err := env.closeOn("2026-03", "2026-03-20"); err != nil {
		t.Fatal(err)
	}
	before, journal := env.scratch.Everything(t), env.scratch.Journal(t)
	if _, err := env.closeOn("2026-03", "2026-03-21"); !errors.Is(err, authored.ErrMonthClosed) {
		t.Errorf("a second close answered %v, want ErrMonthClosed", err)
	}
	if env.scratch.Everything(t) != before || env.scratch.Journal(t) != journal {
		t.Error("a refused close changed the database or the journal")
	}
}
