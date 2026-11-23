package budgetrules

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

const (
	paydayUnnamed    = "Sem nome"
	paydayInvestment = "Investimento"

	paydayPotOne     = "reserve:acc-pots/pot-1"
	paydayPotSeven   = "reserve:acc-pots/pot-7"
	paydayNubankPot  = "reserve:acc-nu/pot-nu-1"
	paydayClosedPot  = "reserve:acc-nu/pot-nu-old"
	paydayMissingPot = "reserve:acc-gone/pot-x"
)

type paydayLine struct {
	group  Group
	id     LineID
	name   string
	amount money.Cents
}

func paydayPlan(lines ...paydayLine) Plan {
	plan := Plan{}
	for _, key := range EditableGroups {
		group := PlanGroup{Key: key, Lines: []PlannedLine{}}
		for _, line := range lines {
			if line.group == key {
				group.Lines = append(group.Lines, PlannedLine{ID: OnLine(line.id), Name: line.name, Amount: line.amount})
			}
		}
		plan = append(plan, group)
	}
	return plan
}

func named(name string) *string { return &name }

// The worked example: every amount, name and id is invented.
func exampleLines(salary money.Cents) []paydayLine {
	return []paydayLine{
		{GroupIncome, 1, "Salário", salary},
		{GroupFixed, 2, "Aluguel", 250000},
		{GroupFixed, 3, "Internet", 12000},
		{GroupVariable, 4, "Mercado", 90000},
		{GroupVariable, 5, "Lazer", 30000},
		{GroupProvisions, 11, "IPTU", 15000},
		{GroupProvisions, 12, "Seguro do carro", 9000},
		{GroupProvisions, 13, "Presentes", 6000},
		{GroupProvisions, 14, "Revisão do carro", 20000},
		{GroupProvisions, 15, "Material escolar", 0},
		{GroupReserves, 16, "Viagem", 40000},
		{GroupReserves, 17, "Celular novo", 25000},
		{GroupReserves, 18, "Bicicleta", 10000},
	}
}

var exampleLinks = map[LineID]string{
	11: paydayPotOne, 12: paydayPotOne, 13: paydayNubankPot,
	16: paydayClosedPot, 17: paydayPotSeven, 18: paydayMissingPot,
}

var examplePots = []TransferPot{
	{ID: paydayPotOne, Name: named("Reserva")},
	{ID: paydayPotSeven},
	{ID: paydayNubankPot, Name: named("Reserva")},
	{ID: paydayClosedPot, Name: named("Antiga"), Closed: true},
}

func exampleInputs(salary money.Cents) PaydayInputs {
	return PaydayInputs{
		Plan: paydayPlan(exampleLines(salary)...), Links: exampleLinks, Pots: examplePots,
		UnnamedPot: paydayUnnamed, InvestmentName: paydayInvestment,
	}
}

// shownTransfer is a transfer as the payday lists it.
type shownTransfer struct {
	key    string
	name   string
	amount money.Cents
}

func shown(transfers []Transfer) []shownTransfer {
	list := []shownTransfer{}
	for _, transfer := range transfers {
		list = append(list, shownTransfer{transfer.Key, transfer.Name, transfer.Amount})
	}
	return list
}

var exampleTransfers = []shownTransfer{
	{"pot:reserve:acc-nu/pot-nu-1", "Reserva", 6000},
	{"pot:reserve:acc-pots/pot-1", "Reserva", 24000},
	{"pot:reserve:acc-pots/pot-7", "Sem nome", 25000},
	{"line:14", "Revisão do carro", 20000},
	{"line:16", "Viagem", 40000},
	{"line:18", "Bicicleta", 10000},
	{"investment", "Investimento", 293000},
}

