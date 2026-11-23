package authored

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"
)

// ProposalID is the identity of a proposal, written on the wire as a decimal
// string.
type ProposalID int64

func (id ProposalID) String() string { return strconv.FormatInt(int64(id), 10) }

// ParseProposalID reads the one spelling String writes; "07", "+7", "teto" and
// "" are the id of no proposal.
func ParseProposalID(text string) (ProposalID, bool) {
	number, err := strconv.ParseInt(text, 10, 64)
	if err != nil || number < 1 {
		return 0, false
	}
	id := ProposalID(number)
	return id, id.String() == text
}

// ProposalOutcome is where a proposal stands.
type ProposalOutcome string

const (
	ProposalWaiting  ProposalOutcome = "waiting"
	ProposalApproved ProposalOutcome = "approved"
	ProposalRefused  ProposalOutcome = "refused"
	ProposalStale    ProposalOutcome = "stale"
)

// ProposalDraft is a waiting proposal to store.
type ProposalDraft struct {
	SubmittedAt time.Time
	Proposer    string
	Method      string
	Arguments   string
	Reason      string
	Title       string
	BasedOn     ChangeID
}

// Proposal is a stored proposal with its answer and whether the change an
// approval recorded has been undone.
type Proposal struct {
	ID          ProposalID
	SubmittedAt string
	Proposer    string
	Method      string
	Arguments   string
	Reason      string
	Title       string
	BasedOn     ChangeID
	Outcome     ProposalOutcome
	AnsweredAt  *string
	Refusal     *string
	StaleReason *string
	ChangeID    *ChangeID
	Undone      bool
}

const (
	proposalSelectColumns = `p.id, p.submitted_at, p.proposer, p.method, p.arguments, p.reason, p.title, p.based_on,
  p.outcome, p.answered_at, p.refusal, p.stale_reason, p.change_id, coalesce(c.undone_at IS NOT NULL, 0)`

	statementNewestChangeID = `SELECT coalesce(max(id), 0) FROM changes`

	statementInsertProposal = `
INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on)
VALUES (?, ?, ?, ?, ?, ?, ?)`

	statementProposalByID = `
SELECT ` + proposalSelectColumns + `
FROM proposals AS p LEFT JOIN changes AS c ON c.id = p.change_id
WHERE p.id = ?`

	statementWaitingProposals = `
SELECT ` + proposalSelectColumns + `
FROM proposals AS p LEFT JOIN changes AS c ON c.id = p.change_id
WHERE p.outcome = 'waiting'
ORDER BY p.id ASC`

	statementAllProposals = `
SELECT ` + proposalSelectColumns + `
FROM proposals AS p LEFT JOIN changes AS c ON c.id = p.change_id
ORDER BY p.id DESC`

	statementApproveProposal = `
UPDATE proposals SET outcome = 'approved', answered_at = ?, change_id = ?
WHERE id = ? AND outcome = 'waiting'`

	statementRefuseProposal = `
UPDATE proposals SET outcome = 'refused', answered_at = ?, refusal = ?
WHERE id = ? AND outcome = 'waiting'`

	statementMarkProposalStale = `
UPDATE proposals SET outcome = 'stale', answered_at = ?, stale_reason = ?
WHERE id = ? AND outcome = 'waiting'`

	// Effects in force strictly between the proposal's based_on and the open
	// approval change itself, so the approval's own effects never compare
	// against themselves.
	statementEffectsInForceBetween = `
SELECT e.tbl, e.key, e.col
FROM change_effects AS e JOIN changes AS c ON c.id = e.change_id
WHERE e.change_id > ? AND e.change_id < ? AND c.undone_at IS NULL`
)

