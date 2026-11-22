package api

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/potrules"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

const (
	// A pot's bucket is the bank's own word for what it is until the person
	// assigns one: a reserve entry is reserved money and an investment is invested.
	bucketReserved = "Reservado"
	bucketInvested = "Investido"

	reservePrefix    = "reserve:"
	slotPrefix       = "slot:"
	investmentPrefix = "investment:"
)

// errUnreadableAmount fails a read that would otherwise show a pot or an
// investment at a figure the ledger did not give: a total quietly short of one
// is worse than an error, and the contract has no null for a balance.
var errUnreadableAmount = errors.New("api: a pot or an investment has no readable amount")

type potMovementWire struct {
	Day         string `json:"day"`
	Description string `json:"description"`
	Amount      Reais  `json:"amount"`
}

// investmentFigures are an investment's own numbers; principal is nil when the
// provider does not state what was applied.
type investmentFigures struct {
	principal *money.Cents
	gross     money.Cents
	net       money.Cents
	due       *brday.Day
}

// potEntry is one thing the pots screens list: a reserve entry or an
// investment. Its balance is what the bank holds now, and zero for a closed one.
type potEntry struct {
	id           string
	accountID    string
	reserveID    string
	itemID       string
	name         *string
	bank         string
	bucket       string
	balance      money.Cents
	cdiPercent   *Percent
	closed       bool
	base         potrules.Base
	historyStart brday.Month
	investment   *investmentFigures
	movements    []potMovementWire
	yield        potrules.MonthYield
	// yieldAsked is false for what the yield rules do not apply to: an
	// investment, and an entry with no identity to follow.
	yieldAsked bool
}

// potsView is what the pots, the net worth and the home summary read from.
type potsView struct {
	today     brday.Day
	month     brday.Month
	accounts  []ledger.Account
	banks     map[string]string
	items     map[string]string
	sightings map[string][]brday.Day
	// What the net worth's history is read from: the bank accounts' sightings,
	// every reserve entry of them, and the days of each investment counted.
	balances       map[string][]ledger.AccountDay
	reserveHistory []ledger.Pot
	investmentDays map[string][]ledger.InvestmentDay
	entries        []potEntry
	unattributed   map[string][]potMovementWire
	// onReserveAccounts counts the investments that sit on a connection whose
	// account also holds reserve entries, and leftOut those of them the ledger
	// itself shows back the reserve and which are therefore not counted twice.
	onReserveAccounts int
	leftOut           int
}

