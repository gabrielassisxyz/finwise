package api

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// cents turns a figure of an answer back into whole cents for an exact
// comparison; an answer's figures are decimal text and were read as numbers.
func cents(amount float64) int64 { return int64(math.Round(amount * 100)) }

func bucketTotals(t *testing.T, worth map[string]any) map[string]int64 {
	t.Helper()
	totals := map[string]int64{}
	for _, bucket := range worth["buckets"].([]any) {
		entry := bucket.(map[string]any)
		var sum int64
		for _, item := range entry["items"].([]any) {
			sum += cents(item.(map[string]any)["amount"].(float64))
		}
		if sum != cents(entry["total"].(float64)) {
			t.Errorf("the items of %v add up to %d, its total is %d", entry["bucket"], sum, cents(entry["total"].(float64)))
		}
		totals[entry["bucket"].(string)] = cents(entry["total"].(float64))
	}
	return totals
}

func TestTheNetWorthIsTheThreePartsToTheCent(t *testing.T) {
	worth := answerOf(t, newReadEnv(t, newFullLedger(t)), "netWorth.overview", `[]`)
	totals := bucketTotals(t, worth)

	// Five bank accounts, three reserve entries and two investments with a net amount.
	want := map[string]int64{"Disponível": 205966, "Reservado": 207580, "Investido": 1000000}
	for bucket, amount := range want {
		if totals[bucket] != amount {
			t.Errorf("%s is %d cents, want %d", bucket, totals[bucket], amount)
		}
	}
	if sum := totals["Disponível"] + totals["Reservado"] + totals["Investido"]; cents(worth["total"].(float64)) != sum {
		t.Errorf("the total is %d cents and its parts add up to %d", cents(worth["total"].(float64)), sum)
	}
}

func TestAReserveEntryIsReservedAndAnInvestmentIsInvested(t *testing.T) {
	worth := answerOf(t, newReadEnv(t, newFullLedger(t)), "netWorth.overview", `[]`)
	names := map[string][]string{}
	for _, bucket := range worth["buckets"].([]any) {
		entry := bucket.(map[string]any)
		for _, item := range entry["items"].([]any) {
			if name, ok := item.(map[string]any)["name"].(string); ok {
				names[entry["bucket"].(string)] = append(names[entry["bucket"].(string)], name)
			}
		}
	}
	contains := func(bucket, name string) bool {
		for _, found := range names[bucket] {
			if found == name {
				return true
			}
		}
		return false
	}
	if !contains("Reservado", "Caixinha da casa") || !contains("Investido", "CDB Exemplo") || contains("Reservado", "CDB Exemplo") {
		t.Errorf("the buckets hold %v", names)
	}
	if contains("Investido", "CDB encerrado") {
		t.Error("a withdrawn investment with nothing in it is listed")
	}
}

func TestTheChartStartsAtTheFirstMonthWithAReading(t *testing.T) {
	chart := answerOf(t, newReadEnv(t, newFullLedger(t)), "netWorth.overview", `[]`)["chart"].(map[string]any)
	months, points := chart["months"].([]any), chart["points"].([]any)
	if len(months) != 3 || months[0] != "2026-01" {
		t.Errorf("months %v, want January to March", months)
	}
	for _, point := range points {
		if point.(map[string]any)["month"].(string) < "2026-01" {
			t.Errorf("a point before the first reading: %v", point)
		}
	}
	if len(points) != 3 || points[0].(map[string]any)["month"] != "2026-01" {
		t.Errorf("points %v", points)
	}
}

func TestAMonthWithNoReadingIsAGapAndNeverAZero(t *testing.T) {
	f := ledgertest.New(t)
	addBank(f, "item-nu", accountNubank, "Conta Nubank", "260/0001/3-3", 10)
	worth := answerOf(t, envAt(t, f, 2026, time.May, 20), "netWorth.overview", `[]`)

	chart := worth["chart"].(map[string]any)
	if len(chart["months"].([]any)) != 3 || len(chart["points"].([]any)) != 1 {
		t.Errorf("months %v points %v, want March to May with the one reading of March", chart["months"], chart["points"])
	}
	if worth["historyStart"] != "2026-03" {
		t.Errorf("history starts %v", worth["historyStart"])
	}
}

