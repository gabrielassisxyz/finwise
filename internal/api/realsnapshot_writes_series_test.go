package api

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// The series of writes, run over a real snapshot and over the invented ledger
// alike. Each step takes its target by position from a list the API answers,
// the first, the middle or the last of those of one shape, and never searches
// for a name, an amount or an id, so no value of a real ledger is in this file.
// The arguments are the series' own words below, or halves of an amount the
// answer just gave, in cents.

// The series' own words: every text a step sends. The test that reads every
// message of the checks looks for each of them.
const (
	seriesIncomeLine       = "Ordenado da série"
	seriesFixedLine        = "Moradia da série"
	seriesVariableLine     = "Mercado da série"
	seriesOtherLine        = "Lazer da série"
	seriesProvisionLine    = "Provisão da série"
	seriesReserveLine      = "Reserva da série"
	seriesNewLine          = "Linha nova da série"
	seriesTag              = "#etiqueta-da-serie"
	seriesBatchTag         = "#lote-da-serie"
	seriesFixedTag         = "#fixa-da-serie"
	seriesSecondFixedTag   = "#fixa-dois-da-serie"
	seriesNote             = "Nota da série"
	seriesNickname         = "Apelido da série"
	seriesReason           = "Motivo da série"
	seriesScenario         = "Cenário da série"
	seriesSubscriptionNote = "Nota da assinatura da série"
	seriesAcceptNickname   = "Apelido aceito da série"
	seriesAcceptReason     = "Motivo aceito da série"
	seriesSettingsNickname = "Apelido do ajuste da série"
	seriesCardLabel        = "Cartão da série"
	seriesExclusionReason  = "Fora da conta pela série"
	seriesBatchReason      = "Lote fora da conta pela série"
	seriesApprovalID       = "aprovacao-da-serie"
	seriesWarningID        = "aviso-da-serie"
)

var seriesArguments = []string{
	seriesIncomeLine, seriesFixedLine, seriesVariableLine, seriesOtherLine, seriesProvisionLine, seriesReserveLine,
	seriesNewLine, seriesTag, seriesBatchTag, seriesFixedTag, seriesSecondFixedTag, seriesNote, seriesNickname,
	seriesReason, seriesScenario, seriesSubscriptionNote, seriesAcceptNickname, seriesAcceptReason,
	seriesSettingsNickname, seriesCardLabel, seriesExclusionReason, seriesBatchReason, seriesApprovalID, seriesWarningID,
}

// seriesBudget is the first budget: a line in each of the five groups, two in
// the variable one, so every later step has a line to name.
func seriesBudget() []seriesGroup {
	line := func(name string, cents money.Cents) documentLineWire {
		return documentLineWire{Name: name, Amount: Reais(cents)}
	}
	return []seriesGroup{
		{Key: "rec", Name: "Receita", Lines: []documentLineWire{line(seriesIncomeLine, 500_000)}},
		{Key: "fix", Name: "Fixos e essenciais", Lines: []documentLineWire{line(seriesFixedLine, 150_000)}},
		{Key: "var", Name: "Variáveis", Lines: []documentLineWire{line(seriesVariableLine, 80_000), line(seriesOtherLine, 30_000)}},
		{Key: "prov", Name: "Provisões", Lines: []documentLineWire{line(seriesProvisionLine, 20_000)}},
		{Key: "res", Name: "Reservas de desejo", Lines: []documentLineWire{line(seriesReserveLine, 10_000)}},
	}
}

// stepMode is what a step expects of its answer.
type stepMode int

const (
	// stepRecorded: 200 with the id of a change the series later undoes.
	stepRecorded stepMode = iota + 1
	// stepExtension: 200 with the id of the line change it appends to.
	stepExtension
	// stepNull: 200 null, and not a row more in the journal.
	stepNull
	// stepEvaluation: an answer that conforms, and no table of the file changed.
	stepEvaluation
)

// seriesStep is one write of the series. target picks what it acts on and
// builds its arguments; found is false when the ledger offers nothing of the
// shape, and missing is then the fixed label it is counted under.
type seriesStep struct {
	method  string
	mode    stepMode
	missing string
	target  func(run *seriesRun) (arguments string, found bool)
}

