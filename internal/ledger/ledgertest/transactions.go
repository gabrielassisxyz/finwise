package ledgertest

import "time"

// Transaction is a current-state transaction. Raw is merged over the typed
// fields at the top level (operationType, paymentData, creditCardMetadata, ...).
type Transaction struct {
	ID          string
	AccountID   string
	Date        time.Time
	Description string
	Amount      float64
	Currency    string
	Type        string
	// Category is stored as null when empty.
	Category string
	// Status is POSTED when empty.
	Status string
	Raw    map[string]any
	// State is "present" when empty; "removed" and "parent_gone" are also valid.
	State string
	// Ambiguous sets ambiguous_run on the row.
	Ambiguous bool
	// Day is the day the transaction is first observed, DefaultDay when empty.
	Day string
}

// AddTransaction adds a transaction observed on DefaultDay.
func (f *Fixture) AddTransaction(transaction Transaction) {
	f.t.Helper()
	status := transaction.Status
	if status == "" {
		status = "POSTED"
	}
	date := formatInstant(transaction.Date)
	body := merge(map[string]any{
		"id": transaction.ID, "accountId": transaction.AccountID, "date": date,
		"description": transaction.Description, "amount": transaction.Amount,
		"currencyCode": transaction.Currency, "type": transaction.Type, "status": status,
	}, transaction.Raw)
	var category any
	if transaction.Category != "" {
		body["category"], category = transaction.Category, transaction.Category
	}
	f.put(document{
		kind: "transactions", id: transaction.ID, parent: transaction.AccountID, body: body,
		state: transaction.State, ambiguous: transaction.Ambiguous, day: transaction.Day, table: "transactions",
		columns: []string{"account_id", "date", "description", "amount", "currency", "type", "category"},
		values: []any{transaction.AccountID, date, transaction.Description, transaction.Amount,
			transaction.Currency, transaction.Type, category},
	})
}

// ObserveTransaction records a further version of a transaction on another day,
// with overrides merged over its body. It adds history only.
func (f *Fixture) ObserveTransaction(id, day string, overrides map[string]any) {
	f.t.Helper()
	f.observeAgain("transactions", id, day, overrides)
}
