package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// The review's warnings over ledgers built for each test: an invented card whose
// charges are added one by one, and the subscriptions kept over them. A router
// is built for each day a test reads on, over the same two files, so the clock
// of one read never leaks into another.

type warningSeen struct {
	ID             string          `json:"id"`
	Text           string          `json:"text"`
	SubscriptionID string          `json:"subscriptionId"`
	Actions        json.RawMessage `json:"actions"`
}

type warningBench struct {
	t       *testing.T
	fixture *ledgertest.Fixture
	scratch *authoredtest.Scratch
	charged int
}

func newWarningBench(t *testing.T) *warningBench {
	t.Helper()
	return &warningBench{t: t, fixture: newSubscriptionsLedger(t), scratch: authoredtest.Open(t, fixtureClock)}
}

func noonOf(t testing.TB, day brday.Day) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.DateOnly, string(day))
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Add(15 * time.Hour)
}

// charge puts one charge of the card, in reais, on the ledger.
func (bench *warningBench) charge(description string, day brday.Day, reais float64) {
	bench.t.Helper()
	bench.charged++
	cardPurchase(bench.fixture, "tx-warning-"+string(rune('a'+bench.charged)), noonOf(bench.t, day), description, reais)
}

// keep makes a subscription of the establishment description names, as it was
// when the person kept it: its creation's purchase stands in for charges the
// ledger does not list.
func (bench *warningBench) keep(description, name string, createdDay brday.Day, cents money.Cents, status authored.SubscriptionStatus, statusDay brday.Day) authored.SubscriptionID {
	bench.t.Helper()
	subscription := keptSubscription("text:"+description, name, createdDay, cents, status)
	subscription.StatusDay = statusDay
	return keepSubscription(bench.t, bench.scratch.Database, "tx-kept-"+description, subscription)
}

func (bench *warningBench) router(day brday.Day) *readEnv {
	bench.t.Helper()
	env := newTestEnv(bench.t, withLedger(bench.fixture.Path, noonOf(bench.t, day)), withDatabase(bench.scratch.Database))
	return &readEnv{testEnv: env, cookie: env.unlock()}
}

// reviewOn is the review as a router with the clock at noon of day answers it
// for month.
func (bench *warningBench) reviewOn(day brday.Day, month string) (*readEnv, []byte) {
	bench.t.Helper()
	env := bench.router(day)
	status, body := env.answer("review.weekly", `["`+month+`"]`)
	if status != http.StatusOK {
		bench.t.Fatalf("review.weekly %s on %s answered %d %s", month, day, status, body)
	}
	if problem := conformanceProblem("review.weekly", body); problem != "" {
		bench.t.Errorf("review.weekly %s on %s does not conform: %s", month, day, problem)
	}
	return env, body
}

