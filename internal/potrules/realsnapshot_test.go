package potrules_test

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/potrules"
)

// The pot rules over a real snapshot, opt-in. Only counts are logged: the
// snapshot is a person's financial history and no value from it may reach a log.
func TestPotRulesOverARealSnapshot(t *testing.T) {
	path := os.Getenv("FINWISE_LEDGER_SNAPSHOT")
	if path == "" {
		t.Skip("FINWISE_LEDGER_SNAPSHOT is not set")
	}
	reader := ledger.New(path)
	accounts := bankAccounts(t, reader)
	sightings, sightingDays := readSightings(t, reader)

	pots := potrules.Identify(sightings)
	logPotCounts(t, pots, sightingDays)

	rows, unmatched := readYieldRows(t, reader, sightingDays)
	months := monthsOf(sightingDays)
	logYieldCounts(t, pots, accounts, months, rows, unmatched)
	logMovementCounts(t, reader, sightings, sightingDays, accounts)
}

func bankAccounts(t *testing.T, reader *ledger.Reader) map[string]ledger.Account {
	t.Helper()
	all, err := reader.Accounts(t.Context())
	if err != nil {
		t.Fatalf("accounts failed (%T); the message is withheld because it may name a real id", err)
	}
	banks := map[string]ledger.Account{}
	for _, account := range all {
		if account.Type == ledger.AccountBank {
			banks[account.ID] = account
		}
	}
	return banks
}

// readSightings walks each account's sightings backwards from the latest: the
// reader returns an account's latest sighting on or before the day asked, so
// asking for the day before the one just read reaches the one before it. An
// account stops where the reader returns no pot for it, which is also where a
// sighting that held no pot would hide the older ones; that is the reader's
// shape, and the counts say how far back the history reaches.
func readSightings(t *testing.T, reader *ledger.Reader) ([]potrules.Sighting, map[string][]brday.Day) {
	t.Helper()
	cursor := map[string]brday.Day{}
	var sightings []potrules.Sighting
	days := map[string][]brday.Day{}
	asOf := brday.Day("2999-12-31")
	for asOf != "" {
		pots, err := reader.Pots(t.Context(), asOf)
		if err != nil {
			t.Fatalf("pots failed (%T); the message is withheld because it may name a real id", err)
		}
		reached := map[string]brday.Day{}
		for _, pot := range pots {
			if cursor[pot.AccountID] == "" || cursor[pot.AccountID] == asOf {
				sightings = append(sightings, sightingOf(pot))
				reached[pot.AccountID] = pot.Day
			}
		}
		for account, day := range reached {
			days[account] = append(days[account], day)
			cursor[account] = previousDay(t, day)
		}
		asOf = latestCursor(cursor, reached, asOf)
	}
	return sightings, days
}

// latestCursor is the next day to ask for: the latest day any account still has
// to be read back from. An account asked for this day that returned no pot has
// no older sighting the reader can show, and is dropped.
func latestCursor(cursor map[string]brday.Day, reached map[string]brday.Day, asked brday.Day) brday.Day {
	var next brday.Day
	for account, day := range cursor {
		if _, returned := reached[account]; !returned && day == asked {
			delete(cursor, account)
			continue
		}
		next = max(next, day)
	}
	return next
}

func sightingOf(pot ledger.Pot) potrules.Sighting {
	sighting := potrules.Sighting{
		AccountID: pot.AccountID, Day: pot.Day, ReserveID: pot.ReserveID, Name: pot.Name,
		StableIdentity: pot.StableIdentity, CDIPercent: pot.CDIPercentage,
	}
	if pot.AmountStatus == "ok" {
		sighting.Balance = pot.Amount
	}
	return sighting
}

func previousDay(t *testing.T, day brday.Day) brday.Day {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", string(day))
	if err != nil {
		t.Fatal(err)
	}
	return brday.Day(parsed.AddDate(0, 0, -1).Format("2006-01-02"))
}

func logPotCounts(t *testing.T, pots []potrules.Pot, days map[string][]brday.Day) {
	t.Helper()
	var stableCount, unstable, closed, open, noBalance, sightingDays int
	for _, pot := range pots {
		switch {
		case !pot.Stable:
			unstable++
		case pot.Closed:
			closed++
		default:
			open++
		}
		if !pot.Closed && pot.Balance == nil {
			noBalance++
		}
		if pot.Stable {
			stableCount++
		}
	}
	for _, accountDays := range days {
		sightingDays += len(accountDays)
	}
	t.Logf("pots=%d stable=%d unstable_on_latest_day=%d open=%d closed=%d open_without_readable_balance=%d account_sighting_days=%d",
		len(pots), stableCount, unstable, open, closed, noBalance, sightingDays)
}

// readYieldRows asks the ledger for every pair of consecutive sightings of each
// account. A row that names no stable reserve cannot be given to a pot and is
// counted instead.
func readYieldRows(t *testing.T, reader *ledger.Reader, days map[string][]brday.Day) ([]potrules.DayYield, int) {
	t.Helper()
	var rows []potrules.DayYield
	unmatched := 0
	for account, accountDays := range days {
		sort.Slice(accountDays, func(i, j int) bool { return accountDays[i] < accountDays[j] })
		for index := 1; index < len(accountDays); index++ {
			yields, err := reader.PotYields(t.Context(), account, accountDays[index-1], accountDays[index])
			if err != nil {
				t.Fatalf("pot yield failed (%T); the message is withheld because it may name a real id", err)
			}
			for _, yield := range yields {
				if yield.ReserveID == nil {
					unmatched++
					continue
				}
				rows = append(rows, potrules.DayYield{
					AccountID: account, ReserveID: *yield.ReserveID,
					From: yield.From, To: yield.To, Yield: yield.Yield,
				})
			}
		}
	}
	return rows, unmatched
}

