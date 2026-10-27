package sqlite

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
)

// DB is the connection every storage type holds, with the file it points at
// able to change underneath.
//
// The daily database was opened once at startup and never reopened. A process
// left running past midnight therefore kept writing into the file named after
// the day it started, and retention -- which skips the active file -- never
// touched it. Measured on this station: adsb_targets grows at 64.7 rows/s,
// 1.49 GB in 4 h 15, so a single file reaches roughly 8.4 GB a day and fills a
// 48 GiB disk in about six days. Nothing in the program said so; the only
// symptom was the disk.
//
// The fix has to keep every existing holder of the connection working, because
// four storage types and several services each captured the same *sql.DB
// pointer at startup. This indirection carries the same method set, so those
// call sites are unchanged and only the field type moves.
type DB struct {
	mu   sync.RWMutex
	db   *sql.DB
	path string
	log  *logger.Logger
}

// execer is what a schema needs to be created with: *sql.DB at open, *DB
// anywhere else.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// Open creates the database file if needed, initializes the schema and returns
// a handle to it.
//
// The schema of every table is created here, on the *sql.DB, and not by the
// storage constructors. Rotation is an open too, and a table created only by a
// constructor exists in the file opened at startup and in no file opened after
// midnight. Measured on 2026-09-21: the new day's file received 829 "no such
// table: transcriptions" errors in its first three minutes, and nothing was
// stored from the 1,877 transmissions transcribed before the 10:51 restart.
func Open(path string, log *logger.Logger) (*DB, error) {
	db, err := openOne(path, log)
	if err != nil {
		return nil, err
	}
	return &DB{db: db, path: path, log: log}, nil
}

func openOne(path string, log *logger.Logger) (*sql.DB, error) {
	// Pragmas travel in the connection string so every pooled connection gets
	// them, not just the first.
	connStr := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=cache_size(10000)", path)
	db, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	// Several concurrent readers; writes are serialized by sqliteWriteMu.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)

	if err := initDatabase(db, log); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Path reports the file currently being written to.
func (h *DB) Path() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.path
}

// DailyPath returns the database file name for a given day, in the directory
// the daily files live in.
func DailyPath(dir string, day time.Time) string {
	return filepath.Join(dir, fmt.Sprintf("co-atc-%s.db", day.Format("2006-01-02")))
}

// RotateIfNewDay switches to today's file when the handle is still on an
// earlier day's. It reports whether it rotated.
func (h *DB) RotateIfNewDay(dir string, now time.Time) (bool, error) {
	want := DailyPath(dir, now)
	if h.Path() == want {
		return false, nil
	}
	if err := h.Rotate(want); err != nil {
		return false, err
	}
	return true, nil
}

// Rotate opens path and makes it the file all future queries run against.
//
// The previous connection is closed on its own goroutine rather than here.
// sql.DB.Close waits for queries that have already started -- including rows a
// caller is still iterating -- so closing inline would make rotation as slow as
// the slowest reader holding a *sql.Rows.
func (h *DB) Rotate(path string) error {
	next, err := openOne(path, h.log)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	h.mu.Lock()
	previous, previousPath := h.db, h.path
	h.db, h.path = next, path
	h.mu.Unlock()

	h.log.Info("Rotated to a new daily database",
		logger.String("from", previousPath),
		logger.String("to", path))

	if previous != nil {
		go func() {
			if err := previous.Close(); err != nil {
				h.log.Warn("Failed to close the previous daily database",
					logger.String("path", previousPath), logger.Error(err))
				return
			}
			h.log.Info("Closed the previous daily database",
				logger.String("path", previousPath))
		}()
	}
	return nil
}

// Close closes the current connection.
func (h *DB) Close() error {
	h.mu.Lock()
	db := h.db
	h.db = nil
	h.mu.Unlock()
	if db == nil {
		return nil
	}
	return db.Close()
}

// current returns the connection in force for one call. Callers hold the read
// lock only long enough to read the pointer: a rotation waits for that, not for
// the query itself.
func (h *DB) current() *sql.DB {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.db
}

// The rest is the *sql.DB surface this project actually uses, so that changing
// a field's type from *sql.DB to *DB leaves every call site alone.

// A call after Close gets sql.ErrConnDone rather than a nil-pointer panic: a
// timer still running as the server stops (the voice index, 26/10) made the
// process exit in error once in seventeen stops.
func (h *DB) Query(query string, args ...any) (*sql.Rows, error) {
	db := h.current()
	if db == nil {
		return nil, sql.ErrConnDone
	}
	return db.Query(query, args...)
}

func (h *DB) QueryRow(query string, args ...any) *sql.Row {
	return h.current().QueryRow(query, args...)
}

func (h *DB) Exec(query string, args ...any) (sql.Result, error) {
	db := h.current()
	if db == nil {
		return nil, sql.ErrConnDone
	}
	return db.Exec(query, args...)
}

func (h *DB) Begin() (*sql.Tx, error) {
	db := h.current()
	if db == nil {
		return nil, sql.ErrConnDone
	}
	return db.Begin()
}