func warningsIn(t testing.TB, body []byte) []warningSeen {
	t.Helper()
	var answer struct {
		Warnings []warningSeen `json:"warnings"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		t.Fatal(err)
	}
	return answer.Warnings
}

// shown is the warnings of the review on day. Every one carries no action and
// opens its subscription; and while nothing is dismissed, the kinds of a
// subscription's warnings are the marks its row of the list shows.
func (bench *warningBench) shown(day brday.Day) []warningSeen {
	bench.t.Helper()
	env, body := bench.reviewOn(day, "2026-03")
	warnings := warningsIn(bench.t, body)
	for _, warning := range warnings {
		if string(warning.Actions) != "[]" || warning.SubscriptionID == "" {
			bench.t.Errorf("warning %+v has an action or opens no subscription", warning)
		}
	}
	if len(dismissedNow(bench.t, bench.scratch.Database)) == 0 {
		kindsMatchTheMarks(bench.t, env, warnings)
	}
	return warnings
}

func dismissedNow(t testing.TB, database *authored.Database) []string {
	t.Helper()
	view := database.View()
	defer view.Close()
	dismissed, err := view.DismissedWarnings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for id := range dismissed {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func kindsMatchTheMarks(t testing.TB, env *readEnv, warnings []warningSeen) {
	t.Helper()
	var list struct {
		Groups []struct {
			Subscriptions []struct {
				ID    string             `json:"id"`
				Marks []subscriptionMark `json:"marks"`
			} `json:"subscriptions"`
		} `json:"groups"`
	}
	status, body := env.answer("subscriptions.list", `[{"grouped":false}]`)
	if status != http.StatusOK || json.Unmarshal(body, &list) != nil {
		t.Fatalf("subscriptions.list answered %d %s", status, body)
	}
	for _, group := range list.Groups {
		for _, row := range group.Subscriptions {
			var want, got []string
			for _, mark := range row.Marks {
				want = append(want, kindOfMarkText(mark.Text))
			}
			for _, warning := range warnings {
				if warning.SubscriptionID == row.ID {
					got = append(got, strings.SplitN(warning.ID, ":", 2)[0])
				}
			}
			slices.Sort(want)
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Errorf("subscription %s warns of %v and its row's marks are %v", row.ID, got, want)
			}
		}
	}
}

// expectWarnings compares the warnings of a day with the ones wanted, by id and
// text, in order.
func expectWarnings(t *testing.T, label string, got []warningSeen, want ...[2]string) {
	t.Helper()
	pairs := make([][2]string, len(got))
	for index, warning := range got {
		pairs[index] = [2]string{warning.ID, warning.Text}
	}
	if len(pairs) == 0 && len(want) == 0 {
		return
	}
	if !slices.Equal(pairs, want) {
		t.Errorf("%s: the warnings are %q, want %q", label, pairs, want)
	}
}

func TestAPriceRiseIsOneWarningAndTheNextChargeEitherEndsItOrStartsAnother(t *testing.T) {
	type charge struct {
		day   brday.Day
		reais float64
	}
	history := []charge{{"2026-09-22", 19.9}, {"2026-10-22", 19.9}, {"2026-11-22", 21.9}}
	cases := []struct {
		description string
		charges     []charge
		today       brday.Day
		want        [][2]string
	}{
		{"a price that rose", history, "2026-11-22", [][2]string{{"price:1:2026-11-22", "Preço subiu: Streaming Exemplo de R$ 19,90 para R$ 21,90"}}},
		{"a fourth charge at the same price", append(slices.Clone(history), charge{"2026-12-22", 21.9}), "2026-12-22", nil},
		{"a fourth charge that rises again", append(slices.Clone(history), charge{"2026-12-22", 23.9}), "2026-12-22",
			[][2]string{{"price:1:2026-12-22", "Preço subiu: Streaming Exemplo de R$ 21,90 para R$ 23,90"}}},
		{"prices that fell", []charge{{"2026-09-22", 21.9}, {"2026-10-22", 21.9}, {"2026-11-22", 19.9}}, "2026-11-22", nil},
		{"prices that stayed", []charge{{"2026-09-22", 19.9}, {"2026-10-22", 19.9}, {"2026-11-22", 19.9}}, "2026-11-22", nil},
	}
	for _, c := range cases {
		bench := newWarningBench(t)
		for _, one := range c.charges {
			bench.charge("STREAMING EXEMPLO", one.day, one.reais)
		}
		bench.keep("STREAMING EXEMPLO", "Streaming Exemplo", "2026-09-22", 1_990, authored.StatusKeep, "2026-09-22")
		expectWarnings(t, c.description, bench.shown(c.today), c.want...)
	}
}

func TestTheWholeWarningIsAnObjectOfFourFieldsWithNoActionAndItsSubscription(t *testing.T) {
	bench := newWarningBench(t)
	for _, c := range []struct {
		day   brday.Day
		reais float64
	}{{"2026-09-22", 19.9}, {"2026-10-22", 19.9}, {"2026-11-22", 21.9}} {
		bench.charge("STREAMING EXEMPLO", c.day, c.reais)
	}
	bench.keep("STREAMING EXEMPLO", "Streaming Exemplo", "2026-09-22", 1_990, authored.StatusKeep, "2026-09-22")
	_, body := bench.reviewOn("2026-11-22", "2026-03")
	var answer struct {
		Warnings json.RawMessage `json:"warnings"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		t.Fatal(err)
	}
	want := `[{"id":"price:1:2026-11-22","text":"Preço subiu: Streaming Exemplo de R$ 19,90 para R$ 21,90","subscriptionId":"1","actions":[]}]`
	if got := string(answer.Warnings); got != want {
		t.Errorf("the warnings are %s, want %s", got, want)
	}
}

