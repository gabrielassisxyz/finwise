package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// The worked example of the payday transfers, from a budget to every read that
// follows them. Its two banks hold two pots named "Reserva", a pot the bank
// gives no name, a pot nothing is linked to, a pot that closed and a pot
// without an identity, beside two open investments and one withdrawn.
// Everything is invented.

const (
	examplePotOne      = "reserve:acc-pots/pot-1"
	examplePotSeven    = "reserve:acc-pots/pot-7"
	exampleNubankPot   = "reserve:acc-nu/pot-nu-1"
	exampleClosedPot   = "reserve:acc-nu/pot-nu-old"
	exampleGonePot     = "reserve:acc-gone/pot-x"
	exampleFreePot     = "reserve:acc-pots/pot-9"
	exampleSlot        = "slot:acc-nu/1"
	exampleInvestment  = "investment:inv-a"
	exampleInvestment2 = "investment:inv-b"
	exampleWithdrawn   = "investment:inv-old"
)

func newPaydayLedger(t testing.TB) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	fresh := utc(2026, time.March, 20, 5, 0)
	for _, item := range []string{"item-pots", "item-nu"} {
		f.AddItem(ledgertest.Item{ID: item, Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
			LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	}
	f.AddAccount(ledgertest.Account{ID: accountPots, ItemID: "item-pots", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Mercado Pago", Currency: "BRL", Balance: 100, TransferNumber: "323/0001/20000-2",
		Reserves: []ledgertest.Reserve{{ID: "pot-1", Name: "Reserva", Amount: 500, CDIRatio: 1}, {ID: "pot-7", Amount: 250, CDIRatio: 1},
			{ID: "pot-9", Name: "Livre", Amount: 70, CDIRatio: 1}}})
	f.ObserveAccount(accountPots, "2026-03-10", sightedAt("2026-03-10", 100,
		ledgertest.Reserve{ID: "pot-1", Name: "Reserva", Amount: 450, CDIRatio: 1}, ledgertest.Reserve{ID: "pot-7", Amount: 200, CDIRatio: 1}))
	f.AddAccount(ledgertest.Account{ID: accountNubank, ItemID: "item-nu", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Nubank", Currency: "BRL", Balance: 200, TransferNumber: "260/0001/30000-3",
		Reserves: []ledgertest.Reserve{{ID: potNubankMain, Name: "Reserva", Amount: 300, CDIRatio: 1}, {Name: "Sem identidade", Amount: 50, CDIRatio: 1}}})
	f.ObserveAccount(accountNubank, "2026-03-13", sightedAt("2026-03-13", 200,
		ledgertest.Reserve{ID: potNubankMain, Name: "Reserva", Amount: 280, CDIRatio: 1},
		ledgertest.Reserve{ID: potNubankOld, Name: "Antiga", Amount: 20, CDIRatio: 1}))
	f.ObserveAccount(accountNubank, "2026-03-14", sightedAt("2026-03-14", 200,
		ledgertest.Reserve{ID: potNubankMain, Name: "Reserva", Amount: 290, CDIRatio: 1},
		ledgertest.Reserve{Name: "Sem identidade", Amount: 50, CDIRatio: 1}))
	for _, investment := range []struct {
		id, name string
		balance  float64
		status   string
	}{{"inv-a", "CDB A", 1000, ""}, {"inv-b", "CDB B", 2000, ""}, {"inv-old", "CDB resgatado", 0, "TOTAL_WITHDRAWAL"}} {
		raw := map[string]any{"amount": investment.balance, "amountOriginal": investment.balance, "taxes": 0, "rate": 100, "rateType": "CDI"}
		if investment.status != "" {
			raw["status"] = investment.status
		}
		f.AddInvestment(ledgertest.Investment{ID: investment.id, ItemID: "item-nu", Type: "FIXED_INCOME", Subtype: "CDB",
			Name: investment.name, Currency: "BRL", Balance: investment.balance, Raw: raw})
	}
	return f
}

// exampleBudget is the example's budget with the salary given.
func exampleBudget(salary money.Cents) authored.Document {
	line := authoredtest.Line
	return authored.Document{
		authoredtest.Group("rec", line("Salário", salary)),
		authoredtest.Group("fix", line("Aluguel", 250_000), line("Internet", 12_000)),
		authoredtest.Group("var", line("Mercado", 90_000), line("Lazer", 30_000)),
		authoredtest.Group("prov", line("IPTU", 15_000), line("Seguro do carro", 9_000), line("Presentes", 6_000),
			line("Revisão do carro", 20_000), line("Material escolar", 0)),
		authoredtest.Group("res", line("Viagem", 40_000), line("Celular novo", 25_000), line("Bicicleta", 10_000)),
	}
}

var exampleLinkedLines = []struct{ line, pot string }{
	{"IPTU", examplePotOne}, {"Seguro do carro", examplePotOne}, {"Presentes", exampleNubankPot},
	{"Viagem", exampleClosedPot}, {"Celular novo", examplePotSeven}, {"Bicicleta", exampleGonePot},
}

// paydayEnv is the example over its own ledger, with the clock on 20 March 2026.
type paydayEnv struct {
	*settingsEnv
	lines map[string]authored.LineID
}

func newPaydayEnv(t *testing.T, salary money.Cents) *paydayEnv {
	t.Helper()
	env := &paydayEnv{settingsEnv: newSettingsEnv(t, newPaydayLedger(t), fixtureNow), lines: map[string]authored.LineID{}}
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-03", exampleBudget(salary))
	for _, group := range exampleBudget(salary) {
		for _, line := range group.Lines {
			env.lines[line.Name] = budgetLineID(t, env.scratch.Database, "2026-03", line.Name)
		}
	}
	for _, link := range exampleLinkedLines {
		authoredtest.ToggleLink(t, env.scratch.Database, fixtureNow, env.lines[link.line], link.pot)
	}
	return env
}

func (env *paydayEnv) lineKey(name string) string { return "line:" + env.lines[name].String() }

// transfer is one transfer of a payday answer, decoded.
type transfer struct {
	Key          string  `json:"key"`
	Name         string  `json:"name"`
	Amount       float64 `json:"amount"`
	State        string  `json:"state"`
	DoneOn       *string `json:"doneOn"`
	MarkedByHand bool    `json:"markedByHand"`
}

func (env *paydayEnv) transfers(month string) []transfer {
	env.t.Helper()
	var transfers []transfer
	if err := json.Unmarshal([]byte(env.fieldOf("review.payday", `["`+month+`"]`, "transfers")), &transfers); err != nil {
		env.t.Fatal(err)
	}
	return transfers
}

// transferMarks is every row of transfer_marks, each as month, key and day.
func transferMarks(t *testing.T, scratch *authoredtest.Scratch) []string {
	t.Helper()
	var tables map[string][]map[string]any
	if err := json.Unmarshal([]byte(scratch.Canonical(t)), &tables); err != nil {
		t.Fatal(err)
	}
	marks := []string{}
	for _, row := range tables["transfer_marks"] {
		marks = append(marks, row["month"].(string)+" "+row["transfer_key"].(string)+" "+row["done_on"].(string))
	}
	return marks
}

func TestTheExamplesPaydayAnswersItsCushionAndSevenTransfers(t *testing.T) {
	env := newPaydayEnv(t, 800_000)
	if got := env.fieldOf("review.payday", `["2026-03"]`, "cushion"); got != "3820.00" {
		t.Errorf("cushion %s, want 3820.00", got)
	}
	pending := func(key, name string, amount float64) transfer {
		return transfer{Key: key, Name: name, Amount: amount, State: "pending"}
	}
	want := []transfer{
		pending("pot:"+exampleNubankPot, "Reserva", 60), pending("pot:"+examplePotOne, "Reserva", 240),
		pending("pot:"+examplePotSeven, "Sem nome", 250), pending(env.lineKey("Revisão do carro"), "Revisão do carro", 200),
		pending(env.lineKey("Viagem"), "Viagem", 400), pending(env.lineKey("Bicicleta"), "Bicicleta", 100),
		pending("investment", "Investimento", 2930),
	}
	if got := env.transfers("2026-03"); !equalTransfers(got, want) {
		t.Errorf("transfers = %+v\nwant %+v", got, want)
	}
	poorer := newPaydayEnv(t, 500_000)
	if got := poorer.transfers("2026-03"); len(got) != 6 || got[5].Key != poorer.lineKey("Bicicleta") {
		t.Errorf("under the negative residual the transfers are %+v, want the first six", got)
	}
}

func equalTransfers(got, want []transfer) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		same := got[i].Key == want[i].Key && got[i].Name == want[i].Name && got[i].Amount == want[i].Amount &&
			got[i].State == want[i].State && got[i].MarkedByHand == want[i].MarkedByHand
		if !same || (got[i].DoneOn == nil) != (want[i].DoneOn == nil) || (got[i].DoneOn != nil && *got[i].DoneOn != *want[i].DoneOn) {
			return false
		}
	}
	return true
}
