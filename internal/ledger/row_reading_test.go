package ledger_test

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// richLedger holds a card row with every optional field set, a bank row with
// none, and a row in March, so that two periods ask for different rows.
func richLedger(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	f.AddAccount(ledgertest.Account{ID: "acc-card", ItemID: "item-1", Type: "CREDIT", Name: "Card", Currency: "BRL"})
	f.AddAccount(ledgertest.Account{ID: "acc-bank", ItemID: "item-1", Type: "BANK", Name: "Checking", Currency: "BRL"})
	f.AddTransaction(ledgertest.Transaction{
		ID: "tx-rich", AccountID: "acc-card", Date: instant(t, "2026-02-10T12:00:00Z"), Description: "Foreign shop",
		Amount: 10.5, Currency: "USD", Type: "DEBIT", Category: "Shopping",
		Raw: map[string]any{
			"operationType": "OUTROS", "amountInAccountCurrency": 52.57, "descriptionRaw": "FOREIGN SHOP 01",
			"paymentData": map[string]any{
				"paymentMethod": "PIX",
				"payer":         map[string]any{"name": "Payer Example", "documentNumber": map[string]any{"type": "CPF", "value": "11111111111"}},
				"receiver":      map[string]any{"name": "Receiver Example", "documentNumber": map[string]any{"type": "CPF", "value": "22222222222"}},
			},
			"creditCardMetadata": map[string]any{
				"cardNumber": "1234", "billId": "bill-9", "installmentNumber": 2, "totalInstallments": 5,
				"purchaseDate": "2026-02-09T10:00:00.000Z", "billForecastDate": "2026-03", "feeType": "FEE", "payeeMCC": 5411,
			},
		},
	})
	f.AddTransaction(ledgertest.Transaction{ID: "tx-bare", AccountID: "acc-bank", Date: instant(t, "2026-02-11T12:00:00Z"),
		Description: "Bare", Amount: -1, Currency: "BRL", Type: "DEBIT"})
	f.AddTransaction(ledgertest.Transaction{ID: "tx-march", AccountID: "acc-bank", Date: instant(t, "2026-03-05T12:00:00Z"),
		Description: "March", Amount: -2, Currency: "BRL", Type: "DEBIT"})
	return f
}

var everyDay = brday.Period{First: "1970-01-01", Last: "2999-12-31"}

func march2026(t *testing.T) brday.Period {
	t.Helper()
	period, err := brday.MonthPeriod("2026-03")
	if err != nil {
		t.Fatal(err)
	}
	return period
}

// countingReader is a reader of path and the number of times its views have
// read the clean list's rows from the file.
func countingReader(path string) (*ledger.Reader, *atomic.Int64) {
	readings := &atomic.Int64{}
	return ledger.New(path).ObservingRowReadings(func() { readings.Add(1) }), readings
}

