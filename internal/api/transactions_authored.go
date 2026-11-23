package api

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// The contract's Classifier: who put a transaction on its line.
const (
	classifiedByPerson = "Você"
	classifiedByRule   = "Regra"
	classifiedByModel  = "IA"
)

// transactionLines is one month's budget as a transaction of that month reads it:
// the name each line has in it, and what Dispose needs of it.
type transactionLines struct {
	names  map[authored.LineID]string
	budget budgetrules.Budget
}

func transactionLinesOf(budget authored.Budget) transactionLines {
	lines := transactionLines{
		names:  map[authored.LineID]string{authored.FeesLineID: authored.FeesLineName},
		budget: budgetrules.Budget{Lines: map[budgetrules.LineID]budgetrules.Group{}, FeesLine: budgetrules.LineID(authored.FeesLineID)},
	}
	for _, line := range budget.Lines {
		lines.names[line.ID] = line.Name
		lines.budget.Lines[budgetrules.LineID(line.ID)] = budgetrules.Group(line.Group)
	}
	return lines
}

// nameOf is the line's name in this month. A line the month's budget does not
// have is no line, as Dispose counts it, so the name shown and the total it
// is counted in never disagree.
func (lines transactionLines) nameOf(id *authored.LineID) *string {
	if id == nil {
		return nil
	}
	name, found := lines.names[*id]
	if !found {
		return nil
	}
	return &name
}

// authoredRows is what the app's database keeps about the listed transactions
// of one answer, read once for all of them. A listed transaction the index has
// never met has nothing kept about it but what its establishment has.
type authoredRows struct {
	keys           map[string]authored.TxKey
	kept           map[authored.TxKey]authored.AuthoredTransaction
	establishments map[string]authored.Establishment
	linkedCredits  map[authored.TxKey]bool
	months         map[brday.Month]transactionLines
	// credits is the listed row of each credit a kept purchase links, for the
	// credits the ledger lists now.
	credits map[authored.TxKey]txrules.Transaction
}

func readAuthoredRows(ctx context.Context, views Views, listed []txrules.Transaction) (authoredRows, error) {
	view := views.Authored
	ids := make([]string, 0, len(listed))
	for _, transaction := range listed {
		ids = append(ids, transaction.ID)
	}
	rows := authoredRows{}
	var err error
	if rows.keys, err = view.TransactionKeys(ctx, ids); err != nil {
		return authoredRows{}, err
	}
	if rows.kept, err = view.Transactions(ctx, keysInOrder(ids, rows.keys)); err != nil {
		return authoredRows{}, err
	}
	whole := sharedWholeListing(ctx, views)
	if rows.credits, err = linkedCreditRows(rows.kept, whole); err != nil {
		return authoredRows{}, err
	}
	named := slices.Clone(listed)
	for _, credit := range rows.credits {
		named = append(named, credit)
	}
	if rows.establishments, err = establishmentsOf(ctx, view, named); err != nil {
		return authoredRows{}, err
	}
	if rows.linkedCredits, err = linkedCreditsOf(ctx, view, listed, rows.keys, whole); err != nil {
		return authoredRows{}, err
	}
	if rows.months, err = budgetsOfMonths(ctx, view, listed); err != nil {
		return authoredRows{}, err
	}
	return rows, nil
}

func keysInOrder(ids []string, keys map[string]authored.TxKey) []authored.TxKey {
	ordered := make([]authored.TxKey, 0, len(keys))
	seen := map[authored.TxKey]bool{}
	for _, id := range ids {
		if key, found := keys[id]; found && !seen[key] {
			seen[key] = true
			ordered = append(ordered, key)
		}
	}
	return ordered
}

