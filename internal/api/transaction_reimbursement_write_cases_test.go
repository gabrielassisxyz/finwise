package api

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// reimbursementState is the invented state with February's market purchase
// marked reimbursable, the friend's PIX of January waiting for a review with a
// suggestion, and the instalment marked and split in two: no credit is linked
// in it, so every received PIX of the full ledger is a candidate.
func reimbursementState(t *testing.T, scratch *authoredtest.Scratch) {
	t.Helper()
	inventedAuthoredState(t, scratch)
	lines := lineIDsByName(t, scratch.Database)
	snapshot := viewOf(t, newFullLedger(t).Path)
	authorOver(t, scratch, snapshot, lines, firstDecisions, "scratch.seedReimbursements", func(a authoring) error {
		return a.reimbursementsToLink()
	})
}

func (a authoring) reimbursementsToLink() error {
	market, err := a.key(txMonthEnd)
	if err != nil {
		return err
	}
	instalment, err := a.key(txInstalment)
	if err != nil {
		return err
	}
	recorder := a.writes.Recorder
	parts := []authored.SplitPart{{Position: 0, Line: a.line("Viagens"), Cents: 6_000}, {Position: 1, Cents: 6_900}}
	return each(
		func() error { return recorder.MarkReimbursable(a.ctx, market, 10_350) },
		func() error {
			return a.state(txStale, authored.TransactionState{
				PendingReason: "Estabelecimento novo", PendingLine: a.line("Renda extra"), PendingConfidence: authoredtest.Some[int64](55),
			})
		},
		func() error { return recorder.MarkReimbursable(a.ctx, instalment, 6_450) },
		func() error { return authoredtest.WriteSplit(a.ctx, recorder, instalment, parts) },
	)
}

// reimbursementArguments are, for each of the four writes, what follows the
// purchase's id in a call that is refused for the purchase alone.
var reimbursementArguments = map[string]string{
	"transactions.setReimbursable":          `,true`,
	"transactions.setExpectedReimbursement": `,61.73`,
	"transactions.linkReimbursement":        `,"tx-stale"`,
	"transactions.unlinkReimbursement":      ``,
}

func notFoundReimbursementCases() []writeCase {
	var cases []writeCase
	for _, method := range []string{
		"transactions.setReimbursable", "transactions.setExpectedReimbursement",
		"transactions.linkReimbursement", "transactions.unlinkReimbursement",
	} {
		for _, id := range idsNoWriteFinds {
			cases = append(cases, writeCase{method: method, arguments: `["` + id + `"` + reimbursementArguments[method] + `]`,
				expects: expectsRefusal, reason: reasonNotFound, state: budgetState()})
		}
	}
	// Credits that are not candidates of the marked market purchase: one the
	// ledger never had, a debit, a bill paid from the card's balance, a card
	// refund and a transfer to an own account.
	for _, credit := range []string{"no-such-transaction", txPixOut, txCardPayment, txRefund, txOwnTransfer} {
		cases = append(cases, writeCase{method: "transactions.linkReimbursement", arguments: `["tx-month-end","` + credit + `"]`,
			expects: expectsRefusal, reason: reasonNotFound, state: reimbursementState})
	}
	return cases
}

// reimbursementWriteCases are the four writes of a purchase paid back by a
// received PIX, each recorded and undone, and their refusals. The link is
// made over a credit waiting for a review, which the link settles; the undo of
// an unlinking is refused once the credit pays another purchase.
var reimbursementWriteCases = append([]writeCase{
	{method: "transactions.setReimbursable", arguments: `["tx-first-of-march",true]`, expects: expectsRecorded, state: startFromTheInventedAuthorship},
	{method: "transactions.setReimbursable", arguments: `["tx-first-of-march",false]`, expects: expectsRecorded, state: startFromTheInventedAuthorship},
	{method: "transactions.setExpectedReimbursement", arguments: `["tx-first-of-march",61.73]`,
		expects: expectsRecorded, state: startFromTheInventedAuthorship},
	{method: "transactions.setExpectedReimbursement", arguments: `["tx-first-of-march",null]`,
		expects: expectsRecorded, state: startFromTheInventedAuthorship},
	{method: "transactions.linkReimbursement", arguments: `["tx-month-end","tx-stale"]`, expects: expectsRecorded, state: reimbursementState},
	{method: "transactions.unlinkReimbursement", arguments: `["tx-first-of-march"]`, expects: expectsRecorded, state: startFromTheInventedAuthorship},
	// The instalment is split; the salary is a candidate there.
	{method: "transactions.linkReimbursement", arguments: `["tx-instalment","tx-salary"]`,
		expects: expectsRefusal, reason: reasonSplitWithReimbursement, state: reimbursementState},
	{method: "transactions.setSplit", arguments: `["tx-first-of-march",[{"line":"Feira","amount":10},{"line":null,"amount":0}]]`,
		expects: expectsRefusal, reason: reasonSplitWithReimbursement, state: startFromTheInventedAuthorship},
	// The friend's PIX pays the coffee of March back.
	{method: "transactions.linkReimbursement", arguments: `["tx-foreign","tx-stale"]`,
		expects: expectsRefusal, reason: reasonNotFound, state: startFromTheInventedAuthorship},
	// Change 4 unlinks the market purchase; change 6 links its credit to another.
	{method: "changes.undo", arguments: `["4"]`, expects: expectsRefusal, reason: reasonStaleChange, state: reimbursementState,
		before: []writeCall{
			{"transactions.linkReimbursement", `["tx-month-end","tx-stale"]`},
			{"transactions.unlinkReimbursement", `["tx-month-end"]`},
			{"transactions.setReimbursable", `["tx-pending",true]`},
			{"transactions.linkReimbursement", `["tx-pending","tx-stale"]`},
		}},
}, notFoundReimbursementCases()...)
