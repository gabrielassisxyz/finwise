package ledger_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

func idsOf(transactions []ledger.Transaction) []string {
	ids := make([]string, 0, len(transactions))
	for _, transaction := range transactions {
		ids = append(ids, transaction.ID)
	}
	sort.Strings(ids)
	return ids
}

func TestCleanListKeepsOnlyWhatTheCollectorVouchesFor(t *testing.T) {
	f := ledgertest.New(t)
	f.AddItem(ledgertest.Item{ID: "item-1", Connector: "Test Bank", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: instant(t, "2026-03-15T02:00:00Z"), NextAutoSync: instant(t, "2026-03-16T02:00:00Z")})
	f.AddAccount(ledgertest.Account{ID: "acc-1", ItemID: "item-1", Type: "BANK", Name: "Checking", Currency: "BRL"})
	f.AddAccount(ledgertest.Account{ID: "acc-late", ItemID: "item-1", Type: "BANK", Name: "Walked late", Currency: "BRL",
		CoverageStart: "2026-12-01"})

	noon := instant(t, "2026-02-10T12:00:00Z")
	add := func(id, account, status, description string, amount float64) ledgertest.Transaction {
		return ledgertest.Transaction{ID: id, AccountID: account, Date: noon, Description: description,
			Amount: amount, Currency: "BRL", Type: "DEBIT", Status: status}
	}
	rows := []ledgertest.Transaction{
		add("posted", "acc-1", "POSTED", "Plain posted row", -10),
		add("pending-alone", "acc-1", "PENDING", "Pending without a twin", -20),
		add("pending-paired", "acc-1", "PENDING", "Pending with a twin", -30),
		add("posted-twin", "acc-1", "POSTED", "Pending with a twin", -30),
		add("uncovered", "acc-late", "POSTED", "Older than the walk reached", -40),
	}
	removed := add("removed", "acc-1", "POSTED", "No longer in the provider", -50)
	removed.State = "removed"
	ambiguous := add("ambiguous", "acc-1", "POSTED", "Seen under two parents", -60)
	ambiguous.Ambiguous = true
	rows = append(rows, removed, ambiguous)
	for _, row := range rows {
		f.AddTransaction(row)
	}

	got, err := viewOf(t, f.Path).CleanTransactions(t.Context(), february2026(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pending-alone", "posted", "posted-twin"}
	if !reflect.DeepEqual(idsOf(got), want) {
		t.Fatalf("clean list = %v, want %v", idsOf(got), want)
	}
}

func TestPendingRowWhosePostedTwinIsOnAnotherDayStaysListed(t *testing.T) {
	f := ledgertest.New(t)
	f.AddAccount(ledgertest.Account{ID: "acc-1", ItemID: "item-1", Type: "CREDIT", Name: "Card", Currency: "BRL"})
	for _, row := range []ledgertest.Transaction{
		{ID: "pending", AccountID: "acc-1", Date: instant(t, "2026-02-10T12:00:00Z"), Description: "Same text", Amount: 5, Currency: "BRL", Type: "DEBIT", Status: "PENDING"},
		{ID: "posted", AccountID: "acc-1", Date: instant(t, "2026-02-12T12:00:00Z"), Description: "Same text", Amount: 5, Currency: "BRL", Type: "DEBIT"},
	} {
		f.AddTransaction(row)
	}

	got, err := viewOf(t, f.Path).CleanTransactions(t.Context(), february2026(t))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"pending", "posted"}; !reflect.DeepEqual(idsOf(got), want) {
		t.Fatalf("clean list = %v, want %v", idsOf(got), want)
	}
}

func TestInstantAfterMidnightUTCBelongsToTheLastDayOfThePreviousMonth(t *testing.T) {
	f := ledgertest.New(t)
	f.AddAccount(ledgertest.Account{ID: "acc-1", ItemID: "item-1", Type: "BANK", Name: "Checking", Currency: "BRL"})
	f.AddTransaction(ledgertest.Transaction{ID: "edge", AccountID: "acc-1",
		Date: instant(t, "2026-03-01T02:30:00Z"), Description: "Late on the 28th", Amount: -1, Currency: "BRL", Type: "DEBIT"})
	view := viewOf(t, f.Path)

	march, err := brday.MonthPeriod("2026-03")
	if err != nil {
		t.Fatal(err)
	}
	inFebruary, err := view.CleanTransactions(t.Context(), february2026(t))
	if err != nil || len(inFebruary) != 1 || inFebruary[0].Day != "2026-02-28" {
		t.Fatalf("February: %v, %v", inFebruary, err)
	}
	inMarch, err := view.CleanTransactions(t.Context(), march)
	if err != nil || len(inMarch) != 0 {
		t.Fatalf("March: %v, %v", inMarch, err)
	}
}

func TestPeriodBoundsAreHalfOpenAtThreeOClockUTC(t *testing.T) {
	f := ledgertest.New(t)
	f.AddAccount(ledgertest.Account{ID: "acc-1", ItemID: "item-1", Type: "BANK", Name: "Checking", Currency: "BRL"})
	for id, at := range map[string]string{
		"before": "2026-02-01T02:59:59Z", "first": "2026-02-01T03:00:00Z",
		"last": "2026-03-01T02:59:59Z", "after": "2026-03-01T03:00:00Z",
	} {
		f.AddTransaction(ledgertest.Transaction{ID: id, AccountID: "acc-1", Date: instant(t, at),
			Description: id, Amount: -1, Currency: "BRL", Type: "DEBIT"})
	}

	got, err := viewOf(t, f.Path).CleanTransactions(t.Context(), february2026(t))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"first", "last"}; !reflect.DeepEqual(idsOf(got), want) {
		t.Fatalf("February = %v, want %v", idsOf(got), want)
	}
}

