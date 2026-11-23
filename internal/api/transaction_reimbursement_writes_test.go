package api

import (
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// refusedWith is the body of a refusal for reason.
func refusedWith(reason refusalReason) string {
	return `{"error":{"kind":"refused","reason":"` + string(reason) + `"}}`
}

// expectRefused posts a write that must be refused for reason with no table of
// the file changed.
func (env *countEnv) expectRefused(reason refusalReason, method, arguments string) {
	env.t.Helper()
	before := env.scratch.Everything(env.t)
	if status, body := env.answer(method, arguments); status != http.StatusConflict || string(body) != refusedWith(reason) {
		env.t.Errorf("%s %s answered %d %s, want 409 %s", method, arguments, status, body, reason)
	}
	if after := env.scratch.Everything(env.t); after != before {
		env.t.Errorf("%s %s changed the file:\n%s", method, arguments, differences(before, after))
	}
}

// expectEmptyChange posts a write that must answer a change with no effect.
func (env *countEnv) expectEmptyChange(method, arguments string) {
	env.t.Helper()
	before := env.scratch.Canonical(env.t)
	id := env.change(method, arguments)
	if effects := env.scratch.EffectsOf(env.t, changeNumber(env.t, id)); len(effects) != 0 {
		env.t.Errorf("%s %s recorded %d effects, want none", method, arguments, len(effects))
	}
	if after := env.scratch.Canonical(env.t); after != before {
		env.t.Errorf("%s %s changed the journaled tables:\n%s", method, arguments, differences(before, after))
	}
}

func (env *countEnv) reimbursementOf(id string) string {
	env.t.Helper()
	return compactJSON(env.t, reimbursementDetailOf(env.t, env.readEnv, id).Reimbursement)
}

// linkDinnerTo marks the dinner of 180.00 and links credit to it.
func (env *countEnv) linkDinnerTo(credit string) {
	env.t.Helper()
	env.change("transactions.setReimbursable", `["`+txDinner+`",true]`)
	env.change("transactions.linkReimbursement", `["`+txDinner+`","`+credit+`"]`)
}

func TestHalfOfWhatWasPaidIsRoundedUpToTheCent(t *testing.T) {
	for amount, want := range map[money.Cents]money.Cents{-18_000: 9_000, -18_001: 9_001, -12_345: 6_173, -1: 1, 18_001: 9_001, 0: 0} {
		if got := halfOf(amount); got != want {
			t.Errorf("half of %d is %d, want %d", amount, got, want)
		}
	}
}

func TestMarkingExpectsHalfOfThePurchaseAndTheFieldSetsWhatIsExpected(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	for id, want := range map[string]money.Cents{txDinner: 9_000, txDinnerCent: 9_001, txBooks: 6_173} {
		env.change("transactions.setReimbursable", `["`+id+`",true]`)
		if state := env.stateOf(id); !state.Reimbursable || state.ExpectedCents == nil || *state.ExpectedCents != want {
			t.Errorf("marking %s stored %+v, want %d expected", id, state, want)
		}
	}
	steps := []struct{ argument, want string }{
		{"", `{"expected":90.00,"link":null}`},
		{"120", `{"expected":120.00,"link":null}`},
		{"null", `{"expected":null,"link":null}`},
	}
	for _, step := range steps {
		if step.argument != "" {
			env.change("transactions.setExpectedReimbursement", `["`+txDinner+`",`+step.argument+`]`)
		}
		if got := env.reimbursementOf(txDinner); got != step.want {
			t.Errorf("after %q the reimbursement is %s, want %s", step.argument, got, step.want)
		}
	}
	before := env.scratch.Everything(t)
	expectError(t, "a negative amount", env.call("/api/transactions/setExpectedReimbursement", `["`+txDinner+`",-0.01]`, env.cookie), http.StatusBadRequest, "bad_request")
	if after := env.scratch.Everything(t); after != before {
		t.Errorf("a negative amount changed the file:\n%s", differences(before, after))
	}
	env.expectEmptyChange("transactions.setExpectedReimbursement", `["`+txLunch+`",30]`)
	if got := env.reimbursementOf(txLunch); got != "null" {
		t.Errorf("the amount expected of a purchase not marked made its reimbursement %s", got)
	}
	env.change("transactions.setReimbursable", `["`+txDinner+`",false]`)
	if got := env.reimbursementOf(txDinner); got != "null" {
		t.Errorf("unmarking left the reimbursement %s", got)
	}
}

func TestLinkingAPIXTakesItOffThePurchaseAndOutOfEveryonesIncome(t *testing.T) {
	for _, onIncome := range []bool{false, true} {
		env := newReimbursementEnv(t, reimbursementLedger(t))
		env.putOnLine(txDinner, "Mercado")
		if onIncome {
			env.putOnLine(txPixAna, "Salário")
		}
		env.linkDinnerTo(txPixAna)
		detail := reimbursementDetailOf(t, env.readEnv, txDinner)
		want := `{"expected":90.00,"link":{"transactionId":"tx-pix-ana","name":"Ana Exemplo","day":"2026-03-12","amount":150.00}}`
		if got := compactJSON(t, detail.Reimbursement); got != want {
			t.Errorf("the reimbursement is %s, want %s", got, want)
		}
		if string(detail.EffectiveAmount) != "-30.00" || !detail.Reimbursed {
			t.Errorf("the dinner answers effective %s reimbursed %v, want -30.00 true", detail.EffectiveAmount, detail.Reimbursed)
		}
		if len(detail.History) == 0 || detail.History[0].Text != "Você ligou ao PIX de Ana Exemplo · R$\u00a0150,00" {
			t.Errorf("the newest step is %+v", detail.History)
		}
		for label, credit := range map[string]reimbursementView{
			"list": reimbursementRowOf(t, env.readEnv, "2026-03", txPixAna), "detail": reimbursementDetailOf(t, env.readEnv, txPixAna),
		} {
			if !credit.ReimbursementIncome || string(credit.EffectiveAmount) != "150.00" {
				t.Errorf("the PIX in the %s answers reimbursementIncome %v effective %s", label, credit.ReimbursementIncome, credit.EffectiveAmount)
			}
		}
		purchase := env.outcome("2026-03", txDinner)
		if purchase.Disposition != budgetrules.OnALine || purchase.AppliedReimbursement != 15_000 || purchase.Lines[0].Amount != 3_000 {
			t.Errorf("the dinner counts %+v, want 3000 on its line", purchase)
		}
		if credit := env.outcome("2026-03", txPixAna); credit.Disposition != budgetrules.Reimbursement || len(credit.Lines) != 0 {
			t.Errorf("the PIX, on an income line %v, counts %+v, want the reimbursement on no line", onIncome, credit)
		}
	}
}

func TestACreditAsLargeAsThePurchaseOrLargerLeavesItAtZeroAndTheExcessCountsNowhere(t *testing.T) {
	cases := []struct {
		credit, effective, linked string
		applied                   money.Cents
	}{
		{txPixAna, "-30.00", "150.00", 15_000},
		{txPixWhole, "0.00", "180.00", 18_000},
		{txPixMore, "0.00", "200.00", 18_000},
	}
	for _, c := range cases {
		fixture := reimbursementLedger(t)
		env := newReimbursementEnv(t, fixture)
		env.putOnLine(txDinner, "Mercado")
		env.linkDinnerTo(c.credit)
		detail := reimbursementDetailOf(t, env.readEnv, txDinner)
		if string(detail.EffectiveAmount) != c.effective || !detail.Reimbursed {
			t.Errorf("%s: the dinner answers effective %s reimbursed %v, want %s true", c.credit, detail.EffectiveAmount, detail.Reimbursed, c.effective)
		}
		if got := compactJSON(t, detail.Reimbursement); !strings.HasSuffix(got, `"amount":`+c.linked+`}}`) {
			t.Errorf("%s: the link answers %s, want the amount %s", c.credit, got, c.linked)
		}
		purchase := env.outcome("2026-03", txDinner)
		if purchase.AppliedReimbursement != c.applied || purchase.Lines[0].Amount != 18_000-c.applied {
			t.Errorf("%s: the dinner counts %+v, want %d applied", c.credit, purchase, c.applied)
		}
		if credit := env.outcome("2026-03", c.credit); credit.Disposition != budgetrules.Reimbursement || len(credit.Lines) != 0 || credit.Unassigned != 0 {
			t.Errorf("%s: the credit counts %+v, want nothing of it anywhere", c.credit, credit)
		}
		for _, problem := range monthInvariants(t, env.readEnv, env.scratch, fixture.Path) {
			t.Errorf("%s: %s", c.credit, problem)
		}
	}
}

func TestACreditPaysOnePurchaseOnly(t *testing.T) {
	for _, free := range []string{"unlinking", "unmarking"} {
		env := newReimbursementEnv(t, reimbursementLedger(t))
		env.linkDinnerTo(txPixAna)
		env.change("transactions.setReimbursable", `["`+txLunch+`",true]`)
		env.expectRefused(reasonNotFound, "transactions.linkReimbursement", `["`+txLunch+`","`+txPixAna+`"]`)
		if slices.Contains(reimbursementCandidateIDs(reimbursementDetailOf(t, env.readEnv, txLunch)), txPixAna) {
			t.Errorf("Ana's PIX, linked to the dinner, is a candidate of the lunch")
		}
		if free == "unlinking" {
			env.change("transactions.unlinkReimbursement", `["`+txDinner+`"]`)
		} else {
			env.change("transactions.setReimbursable", `["`+txDinner+`",false]`)
		}
		env.change("transactions.linkReimbursement", `["`+txLunch+`","`+txPixAna+`"]`)
		if state := env.stateOf(txLunch); state.Credit == nil || *state.CreditCents != 15_000 {
			t.Errorf("after %s the dinner, the lunch keeps %+v", free, state)
		}
	}
}

func TestMarkingALinkedPurchaseAgainStartsOverAndFreesItsCredit(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	env.linkDinnerTo(txPixAna)
	env.change("transactions.setExpectedReimbursement", `["`+txDinner+`",150]`)
	env.change("transactions.setReimbursable", `["`+txDinner+`",true]`)
	if got := env.reimbursementOf(txDinner); got != `{"expected":90.00,"link":null}` {
		t.Errorf("marking the linked dinner again answers %s", got)
	}
	if !slices.Contains(reimbursementCandidateIDs(reimbursementDetailOf(t, env.readEnv, txLunch)), txPixAna) {
		t.Error("the credit of the dinner marked again is not a candidate of the lunch")
	}
}

func TestLinkingChangesNothingOnWhatIsNotAMarkedPurchaseAndRefusesWhatIsNotACandidate(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	for _, id := range []string{txLunch, txSalary, txOwnTransfer} {
		env.expectEmptyChange("transactions.linkReimbursement", `["`+id+`","`+txPixAna+`"]`)
	}
	env.change("transactions.setReimbursable", `["`+txOwnTransfer+`",true]`)
	env.expectEmptyChange("transactions.linkReimbursement", `["`+txOwnTransfer+`","`+txPixAna+`"]`)
	for _, id := range idsNoWriteFinds {
		env.expectRefused(reasonNotFound, "transactions.linkReimbursement", `["`+id+`","`+txPixAna+`"]`)
	}
	env.change("transactions.setReimbursable", `["`+txDinner+`",true]`)
	for _, credit := range []string{"no-such-transaction", txLunch, txPayroll, txOwnTransfer, txPixOwn, txCardPayment, txForeignTax} {
		env.expectRefused(reasonNotFound, "transactions.linkReimbursement", `["`+txDinner+`","`+credit+`"]`)
	}
}

// A split purchase is refused before its credit is looked at, and a linked
// purchase's split before its lines are: each refusal names what the purchase
// is, whatever else is wrong with the call.
func TestASplitPurchaseTakesNoLinkAndALinkedPurchaseNoSplit(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	twoParts := `[{"line":"Transporte","amount":100},{"line":null,"amount":80}]`
	env.change("transactions.setReimbursable", `["`+txDinner+`",true]`)
	env.change("transactions.setSplit", `["`+txDinner+`",`+twoParts+`]`)
	env.expectRefused(reasonSplitWithReimbursement, "transactions.linkReimbursement", `["`+txDinner+`","`+txPixAna+`"]`)
	env.expectRefused(reasonSplitWithReimbursement, "transactions.linkReimbursement", `["`+txDinner+`","`+txPayroll+`"]`)

	env.change("transactions.setReimbursable", `["`+txLunch+`",true]`)
	env.change("transactions.linkReimbursement", `["`+txLunch+`","`+txPixAna+`"]`)
	env.expectRefused(reasonSplitWithReimbursement, "transactions.setSplit", `["`+txLunch+`",[{"line":"Transporte","amount":30},{"line":null,"amount":30}]]`)
	env.expectRefused(reasonSplitWithReimbursement, "transactions.setSplit", `["`+txLunch+`",[{"line":"Inexistente","amount":30},{"line":null,"amount":30}]]`)
	for _, removal := range []string{`null`, `[]`, `[{"line":"Transporte","amount":60}]`} {
		env.change("transactions.setSplit", `["`+txLunch+`",`+removal+`]`)
	}
	if state := env.stateOf(txLunch); state.Credit == nil {
		t.Error("removing the split of a linked purchase dropped its link")
	}
}

func TestLinkingSettlesTheCreditsReviewItsUndoBringsTheReviewBackAndTakingTheStepBackDoesNot(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	review := `{"reason":"Estabelecimento novo","suggestion":"Renda extra","confidence":55,"justification":null}`
	err := inChange(t, env.scratch, env.ledger, func(writes Writes) error {
		target, err := targetNamedBy(t.Context(), writes, txPixAna)
		if err != nil {
			return err
		}
		line := budgetLineID(t, env.scratch.Database, "2026-03", "Renda extra")
		return authoredtest.WriteState(t.Context(), writes.Recorder, target.key, authored.TransactionState{
			PendingReason: "Estabelecimento novo", PendingLine: &line, PendingConfidence: authoredtest.Some[int64](55),
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := compactJSON(t, reimbursementDetailOf(t, env.readEnv, txPixAna).Pending); got != review {
		t.Fatalf("the PIX waits with %s, want %s", got, review)
	}
	env.change("transactions.setReimbursable", `["`+txDinner+`",true]`)
	linked := env.change("transactions.linkReimbursement", `["`+txDinner+`","`+txPixAna+`"]`)
	if got := string(reimbursementDetailOf(t, env.readEnv, txPixAna).Pending); got != "null" {
		t.Errorf("the linked PIX still waits: %s", got)
	}
	env.undo(linked)
	if got := compactJSON(t, reimbursementDetailOf(t, env.readEnv, txPixAna).Pending); got != review {
		t.Errorf("after the undo of the link the PIX waits with %s, want %s", got, review)
	}
	if got := env.reimbursementOf(txDinner); got != `{"expected":90.00,"link":null}` {
		t.Errorf("after the undo of the link the dinner answers %s", got)
	}

	env.change("transactions.linkReimbursement", `["`+txDinner+`","`+txPixAna+`"]`)
	key, step := env.newestStep(txDinner)
	units := authored.TransactionUnits(key, []authored.StepID{step.ID})
	if can, err := env.scratch.Database.CanRestorePart(t.Context(), step.Change, units); err != nil || !can {
		t.Fatalf("the check says the link's step can be taken back: %v, %v", can, err)
	}
	if err := inChange(t, env.scratch, env.ledger, func(writes Writes) error {
		return writes.Recorder.RestorePart(t.Context(), step.Change, units)
	}); err != nil {
		t.Fatal(err)
	}
	if got := env.reimbursementOf(txDinner); got != `{"expected":90.00,"link":null}` {
		t.Errorf("taking the link's step back left the dinner %s", got)
	}
	if got := string(reimbursementDetailOf(t, env.readEnv, txPixAna).Pending); got != "null" {
		t.Errorf("taking the link's step back brought the PIX's review back: %s", got)
	}
}

func TestUnlinkingKeepsTheExpectedAmountAndTheLineGetsTheWholePurchaseBack(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	env.putOnLine(txDinner, "Mercado")
	env.linkDinnerTo(txPixAna)
	env.change("transactions.unlinkReimbursement", `["`+txDinner+`"]`)
	detail := reimbursementDetailOf(t, env.readEnv, txDinner)
	if got := compactJSON(t, detail.Reimbursement); got != `{"expected":90.00,"link":null}` || detail.Reimbursed {
		t.Errorf("after unlinking the dinner answers %s, reimbursed %v", got, detail.Reimbursed)
	}
	if len(detail.History) == 0 || detail.History[0].Text != "Você desfez a ligação com o PIX" {
		t.Errorf("the newest step is %+v", detail.History)
	}
	if outcome := env.outcome("2026-03", txDinner); outcome.AppliedReimbursement != 0 || outcome.Lines[0].Amount != 18_000 {
		t.Errorf("after unlinking the dinner counts %+v", outcome)
	}
	steps := len(detail.History)
	env.expectEmptyChange("transactions.unlinkReimbursement", `["`+txDinner+`"]`)
	if got := len(reimbursementDetailOf(t, env.readEnv, txDinner).History); got != steps {
		t.Errorf("unlinking a purchase with no link wrote a step: %d steps, were %d", got, steps)
	}
}

func TestLinkingAnotherCreditReplacesTheLinkAndFreesTheFirst(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	env.linkDinnerTo(txPixAna)
	env.change("transactions.linkReimbursement", `["`+txDinner+`","`+txPixWhole+`"]`)
	if state := env.stateOf(txDinner); *state.CreditCents != 18_000 {
		t.Errorf("the dinner links %+v", state)
	}
	if row := reimbursementRowOf(t, env.readEnv, "2026-03", txPixAna); row.ReimbursementIncome {
		t.Error("the replaced PIX still answers reimbursementIncome")
	}
	if !slices.Contains(reimbursementCandidateIDs(reimbursementDetailOf(t, env.readEnv, txLunch)), txPixAna) {
		t.Error("the replaced PIX is not a candidate of the lunch")
	}
	env.expectEmptyChange("transactions.linkReimbursement", `["`+txDinner+`","`+txPixWhole+`"]`)
}

func TestAnUndoThatWouldGiveOneCreditTwoPurchasesIsRefusedAsStale(t *testing.T) {
	for _, away := range []writeCall{
		{"transactions.unlinkReimbursement", `["tx-dinner"]`}, {"transactions.setReimbursable", `["tx-dinner",false]`},
	} {
		env := newReimbursementEnv(t, reimbursementLedger(t))
		env.linkDinnerTo(txPixAna)
		taken := env.change(away.method, away.arguments)
		env.change("transactions.setReimbursable", `["`+txLunch+`",true]`)
		env.change("transactions.linkReimbursement", `["`+txLunch+`","`+txPixAna+`"]`)
		canonical, journal := env.scratch.Canonical(t), env.scratch.Journal(t)
		dinner, lunch := env.reimbursementOf(txDinner), env.reimbursementOf(txLunch)
		if status, body := env.answer("changes.undo", `["`+taken+`"]`); status != http.StatusConflict || string(body) != refusedWith(reasonStaleChange) {
			t.Errorf("the undo of %s answered %d %s", away.method, status, body)
		}
		if env.scratch.Canonical(t) != canonical || env.scratch.Journal(t) != journal {
			t.Errorf("the refused undo of %s changed the database", away.method)
		}
		if env.reimbursementOf(txDinner) != dinner || env.reimbursementOf(txLunch) != lunch {
			t.Errorf("after the refused undo of %s the purchases answer %s and %s", away.method, env.reimbursementOf(txDinner), env.reimbursementOf(txLunch))
		}
	}
}

func TestEachWriteAnswersLedgerUnavailableWithNoSnapshotAndAClosedMonthAcceptsEach(t *testing.T) {
	writes := []writeCall{
		{"transactions.setReimbursable", `["tx-dinner",true]`},
		{"transactions.setExpectedReimbursement", `["tx-dinner",61.73]`},
		{"transactions.linkReimbursement", `["tx-dinner","tx-pix-ana"]`},
		{"transactions.unlinkReimbursement", `["tx-dinner"]`},
	}
	scratch := authoredtest.Open(t, fixtureClock)
	budgetToSplitIn(t, scratch)
	missing := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "absent.db"), fixtureNow), withDatabase(scratch.Database))
	cookie := missing.unlock()
	before := scratch.Everything(t)
	for _, call := range writes {
		domain, name := splitMethod(t, call.method)
		expectError(t, call.method, missing.call("/api/"+domain+"/"+name, call.arguments, cookie), http.StatusServiceUnavailable, "ledger_unavailable")
	}
	if after := scratch.Everything(t); after != before {
		t.Errorf("a write over a missing snapshot changed the file:\n%s", differences(before, after))
	}

	env := newReimbursementEnv(t, reimbursementLedger(t))
	authoredtest.CloseMonth(t, env.scratch.Database, fixtureNow, "2026-03", "2026-03-05")
	for _, call := range writes {
		if id := env.change(call.method, call.arguments); len(env.scratch.EffectsOf(t, changeNumber(t, id))) == 0 {
			t.Errorf("%s %s on a purchase of a closed month changed nothing", call.method, call.arguments)
		}
	}
}

// An unconverted purchase counts for nothing until the bank states its value
// in reais; linking a credit to it changes nothing, so no figure ever takes
// reais off an amount in euros and the credit stays someone's income.
func TestAnUnconvertedPurchaseTakesNoLink(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	env.change("transactions.setReimbursable", `["`+txUnconverted+`",true]`)
	env.expectEmptyChange("transactions.linkReimbursement", `["`+txUnconverted+`","`+txPixAna+`"]`)
	detail := reimbursementDetailOf(t, env.readEnv, txUnconverted)
	if detail.Reimbursed || string(detail.EffectiveAmount) != string(detail.Amount) || len(detail.Candidates) == 0 {
		t.Errorf("the unconverted purchase answers reimbursed %v, effective %s of %s, %d candidates", detail.Reimbursed, detail.EffectiveAmount, detail.Amount, len(detail.Candidates))
	}
	if reimbursementRowOf(t, env.readEnv, "2026-03", txPixAna).ReimbursementIncome {
		t.Error("the PIX offered to the unconverted purchase is no one's income")
	}
}

// While the ledger no longer lists a linked PIX no other purchase can take it,
// and once it is listed again it is still the first purchase's.
func TestACreditTheLedgerStoppedListingIsNoOnesToLinkAndListedAgainIsStillTaken(t *testing.T) {
	listed, unlisted, listedAgain := reimbursementLedger(t), reimbursementLedger(t, txPixAna), reimbursementLedger(t)
	env := newReimbursementEnv(t, listed)
	env.linkDinnerTo(txPixAna)
	whileUnlisted := env.snapshotOver(t, unlisted)
	if status, body := whileUnlisted.answer("transactions.setReimbursable", `["`+txLunch+`",true]`); status != http.StatusOK {
		t.Fatalf("marking the lunch answered %d %s", status, body)
	}
	if status, body := whileUnlisted.answer("transactions.linkReimbursement", `["`+txLunch+`","`+txPixAna+`"]`); status != http.StatusConflict ||
		string(body) != refusedWith(reasonNotFound) {
		t.Errorf("linking the PIX the ledger no longer lists answered %d %s", status, body)
	}
	if got := reimbursementCandidateIDs(reimbursementDetailOf(t, env.snapshotOver(t, listedAgain), txLunch)); slices.Contains(got, txPixAna) {
		t.Errorf("listed again, the dinner's PIX is a candidate of the lunch: %v", got)
	}
}

// The name of a PIX is the one shown for its establishment key: a nickname
// the person gave the payer names the candidate, the link and the link's step.
func TestANicknameNamesTheCandidateTheLinkAndTheLinksStep(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	env.change("transactions.setNickname", `["`+txPixAna+`","Aninha Exemplo"]`)
	named := ""
	for _, candidate := range reimbursementDetailOf(t, env.readEnv, txDinner).Candidates {
		if candidate.TransactionID == txPixAna {
			named = candidate.Name
		}
	}
	if named != "Aninha Exemplo" {
		t.Errorf("Ana's PIX is offered as %q, want its nickname", named)
	}
	env.linkDinnerTo(txPixAna)
	detail := reimbursementDetailOf(t, env.readEnv, txDinner)
	if got := compactJSON(t, detail.Reimbursement); !strings.Contains(got, `"name":"Aninha Exemplo"`) {
		t.Errorf("the link answers %s", got)
	}
	if detail.History[0].Text != "Você ligou ao PIX de Aninha Exemplo · R$\u00a0150,00" {
		t.Errorf("the link's step is %q", detail.History[0].Text)
	}
}

// Marking works on any listed transaction, a credit included, but only a
// purchase is paid back: linking a PIX to a marked received PIX or to a marked
// card refund changes nothing, and the PIX offered stays someone's income.
func TestLinkingToAMarkedCreditChangesNothingAndThePIXStaysIncome(t *testing.T) {
	for _, credit := range []string{txPixWhole, txRefund} {
		env := newReimbursementEnv(t, reimbursementLedger(t))
		env.putOnLine(txPixAna, "Salário")
		env.change("transactions.setReimbursable", `["`+credit+`",true]`)
		if got := env.reimbursementOf(credit); got == "null" {
			t.Fatalf("%s: marking the credit kept nothing", credit)
		}

		env.expectEmptyChange("transactions.linkReimbursement", `["`+credit+`","`+txPixAna+`"]`)

		for _, id := range []string{credit, txPixAna} {
			if texts := env.countedPanelOf(id).texts(); len(texts) != 0 {
				t.Errorf("%s: the history of %s is %v, want no step", credit, id, texts)
			}
		}
		if row := reimbursementRowOf(t, env.readEnv, "2026-03", txPixAna); row.ReimbursementIncome || string(row.EffectiveAmount) != "150.00" {
			t.Errorf("%s: the PIX is listed with reimbursementIncome %v and effective %s", credit, row.ReimbursementIncome, row.EffectiveAmount)
		}
		salary := budgetrules.LineID(budgetLineID(t, env.scratch.Database, "2026-03", "Salário"))
		want := []budgetrules.LineAmount{{Line: salary, Effect: budgetrules.Received, Amount: 15_000}}
		if outcome := env.outcome("2026-03", txPixAna); outcome.Disposition != budgetrules.Income || !slices.Equal(outcome.Lines, want) {
			t.Errorf("%s: the PIX counts %+v, want 15000 received on Salário", credit, outcome)
		}
		env.change("transactions.setReimbursable", `["`+txDinner+`",true]`)
		if !slices.Contains(reimbursementCandidateIDs(reimbursementDetailOf(t, env.readEnv, txDinner)), txPixAna) {
			t.Errorf("%s: the PIX is not among the candidates of the marked dinner", credit)
		}
	}
}
