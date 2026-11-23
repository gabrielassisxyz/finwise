package ledger

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// YieldHistory is everything the collector's yield statement reads, for every
// account and every window at once. It is plain rows: which of them a window
// uses, and what they add up to, is the rules' to say.
type YieldHistory struct {
	Sightings  []YieldSighting
	Transfers  []YieldTransfer
	Removals   []YieldRemoval
	Walks      []YieldWalk
	Unresolved map[string]bool
}

// YieldSighting is an unambiguous sighting of an account that reports its
// reserves as a list, which is the only kind a window can start or end on.
// UpdatedAt is empty when the provider gave no time the statement can compare.
type YieldSighting struct {
	AccountID string
	Day       brday.Day
	RunID     int64
	UpdatedAt string
	Entries   []YieldEntry
}

// YieldEntry is a reserve entry with the checks the statement makes on it
// already made: BRL says the currency is, AmountOK says the amount is a finite
// number of at most two decimals, and Cents is nil only when there is no amount.
type YieldEntry struct {
	ReserveID *string
	Name      *string
	Stable    bool
	BRL       bool
	AmountOK  bool
	Cents     *money.Cents
}

// YieldTransfer is one unambiguous version of a document that was a reserve
// transfer. Fault is the reason the version itself gives for not being counted,
// nil when none; Name is the reserve the description names.
type YieldTransfer struct {
	DocumentID    int64
	ProviderID    string
	ObservationID int64
	RunID         int64
	Parent        string
	Day           brday.Day
	IsReserve     bool
	Date          string
	Instant       string
	NotPosted     bool
	Name          *string
	Fault         *string
	Cents         *money.Cents
}

// YieldRemoval is an event that makes a transfer gone from its day on.
type YieldRemoval struct {
	DocumentID int64
	RunID      int64
	Day        brday.Day
}

// YieldWalk is a complete walk of an account's transactions that reached back to
// Since, in the run it belongs to.
type YieldWalk struct {
	AccountID string
	RunID     int64
	Since     string
}

// YieldHistory reads the yield statement's inputs for the whole ledger, in a
// handful of statements that do not depend on the number of windows asked. One
// of them reads the ledger's transactions, for the transfers, their removals
// and the pending pairs at once.
func (view *View) YieldHistory(ctx context.Context) (YieldHistory, error) {
	var history YieldHistory
	err := view.read(ctx, func(tx *sql.Tx) error {
		steps := []func(context.Context, *sql.Tx, *YieldHistory) error{
			readYieldSightings, view.readYieldTransferHistory, readYieldWalks,
		}
		for _, step := range steps {
			if err := step(ctx, tx, &history); err != nil {
				return err
			}
		}
		return nil
	})
	return history, err
}

type sightingKey struct {
	accountID string
	day       string
}

func readYieldSightings(ctx context.Context, tx *sql.Tx, history *YieldHistory) error {
	rows, err := tx.QueryContext(ctx, queryYieldSightings)
	if err != nil {
		return fmt.Errorf("ledger: yield sightings: %w", err)
	}
	defer rows.Close()
	index := map[sightingKey]int{}
	for rows.Next() {
		var (
			sighting YieldSighting
			day      string
			instant  sql.NullString
		)
		if err := rows.Scan(&sighting.AccountID, &day, &sighting.RunID, &instant); err != nil {
			return fmt.Errorf("ledger: yield sightings: %w", err)
		}
		sighting.Day, sighting.UpdatedAt = brday.Day(day), instant.String
		index[sightingKey{sighting.AccountID, day}] = len(history.Sightings)
		history.Sightings = append(history.Sightings, sighting)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("ledger: yield sightings: %w", err)
	}
	return readYieldEntries(ctx, tx, history, index)
}

