package integration

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"todo/internal/cli"
	"todo/internal/clock"
	"todo/internal/db"
)

func perfClock() clock.Clock {
	return clock.NewFake(time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC))
}

func newPerfStore(tb testing.TB) *db.Store {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "todo.db")
	store, err := db.Open(context.Background(), db.OpenOptions{
		Path:        path,
		DisplayPath: path,
		DisplayDir:  filepath.Dir(path),
		Clock:       perfClock(),
	})
	if err != nil {
		tb.Fatalf("open: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		store.Close()
		tb.Fatalf("migrate: %v", err)
	}
	tb.Cleanup(func() { _ = store.Close() })
	return store
}

// seedTasks inserts n tasks, giving every 100th task a due date of 2026-11-12.
func seedTasks(tb testing.TB, store *db.Store, n int) {
	tb.Helper()
	ctx := context.Background()
	err := store.WriteTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx,
			"INSERT INTO tasks (title, notes, due_date, created_at, updated_at) VALUES (?, '', ?, ?, ?)")
		if err != nil {
			return err
		}
		defer stmt.Close()
		created := clock.Timestamp(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC))
		for i := 0; i < n; i++ {
			var due any
			if i%100 == 0 {
				due = "2026-11-12"
			}
			if _, err := stmt.ExecContext(ctx, fmt.Sprintf("Task %06d", i), due, created, created); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		tb.Fatalf("seed: %v", err)
	}
}

func TestSchemaSupports100kTasks(t *testing.T) {
	if testing.Short() {
		t.Skip("100k-task fixture skipped in -short mode")
	}
	store := newPerfStore(t)
	const total = 100000
	seedTasks(t, store, total)

	var count int
	err := store.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM tasks
		 WHERE due_date = '2026-11-12' AND completed_at IS NULL AND deleted_at IS NULL`).Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if want := total / 100; count != want {
		t.Fatalf("today slice = %d, want %d", count, want)
	}
}

func TestStartupBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("startup budget fixture skipped in -short mode")
	}
	store := newPerfStore(t)
	seedTasks(t, store, 100000)
	path := store.Path()

	start := time.Now()
	warm, err := db.Open(context.Background(), db.OpenOptions{
		Path:        path,
		DisplayPath: path,
		DisplayDir:  filepath.Dir(path),
		Clock:       perfClock(),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := warm.Migrate(context.Background()); err != nil {
		warm.Close()
		t.Fatalf("migrate: %v", err)
	}
	elapsed := time.Since(start)
	warm.Close()

	t.Logf("warm startup on 100k tasks: %v (target 150ms)", elapsed)
	if elapsed > 300*time.Millisecond {
		t.Fatalf("warm startup took %v; soft budget is 300ms", elapsed)
	}
}

func BenchmarkStartupNoDatabase(b *testing.B) {
	for i := 0; i < b.N; i++ {
		app := cli.NewApp(cli.Options{
			Out:     &bytes.Buffer{},
			Err:     &bytes.Buffer{},
			Version: "v0.1.0",
			Env:     func(string) string { return "" },
			HomeDir: func() (string, error) { return b.TempDir(), nil },
		})
		app.Root().SetArgs([]string{"--version"})
		if code := app.Execute(context.Background()); code != 0 {
			b.Fatalf("exit code %d", code)
		}
	}
}

func BenchmarkStartupWith100kTasks(b *testing.B) {
	store := newPerfStore(b)
	seedTasks(b, store, 100000)
	path := store.Path()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handle, err := db.Open(context.Background(), db.OpenOptions{
			Path:        path,
			DisplayPath: path,
			DisplayDir:  filepath.Dir(path),
			Clock:       perfClock(),
		})
		if err != nil {
			b.Fatal(err)
		}
		if err := handle.Migrate(context.Background()); err != nil {
			handle.Close()
			b.Fatal(err)
		}
		handle.Close()
	}
}

func BenchmarkTodayQuery100k(b *testing.B) {
	store := newPerfStore(b)
	seedTasks(b, store, 100000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := store.DB().QueryContext(context.Background(),
			`SELECT id, title FROM tasks
			 WHERE due_date = '2026-11-12' AND completed_at IS NULL AND deleted_at IS NULL
			 ORDER BY id`)
		if err != nil {
			b.Fatal(err)
		}
		for rows.Next() {
			var id int64
			var title string
			if err := rows.Scan(&id, &title); err != nil {
				rows.Close()
				b.Fatal(err)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			b.Fatal(err)
		}
		rows.Close()
	}
}
