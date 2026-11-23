package api

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/subrules"
)

func TestTheMoneyTextHasAPlainSpaceAGroupOfThousandsAndTwoDigitsOfCents(t *testing.T) {
	for cents, want := range map[money.Cents]string{
		0: "R$ 0,00", 5: "R$ 0,05", 50: "R$ 0,50", 1_990: "R$ 19,90", 16_680: "R$ 166,80", 99_999: "R$ 999,99",
		100_000: "R$ 1.000,00", 119_900: "R$ 1.199,00", 123_456_789: "R$ 1.234.567,89", 100_000_000_000: "R$ 1.000.000.000,00",
	} {
		got := reaisText(cents)
		if got != want {
			t.Errorf("%d cents are written %q, want %q", cents, got, want)
		}
		if !strings.HasPrefix(got, "R$\x20") || strings.ContainsRune(got, ' ') {
			t.Errorf("%q has no plain space after the symbol", got)
		}
	}
}

func TestTheThreeSentencesAreWrittenCharacterForCharacter(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{priceRoseText("Streaming Exemplo", 1_990, 2_190), "Preço subiu: Streaming Exemplo de R$ 19,90 para R$ 21,90"},
		{priceRoseText("Plano Exemplo", 119_900, 123_456_789), "Preço subiu: Plano Exemplo de R$ 1.199,00 para R$ 1.234.567,89"},
		{renewalText("anual", 0, "Plano Anual Exemplo", 16_680), "Renovação anual hoje: Plano Anual Exemplo · R$ 166,80"},
		{renewalText("anual", 1, "Plano Anual Exemplo", 16_680), "Renovação anual em 1 dia: Plano Anual Exemplo · R$ 166,80"},
		{renewalText("anual", 2, "Plano Anual Exemplo", 16_680), "Renovação anual em 2 dias: Plano Anual Exemplo · R$ 166,80"},
		{renewalText("anual", 30, "Plano Anual Exemplo", 16_680), "Renovação anual em 30 dias: Plano Anual Exemplo · R$ 166,80"},
		{renewalText("bienal", 9, "Plano Bienal Exemplo", 89_900), "Renovação bienal em 9 dias: Plano Bienal Exemplo · R$ 899,00"},
		{renewalText("bienal", 0, "Plano Bienal Exemplo", 89_900), "Renovação bienal hoje: Plano Bienal Exemplo · R$ 899,00"},
		{chargedAfterCancelText("Editor de Fotos Exemplo"), "Marcada para cancelar e cobrou de novo: Editor de Fotos Exemplo"},
	} {
		if c.got != c.want {
			t.Errorf("written %q, want %q", c.got, c.want)
		}
	}
}

func chargeOf(day string, cents money.Cents) subrules.Row {
	return subrules.Row{Day: brday.Day(day), Cents: cents}
}

// derivedOf is subscription number id worked out by the rules, as the review
// reads it.
func derivedOf(t testing.TB, id int64, name string, subscription subrules.Subscription) derivedSubscription {
	t.Helper()
	derivation, err := subrules.Derive(subscription)
	if err != nil {
		t.Fatal(err)
	}
	return derivedSubscription{stored: authored.Subscription{ID: authored.SubscriptionID(id), Name: name}, derived: derivation}
}

type warningExample struct {
	description  string
	subscription subrules.Subscription
	wantID       string
	wantText     string
}

func chargesRead(today string, rows ...subrules.Row) subrules.Subscription {
	return subrules.Subscription{Rows: rows, CreatedDay: "2020-01-01", CreatedCents: 1, StatusDay: "2020-01-01", Today: brday.Day(today)}
}

func cancelledOn(statusDay string, subscription subrules.Subscription) subrules.Subscription {
	subscription.Cancelling, subscription.StatusDay = true, brday.Day(statusDay)
	return subscription
}

var streamingMonths = []subrules.Row{chargeOf("2026-09-22", 1_990), chargeOf("2026-10-22", 1_990), chargeOf("2026-11-22", 2_190)}

var priceExamples = []warningExample{
	{"a price that rose on the latest charge", chargesRead("2026-11-22", streamingMonths...),
		"price:7:2026-11-22", "Preço subiu: Plano Exemplo de R$ 19,90 para R$ 21,90"},
	{"the same price charged once more", chargesRead("2026-12-22", append(slices.Clone(streamingMonths), chargeOf("2026-12-22", 2_190))...), "", ""},
	{"a price that rose again", chargesRead("2026-12-22", append(slices.Clone(streamingMonths), chargeOf("2026-12-22", 2_390))...),
		"price:7:2026-12-22", "Preço subiu: Plano Exemplo de R$ 21,90 para R$ 23,90"},
	{"a price that fell", chargesRead("2026-11-22", chargeOf("2026-09-22", 2_190), chargeOf("2026-10-22", 2_190), chargeOf("2026-11-22", 1_990)), "", ""},
	{"a price that stayed", chargesRead("2026-11-22", chargeOf("2026-09-22", 1_990), chargeOf("2026-10-22", 1_990), chargeOf("2026-11-22", 1_990)), "", ""},
	{"a single charge", chargesRead("2026-11-22", chargeOf("2026-11-22", 1_990)), "", ""},
}

