// Package testutil provides shared fixtures for database and CLI tests.
package testutil

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"todo/internal/clock"
	"todo/internal/db"
)

// Clock is a fixed clock used by date-sensitive fixtures.
var Clock = clock.NewFake(time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC))

// NewStore opens and migrates a fresh database inside t.TempDir().
func NewStore(t *testing.T) *db.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "todo.db")
	store, err := db.Open(context.Background(), db.OpenOptions{
		Path:        path,
		DisplayPath: path,
		DisplayDir:  filepath.Dir(path),
		Clock:       Clock,
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		store.Close()
		t.Fatalf("migrate database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// NewClock returns a fake clock fixed at the given instant.
func NewClock(now time.Time) *clock.Fake { return clock.NewFake(now) }
