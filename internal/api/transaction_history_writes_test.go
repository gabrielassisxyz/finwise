package api

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// A note in the history of the bakery's pending purchase, the first step of a
// fresh database.
func noteOnThePendingPurchase(a authoring) error {
	return a.step(txPending, authored.StepNote, "O banco confirmou a compra", "")
}

// transactionHistoryWriteCases are the history's writes in the harness. The
// three budgets are the first three changes, so the first line change is the
// fourth and writes step 1.
var transactionHistoryWriteCases = []writeCase{
	{method: "transactions.setReason", arguments: `["tx-pending","é do apartamento"]`, expects: expectsRecorded, state: lineVersions,
		before: []writeCall{{"transactions.setLine", `["tx-pending","Feira"]`}}},
	{method: "transactions.setReason", arguments: `["tx-month-end",""]`, expects: expectsRecorded, state: lineVersions,
		before: []writeCall{{"transactions.setLine", `["tx-month-end","Mercado"]`}, {"transactions.setReason", `["tx-month-end","é do apartamento"]`}}},
	{method: "transactions.undoHistoryStep", arguments: `["tx-boleto","1"]`, expects: expectsRecorded,
		state:  states(lineVersions, overTheFullLedger(reviewsOfTheFullLedger)),
		before: []writeCall{{"transactions.setLine", `["tx-boleto","Feira"]`}, {"transactions.setReason", `["tx-boleto","é do apartamento"]`}}},
	{method: "transactions.undoHistoryStep", arguments: `["no-such-tx","1"]`, expects: expectsNothing, state: lineVersions},
	{method: "transactions.undoHistoryStep", arguments: `["tx-pending","no-such-step"]`, expects: expectsNothing, state: lineVersions,
		before: []writeCall{{"transactions.setLine", `["tx-pending","Feira"]`}}},
	{method: "transactions.undoHistoryStep", arguments: `["tx-month-end","1"]`, expects: expectsNothing, state: lineVersions,
		before: []writeCall{{"transactions.setLine", `["tx-pending","Feira"]`}}},
	{method: "transactions.undoHistoryStep", arguments: `["tx-pending","1"]`, expects: expectsNothing,
		state: states(lineVersions, overTheFullLedger(noteOnThePendingPurchase))},
	{method: "transactions.setReason", arguments: `["no-such-tx","é do apartamento"]`, expects: expectsRefusal, reason: reasonNotFound, state: lineVersions},
	{method: "transactions.undoHistoryStep", arguments: `["tx-pending","1"]`, expects: expectsRefusal, reason: reasonStaleChange, state: lineVersions,
		before: []writeCall{{"transactions.setLine", `["tx-pending","Feira"]`}, {"transactions.setLine", `["tx-pending","Delivery"]`}}},
}

func (env *writeEnv) giveReason(id, reason string) string {
	env.t.Helper()
	return changeIDOfAnswer(env.t, env.post("transactions.setReason", `["`+id+`","`+reason+`"]`))
}

func (env *writeEnv) undoStep(id, step string) string {
	env.t.Helper()
	return changeIDOfAnswer(env.t, env.post("transactions.undoHistoryStep", `["`+id+`","`+step+`"]`))
}

// The March purchase waits for a review, so its line change asks why; it has
// an older step of its own.
func TestAReasonStaysBesideTheLatestStepWithoutMakingItsChangeStale(t *testing.T) {
	env, fixture := newWarehouseEnv(t, lineVersions)
	authoredOver(t, env.scratch, fixture, func(a authoring) error {
		return a.step(warehouseMarch, authored.StepEffect, "Regra classificou como Delivery", "")
	})
	before := env.scratch.Canonical(t)
	origin := env.lineChange(warehouseMarch, "Feira").ChangeID
	if !env.panelOf(warehouseMarch).AskReason {
		t.Fatal("the line change of a pending purchase does not ask why")
	}
	env.giveReason(warehouseMarch, "é do apartamento")
	panel := env.panelOf(warehouseMarch)
	if panel.Reason != "é do apartamento" || len(panel.History) != 2 || !textIs(panel.History[0].Reason, "é do apartamento") || panel.History[1].Reason != nil {
		t.Errorf("the panel after the reason reads %q and %+v", panel.Reason, panel.History)
	}
	if answer := env.applyToEstablishment(origin); answer.Changed != 2 {
		t.Errorf("all purchases here after a reason answered %+v", answer)
	}
	expectUndone(t, "the extended change", env.undo(origin))
	if after := env.scratch.Canonical(t); after != before {
		t.Errorf("one undo after a reason and all purchases here left:\n%s", differences(before, after))
	}
}