func TestTheCurrentMonthsPointIsTheNetWorthShownAboveIt(t *testing.T) {
	worth := answerOf(t, newReadEnv(t, newFullLedger(t)), "netWorth.overview", `[]`)
	points := worth["chart"].(map[string]any)["points"].([]any)
	last := points[len(points)-1].(map[string]any)
	if cents(last["total"].(float64)) != cents(worth["total"].(float64)) {
		t.Errorf("the chart ends at %v, the net worth is %v", last["total"], worth["total"])
	}
	totals := bucketTotals(t, worth)
	if cents(last["available"].(float64)) != totals["Disponível"] || cents(last["reserved"].(float64)) != totals["Reservado"] ||
		cents(last["invested"].(float64)) != totals["Investido"] {
		t.Errorf("the chart's last point %v disagrees with the buckets %v", last, totals)
	}
}

func TestOverTheEmptyLedgerTheNetWorthIsZeroWithAnEmptyChart(t *testing.T) {
	worth := answerOf(t, newReadEnv(t, newEmptyLedger(t)), "netWorth.overview", `[]`)
	chart := worth["chart"].(map[string]any)
	if worth["total"].(float64) != 0 || len(chart["months"].([]any)) != 0 || len(chart["points"].([]any)) != 0 || chart["max"].(float64) != 0 {
		t.Errorf("total %v chart %v", worth["total"], chart)
	}
	if worth["yields"].(map[string]any)["total"] != nil {
		t.Error("an empty ledger has a yield total")
	}
}

func TestTheChartsTopHoldsTheHighestPoint(t *testing.T) {
	for total, want := range map[int64]int64{0: 0, 99: 100, 100: 100, 101: 200, 200: 200, 201: 500, 501: 1000, 1413546: 2000000} {
		points := []netWorthPointWire{{Total: Reais(total)}}
		if got := chartMaxOf(points); int64(got) != want {
			t.Errorf("the top for %d cents is %d, want %d", total, got, want)
		}
	}
}

// backedLedger is a Nubank connection with reserves of 1000 and two
// investments on the same connection of 600 and 400 net.
func backedLedger(t *testing.T, secondInvestment float64) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	addBank(f, "item-nu", accountNubank, "Conta Nubank", "260/0001/3-3", 10,
		ledgertest.Reserve{ID: "pot-a", Name: "A", Amount: 600, CDIRatio: 1}, ledgertest.Reserve{ID: "pot-b", Name: "B", Amount: 400, CDIRatio: 1})
	f.AddInvestment(ledgertest.Investment{ID: "inv-a", ItemID: "item-nu", Type: "FIXED_INCOME", Name: "RDB A", Currency: "BRL",
		Balance: 600, Raw: map[string]any{"amountOriginal": 500, "rateType": "CDI"}})
	f.AddInvestment(ledgertest.Investment{ID: "inv-b", ItemID: "item-nu", Type: "FIXED_INCOME", Name: "RDB B", Currency: "BRL",
		Balance: secondInvestment, Raw: map[string]any{"amountOriginal": 300, "rateType": "CDI"}})
	return f
}

func TestAnInvestmentThatTheLedgerShowsBacksTheReservesIsNotCountedTwice(t *testing.T) {
	f := backedLedger(t, 400)
	view, err := loadPots(withNow(t.Context(), func() time.Time { return fixtureNow }), ledgerOf(t, f), false)
	if err != nil {
		t.Fatal(err)
	}
	if view.onReserveAccounts != 2 || view.leftOut != 2 {
		t.Errorf("seen beside reserves %d, left out %d, want 2 and 2", view.onReserveAccounts, view.leftOut)
	}
	worth := answerOf(t, newReadEnv(t, f), "netWorth.overview", `[]`)
	if totals := bucketTotals(t, worth); totals["Investido"] != 0 || totals["Reservado"] != 100000 {
		t.Errorf("reserved and invested are %v, want the 1000 once", totals)
	}
}

