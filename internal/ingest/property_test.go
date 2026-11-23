package ingest_test

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// charge is one generated purchase across the nights: listed from the night it
// appears until the one it vanishes, pending first when it posts later, under
// another id and up to three days after.
type charge struct {
	shop             string
	reais            float64
	day              time.Time
	appears, vanish  int
	pendingUntil     int
	postedAfterDays  int
	pendingID, posID string
}

const generatedNights = 4

var generatedShops = []string{"PADARIA EXEMPLO", "MERCADO EXEMPLO", "FEIRA EXEMPLO"}

func generateCharges(random *rand.Rand) []charge {
	charges := make([]charge, 6+random.IntN(5))
	for i := range charges {
		c := charge{
			shop:  generatedShops[random.IntN(len(generatedShops))],
			reais: float64(10 * (1 + random.IntN(3))),
			day:   at(2026, time.February, 26).AddDate(0, 0, random.IntN(23)),
		}
		c.appears = random.IntN(generatedNights - 1)
		c.vanish = c.appears + 1 + random.IntN(generatedNights+1-c.appears)
		if random.IntN(2) == 0 {
			c.pendingUntil = c.appears + 1 + random.IntN(generatedNights-c.appears)
		}
		c.postedAfterDays = random.IntN(4)
		c.pendingID, c.posID = fmt.Sprintf("p-%d", i), fmt.Sprintf("q-%d", i)
		charges[i] = c
	}
	return charges
}

// rowsOfNight is what the ledger lists of the charges on night n.
func rowsOfNight(charges []charge, n int) []ledgertest.Transaction {
	var rows []ledgertest.Transaction
	for _, c := range charges {
		switch {
		case n < c.appears || n >= c.vanish:
		case n < c.pendingUntil:
			rows = append(rows, pendingPurchase(c.pendingID, c.day, c.shop, c.reais))
		case c.pendingUntil > 0:
			rows = append(rows, purchase(c.posID, c.day.AddDate(0, 0, c.postedAfterDays), c.shop, c.reais))
		default:
			rows = append(rows, purchase(c.posID, c.day, c.shop, c.reais))
		}
	}
	return rows
}

func TestRunsOverGeneratedNightsEvaluateEachTransactionOnceAndRepeatToNothing(t *testing.T) {
	var total ingest.Counts
	for seed := range uint64(30) {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			random := rand.New(rand.NewPCG(seed, 0x1f2e))
			w := newWorld(t, createdOn)
			budgetNight := random.IntN(3)
			charges := generateCharges(random)
			for n := range generatedNights {
				if n == budgetNight {
					w.saveBudget("2026-03", marketBudget())
					w.giveADefaultLine(random)
				}
				w.personActs(random)
				rows := rowsOfNight(charges, n)
				w.night("complete", rows...)
				w.personDecidesBeforeTheTick(random, rows)
				total = sum(total, w.checkOneNight(n))
			}
		})
	}
	// The generator must reach every path it is meant to cover, or the
	// properties above hold over nothing.
	if total.Indexed == 0 || total.Paired == 0 || total.Unpaired == 0 || total.Applied == 0 || total.AlreadyDecided == 0 {
		t.Errorf("the generated nights left a path unvisited: %+v", total)
	}
}

// giveADefaultLine gives one shop, at random, a default line, or none.
func (w *world) giveADefaultLine(random *rand.Rand) {
	w.t.Helper()
	if random.IntN(2) == 0 {
		return
	}
	shop := "text:" + generatedShops[random.IntN(len(generatedShops))]
	line := w.line("2026-03", "Delivery")
	w.asPerson(func(recorder *authored.Recorder) error { return recorder.SetDefaultLine(w.t.Context(), shop, line) })
}

