package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// staleAfter is how long a connection may go without an update before it is in
// trouble: the provider syncs about once a day, so 48 hours tolerates one missed
// cycle (pluggy-sync README lines 77-81).
const staleAfter = 48 * time.Hour

// SyncState says how fresh the ledger is and how each connection stands.
type SyncState struct {
	// LastCompleteSync is the latest finish among runs that ended complete;
	// nil if none did. A later run may have ended partial: see LatestRun.
	LastCompleteSync *time.Time
	LatestRun        *RunSummary
	Connections      []Connection
}

// RunSummary is the most recent run, whatever its outcome.
type RunSummary struct {
	ID         int64
	Outcome    string
	FinishedAt *time.Time
}

// Connection is one item (a bank link) of the ledger.
type Connection struct {
	ItemID          string
	Connector       string
	Status          string
	ExecutionStatus string
	LastUpdated     *time.Time
	NextAutoSync    *time.Time
	// ConsentExpiresAt comes from items.raw.consentExpiresAt, because the
	// consents table's expires_at is null on every row the collector writes.
	ConsentExpiresAt *time.Time
}

// InTrouble applies the collector's staleness rule: the status is not UPDATED,
// the execution status is not SUCCESS, or the last update is older than 48 hours.
func (connection Connection) InTrouble(now time.Time) bool {
	if connection.Status != "UPDATED" || connection.ExecutionStatus != "SUCCESS" {
		return true
	}
	return connection.LastUpdated == nil || now.Sub(*connection.LastUpdated) > staleAfter
}

// SyncState reads the last synchronisation and the state of every connection.
func (view *View) SyncState(ctx context.Context) (SyncState, error) {
	var state SyncState
	err := view.read(ctx, func(tx *sql.Tx) error {
		var err error
		if state.LastCompleteSync, err = readLastCompleteSync(ctx, tx); err != nil {
			return err
		}
		if state.LatestRun, err = readLatestRun(ctx, tx); err != nil {
			return err
		}
		state.Connections, err = readConnections(ctx, tx)
		return err
	})
	return state, err
}

// The maximum is taken after parsing: the run clock carries an offset, and
// comparing the text would order two instants by their wall clocks.
func readLastCompleteSync(ctx context.Context, tx *sql.Tx) (*time.Time, error) {
	rows, err := tx.QueryContext(ctx, queryCompleteRunFinishes)
	if err != nil {
		return nil, fmt.Errorf("ledger: runs: %w", err)
	}
	defer rows.Close()

	var latest *time.Time
	for rows.Next() {
		var finished string
		if err := rows.Scan(&finished); err != nil {
			return nil, fmt.Errorf("ledger: runs: %w", err)
		}
		instant, err := parseInstant(finished, "runs.finished_at")
		if err != nil {
			return nil, err
		}
		if latest == nil || instant.After(*latest) {
			latest = &instant
		}
	}
	return latest, rows.Err()
}

func readLatestRun(ctx context.Context, tx *sql.Tx) (*RunSummary, error) {
	var (
		run      RunSummary
		finished sql.NullString
	)
	err := tx.QueryRowContext(ctx, queryLatestRun).Scan(&run.ID, &run.Outcome, &finished)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ledger: latest run: %w", err)
	}
	if run.FinishedAt, err = optionalInstant(finished, "runs.finished_at"); err != nil {
		return nil, err
	}
	return &run, nil
}

func readConnections(ctx context.Context, tx *sql.Tx) ([]Connection, error) {
	rows, err := tx.QueryContext(ctx, queryConnections)
	if err != nil {
		return nil, fmt.Errorf("ledger: connections: %w", err)
	}
	defer rows.Close()

	var connections []Connection
	for rows.Next() {
		connection, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		connections = append(connections, connection)
	}
	return connections, rows.Err()
}

func scanConnection(rows *sql.Rows) (Connection, error) {
	var (
		connection                   Connection
		connector, status, execution sql.NullString
		updated, next, consent       sql.NullString
	)
	if err := rows.Scan(&connection.ItemID, &connector, &status, &execution, &updated, &next, &consent); err != nil {
		return Connection{}, fmt.Errorf("ledger: connections: %w", err)
	}
	connection.Connector, connection.Status, connection.ExecutionStatus = connector.String, status.String, execution.String

	var err error
	field := func(name string) string { return name + " of " + connection.ItemID }
	if connection.LastUpdated, err = optionalInstant(updated, field("last_updated")); err != nil {
		return Connection{}, err
	}
	if connection.NextAutoSync, err = optionalInstant(next, field("next_auto_sync")); err != nil {
		return Connection{}, err
	}
	connection.ConsentExpiresAt, err = optionalInstant(consent, field("consentExpiresAt"))
	return connection, err
}