// asTheFileHasThem is the clean list of a period on a view opened for it alone.
func asTheFileHasThem(t *testing.T, path string, period brday.Period) []ledger.Transaction {
	t.Helper()
	view := ledger.New(path).View()
	defer view.Close()
	rows, err := view.CleanTransactions(t.Context(), period)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestOneViewReadsTheRowsOnceHoweverManyPeriodsItIsAskedFor(t *testing.T) {
	f := richLedger(t)
	reader, readings := countingReader(f.Path)
	view := reader.View()
	defer view.Close()

	for _, period := range []brday.Period{everyDay, february2026(t), march2026(t), everyDay, february2026(t)} {
		got, err := view.CleanTransactions(t.Context(), period)
		if err != nil {
			t.Fatal(err)
		}
		if want := asTheFileHasThem(t, f.Path, period); !reflect.DeepEqual(got, want) {
			t.Errorf("%v answered %v, want %v", period, idsOf(got), idsOf(want))
		}
	}
	if got := readings.Load(); got != 1 {
		t.Errorf("five calls on one view read the rows %d times, want once", got)
	}
}

func TestEveryViewReadsTheRowsAgain(t *testing.T) {
	f := richLedger(t)
	reader, readings := countingReader(f.Path)
	for range 3 {
		view := reader.View()
		if _, err := view.CleanTransactions(t.Context(), everyDay); err != nil {
			t.Fatal(err)
		}
		if _, err := view.CleanTransactions(t.Context(), february2026(t)); err != nil {
			t.Fatal(err)
		}
		view.Close()
	}
	if got := readings.Load(); got != 3 {
		t.Errorf("three views read the rows %d times, want once each", got)
	}
}

// The replacement is renamed over the file after the first view has read: the
// view keeps answering its own file, and the next view reads the new one.
func TestAViewOpenedAfterTheFileWasReplacedReadsTheNewRows(t *testing.T) {
	f := richLedger(t)
	reader, readings := countingReader(f.Path)
	first := reader.View()
	defer first.Close()
	before, err := first.CleanTransactions(t.Context(), everyDay)
	if err != nil {
		t.Fatal(err)
	}

	replacement := ledgertest.NewAt(t, f.Path+".next")
	replacement.AddAccount(ledgertest.Account{ID: "acc-bank", ItemID: "item-1", Type: "BANK", Name: "Checking", Currency: "BRL"})
	replacement.AddTransaction(ledgertest.Transaction{ID: "tx-new", AccountID: "acc-bank", Date: instant(t, "2026-02-12T12:00:00Z"),
		Description: "New", Amount: -3, Currency: "BRL", Type: "DEBIT"})
	renameOver(t, replacement, f.Path)

	again, err := first.CleanTransactions(t.Context(), everyDay)
	if err != nil || !reflect.DeepEqual(again, before) {
		t.Errorf("the first view answered %v, %v after the rename, want its own %v", idsOf(again), err, idsOf(before))
	}
	later := reader.View()
	defer later.Close()
	after, err := later.CleanTransactions(t.Context(), everyDay)
	if err != nil || len(after) != 1 || after[0].ID != "tx-new" {
		t.Errorf("a view opened after the rename answered %v, %v, want tx-new", idsOf(after), err)
	}
	if got := readings.Load(); got != 2 {
		t.Errorf("two views read the rows %d times, want once each", got)
	}
}

// A caller whose context ends while it reads must not leave the rest of the
// unit of work without rows.
func TestAReadingThatFailedKeepsNothingAndTheNextCallReadsAgain(t *testing.T) {
	f := richLedger(t)
	reader, readings := countingReader(f.Path)
	view := reader.View()
	defer view.Close()
	if _, err := view.Accounts(t.Context()); err != nil {
		t.Fatal(err)
	}
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := view.CleanTransactions(ended, everyDay); err == nil {
		t.Fatal("a reading under an ended context answered rows")
	}

	got, err := view.CleanTransactions(t.Context(), everyDay)
	if want := asTheFileHasThem(t, f.Path, everyDay); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("the call after the failed reading answered %v, %v, want %v", idsOf(got), err, idsOf(want))
	}
	if got := readings.Load(); got != 2 {
		t.Errorf("the view read the rows %d times, want twice: the failed reading and the next", got)
	}
}

func TestAClosedViewAnswersNoneOfTheRowsItRead(t *testing.T) {
	f := richLedger(t)
	reader, readings := countingReader(f.Path)
	view := reader.View()
	if _, err := view.CleanTransactions(t.Context(), everyDay); err != nil {
		t.Fatal(err)
	}
	if err := view.Close(); err != nil {
		t.Fatal(err)
	}
	rows, err := view.CleanTransactions(t.Context(), everyDay)
	if !errors.Is(err, ledger.ErrViewClosed) || len(rows) != 0 {
		t.Errorf("a closed view answered %v, %v, want ErrViewClosed and no row", idsOf(rows), err)
	}
	if got := readings.Load(); got != 1 {
		t.Errorf("the view read the rows %d times, want once", got)
	}
}

