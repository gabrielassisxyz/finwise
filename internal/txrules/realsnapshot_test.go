package txrules_test

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// Every month of a real snapshot through the rules, opt-in. Only counts are
// logged: the snapshot is a person's financial history and no value from it
// (an amount, a description, a name, a document, an id, a month) may reach a log.
func TestEveryMonthOfARealSnapshotIsAccountedFor(t *testing.T) {
	path := os.Getenv("FINWISE_LEDGER_SNAPSHOT")
	if path == "" {
		t.Skip("FINWISE_LEDGER_SNAPSHOT is not set")
	}
	reader, ctx := ledger.New(path), t.Context()
	accounts, err := reader.Accounts(ctx)
	failWithoutValues(t, "accounts", err)
	identities, err := reader.Identities(ctx)
	failWithoutValues(t, "identities", err)
	reference := txrules.Reference{Accounts: accounts, Identities: identities}

	everything := brday.Period{First: "1970-01-01", Last: "2999-12-31"}
	all, err := reader.CleanTransactions(ctx, everything)
	failWithoutValues(t, "transactions", err)

	months := monthsOf(all)
	summed, counts, balanced := txrules.Tally{}, newCounts(), true
	for _, month := range months {
		period, err := brday.MonthPeriod(month)
		failWithoutValues(t, "month period", err)
		rows, err := reader.CleanTransactions(ctx, period)
		failWithoutValues(t, "month transactions", err)

		result := txrules.Interpret(rows, reference)
		if !result.Tally.Balanced() || result.Tally.Input != len(rows) {
			balanced = false
			t.Errorf("a month's tally does not add up: %+v for %d rows", result.Tally, len(rows))
		}
		summed.Input += result.Tally.Input
		summed.Listed += result.Tally.Listed
		summed.AttachedAsTax += result.Tally.AttachedAsTax
		summed.PotMovements += result.Tally.PotMovements
		counts.add(rows, result, accounts)
	}
	whole := txrules.Interpret(all, reference)

	if summed.Input != len(all) {
		t.Errorf("the months cover %d of %d rows", summed.Input, len(all))
	}
	t.Logf("months=%d input=%d listed=%d attached_as_tax=%d pot_movements=%d balanced_every_month=%t",
		len(months), summed.Input, summed.Listed, summed.AttachedAsTax, summed.PotMovements, balanced)
	t.Logf("whole_ledger_in_one_period: listed=%d attached_as_tax=%d pot_movements=%d",
		whole.Tally.Listed, whole.Tally.AttachedAsTax, whole.Tally.PotMovements)
	for _, line := range counts.lines() {
		t.Log(line)
	}
}

func failWithoutValues(t *testing.T, step string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s failed (%T); the message is withheld because it may name a real value", step, err)
	}
}

func monthsOf(rows []ledger.Transaction) []brday.Month {
	seen := make(map[brday.Month]bool)
	for _, row := range rows {
		seen[brday.MonthOf(row.Date)] = true
	}
	months := make([]brday.Month, 0, len(seen))
	for month := range seen {
		months = append(months, month)
	}
	sort.Slice(months, func(a, b int) bool { return months[a] < months[b] })
	return months
}

// counts holds the rows matched per rule and the rows no rule recognised.
type counts struct {
	kinds, rules, attached, other map[string]int
	explicitTaxRows               int
	signSurprises, unconverted    int
	cardDescriptions, cardKeys    map[string]bool
}

func newCounts() *counts {
	return &counts{
		kinds: map[string]int{}, rules: map[string]int{}, attached: map[string]int{}, other: map[string]int{},
		cardDescriptions: map[string]bool{}, cardKeys: map[string]bool{},
	}
}

func (c *counts) add(rows []ledger.Transaction, result txrules.Result, accounts []ledger.Account) {
	c.addInputChecks(rows, accounts)
	for _, listed := range result.Listed {
		c.kinds[string(listed.Kind)]++
		if listed.Rule != "" {
			c.rules[string(listed.Rule)]++
		}
		c.addUnrecognised(listed)
		if listed.Unconverted {
			c.unconverted++
		}
		if listed.Kind == txrules.KindCard {
			c.cardDescriptions[listed.RawDescription] = true
			c.cardKeys[listed.EstablishmentKey] = true
		}
	}
	for _, attached := range result.Attached {
		c.attached[string(attached.Rule)]++
	}
	for _, movements := range result.PotMovements {
		for _, movement := range movements {
			c.rules[map[bool]string{true: "pot-yield", false: "pot-transfer"}[movement.Yield]]++
		}
	}
}

// addInputChecks counts rows whose stored sign contradicts the convention the
// rules rely on (a BANK debit negative, a CREDIT-account debit positive), and
// the explicit tax rows there are to attach.
func (c *counts) addInputChecks(rows []ledger.Transaction, accounts []ledger.Account) {
	types := make(map[string]ledger.AccountType, len(accounts))
	for _, account := range accounts {
		types[account.ID] = account.Type
	}
	for _, row := range rows {
		storedOut := (row.Type == "DEBIT") == (types[row.AccountID] == ledger.AccountBank)
		if (storedOut && row.Amount > 0) || (!storedOut && row.Amount < 0) {
			c.signSurprises++
		}
		if strings.HasPrefix(strings.ToUpper(row.Description), "IOF INTERNACIONAL") {
			c.explicitTaxRows++
		}
	}
}

func (c *counts) addUnrecognised(listed txrules.Transaction) {
	switch {
	case listed.Kind == txrules.KindOther && listed.Subtype == "" && listed.Movement == txrules.NotAMovement:
		c.other["bank rows no rule knows, listed as Outros"]++
	case listed.Kind == txrules.KindCard && listed.Amount > 0 && listed.Rule == "" && !listed.Refund:
		c.other["card credits no rule recognised, listed as ordinary"]++
	}
}

func (c *counts) lines() []string {
	lines := []string{
		"listed per kind: " + render(c.kinds),
		"rows matched per rule: " + render(c.rules),
		"tax rows attached, per rule: " + render(c.attached),
		"explicit tax rows in the ledger (description starts with the keyword): " + strconv.Itoa(c.explicitTaxRows),
		"rows left unrecognised: " + render(c.other),
		"rows whose stored sign contradicts the convention: " + strconv.Itoa(c.signSurprises),
		"foreign rows with no value in reais: " + strconv.Itoa(c.unconverted),
		"card descriptions " + strconv.Itoa(len(c.cardDescriptions)) + " normalise to establishment keys " + strconv.Itoa(len(c.cardKeys)),
	}
	return lines
}

func render(values map[string]int) string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+strconv.Itoa(values[name]))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}
