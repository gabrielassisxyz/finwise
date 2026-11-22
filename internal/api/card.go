package api

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/cardrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// uncertainDaysBeforeClosing is how far before a closing a purchase may still be
// posted to either bill. It is the rule's own slip window, which the rules keep
// to themselves, so the list shown here is the one the amount was added from.
const uncertainDaysBeforeClosing = 2

type cardPurchaseWire struct {
	TransactionID string `json:"transactionId"`
	Name          string `json:"name"`
	Day           string `json:"day"`
	Amount        Reais  `json:"amount"`
}

type cardBillItemWire struct {
	Name   string  `json:"name"`
	Day    *string `json:"day"`
	Amount Reais   `json:"amount"`
}

type cardBillWire struct {
	Month    string             `json:"month"`
	ClosedOn string             `json:"closedOn"`
	DueOn    string             `json:"dueOn"`
	Total    Reais              `json:"total"`
	Items    []cardBillItemWire `json:"items"`
}

type payAheadWire struct {
	Amount    *Reais             `json:"amount"`
	Purchases []cardPurchaseWire `json:"purchases"`
}

// cardOverviewWire is the contract's CardOverview. The closing, the best
// purchase day and the next-bill amount are null for a card whose closing is not
// known, and so is the due date of the open bill, which is placed by the closing.
type cardOverviewWire struct {
	Title             string             `json:"title"`
	OpenBill          Reais              `json:"openBill"`
	ClosesOn          *string            `json:"closesOn"`
	DueOn             *string            `json:"dueOn"`
	BestPurchaseDay   *string            `json:"bestPurchaseDay"`
	Limit             Reais              `json:"limit"`
	AvailableLimit    Reais              `json:"availableLimit"`
	PayAhead          payAheadWire       `json:"payAhead"`
	MayFallOnNextBill []cardPurchaseWire `json:"mayFallOnNextBill"`
	Bills             []cardBillWire     `json:"bills"`
}

func registerCard(registry *Registry) {
	registry.Register("card.overview", readCardOverview)
}

// cardPurchaseWindowDays is how far back a purchase is looked for to tell which
// card is in use; a card with none in it is told apart by its bills.
const cardPurchaseWindowDays = 120

// theCard is the card in use: the card account with the most recent purchase,
// then the later bill by due date, then the lower account id, so the choice does
// not depend on how the ledger is stored. With no purchase in sight it is the
// card with the latest bill, and a card with no bill ranks below one with one.
func theCard(accounts []ledger.Account, bills []ledger.Bill, lastPurchase map[string]brday.Day) (ledger.Account, bool) {
	latestDue := map[string]brday.Day{}
	for _, bill := range bills {
		latestDue[bill.AccountID] = max(latestDue[bill.AccountID], bill.DueDate)
	}
	var chosen ledger.Account
	found := false
	for _, account := range accounts {
		if account.Type != ledger.AccountCredit {
			continue
		}
		if !found || cardRanksAbove(account, chosen, lastPurchase, latestDue) {
			chosen, found = account, true
		}
	}
	return chosen, found
}

func cardRanksAbove(candidate, chosen ledger.Account, lastPurchase, latestDue map[string]brday.Day) bool {
	if lastPurchase[candidate.ID] != lastPurchase[chosen.ID] {
		return lastPurchase[candidate.ID] > lastPurchase[chosen.ID]
	}
	if latestDue[candidate.ID] != latestDue[chosen.ID] {
		return latestDue[candidate.ID] > latestDue[chosen.ID]
	}
	return candidate.ID < chosen.ID
}

// lastPurchaseDays is, per card account, the day of its latest purchase among
// the last cardPurchaseWindowDays days: a listed card row that is money out, not
// a movement and not a tax row attached to another purchase.
func lastPurchaseDays(ctx context.Context, snapshot *ledger.View, around ledgerContext, today brday.Day) (map[string]brday.Day, error) {
	from, ok := shiftedDay(today, -cardPurchaseWindowDays)
	if !ok {
		return nil, errors.New("api: today is not a day")
	}
	view, err := interpretPeriod(ctx, snapshot, around, brday.Period{First: from, Last: today})
	if err != nil {
		return nil, err
	}
	latest := map[string]brday.Day{}
	for _, transaction := range view.listed {
		if transaction.Kind == txrules.KindCard && transaction.Movement == txrules.NotAMovement && transaction.Amount < 0 {
			latest[transaction.AccountID] = max(latest[transaction.AccountID], transaction.Day)
		}
	}
	return latest, nil
}

