package budgetrules

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

func spentOf(cents money.Cents) *money.Cents { return &cents }

func labelsOf(guides []WeekGuide) []string {
	labels := []string{}
	for _, guide := range guides {
		labels = append(labels, guide.Label)
	}
	return labels
}

func TestTheWeeksOfAMonthAreTheDesignsBlocksLabelledWithAnEnDash(t *testing.T) {
	cases := map[brday.Month][]string{
		"2026-02": {"01–07", "08–14", "15–21", "22–28"},
		"2026-04": {"01–07", "08–14", "15–21", "22–28", "29–30"},
		"2026-03": {"01–07", "08–14", "15–21", "22–28", "29–31"},
		"2028-02": {"01–07", "08–14", "15–21", "22–28", "29"},
	}
	for month, want := range cases {
		guides, err := WeekGuides(month, "2026-03-20", 100_000, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := labelsOf(guides); !slices.Equal(got, want) {
			t.Errorf("%s: labels %q, want %q", month, got, want)
		}
		for _, label := range labelsOf(guides) {
			if strings.ContainsRune(label, '-') || (len(label) > 2 && !strings.ContainsRune(label, '\u2013')) {
				t.Errorf("%s: the label %q is not joined by U+2013", month, label)
			}
		}
	}
}

type weekRow struct {
	label   string
	guide   money.Cents
	spent   *money.Cents
	current bool
}

type guideExample struct {
	name     string
	month    brday.Month
	today    brday.Day
	cap      money.Cents
	spending map[brday.Day]money.Cents
	weeks    []weekRow
	// remaining, over and fill are the weekly review's limit; week is its
	// current week as guide, spent and fill, or nil for none.
	remaining money.Cents
	over      bool
	fill      int64
	week      *[3]int64
}

// The worked examples, in cents, with today and the month as inputs. Each
// block's spending is put on its first and last days where it has two, so a
// day on a block's edge is counted in that block.
var guideExamples = []guideExample{
	{
		name: "A, a 30-day month, the current one", month: "2026-04", today: "2026-04-17", cap: 300_000,
		spending: map[brday.Day]money.Cents{
			"2026-04-01": 50_000, "2026-04-07": 30_000, "2026-04-08": 40_000, "2026-04-14": 25_000,
			"2026-04-15": 10_000, "2026-04-17": 11_000,
		},
		weeks: []weekRow{
			{"01–07", 70_000, spentOf(80_000), false}, {"08–14", 66_957, spentOf(65_000), false},
			{"15–21", 67_813, spentOf(21_000), true}, {"22–28", 104_222, nil, false}, {"29–30", 134_000, nil, false},
		},
		remaining: 134_000, fill: 55, week: &[3]int64{67_813, 21_000, 31},
	},
	{
		name: "B, a 28-day month, the current one", month: "2026-02", today: "2026-02-10", cap: 280_000,
		spending: map[brday.Day]money.Cents{"2026-02-02": 70_000, "2026-02-10": 15_000},
		weeks: []weekRow{
			{"01–07", 70_000, spentOf(70_000), false}, {"08–14", 70_000, spentOf(15_000), true},
			{"15–21", 97_500, nil, false}, {"22–28", 195_000, nil, false},
		},
		remaining: 195_000, fill: 30, week: &[3]int64{70_000, 15_000, 21},
	},
	{
		name: "C, an overspent week and then a passed cap", month: "2026-03", today: "2026-03-20", cap: 100_000,
		spending: map[brday.Day]money.Cents{"2026-03-07": 40_000, "2026-03-10": 70_000, "2026-03-16": 3_000},
		weeks: []weekRow{
			{"01–07", 22_581, spentOf(40_000), false}, {"08–14", 17_500, spentOf(70_000), false},
			{"15–21", 0, spentOf(3_000), true}, {"22–28", 0, nil, false}, {"29–31", 0, nil, false},
		},
		remaining: -13_000, over: true, fill: 100, week: &[3]int64{0, 3_000, 0},
	},
	{
		name: "D, a month with no cap and nothing spent", month: "2026-03", today: "2026-03-20", cap: 0,
		weeks: []weekRow{},
	},
	{
		name: "D, variable lines of 0.00 with 50.00 spent", month: "2026-03", today: "2026-03-20", cap: 0,
		spending: map[brday.Day]money.Cents{"2026-03-05": 5_000},
		weeks:    []weekRow{}, remaining: -5_000, over: true,
	},
	{
		name: "E, a past month", month: "2026-02", today: "2026-03-20", cap: 280_000,
		spending: map[brday.Day]money.Cents{"2026-02-01": 70_000, "2026-02-14": 90_000, "2026-02-15": 50_000, "2026-02-28": 40_000},
		weeks: []weekRow{
			{"01–07", 70_000, spentOf(70_000), false}, {"08–14", 70_000, spentOf(90_000), false},
			{"15–21", 60_000, spentOf(50_000), false}, {"22–28", 70_000, spentOf(40_000), false},
		},
		remaining: 30_000, fill: 89,
	},
	{
		name: "F, a future month", month: "2026-04", today: "2026-03-20", cap: 300_000,
		weeks: []weekRow{
			{"01–07", 70_000, nil, false}, {"08–14", 91_304, nil, false}, {"15–21", 131_250, nil, false},
			{"22–28", 233_333, nil, false}, {"29–30", 300_000, nil, false},
		},
		remaining: 300_000,
	},
	{
		name: "G, a 29-day February on its last day", month: "2028-02", today: "2028-02-29", cap: 100_000,
		weeks: []weekRow{
			{"01–07", 24_138, spentOf(0), false}, {"08–14", 31_818, spentOf(0), false}, {"15–21", 46_667, spentOf(0), false},
			{"22–28", 87_500, spentOf(0), false}, {"29", 100_000, spentOf(0), true},
		},
		remaining: 100_000, week: &[3]int64{100_000, 0, 0},
	},
}

func describeWeek(week WeekGuide) string {
	spent := "null"
	if week.Spent != nil {
		spent = fmt.Sprint(*week.Spent)
	}
	return fmt.Sprintf("%s %d/%s current %v", week.Label, week.Guide, spent, week.Current)
}

func describeRow(row weekRow) string {
	return describeWeek(WeekGuide{Label: row.label, Guide: row.guide, Spent: row.spent, Current: row.current})
}

func TestTheWorkedExamplesOfTheWeeklyGuideHoldRowByRow(t *testing.T) {
	for _, example := range guideExamples {
		guides, err := WeekGuides(example.month, example.today, example.cap, example.spending)
		if err != nil {
			t.Fatalf("%s: %v", example.name, err)
		}
		if len(guides) != len(example.weeks) || guides == nil {
			t.Errorf("%s: %d weeks %v, want %d", example.name, len(guides), guides, len(example.weeks))
			continue
		}
		var current *WeekGuide
		for i, row := range example.weeks {
			if got, want := describeWeek(guides[i]), describeRow(row); got != want {
				t.Errorf("%s, week %d: %s, want %s", example.name, i, got, want)
			}
			if guides[i].Current {
				current = &guides[i]
			}
		}
		var spent money.Cents
		for _, cents := range example.spending {
			spent += cents
		}
		limit := MonthLimit{Cap: example.cap, Spent: spent}
		if limit.Remaining() != example.remaining || limit.Over() != example.over || limit.Fill() != example.fill {
			t.Errorf("%s: remaining %d over %v fill %d, want %d %v %d", example.name,
				limit.Remaining(), limit.Over(), limit.Fill(), example.remaining, example.over, example.fill)
		}
		switch {
		case example.week == nil && current != nil:
			t.Errorf("%s: the week %s is current, want none", example.name, describeWeek(*current))
		case example.week != nil && current == nil:
			t.Errorf("%s: no week is current, want %v", example.name, *example.week)
		case example.week != nil:
			if got := [3]int64{int64(current.Guide), int64(*current.Spent), current.Fill()}; got != *example.week {
				t.Errorf("%s: the current week is %v, want %v", example.name, got, *example.week)
			}
		}
	}
}

func TestTheGuideRoundsHalfUpToTheCent(t *testing.T) {
	cases := []struct {
		base            money.Cents
		days, remaining int
		want            money.Cents
	}{
		{155_000, 7, 16, 67_813},
		{220_000, 7, 23, 66_957},
		{134_000, 7, 9, 104_222},
		{1, 7, 14, 1},
		{1, 1, 3, 0},
	}
	for _, c := range cases {
		if got := roundedShareOfDays(c.base, c.days, c.remaining); got != c.want {
			t.Errorf("%d × %d / %d = %d, want %d", c.base, c.days, c.remaining, got, c.want)
		}
	}
}

func TestAMonthWithNoCapHasNoWeekWhateverWasSpent(t *testing.T) {
	spending := map[brday.Day]money.Cents{"2026-03-02": 90_000, "2026-03-25": -10_000}
	for _, month := range []brday.Month{"2026-02", "2026-03", "2026-04"} {
		guides, err := WeekGuides(month, "2026-03-20", 0, spending)
		if err != nil || guides == nil || len(guides) != 0 {
			t.Errorf("%s with no cap: %v, %v; want no week", month, guides, err)
		}
	}
	if _, err := WeekGuides("March", "2026-03-20", 100, nil); err == nil {
		t.Error("a month that is not one was answered")
	}
}

// Only the variable lines' figures count, and a day's figure is what the
// month's own sum would give for that day.
func TestTheDailySpendingIsTheVariableLimitOfEachDay(t *testing.T) {
	plan := Plan{
		{Key: GroupFixed, Lines: []PlannedLine{{ID: OnLine(2), Name: "Aluguel", Amount: 250_000}}},
		{Key: GroupVariable, Lines: []PlannedLine{{ID: OnLine(3), Name: "Mercado", Amount: 90_000}, {ID: OnLine(4), Name: "Delivery", Amount: 20_000}}},
	}
	on := func(line LineID, effect Effect, cents money.Cents) Outcome {
		return Outcome{Lines: []LineAmount{{Line: line, Effect: effect, Amount: cents}}}
	}
	dated := []DatedOutcome{
		{"2026-03-02", on(3, Adds, 10_000)}, {"2026-03-02", on(4, Adds, 2_000)}, {"2026-03-02", on(2, Adds, 250_000)},
		{"2026-03-05", on(3, TakesOff, 4_000)}, {"2026-03-09", on(1, Adds, 900)},
	}
	got := VariableSpendingByDay(plan, dated)
	want := map[brday.Day]money.Cents{"2026-03-02": 12_000, "2026-03-05": -4_000, "2026-03-09": 0}
	if len(got) != len(want) {
		t.Errorf("spending by day %v, want %v", got, want)
	}
	for day, cents := range want {
		if got[day] != cents {
			t.Errorf("%s: %d, want %d", day, got[day], cents)
		}
	}
	outcomes := make([]Outcome, 0, len(dated))
	for _, one := range dated {
		outcomes = append(outcomes, one.Outcome)
	}
	if limit := VariableLimit(plan, LineFigures(outcomes)); limit != (MonthLimit{Cap: 110_000, Spent: 8_000}) {
		t.Errorf("the month's limit is %+v, want cap 110000 spent 8000", limit)
	}
}

const generatedGuideMonths = 3000

type generatedGuide struct {
	month    brday.Month
	today    brday.Day
	cap      money.Cents
	spending map[brday.Day]money.Cents
	negative bool
}

// generateGuide is a month of 28 to 31 days, today in it, before it or after
// it, a cap that is sometimes zero and a spending on some days that a refund
// sometimes turns negative.
func generateGuide(rng *rand.Rand) generatedGuide {
	month := brday.Month(fmt.Sprintf("%04d-%02d", 2020+rng.IntN(20), 1+rng.IntN(12)))
	period, _ := brday.MonthPeriod(month)
	days := dayNumber(period.Last)
	todayMonth := month
	switch rng.IntN(4) {
	case 0:
		todayMonth = shiftMonth(month, -1-rng.IntN(14))
	case 1:
		todayMonth = shiftMonth(month, 1+rng.IntN(14))
	}
	g := generatedGuide{
		month: month, today: brday.Day(fmt.Sprintf("%s-%02d", todayMonth, 1+rng.IntN(28))),
		spending: map[brday.Day]money.Cents{},
	}
	if rng.IntN(6) > 0 {
		g.cap = money.Cents(rng.Int64N(100_000_000))
	}
	if todayMonth == month {
		g.today = brday.Day(fmt.Sprintf("%s-%02d", month, 1+rng.IntN(days)))
	}
	for day := 1; day <= days; day++ {
		if rng.IntN(3) > 0 {
			continue
		}
		cents := money.Cents(rng.Int64N(int64(g.cap)/4 + 1_000))
		if rng.IntN(8) == 0 {
			cents = -cents
			g.negative = g.negative || cents < 0
		}
		g.spending[brday.Day(fmt.Sprintf("%s-%02d", month, day))] = cents
	}
	return g
}

func shiftMonth(month brday.Month, by int) brday.Month {
	var year, number int
	fmt.Sscanf(string(month), "%d-%d", &year, &number)
	index := year*12 + number - 1 + by
	return brday.Month(fmt.Sprintf("%04d-%02d", index/12, index%12+1))
}

// guideProblems checks the guides against the blocks of brday.Weeks and sums
// made here from the generated spending.
func guideProblems(g generatedGuide, guides []WeekGuide) []string {
	weeks, _ := brday.Weeks(g.month)
	if g.cap == 0 {
		if len(guides) != 0 {
			return []string{fmt.Sprintf("%d weeks for a cap of zero", len(guides))}
		}
		return nil
	}
	if len(guides) != len(weeks) {
		return []string{fmt.Sprintf("%d weeks, the month has %d", len(guides), len(weeks))}
	}
	current := brday.Month(g.today[:7])
	var problems []string
	currents := 0
	for i, week := range weeks {
		var before, within money.Cents
		for day, cents := range g.spending {
			switch {
			case day < week.First:
				before += cents
			case day <= week.Last:
				within += cents
			}
		}
		base := max(0, g.cap-before)
		guide := guides[i]
		if guide.Guide < 0 || guide.Guide > base || (!g.negative && guide.Guide > g.cap) {
			problems = append(problems, fmt.Sprintf("%s: guide %d, base %d, cap %d", guide.Label, guide.Guide, base, g.cap))
		}
		if i == len(weeks)-1 && guide.Guide != base {
			problems = append(problems, fmt.Sprintf("%s, the last week: guide %d, want everything left, %d", guide.Label, guide.Guide, base))
		}
		begun := g.month < current || (g.month == current && week.First <= g.today)
		if begun != (guide.Spent != nil) || (begun && *guide.Spent != within) {
			problems = append(problems, fmt.Sprintf("%s: begun %v and spent %s, its days spent %d", guide.Label, begun, describeWeek(guide), within))
		}
		if guide.Current {
			currents++
		}
	}
	if currents > 1 || (currents > 0 && g.month != current) {
		problems = append(problems, fmt.Sprintf("%d weeks current in %s with today %s", currents, g.month, g.today))
	}
	return problems
}

func TestEveryGeneratedMonthsGuidesStayWithinWhatIsLeft(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 2026))
	for range generatedGuideMonths {
		g := generateGuide(rng)
		guides, err := WeekGuides(g.month, g.today, g.cap, g.spending)
		if err != nil {
			t.Fatalf("%s: %v", g.month, err)
		}
		if problems := guideProblems(g, guides); len(problems) > 0 {
			t.Fatalf("%s, today %s, cap %d, spending %v:\n%s", g.month, g.today, g.cap, g.spending, strings.Join(problems, "\n"))
		}
	}
}
