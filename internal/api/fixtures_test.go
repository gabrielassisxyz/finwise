package api

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// The two ledgers every read handler is checked against: one holding every shape
// the rules know, one holding nothing. Everything in them is invented. A handler
// added later reuses them, and its golden files come from the same data.

const (
	accountItau  = "acc-itau"
	accountCard  = "card-itau"
	accountPots  = "acc-pots"
	accountStale = "acc-stale"
	accountOther = "acc-other"

	txSalary         = "tx-salary"
	txPixOut         = "tx-pix-out"
	txOwnTransfer    = "tx-own-transfer"
	txBoleto         = "tx-boleto"
	txBankBill       = "tx-bank-bill"
	txFee            = "tx-fee"
	txLastOfFebruary = "tx-last-of-february"
	txFirstOfMarch   = "tx-first-of-march"
	txPotTransfer    = "tx-pot-transfer"
	txStale          = "tx-stale"
	txInstalment     = "tx-instalment"
	txForeign        = "tx-foreign"
	txForeignTax     = "tx-foreign-tax"
	txMonthEnd       = "tx-month-end"
	txMonthEndTax    = "tx-month-end-tax"
	txPending        = "tx-pending"
	txRefund         = "tx-refund"
	txCardPayment    = "tx-card-payment"
	txUnconverted    = "tx-unconverted"
)

// fixtureNow is the server's clock in the fixture ledgers: noon of 20 March
// 2026 in Brazil.
var fixtureNow = time.Date(2026, time.March, 20, 15, 0, 0, 0, time.UTC)

func utc(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
}

// newEmptyLedger is a collected ledger with no account and no transaction.
func newEmptyLedger(t testing.TB) *ledgertest.Fixture {
	t.Helper()
	return ledgertest.New(t)
}

// newFullLedger holds a bank account and a card of one bank, a bank with a pot,
// an account that has been quiet for months, one no signal places, a foreign
// purchase and its tax, a tax whose purchase is in the previous month, a
// transfer between own accounts, a pot movement, a pending row, and a
// connection in trouble.
func newFullLedger(t testing.TB) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	addFixtureConnections(f)
	addFixtureAccounts(f)
	walkReachesBack(t, f, accountPots)
	addFixtureBankRows(f)
	addFixtureCardRows(f)
	addFixtureSavings(t, f)
	return f
}

func addFixtureConnections(f *ledgertest.Fixture) {
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-itau", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddItem(ledgertest.Item{ID: "item-pots", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddItem(ledgertest.Item{ID: "item-stale", Connector: "Pluggy", Status: "OUTDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddIdentity(ledgertest.Identity{ID: "identity-1", ItemID: "item-itau", Document: "987.654.321-00", FullName: "Pessoa Exemplo"})
}

