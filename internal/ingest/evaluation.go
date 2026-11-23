package ingest

import (
	"cmp"
	"errors"
	"slices"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/classrules"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// ruleStepPrefix begins the history step of a line the rule applied; the
// line's name in the budget of the transaction's month follows.
const ruleStepPrefix = "Regra classificou como "

var errNoTrackingStart = errors.New("ingest: the app's database records no tracking start month")

// candidate is a listed transaction the rules have not met. key is zero for one
// the index has never met either.
type candidate struct {
	transaction txrules.Transaction
	key         authored.TxKey
}

// evaluate is step 4: every listed transaction the rules never met, from the
// tracking start month on, in a month whose budget has a line, is indexed and
// marked evaluated, and Decide is asked about it only when neither the person
// nor a rule has put it on a line or in the queue. A transaction of a month with
// no line stays unindexed, so the queue never holds what cannot be answered.
func (step *pass) evaluate(listed []txrules.Transaction, index indexState, shown map[authored.TxKey]txrules.Transaction) error {
	start, found, err := step.view.TrackingStartMonth(step.ctx)
	if err != nil {
		return err
	}
	if !found {
		return errNoTrackingStart
	}
	settings, err := step.settings()
	if err != nil {
		return err
	}
	months := map[brday.Month][]authored.MonthLine{}
	for _, next := range candidatesOf(listed, index, shown) {
		month := monthOf(next.transaction.Day)
		if month < start {
			continue
		}
		lines, cached := months[month]
		if !cached {
			if lines, err = step.view.LinesOfMonth(step.ctx, month); err != nil {
				return err
			}
			months[month] = lines
		}
		if len(lines) == 0 {
			continue
		}
		if err := step.evaluateOne(next, lines, settings); err != nil {
			return err
		}
	}
	return nil
}

// candidatesOf is every listed transaction the rules have not met, oldest
// first, a key once.
func candidatesOf(listed []txrules.Transaction, index indexState, shown map[authored.TxKey]txrules.Transaction) []candidate {
	var found []candidate
	for key, transaction := range shown {
		if !index.rows[key].Evaluated {
			found = append(found, candidate{transaction: transaction, key: key})
		}
	}
	for _, transaction := range listed {
		if _, met := index.keyOf[transaction.ID]; !met {
			found = append(found, candidate{transaction: transaction})
		}
	}
	slices.SortFunc(found, func(a, b candidate) int {
		return cmp.Or(cmp.Compare(a.transaction.Day, b.transaction.Day), cmp.Compare(a.transaction.ID, b.transaction.ID))
	})
	return found
}

func (step *pass) settings() (classrules.Settings, error) {
	threshold, err := step.view.ConfidenceThreshold(step.ctx)
	if err != nil {
		return classrules.Settings{}, err
	}
	examples, err := step.view.MinExamples(step.ctx)
	if err != nil {
		return classrules.Settings{}, err
	}
	return classrules.Settings{Threshold: int(threshold), MinExamples: int(examples)}, nil
}

func (step *pass) evaluateOne(next candidate, lines []authored.MonthLine, settings classrules.Settings) error {
	key := next.key
	if key == 0 {
		var err error
		if key, err = step.recorder.IndexTransaction(step.ctx, seenOf(next.transaction, 0)); err != nil {
			return err
		}
		if err := step.recorder.MarkListed(step.ctx, key, step.run); err != nil {
			return err
		}
	}
	if err := step.recorder.MarkEvaluated(step.ctx, key); err != nil {
		return err
	}
	step.counts.Indexed++
	decided, err := step.alreadyDecided(key)
	if err != nil {
		return err
	}
	if decided {
		step.counts.AlreadyDecided++
		return nil
	}
	decision, err := step.decide(next.transaction, lines, settings)
	if err != nil {
		return err
	}
	return step.write(key, decision, lines)
}

// alreadyDecided reads, under the lock, whether the transaction is on a line or
// in the queue, so a decision the person took in the minutes before a tick is
// never touched.
func (step *pass) alreadyDecided(key authored.TxKey) (bool, error) {
	kept, err := step.view.Transactions(step.ctx, []authored.TxKey{key})
	if err != nil {
		return false, err
	}
	state := kept[key].State
	return state.Line != nil || state.PendingReason != "", nil
}

func (step *pass) decide(transaction txrules.Transaction, lines []authored.MonthLine, settings classrules.Settings) (classrules.Decision, error) {
	establishments, err := step.view.Establishments(step.ctx, []string{transaction.EstablishmentKey})
	if err != nil {
		return classrules.Decision{}, err
	}
	examples, err := step.view.LineExamples(step.ctx, transaction.EstablishmentKey)
	if err != nil {
		return classrules.Decision{}, err
	}
	kept := establishments[transaction.EstablishmentKey]
	input := classrules.Transaction{
		OwnTransfer: transaction.Movement == txrules.OwnTransfer, BillPayment: transaction.Movement == txrules.BillPayment,
		Unconverted: transaction.Unconverted, Credit: transaction.Amount > 0, Refund: transaction.Refund,
		Fee: transaction.Fee, Person: transaction.Person, Nicknamed: kept.Nickname != "",
		DefaultLine: ruleLine(kept.DefaultLine),
	}
	return classrules.Decide(input, examplesOf(examples), lineIDs(lines), settings)
}

func (step *pass) write(key authored.TxKey, decision classrules.Decision, lines []authored.MonthLine) error {
	switch decision.Outcome {
	case classrules.Apply:
		step.counts.Applied++
		line := authored.LineID(decision.Line)
		if err := step.recorder.SettleLine(step.ctx, key, line, authored.AuthorRule); err != nil {
			return err
		}
		_, err := step.recorder.AddHistoryStep(step.ctx, key, authored.StepEffect, ruleStepPrefix+nameOf(lines, line))
		return err
	case classrules.Pending:
		step.countPending(decision.Reason)
		var line *authored.LineID
		var confidence *int64
		if suggestion := decision.Suggestion; suggestion != nil {
			suggested, percent := authored.LineID(suggestion.Line), int64(suggestion.Confidence)
			line, confidence = &suggested, &percent
		}
		return step.recorder.SetPendingReview(step.ctx, key, string(decision.Reason), line, confidence)
	case classrules.Leave:
		step.counts.Left++
	case classrules.NothingToStore:
		step.counts.NothingToStore++
	}
	return nil
}

func (step *pass) countPending(reason classrules.Reason) {
	switch reason {
	case classrules.ReasonLowConfidence:
		step.counts.PendingLowConfidence++
	case classrules.ReasonUnnamedPerson:
		step.counts.PendingUnnamedPerson++
	case classrules.ReasonNewEstablishment:
		step.counts.PendingNewEstablishment++
	}
}

func examplesOf(kept []authored.LineExample) []classrules.Example {
	examples := make([]classrules.Example, 0, len(kept))
	for _, example := range kept {
		converted := classrules.Example{
			Line: classrules.LineID(example.Line), Day: example.Day, ClassifiedBy: classifiedBy(example.ClassifiedBy),
			Excluded: example.Exclusion != nil, Split: example.Split,
		}
		if example.Exclusion != nil {
			converted.ExclusionReason = *example.Exclusion
		}
		examples = append(examples, converted)
	}
	return examples
}

func classifiedBy(author authored.Author) classrules.ClassifiedBy {
	switch author {
	case authored.AuthorPerson:
		return classrules.ClassifiedByPerson
	case authored.AuthorRule:
		return classrules.ClassifiedByRule
	case authored.AuthorModel:
		return classrules.ClassifiedByModel
	}
	return classrules.ClassifiedByUnknown
}

func ruleLine(line *authored.LineID) *classrules.LineID {
	if line == nil {
		return nil
	}
	converted := classrules.LineID(*line)
	return &converted
}

func lineIDs(lines []authored.MonthLine) []classrules.LineID {
	ids := make([]classrules.LineID, len(lines))
	for i, line := range lines {
		ids[i] = classrules.LineID(line.ID)
	}
	return ids
}

func nameOf(lines []authored.MonthLine, id authored.LineID) string {
	for _, line := range lines {
		if line.ID == id {
			return line.Name
		}
	}
	return ""
}