func readYieldEntries(ctx context.Context, tx *sql.Tx, history *YieldHistory, index map[sightingKey]int) error {
	rows, err := tx.QueryContext(ctx, queryYieldEntries)
	if err != nil {
		return fmt.Errorf("ledger: yield entries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			accountID, day, reserveID, name sql.NullString
			stable, brl, amountOK, cents    sql.NullInt64
		)
		if err := rows.Scan(&accountID, &day, &reserveID, &name, &stable, &brl, &amountOK, &cents); err != nil {
			return fmt.Errorf("ledger: yield entries: %w", err)
		}
		position, sighted := index[sightingKey{accountID.String, day.String}]
		if !sighted {
			continue
		}
		history.Sightings[position].Entries = append(history.Sightings[position].Entries, YieldEntry{
			ReserveID: stringPointer(reserveID), Name: stringPointer(name), Stable: stable.Int64 == 1,
			BRL: brl.Int64 == 1, AmountOK: amountOK.Int64 == 1, Cents: centsPointer(cents),
		})
	}
	return rows.Err()
}

// readYieldTransferHistory is the one reading of the ledger's transactions the
// history makes.
func (view *View) readYieldTransferHistory(ctx context.Context, tx *sql.Tx, history *YieldHistory) error {
	if view.observeYieldReading != nil {
		view.observeYieldReading()
	}
	rows, err := tx.QueryContext(ctx, queryYieldTransferHistory)
	if err != nil {
		return fmt.Errorf("ledger: yield transfer history: %w", err)
	}
	defer rows.Close()
	history.Unresolved = map[string]bool{}
	for rows.Next() {
		var row transferHistoryRow
		if err := row.scan(rows); err != nil {
			return fmt.Errorf("ledger: yield transfer history: %w", err)
		}
		if err := row.addTo(history); err != nil {
			return err
		}
	}
	return rows.Err()
}

// transferHistoryRow is a row of any of the statement's three kinds; a column
// a kind does not use is null.
type transferHistoryRow struct {
	kind                             string
	documentID, observationID, runID sql.NullInt64
	providerID, parent, day          sql.NullString
	isReserve, notPosted, cents      sql.NullInt64
	date, instant, name, fault       sql.NullString
}

func (row *transferHistoryRow) scan(rows *sql.Rows) error {
	return rows.Scan(&row.kind, &row.documentID, &row.providerID, &row.observationID, &row.runID, &row.parent,
		&row.day, &row.isReserve, &row.date, &row.instant, &row.notPosted, &row.name, &row.fault, &row.cents)
}

func (row transferHistoryRow) addTo(history *YieldHistory) error {
	switch row.kind {
	case "version":
		history.Transfers = append(history.Transfers, YieldTransfer{
			DocumentID: row.documentID.Int64, ProviderID: row.providerID.String, ObservationID: row.observationID.Int64,
			RunID: row.runID.Int64, Parent: row.parent.String, Day: brday.Day(row.day.String),
			IsReserve: row.isReserve.Int64 == 1, Date: row.date.String, Instant: row.instant.String,
			NotPosted: row.notPosted.Int64 == 1, Name: stringPointer(row.name), Fault: stringPointer(row.fault),
			Cents: centsPointer(row.cents),
		})
	case "removal":
		history.Removals = append(history.Removals, YieldRemoval{
			DocumentID: row.documentID.Int64, RunID: row.runID.Int64, Day: brday.Day(row.day.String),
		})
	case "pair":
		if row.providerID.Valid {
			history.Unresolved[row.providerID.String] = true
		}
	default:
		return fmt.Errorf("ledger: yield transfer history: a row of unknown kind %q", row.kind)
	}
	return nil
}

func readYieldWalks(ctx context.Context, tx *sql.Tx, history *YieldHistory) error {
	rows, err := tx.QueryContext(ctx, queryYieldWalks)
	if err != nil {
		return fmt.Errorf("ledger: yield walks: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var walk YieldWalk
		var since sql.NullString
		if err := rows.Scan(&walk.AccountID, &walk.RunID, &since); err != nil {
			return fmt.Errorf("ledger: yield walks: %w", err)
		}
		// A minimum instant SQLite cannot read as a time compares as no walk at all.
		if since.Valid {
			walk.Since = since.String
			history.Walks = append(history.Walks, walk)
		}
	}
	return rows.Err()
}
