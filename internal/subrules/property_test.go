package subrules

import (
	"flag"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

var seedFlag = flag.Uint64("seed", 0, "seed of the generated rows; zero draws a new one")

// generatedRows are a history of up to twelve rows over a few years, some on
// one day, some foreign with a tax.
func generatedRows(random *rand.Rand) []Row {
	start := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	days := make([]brday.Day, 1+random.IntN(8))
	for index := range days {
		days[index] = brday.Day(start.AddDate(0, 0, random.IntN(2_500)).Format(dayLayout))
	}
	rows := make([]Row, 1+random.IntN(12))
	for index := range rows {
		row := Row{Day: days[random.IntN(len(days))], Cents: money.Cents(1 + random.IntN(500_000))}
		if random.IntN(3) == 0 {
			row.Foreign, row.Tax = &Foreign{Amount: money.Cents(1 + random.IntN(10_000)), Currency: "USD"}, money.Cents(random.IntN(400))
		}
		rows[index] = row
	}
	return rows
}

func TestGeneratedHistoriesAnswerTheSameInAnyOrderAndKeepEveryCent(t *testing.T) {
	seed := *seedFlag
	if seed == 0 {
		seed = rand.Uint64()
	}
	random := rand.New(rand.NewPCG(seed, 0))
	for run := range 2_000 {
		rows := generatedRows(random)
		subscription := Subscription{Rows: rows, CreatedDay: "2019-01-01", CreatedCents: 1, StatusDay: "2022-06-01",
			Cancelling: random.IntN(2) == 0, Today: "2024-01-01"}
		derivation, err := Derive(subscription)
		if err != nil {
			t.Fatalf("seed %d run %d: %v", seed, run, err)
		}
		shuffled := slices.Clone(rows)
		random.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
		subscription.Rows = shuffled
		if again, err := Derive(subscription); err != nil || !reflect.DeepEqual(again, derivation) {
			t.Fatalf("seed %d run %d: the rows in another order answer %+v, not %+v (%v)", seed, run, again, derivation, err)
		}
		checkCentsKept(t, seed, run, rows)
		checkCycleAndNextCharge(t, seed, run, rows, derivation)
	}
}

func checkCentsKept(t *testing.T, seed uint64, run int, rows []Row) {
	t.Helper()
	var inRows, inCharges money.Cents
	for _, row := range rows {
		inRows += row.Cents
	}
	charges := ChargesOf(rows)
	for index, charge := range charges {
		inCharges += charge.Cents
		if index > 0 && charges[index-1].Day >= charge.Day {
			t.Fatalf("seed %d run %d: two charges are not one per day, oldest first: %+v", seed, run, charges)
		}
	}
	if inRows != inCharges {
		t.Fatalf("seed %d run %d: the rows hold %d cents and the charges %d", seed, run, inRows, inCharges)
	}
}

func checkCycleAndNextCharge(t *testing.T, seed uint64, run int, rows []Row, derivation Derivation) {
	t.Helper()
	if !slices.Contains(Cycles(), derivation.Cycle) {
		t.Fatalf("seed %d run %d: cycle %d is not one of the five", seed, run, derivation.Cycle)
	}
	charges := ChargesOf(rows)
	if latest := charges[len(charges)-1]; derivation.NextCharge <= latest.Day {
		t.Fatalf("seed %d run %d: the next charge %s is not after the latest %s", seed, run, derivation.NextCharge, latest.Day)
	}
}
