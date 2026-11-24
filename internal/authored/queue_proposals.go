package authored

import (
	"context"
	"database/sql"
	"encoding/json"
)

// A line an agent proposes for a transaction that waits for review is stored as
// a proposal and, beside it, the transaction and the line it names. The person
// answers it by settling the review, so the review's own writes close it.

// QueueProposal is a waiting proposal of a line for a transaction that waits
// for review.
type QueueProposal struct {
	Proposal ProposalID
	Key      TxKey
	Line     LineID
	Reason   string
}

const (
	statementTransactionWaitsForReview = `SELECT count(*) FROM tx_state WHERE tx_key = ? AND pending_reason IS NOT NULL`

	statementInsertQueueProposal = `INSERT INTO queue_proposals (proposal_id, tx_key, line_id) VALUES (?, ?, ?)`

	statementStaleQueueProposalsOf = `
UPDATE proposals SET outcome = 'stale', answered_at = ?
WHERE outcome = 'waiting' AND id IN (SELECT proposal_id FROM queue_proposals WHERE tx_key = ?)`

	statementWaitingQueueProposalsOf = `
SELECT q.proposal_id, q.line_id
FROM queue_proposals AS q JOIN proposals AS p ON p.id = q.proposal_id
WHERE q.tx_key = ? AND p.outcome = 'waiting'
ORDER BY q.proposal_id`

	statementQueueProposalsOf = `
SELECT q.tx_key, q.line_id, p.id, p.reason
FROM queue_proposals AS q JOIN proposals AS p ON p.id = q.proposal_id
WHERE p.outcome = 'waiting' AND q.tx_key IN (SELECT value FROM json_each(?))
ORDER BY p.id`
)

// InsertQueueProposal stores a waiting proposal of line for the transaction at
// key in one transaction of its own, closing as stale the one that transaction
// already had, so it never has two. It stores nothing and answers false when
// the transaction no longer waits for review: someone settled it after the
// proposal was checked, and a proposal left waiting for a transaction that does
// not wait would never be closed.
func (database *Database) InsertQueueProposal(ctx context.Context, draft ProposalDraft, key TxKey, line LineID) (ProposalID, bool, error) {
	var id ProposalID
	var stored bool
	err := database.write(ctx, func(tx *sql.Tx) error {
		var waits int
		if err := tx.QueryRowContext(ctx, statementTransactionWaitsForReview, int64(key)).Scan(&waits); err != nil || waits == 0 {
			return err
		}
		if _, err := tx.ExecContext(ctx, statementStaleQueueProposalsOf, formatInstant(draft.SubmittedAt), int64(key)); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, statementInsertProposal,
			formatInstant(draft.SubmittedAt), draft.Proposer, draft.Method, draft.Arguments,
			draft.Reason, draft.Title, int64(draft.BasedOn))
		if err != nil {
			return err
		}
		inserted, err := result.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, statementInsertQueueProposal, inserted, int64(key), int64(line)); err != nil {
			return err
		}
		id, stored = ProposalID(inserted), true
		return nil
	})
	return id, stored, err
}

// QueueProposals is the waiting proposal of each of keys that has one; a key
// with none is absent, and so is every key on the empty view.
func (view *View) QueueProposals(ctx context.Context, keys []TxKey) (map[TxKey]QueueProposal, error) {
	found := map[TxKey]QueueProposal{}
	if len(keys) == 0 {
		return found, nil
	}
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		set, err := json.Marshal(keys)
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, statementQueueProposalsOf, string(set))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key, line, id int64
			var reason string
			if err := rows.Scan(&key, &line, &id, &reason); err != nil {
				return err
			}
			found[TxKey(key)] = QueueProposal{Proposal: ProposalID(id), Key: TxKey(key), Line: LineID(line), Reason: reason}
		}
		return rows.Err()
	})
	return found, err
}

type proposedLine struct {
	proposal ProposalID
	line     LineID
}

func (recorder *Recorder) waitingQueueProposals(ctx context.Context, key TxKey) ([]proposedLine, error) {
	rows, err := recorder.tx.QueryContext(ctx, statementWaitingQueueProposalsOf, int64(key))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var waiting []proposedLine
	for rows.Next() {
		var id, line int64
		if err := rows.Scan(&id, &line); err != nil {
			return nil, err
		}
		waiting = append(waiting, proposedLine{proposal: ProposalID(id), line: LineID(line)})
	}
	return waiting, rows.Err()
}

// closeQueueProposals ends the waiting proposals of the transaction at key,
// whose review this change has just ended by putting it on line, in the
// change's own transaction. What the person did decides how: approved by this
// change when the line is the proposed one, whoever settled it; refused, with
// no text, when the person chose another; stale when a rule or the model did,
// since nobody answered the agent. The change's row is written only for an
// approval, so a change that closes nothing leaves no trace.
func (recorder *Recorder) closeQueueProposals(ctx context.Context, key TxKey, line LineID, by Author) error {
	if err := recorder.usable(); err != nil {
		return err
	}
	waiting, err := recorder.waitingQueueProposals(ctx, key)
	if err != nil {
		return recorder.fail(err)
	}
	at := formatInstant(recorder.facts.At)
	for _, proposal := range waiting {
		var err error
		switch {
		case proposal.line == line:
			var change ChangeID
			if change, err = recorder.ID(ctx); err == nil {
				_, err = recorder.tx.ExecContext(ctx, statementApproveProposal, at, int64(change), int64(proposal.proposal))
			}
		case by == AuthorPerson:
			_, err = recorder.tx.ExecContext(ctx, statementRefuseProposal, at, sql.NullString{}, int64(proposal.proposal))
		default:
			_, err = recorder.tx.ExecContext(ctx, statementMarkProposalStale, at, sql.NullString{}, int64(proposal.proposal))
		}
		if err != nil {
			return recorder.fail(err)
		}
	}
	return nil
}

// staleQueueProposals closes as stale, in the change's own transaction, the
// waiting proposal of the transaction at key, whose review the change ended
// without putting it on any line.
func (recorder *Recorder) staleQueueProposals(ctx context.Context, key TxKey) error {
	if err := recorder.usable(); err != nil {
		return err
	}
	if _, err := recorder.tx.ExecContext(ctx, statementStaleQueueProposalsOf, formatInstant(recorder.facts.At), int64(key)); err != nil {
		return recorder.fail(err)
	}
	return nil
}
