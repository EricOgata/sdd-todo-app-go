package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"todo/internal/clock"
)

func testClock() clock.Clock {
	return clock.NewFake(time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC))
}

func openTestStore(t *testing.T, path string) *Store {
	t.Helper()
	store, err := Open(context.Background(), OpenOptions{
		Path:        path,
		DisplayPath: path,
		DisplayDir:  filepath.Dir(path),
		Clock:       testClock(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "todo.db")
	store := openTestStore(t, path)
	require.NoError(t, store.Migrate(context.Background()))
	return store
}

func TestOpenCreatesFile0600(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	path := filepath.Join(dir, "todo.db")
	store := openTestStore(t, path)
	_ = store

	fileInfo, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())

	dirInfo, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
}

func TestExistingFilePermissionsUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.db")
	require.NoError(t, os.WriteFile(path, nil, 0o644))
	require.NoError(t, os.Chmod(path, 0o644))

	store := openTestStore(t, path)
	_ = store

	fileInfo, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), fileInfo.Mode().Perm())
}

func TestOpenAppliesPragmas(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	var journal string
	require.NoError(t, store.DB().QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal))
	assert.Equal(t, "wal", strings.ToLower(journal))

	var foreignKeys int
	require.NoError(t, store.DB().QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys))
	assert.Equal(t, 1, foreignKeys)

	var busyTimeout int
	require.NoError(t, store.DB().QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout))
	assert.Equal(t, 5000, busyTimeout)

	var synchronous int
	require.NoError(t, store.DB().QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous))
	assert.Equal(t, 1, synchronous) // NORMAL
}

func TestForeignKeysEnforced(t *testing.T) {
	store := newTestStore(t)
	_, err := store.DB().ExecContext(context.Background(),
		"INSERT INTO tasks (title, created_at, updated_at, category_id) VALUES ('x', 'now', 'now', 999)")
	require.Error(t, err)
}

func TestSQLiteVersionAtLeast335(t *testing.T) {
	store := newTestStore(t)
	var version string
	require.NoError(t, store.DB().QueryRowContext(context.Background(), "SELECT sqlite_version()").Scan(&version))
	parts := strings.SplitN(version, ".", 2)
	require.Len(t, parts, 2)
	major, err := strconv.Atoi(parts[0])
	require.NoError(t, err)
	minorPart, _, _ := strings.Cut(parts[1], ".")
	minor, err := strconv.Atoi(minorPart)
	require.NoError(t, err)
	if major < 3 || (major == 3 && minor < 35) {
		t.Fatalf("sqlite version %s is older than 3.35", version)
	}
}

func TestSingleConnectionPool(t *testing.T) {
	store := newTestStore(t)
	assert.Equal(t, 1, store.DB().Stats().MaxOpenConnections)
}

func TestWALSidecarPermissionsBestEffort(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			"INSERT INTO categories (name, created_at, updated_at) VALUES ('Work', 'now', 'now')")
		return err
	}))
	chmodSidecars(store.Path())
	for _, suffix := range []string{"-wal", "-shm"} {
		info, err := os.Stat(store.Path() + suffix)
		if err != nil {
			continue // sidecars are best effort
		}
		assert.Zero(t, info.Mode().Perm()&0o077, "%s must not be group/world accessible", suffix)
	}
}
