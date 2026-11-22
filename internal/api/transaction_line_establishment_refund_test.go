package api

import (
	"fmt"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// What "all purchases here" leaves on a credit is judged from a credit origin,
// so the credits of the warehouse are in the origin's direction and the rule on
// refunds is the only thing that can keep one of them where it is or move it.
// The ledger calls warehouseRefund a refund and warehouseCreditToo an ordinary
// credit.

// personSaysRefund is the person's word on whether a transaction is a refund,
// written without a step so that what a test reads from its history is the
// rule's alone.
func personSaysRefund(id string, refund bool) func(a authoring) error {
	return func(a authoring) error {
		return a.state(id, authored.TransactionState{Refund: &refund})
	}
}

// placementOf is where a transaction stands in the panel: its line, who put it
// there, and the steps of its history, newest first.
func (env *writeEnv) placementOf(id string) string {
	env.t.Helper()
	panel := env.panelOf(id)
	steps := []string{}
	for _, step := range panel.History {
		steps = append(steps, step.Text)
	}
	return fmt.Sprintf("line %s by %s, steps %q", shownText(panel.Line), shownText(panel.ClassifiedBy), steps)
}

const (
	notPlaced = `line null by null, steps []`
	// movedByTheRule is how a credit stands after "all purchases here" moved it.
	movedByTheRule = `line "Feira" by "Regra", steps ["Regra aplicou Feira (você pediu para todas em ` + nicknameOfTheWarehouse + `)"]`
	// putByThePerson is how the origin stands: its line, by the person.
	putByThePerson = `line "Feira" by "Você", steps ["Você alterou para Feira"]`
)

// everyOtherTransactionOfTheWarehouse are the rows no case here may move: the
// purchases are not in a credit's direction and the bill payment is a movement.
var everyOtherTransactionOfTheWarehouse = []string{
	warehouseOrigin, warehouseMarch, warehouseFebruary, warehouseJanuary, warehouseBill,
	warehouseExcluded, warehouseSplit, warehouseForeign, warehouseOnLine,
}

// applyFromTheCredit puts the credit on the line and extends it to the
// establishment, over a warehouse where the person said what states set.
func applyFromTheCredit(t *testing.T, states ...func(a authoring) error) (*writeEnv, establishmentLineResult, map[string]authored.AuthoredTransaction) {
	t.Helper()
	env, fixture := newWarehouseEnv(t, lineVersions)
	for _, state := range states {
		authoredOver(t, env.scratch, fixture, state)
	}
	before := map[string]authored.AuthoredTransaction{}
	for _, id := range everyOtherTransactionOfTheWarehouse {
		before[id] = env.storedAbout(id)
	}
	origin := env.lineChange(warehouseCredit, "Feira")
	return env, env.applyToEstablishment(origin.ChangeID), before
}

func expectUnmoved(t *testing.T, env *writeEnv, before map[string]authored.AuthoredTransaction) {
	t.Helper()
	for id, was := range before {
		if now := env.storedAbout(id); !sameKept(now, was) {
			t.Errorf("%s moved: %+v, was %+v", id, now, was)
		}
	}
}

func expectPlacement(t *testing.T, env *writeEnv, label, id, want string) {
	t.Helper()
	if got := env.placementOf(id); got != want {
		t.Errorf("%s: %s stands at %s, want %s", label, id, got, want)
	}
}

// A credit the ledger does not call a refund stays where it is when the person
// marked it as one: the person's word on a refund decides over the ledger's.
func TestAllPurchasesHereLeavesACreditThePersonMarkedAsARefund(t *testing.T) {
	control, answer, _ := applyFromTheCredit(t)
	if answer.Changed != 1 {
		t.Fatalf("control: with no word on the credit, all purchases here answered %+v, want 1 moved", answer)
	}
	expectPlacement(t, control, "control", warehouseCreditToo, movedByTheRule)

	env, answer, before := applyFromTheCredit(t, personSaysRefund(warehouseCreditToo, true))
	if answer.Changed != 0 {
		t.Errorf("all purchases here answered %+v, want 0 moved: the only other credit is a refund by the person's word", answer)
	}
	expectPlacement(t, env, "marked as a refund", warehouseCreditToo, notPlaced)
	expectPlacement(t, env, "marked as a refund", warehouseRefund, notPlaced)
	expectPlacement(t, env, "marked as a refund", warehouseCredit, putByThePerson)
	expectUnmoved(t, env, before)
}

// A credit the ledger calls a refund goes on the line when the person said it is
// not one, and it is the person's word that decides, not the ledger's.
func TestAllPurchasesHereMovesACreditThePersonSaidIsNoRefund(t *testing.T) {
	control, answer, _ := applyFromTheCredit(t)
	if answer.Changed != 1 {
		t.Fatalf("control: with no word on the refund, all purchases here answered %+v, want 1 moved", answer)
	}
	expectPlacement(t, control, "control", warehouseRefund, notPlaced)

	env, answer, before := applyFromTheCredit(t, personSaysRefund(warehouseRefund, false))
	if answer.Changed != 2 {
		t.Errorf("all purchases here answered %+v, want 2 moved: the other credit and the one the person said is no refund", answer)
	}
	expectPlacement(t, env, "said to be no refund", warehouseRefund, movedByTheRule)
	expectPlacement(t, env, "said to be no refund", warehouseCreditToo, movedByTheRule)
	expectPlacement(t, env, "said to be no refund", warehouseCredit, putByThePerson)
	expectUnmoved(t, env, before)
}