func monthsOf(days map[string][]brday.Day) []brday.Month {
	set := map[brday.Month]bool{}
	for _, accountDays := range days {
		for _, day := range accountDays {
			set[brday.Month(day[:7])] = true
		}
	}
	months := make([]brday.Month, 0, len(set))
	for month := range set {
		months = append(months, month)
	}
	sort.Slice(months, func(i, j int) bool { return months[i] < months[j] })
	return months
}

// baseForTest is the caller's choice, which the rules do not make: Nubank
// reports net of tax and Mercado Pago gross. The name is read and never logged.
func baseForTest(account ledger.Account) potrules.Base {
	name := strings.ToLower(account.Name)
	switch {
	case strings.Contains(name, "nubank") || strings.Contains(name, "nu pagamentos"):
		return potrules.BaseNet
	case strings.Contains(name, "mercado"):
		return potrules.BaseGross
	}
	return potrules.BaseUnknown
}

func logYieldCounts(t *testing.T, pots []potrules.Pot, accounts map[string]ledger.Account,
	months []brday.Month, rows []potrules.DayYield, unmatched int) {
	t.Helper()
	known, unknown, unknownRows, computedRows, startsBefore := 0, 0, 0, 0, 0
	var monthly []potrules.MonthYield
	for _, pot := range pots {
		for _, month := range months {
			yield := potrules.YieldOfMonth(pot, baseForTest(accounts[pot.AccountID]), month, rows)
			if !pot.Stable || (len(yield.Days) == 0 && !yield.Known) {
				continue
			}
			monthly = append(monthly, yield)
			if yield.Since != "" && string(yield.Since[:7]) < string(month) {
				startsBefore++
			}
			if yield.Known {
				known++
			} else {
				unknown++
			}
		}
	}
	for _, row := range rows {
		if row.Yield == nil {
			unknownRows++
		} else {
			computedRows++
		}
	}
	zero, positive, negative := signCounts(rows)
	t.Logf("yield rows: computed=%d (zero=%d positive=%d negative=%d) not_computable=%d transfers_naming_no_stable_pot=%d",
		computedRows, zero, positive, negative, unknownRows, unmatched)
	t.Logf("pot-months with rows: known=%d unknown=%d, first row starting before the month=%d", known, unknown, startsBefore)
	for _, total := range potrules.TotalYieldsByBase(monthly) {
		t.Logf("base %s: pot-months=%d unknown=%d", total.Base, total.Pots, total.Unknown)
	}
}

func logMovementCounts(t *testing.T, reader *ledger.Reader, sightings []potrules.Sighting,
	days map[string][]brday.Day, accounts map[string]ledger.Account) {
	t.Helper()
	movements := reserveMovements(t, reader.CleanTransactions, accounts)
	var inWindow []potrules.Movement
	for _, movement := range movements {
		if first := firstDay(days[movement.AccountID]); first != "" && movement.Day >= first {
			inWindow = append(inWindow, movement)
		}
	}
	result := potrules.AttributeMovements(sightings, inWindow)
	t.Logf("reserve movements=%d, on or after the account's first sighting=%d, with a sighting within three days after=%d: attributed=%d unattributed=%d",
		len(movements), len(inWindow), withSightingSoonAfter(inWindow, days), len(result.Attributed), len(result.Unattributed))
}

func withSightingSoonAfter(movements []potrules.Movement, days map[string][]brday.Day) int {
	count := 0
	for _, movement := range movements {
		for _, day := range days[movement.AccountID] {
			if day > movement.Day && day <= addDays(movement.Day, 3) {
				count++
				break
			}
		}
	}
	return count
}

func addDays(day brday.Day, days int) brday.Day {
	parsed, err := time.Parse("2006-01-02", string(day))
	if err != nil {
		return day
	}
	return brday.Day(parsed.AddDate(0, 0, days).Format("2006-01-02"))
}

func firstDay(days []brday.Day) brday.Day {
	var first brday.Day
	for _, day := range days {
		if first == "" || day < first {
			first = day
		}
	}
	return first
}

// reserveMovements reads the one kind of row that moves money between an account
// and its pots. Reading a kind and a sign is txrules' job; this is the single
// place the test does it, only to have movements to attribute: a deposit is
// negative on the checking account and raises the pot by the same amount.
func reserveMovements(t *testing.T,
	read func(context.Context, brday.Period) ([]ledger.Transaction, error),
	accounts map[string]ledger.Account) []potrules.Movement {
	t.Helper()
	all, err := read(t.Context(), brday.Period{First: "1970-01-01", Last: "2999-12-31"})
	if err != nil {
		t.Fatalf("transactions failed (%T); the message is withheld because it may name a real id", err)
	}
	var movements []potrules.Movement
	for _, transaction := range all {
		if _, isBank := accounts[transaction.AccountID]; !isBank {
			continue
		}
		if transaction.OperationType == nil || *transaction.OperationType != "TRANSFERENCIA_SALDO_RESERVADO" {
			continue
		}
		movements = append(movements, potrules.Movement{
			AccountID: transaction.AccountID, Day: transaction.Day, Change: money.Cents(-transaction.Amount),
		})
	}
	return movements
}

func signCounts(rows []potrules.DayYield) (zero, positive, negative int) {
	for _, row := range rows {
		switch {
		case row.Yield == nil:
		case *row.Yield == 0:
			zero++
		case *row.Yield > 0:
			positive++
		default:
			negative++
		}
	}
	return zero, positive, negative
}
