package classrules

import (
	"math/rand/v2"
	"testing"
)

const generatedCases = 3000

type generatedCase struct {
	tx       Transaction
	examples []Example
	month    []LineID
	settings Settings
}

// generate draws examples over a few lines so that ties, strict leads and the
// threshold's edge all come up, with every filter of the examples in play.
func generate(random *rand.Rand) generatedCase {
	lines := []LineID{21, 22, 23, 24}
	month := lines[:1+random.IntN(len(lines))]
	examples := make([]Example, random.IntN(40))
	for i := range examples {
		examples[i] = Example{
			Line:         lines[random.IntN(len(lines))],
			Day:          marchDay(1 + random.IntN(31)),
			ClassifiedBy: ClassifiedBy(random.IntN(4)),
			Excluded:     random.IntN(6) == 0,
			Split:        random.IntN(8) == 0,
		}
		if random.IntN(2) == 0 {
			examples[i].ClassifiedBy = ClassifiedByPerson
		}
		if examples[i].Excluded && random.IntN(2) == 0 {
			examples[i].ExclusionReason = "pago por outra pessoa"
		}
	}
	tx := Transaction{Person: random.IntN(2) == 0, Nicknamed: random.IntN(2) == 0, Credit: random.IntN(2) == 0}
	if random.IntN(3) == 0 {
		// Only a default line the month lacks, so the examples still decide.
		tx.DefaultLine = lineRef(99)
	}
	settings := Settings{
		Threshold:   ThresholdLowest + random.IntN(ThresholdHighest-ThresholdLowest+1),
		MinExamples: MinExamplesLowest + random.IntN(12),
	}
	return generatedCase{tx: tx, examples: examples, month: month, settings: settings}
}

// countedPerLine restates the filter of the examples independently of the
// package, so the properties below check Decide against it.
func countedPerLine(c generatedCase) (map[LineID]int, int) {
	inMonth := map[LineID]bool{}
	for _, line := range c.month {
		inMonth[line] = true
	}
	perLine, total := map[LineID]int{}, 0
	for _, example := range c.examples {
		excluded := example.Excluded && example.ExclusionReason != ""
		if example.ClassifiedBy == ClassifiedByPerson && !excluded && !example.Split && inMonth[example.Line] {
			perLine[example.Line]++
			total++
		}
	}
	return perLine, total
}

func TestDecideHoldsItsPropertiesOverGeneratedExamples(t *testing.T) {
	for seed := range uint64(generatedCases) {
		random := rand.New(rand.NewPCG(seed, 1))
		c := generate(random)
		got, err := Decide(c.tx, c.examples, c.month, c.settings)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		for range 5 {
			shuffled := append([]Example(nil), c.examples...)
			random.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
			again, err := Decide(c.tx, shuffled, c.month, c.settings)
			if err != nil || !sameDecision(got, again) {
				t.Fatalf("seed %d: %s, and %s with the examples shuffled to %+v (from %+v)", seed, describe(got), describe(again), shuffled, c)
			}
		}
		if problem := propertyBroken(c, got); problem != "" {
			t.Fatalf("seed %d: %s: got %s for %+v", seed, problem, describe(got), c)
		}
	}
}

func propertyBroken(c generatedCase, got Decision) string {
	perLine, total := countedPerLine(c)
	if got.Outcome == Apply {
		top := perLine[got.Line]
		if total < c.settings.MinExamples {
			return "applied below the minimum of examples"
		}
		for line, count := range perLine {
			if line != got.Line && count >= top {
				return "applied without a strict lead"
			}
		}
		if top*100 < c.settings.Threshold*total {
			return "applied below the threshold"
		}
		return ""
	}
	if got.Outcome != Pending {
		return "neither applied nor pending"
	}
	if got.Reason == ReasonLowConfidence {
		if total == 0 {
			return "low confidence with no example counted"
		}
		if got.Suggestion == nil || got.Suggestion.Confidence < 1 || got.Suggestion.Confidence > 100 {
			return "low confidence without a suggestion and a confidence in 1..100"
		}
		return ""
	}
	if got.Suggestion != nil {
		return "a reason other than low confidence carries a suggestion"
	}
	if total != 0 {
		return "examples were counted and yet no suggestion was made"
	}
	return ""
}