func TestAReasonNeedsAStepToStayBeside(t *testing.T) {
	env := newLineEnv(t, newFullLedger(t), lineVersions)
	expectRefused(t, "an unknown transaction", env.post("transactions.setReason", `["no-such-tx","é do apartamento"]`), "not_found")
	expectRefused(t, "a tax row", env.post("transactions.setReason", `["tx-foreign-tax","é do apartamento"]`), "not_found")
	empty := env.giveReason(txBoleto, "é do apartamento")
	if effects := env.scratch.EffectsOf(t, changeNumber(t, empty)); len(effects) != 0 {
		t.Errorf("a reason for a transaction with no step recorded %+v", effects)
	}
	if panel := env.panelOf(txBoleto); panel.Reason != "" {
		t.Errorf("a reason with no step to hold it reads %q", panel.Reason)
	}
	env.lineChange(txBoleto, "Feira")
	env.giveReason(txBoleto, "é do apartamento")
	if same := env.giveReason(txBoleto, "é do apartamento"); len(env.scratch.EffectsOf(t, changeNumber(t, same))) != 0 {
		t.Error("the same reason again recorded an effect")
	}
	// An empty reason removes the step's: the table holds no empty text.
	cleared := env.giveReason(txBoleto, "")
	effects := env.scratch.EffectsOf(t, changeNumber(t, cleared))
	if len(effects) != 1 || effects[0].Table != "history_step_reasons" || effects[0].After != "" {
		t.Errorf("an empty reason recorded %+v, want the reason's row removed", effects)
	}
	if panel := env.panelOf(txBoleto); panel.Reason != "" || panel.History[0].Reason != nil {
		t.Errorf("after an empty reason the panel reads %q and %+v", panel.Reason, panel.History)
	}
}

func TestTakingALineStepBackRestoresWhatWasThereAndClosesTheQuestion(t *testing.T) {
	env := newLineEnv(t, newFullLedger(t), lineVersions)
	authoredOver(t, env.scratch, newFullLedger(t), reviewsOfTheFullLedger)
	before := env.storedAbout(txBoleto)
	original := env.lineChange(txBoleto, "Feira").ChangeID
	env.giveReason(txBoleto, "é do apartamento")
	withStep := env.scratch.Canonical(t)
	step := env.panelOf(txBoleto).History[0].ID
	undone := env.undoStep(txBoleto, step)
	after := env.storedAbout(txBoleto)
	if !sameKept(after, before) {
		t.Errorf("taking the step back left %+v, was %+v", after, before)
	}
	if panel := env.panelOf(txBoleto); panel.AskReason || len(panel.History) != 0 || panel.Pending == nil || panel.Pending.Suggestion == nil || *panel.Pending.Suggestion != "Feira" {
		t.Errorf("the panel after the step's undo reads %+v", panel)
	}
	expectUndone(t, "the step's undo", env.undo(undone))
	if now := env.scratch.Canonical(t); now != withStep {
		t.Errorf("undoing the step's undo does not bring back the step and its reason:\n%s", differences(withStep, now))
	}
	if panel := env.panelOf(txBoleto); len(panel.History) != 1 || panel.History[0].ID != step || !textIs(panel.History[0].Reason, "é do apartamento") {
		t.Errorf("the step came back as %+v", panel.History)
	}
	env.undoStep(txBoleto, step)
	expectRefused(t, "the original change after its step's undo", env.undo(original), "stale_change")

	// A second correction asks nothing, and taking it back puts the first one's
	// question back unless the question is closed by the undo itself.
	env.lineChange(txForeign, "Feira")
	env.lineChange(txForeign, "Delivery")
	second := env.panelOf(txForeign).History[0].ID
	env.undoStep(txForeign, second)
	if stored := env.storedAbout(txForeign).State; !lineIs(stored, marketLine) || stored.ClassifiedBy != authored.AuthorPerson || stored.AskReason {
		t.Errorf("taking the second correction back left %+v, want Feira by the person and no question", stored)
	}
}