// The labels a step without a target is counted under.
const (
	missingHandler     = "the method has no handler yet"
	missingAny         = "no transaction in the current month"
	missingPurchase    = "no purchase in the current month"
	missingAtAPlace    = "no purchase at an establishment in the current month"
	missingPerson      = "no transaction of a person in the current month"
	missingCredit      = "no credit in the current month"
	missingLineChange  = "no line change to extend"
	missingStep        = "no step of the history to take back"
	missingCandidate   = "no received PIX to link as a reimbursement"
	missingLinked      = "no purchase paid back by a linked credit"
	missingExpected    = "no amount expected back"
	missingStale       = "the undo's own rules refused to take the step back"
	missingPurchasable = "no card purchase of the current month to subscribe"
	missingKept        = "no subscription"
	missingDue         = "no subscription due for review"
	missingWarning     = "no warning to dismiss"
	missingQueued      = "nothing waits for review"
	missingTransfer    = "no transfer to mark"
	missingEarlier     = "a single month, so none can be closed"
	missingFree        = "no purchase left to exclude"
	missingPot         = "no pot with a stable identity"
	missingPlace       = "no establishment in the settings"
)

var seriesSteps = []seriesStep{
	{"transactions.setLine", stepRecorded, missingAtAPlace, onTheFirstAtAPlace(seriesVariableLine)},
	{"transactions.applyLineToEstablishment", stepExtension, missingLineChange, func(run *seriesRun) (string, bool) {
		return run.args(run.lineOrigin), run.lineOrigin != ""
	}},
	{"transactions.setLine", stepNull, missingAtAPlace, onTheFirstAtAPlace(seriesVariableLine)},
	{"transactions.setReason", stepRecorded, missingAtAPlace, onTheFirstAtAPlace(seriesReason)},
	{"transactions.confirmLine", stepRecorded, missingAtAPlace, onTheFirstAtAPlace()},
	{"transactions.setLineForMany", stepRecorded, missingAtAPlace, func(run *seriesRun) (string, bool) {
		rest := matching(run.rows(), atAPlace)
		if len(rest) < 2 {
			return "", false
		}
		rest = rest[1:]
		return run.args(idsOf2(middle(rest), last(rest)), seriesOtherLine), true
	}},
	{"transactions.undoHistoryStep", stepRecorded, missingStep, takeBackTheNewestStep},
	{"transactions.addTag", stepRecorded, missingAny, onTheRow(first, everyRow, seriesTag)},
	{"transactions.addTagToMany", stepRecorded, missingAny, func(run *seriesRun) (string, bool) {
		rows := run.rows()
		return run.args(idsOf2(first(rows), last(rows)), seriesBatchTag), len(rows) > 0
	}},
	{"transactions.removeTag", stepRecorded, missingAny, onTheRow(first, everyRow, seriesBatchTag)},
	{"transactions.setNote", stepRecorded, missingAny, onTheRow(middle, everyRow, seriesNote)},
	{"transactions.setNickname", stepRecorded, missingPerson, onTheRow(first, ofAPerson, seriesNickname)},
	{"transactions.setDefaultLine", stepRecorded, missingAtAPlace, onTheFirstAtAPlace(seriesFixedLine)},
	{"transactions.addFixedTag", stepRecorded, missingAtAPlace, onTheFirstAtAPlace(seriesFixedTag)},
	{"transactions.addFixedTag", stepRecorded, missingAtAPlace, onTheFirstAtAPlace(seriesSecondFixedTag)},
	{"transactions.removeFixedTag", stepRecorded, missingAtAPlace, onTheFirstAtAPlace(seriesSecondFixedTag)},
	{"transactions.setSplit", stepRecorded, missingAtAPlace, splitTheLastAtAPlace},
	{"transactions.setReimbursable", stepRecorded, missingPurchase, onTheRow(first, unsplit, true)},
	{"transactions.setExpectedReimbursement", stepRecorded, missingExpected, expectAQuarterBack},
	{"transactions.linkReimbursement", stepRecorded, missingCandidate, linkTheFirstCandidate},
	{"transactions.unlinkReimbursement", stepRecorded, missingLinked, onTheRow(first, paidBack)},
	{"transactions.linkReimbursement", stepRecorded, missingCandidate, linkTheFirstCandidate},
	{"transactions.setRefund", stepRecorded, missingCredit, onTheRow(first, refundable, true)},
	{"transactions.setRefundLine", stepRecorded, missingCredit, onTheRow(first, refundable, seriesVariableLine)},
	{"transactions.setExcluded", stepRecorded, missingAtAPlace, onTheFirstAtAPlace(true)},
	{"subscriptions.create", stepRecorded, missingPurchasable, subscribeTheFirstCandidate},
	{"subscriptions.setStatus", stepRecorded, missingKept, onTheFirstSubscription("Backup e cancelar")},
	{"subscriptions.setLine", stepRecorded, missingKept, onTheFirstSubscription(seriesFixedLine)},
	{"subscriptions.setReviewDate", stepRecorded, missingKept, reviewTheFirstSubscriptionToday},
	{"subscriptions.setNote", stepRecorded, missingKept, onTheFirstSubscription(seriesSubscriptionNote)},
	{"review.keepSubscription", stepRecorded, missingDue, keepTheFirstDue},
	{"review.cancelSubscription", stepRecorded, missingKept, onTheFirstSubscription()},
	{"review.dismissWarning", stepRecorded, missingWarning, dismissTheFirstWarning},
	{"review.acceptPending", stepRecorded, missingQueued, acceptTheFirstInTheQueue},
	{"review.markStepDone", stepRecorded, "", func(run *seriesRun) (string, bool) {
		return run.args(string(run.month), stepPlannedActual), true
	}},
	{"review.markTransferDone", stepRecorded, missingTransfer, markTheFirstTransfer},
	{"review.closeMonth", stepRecorded, missingEarlier, func(run *seriesRun) (string, bool) {
		return run.args(string(run.previous)), run.previous != ""
	}},
	{"budget.saveScenario", stepRecorded, "", func(run *seriesRun) (string, bool) {
		return run.args(seriesScenario, run.document()), true
	}},
	{"budget.evaluateScenario", stepEvaluation, "", func(run *seriesRun) (string, bool) {
		return run.args(string(run.month), withTheSeriesNewLine(run.document())), true
	}},
	{"budget.save", stepRecorded, "", func(run *seriesRun) (string, bool) {
		return run.args(string(run.month), withTheFirstAmountRaised(withTheSeriesNewLine(run.document()), "var")), true
	}},
	{"budget.applyScenario", stepRecorded, "", func(run *seriesRun) (string, bool) {
		return run.args(string(run.month), withTheFirstAmountRaised(run.document(), "rec")), true
	}},
	{"transactions.setExclusionReason", stepRecorded, missingFree, onTheRow(first, free, seriesExclusionReason)},
	{"transactions.excludeMany", stepRecorded, missingFree, func(run *seriesRun) (string, bool) {
		rows := matching(run.rows(), free)
		return run.args(idsOf2(at(rows, 0), at(rows, 1)), seriesBatchReason), len(rows) > 0
	}},
	{"settings.setPotBucket", stepRecorded, missingPot, func(run *seriesRun) (string, bool) {
		pot, found := run.firstStablePot()
		return run.args(pot.ID, anotherBucket(pot.Bucket)), found
	}},
	{"settings.toggleLinkedLine", stepRecorded, missingPot, func(run *seriesRun) (string, bool) {
		pot, found := run.firstStablePot()
		return run.args(pot.ID, seriesProvisionLine), found
	}},
	{"settings.setNickname", stepRecorded, missingPlace, onTheFirstEstablishmentOfTheSettings(seriesSettingsNickname)},
	{"settings.setDefaultLine", stepRecorded, missingPlace, onTheFirstEstablishmentOfTheSettings(seriesReserveLine)},
	{"settings.setCardLabel", stepRecorded, "", constantArguments(seriesCardLabel)},
	{"settings.setThreshold", stepRecorded, "", constantArguments(81)},
	{"settings.setMinExamples", stepRecorded, "", constantArguments(7)},
	{"settings.setClosing", stepRecorded, "", constantArguments("manual", 9)},
	{"subscriptions.acceptSuggestion", stepNull, "", constantArguments()},
	{"subscriptions.rejectSuggestion", stepNull, "", constantArguments()},
	{"review.approve", stepNull, "", constantArguments(seriesApprovalID)},
	{"review.refuse", stepNull, "", constantArguments(seriesApprovalID)},
	{"review.propose", stepNull, "", constantArguments(seriesApprovalID)},
	{"review.runWarningAction", stepNull, "", constantArguments(seriesWarningID, "silence-pair")},
}