var yearlyCharges = []subrules.Row{chargeOf("2024-04-20", 16_680), chargeOf("2025-04-20", 16_680)}

func renewalOf(today string) warningExample {
	return warningExample{"a yearly plan read on " + today, chargesRead(today, yearlyCharges...), "", ""}
}

func renewalExamples() []warningExample {
	examples := []warningExample{renewalOf("2026-04-20"), renewalOf("2026-04-19"), renewalOf("2026-03-21"), renewalOf("2026-03-20"), renewalOf("2026-04-21")}
	examples[0].wantID, examples[0].wantText = "renewal:7:2026-04-20", "Renovação anual hoje: Plano Exemplo · R$ 166,80"
	examples[1].wantID, examples[1].wantText = "renewal:7:2026-04-20", "Renovação anual em 1 dia: Plano Exemplo · R$ 166,80"
	examples[2].wantID, examples[2].wantText = "renewal:7:2026-04-20", "Renovação anual em 30 dias: Plano Exemplo · R$ 166,80"
	examples[3].description += " (31 days before, the day before the notice)"
	examples[4].description += " (one day after)"
	return append(examples,
		warningExample{"a two-year plan 9 days before", chargesRead("2026-05-01", chargeOf("2022-05-10", 89_900), chargeOf("2024-05-10", 89_900)),
			"renewal:7:2026-05-10", "Renovação bienal em 9 dias: Plano Exemplo · R$ 899,00"},
		warningExample{"a half-yearly plan 3 days before", chargesRead("2026-08-01", chargeOf("2025-08-04", 4_990), chargeOf("2026-02-04", 4_990)), "", ""},
	)
}

var cancelExamples = []warningExample{
	{"charged the day before cancelling", cancelledOn("2026-11-10", chargesRead("2026-11-22", chargeOf("2026-11-09", 2_500))), "", ""},
	{"charged the day it was marked to cancel", cancelledOn("2026-11-10", chargesRead("2026-11-22", chargeOf("2026-11-10", 2_500))), "", ""},
	{"charged the day after", cancelledOn("2026-11-10", chargesRead("2026-11-22", chargeOf("2026-11-11", 2_500))),
		"charged-after-cancel:7:2026-11-11", "Marcada para cancelar e cobrou de novo: Plano Exemplo"},
	{"the same charge of a subscription that is kept", chargesRead("2026-11-22", chargeOf("2026-11-11", 2_500)), "", ""},
	{"only the creation's purchase after the day it was marked", cancelledOn("2026-11-10", subrules.Subscription{
		CreatedDay: "2026-11-15", CreatedCents: 2_500, StatusDay: "2026-11-10", Today: "2026-11-22"}), "", ""},
}

// Each row says the id and the text of the one warning the history holds, or
// that it holds none.
func TestEachKindHoldsExactlyWhenItsMarkDoesAndIsWrittenAsTheReviewReadsIt(t *testing.T) {
	for _, examples := range [][]warningExample{priceExamples, renewalExamples(), cancelExamples} {
		for _, c := range examples {
			warnings, err := warningsOf([]derivedSubscription{derivedOf(t, 7, "Plano Exemplo", c.subscription)})
			if err != nil {
				t.Fatalf("%s: %v", c.description, err)
			}
			switch {
			case c.wantID == "" && len(warnings) != 0:
				t.Errorf("%s: warns %+v, want none", c.description, warnings)
			case c.wantID != "" && len(warnings) != 1:
				t.Errorf("%s: warns %+v, want %s", c.description, warnings, c.wantID)
			case c.wantID != "" && (warnings[0].ID != c.wantID || warnings[0].Text != c.wantText ||
				warnings[0].SubscriptionID != "7" || len(warnings[0].Actions) != 0):
				t.Errorf("%s: warns %+v, want %s %q", c.description, warnings[0], c.wantID, c.wantText)
			}
		}
	}
}

