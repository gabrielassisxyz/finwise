package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/classrules"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// The pending example, on the limit's ledger: P1 in 2026-03; P2 in 2026-01,
// before the first version of the budget, which therefore holds for it; P3 in
// 2026-03, a bank-pending row and the posted row that replaced it under one
// key; P4 in 2026-03, which the ledger lists only when asked to; and P5 in
// 2026-03, whose review was settled. Everything is invented.
const (
	pendingP1       = "tx-pending-p1"
	pendingP2       = "tx-pending-p2"
	pendingP3       = "tx-pending-p3"
	pendingP3Posted = "tx-pending-p3-posted"
	pendingP4       = "tx-pending-p4"
	pendingP5       = "tx-pending-p5"
)

const (
	fiveEmptyGroups = `[{"key":"rec","name":"Receita","lines":[]},{"key":"fix","name":"Fixos e essenciais","lines":[]},` +
		`{"key":"var","name":"Variáveis","lines":[]},{"key":"prov","name":"Provisões","lines":[]},{"key":"res","name":"Reservas de desejo","lines":[]}]`
	oneVariableLine = `[{"key":"var","name":"Variáveis","lines":[{"name":"Mercado","amount":1000}]}]`
)

// pendingLedger is the limit's ledger with P1 to P5, P4 only when listed.
func pendingLedger(t *testing.T, listsP4 bool) *ledgertest.Fixture {
	t.Helper()
	bankPending := limitPurchase(pendingP3, utc(2026, time.March, 17, 15, 0), 64)
	bankPending.Status = "PENDING"
	rows := append(slices.Clone(marchPurchases),
		limitPurchase(pendingP1, utc(2026, time.March, 18, 15, 0), 45),
		limitPurchase(pendingP2, utc(2026, time.January, 15, 15, 0), 120),
		bankPending, limitPurchase(pendingP3Posted, utc(2026, time.March, 18, 16, 0), 64),
		limitPurchase(pendingP5, utc(2026, time.March, 11, 15, 0), 33),
	)
	if listsP4 {
		rows = append(rows, limitPurchase(pendingP4, utc(2026, time.March, 19, 15, 0), 21))
	}
	return newLimitLedger(t, rows...)
}

// pendingExample saves a version from 2026-02 with one fixed line before the
// limit's from 2026-03, and leaves the five reviews as the ingest would: P3's
// posted row takes the bank-pending one's key, P4 is indexed from a night that
// listed it, and P5 is answered.
func pendingExample(t *testing.T) *authoredtest.Scratch {
	t.Helper()
	scratch := authoredtest.Open(t, fixtureClock)
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-02", authored.Document{
		authoredtest.Group("fix", authoredtest.Line("Aluguel", 250_000)),
	})
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-03", limitBudget())
	snapshot := viewOf(t, pendingLedger(t, false).Path)
	lines := linesOfMonthByName(t, scratch.Database, "2026-03")
	authorOver(t, scratch, snapshot, lines, fixtureNow, "ingest.run", func(a authoring) error {
		review := func(key authored.TxKey) error {
			return a.writes.Recorder.SetPendingReview(a.ctx, key, string(classrules.ReasonNewEstablishment), nil, nil)
		}
		keys := map[string]authored.TxKey{}
		for _, id := range []string{pendingP1, pendingP2, pendingP3, pendingP5} {
			key, err := a.key(id)
			if err != nil {
				return err
			}
			keys[id] = key
			if err := review(key); err != nil {
				return err
			}
		}
		row, found, err := findListed(a.ctx, a.writes.Ledger, pendingP3Posted)
		if err != nil || !found {
			return errors.Join(err, errors.New("the posted row of P3 is not listed"))
		}
		posted := authored.SeenTransaction{ProviderID: row.listed.ID, AccountID: row.listed.AccountID, Day: row.listed.Day,
			Cents: row.listed.Amount, EstablishmentKey: row.listed.EstablishmentKey, Replaces: keys[pendingP3]}
		if key, err := a.writes.Recorder.IndexTransaction(a.ctx, posted); err != nil || key != keys[pendingP3] {
			return errors.Join(err, errors.New("the posted row of P3 was given a key of its own"))
		}
		night, found, err := findListed(a.ctx, viewOf(t, pendingLedger(t, true).Path), pendingP4)
		if err != nil || !found {
			return errors.Join(err, errors.New("P4 is not listed on the night that lists it"))
		}
		key, err := indexListed(a.ctx, a.writes.Recorder, night.listed)
		if err != nil {
			return err
		}
		if err := review(key); err != nil {
			return err
		}
		return a.writes.Recorder.SettleLine(a.ctx, keys[pendingP5], lines["Mercado"], authored.AuthorPerson)
	})
	return scratch
}