// stepsAllowedWithoutHandler names the methods whose step may find no handler.
// Every write of the contract has one, so none is named.
var stepsAllowedWithoutHandler = []string{}

// stepPlannedActual is the closing's second step, one the person ticks.
const stepPlannedActual = "planned-actual"

// seenRow is a row of the current month's transactions.list as the series
// picks from it. cents is read from the answer's decimal text.
type seenRow struct {
	ID                  string          `json:"id"`
	Amount              json.RawMessage `json:"amount"`
	CountsInTotals      bool            `json:"countsInTotals"`
	Line                *string         `json:"line"`
	Movement            bool            `json:"movement"`
	Person              bool            `json:"person"`
	Excluded            bool            `json:"excluded"`
	Split               bool            `json:"split"`
	Reimbursed          bool            `json:"reimbursed"`
	ReimbursementIncome bool            `json:"reimbursementIncome"`
	cents               money.Cents
}

// The shapes a step picks among.
func everyRow(seenRow) bool      { return true }
func purchase(row seenRow) bool  { return row.cents < 0 && !row.Movement && row.CountsInTotals }
func atAPlace(row seenRow) bool  { return purchase(row) && !row.Person }
func ofAPerson(row seenRow) bool { return row.Person && !row.Movement }
func unsplit(row seenRow) bool   { return purchase(row) && !row.Split }
func paidBack(row seenRow) bool  { return purchase(row) && row.Reimbursed }
func refundable(row seenRow) bool {
	return row.cents > 0 && !row.Movement && row.CountsInTotals && !row.ReimbursementIncome
}
func free(row seenRow) bool {
	return unsplit(row) && !row.Reimbursed && row.Line == nil && !row.Excluded
}
func lastUnpaidAtAPlace(row seenRow) bool { return atAPlace(row) && !row.Reimbursed }