// The id says what triggered the warning and not how far off it is, so a
// dismissal lasts through a renewal's countdown.
func TestARenewalKeepsOneIdFromThirtyDaysToTheDay(t *testing.T) {
	ids := map[string]bool{}
	day := brday.Day("2026-03-21")
	for range 31 {
		warnings, err := warningsOf([]derivedSubscription{derivedOf(t, 7, "Plano Exemplo", chargesRead(string(day), yearlyCharges...))})
		if err != nil || len(warnings) != 1 {
			t.Fatalf("on %s: %+v, %v", day, warnings, err)
		}
		ids[warnings[0].ID] = true
		day = daysAfter(t, day, 1)
	}
	if len(ids) != 1 || !ids["renewal:7:2026-04-20"] {
		t.Errorf("the 31 days gave the ids %v", ids)
	}
}

func daysAfter(t testing.TB, day brday.Day, days int) brday.Day {
	t.Helper()
	parsed, err := time.Parse(time.DateOnly, string(day))
	if err != nil {
		t.Fatal(err)
	}
	return brday.Day(parsed.AddDate(0, 0, days).Format(time.DateOnly))
}

var kindOfMark = map[string]string{markPriceRose: warningKindPrice, markChargedAfterCancel: warningKindChargedAfterCancel}

func kindOfMarkText(text string) string {
	if kind, known := kindOfMark[text]; known {
		return kind
	}
	if strings.HasPrefix(text, "Renova ") {
		return warningKindRenewal
	}
	return "unknown mark " + text
}

// generatedSubscription has a cycle's charges ending near today, so that each
// mark holds for some of them.
func generatedSubscription(t testing.TB, random *rand.Rand, id int64) derivedSubscription {
	t.Helper()
	months := []int{1, 3, 6, 12, 24}[random.IntN(5)]
	last := brday.Day("2026-06-15")
	var rows []subrules.Row
	cents := money.Cents(500 + random.IntN(20_000))
	day := last
	for range 1 + random.IntN(5) {
		rows = append(rows, subrules.Row{Day: day, Cents: cents})
		previous, err := subrules.AddMonths(day, -months)
		if err != nil {
			t.Fatal(err)
		}
		day = previous
		cents = money.Cents(max(100, int(cents)+random.IntN(2_001)-1_000))
	}
	today, err := subrules.AddMonths(last, months)
	if err != nil {
		t.Fatal(err)
	}
	today = daysAfter(t, today, -random.IntN(41))
	subscription := subrules.Subscription{Rows: rows, CreatedDay: "2019-01-01", CreatedCents: 1, Today: today,
		Cancelling: random.IntN(2) == 0, StatusDay: []brday.Day{"2026-06-10", "2026-06-15", "2026-06-16"}[random.IntN(3)]}
	return derivedOf(t, id, fmt.Sprintf("Plano %d", id), subscription)
}

func TestGeneratedSubscriptionsWarnOfExactlyTheMarksOfTheirRowsInTheOrderOfTheKinds(t *testing.T) {
	kinds := []string{warningKindPrice, warningKindRenewal, warningKindChargedAfterCancel}
	produced := map[string]int{}
	for seed := range uint64(300) {
		random := rand.New(rand.NewPCG(seed, 7))
		var subscriptions []derivedSubscription
		for id := range int64(1 + random.IntN(6)) {
			subscriptions = append(subscriptions, generatedSubscription(t, random, id+1))
		}
		warnings, err := warningsOf(subscriptions)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		seen := map[string]bool{}
		for _, subscription := range subscriptions {
			var want, got []string
			for _, mark := range subscriptionMarksOf(subscription.derived.Marks) {
				want = append(want, kindOfMarkText(mark.Text))
			}
			for _, warning := range warnings {
				if warning.SubscriptionID == subscription.stored.ID.String() {
					got = append(got, strings.SplitN(warning.ID, ":", 2)[0])
				}
			}
			slices.Sort(want)
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Fatalf("seed %d: subscription %d warns of %v, its marks are %v", seed, subscription.stored.ID, got, want)
			}
		}
		last := [2]int{-1, -1}
		for _, warning := range warnings {
			if seen[warning.ID] {
				t.Fatalf("seed %d: the id %s is in the answer twice", seed, warning.ID)
			}
			seen[warning.ID] = true
			parts := strings.Split(warning.ID, ":")
			var subscription int
			if _, err := fmt.Sscan(parts[1], &subscription); err != nil {
				t.Fatalf("seed %d: id %s: %v", seed, warning.ID, err)
			}
			position := [2]int{slices.Index(kinds, parts[0]), subscription}
			if position[0] < last[0] || (position[0] == last[0] && position[1] <= last[1]) {
				t.Fatalf("seed %d: %s comes after %v, which is not kind then ascending subscription", seed, warning.ID, last)
			}
			last = position
			produced[parts[0]]++
		}
	}
	for _, kind := range kinds {
		if produced[kind] < 20 {
			t.Errorf("the generated subscriptions produced %d warnings of kind %s, too few to say anything of it", produced[kind], kind)
		}
	}
}