func TestTheWorkedExampleGivesItsSevenTransfersInOrder(t *testing.T) {
	if got := shown(Transfers(exampleInputs(800000))); !reflect.DeepEqual(got, exampleTransfers) {
		t.Errorf("transfers = %v\nwant %v", got, exampleTransfers)
	}
	got := Transfers(exampleInputs(500000))
	if want := exampleTransfers[:6]; !reflect.DeepEqual(shown(got), want) {
		t.Errorf("with the salary at 5.000,00 transfers = %v\nwant %v", shown(got), want)
	}
	var total money.Cents
	for _, transfer := range got {
		total += transfer.Amount
	}
	if total != 125000 {
		t.Errorf("the six add up to %d, want 125000", total)
	}
	if lines := Transfers(exampleInputs(800000))[1].Lines; !reflect.DeepEqual(lines, []LineID{11, 12}) {
		t.Errorf("the pot of lines 11 and 12 carries %v", lines)
	}
}

func TestEachRuleOfTheTransfersHoldsOnItsOwn(t *testing.T) {
	income := paydayLine{GroupIncome, 1, "Salário", 100000}
	cases := []struct {
		name  string
		lines []paydayLine
		links map[LineID]string
		pots  []TransferPot
		want  []shownTransfer
	}{
		{"two pots of one name are two transfers, the lower pot id first",
			[]paydayLine{income, {GroupProvisions, 11, "IPTU", 1000}, {GroupReserves, 12, "Viagem", 2000}},
			map[LineID]string{11: "reserve:b/pot", 12: "reserve:a/pot"},
			[]TransferPot{{ID: "reserve:b/pot", Name: named("Reserva")}, {ID: "reserve:a/pot", Name: named("Reserva")}},
			[]shownTransfer{{"pot:reserve:a/pot", "Reserva", 2000}, {"pot:reserve:b/pot", "Reserva", 1000}, {"investment", "Investimento", 97000}}},
		{"a pot with two linked lines is one transfer of their sum",
			[]paydayLine{income, {GroupProvisions, 11, "IPTU", 1000}, {GroupReserves, 12, "Viagem", 2000}},
			map[LineID]string{11: paydayPotOne, 12: paydayPotOne},
			[]TransferPot{{ID: paydayPotOne, Name: named("Casa")}},
			[]shownTransfer{{"pot:" + paydayPotOne, "Casa", 3000}, {"investment", "Investimento", 97000}}},
		{"a pot with no name shows and sorts as Sem nome",
			[]paydayLine{income, {GroupProvisions, 11, "IPTU", 1000}, {GroupReserves, 12, "Viagem", 2000}, {GroupReserves, 13, "Carro", 500}},
			map[LineID]string{11: "reserve:a/unnamed", 12: "reserve:z/reserva", 13: "reserve:y/tio"},
			[]TransferPot{{ID: "reserve:a/unnamed"}, {ID: "reserve:z/reserva", Name: named("Reserva")}, {ID: "reserve:y/tio", Name: named("Tio")}},
			[]shownTransfer{{"pot:reserve:z/reserva", "Reserva", 2000}, {"pot:reserve:a/unnamed", "Sem nome", 1000},
				{"pot:reserve:y/tio", "Tio", 500}, {"investment", "Investimento", 96500}}},
		{"a line linked to no pot is a transfer of its own",
			[]paydayLine{income, {GroupProvisions, 14, "Revisão do carro", 20000}}, nil, nil,
			[]shownTransfer{{"line:14", "Revisão do carro", 20000}, {"investment", "Investimento", 80000}}},
		{"a line linked to a closed pot is linked to none",
			[]paydayLine{income, {GroupReserves, 16, "Viagem", 40000}}, map[LineID]string{16: paydayClosedPot},
			[]TransferPot{{ID: paydayClosedPot, Name: named("Antiga"), Closed: true}},
			[]shownTransfer{{"line:16", "Viagem", 40000}, {"investment", "Investimento", 60000}}},
		{"a line linked to a pot the ledger does not list is linked to none",
			[]paydayLine{income, {GroupReserves, 18, "Bicicleta", 10000}}, map[LineID]string{18: paydayMissingPot},
			[]TransferPot{{ID: paydayPotOne, Name: named("Reserva")}},
			[]shownTransfer{{"line:18", "Bicicleta", 10000}, {"investment", "Investimento", 90000}}},
		{"a line of 0.00 is no transfer",
			[]paydayLine{income, {GroupProvisions, 15, "Material escolar", 0}, {GroupProvisions, 14, "Revisão", 100}}, nil, nil,
			[]shownTransfer{{"line:14", "Revisão", 100}, {"investment", "Investimento", 99900}}},
		{"a pot whose lines add up to 0.00 is no transfer",
			[]paydayLine{income, {GroupProvisions, 11, "IPTU", 0}, {GroupReserves, 12, "Viagem", 0}},
			map[LineID]string{11: paydayPotOne, 12: paydayPotOne}, []TransferPot{{ID: paydayPotOne, Name: named("Reserva")}},
			[]shownTransfer{{"investment", "Investimento", 100000}}},
		{"a residual of exactly 0.00 is no investment",
			[]paydayLine{income, {GroupFixed, 2, "Aluguel", 60000}, {GroupReserves, 16, "Viagem", 40000}}, nil, nil,
			[]shownTransfer{{"line:16", "Viagem", 40000}}},
		{"a negative residual is no investment",
			[]paydayLine{income, {GroupFixed, 2, "Aluguel", 60000}, {GroupReserves, 16, "Viagem", 40001}}, nil, nil,
			[]shownTransfer{{"line:16", "Viagem", 40001}}},
		{"a linked line of the fixed group is no transfer",
			[]paydayLine{income, {GroupFixed, 2, "Aluguel", 60000}}, map[LineID]string{2: paydayPotOne},
			[]TransferPot{{ID: paydayPotOne, Name: named("Reserva")}},
			[]shownTransfer{{"investment", "Investimento", 40000}}},
		{"an empty budget has no transfer", nil, map[LineID]string{11: paydayPotOne},
			[]TransferPot{{ID: paydayPotOne, Name: named("Reserva")}}, []shownTransfer{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inputs := PaydayInputs{Plan: paydayPlan(c.lines...), Links: c.links, Pots: c.pots,
				UnnamedPot: paydayUnnamed, InvestmentName: paydayInvestment}
			if got := shown(Transfers(inputs)); !reflect.DeepEqual(got, c.want) {
				t.Errorf("transfers = %v\nwant %v", got, c.want)
			}
		})
	}
}