func TestAStepChangedSinceIsRefusedAndTheHistorySaysSoFirst(t *testing.T) {
	env := newLineEnv(t, newFullLedger(t), lineVersions)
	env.lineChange(txInstalment, "Feira")
	env.lineChange(txInstalment, "Delivery")
	history := env.panelOf(txInstalment).History
	if len(history) != 2 || history[1].Undoable || !history[0].Undoable {
		t.Fatalf("the history before the refusal is %+v, want the older step not undoable", history)
	}
	everything := env.scratch.Everything(t)
	expectRefused(t, "a step overwritten since", env.post("transactions.undoHistoryStep", `["tx-instalment","`+history[1].ID+`"]`), "stale_change")
	if after := env.scratch.Everything(t); after != everything {
		t.Errorf("a refused step's undo changed the file:\n%s", differences(everything, after))
	}

	env.lineChange(txBoleto, "Feira")
	setNoteOf(t, env, txBoleto, "Conta de internet")
	if history := env.panelOf(txBoleto).History; len(history) != 1 || !history[0].Undoable {
		t.Fatalf("a step whose transaction only had its note changed since is %+v", history)
	} else {
		env.undoStep(txBoleto, history[0].ID)
	}
	if stored := env.storedAbout(txBoleto).State; stored.Line != nil || stored.Note != "Conta de internet" {
		t.Errorf("the step's undo left %+v, want no line and the later note", stored)
	}
}

func TestTakingBackOneStepOfAnExtendedChangeRestoresThatTransactionAlone(t *testing.T) {
	env, fixture := newWarehouseEnv(t, lineVersions)
	authoredOver(t, env.scratch, fixture, func(a authoring) error {
		return a.step(warehouseOrigin, authored.StepNote, "O banco confirmou a compra", "")
	})
	march := env.storedAbout(warehouseMarch)
	origin := env.lineChange(warehouseOrigin, "Feira").ChangeID
	env.applyToEstablishment(origin)
	marchStep := env.panelOf(warehouseMarch).History[0].ID
	februaryStep := env.panelOf(warehouseFebruary).History[0].ID
	note := env.panelOf(warehouseOrigin).History[1].ID

	before := env.scratch.Journal(t)
	for label, arguments := range map[string]string{
		"an unknown transaction":        `["no-such-tx","` + marchStep + `"]`,
		"a step id that is no number":   `["tx-atacado-marco","nope"]`,
		"a step id nobody wrote":        `["tx-atacado-marco","999"]`,
		"another transaction's step":    `["tx-atacado-marco","` + februaryStep + `"]`,
		"a note":                        `["tx-atacado-origem","` + note + `"]`,
		"a step id in another spelling": `["tx-atacado-marco","0` + marchStep + `"]`,
	} {
		if response := env.post("transactions.undoHistoryStep", arguments); response.Code != http.StatusOK || response.Body.String() != "null" {
			t.Errorf("%s: answered %d %s, want null", label, response.Code, response.Body.String())
		}
	}
	if after := env.scratch.Journal(t); after != before {
		t.Errorf("an answer of null wrote to the journal:\n%s", differences(before, after))
	}

	env.undoStep(warehouseMarch, marchStep)
	if now := env.storedAbout(warehouseMarch); !sameKept(now, march) {
		t.Errorf("the March purchase after its step's undo is %+v, was %+v", now, march)
	}
	for _, id := range []string{warehouseOrigin, warehouseFebruary} {
		if !lineIs(env.storedAbout(id).State, marketLine) {
			t.Errorf("%s lost the line when another transaction's step was taken back", id)
		}
	}
	if panel := env.panelOf(warehouseOrigin); panel.Counterparty == nil || !textIs(panel.Counterparty.DefaultLine, "Feira") {
		t.Errorf("the default line after one step's undo reads %+v", panel.Counterparty)
	}
}

