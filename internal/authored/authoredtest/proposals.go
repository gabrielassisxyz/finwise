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