func matching(rows []seenRow, keep func(seenRow) bool) []seenRow {
	var kept []seenRow
	for _, row := range rows {
		if keep(row) {
			kept = append(kept, row)
		}
	}
	return kept
}

func first(rows []seenRow) seenRow  { return at(rows, 0) }
func middle(rows []seenRow) seenRow { return at(rows, len(rows)/2) }
func last(rows []seenRow) seenRow   { return at(rows, len(rows)-1) }

func at(rows []seenRow, index int) seenRow {
	if index < 0 || index >= len(rows) {
		return seenRow{}
	}
	return rows[index]
}

// idsOf2 is a selection of two rows, each once; a row that is not there is left
// out.
func idsOf2(a, b seenRow) []string {
	ids := []string{}
	for _, row := range []seenRow{a, b} {
		if row.ID != "" && !slices.Contains(ids, row.ID) {
			ids = append(ids, row.ID)
		}
	}
	return ids
}

// onTheRow acts on the row at a position among those of one shape, with the
// rest of the arguments after its id.
func onTheRow(position func([]seenRow) seenRow, shape func(seenRow) bool, rest ...any) func(*seriesRun) (string, bool) {
	return func(run *seriesRun) (string, bool) {
		row := position(matching(run.rows(), shape))
		return run.args(append([]any{row.ID}, rest...)...), row.ID != ""
	}
}

func onTheFirstAtAPlace(rest ...any) func(*seriesRun) (string, bool) {
	return onTheRow(first, atAPlace, rest...)
}

func constantArguments(values ...any) func(*seriesRun) (string, bool) {
	return func(run *seriesRun) (string, bool) { return run.args(values...), true }
}

// takeBackTheNewestStep takes back the newest step of the first purchase at an
// establishment that its panel says can still be taken back.
func takeBackTheNewestStep(run *seriesRun) (string, bool) {
	row := first(matching(run.rows(), atAPlace))
	if row.ID == "" {
		return "", false
	}
	for _, step := range run.detail(row.ID).History {
		if step.Undoable {
			return run.args(row.ID, step.ID), true
		}
	}
	return "", false
}

// splitTheLastAtAPlace splits the last purchase at an establishment that no
// credit pays back in two halves on two lines, in cents.
func splitTheLastAtAPlace(run *seriesRun) (string, bool) {
	row := last(matching(run.rows(), lastUnpaidAtAPlace))
	magnitude := -row.cents
	half := magnitude / 2
	variable, other := seriesVariableLine, seriesOtherLine
	parts := []splitPart{{Line: &variable, Amount: Reais(half)}, {Line: &other, Amount: Reais(magnitude - half)}}
	return run.args(row.ID, parts), row.ID != ""
}