func TestARulesStepIsTakenBackTheSameWay(t *testing.T) {
	env := newLineEnv(t, newFullLedger(t), lineVersions)
	snapshot := viewOf(t, newFullLedger(t).Path)
	facts := authored.ChangeFacts{Author: authored.AuthorRule, Method: "snapshot.classify", At: fixtureNow}
	err := env.scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		view := recorder.View()
		defer view.Close()
		target, _, err := lookupTarget(t.Context(), Writes{Ledger: snapshot, Authored: view, Recorder: recorder}, txBoleto)
		if err != nil {
			return err
		}
		return settleWithStep(t.Context(), recorder, target.key, marketLine, authored.AuthorRule, "Regra classificou como Feira")
	})
	if err != nil {
		t.Fatal(err)
	}
	history := env.panelOf(txBoleto).History
	if len(history) != 1 || !history[0].Undoable {
		t.Fatalf("the rule's step is %+v", history)
	}
	env.undoStep(txBoleto, history[0].ID)
	if stored := env.storedAbout(txBoleto).State; stored.Line != nil || stored.ClassifiedBy != "" {
		t.Errorf("taking the rule's step back left %+v", stored)
	}
}

// The writes the property test draws from, each with arguments drawn from the
// warehouse's transactions and the lines of its budgets.
var (
	propertyLines   = []string{"Feira", "Mercado", "Delivery", "Salário", authored.FeesLineName}
	propertyReasons = []string{"é do apartamento", "", "outra razão"}
)

type historyProperty struct {
	t       *testing.T
	env     *writeEnv
	random  *rand.Rand
	changes []string
}

func (p *historyProperty) anyTransaction() string {
	return warehouseRows[p.random.IntN(len(warehouseRows))].id
}

func (p *historyProperty) anyChange() string {
	if len(p.changes) == 0 {
		return "1"
	}
	return p.changes[p.random.IntN(len(p.changes))]
}

func (p *historyProperty) anyStepOf(id string) string {
	history := p.env.panelOf(id).History
	if len(history) == 0 {
		return "1"
	}
	return history[p.random.IntN(len(history))].ID
}

func pick[T any](random *rand.Rand, from []T) T { return from[random.IntN(len(from))] }

// write makes one of the seven calls, whatever it answers, and keeps the id of
// a change it made.
func (p *historyProperty) write() {
	p.t.Helper()
	var method, arguments string
	switch p.random.IntN(7) {
	case 0:
		method, arguments = "transactions.setLine", jsonArray(p.anyTransaction(), pick(p.random, propertyLines))
	case 1:
		method, arguments = "transactions.confirmLine", jsonArray(p.anyTransaction())
	case 2:
		method, arguments = "transactions.setReason", jsonArray(p.anyTransaction(), pick(p.random, propertyReasons))
	case 3:
		ids := []string{p.anyTransaction(), p.anyTransaction()}
		encoded, _ := json.Marshal([]any{ids, pick(p.random, propertyLines)})
		method, arguments = "transactions.setLineForMany", string(encoded)
	case 4:
		method, arguments = "transactions.applyLineToEstablishment", jsonArray(p.anyChange())
	case 5:
		id := p.anyTransaction()
		method, arguments = "transactions.undoHistoryStep", jsonArray(id, p.anyStepOf(id))
	default:
		method, arguments = "changes.undo", jsonArray(p.anyChange())
	}
	response := p.env.post(method, arguments)
	if response.Code != http.StatusOK && response.Code != http.StatusConflict {
		p.t.Fatalf("%s %s answered %d %s", method, arguments, response.Code, response.Body.String())
	}
	if id, answered := changeIDIn(response.Body.Bytes()); answered && response.Code == http.StatusOK {
		p.changes = append(p.changes, id)
	}
}