// DryRun opens a change with facts, runs write over it and rolls the
// transaction back whatever happened, leaving every table and the journal as
// they were. It answers how many effects write recorded and the newest change
// id that already existed before write ran.
func (database *Database) DryRun(ctx context.Context, facts ChangeFacts, write func(*Recorder) error) (effects int64, basedOn ChangeID, err error) {
	if !facts.Author.valid() {
		return 0, 0, ErrInvalidAuthor
	}
	if facts.Method == "" {
		return 0, 0, ErrNoMethod
	}
	tx, err := database.beginWrite(ctx)
	if err != nil {
		return 0, 0, &TransactionError{cause: err}
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, statementDeferForeignKeys); err != nil {
		return 0, 0, &TransactionError{cause: err}
	}
	var newest int64
	if err := tx.QueryRowContext(ctx, statementNewestChangeID).Scan(&newest); err != nil {
		return 0, 0, &TransactionError{cause: err}
	}
	recorder := &Recorder{tx: tx, registry: database.journal, facts: facts}
	runErr := recorder.run(write)
	return recorder.seq, ChangeID(newest), runErr
}

// InsertProposal stores one waiting proposal in a transaction of its own and
// answers its minted id.
func (database *Database) InsertProposal(ctx context.Context, draft ProposalDraft) (ProposalID, error) {
	var id ProposalID
	err := database.write(ctx, func(tx *sql.Tx) error {
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
		id = ProposalID(inserted)
		return nil
	})
	return id, err
}

// RefuseProposal closes a waiting proposal as refused at the given instant with
// an optional reason (empty stores NULL), recording no change. found is false
// when no waiting proposal has that id.
func (database *Database) RefuseProposal(ctx context.Context, id ProposalID, reason string, at time.Time) (Proposal, bool, error) {
	var proposal Proposal
	var refused bool
	err := database.write(ctx, func(tx *sql.Tx) error {
		found, ok, err := readProposalTx(ctx, tx, id)
		if err != nil || !ok || found.Outcome != ProposalWaiting {
			return err
		}
		var text sql.NullString
		if reason != "" {
			text = sql.NullString{String: reason, Valid: true}
		}
		result, err := tx.ExecContext(ctx, statementRefuseProposal, formatInstant(at), text, int64(id))
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil || affected == 0 {
			return err
		}
		proposal, refused, err = readProposalTx(ctx, tx, id)
		return err
	})
	return proposal, refused, err
}

// MarkProposalStale closes a waiting proposal as stale with the refusal reason
// that prevented applying it, in a transaction of its own. It answers whether
// a waiting proposal was closed.
func (database *Database) MarkProposalStale(ctx context.Context, id ProposalID, reason string, at time.Time) (bool, error) {
	var closed bool
	err := database.write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, statementMarkProposalStale, formatInstant(at), reason, int64(id))
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		closed = affected == 1
		return nil
	})
	return closed, err
}

// EffectCount is how many effects the recorder has written so far.
func (recorder *Recorder) EffectCount() int64 { return recorder.seq }

// OverlapsSince says whether any change after basedOn that is still in force
// has an effect on a unit this change has recorded an effect on, by the undo
// engine's own overlap rule.
func (recorder *Recorder) OverlapsSince(ctx context.Context, basedOn ChangeID) (bool, error) {
	if err := recorder.usable(); err != nil {
		return false, err
	}
	if !recorder.written || recorder.seq == 0 {
		return false, nil
	}
	effects, err := loadEffects(ctx, recorder.tx, recorder.id)
	if err != nil {
		return false, recorder.fail(err)
	}
	units := collapse(effects)
	later, err := effectsInForceBetween(ctx, recorder.tx, basedOn, recorder.id)
	if err != nil {
		return false, recorder.fail(err)
	}
	for _, unit := range units {
		table, err := recorder.registry.table(unit.unit.Table)
		if err != nil {
			return false, recorder.fail(err)
		}
		for _, other := range later {
			if overlaps(table, unit.unit, other) {
				return true, nil
			}
		}
	}
	return false, nil
}

func effectsInForceBetween(ctx context.Context, tx *sql.Tx, after, before ChangeID) ([]Unit, error) {
	rows, err := tx.QueryContext(ctx, statementEffectsInForceBetween, int64(after), int64(before))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var units []Unit
	for rows.Next() {
		var unit Unit
		var column sql.NullString
		if err := rows.Scan(&unit.Table, &unit.Key, &column); err != nil {
			return nil, err
		}
		unit.Column = column.String
		units = append(units, unit)
	}
	return units, rows.Err()
}

