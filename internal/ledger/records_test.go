package ledger_test

import (
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

func TestAccountsAreTheCurrentOnesWithTheirBankAndCardFacts(t *testing.T) {
	f := ledgertest.New(t)
	f.AddAccount(ledgertest.Account{ID: "acc-bank", ItemID: "item-1", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Test Checking", Currency: "BRL", Balance: 1234.56, TransferNumber: "077/0001/12345-6"})
	f.AddAccount(ledgertest.Account{ID: "acc-card", ItemID: "item-1", Type: "CREDIT", Subtype: "CREDIT_CARD",
		Name: "Test Card", Currency: "BRL", Balance: 99.99,
		Credit: map[string]any{"creditLimit": 5000, "availableCreditLimit": 4250.5,
			"balanceCloseDate": nil, "balanceDueDate": "2026-03-10"}})
	f.AddAccount(ledgertest.Account{ID: "acc-gone", ItemID: "item-1", Type: "BANK", Name: "Gone", Currency: "BRL"})
	f.MarkRemoved("accounts", "acc-gone")

	got, err := ledger.New(f.Path).Accounts(t.Context())
	if err != nil || len(got) != 2 {
		t.Fatalf("%v, %v", got, err)
	}
	bank, card := got[0], got[1]
	if bank.ID != "acc-bank" || bank.Type != ledger.AccountBank || bank.Balance != 123456 ||
		*bank.TransferNumber != "077/0001/12345-6" || bank.Credit != nil || bank.Name != "Test Checking" {
		t.Errorf("bank: %+v", bank)
	}
	day := "2026-03-10"
	if card.ID != "acc-card" || card.Type != ledger.AccountCredit || card.Balance != 9999 || card.TransferNumber != nil ||
		*card.Credit.CreditLimit != 500000 || *card.Credit.AvailableCreditLimit != 425050 ||
		card.Credit.BalanceCloseDate != nil || string(*card.Credit.BalanceDueDate) != day {
		t.Errorf("card: %+v credit: %+v", card, card.Credit)
	}
}

func TestBillsCarryTheirDatesAndAmounts(t *testing.T) {
	f := ledgertest.New(t)
	minimum := 12.34
	f.AddBill(ledgertest.Bill{ID: "bill-b", AccountID: "acc-card", DueDate: "2026-04-10", ClosingDate: "2026-04-03", Total: 700.1})
	f.AddBill(ledgertest.Bill{ID: "bill-a", AccountID: "acc-card", DueDate: "2026-03-10", ClosingDate: "2026-03-03", Total: 600.05, Minimum: &minimum})
	f.AddBill(ledgertest.Bill{ID: "bill-gone", AccountID: "acc-card", DueDate: "2026-02-10", ClosingDate: "2026-02-03", Total: 1})
	f.MarkRemoved("bills", "bill-gone")

	got, err := ledger.New(f.Path).Bills(t.Context())
	if err != nil || len(got) != 2 {
		t.Fatalf("%v, %v", got, err)
	}
	first, second := got[0], got[1]
	if first.ID != "bill-a" || first.DueDate != "2026-03-10" || *first.ClosingDate != "2026-03-03" ||
		first.TotalAmount != 60005 || *first.MinimumPayment != 1234 || first.AccountID != "acc-card" {
		t.Errorf("first: %+v", first)
	}
	if second.ID != "bill-b" || second.TotalAmount != 70010 || second.MinimumPayment != nil {
		t.Errorf("second: %+v", second)
	}
}

func TestPotsAreTheLatestSightingOnOrBeforeTheDayAndNothingIsCarriedForward(t *testing.T) {
	f := ledgertest.New(t)
	f.AddAccount(ledgertest.Account{ID: "acc-1", ItemID: "item-1", Type: "BANK", Name: "Checking", Currency: "BRL",
		Reserves: []ledgertest.Reserve{
			{ID: "pot-a", Name: "Trip", Amount: 100.10, CDIRatio: 1.05},
			{ID: "pot-b", Name: "Trip", Amount: 50, CDIRatio: 1},
			{Name: "No identity", Amount: 7, CDIRatio: 1},
			{ID: "dup", Name: "Twin one", Amount: 1, CDIRatio: 1},
			{ID: "dup", Name: "Twin two", Amount: 2, CDIRatio: 1},
		}})
	f.AddAccount(ledgertest.Account{ID: "acc-2", ItemID: "item-1", Type: "BANK", Name: "Empty", Currency: "BRL", Reserves: []ledgertest.Reserve{}})
	f.AddAccount(ledgertest.Account{ID: "acc-3", ItemID: "item-1", Type: "BANK", Name: "Silent", Currency: "BRL"})
	f.ObserveAccount("acc-1", "2026-03-20", ledgertest.ReservesOverride(ledgertest.Reserve{ID: "pot-a", Name: "Trip", Amount: 120.20, CDIRatio: 1.05}))
	reader := ledger.New(f.Path)

	onThe15th, err := reader.Pots(t.Context(), "2026-03-15")
	if err != nil {
		t.Fatal(err)
	}
	if len(onThe15th) != 5 {
		t.Fatalf("15th: %d pots, want 5: %+v", len(onThe15th), onThe15th)
	}
	first := onThe15th[0]
	if first.AccountID != "acc-1" || *first.ReserveID != "pot-a" || *first.Name != "Trip" || !first.StableIdentity ||
		*first.Amount != 10010 || first.Day != "2026-03-15" || *first.Currency != "BRL" ||
		first.AmountStatus != "ok" || first.Position != 0 || first.ProviderUpdatedAt == nil {
		t.Errorf("first pot: %+v", first)
	}
	if got := *first.CDIPercentage; got < 104.99 || got > 105.01 {
		t.Errorf("CDI percentage = %v, want 105", got)
	}
	if onThe15th[2].StableIdentity || onThe15th[2].ReserveID != nil {
		t.Errorf("pot without identification reported stable: %+v", onThe15th[2])
	}
	if onThe15th[3].StableIdentity || onThe15th[4].StableIdentity {
		t.Errorf("duplicated identification reported stable: %+v %+v", onThe15th[3], onThe15th[4])
	}

	onThe20th, err := reader.Pots(t.Context(), "2026-03-20")
	if err != nil || len(onThe20th) != 1 || *onThe20th[0].Amount != 12020 || *onThe20th[0].ReserveID != "pot-a" {
		t.Fatalf("20th: %+v, %v", onThe20th, err)
	}
}

func TestPotsBeforeAnySightingAreNone(t *testing.T) {
	f := populated(t)
	got, err := ledger.New(f.Path).Pots(t.Context(), "2026-03-14")
	if err != nil || len(got) != 0 {
		t.Fatalf("%+v, %v", got, err)
	}
}

func TestPotsRefuseAMalformedDay(t *testing.T) {
	f := populated(t)
	if _, err := ledger.New(f.Path).Pots(t.Context(), "15/03/2026"); err == nil {
		t.Fatal("malformed day accepted")
	}
}

func TestInvestmentsAndTheirDailyHistory(t *testing.T) {
	f := ledgertest.New(t)
	f.AddInvestment(ledgertest.Investment{ID: "inv-1", ItemID: "item-1", Type: "FIXED_INCOME", Subtype: "CDB",
		Name: "Test CDB", Currency: "BRL", Balance: 1000.5, Raw: map[string]any{
			"amount": 1010.55, "taxes": 1.5, "taxes2": 0.25, "amountWithdrawal": 1008.8, "rate": 102.5, "status": "ACTIVE"}})
	f.AddInvestment(ledgertest.Investment{ID: "inv-2", ItemID: "item-1", Type: "FIXED_INCOME", Subtype: "CDB", Name: "Other", Currency: "BRL", Balance: 5})
	f.AddInvestment(ledgertest.Investment{ID: "inv-gone", ItemID: "item-1", Type: "FIXED_INCOME", Name: "Gone", Currency: "BRL", Balance: 1})
	f.MarkRemoved("investments", "inv-gone")
	f.ObserveInvestment("inv-1", "2026-03-10", map[string]any{"amount": 1000.1, "taxes": 0, "amountWithdrawal": 1000.1, "status": "PENDING"})
	reader := ledger.New(f.Path)

	investments, err := reader.Investments(t.Context())
	if err != nil || len(investments) != 2 || investments[0].ID != "inv-1" || investments[0].Balance != 100050 ||
		investments[0].Subtype != "CDB" || investments[0].Name != "Test CDB" {
		t.Fatalf("%+v, %v", investments, err)
	}

	history, err := reader.InvestmentHistory(t.Context(), "inv-1")
	if err != nil || len(history) != 2 {
		t.Fatalf("%+v, %v", history, err)
	}
	earlier, later := history[0], history[1]
	if earlier.Day != "2026-03-10" || *earlier.AmountGross != 100010 || *earlier.Taxes != 0 || *earlier.Status != "PENDING" {
		t.Errorf("earlier: %+v", earlier)
	}
	if later.Day != "2026-03-15" || *later.AmountGross != 101055 || *later.Taxes != 150 || *later.Taxes2 != 25 ||
		*later.AmountNet != 100880 || *later.Rate != 102.5 || *later.RateType != "CDI_PERCENTAGE" ||
		string(*later.DueDate) != "2027-01-15" || *later.Status != "ACTIVE" || later.InvestmentID != "inv-1" {
		t.Errorf("later: %+v", later)
	}
}

func TestIdentitiesAreTheCurrentOnes(t *testing.T) {
	f := ledgertest.New(t)
	f.AddIdentity(ledgertest.Identity{ID: "idn-1", ItemID: "item-1", Document: "00000000000", FullName: "Test Person"})
	f.AddIdentity(ledgertest.Identity{ID: "idn-gone", ItemID: "item-1", Document: "99999999999", FullName: "Gone Person"})
	f.MarkRemoved("identities", "idn-gone")

	got, err := ledger.New(f.Path).Identities(t.Context())
	want := []ledger.Identity{{ID: "idn-1", ItemID: "item-1", Document: "00000000000", FullName: "Test Person"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v, %v", got, err)
	}
}

func TestSyncStateReportsTheLastCompleteRunEvenWhenALaterOneIsPartial(t *testing.T) {
	f := ledgertest.New(t)
	f.AddRun(ledgertest.Run{Outcome: "complete", FinishedAt: instant(t, "2026-03-20T06:00:00Z")})
	f.AddRun(ledgertest.Run{Outcome: "partial", FinishedAt: instant(t, "2026-03-21T06:00:00Z")})
	f.AddRun(ledgertest.Run{Outcome: "failed", FinishedAt: instant(t, "2026-03-22T06:00:00Z")})

	state, err := ledger.New(f.Path).SyncState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if state.LastCompleteSync == nil || !state.LastCompleteSync.Equal(instant(t, "2026-03-20T06:00:00Z")) {
		t.Errorf("last complete = %v", state.LastCompleteSync)
	}
	if state.LatestRun == nil || state.LatestRun.Outcome != "failed" || state.LatestRun.ID != 4 {
		t.Errorf("latest run = %+v", state.LatestRun)
	}
}

func TestSyncStateWithoutAnyCompleteRunHasNoLastSync(t *testing.T) {
	f := ledgertest.NewWithFirstRunOutcome(t, "failed")

	state, err := ledger.New(f.Path).SyncState(t.Context())
	if err != nil || state.LastCompleteSync != nil {
		t.Fatalf("%+v, %v", state, err)
	}
}

func TestConsentExpiryComesFromTheItemBodyNotTheConsentsTable(t *testing.T) {
	f := ledgertest.New(t)
	expiry := instant(t, "2026-09-01T12:00:00Z")
	f.AddItem(ledgertest.Item{ID: "item-1", Connector: "Test Bank", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: instant(t, "2026-03-15T02:00:00Z"), NextAutoSync: instant(t, "2026-03-16T02:00:00Z"), ConsentExpiresAt: &expiry})
	f.AddItem(ledgertest.Item{ID: "item-2", Connector: "Other Bank", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: instant(t, "2026-03-15T02:00:00Z"), NextAutoSync: instant(t, "2026-03-16T02:00:00Z")})
	f.AddConsent(ledgertest.Consent{ID: "consent-1", ItemID: "item-1"})

	state, err := ledger.New(f.Path).SyncState(t.Context())
	if err != nil || len(state.Connections) != 2 {
		t.Fatalf("%+v, %v", state, err)
	}
	if got := state.Connections[0].ConsentExpiresAt; got == nil || !got.Equal(expiry) {
		t.Errorf("item-1 expiry = %v, want %v", got, expiry)
	}
	if state.Connections[1].ConsentExpiresAt != nil {
		t.Errorf("item-2 expiry = %v, want none", state.Connections[1].ConsentExpiresAt)
	}
}

func TestConnectionIsInTroubleOnStatusExecutionOrAge(t *testing.T) {
	now := instant(t, "2026-03-20T12:00:00Z")
	fresh := now.Add(-47 * time.Hour)
	stale := now.Add(-49 * time.Hour)
	cases := []struct {
		name       string
		connection ledger.Connection
		want       bool
	}{
		{"healthy", ledger.Connection{Status: "UPDATED", ExecutionStatus: "SUCCESS", LastUpdated: &fresh}, false},
		{"status", ledger.Connection{Status: "LOGIN_ERROR", ExecutionStatus: "SUCCESS", LastUpdated: &fresh}, true},
		{"execution", ledger.Connection{Status: "UPDATED", ExecutionStatus: "ERROR", LastUpdated: &fresh}, true},
		{"older than 48 hours", ledger.Connection{Status: "UPDATED", ExecutionStatus: "SUCCESS", LastUpdated: &stale}, true},
		{"never updated", ledger.Connection{Status: "UPDATED", ExecutionStatus: "SUCCESS"}, true},
	}
	for _, c := range cases {
		if got := c.connection.InTrouble(now); got != c.want {
			t.Errorf("%s: InTrouble = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMoneyArrivesAsCentsWhateverTheFloatsLookLike(t *testing.T) {
	f := ledgertest.New(t)
	f.AddAccount(ledgertest.Account{ID: "acc-1", ItemID: "item-1", Type: "BANK", Name: "Checking", Currency: "BRL", Balance: 19.99})
	f.InTransaction(func() {
		for i := 0; i < 1000; i++ {
			f.AddBill(ledgertest.Bill{ID: "bill-" + strconv.Itoa(i), AccountID: "acc-1", DueDate: "2026-03-10", ClosingDate: "2026-03-03", Total: 0.10})
		}
	})

	accounts, err := ledger.New(f.Path).Accounts(t.Context())
	if err != nil || accounts[0].Balance != 1999 {
		t.Fatalf("%+v, %v", accounts, err)
	}
	bills, err := ledger.New(f.Path).Bills(t.Context())
	if err != nil || len(bills) != 1000 {
		t.Fatalf("%d bills, %v", len(bills), err)
	}
	var total money.Cents
	for _, bill := range bills {
		total += bill.TotalAmount
	}
	if total != 10000 {
		t.Fatalf("sum = %d, want 10000", total)
	}
}