func TestInvestmentsBesideReservesThatTheLedgerDoesNotShowBackThemAreKeptAndCounted(t *testing.T) {
	f := backedLedger(t, 450)
	view, err := loadPots(withNow(t.Context(), func() time.Time { return fixtureNow }), ledgerOf(t, f), false)
	if err != nil {
		t.Fatal(err)
	}
	if view.onReserveAccounts != 2 || view.leftOut != 0 {
		t.Errorf("seen beside reserves %d, left out %d, want 2 and 0", view.onReserveAccounts, view.leftOut)
	}
	if totals := bucketTotals(t, answerOf(t, newReadEnv(t, f), "netWorth.overview", `[]`)); totals["Investido"] != 105000 {
		t.Errorf("invested is %d, want both investments", totals["Investido"])
	}
}

func TestInvestmentsOnAConnectionWithoutReservesAreNotBesideAnyAndTheFullLedgerCountsTwo(t *testing.T) {
	view, err := loadPots(withNow(t.Context(), func() time.Time { return fixtureNow }), ledgerOf(t, newFullLedger(t)), false)
	if err != nil {
		t.Fatal(err)
	}
	// The two Nubank investments sit beside its reserve entries; the Itaú one does not.
	if view.onReserveAccounts != 2 || view.leftOut != 0 {
		t.Errorf("seen beside reserves %d, left out %d, want 2 and 0", view.onReserveAccounts, view.leftOut)
	}
}

func TestAMonthWithAKnownYieldShowsItAndOnlyTheOneBase(t *testing.T) {
	f := ledgertest.New(t)
	addBank(f, "item-nu", accountNubank, "Conta Nubank", "260/0001/3-3", 10)
	f.ObserveAccount(accountNubank, "2026-03-10", sightedAt("2026-03-10", 10,
		ledgertest.Reserve{ID: "pot-t", Name: "Trip", Amount: 100, CDIRatio: 1}))
	walkReachesBack(t, f, accountNubank)

	yields := answerOf(t, envAt(t, f, 2026, time.March, 20), "netWorth.overview", `[]`)["yields"].(map[string]any)
	if yields["total"] == nil || yields["total"].(float64) != 50 {
		t.Errorf("the total of one base's known yields is %v, want 50", yields["total"])
	}
}

func TestOneNetPotAndOneGrossPotNeverYieldASingleSummedFigure(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	pots := potsOf(potListOf(t, env, includeClosed))
	if pots["reserve:"+accountPots+"/pot-1"]["monthlyYield"] == nil || pots["reserve:"+accountNubank+"/"+potNubankMain]["monthlyYield"] == nil {
		t.Fatal("the fixture's gross pot and net pot must both have a known yield for this to mean anything")
	}
	yields := answerOf(t, env, "netWorth.overview", `[]`)["yields"].(map[string]any)
	if yields["total"] != nil {
		t.Errorf("a gross yield and a net yield were added into %v", yields["total"])
	}
	if len(yields["pots"].([]any)) == 0 {
		t.Error("each pot's own yield should still be listed")
	}
}

func TestNetWorthShowsNoYieldForAPotWhoseYieldIsUnknown(t *testing.T) {
	worth := answerOf(t, envAt(t, pendingTransferLedger(t), 2026, time.March, 20), "netWorth.overview", `[]`)
	yields := worth["yields"].(map[string]any)
	if yields["total"] != nil || yields["pots"].([]any)[0].(map[string]any)["amount"] != nil {
		t.Errorf("net worth shows a yield for a pot whose yield is unknown: %v", yields)
	}
}

func TestAPotThatWasEmptiedNoLongerCountsInTheNetWorth(t *testing.T) {
	worth := answerOf(t, envAt(t, emptiedLedger(t), 2026, time.March, 20), "netWorth.overview", `[]`)
	if worth["total"].(float64) != 15 {
		t.Errorf("the net worth is %v, want the two accounts' 15 and nothing for the pot that was emptied", worth["total"])
	}
}

