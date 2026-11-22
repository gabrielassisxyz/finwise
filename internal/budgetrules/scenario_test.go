package budgetrules

import (
	"reflect"
	"slices"
	"testing"
)

func expectFigures(t *testing.T, label string, got InvestmentFigures, investment int64, rate *int64) {
	t.Helper()
	if int64(got.Investment) != investment || !reflect.DeepEqual(got.Rate, rate) {
		t.Errorf("%s: investment %d and rate %v, want %d and %v", label, got.Investment, got.Rate, investment, rate)
	}
}

func anyChanged(changed [][]bool) bool {
	return slices.ContainsFunc(changed, func(group []bool) bool { return slices.Contains(group, true) })
}

// The evaluateScenario cases of the group "scenarios" of
// web/src/api/fake/budget.test.ts, in cents.
func TestAScenarioIsWeighedAgainstTheMonthsBudget(t *testing.T) {
	impact := ScenarioImpact(november(), atAmount(november(), "Renda extra", 0))
	expectFigures(t, "current", impact.Current, 380000, whole(32))
	expectFigures(t, "scenario", impact.Scenario, 230000, whole(22))
	want := [][]bool{{false, true, false}, make([]bool, 4), make([]bool, 9), make([]bool, 3), make([]bool, 2)}
	if impact.Difference != -150000 || !reflect.DeepEqual(impact.Changed, want) {
		t.Errorf("difference %d and changed %v, want -150000 and %v", impact.Difference, impact.Changed, want)
	}

	untouched := ScenarioImpact(november(), november())
	if untouched.Difference != 0 || anyChanged(untouched.Changed) {
		t.Errorf("an untouched copy: difference %d, changed %v", untouched.Difference, untouched.Changed)
	}
}

func TestAScenarioLineIsComparedWithTheLineOfItsIdWhereverItSits(t *testing.T) {
	scenario := edited(november(), "Mercado", func(line *PlannedLine) { line.Name = "Feira" })
	scenario = edited(scenario, "Saúde", func(line *PlannedLine) { line.Amount = 90000 })
	scenario = edited(scenario, "Pets", func(line *PlannedLine) { line.ID = LineRef{} })
	scenario = edited(scenario, "Casa", func(line *PlannedLine) { line.ID = OnLine(unknownLine) })
	scenario[4].Lines = append(scenario[4].Lines, scenario[1].Lines[3])
	scenario[1].Lines = scenario[1].Lines[:3]
	impact := ScenarioImpact(november(), scenario)
	want := [][]bool{
		{false, false, false},
		{false, true, true},
		{false, false, false, false, true, false, false, false, false},
		{false, false, false},
		{false, false, false},
	}
	if !reflect.DeepEqual(impact.Changed, want) {
		t.Errorf("changed %v, want %v: a renamed or moved line at its amount is unchanged; a new id, an unknown one or "+
			"another amount is changed", impact.Changed, want)
	}

	inserted := november()
	inserted[2].Lines = append([]PlannedLine{{Name: "Pets", Amount: 20000}}, inserted[2].Lines...)
	impact = ScenarioImpact(november(), inserted)
	want = [][]bool{make([]bool, 3), make([]bool, 4), append([]bool{true}, make([]bool, 9)...), make([]bool, 3), make([]bool, 2)}
	if !reflect.DeepEqual(impact.Changed, want) {
		t.Errorf("a line inserted at the head of var: changed %v, want %v", impact.Changed, want)
	}
}

func TestChangedHasOneEntryPerGroupAndLineAsSent(t *testing.T) {
	plan := november()
	scenario := Plan{plan[2], plan[2], {Key: GroupIncome, Lines: []PlannedLine{}}}
	impact := ScenarioImpact(november(), scenario)
	if len(impact.Changed) != 3 || len(impact.Changed[0]) != 9 || len(impact.Changed[1]) != 9 || impact.Changed[2] == nil ||
		len(impact.Changed[2]) != 0 || anyChanged(impact.Changed) {
		t.Errorf("changed %v, want two groups of nine unchanged lines and one empty group", impact.Changed)
	}
	expectFigures(t, "a scenario with no income", impact.Scenario, -576000, nil)
}

func TestAMonthWithNoBudgetHasNothingToInvest(t *testing.T) {
	impact := ScenarioImpact(Plan{}, november())
	expectFigures(t, "current", impact.Current, 0, nil)
	if impact.Difference != 380000 || !anyChanged(impact.Changed) {
		t.Errorf("difference %d, changed %v; want 380000 and every line changed", impact.Difference, impact.Changed)
	}
}

func TestTheScenarioIsWeighedAgainstTheVersionOfTheMonthItIsGiven(t *testing.T) {
	october := november()
	raised := atAmount(october, "Saúde", 90000)
	expectFigures(t, "against October", ScenarioImpact(october, october).Current, 380000, whole(32))
	expectFigures(t, "against November", ScenarioImpact(raised, october).Current, 340000, whole(28))
}