// A renewal's warning counts down in its text and not in its id, from the
// thirtieth day before to the day, and is gone the day before and the day
// after.
func TestAYearlyPlanWarnsOfItsRenewalFromThirtyDaysBeforeUntilTheDayItRenews(t *testing.T) {
	bench := newWarningBench(t)
	bench.charge("PLANO ANUAL EXEMPLO", "2024-04-20", 166.8)
	bench.charge("PLANO ANUAL EXEMPLO", "2025-04-20", 166.8)
	bench.keep("PLANO ANUAL EXEMPLO", "Plano Anual Exemplo", "2025-04-20", 16_680, authored.StatusKeep, "2025-04-20")
	id := "renewal:1:2026-04-20"
	for _, c := range []struct {
		today brday.Day
		want  [][2]string
	}{
		{"2026-04-20", [][2]string{{id, "Renovação anual hoje: Plano Anual Exemplo · R$ 166,80"}}},
		{"2026-04-19", [][2]string{{id, "Renovação anual em 1 dia: Plano Anual Exemplo · R$ 166,80"}}},
		{"2026-03-21", [][2]string{{id, "Renovação anual em 30 dias: Plano Anual Exemplo · R$ 166,80"}}},
		{"2026-03-20", nil},
		{"2026-04-21", nil},
	} {
		expectWarnings(t, "read on "+string(c.today), bench.shown(c.today), c.want...)
	}
	for day := brday.Day("2026-03-21"); day <= "2026-04-20"; day = daysAfter(t, day, 1) {
		if got := bench.shown(day); len(got) != 1 || got[0].ID != id {
			t.Fatalf("on %s the warnings are %+v, want the one renewal %s", day, got, id)
		}
	}
}

func TestTwoReadsOfOneStateAnswerTheSameBytes(t *testing.T) {
	bench := newWarningBench(t)
	bench.charge("PLANO ANUAL EXEMPLO", "2024-04-20", 166.8)
	bench.charge("PLANO ANUAL EXEMPLO", "2025-04-20", 166.8)
	bench.keep("PLANO ANUAL EXEMPLO", "Plano Anual Exemplo", "2025-04-20", 16_680, authored.StatusKeep, "2025-04-20")
	_, first := bench.reviewOn("2026-04-10", "2026-03")
	_, second := bench.reviewOn("2026-04-10", "2026-03")
	if string(first) != string(second) || !strings.Contains(string(first), "renewal:1:2026-04-20") {
		t.Errorf("two reads answer %s and %s", first, second)
	}
}

func TestATwoYearPlanIsCalledBienalAndAHalfYearlyOneNeverRenews(t *testing.T) {
	bench := newWarningBench(t)
	bench.charge("PLANO BIENAL EXEMPLO", "2022-05-10", 899)
	bench.charge("PLANO BIENAL EXEMPLO", "2024-05-10", 899)
	bench.charge("PLANO SEMESTRAL EXEMPLO", "2025-11-04", 49.9)
	bench.charge("PLANO SEMESTRAL EXEMPLO", "2026-05-04", 49.9)
	bench.keep("PLANO BIENAL EXEMPLO", "Plano Bienal Exemplo", "2024-05-10", 89_900, authored.StatusKeep, "2024-05-10")
	bench.keep("PLANO SEMESTRAL EXEMPLO", "Plano Semestral Exemplo", "2026-05-04", 4_990, authored.StatusKeep, "2026-05-04")
	expectWarnings(t, "on 2026-05-01", bench.shown("2026-05-01"),
		[2]string{"renewal:1:2026-05-10", "Renovação bienal em 9 dias: Plano Bienal Exemplo · R$ 899,00"})
	expectWarnings(t, "three days before the half-yearly plan's next charge", bench.shown("2026-11-01"))
}

