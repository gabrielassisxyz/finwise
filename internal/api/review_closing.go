package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// The closing of a month gathers figures other screens already work out for
// that month: the budget view's spending by line, the payday's transfers with
// their marks, the card's bill after the closing that falls in the month, the
// reviews still waiting up to its last day and the subscriptions whose review
// has come. Nothing here adds spending up or derives a transfer a second time,
// so the closing cannot disagree with the screen each figure comes from.

const (
	stepPending = "pending"
	stepClose   = "close"
)

// closingStepOrder is the contract's ClosingStepId, in the order the screen
// walks them. A test holds it to the manifest.
var closingStepOrder = []string{
	stepPending, "planned-actual", "leftover", "contributions", "card-bill", "subscriptions", "report", stepClose,
}

// contributionGroupKeys are the budget groups compared with what was set aside
// and not with what was spent.
var contributionGroupKeys = map[string]bool{"prov": true, "res": true}

// The contract's ContributionState the server answers. A pot at its target is
// "on-target", and nobody can give a pot a target yet.
const (
	closingContributionDone     = "done"
	closingContributionNotFound = "not-found"
)

type closingStepWire struct {
	ID       string `json:"id"`
	State    string `json:"state"`
	Markable bool   `json:"markable"`
}

type plannedActualLineWire struct {
	Name       string `json:"name"`
	Planned    Reais  `json:"planned"`
	Actual     Reais  `json:"actual"`
	Difference Reais  `json:"difference"`
}

type plannedActualGroupWire struct {
	Name          string                  `json:"name"`
	Planned       Reais                   `json:"planned"`
	Actual        Reais                   `json:"actual"`
	Difference    Reais                   `json:"difference"`
	Contributions bool                    `json:"contributions"`
	Lines         []plannedActualLineWire `json:"lines"`
}

type closingContributionWire struct {
	Name   string `json:"name"`
	Amount Reais  `json:"amount"`
	State  string `json:"state"`
}

type closingPurchaseWire struct {
	Name   string `json:"name"`
	Day    string `json:"day"`
	Amount Reais  `json:"amount"`
}

type closingCardBillWire struct {
	Amount    Reais                 `json:"amount"`
	CardLabel string                `json:"cardLabel"`
	Purchases []closingPurchaseWire `json:"purchases"`
}

type subscriptionToReviewWire struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Amount   Reais  `json:"amount"`
	Cycle    string `json:"cycle"`
	ReviewOn string `json:"reviewOn"`
}

type closingViewWire struct {
	Month                 brday.Month                `json:"month"`
	Closed                bool                       `json:"closed"`
	ClosedOn              *string                    `json:"closedOn"`
	Steps                 []closingStepWire          `json:"steps"`
	PendingCount          int                        `json:"pendingCount"`
	PlannedActual         []plannedActualGroupWire   `json:"plannedActual"`
	Leftover              Reais                      `json:"leftover"`
	Contributions         []closingContributionWire  `json:"contributions"`
	CardBill              *closingCardBillWire       `json:"cardBill"`
	SubscriptionsToReview []subscriptionToReviewWire `json:"subscriptionsToReview"`
	Report                []any                      `json:"report"`
	ClosedMonths          []closedMonth              `json:"closedMonths"`
}

func registerClosing(registry *Registry) {
	registry.Register("review.closing", readClosing)
	registerClosingWrites(registry)
}

// readClosing reads the card first: it is what stops an unreadable ledger from
// being answered as a month nobody has worked on.
func readClosing(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	month, err := onlyMonthArgument(args)
	if err != nil {
		return nil, err
	}
	period, err := brday.MonthPeriod(month)
	if err != nil {
		return nil, err
	}
	card, err := loadCardWithClosingSetting(ctx, views)
	if err != nil {
		return nil, err
	}
	closedMonths, err := views.Authored.ClosedMonths(ctx)
	if err != nil {
		return nil, err
	}
	closedOn, closed := closedOnOf(closedMonths, month)
	pending, err := pendingTransactions(ctx, views, period.Last)
	if err != nil {
		return nil, err
	}
	ticked, err := views.Authored.ClosingStepsDone(ctx, month)
	if err != nil {
		return nil, err
	}
	figures, err := closingFiguresOf(ctx, views, month, closed)
	if err != nil {
		return nil, err
	}
	toReview, err := subscriptionsToReview(ctx, views, period.Last)
	if err != nil {
		return nil, err
	}
	view := closingViewWire{
		Month: month, Closed: closed, Steps: closingSteps(closed, len(pending), ticked), PendingCount: len(pending),
		PlannedActual: figures.plannedActual, Leftover: figures.leftover, Contributions: figures.contributions,
		CardBill: closingCardBillOf(card, month), SubscriptionsToReview: toReview, Report: []any{},
		ClosedMonths: closedMonthsWire(closedMonths),
	}
	if closed {
		day := string(closedOn)
		view.ClosedOn = &day
	}
	return view, nil
}

