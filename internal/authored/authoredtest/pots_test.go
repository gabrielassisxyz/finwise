package authoredtest_test

import (
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

const (
	reservePot    = "reserve:acc-pots/pot-1"
	investmentPot = "investment:inv-1"
)

// budgetLineIDs saves the invented budget and answers the ids it minted, by name.
func budgetLineIDs(t *testing.T, database *authored.Database, at time.Time) map[string]authored.LineID {
	t.Helper()
	authoredtest.SaveBudget(t, database, at, "2026-03", authoredtest.InventedBudget())
	view := database.View()
	defer view.Close()
	budget, err := view.BudgetOf(t.Context(), "2026-03")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]authored.LineID{}
	for _, line := range budget.Lines {
		ids[line.Name] = line.ID
	}
	return ids
}

func TestThePotBuildersSetABucketAndALinkThatTheViewReadsBack(t *testing.T) {
	at := time.Date(2026, time.March, 20, 15, 0, 0, 0, time.UTC)
	databases := map[string]*authored.Database{
		"a created database": authoredtest.Fresh(t, func() time.Time { return at }),
		"a scratch database": authoredtest.Open(t, func() time.Time { return at }).Database,
	}
	for name, database := range databases {
		t.Run(name, func(t *testing.T) {
			ids := budgetLineIDs(t, database, at)
			authoredtest.SetPotBucket(t, database, at, reservePot, authored.BucketInvested)
			authoredtest.ToggleLink(t, database, at, ids["Remédios"], reservePot)
			authoredtest.ToggleLink(t, database, at, ids["Viagens"], reservePot)

			view := database.View()
			defer view.Close()
			ctx := t.Context()
			if bucket, found, err := view.PotBucket(ctx, reservePot); err != nil || !found || bucket != authored.BucketInvested {
				t.Errorf("bucket %q (%v, %v), want Investido", bucket, found, err)
			}
			if _, found, err := view.PotBucket(ctx, investmentPot); err != nil || found {
				t.Errorf("a pot the builders never named has a bucket (%v, %v)", found, err)
			}
			if pot, found, err := view.PotOfLine(ctx, ids["Remédios"]); err != nil || !found || pot != reservePot {
				t.Errorf("the line is on %q (%v, %v), want %s", pot, found, err, reservePot)
			}
			want := []authored.LineID{ids["Remédios"], ids["Viagens"]}
			slices.Sort(want)
			if lines, err := view.LinesOfPot(ctx, reservePot); err != nil || !slices.Equal(lines, want) {
				t.Errorf("the pot has the lines %v (%v), want %v", lines, err, want)
			}
		})
	}
}

func TestTheLinkBuilderTogglesAndEachBuildIsOneChangeThatCanBeUndone(t *testing.T) {
	at := time.Date(2026, time.March, 20, 15, 0, 0, 0, time.UTC)
	database := authoredtest.Fresh(t, func() time.Time { return at })
	ids := budgetLineIDs(t, database, at)
	line := ids["Remédios"]

	linked := authoredtest.ToggleLink(t, database, at, line, reservePot)
	moved := authoredtest.ToggleLink(t, database, at.Add(time.Hour), line, investmentPot)
	if linked == moved {
		t.Fatalf("two builds are the one change %d", linked)
	}
	read := func() (string, bool) {
		view := database.View()
		defer view.Close()
		pot, found, err := view.PotOfLine(t.Context(), line)
		if err != nil {
			t.Fatal(err)
		}
		return pot, found
	}
	if pot, found := read(); !found || pot != investmentPot {
		t.Fatalf("the line is on %q (%v), want the second pot: a second build moves it", pot, found)
	}
	if err := database.Undo(t.Context(), moved, at.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if pot, found := read(); !found || pot != reservePot {
		t.Errorf("after undoing the move the line is on %q (%v), want the first pot", pot, found)
	}
	if err := database.Undo(t.Context(), linked, at.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if pot, found := read(); found {
		t.Errorf("after undoing the link the line is on %q", pot)
	}
}