func TestACardChargeAfterTheDayItWasMarkedToCancelIsAWarningAndOneBeforeOrOnThatDayIsNot(t *testing.T) {
	bench := newWarningBench(t)
	for _, c := range []struct {
		name string
		day  brday.Day
	}{{"LOJA A", "2026-11-09"}, {"LOJA B", "2026-11-10"}, {"LOJA C", "2026-11-11"}} {
		bench.charge(c.name, c.day, 25)
		bench.keep(c.name, "Loja "+c.name[5:], c.day, 2_500, authored.StatusCancel, "2026-11-10")
	}
	bench.charge("LOJA D", "2026-11-11", 25)
	bench.keep("LOJA D", "Loja D", "2026-11-11", 2_500, authored.StatusKeep, "2026-11-10")
	// Its only charge is the purchase it was made from, after the day it was
	// marked, and the ledger lists none of it.
	bench.keep("LOJA E", "Loja E", "2026-11-15", 2_500, authored.StatusCancel, "2026-11-10")
	warnings := bench.shown("2026-11-22")
	expectWarnings(t, "four charges and a stored one", warnings,
		[2]string{"charged-after-cancel:3:2026-11-11", "Marcada para cancelar e cobrou de novo: Loja C"})
	if warnings[0].SubscriptionID != "3" {
		t.Errorf("the warning opens subscription %q, want its own, 3", warnings[0].SubscriptionID)
	}
}

// Three subscriptions each carry a warning and the third carries two: the
// warnings come by kind, and within a kind by subscription.
func orderedWarningsBench(t *testing.T) *warningBench {
	t.Helper()
	bench := newWarningBench(t)
	bench.charge("LOJA CANCELADA", "2026-11-11", 25)
	bench.keep("LOJA CANCELADA", "Loja Cancelada", "2026-11-11", 2_500, authored.StatusCancel, "2026-11-01")
	for _, c := range []struct {
		day   brday.Day
		reais float64
	}{{"2026-09-22", 19.9}, {"2026-10-22", 19.9}, {"2026-11-22", 21.9}} {
		bench.charge("STREAMING EXEMPLO", c.day, c.reais)
	}
	bench.keep("STREAMING EXEMPLO", "Streaming Exemplo", "2026-09-22", 1_990, authored.StatusKeep, "2026-09-22")
	bench.charge("PLANO ANUAL EXEMPLO", "2024-12-01", 100)
	bench.charge("PLANO ANUAL EXEMPLO", "2025-12-01", 120)
	bench.keep("PLANO ANUAL EXEMPLO", "Plano Anual Exemplo", "2025-12-01", 12_000, authored.StatusKeep, "2025-12-01")
	return bench
}

var orderedWarningIDs = []string{"price:2:2026-11-22", "price:3:2025-12-01", "renewal:3:2026-12-01", "charged-after-cancel:1:2026-11-11"}

func TestWarningsComeByKindAndThenBySubscriptionWhateverMonthIsAsked(t *testing.T) {
	bench := orderedWarningsBench(t)
	var ids []string
	for _, warning := range bench.shown("2026-11-22") {
		ids = append(ids, warning.ID)
	}
	if !slices.Equal(ids, orderedWarningIDs) {
		t.Errorf("the warnings are %v, want %v", ids, orderedWarningIDs)
	}
	_, january := bench.reviewOn("2026-11-22", "2026-01")
	_, march := bench.reviewOn("2026-11-22", "2026-03")
	if !reflect.DeepEqual(warningsIn(t, january), warningsIn(t, march)) || len(warningsIn(t, march)) != 4 {
		t.Errorf("January answers %s and March %s: the warnings are the account's now, not a month's", january, march)
	}
}