func TestTheCushionIsTheFixedAndTheVariableGroups(t *testing.T) {
	if got := Cushion(paydayPlan(exampleLines(800000)...)); got != 382000 {
		t.Errorf("cushion = %d, want 382000", got)
	}
	richer := exampleLines(800000)
	richer[5].amount += 99999
	if got := Cushion(paydayPlan(richer...)); got != 382000 {
		t.Errorf("a provision raised moved the cushion to %d", got)
	}
	if got := Cushion(paydayPlan()); got != 0 {
		t.Errorf("the cushion of an empty budget = %d", got)
	}
}

func TestAMarkCountsOnlyForATransferTheMonthLists(t *testing.T) {
	transfers := Transfers(exampleInputs(800000))
	marked := Marked(transfers, TransferMarks{"pot:" + paydayPotOne: "2026-03-20", "line:15": "2026-03-20", "Reserva": "2026-03-20"})
	var done []string
	for _, transfer := range marked {
		if transfer.Done() {
			done = append(done, transfer.Key+" "+string(transfer.DoneOn))
		}
	}
	if want := []string{"pot:" + paydayPotOne + " 2026-03-20"}; !reflect.DeepEqual(done, want) {
		t.Errorf("done = %v, want %v", done, want)
	}
	if got := ContributionOf(marked); got != (Contribution{Planned: 418000, Done: 24000, Progress: 6}) {
		t.Errorf("contribution = %+v", got)
	}
}

