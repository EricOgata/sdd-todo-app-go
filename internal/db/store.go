// Package db owns the SQLite storage layer: opening, permissions, connection
// pragmas, write-contention retries, corruption handling, and schema
// migrations.
package db

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"todo/internal/apperr"
	"todo/internal/clock"
)

// OpenOptions configures a Store.
type OpenOptions struct {
	// Path is the absolute database file path.
	Path string
	// DisplayPath is used in user-facing error messages.
	DisplayPath string
	// DisplayDir is the parent directory used in user-facing messages.
	DisplayDir string
	// Clock supplies timestamps for migrations and tests.
	Clock clock.Clock
	// Sleeper overrides the retry backoff. Tests inject a fake.
	Sleeper func(context.Context, time.Duration) error
}

// Store is the process-local handle to the database.
type Store struct {
	db            *sql.DB
	path          string
	displayPath   string
	clk           clock.Clock
	sleep         func(context.Context, time.Duration) error
	schemaVersion int
}

// Open creates the data directory and database file when missing, opens a
// single SQLite connection, and applies the connection pragmas.
func Open(ctx context.Context, opts OpenOptions) (*Store, error) {
	if opts.Path == "" {
		return nil, apperr.System("internal error: database path is empty")
	}
	if opts.DisplayPath == "" {
		opts.DisplayPath = opts.Path
	}
	if opts.DisplayDir == "" {
		opts.DisplayDir = filepath.Dir(opts.Path)
	}
	if opts.Clock == nil {
		opts.Clock = clock.System{}
	}
	if opts.Sleeper == nil {
		opts.Sleeper = SleepContext
	}

	if err := ensureDir(filepath.Dir(opts.Path), opts.DisplayDir); err != nil {
		return nil, err
	}
	if err := ensureFile(opts.Path, opts.DisplayPath); err != nil {
		return nil, err
	}

	sqlDB, err := sql.Open("sqlite", dsn(opts.Path))
	if err != nil {
		return nil, classifyOpenError(err, opts.DisplayPath)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	store := &Store{
		db:          sqlDB,
		path:        opts.Path,
		displayPath: opts.DisplayPath,
		clk:         opts.Clock,
		sleep:       opts.Sleeper,
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, store.classify(err)
	}
	if err := store.applyPragmas(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, store.classify(err)
	}
	chmodSidecars(opts.Path)
	return store, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB exposes the underlying handle for commands that need read queries.
func (s *Store) DB() *sql.DB { return s.db }

// Path returns the database file path.
func (s *Store) Path() string { return s.path }

// DisplayPath returns the user-facing database path.
func (s *Store) DisplayPath() string { return s.displayPath }

// Clock returns the clock bound to the store.
func (s *Store) Clock() clock.Clock { return s.clk }

// SetSleeper replaces the retry backoff function. It exists for tests.
func (s *Store) SetSleeper(fn func(context.Context, time.Duration) error) {
	if fn != nil {
		s.sleep = fn
	}
}

// SleepContext sleeps for d unless ctx is cancelled first.
func SleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func dsn(path string) string {
	u := url.URL{Scheme: "file", Path: path}
	q := url.Values{}
	q.Set("_txlock", "immediate")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(NORMAL)")
	u.RawQuery = q.Encode()
	return u.String()
}

func ensureDir(dir, display string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		detail := errorDetail(err)
		if errors.Is(err, fs.ErrPermission) {
			detail = "permission denied"
		}
		return apperr.Userf("cannot create data directory %s: %s. Set TODO_DB_PATH to a writable location.", display, detail)
	}
	return nil
}

func ensureFile(path, display string) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return apperr.Userf("cannot create database file at %s: %s.", display, errorDetail(err))
	}
	return f.Close()
}

func errorDetail(err error) string {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err.Error()
	}
	return err.Error()
}

func chmodSidecars(path string) {
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Chmod(path+suffix, 0o600)
	}
}

func classifyOpenError(err error, displayPath string) error {
	if IsCorruption(err) {
		return apperr.Systemf("database file at %s failed the integrity check. Back up the file before retrying.", displayPath)
	}
	return apperr.Unexpected(err)
}

// classify converts corruption-class failures into the documented error and
// leaves everything else untouched.
func (s *Store) classify(err error) error {
	if err == nil || !IsCorruption(err) {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result string
	checkErr := s.db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result)
	if checkErr == nil && strings.EqualFold(result, "ok") {
		return err
	}
	return apperr.Systemf("database file at %s failed the integrity check. Back up the file before retrying.", s.displayPath)
}

// Check runs corruption confirmation when err looks like corruption.
func (s *Store) Check(err error) error { return s.classify(err) }

// SchemaVersion reports the schema version observed during the last
// successful migration pass.
func (s *Store) SchemaVersion() int { return s.schemaVersion }

// ReadTx runs a read-only function against the database, classifying
// corruption-class failures.
func (s *Store) ReadTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return s.classify(err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return s.classify(err)
	}
	if err := tx.Commit(); err != nil {
		return s.classify(err)
	}
	return nil
}

// WriteTx runs fn inside a SQLite write transaction. Busy and locked errors
// are retried up to three times with a 100 ms backoff; application errors are
// returned immediately after a rollback. The transaction uses BEGIN IMMEDIATE
// so contention is surfaced up front.
func (s *Store) WriteTx(ctx context.Context, fn func(*sql.Tx) error) error {
	const maxAttempts = 4
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return apperr.System("interrupted.")
		}
		if attempt > 0 {
			if err := s.sleep(ctx, 100*time.Millisecond); err != nil {
				return apperr.System("interrupted.")
			}
		}

		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			if interrupted(err) {
				return apperr.System("interrupted.")
			}
			if IsBusy(err) {
				continue
			}
			return s.classify(err)
		}

		if err := fn(tx); err != nil {
			_ = tx.Rollback()
			if interrupted(err) {
				return apperr.System("interrupted.")
			}
			if IsBusy(err) {
				continue
			}
			return s.classify(err)
		}

		if err := tx.Commit(); err != nil {
			if interrupted(err) {
				return apperr.System("interrupted.")
			}
			if IsBusy(err) {
				continue
			}
			return s.classify(err)
		}
		chmodSidecars(s.path)
		return nil
	}
	return errDatabaseLocked
}

var errDatabaseLocked = apperr.System("database is locked by another process; try again.")

func interrupted(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func fmtReason(err error) string {
	msg := err.Error()
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return msg
}
