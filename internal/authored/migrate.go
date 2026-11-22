package authored

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var embeddedFiles embed.FS

// journalMigration creates meta, so the tracking start month is written in the
// transaction that applies it, and in no other.
const journalMigration = "0001_journal"

// copyTimestampLayout is the UTC instant, to the second, in the name of the copy
// taken before migrating: finwise.db.before-20261201T013000Z.
const copyTimestampLayout = "20060102T150405Z"

const statementMigrationsTableExists = `
SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'schema_migrations'`

const statementRecordedMigrations = `SELECT name, sha256 FROM schema_migrations ORDER BY name`

// The runner's own table, created by the runner and by no migration file.
const statementCreateMigrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
  name   TEXT PRIMARY KEY NOT NULL CHECK (name <> ''),
  sha256 TEXT NOT NULL CHECK (length(sha256) = 64)
) STRICT`

const statementRecordMigration = `INSERT INTO schema_migrations (name, sha256) VALUES (?, ?)`

const statementCopyInto = `VACUUM INTO ?`

type migrationFile struct {
	name   string
	body   string
	sha256 string
}

// embeddedMigrations is the migration set the binary carries.
func embeddedMigrations() fs.FS {
	set, err := fs.Sub(embeddedFiles, "migrations")
	if err != nil {
		panic(fmt.Sprintf("authored: embedded migrations: %v", err))
	}
	return set
}

// loadMigrations reads every *.sql file of the set; a migration's name is its
// file name without the extension, and the set is applied in name order.
func loadMigrations(set fs.FS) ([]migrationFile, error) {
	if set == nil {
		set = embeddedMigrations()
	}
	paths, err := fs.Glob(set, "*.sql")
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errors.New("authored: the migration set is empty")
	}
	files := make([]migrationFile, 0, len(paths))
	for _, path := range paths {
		content, err := fs.ReadFile(set, path)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(content)
		files = append(files, migrationFile{
			name: strings.TrimSuffix(path, ".sql"), body: string(content), sha256: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	return files, nil
}

// migrate applies every file of the set that is not recorded, whatever its
// position: work lands in parallel, so a lower name can arrive after a higher
// one. Nothing is applied, and no copy taken, unless every recorded migration
// is one the set has, with the same checksum.
func (database *Database) migrate(ctx context.Context, set []migrationFile) error {
	recorded, err := database.recordedMigrations(ctx)
	if err != nil {
		return &OpenError{Path: database.path, Failure: FailureOpen, cause: err}
	}
	pending, err := database.pendingMigrations(recorded, set)
	if err != nil || len(pending) == 0 {
		return err
	}
	// A database that records no migration holds nothing the person authored:
	// it was created by this start, found missing or empty, or stopped right
	// after the stamp or a failed first migration. Any other needs the copy.
	if len(recorded) > 0 {
		if err := database.copyBeforeMigrating(ctx); err != nil {
			return err
		}
	}
	for _, migration := range pending {
		if err := database.apply(ctx, migration); err != nil {
			return err
		}
	}
	return nil
}

func (database *Database) recordedMigrations(ctx context.Context) (map[string]string, error) {
	recorded := map[string]string{}
	err := database.read(ctx, func(tx *sql.Tx) error {
		var tables int
		if err := tx.QueryRowContext(ctx, statementMigrationsTableExists).Scan(&tables); err != nil || tables == 0 {
			return err
		}
		rows, err := tx.QueryContext(ctx, statementRecordedMigrations)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name, sum string
			if err := rows.Scan(&name, &sum); err != nil {
				return err
			}
			recorded[name] = sum
		}
		return rows.Err()
	})
	return recorded, err
}

func (database *Database) pendingMigrations(recorded map[string]string, set []migrationFile) ([]migrationFile, error) {
	known := make(map[string]migrationFile, len(set))
	for _, migration := range set {
		known[migration.name] = migration
	}
	for name, sum := range recorded {
		migration, ok := known[name]
		if !ok {
			return nil, &OpenError{Path: database.path, Failure: FailureUnknownMigration}
		}
		if migration.sha256 != sum {
			return nil, &OpenError{Path: database.path, Failure: FailureChangedMigration, Migration: name}
		}
	}
	var pending []migrationFile
	for _, migration := range set {
		if _, done := recorded[migration.name]; !done {
			pending = append(pending, migration)
		}
	}
	return pending, nil
}

// apply runs one file and records it in the same transaction, so a file is
// either applied and recorded or neither.
func (database *Database) apply(ctx context.Context, migration migrationFile) error {
	err := database.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, statementCreateMigrationsTable); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migration.body); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, statementRecordMigration, migration.name, migration.sha256); err != nil {
			return err
		}
		if migration.name == journalMigration {
			return recordTrackingStart(ctx, tx, database.now())
		}
		return nil
	})
	if err != nil {
		return &OpenError{Path: database.path, Failure: FailureMigration, Migration: migration.name, cause: err}
	}
	return nil
}

// copyBeforeMigrating writes a consistent copy of the database beside it. When
// the name is taken the start is refused instead of another name being chosen:
// the copy is never overwritten (VACUUM INTO would overwrite an empty file), and
// a start one second later finds a free name.
func (database *Database) copyBeforeMigrating(ctx context.Context) error {
	target := database.path + ".before-" + database.now().UTC().Format(copyTimestampLayout)
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		return &OpenError{Path: database.path, Failure: FailureCopy, cause: err}
	}
	if _, err := database.pool.ExecContext(ctx, statementCopyInto, target); err != nil {
		return &OpenError{Path: database.path, Failure: FailureCopy, cause: err}
	}
	if err := syncFile(target); err != nil {
		return &OpenError{Path: database.path, Failure: FailureCopy, cause: err}
	}
	if err := syncFile(filepath.Dir(target)); err != nil {
		return &OpenError{Path: database.path, Failure: FailureCopy, cause: err}
	}
	return nil
}

func syncFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}
