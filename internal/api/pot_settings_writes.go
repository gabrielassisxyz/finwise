package api

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// The person's choices about a pot: the bucket it counts in and the budget lines
// it feeds. Each write is one change through the pots' store, and the pot it
// names is found as the pots read finds it, in the request's snapshot.

func registerPotSettingsWrites(registry *Registry) {
	registry.RegisterWrite("settings.setPotBucket", setPotBucket)
	registry.RegisterWrite("settings.toggleLinkedLine", toggleLinkedLine)
}

// setPotBucket stores the bucket for a pot. Choosing the bank's own word for it
// clears the row rather than storing it, so a stored bucket always says the
// person moved the pot; either way every answer is the same.
func setPotBucket(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	potID, err := textArgument(args[0])
	if err != nil {
		return nil, err
	}
	bucket, err := bucketArgument(args[1])
	if err != nil {
		return nil, err
	}
	pot, err := potOfWrite(ctx, writes.Ledger, potID)
	if err != nil {
		return nil, err
	}
	if string(bucket) == pot.bucket {
		err = writes.Recorder.ClearPotBucket(ctx, potID)
	} else {
		err = writes.Recorder.SetPotBucket(ctx, potID, bucket)
	}
	if err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// toggleLinkedLine links a line to a pot, unlinks it from that pot, or moves it
// from another, by the line's id. A line has no month of its own, so its name
// is resolved in the budget in force for the current month. The pot is checked
// before the line.
func toggleLinkedLine(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	potID, err := textArgument(args[0])
	if err != nil {
		return nil, err
	}
	name, err := textArgument(args[1])
	if err != nil {
		return nil, err
	}
	if _, err := potOfWrite(ctx, writes.Ledger, potID); err != nil {
		return nil, err
	}
	lines, err := linesOfMonth(ctx, writes.Authored, brday.MonthOf(nowOf(ctx)))
	if err != nil {
		return nil, err
	}
	line, found := lines.idOf(name)
	if !found {
		return nil, refuse(reasonUnknownLine)
	}
	if err := writes.Recorder.ToggleLink(ctx, line, potID); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// potOfWrite is the pot a write names, among the pots the pots read lists, the
// closed ones included. A slot's id is refused before the snapshot is read:
// whatever it names today, it names another pot tomorrow, and a choice stored
// for it would move by itself.
func potOfWrite(ctx context.Context, snapshot *ledger.View, potID string) (potEntry, error) {
	if strings.HasPrefix(potID, slotPrefix) {
		return potEntry{}, refuse(reasonUnstablePot)
	}
	view, err := loadPots(ctx, snapshot, false)
	if err != nil {
		return potEntry{}, err
	}
	for _, entry := range view.entries {
		if entry.id == potID {
			return entry, nil
		}
	}
	return potEntry{}, refuse(reasonNotFound)
}

func textArgument(raw json.RawMessage) (string, error) {
	var text *string
	if err := decodeArgument(raw, &text); err != nil || text == nil {
		return "", &badArgumentsError{}
	}
	return *text, nil
}

// bucketArgument holds the bucket to the three the contract names, as the
// router's schema does, so that the store is never handed another.
func bucketArgument(raw json.RawMessage) (authored.PotBucket, error) {
	text, err := textArgument(raw)
	if err != nil {
		return "", err
	}
	switch bucket := authored.PotBucket(text); bucket {
	case authored.BucketAvailable, authored.BucketReserved, authored.BucketInvested:
		return bucket, nil
	}
	return "", &badArgumentsError{}
}