func addFixtureAccounts(f *ledgertest.Fixture) {
	f.AddAccount(ledgertest.Account{ID: accountItau, ItemID: "item-itau", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Corrente", Currency: "BRL", Balance: 1234.56, TransferNumber: "341/0001/10000-1"})
	f.AddAccount(ledgertest.Account{ID: accountCard, ItemID: "item-itau", Type: "CREDIT", Subtype: "CREDIT_CARD",
		Name: "Platinum Visa", Currency: "BRL", Balance: 1500,
		Credit: map[string]any{
			"creditLimit": 5000, "availableCreditLimit": 3500,
			"balanceCloseDate": "2026-03-28T00:00:00.000Z", "balanceDueDate": "2026-04-05T00:00:00.000Z",
		}})
	f.AddAccount(ledgertest.Account{ID: accountPots, ItemID: "item-pots", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Mercado Pago", Currency: "BRL", Balance: 300.1, TransferNumber: "323/0001/20000-2",
		Reserves: []ledgertest.Reserve{{ID: "pot-1", Name: "Caixinha da casa", Amount: 800, CDIRatio: 1.1}}})
	f.AddAccount(ledgertest.Account{ID: accountStale, ItemID: "item-stale", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta C6", Currency: "BRL", Balance: 25})
	f.AddAccount(ledgertest.Account{ID: accountOther, ItemID: "item-other", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Digital", Currency: "BRL", Balance: 0})
	f.AddBill(ledgertest.Bill{ID: "bill-mar", AccountID: accountCard, DueDate: "2026-04-05", ClosingDate: "2026-03-28", Total: 1500})
}

func party(name, document string) map[string]any {
	return map[string]any{"name": name, "documentNumber": map[string]any{"type": "CNPJ", "value": document}}
}

func payment(method string, payer, receiver map[string]any) map[string]any {
	return map[string]any{"paymentMethod": method, "payer": payer, "receiver": receiver}
}

func addFixtureBankRows(f *ledgertest.Fixture) {
	row := func(id, account string, at time.Time, description string, amount float64, kind string, raw map[string]any) {
		f.AddTransaction(ledgertest.Transaction{ID: id, AccountID: account, Date: at, Description: description,
			Amount: amount, Currency: "BRL", Type: kind, Raw: raw})
	}
	pix := func(operation string, body map[string]any) map[string]any {
		body["operationType"] = operation
		return body
	}
	row(txSalary, accountItau, utc(2026, time.March, 5, 12, 0), "PIX RECEBIDO EMPRESA EXEMPLO", 5000, "CREDIT",
		pix("PIX", map[string]any{"paymentData": payment("PIX", party("Empresa Exemplo Ltda", "12345678000199"), nil)}))
	row(txPixOut, accountItau, utc(2026, time.March, 12, 12, 0), "PIX ENVIADO MARIA EXEMPLO", -80, "DEBIT",
		pix("PIX", map[string]any{"paymentData": payment("PIX", nil, party("Maria Exemplo", "11122233344"))}))
	row(txOwnTransfer, accountItau, utc(2026, time.March, 8, 12, 0), "PIX ENVIADO PESSOA EXEMPLO", -300, "DEBIT",
		pix("PIX", map[string]any{"paymentData": payment("PIX", nil, party("Pessoa Exemplo", "98765432100"))}))
	row(txBoleto, accountItau, utc(2026, time.March, 10, 12, 0), "ALVORADA TELECOM S.A.", -109.9, "DEBIT",
		pix("BOLETO", map[string]any{"paymentData": payment("BOLETO", nil, party("Alvorada Telecom S.A.", "55566677000188"))}))
	row(txBankBill, accountItau, utc(2026, time.March, 18, 12, 0), "PAGAMENTO DE FATURA CARTAO", -1500, "DEBIT",
		map[string]any{"operationType": "OUTROS"})
	row(txFee, accountItau, utc(2026, time.March, 3, 12, 0), "TAR PACOTE SERVICOS", -25, "DEBIT",
		map[string]any{"operationType": "PACOTE_TARIFA_SERVICOS"})
	row(txLastOfFebruary, accountItau, utc(2026, time.March, 1, 2, 30), "PADARIA EXEMPLO", -42, "DEBIT",
		map[string]any{"operationType": "CARTAO"})
	row(txFirstOfMarch, accountItau, utc(2026, time.March, 1, 3, 0), "PIX ENVIADO CAFETERIA EXEMPLO", -10, "DEBIT",
		pix("PIX", map[string]any{"paymentData": payment("PIX", nil, party("Cafeteria Exemplo", "22233344000155"))}))
	// Seen the night it happened, as the collector sees it: a daily yield counts a
	// transfer only from the sighting that knows of it.
	f.AddTransaction(ledgertest.Transaction{ID: txPotTransfer, AccountID: accountPots, Date: utc(2026, time.March, 9, 12, 0),
		Day: "2026-03-09", Description: "Dinheiro reservado Caixinha da casa", Amount: -200, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "TRANSFERENCIA_SALDO_RESERVADO"}})
	row(txStale, accountStale, utc(2026, time.January, 5, 12, 0), "PIX RECEBIDO AMIGO EXEMPLO", 50, "CREDIT",
		pix("PIX", map[string]any{"paymentData": payment("PIX", party("Amigo Exemplo", "44455566677"), nil)}))
}

func addFixtureCardRows(f *ledgertest.Fixture) {
	card := func(id string, at time.Time, description string, amount float64, kind, operation string, extra map[string]any) {
		metadata := map[string]any{"cardNumber": "1111222233331234"}
		raw := map[string]any{"operationType": operation, "creditCardMetadata": metadata}
		for key, value := range extra {
			metadata[key] = value
		}
		f.AddTransaction(ledgertest.Transaction{ID: id, AccountID: accountCard, Date: at, Description: description,
			Amount: amount, Currency: "BRL", Type: kind, Raw: raw})
	}
	card(txInstalment, utc(2026, time.March, 6, 15, 0), "LIVRARIA EXEMPLO 1/3", 129, "DEBIT", "PAGAMENTO",
		map[string]any{"billId": "bill-mar", "installmentNumber": 1, "totalInstallments": 3})
	addForeignPurchase(f)
	card(txMonthEnd, utc(2026, time.February, 28, 15, 0), "MERCADO EXEMPLO", 200, "DEBIT", "PAGAMENTO", nil)
	card(txMonthEndTax, utc(2026, time.March, 1, 15, 0), "MERCADO EXEMPLO", 7, "DEBIT", "OUTROS", nil)
	card(txRefund, utc(2026, time.March, 12, 15, 0), "ESTORNO LIVRARIA EXEMPLO", 30, "CREDIT", "ESTORNO", nil)
	card(txCardPayment, utc(2026, time.March, 18, 15, 0), "PAGAMENTO COM SALDO", 1500, "CREDIT", "PAGAMENTO_FATURA", nil)
	f.AddTransaction(ledgertest.Transaction{ID: txPending, AccountID: accountCard, Date: utc(2026, time.March, 19, 14, 0),
		Description: "PADARIA EXEMPLO", Amount: 18.5, Currency: "BRL", Type: "DEBIT", Status: "PENDING",
		Raw: map[string]any{"operationType": "PAGAMENTO", "creditCardMetadata": map[string]any{"cardNumber": "1111222233331234"}}})
}

// addForeignPurchase adds a purchase in dollars whose value in reais the bank
// states, its explicit tax row on the same day, and a purchase in euros for
// which the bank states no value in reais.
func addForeignPurchase(f *ledgertest.Fixture) {
	metadata := map[string]any{"cardNumber": "1111222233331234"}
	f.AddTransaction(ledgertest.Transaction{ID: txForeign, AccountID: accountCard, Date: utc(2026, time.March, 10, 15, 0),
		Description: "CLOUDSVC SUBSCRIPTION", Amount: 20, Currency: "USD", Type: "DEBIT",
		Raw: map[string]any{"operationType": "PAGAMENTO", "amountInAccountCurrency": 110, "creditCardMetadata": metadata}})
	f.AddTransaction(ledgertest.Transaction{ID: txForeignTax, AccountID: accountCard, Date: utc(2026, time.March, 10, 15, 0),
		Description: "IOF INTERNACIONAL", Amount: 3.85, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "OUTROS", "creditCardMetadata": metadata}})
	f.AddTransaction(ledgertest.Transaction{ID: txUnconverted, AccountID: accountCard, Date: utc(2026, time.March, 14, 15, 0),
		Description: "HOTELSVC EXEMPLO", Amount: 15, Currency: "EUR", Type: "DEBIT",
		Raw: map[string]any{"operationType": "PAGAMENTO", "creditCardMetadata": metadata}})
}

// ledgerNamed builds one of the two fixture ledgers by the name its golden
// files are stored under.
func ledgerNamed(t testing.TB, name string) *ledgertest.Fixture {
	t.Helper()
	switch name {
	case "full":
		return newFullLedger(t)
	case "empty":
		return newEmptyLedger(t)
	}
	t.Fatalf("no fixture ledger called %q", name)
	return nil
}

// inventedLabel is the card label of the invented authored state.
const inventedLabel = "Azulão"

// inventedSettings sets every setting to a value other than its default.
var inventedSettings = authoredtest.Settings{
	ConfidenceThreshold: authoredtest.Some[int64](70), MinExamples: authoredtest.Some[int64](12),
	CardLabel: authoredtest.Some(inventedLabel), ClosingMode: authoredtest.Some(authored.ClosingManual),
	ManualClosingDay: authoredtest.Some[int64](10),
}

// inventedAuthoredState is what a person who has used the app for a while
// would have authored, written through the recorder in one change: a fact, a
// created row, a tag and a parent with a child in the scratch tables, every
// setting at a value other than its default, and a budget. Nothing of it is
// real. A test that builds on it checks that it leaves no journaled table
// empty, so a state that silently holds nothing, or misses a table a later
// piece of work adds, cannot make a comparison pass.
func inventedAuthoredState(t testing.TB, scratch *authoredtest.Scratch) {
	t.Helper()
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "scratch.seed", At: fixtureNow}
	err := scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		ctx := t.Context()
		if err := recorder.Set(ctx, authoredtest.Facts, []authored.Value{authored.Text("fact-seed")}, authored.Row{"amount": authored.Integer(1200)}); err != nil {
			return err
		}
		if err := recorder.Create(ctx, authoredtest.Things, authored.Row{"id": authored.Integer(1), "name": authored.Text("nota-seed"), "amount": authored.Integer(350)}); err != nil {
			return err
		}
		if err := recorder.Put(ctx, authoredtest.Tags, authored.Row{"tx": authored.Text("tx-fee"), "tag": authored.Text("tag-seed")}); err != nil {
			return err
		}
		if err := recorder.Put(ctx, authoredtest.Parents, authored.Row{"id": authored.Integer(1), "name": authored.Text("nota-parent")}); err != nil {
			return err
		}
		if err := recorder.Put(ctx, authoredtest.Children, authored.Row{"id": authored.Integer(1), "parent_id": authored.Integer(1), "label": authored.Text("nota-child")}); err != nil {
			return err
		}
		if err := recorder.SetConfidenceThreshold(ctx, *inventedSettings.ConfidenceThreshold); err != nil {
			return err
		}
		if err := recorder.SetMinExamples(ctx, *inventedSettings.MinExamples); err != nil {
			return err
		}
		if err := recorder.SetCardLabel(ctx, *inventedSettings.CardLabel); err != nil {
			return err
		}
		if err := recorder.SetCardClosing(ctx, *inventedSettings.ClosingMode, *inventedSettings.ManualClosingDay); err != nil {
			return err
		}
		if err := authoredtest.SeedBudget(recorder); err != nil {
			return err
		}
		if err := authoredtest.SeedTransactions(recorder); err != nil {
			return err
		}
		if err := seedPotChoices(ctx, recorder); err != nil {
			return err
		}
		if _, err := authoredtest.SeedSubscription(recorder); err != nil {
			return err
		}
		if err := authoredtest.SeedReviewMarks(recorder); err != nil {
			return err
		}
		return seedPaydayMarks(ctx, recorder)
	})
	if err != nil {
		t.Fatalf("the invented authored state: %v", err)
	}
}

// seedPaydayMarks marks two of the payday transfers of 2026-03 that the seeded
// budget and links derive, so the reads that follow the transfers show one
// done; the third, into the linked investment, stays pending.
func seedPaydayMarks(ctx context.Context, recorder *authored.Recorder) error {
	marks := []struct {
		key string
		day brday.Day
	}{{"pot:reserve:acc-pots/pot-1", "2026-03-18"}, {"investment", "2026-03-19"}}
	for _, mark := range marks {
		if _, err := recorder.MarkTransferDone(ctx, "2026-03", mark.key, mark.day); err != nil {
			return err
		}
	}
	return nil
}

// seedPotChoices places two pots of the full ledger in a bucket other than the
// bank's word, a reserve in Disponível and an investment in Reservado, and
// links a line of the seeded budget to each, in the change that is being
// recorded.
func seedPotChoices(ctx context.Context, recorder *authored.Recorder) error {
	choices := []struct {
		pot, line string
		bucket    authored.PotBucket
	}{
		{"reserve:acc-pots/pot-1", "Remédios", authored.BucketAvailable},
		{"investment:inv-it-1", "Viagens", authored.BucketReserved},
	}
	view := recorder.View()
	defer view.Close()
	budget, err := view.BudgetOf(ctx, "2026-03")
	if err != nil {
		return err
	}
	for _, choice := range choices {
		if err := recorder.SetPotBucket(ctx, choice.pot, choice.bucket); err != nil {
			return err
		}
		index := slices.IndexFunc(budget.Lines, func(line authored.BudgetLine) bool { return line.Name == choice.line })
		if index < 0 {
			return errors.New("the seeded budget has no line to link")
		}
		if err := recorder.ToggleLink(ctx, budget.Lines[index].ID, choice.pot); err != nil {
			return err
		}
	}
	return nil
}
