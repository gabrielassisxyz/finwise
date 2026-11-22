package api

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// changeResultWire is the contract's ChangeResult: the handle of the change a
// write recorded, which the interface hands back to undo it.
type changeResultWire struct {
	ChangeID string `json:"changeId"`
}

// changeResultOf answers the change the recorder holds. Asking for its id
// writes the change's row when no effect has, so a write that changed nothing
// still answers a change, whose undo does nothing.
func changeResultOf(ctx context.Context, recorder *authored.Recorder) (changeResultWire, error) {
	id, err := recorder.ID(ctx)
	if err != nil {
		return changeResultWire{}, err
	}
	return changeResultWire{ChangeID: id.String()}, nil
}