// ApproveProposal claims a waiting proposal as approved by this recorder's
// change, in the same transaction. It answers false when the proposal no longer
// waits.
func (recorder *Recorder) ApproveProposal(ctx context.Context, id ProposalID, at time.Time) (bool, error) {
	changeID, err := recorder.ID(ctx)
	if err != nil {
		return false, err
	}
	result, err := recorder.tx.ExecContext(ctx, statementApproveProposal, formatInstant(at), int64(changeID), int64(id))
	if err != nil {
		return false, recorder.fail(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, recorder.fail(err)
	}
	return affected == 1, nil
}

// NewestChangeID is the highest change id in the journal, or 0 when there is
// none or on the empty view.
func (view *View) NewestChangeID(ctx context.Context) (ChangeID, error) {
	var newest int64
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, statementNewestChangeID).Scan(&newest)
	})
	return ChangeID(newest), err
}

// Proposal is the stored proposal with that id; found is false when none has it
// or on the empty view.
func (view *View) Proposal(ctx context.Context, id ProposalID) (Proposal, bool, error) {
	var proposal Proposal
	var found bool
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		var err error
		proposal, found, err = readProposalTx(ctx, tx, id)
		return err
	})
	return proposal, found, err
}

// WaitingProposals is every proposal waiting for an answer, oldest first; empty
// on the empty view.
func (view *View) WaitingProposals(ctx context.Context) ([]Proposal, error) {
	return view.readProposals(ctx, func(tx *sql.Tx) (*sql.Rows, error) {
		return tx.QueryContext(ctx, statementWaitingProposals)
	})
}

// Proposals is every proposal, newest first; empty on the empty view.
func (view *View) Proposals(ctx context.Context) ([]Proposal, error) {
	return view.readProposals(ctx, func(tx *sql.Tx) (*sql.Rows, error) {
		return tx.QueryContext(ctx, statementAllProposals)
	})
}

func (view *View) readProposals(ctx context.Context, query func(*sql.Tx) (*sql.Rows, error)) ([]Proposal, error) {
	proposals := []Proposal{}
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := query(tx)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			proposal, err := scanProposal(rows)
			if err != nil {
				return err
			}
			proposals = append(proposals, proposal)
		}
		return rows.Err()
	})
	return proposals, err
}

func readProposalTx(ctx context.Context, tx *sql.Tx, id ProposalID) (Proposal, bool, error) {
	row := tx.QueryRowContext(ctx, statementProposalByID, int64(id))
	proposal, err := scanProposal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, false, nil
	}
	return proposal, err == nil, err
}

type proposalScanner interface {
	Scan(dest ...any) error
}

func scanProposal(scanner proposalScanner) (Proposal, error) {
	var id, basedOn, undone int64
	var proposal Proposal
	var outcome string
	var answeredAt, refusal, staleReason sql.NullString
	var changeID sql.NullInt64
	err := scanner.Scan(&id, &proposal.SubmittedAt, &proposal.Proposer, &proposal.Method,
		&proposal.Arguments, &proposal.Reason, &proposal.Title, &basedOn,
		&outcome, &answeredAt, &refusal, &staleReason, &changeID, &undone)
	if err != nil {
		return Proposal{}, err
	}
	proposal.ID = ProposalID(id)
	proposal.BasedOn = ChangeID(basedOn)
	proposal.Outcome = ProposalOutcome(outcome)
	proposal.Undone = undone != 0
	if answeredAt.Valid {
		value := answeredAt.String
		proposal.AnsweredAt = &value
	}
	if refusal.Valid {
		value := refusal.String
		proposal.Refusal = &value
	}
	if staleReason.Valid {
		value := staleReason.String
		proposal.StaleReason = &value
	}
	if changeID.Valid {
		value := ChangeID(changeID.Int64)
		proposal.ChangeID = &value
	}
	return proposal, nil
}