// pendingCounts is the frame's count, the home's for each month asked and the
// length of the queue.
type pendingCounts struct {
	frame int
	home  map[string]int
	queue int
}

func (env *writeEnv) pendingCounts() pendingCounts {
	env.t.Helper()
	counts := pendingCounts{home: map[string]int{}}
	read := func(method, arguments string) map[string]json.RawMessage {
		response := env.post(method, arguments)
		var answer map[string]json.RawMessage
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &answer) != nil {
			env.t.Fatalf("%s %s answered %d %s", method, arguments, response.Code, response.Body)
		}
		return answer
	}
	count := func(raw json.RawMessage) int {
		var n int
		if err := json.Unmarshal(raw, &n); err != nil {
			env.t.Fatal(err)
		}
		return n
	}
	counts.frame = count(read("shell.overview", `[]`)["pendingCount"])
	for _, month := range []string{"2026-01", "2026-03", "2026-05"} {
		counts.home[month] = count(read("home.summary", `["`+month+`"]`)["pendingCount"])
	}
	counts.queue = len(env.queueOf("2026-03").Items)
	return counts
}

// expectCount is every count at want: the frame's, the home's in every month
// asked, and the length of the queue, which the count is defined as.
func expectCount(t *testing.T, label string, got pendingCounts, want int) {
	t.Helper()
	if got.frame != want || got.queue != want {
		t.Errorf("%s: the frame counts %d and the queue holds %d, want %d", label, got.frame, got.queue, want)
	}
	for month, n := range got.home {
		if n != want {
			t.Errorf("%s: the home of %s counts %d, want %d", label, month, n, want)
		}
	}
}

func TestThePendingCountIsTheQueueOfTransactionsListedNowInAMonthWithALine(t *testing.T) {
	scratch := pendingExample(t)
	unlisted := newReviewEnv(t, pendingLedger(t, false).Path, scratch, fixtureNow)
	expectCount(t, "P1, P2 and P3 once", unlisted.pendingCounts(), 3)

	listed := newReviewEnv(t, pendingLedger(t, true).Path, scratch, fixtureNow)
	expectCount(t, "with P4 listed again", listed.pendingCounts(), 4)

	expectRecorded(t, listed, "budget.save", `["2026-03",`+fiveEmptyGroups+`]`)
	expectCount(t, "with March left with no line", listed.pendingCounts(), 1)

	expectRecorded(t, listed, "budget.save", `["2026-03",`+oneVariableLine+`]`)
	expectCount(t, "with March given a line again", listed.pendingCounts(), 4)
}

func expectRecorded(t *testing.T, env *writeEnv, method, arguments string) {
	t.Helper()
	if response := env.post(method, arguments); response.Code != http.StatusOK {
		t.Fatalf("%s %s answered %d %s", method, arguments, response.Code, response.Body)
	}
}

// A server with --db runs the ingest after a budget save. Before the save the
// month the database was created in has no line, so its new purchase was
// never evaluated; the save gives it one, and the run that follows puts the
// purchase in the queue before the save is answered.
func TestTheCountRisesWhenABudgetSaveMakesANewPurchasePending(t *testing.T) {
	f := newLimitLedger(t, limitPurchase("tx-new-place", utc(2026, time.March, 12, 15, 0), 89))
	scratch := authoredtest.Open(t, fixtureClock)
	sources := ingest.Sources{Ledger: ledger.New(f.Path), Database: scratch.Database}
	if _, err := ingest.Run(t.Context(), sources, ingest.TriggerStart, fixtureNow); err != nil {
		t.Fatal(err)
	}
	server := newTestEnv(t, withLedger(f.Path, fixtureNow), withDatabase(scratch.Database), func(config *Config) {
		config.AfterBudgetSaved = func(ctx context.Context, at time.Time) {
			if _, err := ingest.Run(ctx, sources, ingest.TriggerBudgetChange, at); err != nil {
				t.Errorf("the ingest after the save: %v", err)
			}
		}
	})
	env := &writeEnv{testEnv: server, scratch: scratch, cookie: server.unlock()}
	if got := env.pendingCounts(); got.frame != 0 {
		t.Fatalf("before any budget the frame counts %d pending", got.frame)
	}
	expectRecorded(t, env, "budget.save", `["2026-03",`+oneVariableLine+`]`)
	expectCount(t, "after the save", env.pendingCounts(), 1)
}