func TestNoSubscriptionIsNoWarningAndNoDatabaseIsEmptyListsForTheWholeReview(t *testing.T) {
	bench := newWarningBench(t)
	if got := bench.shown("2026-11-22"); len(got) != 0 {
		t.Errorf("no subscription gives %+v", got)
	}
	bench = orderedWarningsBench(t)
	env := newTestEnv(t, withLedger(bench.fixture.Path, noonOf(t, "2026-11-22")))
	reader := &readEnv{testEnv: env, cookie: env.unlock()}
	status, body := reader.answer("review.weekly", `["2026-03"]`)
	if status != http.StatusOK || !strings.Contains(string(body), `"warnings":[]`) || !strings.Contains(string(body), `"approvals":[]`) {
		t.Errorf("without the app's database the review answers %d %s", status, body)
	}
}

func dismiss(env *readEnv, id string) (status int, body []byte) {
	env.t.Helper()
	return env.answer("review.dismissWarning", `["`+id+`"]`)
}

func TestDismissingAWarningStoresItsIdAndAnUndoShowsItAgain(t *testing.T) {
	bench := orderedWarningsBench(t)
	for _, id := range orderedWarningIDs {
		before := bench.scratch.Canonical(t)
		env := bench.router("2026-11-22")
		status, body := dismiss(env, id)
		change, ok := changeIDIn(body)
		if status != http.StatusOK || !ok || string(body) != `{"changeId":"`+change+`"}` {
			t.Fatalf("dismissing %s answered %d %s", id, status, body)
		}
		if conformanceProblem("review.dismissWarning", body) != "" {
			t.Errorf("the answer %s does not conform", body)
		}
		if got := dismissedNow(t, bench.scratch.Database); !slices.Equal(got, []string{id}) {
			t.Fatalf("after dismissing %s the table holds %v", id, got)
		}
		for _, warning := range bench.shown("2026-11-22") {
			if warning.ID == id {
				t.Errorf("%s is listed after it was dismissed", id)
			}
		}
		if got := bench.shown("2026-11-22"); len(got) != len(orderedWarningIDs)-1 {
			t.Errorf("dismissing %s left %d warnings, want %d", id, len(got), len(orderedWarningIDs)-1)
		}
		env.undo(change)
		if after := bench.scratch.Canonical(t); after != before {
			t.Errorf("after the undo of %s:\n%s", id, differences(before, after))
		}
		if got := bench.shown("2026-11-22"); len(got) != len(orderedWarningIDs) {
			t.Errorf("after the undo of %s the review lists %d warnings, want all %d", id, len(got), len(orderedWarningIDs))
		}
	}
}

func (env *readEnv) undo(change string) {
	env.t.Helper()
	if status, body := env.answer("changes.undo", `["`+change+`"]`); status != http.StatusOK || string(body) != "null" {
		env.t.Fatalf("the undo of %s answered %d %s", change, status, body)
	}
}

func TestADismissedWarningStaysDismissedOnLaterReadsAndAfterTheClockMoves(t *testing.T) {
	bench := orderedWarningsBench(t)
	renewal := "renewal:3:2026-12-01"
	if status, body := dismiss(bench.router("2026-11-22"), renewal); status != http.StatusOK {
		t.Fatalf("dismissing answered %d %s", status, body)
	}
	for _, day := range []brday.Day{"2026-11-22", "2026-11-22", "2026-11-23", "2026-11-30", "2026-12-01"} {
		for _, warning := range bench.shown(day) {
			if warning.ID == renewal {
				t.Errorf("%s is listed on %s after it was dismissed", renewal, day)
			}
		}
	}
	if got := bench.shown("2026-11-23"); len(got) != 3 {
		t.Errorf("the other three warnings are %+v", got)
	}
}

// A warning whose reason went away disappears by itself, and what was
// dismissed stays stored, doing nothing.
func TestADismissedRowOutlivesItsWarningAndDoesNothing(t *testing.T) {
	bench := newWarningBench(t)
	bench.charge("PLANO ANUAL EXEMPLO", "2024-04-20", 166.8)
	bench.charge("PLANO ANUAL EXEMPLO", "2025-04-20", 166.8)
	bench.keep("PLANO ANUAL EXEMPLO", "Plano Anual Exemplo", "2025-04-20", 16_680, authored.StatusKeep, "2025-04-20")
	id := "renewal:1:2026-04-20"
	dismiss(bench.router("2026-04-10"), id)
	if got := bench.shown("2026-04-21"); len(got) != 0 {
		t.Errorf("a renewal that passed is still listed: %+v", got)
	}
	if got := dismissedNow(t, bench.scratch.Database); !slices.Equal(got, []string{id}) {
		t.Errorf("the row is gone with its warning: %v", got)
	}
}

