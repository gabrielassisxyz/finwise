package api

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// A transaction's history: the person's reason beside its latest step, and
// taking one step back without overwriting what was done after it.

func registerTransactionHistoryWrites(registry *Registry) {
	registry.RegisterWrite("transactions.setReason", setReason)
	registry.RegisterWrite("transactions.undoHistoryStep", undoHistoryStep)
}

// setReason keeps the reason, as sent, beside the transaction's latest step,
// whatever its kind. The reason is the step's own row, so giving one leaves
// the step's change undoable and extendable. A transaction with no step has
// nowhere to keep it, and the change is empty.
func setReason(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, reason, err := twoTextArguments(args)
	if err != nil {
		return nil, err
	}
	target, found, err := lookupTarget(ctx, writes, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, refuse(reasonNotFound)
	}
	kept, err := keptAbout(ctx, writes.Authored, target.key)
	if err != nil {
		return nil, err
	}
	if latest := len(kept.History) - 1; latest >= 0 {
		if err := writes.Recorder.SetStepReason(ctx, kept.History[latest].ID, reason); err != nil {
			return nil, err
		}
	}
	return changeResultOf(ctx, writes.Recorder)
}

// undoHistoryStep is a new change that restores what one step's change did to
// that transaction alone, refused as stale when any of it changed since. It
// answers null, recording nothing, for a transaction the list does not show,
// for a step that is not one of its own and for a note, which records that
// something happened and has nothing to restore.
func undoHistoryStep(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, stepText, err := twoTextArguments(args)
	if err != nil {
		return nil, err
	}
	target, found, err := lookupTarget(ctx, writes, id)
	if err != nil || !found {
		return nil, err
	}
	kept, err := keptAbout(ctx, writes.Authored, target.key)
	if err != nil {
		return nil, err
	}
	step, found := stepNamed(kept.History, stepText)
	if !found || step.Kind != authored.StepEffect {
		return nil, nil
	}
	if err := writes.Recorder.TakeBackStep(ctx, target.key, step, stepIDsOf(kept.History)); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// stepNamed is the step of steps whose id is text in the spelling the API
// writes it in.
func stepNamed(steps []authored.HistoryStep, text string) (authored.HistoryStep, bool) {
	number, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return authored.HistoryStep{}, false
	}
	for _, step := range steps {
		if step.ID == authored.StepID(number) && step.ID.String() == text {
			return step, true
		}
	}
	return authored.HistoryStep{}, false
}

func stepIDsOf(steps []authored.HistoryStep) []authored.StepID {
	ids := make([]authored.StepID, 0, len(steps))
	for _, step := range steps {
		ids = append(ids, step.ID)
	}
	return ids
}