func closedOnOf(closedMonths []authored.ClosedMonth, month brday.Month) (brday.Day, bool) {
	for _, closed := range closedMonths {
		if closed.Month == month {
			return closed.ClosedOn, true
		}
	}
	return "", false
}

func closedMonthsWire(closedMonths []authored.ClosedMonth) []closedMonth {
	wire := make([]closedMonth, 0, len(closedMonths))
	for _, closed := range closedMonths {
		wire = append(wire, closedMonth{Month: string(closed.Month), ClosedOn: string(closed.ClosedOn)})
	}
	return wire
}

// closingSteps follows the data for the first step and the person's ticks for
// the six in between; a closed month is done throughout and nothing in it is
// ticked again.
func closingSteps(closed bool, pending int, ticked map[string]bool) []closingStepWire {
	steps := make([]closingStepWire, 0, len(closingStepOrder))
	for _, id := range closingStepOrder {
		step := closingStepWire{ID: id, State: "todo"}
		switch {
		case closed:
			step.State = "done"
		case id == stepPending && pending == 0:
			step.State = "done"
		case id == stepPending:
			step.State = "pending"
		case id == stepClose:
		default:
			step.Markable = true
			if ticked[id] {
				step.State = "done"
			}
		}
		steps = append(steps, step)
	}
	return steps
}

// closingFigures are the month's budget weighed as the closing shows it.
type closingFigures struct {
	plannedActual []plannedActualGroupWire
	leftover      Reais
	contributions []closingContributionWire
}

// closingFiguresOf weighs the budget in force for the month against what the
// budget view's own dispositions put on each line, and takes the transfers as
// the payday derives and marks them.
func closingFiguresOf(ctx context.Context, views Views, month brday.Month, closed bool) (closingFigures, error) {
	planned, err := transfersOfMonth(ctx, views, month)
	if err != nil {
		return closingFigures{}, err
	}
	disposed, err := disposeMonth(ctx, views, month)
	if err != nil {
		return closingFigures{}, err
	}
	figures := budgetrules.LineFigures(disposed.outcomes)
	return closingFigures{
		plannedActual: plannedActualOf(planned.plan, figures, planned.transfers, closed),
		leftover:      Reais(leftoverOf(planned.plan, figures)),
		contributions: contributionsOf(planned.transfers, closed),
	}, nil
}

// leftoverOf is the variable cap less what the variable lines spent, negative
// once they went over: the home's limit of the same month.
func leftoverOf(plan budgetrules.Plan, figures map[budgetrules.LineID]money.Cents) money.Cents {
	return budgetrules.VariableLimit(plan, figures).Remaining()
}

// plannedActualOf is each group of the plan with its lines. A provision or a
// reserve is set aside when the transfer that carries it is marked, a link
// being what puts a line in a transfer, or once the month is closed; any other
// line is what was spent or received on it. The difference is negative in the
// bad direction: income that fell short, or anything else past its amount.
func plannedActualOf(plan budgetrules.Plan, figures map[budgetrules.LineID]money.Cents, transfers []budgetrules.MarkedTransfer, closed bool) []plannedActualGroupWire {
	setAside := linesSetAside(transfers)
	groups := make([]plannedActualGroupWire, 0, len(plan))
	for _, group := range plan {
		key := string(group.Key)
		contributions := contributionGroupKeys[key]
		wire := plannedActualGroupWire{Name: editableGroupName(key), Contributions: contributions, Lines: []plannedActualLineWire{}}
		var planned, actual money.Cents
		for _, line := range group.Lines {
			done := figureOfLine(figures, line)
			if contributions {
				done = 0
				if closed || (line.ID.Set && setAside[line.ID.ID]) {
					done = line.Amount
				}
			}
			wire.Lines = append(wire.Lines, plannedActualLineWire{
				Name: line.Name, Planned: Reais(line.Amount), Actual: Reais(done), Difference: Reais(differenceOf(group.Key, line.Amount, done)),
			})
			planned, actual = planned+line.Amount, actual+done
		}
		wire.Planned, wire.Actual, wire.Difference = Reais(planned), Reais(actual), Reais(differenceOf(group.Key, planned, actual))
		groups = append(groups, wire)
	}
	return groups
}

