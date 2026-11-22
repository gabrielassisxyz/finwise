package classrules

import "github.com/gabrielassisxyz/finwise/internal/brday"

// lineTally is how many examples one line has and the latest day among them.
type lineTally struct {
	line   LineID
	count  int
	latest brday.Day
}

type exampleTally struct {
	total int
	lines map[LineID]*lineTally
}

// tallyExamples keeps only the person's own decisions that still say where the
// money went: a line a rule or the model applied is not one, an excluded or
// split transaction is not on one line, and a line the month lacks cannot be
// applied in it.
func tallyExamples(examples []Example, inMonth map[LineID]bool) exampleTally {
	tally := exampleTally{lines: map[LineID]*lineTally{}}
	for _, example := range examples {
		if !counts(example, inMonth) {
			continue
		}
		tally.total++
		entry, seen := tally.lines[example.Line]
		if !seen {
			entry = &lineTally{line: example.Line}
			tally.lines[example.Line] = entry
		}
		entry.count++
		if example.Day > entry.latest {
			entry.latest = example.Day
		}
	}
	return tally
}

func counts(example Example, inMonth map[LineID]bool) bool {
	if example.ClassifiedBy != ClassifiedByPerson || example.Split {
		return false
	}
	if example.Excluded && example.ExclusionReason != "" {
		return false
	}
	return inMonth[example.Line]
}

func (tally exampleTally) decide(settings Settings) Decision {
	leader, strictLead := tally.leader()
	if tally.total >= settings.MinExamples && strictLead && leader.count*100 >= settings.Threshold*tally.total {
		return Decision{Outcome: Apply, Line: leader.line}
	}
	return Decision{
		Outcome:    Pending,
		Reason:     ReasonLowConfidence,
		Suggestion: &Suggestion{Line: leader.line, Confidence: percentRoundedHalfUp(leader.count, tally.total)},
	}
}

// leader is the most frequent line; among lines tied on count, the one with
// the latest example, then the lower id. strictLead is false when another line
// has as many examples. The order is total, so the answer does not depend on
// the order the examples arrived in.
func (tally exampleTally) leader() (lineTally, bool) {
	var best *lineTally
	for _, entry := range tally.lines {
		if best == nil || ranksAbove(entry, best) {
			best = entry
		}
	}
	for _, entry := range tally.lines {
		if entry != best && entry.count == best.count {
			return *best, false
		}
	}
	return *best, true
}

func ranksAbove(entry, best *lineTally) bool {
	if entry.count != best.count {
		return entry.count > best.count
	}
	if entry.latest != best.latest {
		return entry.latest > best.latest
	}
	return entry.line < best.line
}

// percentRoundedHalfUp is part * 100 / whole rounded half up, in integers.
func percentRoundedHalfUp(part, whole int) int {
	return (part*200 + whole) / (2 * whole)
}
