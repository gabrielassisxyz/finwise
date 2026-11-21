package api

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

func transferNumber(text string) *string { return &text }

func TestAnAccountIsPlacedByItsBankCodeByItsNameOrByBoth(t *testing.T) {
	accounts := []ledger.Account{
		{ID: "code", ItemID: "i1", Name: "Conta Corrente", TransferNumber: transferNumber("341/0001/1-1")},
		{ID: "padded", ItemID: "i2", Name: "Conta", TransferNumber: transferNumber("0323/0001/2-2")},
		{ID: "name", ItemID: "i3", Name: "Conta C6 Bank"},
		{ID: "accented", ItemID: "i4", Name: "ITAÚ Mastercard"},
		{ID: "both", ItemID: "i5", Name: "Nubank Conta", TransferNumber: transferNumber("260/0001/3-3")},
		{ID: "punctuated", ItemID: "i6", Name: "Mercado Pago - Conta"},
	}
	labels := bankLabels(accounts)
	want := map[string]string{
		"code": "Itaú", "padded": "Mercado Pago", "name": "C6", "accented": "Itaú", "both": "Nubank", "punctuated": "Mercado Pago",
	}
	for id, bank := range want {
		if labels[id] != bank {
			t.Errorf("%s placed in %q, want %q", id, labels[id], bank)
		}
	}
}

func TestAnAccountNoSignalPlacesTakesTheBankOfItsConnection(t *testing.T) {
	accounts := []ledger.Account{
		{ID: "bank", ItemID: "item", Name: "Conta Corrente", TransferNumber: transferNumber("341/1/1")},
		{ID: "card", ItemID: "item", Name: "Platinum Visa"},
	}
	if got := bankLabels(accounts)["card"]; got != "Itaú" {
		t.Errorf("the card was placed in %q, want its connection's bank", got)
	}
}

func TestAnAccountIsNeverGuessed(t *testing.T) {
	accounts := []ledger.Account{
		{ID: "contradiction", ItemID: "a", Name: "C6 Conta", TransferNumber: transferNumber("341/1/1")},
		{ID: "alone", ItemID: "b", Name: "Conta Digital"},
		{ID: "unknown-code", ItemID: "c", Name: "Conta", TransferNumber: transferNumber("999/1/1")},
		{ID: "mixed-card", ItemID: "d", Name: "Visa"},
		{ID: "mixed-one", ItemID: "d", Name: "Itaú Conta"},
		{ID: "mixed-two", ItemID: "d", Name: "C6 Conta"},
		{ID: "no-item", Name: "Visa"},
		{ID: "two-names", ItemID: "e", Name: "Itaú e Nubank"},
	}
	labels := bankLabels(accounts)
	for _, id := range []string{"contradiction", "alone", "unknown-code", "mixed-card", "no-item", "two-names"} {
		if labels[id] != neutralBankLabel {
			t.Errorf("%s placed in %q, want the neutral label", id, labels[id])
		}
	}
}

func TestAContradictoryAccountDoesNotLendItsBankToItsConnection(t *testing.T) {
	accounts := []ledger.Account{
		{ID: "contradiction", ItemID: "a", Name: "C6 Conta", TransferNumber: transferNumber("341/1/1")},
		{ID: "card", ItemID: "a", Name: "Visa"},
	}
	if got := bankLabels(accounts)["card"]; got != neutralBankLabel {
		t.Errorf("the card was placed in %q", got)
	}
}

func TestAConnectionTakesTheBankOfAllItsAccountsOrNone(t *testing.T) {
	accounts := []ledger.Account{
		{ID: "a1", ItemID: "same", Name: "Itaú Conta"},
		{ID: "a2", ItemID: "same", Name: "Visa"},
		{ID: "b1", ItemID: "split", Name: "Itaú Conta"},
		{ID: "b2", ItemID: "split", Name: "C6 Conta"},
	}
	items := itemBankLabels(accounts, bankLabels(accounts))
	if items["same"] != "Itaú" || items["split"] != neutralBankLabel {
		t.Errorf("items = %v", items)
	}
}
