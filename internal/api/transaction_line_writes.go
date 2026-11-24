package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// Putting transactions on budget lines: one, a selection, and every other
// purchase of an establishment. A line is named by the budget in force for the
// transaction's own month, so a transaction of a closed month can still move
// and one never lands on a line its month does not count, and it is stored by
// its id.

func registerTransactionLineWrites(registry *Registry) {
	registry.RegisterWrite("transactions.setLine", setLine)
	registry.RegisterWrite("transactions.confirmLine", confirmLine)
	registry.RegisterWrite("transactions.setLineForMany", setLineForMany)
	registry.RegisterJournal("transactions.applyLineToEstablishment", applyLineToEstablishment)
}

// lineChangeOrigin is the method whose changes "all purchases here" extends.
const lineChangeOrigin = "transactions.setLine"

type establishmentOfChange struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type lineChangeResult struct {
	ChangeID      string                 `json:"changeId"`
	Establishment *establishmentOfChange `json:"establishment"`
}

type establishmentLineResult struct {
	ChangeID string `json:"changeId"`
	Changed  int    `json:"changed"`
}

// setLine answers null when the transaction is not listed, or when it is on
// that line already with no review waiting; the name is resolved first, so a
// name its month lacks is refused whatever the transaction holds. The panel
// asks why only when the line corrected was the model's or settled a review.
func setLine(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, name, err := idAndText(args)
	if err != nil {
		return nil, err
	}
	target, found, err := lookupTarget(ctx, writes, id)
	if err != nil || !found {
		return nil, err
	}
	line, err := lineNamedInMonth(ctx, writes.Authored, target.listed.Day, name)
	if err != nil {
		return nil, err
	}
	kept, err := keptAbout(ctx, writes.Authored, target.key)
	if err != nil {
		return nil, err
	}
	state := kept.State
	if state.Line != nil && *state.Line == line && state.PendingReason == "" {
		return nil, nil
	}
	ask := state.ClassifiedBy == authored.AuthorModel || state.PendingReason != ""
	if err := writes.Recorder.SetAskReason(ctx, target.key, ask); err != nil {
		return nil, err
	}
	if err := settleWithStep(ctx, writes.Recorder, target.key, line, authored.AuthorPerson, lineChangedStep(name)); err != nil {
		return nil, err
	}
	establishment, err := establishmentOf(ctx, writes.Authored, target.listed)
	if err != nil {
		return nil, err
	}
	change, err := writes.Recorder.ID(ctx)
	if err != nil {
		return nil, err
	}
	return lineChangeResult{ChangeID: change.String(), Establishment: establishment}, nil
}

