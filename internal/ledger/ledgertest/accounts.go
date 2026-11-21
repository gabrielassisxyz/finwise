package ledgertest

// Account is a bank or card account. Raw is merged over the typed fields at the
// top level, so a "bankData" or "creditData" in it replaces what the typed
// fields built.
type Account struct {
	ID       string
	ItemID   string
	Type     string
	Subtype  string
	Name     string
	Currency string
	Balance  float64
	// TransferNumber, if set, becomes bankData.transferNumber.
	TransferNumber string
	// Credit, if set, becomes creditData.
	Credit map[string]any
	// Reserves, if not nil, becomes bankData.reservedBalances: an empty slice is
	// an account that reports no pots, nil is one that does not report the key.
	Reserves []Reserve
	Raw      map[string]any
	// CoverageStart is the earliest date the account's last absence-safe walk
	// returned. A transaction or bill dated before it is not covered. Empty
	// means the walk reached back before every date.
	CoverageStart string
}

// Reserve is one pot (caixinha) in an account's reservedBalances.
type Reserve struct {
	// ID is the entry's identification; empty omits it, which makes the entry
	// unstable.
	ID     string
	Name   string
	Amount float64
	// CDIRatio is postFixedIndexerPercentage, 1.1 for 110% of the CDI.
	CDIRatio float64
}

func accountBody(account Account) map[string]any {
	body := map[string]any{
		"id": account.ID, "itemId": account.ItemID, "type": account.Type, "subtype": account.Subtype,
		"name": account.Name, "currencyCode": account.Currency, "balance": account.Balance,
		"updatedAt": "2026-03-15T02:00:00.000Z",
	}
	bankData := map[string]any{}
	if account.TransferNumber != "" {
		bankData["transferNumber"] = account.TransferNumber
	}
	if account.Reserves != nil {
		bankData["reservedBalances"] = reservesBody(account.Reserves)
	}
	if len(bankData) > 0 {
		body["bankData"] = bankData
	}
	if account.Credit != nil {
		body["creditData"] = account.Credit
	}
	return merge(body, account.Raw)
}

func reservesBody(reserves []Reserve) []any {
	entries := make([]any, 0, len(reserves))
	for _, reserve := range reserves {
		entry := map[string]any{
			"name": reserve.Name,
			"availableAmounts": []any{map[string]any{
				"currencyCode": "BRL", "amount": reserve.Amount,
				"remuneration": map[string]any{"indexer": "CDI", "postFixedIndexerPercentage": reserve.CDIRatio},
			}},
		}
		if reserve.ID != "" {
			entry["identification"] = reserve.ID
		}
		entries = append(entries, entry)
	}
	return entries
}

// AddAccount adds a present account observed on DefaultDay. Add the account
// before its transactions and bills, so that its coverage start applies to them.
func (f *Fixture) AddAccount(account Account) {
	f.t.Helper()
	if account.CoverageStart != "" {
		f.coverageStart[account.ID] = account.CoverageStart
	}
	f.put(document{
		kind: "accounts", id: account.ID, parent: account.ItemID, body: accountBody(account), table: "accounts",
		columns: []string{"item_id", "type", "subtype", "name", "balance", "currency"},
		values: []any{account.ItemID, account.Type, account.Subtype, account.Name,
			account.Balance, account.Currency},
	})
}

// ObserveAccount records a further sighting on another day, with overrides
// merged over the account's body (a new "bankData" replaces the old one). It
// adds history only: the current-state row is not touched.
func (f *Fixture) ObserveAccount(id, day string, overrides map[string]any) {
	f.t.Helper()
	f.observeAgain("accounts", id, day, overrides)
}

// ReservesOverride is an ObserveAccount override that replaces the account's
// pots with the given ones.
func ReservesOverride(reserves ...Reserve) map[string]any {
	return map[string]any{"bankData": map[string]any{"reservedBalances": reservesBody(reserves)}}
}
