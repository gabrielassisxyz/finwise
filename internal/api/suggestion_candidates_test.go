package api

import (
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/classrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// The ledger of the suggestion candidates: one purchase at a new establishment,
// which is the only thing a model may be asked about, and every other row the
// step has to leave alone while it too waits for review. Everything is invented.
const (
	suggestBank = "acc-suggest-bank"
	suggestCard = "card-suggest"

	suggestPurchase  = "tx-suggest-purchase"
	suggestCredit    = "tx-suggest-credit"
	suggestOwnMove   = "tx-suggest-own-move"
	suggestBillPay   = "tx-suggest-bill-pay"
	suggestFairOld   = "tx-suggest-fair-old"
	suggestPersonOut = "tx-suggest-person-out"
	suggestPersonRaw = "tx-suggest-person-raw"
	suggestPersonIn  = "tx-suggest-person-in"
)

// newSuggestionLedger holds a bank account whose owner also holds a company,
// so that a PIX to the company's document is a movement between own accounts
// that is not to a person, and a card.
func newSuggestionLedger(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-suggest", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddIdentity(ledgertest.Identity{ID: "identity-suggest", ItemID: "item-suggest", Document: "12.345.678/0001-99", FullName: "Pessoa Exemplo"})
	f.AddAccount(ledgertest.Account{ID: suggestBank, ItemID: "item-suggest", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Corrente", Currency: "BRL", Balance: 900, TransferNumber: "341/0001/40000-4"})
	f.AddAccount(ledgertest.Account{ID: suggestCard, ItemID: "item-suggest", Type: "CREDIT", Subtype: "CREDIT_CARD",
		Name: "Gold Visa", Currency: "BRL", Balance: 400, Credit: map[string]any{"creditLimit": 3000, "availableCreditLimit": 2600}})
	card := func(id string, at time.Time, description string, amount float64) {
		f.AddTransaction(ledgertest.Transaction{ID: id, AccountID: suggestCard, Date: at, Description: description,
			Amount: amount, Currency: "BRL", Type: "DEBIT",
			Raw: map[string]any{"operationType": "PAGAMENTO", "creditCardMetadata": map[string]any{"cardNumber": "5555666677774321"}}})
	}
	pix := func(id string, at time.Time, description string, amount float64, kind string, parties map[string]any) {
		f.AddTransaction(ledgertest.Transaction{ID: id, AccountID: suggestBank, Date: at, Description: description,
			Amount: amount, Currency: "BRL", Type: kind,
			Raw: map[string]any{"operationType": "PIX", "paymentData": parties}})
	}
	card(suggestPurchase, utc(2026, time.March, 18, 15, 0), "LIVRARIA NOVA EXEMPLO", 40)
	card(suggestFairOld, utc(2026, time.March, 2, 15, 0), "FEIRA EXEMPLO", 25)
	pix(suggestCredit, utc(2026, time.March, 17, 12, 0), "PIX RECEBIDO EMPRESA EXEMPLO", 250, "CREDIT",
		payment("PIX", party("Empresa Exemplo Ltda", "55566677000188"), nil))
	pix(suggestOwnMove, utc(2026, time.March, 16, 12, 0), "PIX ENVIADO MINHA EMPRESA", -500, "DEBIT",
		payment("PIX", nil, party("Minha Empresa Exemplo", "12345678000199")))
	f.AddTransaction(ledgertest.Transaction{ID: suggestBillPay, AccountID: suggestBank, Date: utc(2026, time.March, 15, 12, 0),
		Description: "PAGAMENTO DE FATURA CARTAO", Amount: -300, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "OUTROS"}})
	pix(suggestPersonOut, utc(2026, time.March, 14, 12, 0), "PIX ENVIADO JOANA EXEMPLO", -150, "DEBIT",
		payment("PIX", nil, party("Joana Exemplo", "333.444.555-66")))
	pix(suggestPersonRaw, utc(2026, time.March, 13, 12, 0), "PIX ENVIADO RAUL EXEMPLO", -60, "DEBIT",
		payment("PIX", nil, party("Raul Exemplo", "11122233344")))
	pix(suggestPersonIn, utc(2026, time.March, 12, 12, 0), "PIX RECEBIDO AMIGO EXEMPLO", 90, "CREDIT",
		payment("PIX", party("Amigo Exemplo", "44455566677"), nil))
	return f
}

// newSuggestionStores keeps a review as a new establishment on each row of
// queued, and gives the old fair and every transfer to or from a person a line
// by hand.
func newSuggestionStores(t *testing.T, queued ...string) (*authoredtest.Scratch, *ledgertest.Fixture) {
	t.Helper()
	full := newSuggestionLedger(t)
	scratch := authoredtest.Open(t, fixtureClock)
	savedReviewBudget(t, scratch)
	reviews := make([]waitingReview, 0, len(queued))
	for _, id := range queued {
		reviews = append(reviews, waitingReview{id: id, reason: string(classrules.ReasonNewEstablishment)})
	}
	rulesLeave(t, scratch, full.Path, reviews...)
	for _, id := range []string{suggestFairOld, suggestPersonOut, suggestPersonRaw, suggestPersonIn} {
		settledOn(t, scratch, full.Path, "2026-03-19", "transactions.setLine", id, "Mercado")
	}
	return scratch, full
}

func suggestionCandidatesOf(t *testing.T, scratch *authoredtest.Scratch, full *ledgertest.Fixture) SuggestionCandidates {
	t.Helper()
	ledgerView := ledger.New(full.Path).View()
	defer ledgerView.Close()
	authoredView := scratch.Database.View()
	defer authoredView.Close()
	candidates, err := ReadSuggestionCandidates(t.Context(), Views{Ledger: ledgerView, Authored: authoredView})
	if err != nil {
		t.Fatal(err)
	}
	return candidates
}

// keysAsked are the transaction ids of the purchases the model would be asked about.
func keysAsked(t *testing.T, scratch *authoredtest.Scratch, candidates SuggestionCandidates, ids ...string) map[string]bool {
	t.Helper()
	view := scratch.Database.View()
	defer view.Close()
	keys, err := view.TransactionKeys(t.Context(), ids)
	if err != nil {
		t.Fatal(err)
	}
	asked := map[string]bool{}
	for _, month := range candidates.Months {
		for _, purchase := range month.Purchases {
			for id, key := range keys {
				if key == purchase.Key {
					asked[id] = true
				}
			}
		}
	}
	return asked
}

// waitingListed is the listed row of id as the queue holds it, with the
// review's reason, or a failure when nothing waits on it.
func waitingListed(t *testing.T, scratch *authoredtest.Scratch, full *ledgertest.Fixture, id string) (txrules.Transaction, string) {
	t.Helper()
	ledgerView := ledger.New(full.Path).View()
	defer ledgerView.Close()
	authoredView := scratch.Database.View()
	defer authoredView.Close()
	waiting, err := pendingTransactions(t.Context(), Views{Ledger: ledgerView, Authored: authoredView}, wholeLedger.Last)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range waiting {
		if item.listed.ID == id {
			return item.listed, item.state.PendingReason
		}
	}
	t.Fatalf("%s does not wait for review", id)
	return txrules.Transaction{}, ""
}

func TestMoneyInAtAnEstablishmentWaitingForReviewIsNeverAskedAbout(t *testing.T) {
	scratch, full := newSuggestionStores(t, suggestPurchase, suggestCredit)

	credit, reason := waitingListed(t, scratch, full, suggestCredit)
	if credit.Amount <= 0 || credit.Person || credit.Refund || credit.Movement != txrules.NotAMovement ||
		reason != string(classrules.ReasonNewEstablishment) {
		t.Fatalf("the credit is %+v waiting as %q, want money in at an establishment, no refund, waiting as a new establishment", credit, reason)
	}

	asked := keysAsked(t, scratch, suggestionCandidatesOf(t, scratch, full), suggestPurchase, suggestCredit)
	if !asked[suggestPurchase] {
		t.Errorf("the purchase at a new establishment is not a candidate: %v", asked)
	}
	if asked[suggestCredit] {
		t.Errorf("a credit at an establishment is a candidate: %v", asked)
	}
}

func TestAMovementBetweenOwnAccountsWaitingForReviewIsNeverAskedAbout(t *testing.T) {
	scratch, full := newSuggestionStores(t, suggestPurchase, suggestOwnMove, suggestBillPay)

	for id, want := range map[string]txrules.MovementKind{suggestOwnMove: txrules.OwnTransfer, suggestBillPay: txrules.BillPayment} {
		movement, _ := waitingListed(t, scratch, full, id)
		if movement.Amount >= 0 || movement.Person || movement.Movement != want {
			t.Fatalf("%s is %+v, want money out, not to a person, as the movement %q", id, movement, want)
		}
	}

	asked := keysAsked(t, scratch, suggestionCandidatesOf(t, scratch, full), suggestPurchase, suggestOwnMove, suggestBillPay)
	if !asked[suggestPurchase] {
		t.Errorf("the purchase at a new establishment is not a candidate: %v", asked)
	}
	for _, id := range []string{suggestOwnMove, suggestBillPay} {
		if asked[id] {
			t.Errorf("%s, a movement between own accounts, is a candidate: %v", id, asked)
		}
	}
}

// The statement that reads the person's decisions drops a document of eleven
// digits and the check on the listed row drops a person; either alone keeps a
// transfer to a person out of the examples, whatever its document is punctuated
// like, and the transfer from one too.
func TestNoPersonIsEverAnExampleHoweverItsDocumentIsWritten(t *testing.T) {
	scratch, full := newSuggestionStores(t, suggestPurchase)

	for _, id := range []string{suggestPersonOut, suggestPersonRaw, suggestPersonIn} {
		person := ledgerRow(t, full, id)
		if !person.Person {
			t.Fatalf("%s is %+v, want a transfer to or from a person", id, person)
		}
	}

	examples := suggestionCandidatesOf(t, scratch, full).Examples
	var shown []string
	for _, example := range examples {
		shown = append(shown, example.Establishment)
	}
	if want := []string{"FEIRA EXEMPLO"}; !slices.Equal(shown, want) {
		t.Errorf("examples = %v, want only the establishment %v", shown, want)
	}
}

func ledgerRow(t *testing.T, full *ledgertest.Fixture, id string) txrules.Transaction {
	t.Helper()
	view := ledger.New(full.Path).View()
	defer view.Close()
	whole, _, err := listedLedger(t.Context(), view)
	if err != nil {
		t.Fatal(err)
	}
	for _, listed := range whole.listed {
		if listed.ID == id {
			return listed
		}
	}
	t.Fatalf("%s is not listed", id)
	return txrules.Transaction{}
}
