package authored

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
)

// applicationID is what finwise stamps in bytes 68 to 71 of its file: "FINW".
const applicationID = 0x46494E57

const (
	headerSize          = 100
	applicationIDOffset = 68
)

var sqliteMagic = []byte("SQLite format 3\x00")

// statementStampApplicationID cannot take a parameter: SQLite reads a pragma's
// value from the statement text.
const statementStampApplicationID = "PRAGMA application_id = 1179209303"

type fileKind int

const (
	fileNew fileKind = iota + 1
	fileOurs
)

// recognize decides from the first hundred bytes alone, without opening the
// file as a database: even a read-only open of a database in write-ahead mode
// can leave a -wal and a -shm beside it, and a write open rewrites its header.
func recognize(path string) (fileKind, error) {
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return 0, &OpenError{Path: path, Failure: FailureMissingDirectory, cause: err}
	}
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fileNew, nil
	}
	if err != nil {
		return 0, &OpenError{Path: path, Failure: FailureOpen, cause: err}
	}
	defer file.Close()

	header := make([]byte, headerSize)
	read, err := io.ReadFull(file, header)
	switch {
	case read == 0 && errors.Is(err, io.EOF):
		return fileNew, nil
	case errors.Is(err, io.ErrUnexpectedEOF):
		return 0, &OpenError{Path: path, Failure: FailureNotOurs}
	case err != nil:
		return 0, &OpenError{Path: path, Failure: FailureOpen, cause: err}
	}
	if !isOurHeader(header) {
		return 0, &OpenError{Path: path, Failure: FailureNotOurs}
	}
	return fileOurs, nil
}

func isOurHeader(header []byte) bool {
	if !bytes.Equal(header[:len(sqliteMagic)], sqliteMagic) {
		return false
	}
	return binary.BigEndian.Uint32(header[applicationIDOffset:applicationIDOffset+4]) == applicationID
}

// stamp writes the application id into a new file in rollback-journal mode, on
// a connection of its own, and closes it before the pool asks for write-ahead
// mode. In write-ahead mode a committed change stays in the -wal file until a
// checkpoint, so an id stamped after the switch could be missing from the main
// file after a crash, and recognize would then refuse the app's own database.
// synchronous(FULL) makes the commit flush the main file before it returns.
func stamp(ctx context.Context, path string) error {
	query := url.Values{}
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "synchronous(FULL)")
	database, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}).String())
	if err != nil {
		return &OpenError{Path: path, Failure: FailureOpen, cause: err}
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	if _, err := database.ExecContext(ctx, statementStampApplicationID); err != nil {
		return &OpenError{Path: path, Failure: FailureOpen, cause: err}
	}
	if err := database.Close(); err != nil {
		return &OpenError{Path: path, Failure: FailureOpen, cause: err}
	}
	// What is on disk now is what the next start will judge, so judge it here.
	if kind, err := recognize(path); err != nil || kind != fileOurs {
		return &OpenError{Path: path, Failure: FailureOpen, cause: err}
	}
	return nil
}
