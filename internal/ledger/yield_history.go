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
// handful of statements that do not depend on the number of windows asked.
func (reader *Reader) YieldHistory(ctx context.Context) (YieldHistory, error) {
	var history YieldHistory
	err := reader.read(ctx, func(tx *sql.Tx) error {
		steps := []func(context.Context, *sql.Tx, *YieldHistory) error{
			readYieldSightings, readYieldTransfers, readYieldRemovals, readYieldWalks, readYieldUnresolved,
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

func readYieldTransfers(ctx context.Context, tx *sql.Tx, history *YieldHistory) error {
	rows, err := tx.QueryContext(ctx, queryYieldTransfers)
	if err != nil {
		return fmt.Errorf("ledger: yield transfers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			transfer                   YieldTransfer
			day                        string
			isReserve, notPosted       sql.NullInt64
			date, instant, name, fault sql.NullString
			cents                      sql.NullInt64
		)
		if err := rows.Scan(&transfer.DocumentID, &transfer.ProviderID, &transfer.ObservationID, &transfer.RunID,
			&transfer.Parent, &day, &isReserve, &date, &instant, &notPosted, &name, &fault, &cents); err != nil {
			return fmt.Errorf("ledger: yield transfers: %w", err)
		}
		transfer.Day, transfer.IsReserve, transfer.NotPosted = brday.Day(day), isReserve.Int64 == 1, notPosted.Int64 == 1
		transfer.Date, transfer.Instant = date.String, instant.String
		transfer.Name, transfer.Fault, transfer.Cents = stringPointer(name), stringPointer(fault), centsPointer(cents)
		history.Transfers = append(history.Transfers, transfer)
	}
	return rows.Err()
}

func readYieldRemovals(ctx context.Context, tx *sql.Tx, history *YieldHistory) error {
	rows, err := tx.QueryContext(ctx, queryYieldRemovals)
	if err != nil {
		return fmt.Errorf("ledger: yield removals: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var removal YieldRemoval
		var day string
		if err := rows.Scan(&removal.DocumentID, &removal.RunID, &day); err != nil {
			return fmt.Errorf("ledger: yield removals: %w", err)
		}
		removal.Day = brday.Day(day)
		history.Removals = append(history.Removals, removal)
	}
	return rows.Err()
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

func readYieldUnresolved(ctx context.Context, tx *sql.Tx, history *YieldHistory) error {
	rows, err := tx.QueryContext(ctx, queryYieldUnresolved)
	if err != nil {
		return fmt.Errorf("ledger: yield unresolved: %w", err)
	}
	defer rows.Close()
	history.Unresolved = map[string]bool{}
	for rows.Next() {
		var id sql.NullString
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("ledger: yield unresolved: %w", err)
		}
		if id.Valid {
			history.Unresolved[id.String] = true
		}
	}
	return rows.Err()
}
