package authored

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// The instant of the last backup, in UTC as RFC 3339. Each backup replaces it.
const statementRecordLastBackup = `
INSERT INTO meta (key, value) VALUES ('last_backup_at', ?)
ON CONFLICT (key) DO UPDATE SET value = excluded.value`

const statementLastBackup = `SELECT value FROM meta WHERE key = 'last_backup_at'`

const statementApplicationID = `PRAGMA application_id`

const statementIntegrityCheck = `PRAGMA integrity_check`

// BackupOptions is what Backup needs besides the two paths.
type BackupOptions struct {
	// Now is the clock the instant of the backup is recorded from. It is required.
	Now   func() time.Time
	hooks backupHooks
}

// backupHooks let a test act between two steps of a backup, which it cannot
// reach from outside: a copy that goes bad, a destination that appears, a
// record that fails after the publication.
type backupHooks struct {
	afterCopy     func(temporary string)
	beforePublish func()
	afterPublish  func()
}

// Backup writes a checked copy of the database at source to destination and
// records its instant in source. It never creates source, never migrates it and
// never replaces a file: a destination that exists, or appears meanwhile, is
// refused and left as it was. Every failure removes the temporary copy, and a
// failure to record the instant removes the copy just published, so a backup
// and its record exist together or not at all.
func Backup(ctx context.Context, source, destination string, options BackupOptions) error {
	if options.Now == nil {
		return ErrNoClock
	}
	// Both files are opened through file: URIs, where a relative path would be
	// read as a host name and refused.
	absoluteSource, err := filepath.Abs(source)
	if err != nil {
		return &OpenError{Path: source, Failure: FailureOpen, cause: err}
	}
	absoluteDestination, err := filepath.Abs(destination)
	if err != nil {
		return &BackupError{Source: source, Destination: destination, Failure: BackupCopyFailed, cause: err}
	}
	source, destination = absoluteSource, absoluteDestination
	database, err := openBackupSource(ctx, source)
	if err != nil {
		return err
	}
	defer database.Close()
	temporary, err := database.checkedCopy(ctx, destination, options.hooks)
	if err != nil {
		return err
	}
	runHook(options.hooks.beforePublish)
	published, err := publish(source, temporary, destination)
	if err != nil {
		return err
	}
	runHook(options.hooks.afterPublish)
	if err := database.recordBackup(ctx, options.Now()); err != nil {
		return withdraw(source, destination, published, err)
	}
	return nil
}

func runHook(hook func()) {
	if hook != nil {
		hook()
	}
}

// openBackupSource differs from Open: a missing or empty file is an error,
// never a new database, so a mistyped path leaves nothing behind; and nothing
// is migrated.
func openBackupSource(ctx context.Context, source string) (*Database, error) {
	kind, err := recognize(source)
	if err != nil {
		return nil, err
	}
	if kind != fileOurs {
		return nil, &BackupError{Source: source, Failure: BackupSourceMissing}
	}
	return openPool(ctx, source, nil)
}

// checkedCopy reserves an empty, uniquely named file beside the destination,
// which VACUUM INTO accepts, and returns it once the copy in it passed its
// check. The same directory is what lets publish link it.
func (database *Database) checkedCopy(ctx context.Context, destination string, hooks backupHooks) (string, error) {
	failure := func(kind BackupFailure, cause error) *BackupError {
		return &BackupError{Source: database.path, Destination: destination, Failure: kind, cause: cause}
	}
	reserved, err := os.CreateTemp(filepath.Dir(destination), "."+filepath.Base(destination)+".partial-*")
	if err != nil {
		return "", failure(BackupCopyFailed, err)
	}
	temporary := reserved.Name()
	if err := reserved.Close(); err != nil {
		removeTemporary(temporary)
		return "", failure(BackupCopyFailed, err)
	}
	if _, err := database.pool.ExecContext(ctx, statementCopyInto, temporary); err != nil {
		removeTemporary(temporary)
		return "", failure(BackupCopyFailed, err)
	}
	if hooks.afterCopy != nil {
		hooks.afterCopy(temporary)
	}
	if err := checkCopy(ctx, temporary); err != nil {
		removeTemporary(temporary)
		return "", failure(BackupCheckFailed, err)
	}
	if err := syncFile(temporary); err != nil {
		removeTemporary(temporary)
		return "", failure(BackupCopyFailed, err)
	}
	return temporary, nil
}