// cardView is everything the card page and the home summary need about the card
// in use, worked out once.
type cardView struct {
	account ledger.Account
	rules   cardrules.Card
	bills   []ledger.Bill
	today   brday.Day
	// charged is every row of the card that is money charged or credited on it,
	// and listed the part of it that is money out.
	charged   []txrules.Transaction
	listed    []txrules.Transaction
	rows      map[string]ledger.Transaction
	purchases []cardrules.Purchase
}

// loadCardWithClosingSetting is the card as the person's closing setting places
// its bills: in manual mode the rules take the manual day for every bill the
// ledger gives no closing date, in auto none, whatever day is stored.
func loadCardWithClosingSetting(ctx context.Context, views Views) (*cardView, error) {
	card, err := loadCard(ctx, views.Ledger)
	if err != nil || card == nil {
		return card, err
	}
	if card.rules.ManualClosingDay, err = manualClosingDayOf(ctx, views.Authored); err != nil {
		return nil, err
	}
	return card, nil
}

func manualClosingDayOf(ctx context.Context, view *authored.View) (int, error) {
	mode, err := view.CardClosingMode(ctx)
	if err != nil || mode != authored.ClosingManual {
		return 0, err
	}
	day, err := view.ManualClosingDay(ctx)
	return int(day), err
}