// linkedCreditsOf says which listed credits are a purchase's reimbursement:
// those a stored purchase names, while the ledger lists that purchase. While
// it does not, the link is kept and the credit counts as an unlinked one, or
// its amount would count nowhere. The purchase may be in a month the answer
// does not cover, so it is looked for in the whole ledger, read only when a
// stored purchase names a credit of the answer.
func linkedCreditsOf(ctx context.Context, view *authored.View, listed []txrules.Transaction, keys map[string]authored.TxKey,
	whole func() (wholeListing, error)) (map[authored.TxKey]bool, error) {
	purchases := map[authored.TxKey]authored.TxKey{}
	for _, transaction := range listed {
		key, indexed := keys[transaction.ID]
		if !indexed || transaction.Amount <= 0 {
			continue
		}
		purchase, found, err := view.PurchaseLinkedTo(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			purchases[key] = purchase
		}
	}
	linked := map[authored.TxKey]bool{}
	if len(purchases) == 0 {
		return linked, nil
	}
	listing, err := whole()
	if err != nil {
		return nil, err
	}
	listedKeys := listing.listedKeys()
	for credit, purchase := range purchases {
		linked[credit] = listedKeys[purchase]
	}
	return linked, nil
}

// budgetsOfMonths is the budget of every month a listed transaction falls in:
// each transaction is read against its own month's, never the current one's.
func budgetsOfMonths(ctx context.Context, view *authored.View, listed []txrules.Transaction) (map[brday.Month]transactionLines, error) {
	months := map[brday.Month]transactionLines{}
	for _, transaction := range listed {
		month := monthOfDay(transaction.Day)
		if _, read := months[month]; read {
			continue
		}
		budget, err := view.BudgetOf(ctx, month)
		if err != nil {
			return nil, err
		}
		months[month] = transactionLinesOf(budget)
	}
	return months, nil
}

// keptOf is what is kept about a listed transaction, and its key; indexed is
// false, and kept the zero value, for one the index has never met.
func (rows authoredRows) keptOf(listed txrules.Transaction) (kept authored.AuthoredTransaction, key authored.TxKey, indexed bool) {
	key, indexed = rows.keys[listed.ID]
	if !indexed {
		return authored.AuthoredTransaction{}, 0, false
	}
	return rows.kept[key], key, true
}

// disposable is a listed transaction as Dispose takes it: the ledger's facts
// and what the person authored about it.
func (rows authoredRows) disposable(listed txrules.Transaction) budgetrules.Transaction {
	kept, key, indexed := rows.keptOf(listed)
	state := kept.State
	transaction := budgetrules.Transaction{
		Amount: listed.Amount, Unconverted: listed.Unconverted, Movement: listed.Movement != txrules.NotAMovement,
		Fee: listed.Fee, Posted: listed.Posted, LedgerRefund: listed.Refund,
		Line: lineRefOf(state.Line), LineSetBy: lineSourceOf(state.ClassifiedBy), ReviewPending: state.PendingReason != "",
		Exclusion: exclusionOf(state.Exclusion), RefundOverride: refundOverrideOf(state.Refund),
		RefundLine: lineRefOf(state.RefundLine), LinkedToPurchase: indexed && rows.linkedCredits[key],
	}
	for _, part := range kept.Split {
		transaction.Split = append(transaction.Split, budgetrules.SplitPart{Line: lineRefOf(part.Line), Amount: part.Cents})
	}
	if _, applied := rows.appliedCredit(state); applied {
		transaction.LinkedCredit = *state.CreditCents
	}
	return transaction
}

// appliedCredit is the credit linked to a purchase, while the ledger lists it:
// a link to a credit it stopped listing is kept, and takes nothing off until
// the credit is listed again, so the purchase never vanishes from the totals
// for money nobody can see arriving.
func (rows authoredRows) appliedCredit(state authored.TransactionState) (txrules.Transaction, bool) {
	if state.Credit == nil || state.CreditCents == nil {
		return txrules.Transaction{}, false
	}
	credit, listed := rows.credits[*state.Credit]
	return credit, listed
}

func lineRefOf(id *authored.LineID) budgetrules.LineRef {
	if id == nil {
		return budgetrules.LineRef{}
	}
	return budgetrules.OnLine(budgetrules.LineID(*id))
}

func lineSourceOf(author authored.Author) budgetrules.LineSource {
	if author == authored.AuthorPerson || author == "" {
		return budgetrules.SetByPerson
	}
	return budgetrules.SetByRule
}

func exclusionOf(exclusion *string) budgetrules.Exclusion {
	switch {
	case exclusion == nil:
		return budgetrules.NotExcluded
	case *exclusion == "":
		return budgetrules.ExcludedWithoutReason
	}
	return budgetrules.ExcludedWithReason
}

