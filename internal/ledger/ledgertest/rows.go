package ledgertest

import "time"

// Item is a bank connection.
type Item struct {
	ID               string
	Connector        string
	Status           string
	ExecutionStatus  string
	LastUpdated      time.Time
	NextAutoSync     time.Time
	ConsentExpiresAt *time.Time
}

// AddItem adds a present, unambiguous item.
func (f *Fixture) AddItem(item Item) {
	f.t.Helper()
	body := map[string]any{
		"id": item.ID, "connector": map[string]any{"name": item.Connector},
		"status": item.Status, "executionStatus": item.ExecutionStatus,
		"lastUpdatedAt": formatInstant(item.LastUpdated), "nextAutoSyncAt": formatInstant(item.NextAutoSync),
	}
	if item.ConsentExpiresAt != nil {
		body["consentExpiresAt"] = formatInstant(*item.ConsentExpiresAt)
	}
	f.put(document{
		kind: "items", id: item.ID, parent: item.ID, body: body, table: "items",
		columns: []string{"connector", "status", "execution_status", "last_updated", "next_auto_sync"},
		values: []any{item.Connector, item.Status, item.ExecutionStatus,
			formatInstant(item.LastUpdated), formatInstant(item.NextAutoSync)},
	})
}

// Consent is a row of the consents table.
type Consent struct {
	ID        string
	ItemID    string
	ExpiresAt *string
}

// AddConsent adds a consent whose own expires_at is as given, null by default.
func (f *Fixture) AddConsent(consent Consent) {
	f.t.Helper()
	body := map[string]any{"id": consent.ID}
	var expires any
	if consent.ExpiresAt != nil {
		body["expiresAt"], expires = *consent.ExpiresAt, *consent.ExpiresAt
	}
	f.put(document{
		kind: "consents", id: consent.ID, parent: consent.ItemID, body: body, table: "consents",
		columns: []string{"item_id", "expires_at", "revoked_at"}, values: []any{consent.ItemID, expires, nil},
	})
}

// Identity is a holder's identity record; the values are invented by the caller.
type Identity struct {
	ID       string
	ItemID   string
	Document string
	FullName string
}

// AddIdentity adds a present identity.
func (f *Fixture) AddIdentity(identity Identity) {
	f.t.Helper()
	body := map[string]any{"id": identity.ID, "document": identity.Document, "fullName": identity.FullName}
	f.put(document{
		kind: "identities", id: identity.ID, parent: identity.ItemID, body: body, table: "identities",
		columns: []string{"item_id", "document", "full_name"},
		values:  []any{identity.ItemID, identity.Document, identity.FullName},
	})
}

// Bill is a card bill; its dates are YYYY-MM-DD and are stored as midnight UTC.
type Bill struct {
	ID          string
	AccountID   string
	DueDate     string
	ClosingDate string
	Total       float64
	Minimum     *float64
}

// AddBill adds a present bill.
func (f *Fixture) AddBill(bill Bill) {
	f.t.Helper()
	body := map[string]any{
		"id": bill.ID, "dueDate": bill.DueDate + "T00:00:00.000Z",
		"billClosingDate": bill.ClosingDate + "T00:00:00.000Z", "totalAmount": bill.Total,
	}
	var minimum any
	if bill.Minimum != nil {
		body["minimumPaymentAmount"], minimum = *bill.Minimum, *bill.Minimum
	}
	f.put(document{
		kind: "bills", id: bill.ID, parent: bill.AccountID, body: body, table: "bills",
		columns: []string{"account_id", "due_date", "closing_date", "total_amount", "minimum_payment"},
		values: []any{bill.AccountID, bill.DueDate + "T00:00:00.000Z",
			bill.ClosingDate + "T00:00:00.000Z", bill.Total, minimum},
	})
}

// Investment is a current investment. Raw is merged over the defaults.
type Investment struct {
	ID       string
	ItemID   string
	Type     string
	Subtype  string
	Name     string
	Currency string
	Balance  float64
	Raw      map[string]any
}

func investmentBody(investment Investment) map[string]any {
	return merge(map[string]any{
		"id": investment.ID, "itemId": investment.ItemID, "type": investment.Type,
		"subtype": investment.Subtype, "name": investment.Name, "currencyCode": investment.Currency,
		"balance": investment.Balance, "amount": investment.Balance, "amountOriginal": investment.Balance,
		"amountWithdrawal": investment.Balance, "taxes": 0, "taxes2": 0, "rate": 100,
		"rateType": "CDI_PERCENTAGE", "status": "ACTIVE", "dueDate": "2027-01-15T00:00:00.000Z",
		"date": "2026-03-15T00:00:00.000Z", "updatedAt": "2026-03-15T02:00:00.000Z",
	}, investment.Raw)
}

// AddInvestment adds a present investment observed on DefaultDay.
func (f *Fixture) AddInvestment(investment Investment) {
	f.t.Helper()
	f.put(document{
		kind: "investments", id: investment.ID, parent: investment.ItemID,
		body: investmentBody(investment), table: "investments",
		columns: []string{"item_id", "type", "subtype", "name", "balance", "currency"},
		values: []any{investment.ItemID, investment.Type, investment.Subtype, investment.Name,
			investment.Balance, investment.Currency},
	})
}

// ObserveInvestment records a further sighting on another day, with overrides
// merged over the investment's body. It adds history only: the current-state row
// is not touched.
func (f *Fixture) ObserveInvestment(id, day string, overrides map[string]any) {
	f.t.Helper()
	f.observeAgain("investments", id, day, overrides)
}

func (f *Fixture) observeAgain(kind, id, day string, overrides map[string]any) {
	f.t.Helper()
	base, ok := f.bodies[kind+"/"+id]
	if !ok {
		f.t.Fatalf("ledgertest: %s %s was never added", kind, id)
	}
	var parent string
	if err := f.db.QueryRow(`SELECT ds.parent FROM document_state ds JOIN documents d ON d.id = ds.document_id
	        WHERE d.kind = ? AND d.provider_id = ?`, kind, id).Scan(&parent); err != nil {
		f.t.Fatalf("ledgertest: parent lookup: %v", err)
	}
	f.observe(kind, id, parent, day, merge(base, overrides))
}

// CorruptTransactionDate overwrites the date column of a transaction, to show
// what a reader does with a value the collector should never have written. The
// collector's views still see the original body.
func (f *Fixture) CorruptTransactionDate(id, date string) {
	f.t.Helper()
	f.exec("UPDATE transactions SET date = ? WHERE id = ?", date, id)
}

// MarkRemoved turns a current-state row into one the collector no longer sees:
// state 'removed', both in the projection and in the document state.
func (f *Fixture) MarkRemoved(table, id string) {
	f.t.Helper()
	f.exec("UPDATE "+table+" SET state = 'removed' WHERE id = ?", id)
	f.exec(`UPDATE document_state SET state = 'removed' WHERE document_id =
	        (SELECT id FROM documents WHERE kind = ? AND provider_id = ?)`, table, id)
}