// confirmLine settles a transaction on the line it shows, or on the suggestion
// of its review when it shows none, each as its own month names it. It records
// the step even when nothing else changes, as the panel lists every
// confirmation.
func confirmLine(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, err := stringArgument(args)
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
	suggestions, err := storedSuggestionInputs(ctx, writes.Authored, target.key)
	if err != nil {
		return nil, err
	}
	budget, err := writes.Authored.BudgetOf(ctx, monthOfDay(target.listed.Day))
	if err != nil {
		return nil, err
	}
	line, name, found := confirmableLine(kept.State, suggestions, budget)
	if !found {
		return nil, nil
	}
	if err := settleWithStep(ctx, writes.Recorder, target.key, line, authored.AuthorPerson, lineConfirmedStep(name)); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

func confirmableLine(state authored.TransactionState, suggestions suggestionInputs, budget authored.Budget) (authored.LineID, string, bool) {
	if !budget.HasLines() {
		return 0, "", false
	}
	lines := transactionLinesOf(budget)
	if name := lines.nameOf(state.Line); name != nil {
		return *state.Line, *name, true
	}
	if shown := lines.pendingSuggestion(state, suggestions); shown.source != suggestionNone {
		return shown.line, shown.name, true
	}
	return 0, "", false
}

// setLineForMany is all or nothing: every id is resolved, then the line in
// every month they fall in, before anything is written. An id the list does
// not show is refused before a month that lacks the line, whatever their
// order. No step is added and the question of why is left as it was.
func setLineForMany(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	ids, name, err := idsAndText(args)
	if err != nil {
		return nil, err
	}
	rows, err := listedSelection(ctx, writes.Ledger, ids)
	if err != nil {
		return nil, err
	}
	lines := make([]authored.LineID, len(rows))
	for i, row := range rows {
		if lines[i], err = lineNamedInMonth(ctx, writes.Authored, row.listed.Day, name); err != nil {
			return nil, err
		}
	}
	for i, row := range rows {
		key, err := indexListed(ctx, writes.Ledger, writes.Recorder, row.listed)
		if err != nil {
			return nil, err
		}
		if err := writes.Recorder.SettleLine(ctx, key, lines[i], authored.AuthorPerson); err != nil {
			return nil, err
		}
	}
	return changeResultOf(ctx, writes.Recorder)
}

// applyLineToEstablishment is "all purchases here": it appends to a live,
// unextended setLine change the establishment's default line and every other
// transaction of that establishment that can go on the line, so one undo takes
// back both.
func applyLineToEstablishment(ctx context.Context, args []json.RawMessage, journal Journal, ledgerView *ledger.View) (any, error) {
	text, err := stringArgument(args)
	if err != nil {
		return nil, err
	}
	origin, ok := changeIDOf(text)
	if !ok {
		return nil, refuse(reasonStaleChange)
	}
	var changed int
	err = journal.Extend(ctx, origin, lineChangeOrigin, func(recorder *authored.Recorder) error {
		view := recorder.View()
		defer view.Close()
		var err error
		changed, err = extendToEstablishment(ctx, recorder, view, ledgerView, origin)
		return err
	})
	if err != nil {
		return nil, err
	}
	return establishmentLineResult{ChangeID: origin.String(), Changed: changed}, nil
}

// establishmentWide is the whole ledger as the list shows it, with what is kept
// about every listed transaction of one establishment.
type establishmentWide struct {
	listed []txrules.Transaction
	keys   map[string]authored.TxKey
	kept   map[authored.TxKey]authored.AuthoredTransaction
}

func extendToEstablishment(ctx context.Context, recorder *authored.Recorder, view *authored.View, snapshot *ledger.View, origin authored.ChangeID) (int, error) {
	steps, err := view.StepsOfChange(ctx, origin)
	if err != nil || len(steps) == 0 {
		return 0, errOrNotFound(err)
	}
	wide, origins, err := readEstablishmentWide(ctx, view, snapshot, steps[0].Key)
	if err != nil {
		return 0, err
	}
	line := wide.kept[steps[0].Key].State.Line
	if !origins.found || origins.listed.Person || origins.listed.Movement != txrules.NotAMovement || line == nil {
		return 0, refuse(reasonNotFound)
	}
	establishment := origins.listed.EstablishmentKey
	if err := recorder.SetDefaultLine(ctx, establishment, *line); err != nil {
		return 0, err
	}
	kept, err := view.Establishments(ctx, []string{establishment})
	if err != nil {
		return 0, err
	}
	mover := establishmentMover{
		recorder: recorder, view: view, snapshot: snapshot, line: *line, origin: origins.listed,
		name: shownName(kept, establishment, origins.listed.Name), months: map[brday.Month]monthLines{},
	}
	return mover.moveAll(ctx, wide, steps[0].Key)
}

// originOfChange is the listed transaction a line change was made on, when the
// ledger still lists it.
type originOfChange struct {
	listed txrules.Transaction
	found  bool
}

// readEstablishmentWide interprets the whole ledger in one pass, as the card
// does, and reads what is kept about the origin and every other transaction of
// its establishment.
func readEstablishmentWide(ctx context.Context, view *authored.View, snapshot *ledger.View, originKey authored.TxKey) (establishmentWide, originOfChange, error) {
	whole, _, err := listedLedger(ctx, snapshot)
	if err != nil {
		return establishmentWide{}, originOfChange{}, err
	}
	ids := make([]string, 0, len(whole.listed))
	for _, listed := range whole.listed {
		ids = append(ids, listed.ID)
	}
	keys, err := view.TransactionKeys(ctx, ids)
	if err != nil {
		return establishmentWide{}, originOfChange{}, err
	}
	var origin originOfChange
	for _, listed := range whole.listed {
		if key, indexed := keys[listed.ID]; indexed && key == originKey {
			origin = originOfChange{listed: listed, found: true}
		}
	}
	wanted := []authored.TxKey{originKey}
	for _, listed := range whole.listed {
		if key, indexed := keys[listed.ID]; indexed && origin.found && listed.EstablishmentKey == origin.listed.EstablishmentKey {
			wanted = append(wanted, key)
		}
	}
	kept, err := view.Transactions(ctx, wanted)
	if err != nil {
		return establishmentWide{}, originOfChange{}, err
	}
	return establishmentWide{listed: whole.listed, keys: keys, kept: kept}, origin, nil
}

// establishmentMover puts the other transactions of the origin's establishment
// on its line, each by a rule and with a step that names the line as its own
// month does.
type establishmentMover struct {
	recorder *authored.Recorder
	view     *authored.View
	snapshot *ledger.View
	line     authored.LineID
	origin   txrules.Transaction
	name     string
	months   map[brday.Month]monthLines
}

func (mover *establishmentMover) moveAll(ctx context.Context, wide establishmentWide, originKey authored.TxKey) (int, error) {
	moved := 0
	for _, listed := range wide.listed {
		key, indexed := wide.keys[listed.ID]
		if listed.EstablishmentKey != mover.origin.EstablishmentKey || listed.ID == mover.origin.ID || (indexed && key == originKey) {
			continue
		}
		var kept authored.AuthoredTransaction
		if indexed {
			kept = wide.kept[key]
		}
		lineName, moves, err := mover.movable(ctx, listed, kept)
		if err != nil {
			return 0, err
		}
		if !moves {
			continue
		}
		key, err = indexListed(ctx, mover.snapshot, mover.recorder, listed)
		if err != nil {
			return 0, err
		}
		if err := settleWithStep(ctx, mover.recorder, key, mover.line, authored.AuthorRule, lineAppliedByRuleStep(lineName, mover.name)); err != nil {
			return 0, err
		}
		moved++
	}
	return moved, nil
}

// movable says whether a transaction of the establishment goes on the line, and
// the line's name in its month: one in the origin's direction (a zero amount is
// a purchase), not a movement, not a refund by the person's word or else the
// ledger's, in reais, not split, not excluded with a reason, not on the line
// already, and in a month whose budget has the line.
func (mover *establishmentMover) movable(ctx context.Context, listed txrules.Transaction, kept authored.AuthoredTransaction) (string, bool, error) {
	state := kept.State
	refund := listed.Refund
	if state.Refund != nil {
		refund = *state.Refund
	}
	switch {
	case (listed.Amount > 0) != (mover.origin.Amount > 0),
		listed.Movement != txrules.NotAMovement,
		refund,
		listed.Unconverted,
		len(kept.Split) >= 2,
		exclusionOf(state.Exclusion) == budgetrules.ExcludedWithReason,
		state.Line != nil && *state.Line == mover.line:
		return "", false, nil
	}
	month := monthOfDay(listed.Day)
	lines, read := mover.months[month]
	if !read {
		var err error
		if lines, err = linesOfMonth(ctx, mover.view, month); err != nil {
			return "", false, err
		}
		mover.months[month] = lines
	}
	name, found := lines.nameOf(mover.line)
	return name, found, nil
}

// settleWithStep puts a transaction on a line and adds the step that says so.
func settleWithStep(ctx context.Context, recorder *authored.Recorder, key authored.TxKey, line authored.LineID, by authored.Author, text string) error {
	if err := recorder.SettleLine(ctx, key, line, by); err != nil {
		return err
	}
	_, err := recorder.AddHistoryStep(ctx, key, authored.StepEffect, text)
	return err
}

// lineNamedInMonth is the line a name names in the budget in force for the
// month of day, the fees line included once that budget has any line.
func lineNamedInMonth(ctx context.Context, view *authored.View, day brday.Day, name string) (authored.LineID, error) {
	lines, err := linesOfMonth(ctx, view, monthOfDay(day))
	if err != nil {
		return 0, err
	}
	line, found := lines.idOf(name)
	if !found {
		return 0, refuse(reasonUnknownLine)
	}
	return line, nil
}

// nameOf is a line's name in the month; a line the month's budget lacks has none.
func (lines monthLines) nameOf(id authored.LineID) (string, bool) {
	for _, line := range lines {
		if line.id == id {
			return line.name, true
		}
	}
	return "", false
}

// establishmentOf is what "all purchases here" would apply to: none for a person
// or a movement.
func establishmentOf(ctx context.Context, view *authored.View, listed txrules.Transaction) (*establishmentOfChange, error) {
	if listed.Person || listed.Movement != txrules.NotAMovement {
		return nil, nil
	}
	kept, err := view.Establishments(ctx, []string{listed.EstablishmentKey})
	if err != nil {
		return nil, err
	}
	return &establishmentOfChange{Key: listed.EstablishmentKey, Name: shownName(kept, listed.EstablishmentKey, listed.Name)}, nil
}

// keptAbout is everything kept about the transaction at key.
func keptAbout(ctx context.Context, view *authored.View, key authored.TxKey) (authored.AuthoredTransaction, error) {
	kept, err := view.Transactions(ctx, []authored.TxKey{key})
	if err != nil {
		return authored.AuthoredTransaction{}, err
	}
	return kept[key], nil
}

func idsAndText(args []json.RawMessage) ([]string, string, error) {
	if len(args) != 2 {
		return nil, "", &badArgumentsError{}
	}
	var ids *[]string
	if err := decodeArgument(args[0], &ids); err != nil || ids == nil {
		return nil, "", &badArgumentsError{}
	}
	text, err := textArgument(args[1])
	return *ids, text, err
}

func errOrNotFound(err error) error {
	if err != nil {
		return err
	}
	return refuse(reasonNotFound)
}
