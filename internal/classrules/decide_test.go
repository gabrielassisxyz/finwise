package classrules

import (
	"fmt"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// Invented lines of one month's budget.
const (
	lineMarket   LineID = 12
	lineFair     LineID = 13
	lineDining   LineID = 14
	lineFees     LineID = 90
	lineNotInMon LineID = 40
)

var marchLines = []LineID{lineMarket, lineFair, lineDining, lineFees}

var defaults = Settings{Threshold: 85, MinExamples: 5}

func lineRef(line LineID) *LineID { return &line }

// byPerson is an example the person put on its line, on a day of March 2026.
func byPerson(line LineID, day int) Example {
	return Example{Line: line, Day: marchDay(day), ClassifiedBy: ClassifiedByPerson}
}

func marchDay(day int) brday.Day { return brday.Day(fmt.Sprintf("2026-03-%02d", day)) }

// onLine is count examples on one line, on days from first onward.
func onLine(line LineID, count, first int) []Example {
	examples := make([]Example, count)
	for i := range examples {
		examples[i] = byPerson(line, first+i)
	}
	return examples
}

func join(groups ...[]Example) []Example {
	var all []Example
	for _, group := range groups {
		all = append(all, group...)
	}
	return all
}

func applied(line LineID) Decision { return Decision{Outcome: Apply, Line: line} }

func lowConfidence(line LineID, confidence int) Decision {
	return Decision{Outcome: Pending, Reason: ReasonLowConfidence, Suggestion: &Suggestion{Line: line, Confidence: confidence}}
}

func pendingWithout(reason Reason) Decision { return Decision{Outcome: Pending, Reason: reason} }

func sameDecision(a, b Decision) bool {
	if a.Outcome != b.Outcome || a.Left != b.Left || a.Line != b.Line || a.Reason != b.Reason {
		return false
	}
	if a.Suggestion == nil || b.Suggestion == nil {
		return a.Suggestion == nil && b.Suggestion == nil
	}
	return *a.Suggestion == *b.Suggestion
}

func describe(decision Decision) string {
	if decision.Suggestion == nil {
		return fmt.Sprintf("%+v", decision)
	}
	return fmt.Sprintf("%+v suggesting %+v", decision, *decision.Suggestion)
}

type decideCase struct {
	name     string
	tx       Transaction
	examples []Example
	settings Settings
	want     Decision
}

func runDecideCases(t *testing.T, cases []decideCase) {
	t.Helper()
	for _, c := range cases {
		settings := c.settings
		if settings == (Settings{}) {
			settings = defaults
		}
		got, err := Decide(c.tx, c.examples, marchLines, settings)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !sameDecision(got, c.want) {
			t.Errorf("%s: got %s, want %s", c.name, describe(got), describe(c.want))
		}
	}
}

// A default line the month has and five examples on it would apply that line,
// so every row below shows its own step winning over both.
var tempting = onLine(lineMarket, 5, 1)

func TestMovementsUnconvertedRowsAndRefundsAreLeft(t *testing.T) {
	market := lineRef(lineMarket)
	runDecideCases(t, []decideCase{
		{"own transfer", Transaction{OwnTransfer: true, DefaultLine: market}, tempting, defaults, Decision{Outcome: Leave, Left: LeftOwnTransfer}},
		{"bill payment", Transaction{BillPayment: true, DefaultLine: market}, tempting, defaults, Decision{Outcome: Leave, Left: LeftBillPayment}},
		{"unconverted", Transaction{Unconverted: true, DefaultLine: market}, tempting, defaults, Decision{Outcome: Leave, Left: LeftUnconverted}},
		{"refund credit", Transaction{Credit: true, Refund: true, DefaultLine: market}, tempting, defaults, Decision{Outcome: Leave, Left: LeftRefund}},
		{"refund credit that is also a fee row", Transaction{Credit: true, Refund: true, Fee: true}, nil, defaults, Decision{Outcome: Leave, Left: LeftRefund}},
	})
}

func TestFeeRowsHaveNothingToStore(t *testing.T) {
	market := lineRef(lineMarket)
	runDecideCases(t, []decideCase{
		{"fee charge", Transaction{Fee: true, DefaultLine: market}, tempting, defaults, Decision{Outcome: NothingToStore}},
		{"fee discount credit", Transaction{Fee: true, Credit: true, DefaultLine: market}, tempting, defaults, Decision{Outcome: NothingToStore}},
	})
}

func TestADefaultLineAppliesOnlyWhenTheMonthHasIt(t *testing.T) {
	runDecideCases(t, []decideCase{
		{"default line, examples elsewhere", Transaction{DefaultLine: lineRef(lineDining)}, onLine(lineMarket, 7, 1), defaults, applied(lineDining)},
		{"default line, no examples", Transaction{DefaultLine: lineRef(lineDining)}, nil, defaults, applied(lineDining)},
		{"default line the month lacks, examples apply", Transaction{DefaultLine: lineRef(lineNotInMon)}, onLine(lineMarket, 5, 1), defaults, applied(lineMarket)},
		{"default line the month lacks, no examples", Transaction{DefaultLine: lineRef(lineNotInMon)}, nil, defaults, pendingWithout(ReasonNewEstablishment)},
	})
}

func TestExamplesApplyOrSuggestAtTheDefaultSettings(t *testing.T) {
	runDecideCases(t, []decideCase{
		{"5 of 5", Transaction{}, onLine(lineMarket, 5, 1), defaults, applied(lineMarket)},
		{"4 of 4", Transaction{}, onLine(lineMarket, 4, 1), defaults, lowConfidence(lineMarket, 100)},
		{"5 of 6", Transaction{}, join(onLine(lineMarket, 5, 1), onLine(lineFair, 1, 20)), defaults, lowConfidence(lineMarket, 83)},
		{"6 of 7", Transaction{}, join(onLine(lineMarket, 6, 1), onLine(lineFair, 1, 20)), defaults, applied(lineMarket)},
		{"3 and 3, the fair's example latest", Transaction{}, join(onLine(lineMarket, 3, 1), onLine(lineFair, 3, 10)), defaults, lowConfidence(lineFair, 50)},
		{"3 and 3, the market's example latest", Transaction{}, join(onLine(lineMarket, 3, 10), onLine(lineFair, 3, 1)), defaults, lowConfidence(lineMarket, 50)},
		{"3 and 3, both latest on one day", Transaction{}, join(onLine(lineFair, 3, 5), onLine(lineMarket, 3, 5)), defaults, lowConfidence(lineMarket, 50)},
		{"3 and 3 and 1, the single line latest", Transaction{}, join(onLine(lineFair, 3, 1), onLine(lineMarket, 3, 2), onLine(lineDining, 1, 30)), defaults, lowConfidence(lineMarket, 43)},
	})
}

func TestExamplesAtOtherSettings(t *testing.T) {
	low := Settings{Threshold: 50, MinExamples: 2}
	sixty := Settings{Threshold: 60, MinExamples: 5}
	runDecideCases(t, []decideCase{
		{"1 and 1 has no strict lead", Transaction{}, join(onLine(lineMarket, 1, 1), onLine(lineFair, 1, 2)), low, lowConfidence(lineFair, 50)},
		{"2 of 3", Transaction{}, join(onLine(lineMarket, 2, 1), onLine(lineFair, 1, 9)), low, applied(lineMarket)},
		{"3 of 5 against 1 and 1, equality applies", Transaction{}, join(onLine(lineMarket, 3, 1), onLine(lineFair, 1, 9), onLine(lineDining, 1, 10)), sixty, applied(lineMarket)},
		{"1 of 1 below a minimum of 2", Transaction{}, onLine(lineMarket, 1, 1), low, lowConfidence(lineMarket, 100)},
	})
}

func TestConfidenceIsRoundedHalfUp(t *testing.T) {
	cases := []struct{ top, total, want int }{
		{2, 3, 67}, {1, 3, 33}, {1, 2, 50}, {5, 6, 83}, {1, 8, 13}, {3, 8, 38}, {1, 1, 100},
	}
	for _, c := range cases {
		if got := percentRoundedHalfUp(c.top, c.total); got != c.want {
			t.Errorf("%d of %d: %d, want %d", c.top, c.total, got, c.want)
		}
	}
	// The same vectors through Decide: the rest of the examples spread one per
	// line so the leader keeps a strict lead without reaching the threshold.
	through := []struct {
		examples []Example
		want     int
	}{
		{join(onLine(lineMarket, 2, 1), onLine(lineFair, 1, 9)), 67},
		{join(onLine(lineMarket, 3, 1), onLine(lineFair, 2, 9), onLine(lineDining, 2, 20), onLine(lineFees, 1, 25)), 38},
	}
	for _, c := range through {
		got, err := Decide(Transaction{}, c.examples, marchLines, defaults)
		if err != nil || got.Suggestion == nil || got.Suggestion.Confidence != c.want {
			t.Errorf("got %s, %v; want confidence %d", describe(got), err, c.want)
		}
	}
}

func TestOnlyThePersonsOwnExamplesOnTheMonthsLinesCount(t *testing.T) {
	// Four examples on the market fall one short of the minimum; a fifth that
	// counts applies the line, one that does not leaves it pending at 4 of 4.
	four := onLine(lineMarket, 4, 1)
	fifth := func(change func(*Example)) []Example {
		example := byPerson(lineMarket, 20)
		change(&example)
		return join(four, []Example{example})
	}
	runDecideCases(t, []decideCase{
		{"the person's own", Transaction{}, fifth(func(*Example) {}), defaults, applied(lineMarket)},
		{"by a rule", Transaction{}, fifth(func(e *Example) { e.ClassifiedBy = ClassifiedByRule }), defaults, lowConfidence(lineMarket, 100)},
		{"by the model", Transaction{}, fifth(func(e *Example) { e.ClassifiedBy = ClassifiedByModel }), defaults, lowConfidence(lineMarket, 100)},
		{"by nobody known", Transaction{}, fifth(func(e *Example) { e.ClassifiedBy = ClassifiedByUnknown }), defaults, lowConfidence(lineMarket, 100)},
		{"excluded with a reason", Transaction{}, fifth(func(e *Example) { e.Excluded, e.ExclusionReason = true, "reembolsado" }), defaults, lowConfidence(lineMarket, 100)},
		{"excluded without a reason", Transaction{}, fifth(func(e *Example) { e.Excluded = true }), defaults, applied(lineMarket)},
		{"split", Transaction{}, fifth(func(e *Example) { e.Split = true }), defaults, lowConfidence(lineMarket, 100)},
		{"on a line the month lacks", Transaction{}, fifth(func(e *Example) { e.Line = lineNotInMon }), defaults, lowConfidence(lineMarket, 100)},
	})
}

func TestWithNoExamplesThePendingReasonCarriesNoSuggestion(t *testing.T) {
	notCounted := []Example{{Line: lineMarket, Day: marchDay(3), ClassifiedBy: ClassifiedByRule}}
	runDecideCases(t, []decideCase{
		{"person with no nickname", Transaction{Person: true}, nil, defaults, pendingWithout(ReasonUnnamedPerson)},
		{"received PIX from an unknown person", Transaction{Person: true, Credit: true}, nil, defaults, pendingWithout(ReasonUnnamedPerson)},
		{"person with no nickname, only uncounted examples", Transaction{Person: true}, notCounted, defaults, pendingWithout(ReasonUnnamedPerson)},
		{"person with a nickname", Transaction{Person: true, Nicknamed: true}, nil, defaults, pendingWithout(ReasonNewEstablishment)},
		{"establishment", Transaction{}, nil, defaults, pendingWithout(ReasonNewEstablishment)},
		{"nicknamed establishment", Transaction{Nicknamed: true}, nil, defaults, pendingWithout(ReasonNewEstablishment)},
		{"credit from an establishment", Transaction{Credit: true}, nil, defaults, pendingWithout(ReasonNewEstablishment)},
		{"debit marked refund is not left", Transaction{Refund: true}, nil, defaults, pendingWithout(ReasonNewEstablishment)},
	})
}

func TestTheReasonsAreTheQueuesWords(t *testing.T) {
	if ReasonLowConfidence != "Baixa confiança" || ReasonUnnamedPerson != "PIX para pessoa sem apelido" || ReasonNewEstablishment != "Estabelecimento novo" {
		t.Errorf("reasons %q, %q, %q", ReasonLowConfidence, ReasonUnnamedPerson, ReasonNewEstablishment)
	}
}

func TestSettingsOutsideTheirRangeAreRefused(t *testing.T) {
	for _, settings := range []Settings{
		{}, {Threshold: 49, MinExamples: 5}, {Threshold: 100, MinExamples: 5},
		{Threshold: 85, MinExamples: 0}, {Threshold: 85, MinExamples: 51},
	} {
		if _, err := Decide(Transaction{}, onLine(lineMarket, 5, 1), marchLines, settings); err == nil {
			t.Errorf("%+v was accepted", settings)
		}
	}
	for _, settings := range []Settings{{Threshold: 50, MinExamples: 1}, {Threshold: 99, MinExamples: 50}} {
		if _, err := Decide(Transaction{}, nil, marchLines, settings); err != nil {
			t.Errorf("%+v: %v", settings, err)
		}
	}
}