func loadPots(ctx context.Context, snapshot *ledger.View, withYield bool) (*potsView, error) {
	around, err := loadContext(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	days, err := snapshot.BalanceHistory(ctx)
	if err != nil {
		return nil, err
	}
	history, err := snapshot.ReserveHistory(ctx)
	if err != nil {
		return nil, err
	}
	now := nowOf(ctx)
	view := &potsView{
		today: brday.DayOf(now), month: brday.MonthOf(now), accounts: around.accounts, banks: around.banks,
		items:     itemBankLabels(around.accounts, around.banks),
		sightings: sightingDaysOf(days, bankAccountSet(around.accounts)),
	}
	view.balances = balancesOf(days, view.sightings)
	view.reserveHistory = reserveRowsOf(history, view.sightings)
	sightings := sightingsOf(history, view.sightings)
	reserves, err := view.reserveEntries(sightings)
	if err != nil {
		return nil, err
	}
	if err := view.attachMovements(ctx, snapshot, around, sightings, reserves); err != nil {
		return nil, err
	}
	if err := view.attachYields(ctx, snapshot, withYield, reserves); err != nil {
		return nil, err
	}
	invested, err := loadInvestments(ctx, snapshot, view, reserves)
	if err != nil {
		return nil, err
	}
	view.entries = sortedEntries(append(reserves, invested...))
	return view, nil
}

func bankAccountSet(accounts []ledger.Account) map[string]bool {
	banks := map[string]bool{}
	for _, account := range accounts {
		if account.Type == ledger.AccountBank {
			banks[account.ID] = true
		}
	}
	return banks
}

// sightingDaysOf lists, per bank account, the days it was sighted in order.
func sightingDaysOf(days []ledger.AccountDay, banks map[string]bool) map[string][]brday.Day {
	perAccount := map[string][]brday.Day{}
	for _, day := range days {
		if banks[day.AccountID] {
			perAccount[day.AccountID] = append(perAccount[day.AccountID], day.Day)
		}
	}
	for _, own := range perAccount {
		sort.Slice(own, func(a, b int) bool { return own[a] < own[b] })
	}
	return perAccount
}

func balancesOf(days []ledger.AccountDay, sighted map[string][]brday.Day) map[string][]ledger.AccountDay {
	perAccount := map[string][]ledger.AccountDay{}
	for _, day := range days {
		if _, current := sighted[day.AccountID]; current {
			perAccount[day.AccountID] = append(perAccount[day.AccountID], day)
		}
	}
	return perAccount
}

func reserveRowsOf(history []ledger.Pot, sighted map[string][]brday.Day) []ledger.Pot {
	var rows []ledger.Pot
	for _, pot := range history {
		if _, current := sighted[pot.AccountID]; current {
			rows = append(rows, pot)
		}
	}
	return rows
}

func sightingsOf(history []ledger.Pot, sighted map[string][]brday.Day) []potrules.Sighting {
	sightings := make([]potrules.Sighting, 0, len(history))
	for _, pot := range history {
		if _, current := sighted[pot.AccountID]; !current {
			continue
		}
		sighting := potrules.Sighting{
			AccountID: pot.AccountID, Day: pot.Day, ReserveID: pot.ReserveID, Name: pot.Name,
			StableIdentity: pot.StableIdentity, CDIPercent: pot.CDIPercentage,
		}
		if pot.AmountStatus == "ok" {
			sighting.Balance = pot.Amount
		}
		sightings = append(sightings, sighting)
	}
	return sightings
}

// reserveEntries turns the sightings into entries. The rules take an account's
// latest day from its pot rows, so an account that emptied every pot would leave
// them looking open: the account's own latest sighting settles it here.
func (view *potsView) reserveEntries(sightings []potrules.Sighting) ([]potEntry, error) {
	entries := []potEntry{}
	slots := map[string]int{}
	for _, pot := range potrules.Identify(sightings) {
		latest := latestOf(view.sightings[pot.AccountID])
		gone := pot.LatestDay < latest
		if gone && !pot.Stable {
			continue
		}
		if !gone && !pot.Closed && pot.Balance == nil {
			return nil, errUnreadableAmount
		}
		entry := view.entryOf(pot, gone || pot.Closed, slots)
		entries = append(entries, entry)
	}
	return entries, nil
}

func latestOf(days []brday.Day) brday.Day {
	if len(days) == 0 {
		return ""
	}
	return days[len(days)-1]
}

func (view *potsView) entryOf(pot potrules.Pot, closed bool, slots map[string]int) potEntry {
	entry := potEntry{
		accountID: pot.AccountID, reserveID: pot.ReserveID, name: namedOrNil(pot.Name),
		bank: view.banks[pot.AccountID], bucket: bucketReserved,
		closed: closed, base: baseOfBank(view.banks[pot.AccountID]),
		historyStart: monthOfDay(firstOf(view.sightings[pot.AccountID])), yieldAsked: pot.Stable && !closed,
	}
	for _, account := range view.accounts {
		if account.ID == pot.AccountID {
			entry.itemID = account.ItemID
		}
	}
	if pot.Stable {
		entry.id = reservePrefix + pot.AccountID + "/" + pot.ReserveID
		if len(pot.History) > 0 {
			entry.historyStart = monthOfDay(pot.History[0].Day)
		}
	} else {
		slots[pot.AccountID]++
		entry.id = slotPrefix + pot.AccountID + "/" + strconv.Itoa(slots[pot.AccountID])
	}
	if !closed && pot.Balance != nil {
		entry.balance = *pot.Balance
	}
	if pot.CDIPercent != nil {
		entry.cdiPercent = percentPointer(fmt.Sprintf("%.2f", *pot.CDIPercent))
	}
	entry.yield = potrules.MonthYield{AccountID: pot.AccountID, ReserveID: pot.ReserveID, Base: entry.base}
	return entry
}

func firstOf(days []brday.Day) brday.Day {
	if len(days) == 0 {
		return ""
	}
	return days[0]
}

func monthOfDay(day brday.Day) brday.Month {
	if len(day) < 7 {
		return ""
	}
	return brday.Month(day[:7])
}

// baseOfBank says how a bank reports a yield: Nubank net of tax and Mercado
// Pago gross. Any other bank has no base, and its figures are not mixed with
// either.
func baseOfBank(bank string) potrules.Base {
	switch bank {
	case bankNubank:
		return potrules.BaseNet
	case bankMercadoPago:
		return potrules.BaseGross
	}
	return potrules.BaseUnknown
}

// attachMovements gives each pot the deposits and withdrawals the rules can
// place on it, and keeps the rest on their account.
func (view *potsView) attachMovements(ctx context.Context, snapshot *ledger.View, around ledgerContext,
	sightings []potrules.Sighting, reserves []potEntry) error {
	rows, err := snapshot.CleanTransactions(ctx, wholeLedger)
	if err != nil {
		return err
	}
	result := txrules.Interpret(rows, around.reference)
	if err := checkPlaced(result.Tally, len(rows)); err != nil {
		return err
	}
	attribution := potrules.AttributeMovements(sightings, reserveMovementsOf(result.PotMovements))
	byID := map[string]*potEntry{}
	for index := range reserves {
		byID[reserves[index].id] = &reserves[index]
	}
	view.unattributed = map[string][]potMovementWire{}
	for _, placed := range attribution.Attributed {
		entry := byID[reservePrefix+placed.Pot.AccountID+"/"+placed.Pot.ReserveID]
		if entry == nil {
			view.keepOnAccount(placed.Movement)
			continue
		}
		entry.movements = append(entry.movements, movementWireOf(placed.Movement))
	}
	for _, movement := range attribution.Unattributed {
		view.keepOnAccount(movement)
	}
	return nil
}

func (view *potsView) keepOnAccount(movement potrules.Movement) {
	view.unattributed[movement.AccountID] = append(view.unattributed[movement.AccountID], movementWireOf(movement))
}

// reserveMovementsOf are the transfers into and out of pots. A deposit is money
// out of the account and raises the pot by the same amount. A yield row moves
// no money into or out of a pot, so it is not a deposit or a withdrawal.
func reserveMovementsOf(byAccount map[string][]txrules.PotMovement) []potrules.Movement {
	var movements []potrules.Movement
	for accountID, own := range byAccount {
		for _, movement := range own {
			if !movement.Yield {
				movements = append(movements, potrules.Movement{AccountID: accountID, Day: movement.Day, Change: -movement.Amount})
			}
		}
	}
	sort.SliceStable(movements, func(a, b int) bool {
		if movements[a].Day != movements[b].Day {
			return movements[a].Day > movements[b].Day
		}
		if movements[a].AccountID != movements[b].AccountID {
			return movements[a].AccountID < movements[b].AccountID
		}
		return movements[a].Change < movements[b].Change
	})
	return movements
}

func movementWireOf(movement potrules.Movement) potMovementWire {
	description := potDepositDescription
	if movement.Change < 0 {
		description = potWithdrawalDescription
	}
	return potMovementWire{Day: string(movement.Day), Description: description, Amount: Reais(movement.Change)}
}

func sortedEntries(entries []potEntry) []potEntry {
	sort.SliceStable(entries, func(a, b int) bool {
		left, right := entries[a], entries[b]
		if left.bank != right.bank {
			return left.bank < right.bank
		}
		if nameOrEmpty(left.name) != nameOrEmpty(right.name) {
			return nameOrEmpty(left.name) < nameOrEmpty(right.name)
		}
		return left.id < right.id
	})
	return entries
}

func nameOrEmpty(name *string) string {
	if name == nil {
		return ""
	}
	return *name
}

// namedOrNil is the name a pot was given, or nil for one nobody named.
func namedOrNil(name *string) *string {
	if name == nil || *name == "" {
		return nil
	}
	return name
}