func refundOverrideOf(refund *bool) budgetrules.RefundOverride {
	switch {
	case refund == nil:
		return budgetrules.RefundAsLedgerSays
	case *refund:
		return budgetrules.RefundYes
	}
	return budgetrules.RefundNo
}

// transactionOf is a listed transaction on the wire with what was authored
// about it merged in. With nothing authored it is transactionFrom's answer.
func (rows authoredRows) transactionOf(listed txrules.Transaction, source ledger.Transaction, around ledgerContext) transactionWire {
	wire := transactionFrom(listed, source, around)
	kept, key, indexed := rows.keptOf(listed)
	state := kept.State
	month := rows.months[monthOfDay(listed.Day)]
	input := rows.disposable(listed)
	outcome := budgetrules.Dispose(month.budget, []budgetrules.Transaction{input})[0]
	excluded := input.Exclusion == budgetrules.ExcludedWithReason

	wire.Name = shownName(rows.establishments, listed.EstablishmentKey, listed.Name)
	wire.EffectiveAmount = Reais(listed.Amount + outcome.AppliedReimbursement)
	wire.CountsInTotals = wire.CountsInTotals && !excluded
	wire.Line = month.nameOf(state.Line)
	wire.ClassifiedBy = classifierOf(state.ClassifiedBy)
	if state.Confidence != nil {
		confidence := int(*state.Confidence)
		wire.Confidence = &confidence
	}
	wire.Pending = month.pendingOf(state)
	wire.Tags = tagsOf(kept.Tags, rows.establishments[listed.EstablishmentKey].FixedTags)
	if state.Refund != nil {
		wire.Refund = *state.Refund
	}
	wire.ReimbursementIncome = indexed && rows.linkedCredits[key]
	_, wire.Reimbursed = rows.appliedCredit(state)
	wire.Excluded = excluded
	wire.Split = len(kept.Split) >= 2
	return wire
}

func classifierOf(author authored.Author) *string {
	labels := map[authored.Author]string{
		authored.AuthorPerson: classifiedByPerson, authored.AuthorRule: classifiedByRule, authored.AuthorModel: classifiedByModel,
	}
	label, found := labels[author]
	if !found {
		return nil
	}
	return &label
}

// pendingOf is the review a transaction waits for, its suggestion named in the
// transaction's month. A review whose line is not kept, or is not in that
// month's budget, has no suggestion and so no confidence either: a percentage
// with nothing to be a percentage of would only mislead.
func (lines transactionLines) pendingOf(state authored.TransactionState) *pendingReview {
	if state.PendingReason == "" {
		return nil
	}
	review := &pendingReview{Reason: state.PendingReason}
	review.Suggestion = lines.nameOf(state.PendingLine)
	if review.Suggestion != nil && state.PendingConfidence != nil {
		confidence := int(*state.PendingConfidence)
		review.Confidence = &confidence
	}
	return review
}

// tagsOf is the contract's tags: a transaction's own in the order they were
// added, then its establishment's fixed ones in theirs, each once.
func tagsOf(own, fixed []authored.Tag) []string {
	tags := make([]string, 0, len(own)+len(fixed))
	for _, tag := range slices.Concat(own, fixed) {
		if !slices.Contains(tags, tag.Name) {
			tags = append(tags, tag.Name)
		}
	}
	return tags
}

// tagFolds take the accents off the letters Portuguese writes them on, so a
// tag sorts where it is read.
var tagFolds = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

// sortedTags orders tags as a person reads a list of words: case and accents
// aside, and only where two differ in nothing else by their bytes, so the
// order is the same on every call.
func sortedTags(tags []string) []string {
	sorted := slices.Clone(tags)
	slices.SortFunc(sorted, func(a, b string) int {
		if order := cmp.Compare(tagFolds.Replace(strings.ToLower(a)), tagFolds.Replace(strings.ToLower(b))); order != 0 {
			return order
		}
		return cmp.Compare(a, b)
	})
	return sorted
}

// knownTags is every tag in use, a transaction's own or an establishment's
// fixed one, each once, in the order sortedTags gives.
func knownTags(ctx context.Context, view *authored.View) ([]string, error) {
	tags, err := view.KnownTags(ctx)
	if err != nil {
		return nil, err
	}
	return append([]string{}, sortedTags(tags)...), nil
}