func jsonArray(values ...string) string {
	encoded, _ := json.Marshal(values)
	return string(encoded)
}

// agrees checks, for every step of every transaction, that the panel's
// undoable is whether taking that step back succeeds. A step's undo that
// succeeded is undone at once, so the state the walk goes on from is the same.
func (p *historyProperty) agrees(walk string) {
	p.t.Helper()
	for _, row := range warehouseRows {
		history := p.env.panelOf(row.id).History
		for _, entry := range history {
			before := p.env.scratch.Canonical(p.t)
			response := p.env.post("transactions.undoHistoryStep", jsonArray(row.id, entry.ID))
			id, succeeded := changeIDIn(response.Body.Bytes())
			succeeded = succeeded && response.Code == http.StatusOK
			if succeeded != entry.Undoable {
				p.t.Fatalf("%s: %s step %s %q reads undoable %v and its undo answered %d %s",
					walk, row.id, entry.ID, entry.Text, entry.Undoable, response.Code, response.Body.String())
			}
			if !succeeded {
				continue
			}
			p.onlyItsOwnUnits(walk, row.id, history, id)
			expectUndone(p.t, walk+": the step's undo", p.env.undo(id))
			if after := p.env.scratch.Canonical(p.t); after != before {
				p.t.Fatalf("%s: undoing a step's undo left:\n%s", walk, differences(before, after))
			}
		}
	}
}

// onlyItsOwnUnits checks that a step's undo wrote nothing but the units of
// its own transaction: its facts, tags and parts, and the steps of its history
// as it was before the undo, with their reasons.
func (p *historyProperty) onlyItsOwnUnits(walk, id string, history []historyEntry, change string) {
	p.t.Helper()
	view := p.env.scratch.Database.View()
	defer view.Close()
	keys, err := view.TransactionKeys(p.t.Context(), []string{id})
	if err != nil {
		p.t.Fatal(err)
	}
	owner := strconv.FormatInt(int64(keys[id]), 10)
	own := map[string]bool{}
	for _, entry := range history {
		own["["+entry.ID+"]"] = true
	}
	for _, effect := range p.env.scratch.EffectsOf(p.t, changeNumber(p.t, change)) {
		mine := false
		switch effect.Table {
		case "tx_state", "tx_tags", "tx_split_parts":
			mine = effect.Key == "["+owner+"]" || strings.HasPrefix(effect.Key, "["+owner+",")
		case "history_steps", "history_step_reasons":
			mine = own[effect.Key]
		}
		if !mine {
			p.t.Fatalf("%s: taking back a step of %s wrote %+v", walk, id, effect)
		}
	}
}

func TestEveryStepReadsUndoableExactlyWhenTakingItBackSucceeds(t *testing.T) {
	undoable, refused := 0, 0
	for seed := uint64(1); seed <= 6; seed++ {
		env, _ := newWarehouseEnv(t, lineVersions)
		p := &historyProperty{t: t, env: env, random: rand.New(rand.NewPCG(seed, 2026))}
		for move := range 16 {
			p.write()
			p.agrees(fmt.Sprintf("seed %d, move %d", seed, move))
		}
		for _, row := range warehouseRows {
			for _, entry := range env.panelOf(row.id).History {
				if entry.Undoable {
					undoable++
				} else {
					refused++
				}
			}
		}
	}
	t.Logf("the walks ended with %d undoable and %d refused steps", undoable, refused)
	// The walks must reach both answers, or the agreement says nothing.
	if undoable == 0 || refused == 0 {
		t.Errorf("the walks ended with %d undoable and %d refused steps; both must occur", undoable, refused)
	}
}
