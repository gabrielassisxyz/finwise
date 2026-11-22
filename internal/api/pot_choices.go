package api

import (
	"context"
	"slices"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
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
	if err := view.attachLinkedLines(ctx, views.Authored); err != nil {
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

// attachLinkedLines names the lines linked to each pot as the current month's
// budget names them, so a rename shows at once. A link to a line that budget no
// longer has is not shown; its row is kept, since a budget saved for another
// month must not change what names a line in this one.
func (view *potsView) attachLinkedLines(ctx context.Context, chosen *authored.View) error {
	lines, err := linesOfMonth(ctx, chosen, view.month)
	if err != nil {
		return err
	}
	for index := range view.entries {
		entry := &view.entries[index]
		linked, err := chosen.LinesOfPot(ctx, entry.id)
		if err != nil {
			return err
		}
		entry.linkedLines = lines.namesOf(linked)
	}
	return nil
}

// feesLineID is the built-in fees line: the budget's migration gives it this id
// in every database, and it is in no version of the budget.
const feesLineID authored.LineID = 1

type monthLine struct {
	id   authored.LineID
	name string
}

// monthLines is the budget in force for a month as a pot's link names it, in
// the budget's order: the five groups in theirs, each line at its place, and
// the fees line after them once the budget has any line.
type monthLines []monthLine

func linesOfMonth(ctx context.Context, view *authored.View, month brday.Month) (monthLines, error) {
	budget, err := view.BudgetOf(ctx, month)
	if err != nil {
		return nil, err
	}
	lines := make(monthLines, 0, len(budget.Lines)+1)
	for _, line := range budget.Lines {
		lines = append(lines, monthLine{id: line.ID, name: line.Name})
	}
	if budget.HasLines() {
		lines = append(lines, monthLine{id: feesLineID, name: authored.FeesLineName})
	}
	return lines, nil
}

// idOf is the line a name names in the month; an empty name names none.
func (lines monthLines) idOf(name string) (authored.LineID, bool) {
	for _, line := range lines {
		if line.name == name {
			return line.id, true
		}
	}
	return 0, false
}

// namesOf names the linked lines the month has, in the month's order.
func (lines monthLines) namesOf(linked []authored.LineID) []string {
	names := []string{}
	for _, line := range lines {
		if slices.Contains(linked, line.id) {
			names = append(names, line.name)
		}
	}
	return names
}
