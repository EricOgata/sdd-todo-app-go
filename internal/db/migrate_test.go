package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"todo/internal/apperr"
	"todo/internal/buildinfo"
	"todo/internal/clock"
)

func TestFreshDatabaseAppliesV1(t *testing.T) {
	store := newTestStore(t)

	var version int
	require.NoError(t, store.DB().QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&version))
	assert.Equal(t, 1, version)
	assert.Equal(t, 1, store.SchemaVersion())

	tables := listNames(t, store, "table")
	for _, want := range []string{"categories", "tags", "tasks", "task_tags", "schema_migrations"} {
		assert.Contains(t, tables, want)
	}
	indexes := listNames(t, store, "index")
	for _, want := range []string{
		"idx_tasks_due_date",
		"idx_tasks_completed_at",
		"idx_tasks_deleted_at",
		"idx_tasks_category_id",
		"idx_task_tags_tag_id",
	} {
		assert.Contains(t, indexes, want)
	}
}

func TestMigrationIdempotent(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Migrate(context.Background()))

	var count int
	require.NoError(t, store.DB().QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM schema_migrations").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestSchemaMigrationsRecorded(t *testing.T) {
	store := newTestStore(t)
	var version int
	var appliedAt string
	require.NoError(t, store.DB().QueryRowContext(context.Background(),
		"SELECT version, applied_at FROM schema_migrations").Scan(&version, &appliedAt))
	assert.Equal(t, 1, version)
	parsed, err := time.Parse(time.RFC3339, appliedAt)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, parsed.Location())
}

func TestUserVersionSynced(t *testing.T) {
	store := newTestStore(t)
	var version int
	require.NoError(t, store.DB().QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&version))
	assert.Equal(t, 1, version)
}

func TestNewerSchemaRefused(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "todo.db"))
	ctx := context.Background()
	_, err := store.DB().ExecContext(ctx, createSchemaMigrations)
	require.NoError(t, err)
	_, err = store.DB().ExecContext(ctx, "INSERT INTO schema_migrations (version, applied_at) VALUES (3, '2026-11-12T00:00:00Z')")
	require.NoError(t, err)

	err = store.Migrate(ctx)
	require.Error(t, err)
	assert.Equal(t, 1, apperr.ExitCode(err))
	assert.Equal(t,
		"database schema v3 is newer than supported v1. Upgrade todo before using this database.",
		apperr.Message(err))
}

func TestMigrationFailureRollsBack(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	migrations, err := loadMigrations()
	require.NoError(t, err)
	migrations = append(migrations, migration{
		Version: 2,
		Name:    "0002_bad.sql",
		SQL:     "CREATE TABLE two (id INTEGER);\nTHIS IS NOT VALID SQL;",
	})

	err = store.migrate(ctx, migrations)
	require.Error(t, err)
	assert.Equal(t, 2, apperr.ExitCode(err))
	assert.Contains(t, apperr.Message(err), "migration to v2 failed:")
	assert.Contains(t, apperr.Message(err), "Database left unchanged.")

	tables := listNames(t, store, "table")
	assert.NotContains(t, tables, "two")

	var version int
	require.NoError(t, store.DB().QueryRowContext(ctx, "PRAGMA user_version").Scan(&version))
	assert.Equal(t, 1, version)

	var count int
	require.NoError(t, store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestFreshInitFailureLeavesEmptyDatabase(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "todo.db"))
	ctx := context.Background()

	err := store.migrate(ctx, []migration{{
		Version: 1,
		Name:    "0001_bad.sql",
		SQL:     "CREATE TABLE one (id INTEGER);\nTHIS IS NOT VALID SQL;",
	}})
	require.Error(t, err)
	assert.Equal(t, 2, apperr.ExitCode(err))

	tables, err := store.userTables(ctx)
	require.NoError(t, err)
	assert.Empty(t, tables)
}

func TestNonTodoDatabaseRefused(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "todo.db"))
	ctx := context.Background()
	_, err := store.DB().ExecContext(ctx, "CREATE TABLE foo (id INTEGER)")
	require.NoError(t, err)

	err = store.Migrate(ctx)
	require.Error(t, err)
	assert.Equal(t, 2, apperr.ExitCode(err))
	assert.Contains(t, apperr.Message(err), "is not a todo database. Back up the file before retrying.")
}

func TestMigrationAppliedAtUTC(t *testing.T) {
	loc := time.FixedZone("UTC+02", 2*60*60)
	instant := time.Date(2026, 11, 12, 1, 15, 0, 0, loc)
	path := filepath.Join(t.TempDir(), "todo.db")
	store := openTestStoreWithClock(t, path, clock.NewFake(instant))
	require.NoError(t, store.Migrate(context.Background()))

	var appliedAt string
	require.NoError(t, store.DB().QueryRowContext(context.Background(),
		"SELECT applied_at FROM schema_migrations WHERE version = 1").Scan(&appliedAt))
	assert.Equal(t, "2026-11-11T23:15:00Z", appliedAt)
}

func TestMigrationRetriesOnBusy(t *testing.T) {
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

	var backoffs int32
	store.SetSleeper(func(ctx context.Context, d time.Duration) error {
		atomic.AddInt32(&backoffs, 1)
		return SleepContext(ctx, d)
	})

	released := make(chan struct{})
	go func() {
		time.Sleep(150 * time.Millisecond)
		_, _ = conn.ExecContext(context.Background(), "COMMIT")
		close(released)
	}()

	migrations, err := loadMigrations()
	require.NoError(t, err)
	migrations = append(migrations, migration{
		Version: 2,
		Name:    "0002_later.sql",
		SQL:     "CREATE TABLE later (id INTEGER);",
	})
	err = store.migrate(ctx, migrations)
	<-released
	require.NoError(t, err)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&backoffs), int32(1))

	tables := listNames(t, store, "table")
	assert.Contains(t, tables, "later")
	assert.Equal(t, 2, store.SchemaVersion())
}

func TestSupportedVersionMatchesBuildInfo(t *testing.T) {
	assert.Equal(t, buildinfo.SchemaVersion, SupportedVersion())
}

func openTestStoreWithClock(t *testing.T, path string, clk clock.Clock) *Store {
	t.Helper()
	store, err := Open(context.Background(), OpenOptions{
		Path:        path,
		DisplayPath: path,
		DisplayDir:  filepath.Dir(path),
		Clock:       clk,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func listNames(t *testing.T, store *Store, kind string) []string {
	t.Helper()
	rows, err := store.DB().QueryContext(context.Background(),
		"SELECT name FROM sqlite_master WHERE type = ? AND name NOT LIKE 'sqlite_%' ORDER BY name", kind)
	require.NoError(t, err)
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())
	return names
}
