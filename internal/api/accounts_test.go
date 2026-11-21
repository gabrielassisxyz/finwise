package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

func accountRowsOf(t *testing.T, env *readEnv) []map[string]any {
	t.Helper()
	status, body := env.answer("accounts.list", `[]`)
	if status != http.StatusOK {
		t.Fatalf("accounts.list answered %d", status)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestAccountsListOverTheFullLedger(t *testing.T) {
	rows := accountRowsOf(t, newReadEnv(t, newFullLedger(t)))
	type expected struct {
		bank, name, kind string
		idle             bool
	}
	want := map[string]expected{
		accountItau:   {"Itaú", "Conta Corrente", "Conta", false},
		accountPots:   {"Mercado Pago", "Mercado Pago", "Conta", false},
		accountStale:  {"C6", "Conta C6", "Conta", true},
		accountOther:  {"Banco não identificado", "Conta Digital", "Conta", true},
		accountNubank: {"Nubank", "Conta Nubank", "Conta", false},
		accountCard:   {"Itaú", "Platinum Visa", "Cartão", false},
	}
	if len(rows) != len(want) {
		t.Fatalf("%d accounts listed, want %d", len(rows), len(want))
	}
	for _, row := range rows {
		id := row["id"].(string)
		got := expected{row["bank"].(string), row["name"].(string), row["kind"].(string), row["idle"].(bool)}
		if got != want[id] {
			t.Errorf("%s listed as %+v, want %+v", id, got, want[id])
		}
	}
}

func TestAccountsComeBeforeCardsThenByBank(t *testing.T) {
	rows := accountRowsOf(t, newReadEnv(t, newFullLedger(t)))
	var order []string
	for _, row := range rows {
		order = append(order, row["id"].(string))
	}
	want := []string{accountOther, accountStale, accountItau, accountPots, accountNubank, accountCard}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("order %v, want %v", order, want)
	}
}

func TestAccountAmountsAreExactDecimals(t *testing.T) {
	_, body := newReadEnv(t, newFullLedger(t)).answer("accounts.list", `[]`)
	for _, literal := range []string{`"amount":1234.56`, `"amount":300.10`, `"amount":25.00`, `"amount":0.00`, `"amount":1500.00`} {
		if !strings.Contains(string(body), literal) {
			t.Errorf("the answer lacks %s", literal)
		}
	}
}

func TestAccountsListOverTheEmptyLedgerIsAnEmptyList(t *testing.T) {
	status, body := newReadEnv(t, newEmptyLedger(t)).answer("accounts.list", `[]`)
	if status != http.StatusOK || string(body) != "[]" {
		t.Errorf("status %d body %s, want 200 and []", status, body)
	}
}

func TestIdleMeansMoreThanThirtyDaysWithoutARow(t *testing.T) {
	today := brday.Day("2026-03-20")
	for _, c := range []struct {
		latest brday.Day
		idle   bool
	}{
		{"2026-03-20", false},
		{"2026-02-18", false},
		{"2026-02-17", true},
		{"", true},
	} {
		if got := isIdle(c.latest, today); got != c.idle {
			t.Errorf("latest %q idle %t, want %t", c.latest, got, c.idle)
		}
	}
}

func TestAccountDetailOfABankAccount(t *testing.T) {
	_, body := newReadEnv(t, newFullLedger(t)).answer("accounts.detail", `["acc-itau"]`)
	got := decodeAnswer(t, body)
	want := map[string]any{
		"id": "acc-itau", "bank": "Itaú", "name": "Conta Corrente", "ledgerAccount": "acc-itau",
		"bucket": "Disponível", "balance": 1234.56,
	}
	for field, value := range want {
		if got[field] != value {
			t.Errorf("%s = %v, want %v", field, got[field], value)
		}
	}
}

func TestAccountDetailIsNullForAnUnknownIdAndForTheCard(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	for _, id := range []string{"no-such-account", accountCard} {
		status, body := env.answer("accounts.detail", `["`+id+`"]`)
		if status != http.StatusOK || string(body) != "null" {
			t.Errorf("%s: status %d body %s, want 200 and null", id, status, body)
		}
	}
}

func TestAccountDetailRefusesArgumentsThatAreNotAnId(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	for _, arguments := range []string{`[5]`, `[null]`, `[{"id":"a"}]`, `[["a"]]`} {
		response := env.call("/api/accounts/detail", arguments, env.cookie)
		expectError(t, arguments, response, http.StatusBadRequest, "bad_request")
	}
}
