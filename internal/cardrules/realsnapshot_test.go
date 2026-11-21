package cardrules_test

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/cardrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// The card rules over a real snapshot, opt-in. Only counts are logged: the
// snapshot is a person's financial history and no value from it may reach a log.
func TestCardRulesOverARealSnapshot(t *testing.T) {
	path := os.Getenv("FINWISE_LEDGER_SNAPSHOT")
	if path == "" {
		t.Skip("FINWISE_LEDGER_SNAPSHOT is not set")
	}
	reader := ledger.New(path)
	accounts, bills, transactions := readCardLedger(t, reader)

	var counts snapshotCounts
	for _, account := range accounts {
		if account.Type != ledger.AccountCredit {
			continue
		}
		card := cardrules.Card{Table: tableForTest(account)}
		for _, bill := range bills[account.ID] {
			card.Bills = append(card.Bills, cardBillsOf([]ledger.Bill{bill})...)
		}
		counts.cards++
		counts.add(card, bills[account.ID], transactions[account.ID])
	}
	t.Logf("card_accounts=%d bills=%d purchases=%d", counts.cards, counts.bills, counts.purchases)
	t.Logf("placed by billId=%d; with no billId placed by date=%d (on a predicted bill=%d), unknown=%d",
		counts.byBillID, counts.byDate, counts.byDatePredicted, counts.unknown)
	t.Logf("purchases carrying a billId that the date rule would place on the same bill=%d, on another bill=%d, on none=%d",
		counts.dateAgrees, counts.dateDisagrees, counts.dateUnknown)
	t.Logf("next-bill rule back-tested on %d closed bills: set aside and later billed to the next bill=%d, set aside only=%d, billed to the next bill only=%d",
		counts.backtested, counts.ruleAndBilled, counts.ruleOnly, counts.billedOnly)
	t.Logf("closing dates missing in the ledger=%d, amount on next bill: known=%d unknown=%d, purchases included=%d",
		counts.closingMissing, counts.nextKnown, counts.nextUnknown, counts.nextIncluded)
}

// tableForTest picks the table the way a caller would; the rules do not. The
// account's name is read for that and never logged.
func tableForTest(account ledger.Account) cardrules.ClosingTable {
	if strings.Contains(strings.ToUpper(account.Name), "C6") {
		return cardrules.C6ClosingTable()
	}
	return nil
}

func readCardLedger(t *testing.T, reader *ledger.Reader) ([]ledger.Account, map[string][]ledger.Bill, map[string][]ledger.Transaction) {
	t.Helper()
	accounts, err := reader.Accounts(t.Context())
	if err != nil {
		t.Fatalf("accounts failed (%T); the message is withheld because it may name a real id", err)
	}
	allBills, err := reader.Bills(t.Context())
	if err != nil {
		t.Fatalf("bills failed (%T); the message is withheld because it may name a real id", err)
	}
	all, err := reader.CleanTransactions(t.Context(), brday.Period{First: "1970-01-01", Last: "2999-12-31"})
	if err != nil {
		t.Fatalf("transactions failed (%T); the message is withheld because it may name a real id", err)
	}
	bills, transactions := map[string][]ledger.Bill{}, map[string][]ledger.Transaction{}
	for _, bill := range allBills {
		bills[bill.AccountID] = append(bills[bill.AccountID], bill)
	}
	for _, transaction := range all {
		transactions[transaction.AccountID] = append(transactions[transaction.AccountID], transaction)
	}
	return accounts, bills, transactions
}

type snapshotCounts struct {
	cards, bills, purchases                         int
	byBillID, byDate, byDatePredicted, unknown      int
	dateAgrees, dateDisagrees, dateUnknown          int
	closingMissing                                  int
	nextKnown, nextUnknown, nextIncluded            int
	backtested, ruleAndBilled, ruleOnly, billedOnly int
}

// A purchase here is any DEBIT row of a card account: the rules do not read a
// transaction's kind, and the counts only need every row to be placed once.
func (counts *snapshotCounts) add(card cardrules.Card, bills []ledger.Bill, transactions []ledger.Transaction) {
	counts.bills += len(bills)
	for _, bill := range bills {
		if bill.ClosingDate == nil {
			counts.closingMissing++
		}
	}
	var purchases []cardrules.Purchase
	var latest brday.Day
	for _, transaction := range transactions {
		if transaction.Type != "DEBIT" {
			continue
		}
		purchases = append(purchases, purchaseOf(transaction))
		latest = max(latest, transaction.Day)
	}
	for _, purchase := range purchases {
		counts.purchases++
		counts.place(card, purchase)
	}
	if latest == "" {
		return
	}
	amount := card.AmountOnNextBill(brday.Month(latest[:7]), latest, purchases)
	if !amount.Known {
		counts.nextUnknown++
		return
	}
	counts.nextKnown++
	counts.nextIncluded += len(amount.Included)
	counts.backtest(card, purchases)
}

// backtest runs the next-bill rule on every closed bill that has a successor in
// the ledger and compares what it would have set aside with the purchases the
// provider later put on that successor, in the closing's month.
func (counts *snapshotCounts) backtest(card cardrules.Card, purchases []cardrules.Purchase) {
	ordered := append([]cardrules.Bill(nil), card.Bills...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Due < ordered[j].Due })
	for index := 0; index+1 < len(ordered); index++ {
		closed, successor := ordered[index], ordered[index+1]
		if closed.Closing == nil {
			continue
		}
		month := brday.Month((*closed.Closing)[:7])
		amount := card.AmountOnNextBill(month, *closed.Closing, purchases)
		if !amount.Known {
			continue
		}
		counts.backtested++
		picked := map[string]bool{}
		for _, included := range amount.Included {
			picked[included.ID] = true
		}
		for _, purchase := range purchases {
			onSuccessor := purchase.BillID != nil && *purchase.BillID == successor.ID &&
				strings.HasPrefix(string(purchase.Day), string(month)+"-")
			switch {
			case picked[purchase.ID] && onSuccessor:
				counts.ruleAndBilled++
			case picked[purchase.ID]:
				counts.ruleOnly++
			case onSuccessor:
				counts.billedOnly++
			}
		}
	}
}

func (counts *snapshotCounts) place(card cardrules.Card, purchase cardrules.Purchase) {
	byDate := card.PlacePurchase(cardrules.Purchase{ID: purchase.ID, Day: purchase.Day})
	if purchase.BillID == nil {
		switch {
		case byDate.Source == cardrules.SourceUnknown:
			counts.unknown++
		case byDate.Predicted:
			counts.byDate++
			counts.byDatePredicted++
		default:
			counts.byDate++
		}
		return
	}
	counts.byBillID++
	switch {
	case byDate.Source == cardrules.SourceUnknown:
		counts.dateUnknown++
	case byDate.BillID == *purchase.BillID:
		counts.dateAgrees++
	default:
		counts.dateDisagrees++
	}
}
