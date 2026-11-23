package api

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// What the person answers in the weekly queue: the line, and optionally why, a
// nickname for the person paid, or that every purchase at the place goes on
// that line from now on. All of it is one change, so one undo takes the line,
// the rule and the nickname back together.

func registerReviewWrites(registry *Registry) {
	registry.RegisterWrite(acceptPendingMethod, acceptPending)
}

// pendingAnswer is the contract's PendingAnswer.
type pendingAnswer struct {
	Line     string `json:"line"`
	Always   bool   `json:"always"`
	Nickname string `json:"nickname"`
	Reason   string `json:"reason"`
}

type pendingAccepted struct {
	ChangeID string `json:"changeId"`
	Name     string `json:"name"`
}

// acceptPending settles one transaction waiting for review on a line of its own
// month. It answers null, recording nothing, for one that does not wait under
// the queue's own definition, so a second tab answering the same review finds
// nothing to do; only then is the line looked up, so a name the month lacks is
// refused for a transaction that does wait.
func acceptPending(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, answer, err := idAndAnswer(args)
	if err != nil {
		return nil, err
	}
	waiting, found, err := waitingFor(ctx, writes, id)
	if err != nil || !found {
		return nil, err
	}
	line, err := lineNamedInMonth(ctx, writes.Authored, waiting.listed.Day, answer.Line)
	if err != nil {
		return nil, err
	}
	kept, err := writes.Authored.Establishments(ctx, []string{waiting.listed.EstablishmentKey})
	if err != nil {
		return nil, err
	}
	shown := shownName(kept, waiting.listed.EstablishmentKey, waiting.listed.Name)
	if err := recordAnswer(ctx, writes, waiting, line, answer, shown); err != nil {
		return nil, err
	}
	change, err := writes.Recorder.ID(ctx)
	if err != nil {
		return nil, err
	}
	name := shown
	if nickname := givenNickname(waiting.listed, answer); nickname != "" {
		name = nickname
	}
	return pendingAccepted{ChangeID: change.String(), Name: name}, nil
}

// waitingFor is the transaction id names, when it waits for review.
func waitingFor(ctx context.Context, writes Writes, id string) (pendingTransaction, bool, error) {
	if err := requireReadableLedger(ctx, writes.Ledger); err != nil {
		return pendingTransaction{}, false, err
	}
	pending, err := pendingTransactions(ctx, Views{Ledger: writes.Ledger, Authored: writes.Authored}, wholeLedger.Last)
	if err != nil {
		return pendingTransaction{}, false, err
	}
	for _, transaction := range pending {
		if transaction.listed.ID == id {
			return transaction, true, nil
		}
	}
	return pendingTransaction{}, false, nil
}

// recordAnswer writes the answer through the store functions the panel's own
// writes use. The establishment's default line is the very line the
// transaction was put on, by id: after a rename one name can mean two lines in
// two months, and the person ticked the box for the line they answered with.
// So it is not looked up again in the current month, which may lack it. The
// note goes after the step, so the history, newest first, reads it first.
func recordAnswer(ctx context.Context, writes Writes, waiting pendingTransaction, line authored.LineID, answer pendingAnswer, shown string) error {
	key, err := indexListed(ctx, writes.Ledger, writes.Recorder, waiting.listed)
	if err != nil {
		return err
	}
	if err := writes.Recorder.SettleLine(ctx, key, line, authored.AuthorPerson); err != nil {
		return err
	}
	step, err := writes.Recorder.AddHistoryStep(ctx, key, authored.StepEffect, answeredStep(waiting.state, line, answer.Line))
	if err != nil {
		return err
	}
	if reason := strings.TrimSpace(answer.Reason); reason != "" {
		if err := writes.Recorder.SetStepReason(ctx, step, reason); err != nil {
			return err
		}
	}
	if nickname := givenNickname(waiting.listed, answer); nickname != "" {
		if err := writes.Recorder.SetNickname(ctx, waiting.listed.EstablishmentKey, nickname); err != nil {
			return err
		}
	}
	if !answer.Always || !sectionsOf(waiting.listed).Establishment {
		return nil
	}
	if err := writes.Recorder.SetDefaultLine(ctx, waiting.listed.EstablishmentKey, line); err != nil {
		return err
	}
	_, err = writes.Recorder.AddHistoryStep(ctx, key, authored.StepNote, ruleCreatedNote(shown, answer.Line))
	return err
}

// answeredStep compares lines by id: a review with no suggestion is always a
// change of line.
func answeredStep(state authored.TransactionState, line authored.LineID, name string) string {
	if state.PendingLine != nil && *state.PendingLine == line {
		return lineAcceptedStep(name)
	}
	return lineChangedStep(name)
}

// givenNickname is the nickname the answer gives, when it counts: only a
// person has one here, and a blank one is none given, so it never takes away
// a nickname the person already has.
func givenNickname(listed txrules.Transaction, answer pendingAnswer) string {
	if !sectionsOf(listed).Person {
		return ""
	}
	return strings.TrimSpace(answer.Nickname)
}

func idAndAnswer(args []json.RawMessage) (string, pendingAnswer, error) {
	if len(args) != 2 {
		return "", pendingAnswer{}, &badArgumentsError{}
	}
	id, err := textArgument(args[0])
	if err != nil {
		return "", pendingAnswer{}, err
	}
	var answer *pendingAnswer
	if err := decodeArgument(args[1], &answer); err != nil || answer == nil {
		return "", pendingAnswer{}, &badArgumentsError{}
	}
	return id, *answer, nil
}