// expectAQuarterBack sets what is expected back to half of what the panel says
// is expected now.
func expectAQuarterBack(run *seriesRun) (string, bool) {
	row := first(matching(run.rows(), unsplit))
	if row.ID == "" {
		return "", false
	}
	reimbursement := run.detail(row.ID).Reimbursement
	if reimbursement == nil || reimbursement.Expected == nil {
		return "", false
	}
	return run.args(row.ID, *reimbursement.Expected/2), true
}

func linkTheFirstCandidate(run *seriesRun) (string, bool) {
	row := first(matching(run.rows(), unsplit))
	if row.ID == "" {
		return "", false
	}
	candidates := run.detail(row.ID).ReimbursementCandidates
	if len(candidates) == 0 {
		return "", false
	}
	return run.args(row.ID, candidates[0].TransactionID), true
}

// The other lists the series picks from, each read through the router. Each
// answer is read into the few fields a pick needs.

func (run *seriesRun) rows() []seenRow { return run.listOf(run.month) }

// listOf is a month's transactions.list, each amount read into cents from its
// decimal text.
func (run *seriesRun) listOf(month brday.Month) []seenRow {
	var rows []seenRow
	run.decode("transactions.list", run.read("transactions.list", listRaw(string(month), periodMonth)), &rows)
	for i := range rows {
		cents, err := ParseReais(rows[i].Amount)
		withoutValues(run.t, "reading an amount of the list", err)
		rows[i].cents = cents
	}
	return rows
}

// seriesDetail is the part of transactions.detail the series reads.
type seriesDetail struct {
	History []struct {
		ID       string `json:"id"`
		Undoable bool   `json:"undoable"`
	} `json:"history"`
	Reimbursement *struct {
		Expected *Reais `json:"expected"`
	} `json:"reimbursement"`
	ReimbursementCandidates []seriesTransactionRef `json:"reimbursementCandidates"`
}

type seriesTransactionRef struct {
	TransactionID string `json:"transactionId"`
}

type seriesRef struct {
	ID string `json:"id"`
}

func (run *seriesRun) detail(id string) seriesDetail {
	var detail seriesDetail
	run.decode("transactions.detail", run.read("transactions.detail", run.args(id)), &detail)
	return detail
}

func subscribeTheFirstCandidate(run *seriesRun) (string, bool) {
	var candidates []seriesTransactionRef
	run.decode("subscriptions.candidates", run.read("subscriptions.candidates", `[]`), &candidates)
	if len(candidates) == 0 {
		return "", false
	}
	return run.args(candidates[0].TransactionID), true
}

func (run *seriesRun) firstSubscription() (string, bool) {
	var list struct {
		Groups []struct {
			Subscriptions []seriesRef `json:"subscriptions"`
		} `json:"groups"`
	}
	run.decode("subscriptions.list", run.read("subscriptions.list", `[{"grouped":false}]`), &list)
	for _, group := range list.Groups {
		for _, subscription := range group.Subscriptions {
			return subscription.ID, true
		}
	}
	return "", false
}

func onTheFirstSubscription(rest ...any) func(*seriesRun) (string, bool) {
	return func(run *seriesRun) (string, bool) {
		id, found := run.firstSubscription()
		return run.args(append([]any{id}, rest...)...), found
	}
}

func reviewTheFirstSubscriptionToday(run *seriesRun) (string, bool) {
	id, found := run.firstSubscription()
	return run.args(id, run.clock.Format(time.DateOnly)), found
}

func keepTheFirstDue(run *seriesRun) (string, bool) {
	var closing struct {
		SubscriptionsToReview []seriesRef `json:"subscriptionsToReview"`
	}
	run.decode("review.closing", run.read("review.closing", run.args(string(run.month))), &closing)
	if len(closing.SubscriptionsToReview) == 0 {
		return "", false
	}
	return run.args(string(run.month), closing.SubscriptionsToReview[0].ID), true
}

// seriesQueued is an item of the weekly queue as the series reads it.
type seriesQueued struct {
	TransactionID string `json:"transactionId"`
	Day           string `json:"day"`
	Pending       *struct {
		Suggestion *string `json:"suggestion"`
	} `json:"pending"`
}

type seriesWeekly struct {
	Queue struct {
		Items []seriesQueued `json:"items"`
	} `json:"queue"`
	Warnings []seriesRef `json:"warnings"`
}