// The first caller reorders its rows, rewrites every field and writes through
// every pointer; each later caller of the same view still gets the rows as the
// file has them.
func TestWhatACallerDoesToItsRowsReachesNoOtherCaller(t *testing.T) {
	f := richLedger(t)
	view := viewOf(t, f.Path)
	first, err := view.CleanTransactions(t.Context(), everyDay)
	if err != nil {
		t.Fatal(err)
	}
	if first[len(first)-1].Card == nil || first[len(first)-1].Payment == nil {
		t.Fatalf("the oldest row carries no card or payment data: %+v", first[len(first)-1])
	}
	for index := range first {
		scramble(reflect.ValueOf(&first[index]).Elem())
	}
	first[0], first[len(first)-1] = first[len(first)-1], first[0]
	_ = append(first[:1], ledger.Transaction{ID: "tx-appended"})

	for _, period := range []brday.Period{everyDay, february2026(t)} {
		got, err := view.CleanTransactions(t.Context(), period)
		if want := asTheFileHasThem(t, f.Path, period); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("after the first caller changed its rows, %v answered %+v, %v; want %+v", period, got, err, want)
		}
	}
}

func TestACopiedRowSharesNoMemoryWithItsOriginal(t *testing.T) {
	var original ledger.Transaction
	seed := 0
	fill(t, reflect.ValueOf(&original).Elem(), &seed)

	copied := ledger.CloneTransaction(original)

	if !reflect.DeepEqual(copied, original) {
		t.Fatalf("the copy differs from its original:\n%+v\n%+v", copied, original)
	}
	for _, path := range sharedPointers(reflect.ValueOf(original), reflect.ValueOf(copied), "Transaction") {
		t.Errorf("the copy shares %s with its original", path)
	}
}

var timeType = reflect.TypeFor[time.Time]()

// fill sets every field reachable from value, allocating every pointer, so
// that a field the copy forgets shows up as a difference or a shared pointer.
// A kind it does not know fails the test: a new field of that kind needs the
// copy looked at.
func fill(t *testing.T, value reflect.Value, seed *int) {
	t.Helper()
	*seed++
	switch {
	case value.Type() == timeType:
		value.Set(reflect.ValueOf(time.Date(2026, time.January, *seed, 12, 0, 0, 0, time.UTC)))
	case value.Kind() == reflect.String:
		value.SetString("value-" + string(rune('a'+*seed%26)))
	case value.CanInt():
		value.SetInt(int64(*seed))
	case value.Kind() == reflect.Bool:
		value.SetBool(true)
	case value.Kind() == reflect.Pointer:
		value.Set(reflect.New(value.Type().Elem()))
		fill(t, value.Elem(), seed)
	case value.Kind() == reflect.Struct:
		for index := range value.NumField() {
			fill(t, value.Field(index), seed)
		}
	default:
		t.Fatalf("a row holds a %s, which this test does not know how to fill", value.Type())
	}
}

// scramble changes every value reachable from value, through pointers too.
func scramble(value reflect.Value) {
	switch {
	case value.Type() == timeType:
		value.Set(reflect.ValueOf(value.Interface().(time.Time).Add(time.Hour)))
	case value.Kind() == reflect.String:
		value.SetString(value.String() + "-changed")
	case value.CanInt():
		value.SetInt(value.Int() + 1)
	case value.Kind() == reflect.Bool:
		value.SetBool(!value.Bool())
	case value.Kind() == reflect.Pointer:
		if !value.IsNil() {
			scramble(value.Elem())
		}
	case value.Kind() == reflect.Struct:
		for index := range value.NumField() {
			scramble(value.Field(index))
		}
	}
}

// sharedPointers names every pointer of a that b holds at the same address.
func sharedPointers(a, b reflect.Value, path string) []string {
	switch {
	case a.Type() == timeType:
		return nil
	case a.Kind() == reflect.Pointer:
		if a.IsNil() || b.IsNil() {
			return nil
		}
		if a.Pointer() == b.Pointer() {
			return []string{path}
		}
		return sharedPointers(a.Elem(), b.Elem(), path)
	case a.Kind() == reflect.Struct:
		var shared []string
		for index := range a.NumField() {
			shared = append(shared, sharedPointers(a.Field(index), b.Field(index), path+"."+a.Type().Field(index).Name)...)
		}
		return shared
	}
	return nil
}