func TestCleanListIsNewestFirst(t *testing.T) {
	f := ledgertest.New(t)
	f.AddAccount(ledgertest.Account{ID: "acc-1", ItemID: "item-1", Type: "BANK", Name: "Checking", Currency: "BRL"})
	for _, row := range []struct{ id, at string }{{"b", "2026-02-02T12:00:00Z"}, {"c", "2026-02-20T12:00:00Z"}, {"a", "2026-02-02T12:00:00Z"}} {
		f.AddTransaction(ledgertest.Transaction{ID: row.id, AccountID: "acc-1", Date: instant(t, row.at),
			Description: row.id, Amount: -1, Currency: "BRL", Type: "DEBIT"})
	}

	got, err := viewOf(t, f.Path).CleanTransactions(t.Context(), february2026(t))
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, transaction := range got {
		order = append(order, transaction.ID)
	}
	if want := []string{"c", "a", "b"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestTransactionCarriesEveryRawFieldParsed(t *testing.T) {
	f := ledgertest.New(t)
	f.AddAccount(ledgertest.Account{ID: "acc-1", ItemID: "item-1", Type: "CREDIT", Name: "Card", Currency: "BRL"})
	f.AddTransaction(ledgertest.Transaction{
		ID: "tx-1", AccountID: "acc-1", Date: instant(t, "2026-02-10T12:00:00Z"), Description: "Foreign shop",
		Amount: 10.5, Currency: "USD", Type: "DEBIT", Category: "Shopping",
		Raw: map[string]any{
			"operationType": "OUTROS", "amountInAccountCurrency": 52.57, "descriptionRaw": "FOREIGN SHOP 01",
			"paymentData": map[string]any{
				"paymentMethod": "PIX",
				"payer":         map[string]any{"name": "Test Payer", "documentNumber": map[string]any{"type": "CPF", "value": "11111111111"}},
				"receiver":      map[string]any{"name": "Test Receiver"},
			},
			"creditCardMetadata": map[string]any{
				"cardNumber": "1234", "billId": "bill-9", "installmentNumber": 2, "totalInstallments": 5,
				"purchaseDate": "2026-02-09T10:00:00.000Z", "billForecastDate": "2026-03", "feeType": "FEE", "payeeMCC": 5411,
			},
		},
	})

	got, err := viewOf(t, f.Path).CleanTransactions(t.Context(), february2026(t))
	if err != nil || len(got) != 1 {
		t.Fatalf("%v, %v", got, err)
	}
	tx := got[0]
	if tx.Amount != 1050 || *tx.AmountInAccountCurrency != 5257 || tx.Currency != "USD" || tx.Type != "DEBIT" ||
		*tx.Category != "Shopping" || tx.Status != "POSTED" || *tx.OperationType != "OUTROS" || *tx.DescriptionRaw != "FOREIGN SHOP 01" {
		t.Errorf("scalar fields: %+v", tx)
	}
	if *tx.Payment.Method != "PIX" || *tx.Payment.Payer.Name != "Test Payer" ||
		*tx.Payment.Payer.DocumentNumber != "11111111111" || *tx.Payment.Receiver.Name != "Test Receiver" ||
		tx.Payment.Receiver.DocumentNumber != nil {
		t.Errorf("payment: %+v", tx.Payment)
	}
	card := tx.Card
	if *card.CardNumber != "1234" || *card.BillID != "bill-9" || *card.InstallmentNumber != 2 || *card.TotalInstallments != 5 ||
		!card.PurchaseDate.Equal(instant(t, "2026-02-09T10:00:00Z")) || *card.BillForecastDate != "2026-03" ||
		*card.FeeType != "FEE" || *card.PayeeMCC != 5411 {
		t.Errorf("card metadata: %+v", card)
	}
}

func TestTransactionWithoutOptionalFieldsHasNoneOfThem(t *testing.T) {
	f := ledgertest.New(t)
	f.AddAccount(ledgertest.Account{ID: "acc-1", ItemID: "item-1", Type: "BANK", Name: "Checking", Currency: "BRL"})
	f.AddTransaction(ledgertest.Transaction{ID: "tx-1", AccountID: "acc-1", Date: instant(t, "2026-02-10T12:00:00Z"),
		Description: "Bare", Amount: -1, Currency: "BRL", Type: "DEBIT"})

	got, err := viewOf(t, f.Path).CleanTransactions(t.Context(), february2026(t))
	if err != nil || len(got) != 1 {
		t.Fatalf("%v, %v", got, err)
	}
	tx := got[0]
	if tx.Category != nil || tx.OperationType != nil || tx.AmountInAccountCurrency != nil || tx.Payment != nil || tx.Card != nil {
		t.Errorf("absent fields came back set: %+v", tx)
	}
}

func TestRowWithAnUnreadableDateFailsTheCallInsteadOfVanishing(t *testing.T) {
	f := ledgertest.New(t)
	f.AddAccount(ledgertest.Account{ID: "acc-1", ItemID: "item-1", Type: "BANK", Name: "Checking", Currency: "BRL"})
	f.AddTransaction(ledgertest.Transaction{ID: "tx-1", AccountID: "acc-1", Date: instant(t, "2026-02-10T12:00:00Z"),
		Description: "Fine", Amount: -1, Currency: "BRL", Type: "DEBIT"})
	f.CorruptTransactionDate("tx-1", "yesterday")

	if _, err := viewOf(t, f.Path).CleanTransactions(t.Context(), february2026(t)); err == nil {
		t.Fatal("a row with an unreadable date was dropped without an error")
	}
}

func TestPeriodThatEndsBeforeItStartsIsRefused(t *testing.T) {
	f := populated(t)
	_, err := viewOf(t, f.Path).CleanTransactions(t.Context(), brday.Period{First: "2026-03-02", Last: "2026-03-01"})
	if err == nil {
		t.Fatal("inverted period accepted")
	}
}