// A price that rises again is another warning, with an id of its own.
func TestAPriceThatRisesAgainIsShownAlthoughTheFirstRiseWasDismissed(t *testing.T) {
	bench := newWarningBench(t)
	for _, c := range []struct {
		day   brday.Day
		reais float64
	}{{"2026-09-22", 19.9}, {"2026-10-22", 19.9}, {"2026-11-22", 21.9}} {
		bench.charge("STREAMING EXEMPLO", c.day, c.reais)
	}
	bench.keep("STREAMING EXEMPLO", "Streaming Exemplo", "2026-09-22", 1_990, authored.StatusKeep, "2026-09-22")
	if status, body := dismiss(bench.router("2026-11-22"), "price:1:2026-11-22"); status != http.StatusOK {
		t.Fatalf("dismissing answered %d %s", status, body)
	}
	expectWarnings(t, "after the dismissal", bench.shown("2026-11-23"))
	bench.charge("STREAMING EXEMPLO", "2026-12-22", 23.9)
	expectWarnings(t, "after a charge that rises again", bench.shown("2026-12-22"),
		[2]string{"price:1:2026-12-22", "Preço subiu: Streaming Exemplo de R$ 21,90 para R$ 23,90"})
}

func TestDismissingWhatNoWarningHoldsAnswersNullAndRecordsNothing(t *testing.T) {
	bench := orderedWarningsBench(t)
	// The price warning of subscription 2 held until a later charge made the
	// same price twice.
	bench.charge("STREAMING EXEMPLO", "2026-12-22", 21.9)
	dismissed := "charged-after-cancel:1:2026-11-11"
	if status, body := dismiss(bench.router("2026-12-22"), dismissed); status != http.StatusOK {
		t.Fatalf("dismissing answered %d %s", status, body)
	}
	env := bench.router("2026-12-22")
	for _, c := range []struct{ description, id string }{
		{"an id no warning has", "nope"},
		{"an empty id", ""},
		{"an id of the stand-in", "preco"},
		{"another id of the stand-in", "renov"},
		{"a kind of the stand-in", "cobrou"},
		{"an id already dismissed", dismissed},
		{"a warning that held before a later charge", "price:2:2026-11-22"},
		{"a renewal that passed", "renewal:3:2026-12-01"},
		{"a subscription nothing keeps", "price:9:2026-11-22"},
	} {
		before := bench.scratch.Everything(t)
		changes := len(bench.scratch.Changes(t))
		status, body := dismiss(env, c.id)
		if status != http.StatusOK || string(body) != "null" {
			t.Errorf("%s: answered %d %s, want 200 null", c.description, status, body)
		}
		if after := bench.scratch.Everything(t); after != before || len(bench.scratch.Changes(t)) != changes {
			t.Errorf("%s: recorded %s", c.description, differences(before, after))
		}
	}
	expectError(t, "a number", env.call("/api/review/dismissWarning", `[7]`, env.cookie), http.StatusBadRequest, "bad_request")
}

func TestWithTheSnapshotMissingDismissingIsUnavailableAndRecordsNothing(t *testing.T) {
	bench := orderedWarningsBench(t)
	env := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "pluggy-snapshot.db"), noonOf(t, "2026-11-22")), withDatabase(bench.scratch.Database))
	before := bench.scratch.Everything(t)
	for _, id := range append([]string{"nope"}, orderedWarningIDs...) {
		expectError(t, id, env.call("/api/review/dismissWarning", `["`+id+`"]`, env.unlock()), http.StatusServiceUnavailable, "ledger_unavailable")
	}
	if after := bench.scratch.Everything(t); after != before {
		t.Errorf("a dismissal with the snapshot missing recorded %s", differences(before, after))
	}
	expectError(t, "the review itself", env.call("/api/review/weekly", `["2026-03"]`, env.unlock()), http.StatusServiceUnavailable, "ledger_unavailable")
}