func TestTheContributionsProgressIsRoundedHalfUpAndHeldTo100(t *testing.T) {
	cases := []struct {
		done, planned money.Cents
		progress      int64
	}{{100, 800, 13}, {24000, 418000, 6}, {317000, 418000, 76}, {0, 0, 0}, {800, 800, 100}, {1, 3, 33}, {2, 3, 67}}
	for _, c := range cases {
		transfers := []MarkedTransfer{{Transfer: Transfer{Key: "a", Amount: c.done}, DoneOn: "2026-03-20"},
			{Transfer: Transfer{Key: "b", Amount: c.planned - c.done}}}
		if got := ContributionOf(transfers).Progress; got != c.progress {
			t.Errorf("%d done of %d: progress %d, want %d", c.done, c.planned, got, c.progress)
		}
	}
}

func TestAPotFollowsItsOwnTransferAndAnInvestmentTheSharedOne(t *testing.T) {
	marked := Marked(Transfers(exampleInputs(800000)), TransferMarks{InvestmentTransferKey: "2026-03-20"})
	for _, c := range []struct {
		pot        string
		investment bool
		key        string
	}{
		{paydayPotOne, false, "pot:" + paydayPotOne},
		{"reserve:acc-pots/unlinked", false, ""},
		{"investment:inv-1", true, InvestmentTransferKey},
		{paydayPotSeven, true, "pot:" + paydayPotSeven},
	} {
		transfer, found := TransferOfPot(marked, c.pot, c.investment)
		if found != (c.key != "") || transfer.Key != c.key {
			t.Errorf("%s follows %q (%v), want %q", c.pot, transfer.Key, found, c.key)
		}
	}
	if _, found := TransferOfPot(Marked(Transfers(exampleInputs(500000)), nil), "investment:inv-1", true); found {
		t.Error("an investment follows a transfer under a negative residual")
	}
}

func TestTheInvestmentRatesFollowTheInvestmentTransfer(t *testing.T) {
	rates := func(salary money.Cents, marks TransferMarks) string {
		inputs := exampleInputs(salary)
		planned, actual := InvestmentRates(inputs.Plan, Marked(Transfers(inputs), marks))
		return percentText(planned) + " " + percentText(actual)
	}
	for _, c := range []struct {
		label  string
		salary money.Cents
		marks  TransferMarks
		want   string
	}{
		{"nothing marked", 800000, nil, "37 0"},
		{"a pot marked", 800000, TransferMarks{"pot:" + paydayPotOne: "2026-03-20"}, "37 0"},
		{"the investment marked", 800000, TransferMarks{InvestmentTransferKey: "2026-03-20"}, "37 37"},
		{"a negative residual, with a stale mark", 500000, TransferMarks{InvestmentTransferKey: "2026-03-20"}, "-1 0"},
		{"no income", 0, TransferMarks{InvestmentTransferKey: "2026-03-20"}, "nil nil"},
	} {
		if got := rates(c.salary, c.marks); got != c.want {
			t.Errorf("%s: rates %s, want %s", c.label, got, c.want)
		}
	}
}

func percentText(percent *int64) string {
	if percent == nil {
		return "nil"
	}
	return fmt.Sprint(*percent)
}

func TestTheInvestmentIsMadeOnceItsTransferIsMarkedOrTheMonthIsClosed(t *testing.T) {
	rich, poor := paydayPlan(exampleLines(800000)...), paydayPlan(exampleLines(500000)...)
	mark := TransferMarks{InvestmentTransferKey: "2026-03-20"}
	for _, c := range []struct {
		label  string
		plan   Plan
		marks  TransferMarks
		closed bool
		want   money.Cents
	}{
		{"open, unmarked", rich, nil, false, 0},
		{"open, marked", rich, mark, false, 293000},
		{"open, another transfer marked", rich, TransferMarks{"line:14": "2026-03-20"}, false, 0},
		{"closed, unmarked", rich, nil, true, 293000},
		{"closed past its income", poor, nil, true, -7000},
		{"marked under a negative residual", poor, mark, false, 0},
	} {
		if got := MadeInvestment(c.plan, c.marks, c.closed); got != c.want {
			t.Errorf("%s: made %d, want %d", c.label, got, c.want)
		}
	}
}