func TestAPotPlacedInDisponivelCountsAsAvailableAfterTheAccountsAndTheTotalStays(t *testing.T) {
	env := potsPlacedEnv(t, newFullLedger(t), map[string]authored.PotBucket{potCaixinha: authored.BucketAvailable})
	worth := answerOf(t, env, "netWorth.overview", `[]`)
	totals := bucketTotals(t, worth)
	want := map[string]int64{"Disponível": 285966, "Reservado": 127580, "Investido": 1000000}
	for bucket, amount := range want {
		if totals[bucket] != amount {
			t.Errorf("%s is %d cents, want %d", bucket, totals[bucket], amount)
		}
	}
	if cents(worth["total"].(float64)) != 1413546 {
		t.Errorf("the total is %v, want 14135.46 whatever the buckets", worth["total"])
	}
	available := worth["buckets"].([]any)[0].(map[string]any)
	var names []string
	for _, item := range available["items"].([]any) {
		names = append(names, item.(map[string]any)["name"].(string))
	}
	wantNames := []string{"Banco não identificado · Conta Digital", "C6 · Conta C6", "Itaú · Conta Corrente", "Mercado Pago",
		"Nubank · Conta Nubank", "Caixinha da casa"}
	if available["bucket"] != "Disponível" || !slices.Equal(names, wantNames) {
		t.Errorf("%v lists %v, want the five accounts and then the pot", available["bucket"], names)
	}
}

// chartLedger is the full ledger with a February sighting of the Mercado Pago
// pot (400.00, on an account holding 100.00) and a February day of the Itaú
// investment (6000.00), so that a point before the current month holds both.
func chartLedger(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := newFullLedger(t)
	f.ObserveAccount(accountPots, "2026-02-12", sightedAt("2026-02-12", 100,
		ledgertest.Reserve{ID: "pot-1", Name: "Caixinha da casa", Amount: 400, CDIRatio: 1.1}))
	f.ObserveInvestment(investmentItau, "2026-02-11", map[string]any{"amount": 6000, "amountWithdrawal": 6000, "balance": 6000})
	return f
}

type chartPoint struct {
	month                         string
	available, reserved, invested int64
}

func chartPointsOf(t *testing.T, worth map[string]any) []chartPoint {
	t.Helper()
	var points []chartPoint
	for _, raw := range worth["chart"].(map[string]any)["points"].([]any) {
		point := raw.(map[string]any)
		read := chartPoint{point["month"].(string), cents(point["available"].(float64)), cents(point["reserved"].(float64)),
			cents(point["invested"].(float64))}
		if sum := read.available + read.reserved + read.invested; cents(point["total"].(float64)) != sum {
			t.Errorf("%s: the total is %v and its parts add up to %d cents", read.month, point["total"], sum)
		}
		points = append(points, read)
	}
	return points
}

// The figures are worked out by hand from chartLedger: January holds the Nubank
// account alone, February adds the Mercado Pago account and its pot and the Itaú
// investment's February day, and March is the net worth shown above the chart.
func TestEveryPointOfTheChartCountsEachPotInTheBucketItHasToday(t *testing.T) {
	cases := []struct {
		label  string
		placed map[string]authored.PotBucket
		want   []chartPoint
	}{
		{"nothing chosen", nil, []chartPoint{
			{"2026-01", 12000, 90000, 0}, {"2026-02", 36000, 140000, 600000}, {"2026-03", 205966, 207580, 1000000}}},
		{"the Mercado Pago pot available", map[string]authored.PotBucket{potCaixinha: authored.BucketAvailable}, []chartPoint{
			{"2026-01", 12000, 90000, 0}, {"2026-02", 76000, 100000, 600000}, {"2026-03", 285966, 127580, 1000000}}},
		{"the Itaú investment reserved", map[string]authored.PotBucket{potLCI: authored.BucketReserved}, []chartPoint{
			{"2026-01", 12000, 90000, 0}, {"2026-02", 36000, 740000, 0}, {"2026-03", 205966, 907580, 300000}}},
	}
	for _, c := range cases {
		worth := answerOf(t, potsPlacedEnv(t, chartLedger(t), c.placed), "netWorth.overview", `[]`)
		points := chartPointsOf(t, worth)
		if !slices.Equal(points, c.want) {
			t.Errorf("%s: the chart is %+v, want %+v", c.label, points, c.want)
			continue
		}
		totals, last := bucketTotals(t, worth), points[len(points)-1]
		if last.available != totals["Disponível"] || last.reserved != totals["Reservado"] || last.invested != totals["Investido"] {
			t.Errorf("%s: the chart ends at %+v and the net worth shows %v", c.label, last, totals)
		}
	}
}
