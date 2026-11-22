package api

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

func registerChanges(registry *Registry) {
	registry.RegisterJournal("changes.undo", undoChange)
}

// undoChange takes a change back. The engine decides whether it still can: it
// refuses a change that is unknown, already undone or overwritten since, and
// every one of those is answered as the refusal `stale_change`. The method
// answers null, and records no change of its own, so it cannot itself be undone.
func undoChange(ctx context.Context, args []json.RawMessage, journal Journal) (any, error) {
	text, err := stringArgument(args)
	if err != nil {
		return nil, err
	}
	id, ok := changeIDOf(text)
	if !ok {
		return nil, refuse(reasonStaleChange)
	}
	if err := journal.Undo(ctx, id, nowOf(ctx)); err != nil {
		return nil, err
	}
	return nil, nil
}

// changeIDOf reads a change's handle, a decimal string. Only the spelling the
// API itself writes is a handle: "07" and "+7" are not the id of any change the
// journal holds, so they are unknown, and so is whatever else is not a number.
func changeIDOf(text string) (authored.ChangeID, bool) {
	number, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, false
	}
	id := authored.ChangeID(number)
	return id, id.String() == text
}
