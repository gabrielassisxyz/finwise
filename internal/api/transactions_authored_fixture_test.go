package api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// What the person said about the transactions of the full ledger, over the
// budget of the invented authored state. Every text and figure is invented.
const (
	nicknameOfTheBakery = "Padaria do Bairro"
	keyOfTheBakery      = "text:PADARIA EXEMPLO"
	keyOfTheMarket      = "text:MERCADO EXEMPLO"
	keyOfTheTelecom     = "doc:55566677000188"
	// plannedLine is in the budget from 2026-04 only, so the current month
	// lacks it.
	plannedLine = "Lanches"

	reasonOfTheFirstStep  = "Pedi pelo aplicativo"
	reasonOfTheSecondStep = "Era a compra do mês"
)

// The two changes of the person: the first decisions, and a correction a
// week later that overwrites one of them.
var (
	firstDecisions  = utc(2026, time.March, 8, 15, 0)
	laterCorrection = utc(2026, time.March, 15, 15, 0)
)

// inventedTransactionAuthorship adds to the invented authored state what a
// person would have said about the full ledger's transactions: a line put by
// the person and one by a rule, one by the model, a pending review with a
// suggestion, a nickname, own tags and fixed tags that overlap, a note,
// default lines the current month has and lacks, a split of three parts that
// lack part of what was paid and one of two that exceed it, an exclusion with a
// reason and one without, a credit on its own line marked as
// a refund of another, a refund the person says is not one, a reimbursable
// purchase with a credit linked to it and one with none, and a history with a reason, a step
// overwritten since and a note. Each transaction is found with the lookup every write
// uses, and tx-last-of-february is left out of the index on purpose: it has
// only what its establishment has.
func inventedTransactionAuthorship(t testing.TB, scratch *authoredtest.Scratch, fixture *ledgertest.Fixture) {
	t.Helper()
	planALineForApril(t, scratch)
	lines := lineIDsByName(t, scratch.Database)
	snapshot := viewOf(t, fixture.Path)
	authorOver(t, scratch, snapshot, lines, firstDecisions, "scratch.seedTransactions", func(a authoring) error {
		return each(a.linesAndReviews, a.establishments, a.tagsAndNote, a.splitAndExclusions, a.refundAndReimbursement, a.firstStepOfTheMarket,
			a.reimbursementsToSeed)
	})
	authorOver(t, scratch, snapshot, lines, laterCorrection, "scratch.correctLine", func(a authoring) error {
		return a.laterCorrections()
	})
}

// planALineForApril saves a budget from 2026-04 with one line more, which the
// months before it do not have.
func planALineForApril(t testing.TB, scratch *authoredtest.Scratch) {
	t.Helper()
	view := scratch.Database.View()
	defer view.Close()
	march, err := view.BudgetOf(t.Context(), "2026-03")
	if err != nil {
		t.Fatal(err)
	}
	document := authoredtest.DocumentOf(march)
	for i := range document {
		if document[i].Key == groupVariable {
			document[i].Lines = append(document[i].Lines, authoredtest.Line(plannedLine, 15_000))
		}
	}
	authoredtest.SaveBudget(t, scratch.Database, firstDecisions, "2026-04", document)
}

// lineIDsByName is every line of the invented budgets by each name it has had.
func lineIDsByName(t testing.TB, database *authored.Database) map[string]authored.LineID {
	t.Helper()
	view := database.View()
	defer view.Close()
	ids := map[string]authored.LineID{}
	for _, month := range []brday.Month{"2026-02", "2026-04"} {
		budget, err := view.BudgetOf(t.Context(), month)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range budget.Lines {
			ids[line.Name] = line.ID
		}
	}
	return ids
}

// authoring is one change being recorded over the fixture ledger.
type authoring struct {
	ctx    context.Context
	writes Writes
	lines  map[string]authored.LineID
}

func authorOver(t testing.TB, scratch *authoredtest.Scratch, snapshot *ledger.View, lines map[string]authored.LineID, at time.Time, method string, write func(authoring) error) {
	t.Helper()
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: method, At: at}
	err := scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		view := recorder.View()
		defer view.Close()
		return write(authoring{ctx: t.Context(), writes: Writes{Ledger: snapshot, Authored: view, Recorder: recorder}, lines: lines})
	})
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
}

func each(writes ...func() error) error {
	for _, write := range writes {
		if err := write(); err != nil {
			return err
		}
	}
	return nil
}

func (a authoring) key(id string) (authored.TxKey, error) {
	target, found, err := lookupTarget(a.ctx, a.writes, id)
	if err == nil && !found {
		err = fmt.Errorf("%s is not listed", id)
	}
	return target.key, err
}

func (a authoring) line(name string) *authored.LineID {
	id, found := a.lines[name]
	if !found {
		panic("no invented line is called " + name)
	}
	return &id
}

func (a authoring) state(id string, state authored.TransactionState) error {
	key, err := a.key(id)
	if err != nil {
		return err
	}
	return authoredtest.WriteState(a.ctx, a.writes.Recorder, key, state)
}

// step adds a step to the history of id, with its reason when there is one.
func (a authoring) step(id string, kind authored.StepKind, text, reason string) error {
	key, err := a.key(id)
	if err != nil {
		return err
	}
	step, err := a.writes.Recorder.AddHistoryStep(a.ctx, key, kind, text)
	if err != nil || reason == "" {
		return err
	}
	return authoredtest.GiveReason(a.ctx, a.writes.Recorder, step, reason)
}

