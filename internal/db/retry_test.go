package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"todo/internal/apperr"
)

func insertCategory(ctx context.Context, store *Store, name string) error {
	return store.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO categories (name, created_at, updated_at) VALUES (?, '2026-11-12T09:00:00Z', '2026-11-12T09:00:00Z')",
			name)
		return err
	})
}

func TestRetryOnBusy(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	_, err := store.DB().ExecContext(ctx, "PRAGMA busy_timeout = 0")
	require.NoError(t, err)

	side, err := sql.Open("sqlite", dsn(store.Path()))
	require.NoError(t, err)
	defer side.Close()
	conn, err := side.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(ctx, "PRAGMA busy_timeout = 0")
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE")
	require.NoError(t, err)

	released := make(chan struct{})
	go func() {
		time.Sleep(150 * time.Millisecond)
		_, _ = conn.ExecContext(context.Background(), "COMMIT")
		close(released)
	}()

	err = insertCategory(ctx, store, "Busy")
	<-released
	require.NoError(t, err)

	var count int
	require.NoError(t, store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM categories WHERE name = 'Busy'").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestRetryExhaustion(t *testing.T) {
	store := newTestStore(t)
	backoffs := 0
	store.SetSleeper(func(context.Context, time.Duration) error {
		backoffs++
		return nil
	})

	err := store.WriteTx(context.Background(), func(*sql.Tx) error {
		return errors.New("database is locked")
	})
	require.Error(t, err)
	assert.Equal(t, "database is locked by another process; try again.", apperr.Message(err))
	assert.Equal(t, 2, apperr.ExitCode(err))
	assert.Equal(t, 3, backoffs)
}

func TestNoRetryOnApplicationError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	backoffs := 0
	store.SetSleeper(func(context.Context, time.Duration) error {
		backoffs++
		return nil
	})

	appErr := apperr.User("validation failed")
	err := store.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO categories (name, created_at, updated_at) VALUES ('Rolled', 'now', 'now')"); err != nil {
			return err
		}
		return appErr
	})
	require.Error(t, err)
	assert.Equal(t, appErr, err)
	assert.Zero(t, backoffs)

	var count int
	require.NoError(t, store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM categories").Scan(&count))
	assert.Zero(t, count)
}

func TestRetryCancelledByContext(t *testing.T) {
	store := newTestStore(t)
	store.SetSleeper(func(ctx context.Context, _ time.Duration) error { return ctx.Err() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := store.WriteTx(ctx, func(*sql.Tx) error {
		return errors.New("database is locked")
	})
	require.Error(t, err)
	assert.Equal(t, "interrupted.", apperr.Message(err))
	assert.Equal(t, 2, apperr.ExitCode(err))
}

func TestRetryBackoffIs100ms(t *testing.T) {
	store := newTestStore(t)
	var durations []time.Duration
	store.SetSleeper(func(_ context.Context, d time.Duration) error {
		durations = append(durations, d)
		return nil
	})

	err := store.WriteTx(context.Background(), func(*sql.Tx) error {
		return errors.New("database is locked")
	})
	require.Error(t, err)
	require.Len(t, durations, 3)
	for _, d := range durations {
		assert.Equal(t, 100*time.Millisecond, d)
	}
}

func TestWriteTxCommit(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, insertCategory(context.Background(), store, "Committed"))

	var count int
	require.NoError(t, store.DB().QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM categories WHERE name = 'Committed'").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestOpenRejectsMissingPath(t *testing.T) {
	_, err := Open(context.Background(), OpenOptions{Path: ""})
	require.Error(t, err)
	assert.Equal(t, 2, apperr.ExitCode(err))
}

func TestOpenRejectsUnwritableDirectory(t *testing.T) {
	base := t.TempDir()
	readonly := filepath.Join(base, "readonly")
	require.NoError(t, os.Mkdir(readonly, 0o500))
	t.Cleanup(func() { _ = os.Chmod(readonly, 0o700) })

	path := filepath.Join(readonly, "sub", "todo.db")
	_, err := Open(context.Background(), OpenOptions{
		Path:        path,
		DisplayPath: path,
		DisplayDir:  filepath.Join(readonly, "sub"),
	})
	require.Error(t, err)
	assert.Equal(t, 1, apperr.ExitCode(err))
	assert.Contains(t, apperr.Message(err), "cannot create data directory")
	assert.Contains(t, apperr.Message(err), "Set TODO_DB_PATH to a writable location.")
}