// Nothing proposes a change yet, so there is nothing to approve, refuse,
// propose or run: each answers null and touches no table, with a snapshot or not.
func TestTheMethodsNothingProducesYetAnswerNullAndRecordNothing(t *testing.T) {
	bench := orderedWarningsBench(t)
	missing := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "pluggy-snapshot.db"), noonOf(t, "2026-11-22")), withDatabase(bench.scratch.Database))
	for label, env := range map[string]*readEnv{
		"with the snapshot": bench.router("2026-11-22"), "with the snapshot missing": {testEnv: missing, cookie: missing.unlock()},
	} {
		for _, c := range []struct{ method, arguments string }{
			{"review.runWarningAction", `["price:2:2026-11-22","silence-pair"]`},
			{"review.runWarningAction", `["price:2:2026-11-22","accept-suggestion"]`},
			{"review.runWarningAction", `["nope","reject-suggestion"]`},
			{"review.approve", `["teto"]`}, {"review.approve", `["nope"]`},
			{"review.refuse", `["teto"]`}, {"review.refuse", `["nope"]`},
			{"review.propose", `["teto"]`}, {"review.propose", `["nope"]`},
		} {
			before := bench.scratch.Everything(t)
			status, body := env.answer(c.method, c.arguments)
			if status != http.StatusOK || string(body) != "null" {
				t.Errorf("%s %s %s: answered %d %s, want 200 null", label, c.method, c.arguments, status, body)
			}
			if after := bench.scratch.Everything(t); after != before {
				t.Errorf("%s %s %s: recorded %s", label, c.method, c.arguments, differences(before, after))
			}
		}
		expectError(t, label+" pause", env.call("/api/review/runWarningAction", `["price:2:2026-11-22","pause"]`, env.cookie),
			http.StatusBadRequest, "bad_request")
	}
}

func TestWithoutTheAppsDatabaseTheFiveMethodsAreReadOnlyBeforeTheirBodyIsRead(t *testing.T) {
	env := &readEnv{testEnv: newTestEnv(t, withLedger(newSubscriptionsLedger(t).Path, noonOf(t, "2026-11-22")))}
	env.cookie = env.unlock()
	cookie := env.cookie
	for _, path := range []string{
		"/api/review/dismissWarning", "/api/review/runWarningAction", "/api/review/approve", "/api/review/refuse", "/api/review/propose",
	} {
		for _, body := range postBodies {
			expectError(t, path+" body "+body, env.call(path, body, cookie), http.StatusForbidden, "read_only")
		}
		if reads, status := readsOfBody(env.testEnv, path, cookie); reads != 0 || status != http.StatusForbidden {
			t.Errorf("%s: the body was read %d times and the status was %d, want 0 and 403", path, reads, status)
		}
	}
}

// The undo of a dismissal is by inverse: a later change of the same row in
// force makes it stale, and nothing is restored.
func TestTheUndoOfADismissalIsRefusedWhenALaterChangeTookTheRowAway(t *testing.T) {
	bench := orderedWarningsBench(t)
	env := bench.router("2026-11-22")
	status, body := dismiss(env, "price:2:2026-11-22")
	change, ok := changeIDIn(body)
	if status != http.StatusOK || !ok {
		t.Fatalf("dismissing answered %d %s", status, body)
	}
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "scratch.takeAway", At: noonOf(t, "2026-11-23")}
	err := bench.scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		return recorder.Delete(t.Context(), "dismissed_warnings", key(text("price:2:2026-11-22")))
	})
	if err != nil {
		t.Fatal(err)
	}
	before := bench.scratch.Everything(t)
	expectRefused(t, "the undo", env.call("/api/changes/undo", `["`+change+`"]`, env.cookie), "stale_change")
	if after := bench.scratch.Everything(t); after != before {
		t.Errorf("the refused undo changed %s", differences(before, after))
	}
}