func (a authoring) linesAndReviews() error {
	fees := feesLineID
	return each(
		func() error {
			return a.state(txSalary, authored.TransactionState{Line: a.line("Salário"), ClassifiedBy: authored.AuthorPerson})
		},
		func() error { return a.step(txSalary, authored.StepEffect, "Você colocou em Salário", "") },
		func() error {
			return a.state(txFee, authored.TransactionState{Line: &fees, ClassifiedBy: authored.AuthorRule})
		},
		func() error {
			return a.state(txBoleto, authored.TransactionState{
				PendingReason: "Confiança abaixo do limite", PendingLine: a.line("Contas da casa"), PendingConfidence: authoredtest.Some[int64](64),
			})
		},
	)
}

func (a authoring) establishments() error {
	recorder := a.writes.Recorder
	return each(
		func() error {
			return authoredtest.WriteEstablishment(a.ctx, recorder, keyOfTheBakery, nicknameOfTheBakery, a.line(plannedLine))
		},
		func() error { return authoredtest.AddFixedTag(a.ctx, recorder, keyOfTheBakery, "#lanche") },
		func() error { return authoredtest.AddFixedTag(a.ctx, recorder, keyOfTheBakery, "#Trabalho") },
		func() error {
			return authoredtest.WriteEstablishment(a.ctx, recorder, keyOfTheMarket, "", a.line("Mercado"))
		},
		func() error {
			return authoredtest.WriteEstablishment(a.ctx, recorder, keyOfTheTelecom, "", a.line("Contas da casa"))
		},
	)
}

func (a authoring) tagsAndNote() error {
	pending, err := a.key(txPending)
	if err != nil {
		return err
	}
	firstOfMarch, err := a.key(txFirstOfMarch)
	if err != nil {
		return err
	}
	recorder := a.writes.Recorder
	return each(
		func() error { return authoredtest.AddTag(a.ctx, recorder, pending, "#manhã") },
		func() error { return authoredtest.AddTag(a.ctx, recorder, pending, "#lanche") },
		func() error {
			return authoredtest.WriteState(a.ctx, recorder, pending, authored.TransactionState{Note: "Café com a equipe"})
		},
		func() error { return authoredtest.AddTag(a.ctx, recorder, firstOfMarch, "#água") },
	)
}

func (a authoring) splitAndExclusions() error {
	instalment, err := a.key(txInstalment)
	if err != nil {
		return err
	}
	parts := []authored.SplitPart{
		{Position: 0, Line: a.line("Viagens"), Cents: 6_000},
		{Position: 1, Line: a.line("Saúde"), Cents: 4_000},
		{Position: 2, Cents: 2_000},
	}
	boleto, err := a.key(txBoleto)
	if err != nil {
		return err
	}
	exceeding := []authored.SplitPart{{Position: 0, Line: a.line("Contas da casa"), Cents: 10_000}, {Position: 1, Cents: 2_000}}
	return each(
		func() error { return authoredtest.WriteSplit(a.ctx, a.writes.Recorder, instalment, parts) },
		func() error { return authoredtest.WriteSplit(a.ctx, a.writes.Recorder, boleto, exceeding) },
		func() error {
			return a.state(txPixOut, authored.TransactionState{Exclusion: authoredtest.Some("Empréstimo, ela devolve")})
		},
		func() error {
			return a.state(txForeign, authored.TransactionState{
				Line: a.line("Viagens"), ClassifiedBy: authored.AuthorModel, Confidence: authoredtest.Some[int64](91),
				Exclusion: authoredtest.Some(""),
			})
		},
	)
}

// refundAndReimbursement says the card's own refund is not one, and links the
// friend's PIX of January to the coffee of March as its reimbursement: the
// credit is larger than the purchase, so what it takes off is capped.
func (a authoring) refundAndReimbursement() error {
	credit, found, err := lookupTarget(a.ctx, a.writes, txStale)
	if err == nil && !found {
		err = fmt.Errorf("%s is not listed", txStale)
	}
	if err != nil {
		return err
	}
	return each(
		func() error { return a.state(txRefund, authored.TransactionState{Refund: authoredtest.Some(false)}) },
		func() error {
			return a.state(txFirstOfMarch, authored.TransactionState{
				Line: a.line("Mercado"), ClassifiedBy: authored.AuthorPerson, AskReason: true,
				Reimbursable: true, ExpectedCents: authoredtest.Some[money.Cents](500),
				Credit: &credit.key, CreditCents: authoredtest.Some(credit.listed.Amount),
			})
		},
	)
}

func (a authoring) firstStepOfTheMarket() error {
	return each(
		func() error {
			return a.state(txMonthEnd, authored.TransactionState{Line: a.line("Delivery"), ClassifiedBy: authored.AuthorPerson})
		},
		func() error {
			return a.step(txMonthEnd, authored.StepEffect, "Você colocou em Delivery", reasonOfTheFirstStep)
		},
	)
}

// laterCorrections moves the market's purchase to another line, and marks the
// salary, which the person had put on Salário, as a refund that takes off
// Viagens: the company paying a trip back.
func (a authoring) laterCorrections() error {
	return each(
		a.correctTheMarket,
		func() error {
			return a.state(txSalary, authored.TransactionState{
				Line: a.line("Salário"), ClassifiedBy: authored.AuthorPerson, Refund: authoredtest.Some(true), RefundLine: a.line("Viagens"),
			})
		},
	)
}

func (a authoring) correctTheMarket() error {
	return each(
		func() error {
			return a.state(txMonthEnd, authored.TransactionState{Line: a.line("Mercado"), ClassifiedBy: authored.AuthorPerson})
		},
		func() error {
			return a.step(txMonthEnd, authored.StepEffect, "Você alterou para Mercado", reasonOfTheSecondStep)
		},
		func() error { return a.step(txMonthEnd, authored.StepNote, "O banco juntou o IOF à compra", "") },
	)
}
