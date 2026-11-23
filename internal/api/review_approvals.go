package api

import (
	"context"
	"encoding/json"
)

// Nothing in the app proposes a change yet: the nightly model call and the
// command-line client for agents are later work. Until one does, there is
// nothing to approve, refuse or propose, so each of these answers null, reads
// neither store and records nothing, the snapshot missing or not.

func registerReviewApprovals(registry *Registry) {
	registry.RegisterWrite("review.approve", answerNothingProposed)
	registry.RegisterWrite("review.refuse", answerNothingProposed)
	registry.RegisterWrite("review.propose", answerNothingProposed)
}

func answerNothingProposed(context.Context, []json.RawMessage, Writes) (any, error) {
	return nil, nil
}