func (run *seriesRun) weekly() seriesWeekly {
	var weekly seriesWeekly
	run.decode("review.weekly", run.read("review.weekly", run.args(string(run.month))), &weekly)
	return weekly
}

func (run *seriesRun) queue() []seriesQueued { return run.weekly().Queue.Items }

func dismissTheFirstWarning(run *seriesRun) (string, bool) {
	warnings := run.weekly().Warnings
	if len(warnings) == 0 {
		return "", false
	}
	return run.args(warnings[0].ID), true
}

func acceptTheFirstInTheQueue(run *seriesRun) (string, bool) {
	queue := run.queue()
	if len(queue) == 0 {
		return "", false
	}
	answer := pendingAnswer{Line: seriesOtherLine, Always: true, Nickname: seriesAcceptNickname, Reason: seriesAcceptReason}
	return run.args(queue[0].TransactionID, answer), true
}

func markTheFirstTransfer(run *seriesRun) (string, bool) {
	var payday struct {
		Transfers []struct {
			Key   string `json:"key"`
			State string `json:"state"`
		} `json:"transfers"`
	}
	run.decode("review.payday", run.read("review.payday", run.args(string(run.month))), &payday)
	for _, transfer := range payday.Transfers {
		if transfer.State == transferPending {
			return run.args(string(run.month), transfer.Key), true
		}
	}
	return "", false
}

// seriesShell is the part of shell.overview the series reads.
type seriesShell struct {
	PendingCount int               `json:"pendingCount"`
	ClosedMonths []json.RawMessage `json:"closedMonths"`
}

func (run *seriesRun) shell() seriesShell {
	var overview seriesShell
	run.decode("shell.overview", run.read("shell.overview", `[]`), &overview)
	return overview
}

// seriesPot is a pot of savingsPots.list as the series reads it.
type seriesPot struct {
	ID     string  `json:"id"`
	Bucket *string `json:"bucket"`
}

// firstStablePot is the first pot of the list whose identity does not move.
func (run *seriesRun) firstStablePot() (seriesPot, bool) {
	var groups []struct {
		Pots []seriesPot `json:"pots"`
	}
	run.decode("savingsPots.list", run.read("savingsPots.list", `[{"includeClosed":false}]`), &groups)
	for _, group := range groups {
		for _, pot := range group.Pots {
			if !strings.HasPrefix(pot.ID, slotPrefix) {
				return pot, true
			}
		}
	}
	return seriesPot{}, false
}

// anotherBucket is a bucket other than the one the pot counts in, so the write
// stores a choice.
func anotherBucket(current *string) string {
	if current != nil && *current == string(authored.BucketAvailable) {
		return string(authored.BucketInvested)
	}
	return string(authored.BucketAvailable)
}

func onTheFirstEstablishmentOfTheSettings(rest ...any) func(*seriesRun) (string, bool) {
	return func(run *seriesRun) (string, bool) {
		var overview struct {
			Establishments []struct {
				Key string `json:"key"`
			} `json:"establishments"`
		}
		run.decode("settings.overview", run.read("settings.overview", `[]`), &overview)
		if len(overview.Establishments) == 0 {
			return "", false
		}
		return run.args(append([]any{overview.Establishments[0].Key}, rest...)...), true
	}
}

// seriesGroup is a group of a budget document, on the way in and out.
type seriesGroup struct {
	Key   string             `json:"key"`
	Name  string             `json:"name"`
	Lines []documentLineWire `json:"lines"`
}

// document is the budget in force for the current month, every line with its
// id, so a write of it keeps every line.
func (run *seriesRun) document() []seriesGroup {
	var document []seriesGroup
	run.decode("budget.document", run.read("budget.document", run.args(string(run.month))), &document)
	return document
}

func withTheSeriesNewLine(document []seriesGroup) []seriesGroup {
	for i := range document {
		if document[i].Key == "var" {
			document[i].Lines = append(document[i].Lines, documentLineWire{Name: seriesNewLine, Amount: Reais(5_000)})
		}
	}
	return document
}

func withTheFirstAmountRaised(document []seriesGroup, group string) []seriesGroup {
	for i := range document {
		if document[i].Key == group && len(document[i].Lines) > 0 {
			document[i].Lines[0].Amount += Reais(1_000)
		}
	}
	return document
}
