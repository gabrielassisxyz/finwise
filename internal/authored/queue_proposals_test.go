package authored_test

import (
	"context"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

var queueProposalInstant = time.Date(2026, time.November, 28, 12, 0, 0, 0, time.UTC)

// waitingForReview indexes a purchase that waits for review with the rule
// suggesting line, and answers its key.
func waitingForReview(t *testing.T, database *authored.Database, providerID string, cents money.Cents, line authored.LineID) authored.TxKey {
	t.Helper()
	key := indexed(t, database, seenRow(providerID, brday.Day("2026-11-28"), cents, "name:"+providerID))
	changeBy(t, database, "scratch.seed", func(recorder *authored.Recorder) error {
		return authoredtest.WriteState(context.Background(), recorder, key, authored.TransactionState{
			PendingReason: "Estabelecimento novo", PendingLine: &line, PendingConfidence: authoredtest.Some[int64](70),
		})
	})
	return key
}

func queueDraft(reason string, at time.Time) authored.ProposalDraft {
	return authored.ProposalDraft{
		SubmittedAt: at, Proposer: "assistente", Method: "transactions.setLine",
		Arguments: `["p-1","Mercado"]`, Reason: reason, Title: "Pôr a compra em Mercado", BasedOn: 0,
	}
}

func proposalOf(t *testing.T, database *authored.Database, id authored.ProposalID) authored.Proposal {
	t.Helper()
	view := database.View()
	defer view.Close()
	proposal, found, err := view.Proposal(t.Context(), id)
	if err != nil || !found {
		t.Fatalf("proposal %s: found=%v err=%v", id, found, err)
	}
	return proposal
}

func queueProposalsOf(t *testing.T, database *authored.Database, keys ...authored.TxKey) map[authored.TxKey]authored.QueueProposal {
	t.Helper()
	view := database.View()
	defer view.Close()
	found, err := view.QueueProposals(t.Context(), keys)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func settleBy(t *testing.T, database *authored.Database, author authored.Author, key authored.TxKey, line authored.LineID) authored.ChangeID {
	t.Helper()
	var id authored.ChangeID
	err := database.Change(t.Context(), authored.ChangeFacts{Author: author, Method: "transactions.setLine", At: changeInstant}, func(recorder *authored.Recorder) error {
		if err := recorder.SettleLine(context.Background(), key, line, author); err != nil {
			return err
		}
		var err error
		id, err = recorder.ID(context.Background())
		return err
	})
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	return id
}

func TestAQueueProposalIsKeptOnlyForATransactionThatWaitsAndNeverReachesTheCard(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	proposed, other := twoLines(t, database)
	key := waitingForReview(t, database, "p-1", -100, other)
	settled := indexed(t, database, seenRow("p-2", "2026-11-28", -200, "name:b"))

	id := authoredtest.InsertQueueProposal(t, database, queueDraft("li o recibo", queueProposalInstant), key, proposed)
	if got := queueProposalsOf(t, database, key, settled); len(got) != 1 || got[key] != (authored.QueueProposal{Proposal: id, Key: key, Line: proposed, Reason: "li o recibo"}) {
		t.Errorf("the proposals kept are %+v, want only the one for the waiting transaction", got)
	}
	if proposal := proposalOf(t, database, id); !proposal.Queue || proposal.Outcome != authored.ProposalWaiting {
		t.Errorf("the proposal reads as %+v, want a waiting one of the queue", proposal)
	}
	view := database.View()
	onTheCard, err := view.WaitingProposals(t.Context())
	view.Close()
	if err != nil || len(onTheCard) != 0 {
		t.Errorf("the approvals card waits on %+v (%v), want nothing", onTheCard, err)
	}

	_, stored, err := database.InsertQueueProposal(t.Context(), queueDraft("sem revisão", queueProposalInstant), settled, proposed)
	if err != nil || stored {
		t.Errorf("a proposal for a transaction that waits for nothing: stored=%v err=%v", stored, err)
	}
	if count := columnValues(t, database.Pool(), `SELECT count(*) FROM proposals`); !equalStrings(count, []string{"1"}) {
		t.Errorf("proposals after the refused one = %v, want 1", count)
	}
}

func TestASecondQueueProposalClosesTheFirstAsStaleWithNoReason(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	proposed, other := twoLines(t, database)
	key := waitingForReview(t, database, "p-1", -100, other)
	sibling := waitingForReview(t, database, "p-2", -200, other)

	first := authoredtest.InsertQueueProposal(t, database, queueDraft("primeira", queueProposalInstant), key, proposed)
	untouched := authoredtest.InsertQueueProposal(t, database, queueDraft("de outra", queueProposalInstant), sibling, proposed)
	later := queueProposalInstant.Add(time.Hour)
	second := authoredtest.InsertQueueProposal(t, database, queueDraft("segunda", later), key, other)

	closed := proposalOf(t, database, first)
	if closed.Outcome != authored.ProposalStale || closed.StaleReason != nil || closed.ChangeID != nil || closed.Refusal != nil || closed.AnsweredAt == nil {
		t.Errorf("the first proposal reads as %+v, want stale with no reason", closed)
	}
	if got := queueProposalsOf(t, database, key, sibling); got[key].Proposal != second || got[key].Line != other || got[sibling].Proposal != untouched {
		t.Errorf("the proposals waiting are %+v, want the second for the transaction and the other's untouched", got)
	}
}

func TestSettlingAReviewClosesTheProposalByWhoAnsweredAndWhichLine(t *testing.T) {
	cases := []struct {
		name    string
		author  authored.Author
		onLine  func(proposed, other authored.LineID) authored.LineID
		outcome authored.ProposalOutcome
	}{
		{"the person on the proposed line", authored.AuthorPerson, func(proposed, _ authored.LineID) authored.LineID { return proposed }, authored.ProposalApproved},
		{"the person on another line", authored.AuthorPerson, func(_, other authored.LineID) authored.LineID { return other }, authored.ProposalRefused},
		{"a rule on the proposed line", authored.AuthorRule, func(proposed, _ authored.LineID) authored.LineID { return proposed }, authored.ProposalApproved},
		{"a rule on another line", authored.AuthorRule, func(_, other authored.LineID) authored.LineID { return other }, authored.ProposalStale},
		{"the model on another line", authored.AuthorModel, func(_, other authored.LineID) authored.LineID { return other }, authored.ProposalStale},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			database := openDatabase(t, databasePath(t), creationInstant, nil)
			proposed, other := twoLines(t, database)
			key := waitingForReview(t, database, "p-1", -100, other)
			id := authoredtest.InsertQueueProposal(t, database, queueDraft("li o recibo", queueProposalInstant), key, proposed)

			change := settleBy(t, database, c.author, key, c.onLine(proposed, other))

			closed := proposalOf(t, database, id)
			if closed.Outcome != c.outcome || closed.AnsweredAt == nil || closed.Refusal != nil || closed.StaleReason != nil {
				t.Fatalf("the proposal reads as %+v, want %s with no text", closed, c.outcome)
			}
			if c.outcome == authored.ProposalApproved && (closed.ChangeID == nil || *closed.ChangeID != change) {
				t.Errorf("approved by change %v, want the one that settled the review, %s", closed.ChangeID, change)
			}
			if c.outcome != authored.ProposalApproved && closed.ChangeID != nil {
				t.Errorf("a proposal %s carries the change %v", c.outcome, closed.ChangeID)
			}
			if left := queueProposalsOf(t, database, key); len(left) != 0 {
				t.Errorf("the queue still shows %+v", left)
			}
		})
	}
}