// loadCard returns nil when the ledger holds no card account. It places the
// bills as the ledger and the bank's table do, with no closing day of the
// person's.
func loadCard(ctx context.Context, snapshot *ledger.View) (*cardView, error) {
	around, err := loadContext(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	bills, err := snapshot.Bills(ctx)
	if err != nil {
		return nil, err
	}
	lastPurchase, err := lastPurchaseDays(ctx, snapshot, around, brday.DayOf(nowOf(ctx)))
	if err != nil {
		return nil, err
	}
	account, found := theCard(around.accounts, bills, lastPurchase)
	if !found {
		return nil, nil
	}
	rows, err := snapshot.CleanTransactions(ctx, wholeLedger)
	if err != nil {
		return nil, err
	}
	view, err := interpretRows(rows, around, wholeLedger)
	if err != nil {
		return nil, err
	}
	own := billsOf(bills, account.ID)
	card := &cardView{
		account: account, bills: own, today: brday.DayOf(nowOf(ctx)),
		rules: cardrules.Card{Bills: rulesBillsOf(own), Table: closingTableOf(around.banks[account.ID])},
		rows:  view.rows,
	}
	card.charged = chargedOn(view.listed, account.ID)
	card.listed = moneyOut(card.charged)
	card.purchases = purchasesForRules(card.listed, view.rows)
	return card, nil
}

// closingTableOf picks the closing rule by bank: only C6's has been read off
// real bills, and any other bank's closing date stays unknown.
func closingTableOf(bank string) cardrules.ClosingTable {
	if bank == bankC6 {
		return cardrules.C6ClosingTable()
	}
	return nil
}

func billsOf(bills []ledger.Bill, accountID string) []ledger.Bill {
	var own []ledger.Bill
	for _, bill := range bills {
		if bill.AccountID == accountID {
			own = append(own, bill)
		}
	}
	return own
}

func rulesBillsOf(bills []ledger.Bill) []cardrules.Bill {
	converted := make([]cardrules.Bill, 0, len(bills))
	for _, bill := range bills {
		converted = append(converted, cardrules.Bill{ID: bill.ID, Due: bill.DueDate, Closing: bill.ClosingDate})
	}
	return converted
}

// chargedOn keeps the rows that are the card's own: a bill payment is the bank
// account's movement, and a foreign row with no value in reais cannot be added
// to anything.
func chargedOn(listed []txrules.Transaction, accountID string) []txrules.Transaction {
	var charged []txrules.Transaction
	for _, transaction := range listed {
		if transaction.AccountID == accountID && transaction.Kind == txrules.KindCard &&
			transaction.Movement == txrules.NotAMovement && !transaction.Unconverted {
			charged = append(charged, transaction)
		}
	}
	return charged
}

func moneyOut(charged []txrules.Transaction) []txrules.Transaction {
	var out []txrules.Transaction
	for _, transaction := range charged {
		if transaction.Amount < 0 {
			out = append(out, transaction)
		}
	}
	return out
}

func purchasesForRules(listed []txrules.Transaction, rows map[string]ledger.Transaction) []cardrules.Purchase {
	purchases := make([]cardrules.Purchase, 0, len(listed))
	for _, transaction := range listed {
		purchases = append(purchases, cardrules.Purchase{
			ID: transaction.ID, Day: transaction.Day, BillID: billIDOf(rows[transaction.ID]), Amount: transaction.Amount,
		})
	}
	return purchases
}

func billIDOf(row ledger.Transaction) *string {
	if row.Card == nil {
		return nil
	}
	return row.Card.BillID
}

// openBill is the bill that has not closed yet and when it closes and falls
// due. known is false when the ledger and the bank's rule cannot say.
type openBill struct {
	known   bool
	billID  string
	closing brday.Day
	due     brday.Day
}

// openBillOf asks the rules which bill a purchase made today lands on: that is
// the one still open, and the day it closes is the first whose purchases leave it.
func (card *cardView) openBillOf() openBill {
	placement := card.rules.PlacePurchase(cardrules.Purchase{ID: "today", Day: card.today})
	if placement.Source == cardrules.SourceUnknown {
		return openBill{}
	}
	closing, ok := card.closingOf(placement)
	if !ok {
		return openBill{}
	}
	return openBill{known: true, billID: placement.BillID, closing: closing, due: placement.Due}
}

// closingOf is the closing of the bill a placement names by the rule that
// placed it: the ledger's date when the bill carries one, the card's rule
// otherwise.
func (card *cardView) closingOf(placement cardrules.Placement) (brday.Day, bool) {
	for _, bill := range card.bills {
		if bill.ID == placement.BillID && placement.BillID != "" && bill.ClosingDate != nil {
			return *bill.ClosingDate, true
		}
	}
	return card.rules.PredictedClosing(placement.Due)
}

func (card *cardView) title() string {
	name := nameOfAccount(card.account)
	if digits := card.finalDigits(); digits != "" {
		return cardAccountDescription(name, digits)
	}
	return name
}

// finalDigits is the last four digits of the card's number, when every row of
// the account agrees on them; two cards under one account name no single one.
func (card *cardView) finalDigits() string {
	found := map[string]bool{}
	for _, row := range card.rows {
		if row.AccountID == card.account.ID {
			if digits := lastDigitsOf(row); digits != "" {
				found[digits] = true
			}
		}
	}
	if len(found) != 1 {
		return ""
	}
	for digits := range found {
		return digits
	}
	return ""
}

func readCardOverview(ctx context.Context, _ []json.RawMessage, views Views) (any, error) {
	card, err := loadCardWithClosingSetting(ctx, views)
	if err != nil || card == nil {
		return nil, err
	}
	return card.overview(), nil
}

func (card *cardView) overview() cardOverviewWire {
	open := card.openBillOf()
	wire := cardOverviewWire{
		Title: card.title(), OpenBill: Reais(card.account.Balance),
		PayAhead: payAheadWire{Purchases: []cardPurchaseWire{}}, MayFallOnNextBill: []cardPurchaseWire{},
		Bills: card.pastBills(open),
	}
	if card.account.Credit != nil {
		wire.Limit = optionalReais(card.account.Credit.CreditLimit)
		wire.AvailableLimit = optionalReais(card.account.Credit.AvailableCreditLimit)
	}
	if open.known {
		closing, due := string(open.closing), string(open.due)
		wire.ClosesOn, wire.DueOn, wire.BestPurchaseDay = &closing, &due, &closing
		wire.MayFallOnNextBill = card.mayFallBefore(open.closing)
	}
	if next := card.nextBill(brday.Month(card.today[:7])); next.Known {
		wire.PayAhead = card.payAheadOf(next)
	}
	return wire
}

// nextBill is what the month's spending will charge on the bill after the one
// that closes in the month; Known is false when that closing is not known.
func (card *cardView) nextBill(month brday.Month) cardrules.NextBillAmount {
	return card.rules.AmountOnNextBill(month, card.today, card.purchases)
}

func (card *cardView) payAheadOf(next cardrules.NextBillAmount) payAheadWire {
	amount := Reais(-next.Total)
	wire := payAheadWire{Amount: &amount, Purchases: []cardPurchaseWire{}}
	included := map[string]bool{}
	for _, purchase := range next.Included {
		included[purchase.ID] = true
	}
	for _, transaction := range card.listed {
		if included[transaction.ID] {
			wire.Purchases = append(wire.Purchases, purchaseWireOf(transaction))
		}
	}
	return wire
}

// mayFallBefore lists the purchases of the days just before a closing that the
// bank has not yet put on a bill.
func (card *cardView) mayFallBefore(closing brday.Day) []cardPurchaseWire {
	listed := []cardPurchaseWire{}
	from, ok := shiftedDay(closing, -uncertainDaysBeforeClosing)
	if !ok {
		return listed
	}
	for _, transaction := range card.listed {
		billID := billIDOf(card.rows[transaction.ID])
		if transaction.Day >= from && transaction.Day < closing && (billID == nil || *billID == "") {
			listed = append(listed, purchaseWireOf(transaction))
		}
	}
	return listed
}

func shiftedDay(day brday.Day, days int) (brday.Day, bool) {
	parsed, err := time.Parse("2006-01-02", string(day))
	if err != nil {
		return "", false
	}
	return dayOf(parsed.AddDate(0, 0, days)), true
}

func purchaseWireOf(transaction txrules.Transaction) cardPurchaseWire {
	return cardPurchaseWire{
		TransactionID: transaction.ID, Name: transaction.Name, Day: string(transaction.Day), Amount: Reais(transaction.Amount),
	}
}

// pastBills are the bills of the card other than the open one, newest first.
func (card *cardView) pastBills(open openBill) []cardBillWire {
	past := []cardBillWire{}
	for _, bill := range card.bills {
		if open.known && bill.ID == open.billID {
			continue
		}
		past = append(past, card.billWire(bill))
	}
	sort.SliceStable(past, func(a, b int) bool { return past[a].DueOn > past[b].DueOn })
	return past
}

func (card *cardView) billWire(bill ledger.Bill) cardBillWire {
	wire := cardBillWire{
		Month: string(bill.DueDate[:7]), DueOn: string(bill.DueDate), Total: Reais(bill.TotalAmount),
		Items: card.itemsOf(bill.ID),
	}
	if closing, ok := card.closingOf(cardrules.Placement{BillID: bill.ID, Due: bill.DueDate}); ok {
		wire.ClosedOn = string(closing)
	}
	return wire
}

// itemsOf lists what the bank put on the bill, newest first. An amount is what
// was charged, so a refund on the bill is negative.
func (card *cardView) itemsOf(billID string) []cardBillItemWire {
	items := []cardBillItemWire{}
	for _, transaction := range card.billRows(billID) {
		day := string(transaction.Day)
		items = append(items, cardBillItemWire{Name: transaction.Name, Day: &day, Amount: Reais(-transaction.Amount)})
	}
	return items
}

func (card *cardView) billRows(billID string) []txrules.Transaction {
	var rows []txrules.Transaction
	for _, transaction := range card.charged {
		if id := billIDOf(card.rows[transaction.ID]); id != nil && *id == billID {
			rows = append(rows, transaction)
		}
	}
	sort.SliceStable(rows, func(a, b int) bool { return rows[a].Day > rows[b].Day })
	return rows
}

// optionalReais is the card's limit as the bank states it. The contract has no
// null for a limit, so a bank that states none shows 0 here.
func optionalReais(cents *money.Cents) Reais {
	if cents == nil {
		return 0
	}
	return Reais(*cents)
}