func figureOfLine(figures map[budgetrules.LineID]money.Cents, line budgetrules.PlannedLine) money.Cents {
	if !line.ID.Set {
		return 0
	}
	return figures[line.ID.ID]
}

func differenceOf(group budgetrules.Group, planned, actual money.Cents) money.Cents {
	if group == budgetrules.GroupIncome {
		return actual - planned
	}
	return planned - actual
}

// linesSetAside is every line a marked transfer carries.
func linesSetAside(transfers []budgetrules.MarkedTransfer) map[budgetrules.LineID]bool {
	set := map[budgetrules.LineID]bool{}
	for _, transfer := range transfers {
		if !transfer.Done() {
			continue
		}
		for _, line := range transfer.Lines {
			set[line] = true
		}
	}
	return set
}

// contributionsOf is one row per transfer, in the payday's order: two
// transfers of one name stay two rows. A closed month is taken as having made
// every transfer it planned.
func contributionsOf(transfers []budgetrules.MarkedTransfer, closed bool) []closingContributionWire {
	rows := make([]closingContributionWire, 0, len(transfers))
	for _, transfer := range transfers {
		state := closingContributionNotFound
		if closed || transfer.Done() {
			state = closingContributionDone
		}
		rows = append(rows, closingContributionWire{Name: transfer.Name, Amount: Reais(transfer.Amount), State: state})
	}
	return rows
}

// closingCardBillOf is what the month's purchases charge on the bill after the
// closing that falls in the month, as the card page's pay-ahead works it out
// for the current month. A purchase is money out on the card page and an amount
// owed here. A card whose closing in the month is not known has no bill to
// show: a zero would read as nothing to set aside.
func closingCardBillOf(card *cardView, month brday.Month) *closingCardBillWire {
	if card == nil {
		return nil
	}
	next := card.nextBill(month)
	if !next.Known {
		return nil
	}
	payAhead := card.payAheadOf(next)
	bill := &closingCardBillWire{Amount: *payAhead.Amount, CardLabel: card.title(), Purchases: []closingPurchaseWire{}}
	for _, purchase := range payAhead.Purchases {
		bill.Purchases = append(bill.Purchases, closingPurchaseWire{Name: purchase.Name, Day: purchase.Day, Amount: -purchase.Amount})
	}
	return bill
}

// subscriptionsToReview is every subscription whose review has come by last
// and that is not already being cancelled, in the order they were created,
// with the amount and cycle subscriptions.list answers for it.
func subscriptionsToReview(ctx context.Context, views Views, last brday.Day) ([]subscriptionToReviewWire, error) {
	stored, err := views.Authored.Subscriptions(ctx)
	if err != nil {
		return nil, err
	}
	var due []authored.Subscription
	for _, subscription := range stored {
		if subscription.ReviewOn <= last && subscription.Status != authored.StatusCancel {
			due = append(due, subscription)
		}
	}
	derived, err := deriveSubscriptions(ctx, views, due)
	if err != nil {
		return nil, err
	}
	rows := make([]subscriptionToReviewWire, 0, len(derived))
	for _, subscription := range derived {
		rows = append(rows, subscriptionToReviewWire{
			ID: subscription.stored.ID.String(), Name: subscription.stored.Name, Amount: Reais(subscription.derived.Amount),
			Cycle: subscriptionCycleName(subscription.derived.Cycle), ReviewOn: string(subscription.stored.ReviewOn),
		})
	}
	return rows, nil
}
