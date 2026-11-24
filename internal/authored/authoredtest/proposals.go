package authoredtest

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// InsertProposal stores a waiting proposal in the scratch database and returns
// its id.
func InsertProposal(t testing.TB, database *authored.Database, draft authored.ProposalDraft) authored.ProposalID {
	t.Helper()
	id, err := database.InsertProposal(t.Context(), draft)
	if err != nil {
		t.Fatalf("authoredtest: insert proposal: %v", err)
	}
	return id
}

// RefuseProposal closes a waiting proposal as refused in the scratch database.
func RefuseProposal(t testing.TB, database *authored.Database, id authored.ProposalID, reason string, at time.Time) {
	t.Helper()
	_, refused, err := database.RefuseProposal(t.Context(), id, reason, at)
	if err != nil || !refused {
		t.Fatalf("authoredtest: refuse proposal %s: refused=%v err=%v", id, refused, err)
	}
}

// MarkProposalStale closes a waiting proposal as stale in the scratch database.
func MarkProposalStale(t testing.TB, database *authored.Database, id authored.ProposalID, reason string, at time.Time) {
	t.Helper()
	closed, err := database.MarkProposalStale(t.Context(), id, reason, at)
	if err != nil || !closed {
		t.Fatalf("authoredtest: mark proposal %s stale: closed=%v err=%v", id, closed, err)
	}
}

// Proposals reads every stored proposal, newest first.
func Proposals(t testing.TB, database *authored.Database) []authored.Proposal {
	t.Helper()
	view := database.View()
	defer view.Close()
	proposals, err := view.Proposals(t.Context())
	if err != nil {
		t.Fatalf("authoredtest: read proposals: %v", err)
	}
	return proposals
}

// InsertQueueProposal stores a waiting proposal of line for the transaction at
// key in the scratch database and answers its id; the test fails when the
// transaction does not wait for review.
func InsertQueueProposal(t testing.TB, database *authored.Database, draft authored.ProposalDraft, key authored.TxKey, line authored.LineID) authored.ProposalID {
	t.Helper()
	id, stored, err := database.InsertQueueProposal(t.Context(), draft, key, line)
	if err != nil || !stored {
		t.Fatalf("authoredtest: insert queue proposal: stored=%v err=%v", stored, err)
	}
	return id
}
