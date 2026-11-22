package api

import (
	"context"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// loadPotsWithChoices is the pots as the person placed them: each in the bucket
// chosen for it, else where the bank's word puts it. Without the app's database
// nothing is chosen and the answer is loadPots's.
func loadPotsWithChoices(ctx context.Context, views Views, withYield bool) (*potsView, error) {
	view, err := loadPots(ctx, views.Ledger, withYield)
	if err != nil {
		return nil, err
	}
	if err := view.applyChosenBuckets(ctx, views.Authored); err != nil {
		return nil, err
	}
	return view, nil
}

// applyChosenBuckets moves each pot to the bucket the person chose for it. A
// slot's id is never stored, so a pot without an identity keeps the bank's word.
func (view *potsView) applyChosenBuckets(ctx context.Context, chosen *authored.View) error {
	view.chosenBuckets = map[string]string{}
	for index := range view.entries {
		entry := &view.entries[index]
		if strings.HasPrefix(entry.id, slotPrefix) {
			continue
		}
		bucket, found, err := chosen.PotBucket(ctx, entry.id)
		if err != nil {
			return err
		}
		if found {
			entry.bucket = string(bucket)
			view.chosenBuckets[entry.id] = string(bucket)
		}
	}
	return nil
}

// effectiveBucket is the bucket a pot counts in: the one chosen for it, else
// bankWord. It is a label of today, so the chart applies it to every month.
func (view *potsView) effectiveBucket(potID, bankWord string) string {
	if bucket, chosen := view.chosenBuckets[potID]; chosen {
		return bucket
	}
	return bankWord
}

// reservePotID is the id of the pot a reserve row belongs to when the bank keeps
// its identity, as entryOf names it.
func reservePotID(accountID, reserveID string) string {
	return reservePrefix + accountID + "/" + reserveID
}