var errCopyNotOurs = errors.New("authored: the copy does not carry finwise's application id")

// checkCopy opens the copy immutable: nobody else knows the file, and an
// immutable open creates no -wal, -shm or -journal beside it.
func checkCopy(ctx context.Context, path string) error {
	query := url.Values{}
	query.Set("mode", "ro")
	query.Set("immutable", "1")
	copied, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}).String())
	if err != nil {
		return err
	}
	defer copied.Close()
	copied.SetMaxOpenConns(1)
	var id int64
	if err := copied.QueryRowContext(ctx, statementApplicationID).Scan(&id); err != nil {
		return err
	}
	if id != applicationID {
		return errCopyNotOurs
	}
	return integrityIsOK(ctx, copied)
}

var errCopyDamaged = errors.New("authored: the copy did not pass integrity_check")

func integrityIsOK(ctx context.Context, copied *sql.DB) error {
	rows, err := copied.QueryContext(ctx, statementIntegrityCheck)
	if err != nil {
		return err
	}
	defer rows.Close()
	var answers []string
	for rows.Next() {
		var answer string
		if err := rows.Scan(&answer); err != nil {
			return err
		}
		answers = append(answers, answer)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(answers) != 1 || answers[0] != "ok" {
		return errCopyDamaged
	}
	return nil
}

// publish links the copy to the destination name: unlike a rename, a link
// fails when the name is taken, so whatever is there stays. It returns what
// was published, so that only that file can ever be withdrawn.
func publish(source, temporary, destination string) (fs.FileInfo, error) {
	failure := func(kind BackupFailure, cause error) *BackupError {
		return &BackupError{Source: source, Destination: destination, Failure: kind, cause: cause}
	}
	published, err := os.Lstat(temporary)
	if err != nil {
		removeTemporary(temporary)
		return nil, failure(BackupPublishFailed, err)
	}
	if err := os.Link(temporary, destination); err != nil {
		removeTemporary(temporary)
		if errors.Is(err, fs.ErrExist) {
			return nil, failure(BackupDestinationTaken, err)
		}
		return nil, failure(BackupPublishFailed, err)
	}
	if err := os.Remove(temporary); err != nil {
		return nil, failure(BackupTemporaryLeft, err)
	}
	if err := syncFile(filepath.Dir(destination)); err != nil {
		return nil, failure(BackupTemporaryLeft, err)
	}
	return published, nil
}

// removeTemporary removes the reserved file and anything SQLite may have left
// under its name; all of it is this backup's own.
func removeTemporary(temporary string) {
	for _, suffix := range []string{"", "-journal", "-wal", "-shm"} {
		_ = os.Remove(temporary + suffix)
	}
}

func (database *Database) recordBackup(ctx context.Context, now time.Time) error {
	return database.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, statementRecordLastBackup, now.UTC().Format(time.RFC3339))
		return err
	})
}

// withdraw removes the copy published by this backup, and nothing that took
// its name meanwhile.
func withdraw(source, destination string, published fs.FileInfo, recordErr error) error {
	current, err := os.Lstat(destination)
	if err != nil || !os.SameFile(current, published) {
		return &BackupError{Source: source, Destination: destination, Failure: BackupNotWithdrawn, cause: recordErr}
	}
	if err := os.Remove(destination); err != nil {
		return &BackupError{Source: source, Destination: destination, Failure: BackupNotWithdrawn, cause: recordErr}
	}
	_ = syncFile(filepath.Dir(destination))
	return &BackupError{Source: source, Destination: destination, Failure: BackupNotRecorded, cause: recordErr}
}

// LastBackup is the instant of the last backup taken of the database. found is
// false on the empty view and while no backup has been recorded.
func (view *View) LastBackup(ctx context.Context) (instant time.Time, found bool, err error) {
	var stored sql.NullString
	found, err = view.read(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, statementLastBackup).Scan(&stored)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil || !found || !stored.Valid {
		return time.Time{}, false, err
	}
	instant, err = time.Parse(time.RFC3339, stored.String)
	if err != nil {
		return time.Time{}, false, &TransactionError{cause: err}
	}
	return instant, true, nil
}