func TestLinkingACreditAsAReimbursementClosesItsProposalAsStale(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	proposed, other := twoLines(t, database)
	credit := waitingForReview(t, database, "p-1", 5_000, other)
	purchase := indexed(t, database, seenRow("p-2", "2026-11-27", -9_000, "name:b"))
	id := authoredtest.InsertQueueProposal(t, database, queueDraft("é o reembolso", queueProposalInstant), credit, proposed)

	changeBy(t, database, "transactions.linkReimbursement", func(recorder *authored.Recorder) error {
		_, err := recorder.LinkReimbursement(context.Background(), purchase, credit, 5_000)
		return err
	})

	if closed := proposalOf(t, database, id); closed.Outcome != authored.ProposalStale || closed.StaleReason != nil || closed.ChangeID != nil {
		t.Errorf("the credit's proposal reads as %+v, want stale with no reason", closed)
	}
}

func TestTheProposalIsClosedInTheTransactionOfTheChangeThatSettlesTheReview(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	proposed, other := twoLines(t, database)
	key := waitingForReview(t, database, "p-1", -100, other)
	id := authoredtest.InsertQueueProposal(t, database, queueDraft("li o recibo", queueProposalInstant), key, proposed)

	_, _, err := database.DryRun(t.Context(), authored.ChangeFacts{Author: authored.AuthorPerson, Method: "transactions.setLine", At: changeInstant}, func(recorder *authored.Recorder) error {
		return recorder.SettleLine(context.Background(), key, proposed, authored.AuthorPerson)
	})
	if err != nil {
		t.Fatal(err)
	}
	if proposal := proposalOf(t, database, id); proposal.Outcome != authored.ProposalWaiting {
		t.Errorf("a settle that was rolled back closed the proposal: %+v", proposal)
	}
}

func TestUndoingTheChangeThatSettledTheReviewBringsItBackAndLeavesTheProposalClosed(t *testing.T) {
	for _, line := range []string{"proposed", "other"} {
		t.Run(line, func(t *testing.T) {
			database := openDatabase(t, databasePath(t), creationInstant, nil)
			proposed, other := twoLines(t, database)
			key := waitingForReview(t, database, "p-1", -100, other)
			id := authoredtest.InsertQueueProposal(t, database, queueDraft("li o recibo", queueProposalInstant), key, proposed)
			chosen, want := proposed, authored.ProposalApproved
			if line == "other" {
				chosen, want = other, authored.ProposalRefused
			}
			change := settleBy(t, database, authored.AuthorPerson, key, chosen)

			undo(t, database, change)

			if state := keptAbout(t, database, key).State; state.PendingReason == "" || state.PendingLine == nil || *state.PendingLine != other {
				t.Errorf("the review after the undo is %+v, want it back with the rule's line", state)
			}
			if proposal := proposalOf(t, database, id); proposal.Outcome != want || proposal.Undone != (want == authored.ProposalApproved) {
				t.Errorf("the proposal after the undo reads as %+v, want %s still closed", proposal, want)
			}
			if left := queueProposalsOf(t, database, key); len(left) != 0 {
				t.Errorf("the undo brought the proposal back: %+v", left)
			}
		})
	}
}

func TestTheApprovalsCardCannotAnswerAQueueProposal(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	proposed, other := twoLines(t, database)
	key := waitingForReview(t, database, "p-1", -100, other)
	id := authoredtest.InsertQueueProposal(t, database, queueDraft("li o recibo", queueProposalInstant), key, proposed)

	if _, refused, err := database.RefuseProposal(t.Context(), id, "não", queueProposalInstant); err != nil || refused {
		t.Errorf("refusing a queue proposal: refused=%v err=%v, want it left alone", refused, err)
	}
	if proposal := proposalOf(t, database, id); proposal.Outcome != authored.ProposalWaiting {
		t.Errorf("the proposal reads as %+v after the refusal, want it waiting", proposal)
	}
}