// personActs puts one indexed transaction, at random, on a line of its month,
// as the person would between two nights.
func (w *world) personActs(random *rand.Rand) {
	w.t.Helper()
	rows := w.index()
	if len(rows) == 0 || random.IntN(2) == 0 {
		return
	}
	row := rows[random.IntN(len(rows))]
	view := w.database().View()
	lines, err := view.LinesOfMonth(w.t.Context(), brday.Month(monthOf(row.Day)))
	view.Close()
	if err != nil {
		w.t.Fatal(err)
	}
	if len(lines) == 0 {
		return
	}
	line := lines[random.IntN(len(lines))].ID
	w.asPerson(func(recorder *authored.Recorder) error {
		return recorder.SettleLine(w.t.Context(), row.Key, line, authored.AuthorPerson)
	})
}

// personDecidesBeforeTheTick puts one row of the night the index has not met
// on a line, at random, as a write of the person between the collector's run
// and the next tick would, indexing it.
func (w *world) personDecidesBeforeTheTick(random *rand.Rand, rows []ledgertest.Transaction) {
	w.t.Helper()
	if len(rows) == 0 || random.IntN(2) == 0 {
		return
	}
	row := rows[random.IntN(len(rows))]
	if _, met := w.key(row.ID); met {
		return
	}
	day := brday.DayOf(row.Date)
	view := w.database().View()
	lines, err := view.LinesOfMonth(w.t.Context(), brday.Month(monthOf(day)))
	view.Close()
	if err != nil {
		w.t.Fatal(err)
	}
	if len(lines) == 0 {
		return
	}
	seen := authored.SeenTransaction{
		ProviderID: row.ID, AccountID: row.AccountID, Day: day, Cents: -100, EstablishmentKey: "text:" + row.Description,
		BankPending: row.Status == "PENDING",
	}
	w.asPerson(func(recorder *authored.Recorder) error {
		key, err := recorder.IndexTransaction(w.t.Context(), seen)
		if err != nil {
			return err
		}
		return recorder.SettleLine(w.t.Context(), key, lines[0].ID, authored.AuthorPerson)
	})
}

func (w *world) evaluatedKeys() map[authored.TxKey]authored.IndexedTransaction {
	w.t.Helper()
	evaluated := map[authored.TxKey]authored.IndexedTransaction{}
	for _, row := range w.index() {
		if row.Evaluated {
			evaluated[row.Key] = row
		}
	}
	return evaluated
}

func (w *world) checkOneNight(n int) ingest.Counts {
	w.t.Helper()
	before := w.evaluatedKeys()
	counts := w.run(ingest.TriggerTick)
	if counts.Outcomes() != counts.Indexed {
		w.t.Errorf("night %d: %d evaluated and %d outcomes in %+v", n, counts.Indexed, counts.Outcomes(), counts)
	}
	after := w.evaluatedKeys()
	for key := range before {
		if _, still := after[key]; !still {
			w.t.Errorf("night %d: key %d is no longer evaluated", n, key)
		}
	}
	if added := len(after) - len(before); added != counts.Indexed {
		w.t.Errorf("night %d: %d keys became evaluated and the run counts %d", n, added, counts.Indexed)
	}
	for key, row := range after {
		if _, old := before[key]; !old && monthOf(row.Day) < "2026-03" {
			w.t.Errorf("night %d: key %d of %s was evaluated before the tracking start month", n, key, row.Day)
		}
	}
	w.wantOneRuleStepAtMost(n)

	state := w.everything()
	wantCounts(w.t, fmt.Sprintf("night %d, the command right after", n), w.run(ingest.TriggerCommand), ingest.Counts{})
	if w.everything() != state {
		w.t.Errorf("night %d: a second run over the same snapshot changed something", n)
	}
	return counts
}

func (w *world) wantOneRuleStepAtMost(n int) {
	w.t.Helper()
	var keys []authored.TxKey
	for _, row := range w.index() {
		keys = append(keys, row.Key)
	}
	view := w.database().View()
	defer view.Close()
	kept, err := view.Transactions(w.t.Context(), keys)
	if err != nil {
		w.t.Fatal(err)
	}
	for key, transaction := range kept {
		steps := 0
		for _, step := range transaction.History {
			if strings.HasPrefix(step.Text, ingest.RuleStepPrefix) {
				steps++
			}
		}
		if steps > 1 {
			w.t.Errorf("night %d: key %d has %d steps of the rule", n, key, steps)
		}
	}
}
