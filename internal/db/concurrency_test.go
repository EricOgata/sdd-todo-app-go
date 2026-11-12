package db

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcurrentWritesNoLostUpdates(t *testing.T) {
	store := newTestStore(t)
	path := store.Path()
	require.NoError(t, store.Close())

	const writers = 20
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			handle, err := Open(context.Background(), OpenOptions{
				Path:        path,
				DisplayPath: path,
				DisplayDir:  filepath.Dir(path),
				Clock:       testClock(),
			})
			if err != nil {
				errs <- err
				return
			}
			defer handle.Close()
			errs <- insertCategory(context.Background(), handle, fmt.Sprintf("Cat%02d", i))
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	verify, err := Open(context.Background(), OpenOptions{
		Path:        path,
		DisplayPath: path,
		DisplayDir:  filepath.Dir(path),
		Clock:       testClock(),
	})
	require.NoError(t, err)
	defer verify.Close()

	var count int
	require.NoError(t, verify.DB().QueryRowContext(context.Background(),
		"SELECT COUNT(DISTINCT name) FROM categories").Scan(&count))
	assert.Equal(t, writers, count)

	var integrity string
	require.NoError(t, verify.DB().QueryRowContext(context.Background(),
		"PRAGMA integrity_check").Scan(&integrity))
	assert.Equal(t, "ok", integrity)
}

func TestTwentyConcurrentWriteProcesses(t *testing.T) {
	if os.Getenv("TODO_HELPER_WRITE") == "1" {
		return // the parent runs the helper in a subprocess
	}
	if testing.Short() {
		t.Skip("subprocess concurrency test skipped in -short mode")
	}

	store := newTestStore(t)
	path := store.Path()

	exe, err := os.Executable()
	require.NoError(t, err)

	const processes = 20
	var wg sync.WaitGroup
	errs := make(chan error, processes)
	for i := 0; i < processes; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := exec.Command(exe, "-test.run=^TestHelperWriteProcess$")
			cmd.Env = append(os.Environ(),
				"TODO_HELPER_WRITE=1",
				"TODO_HELPER_DB="+path,
				fmt.Sprintf("TODO_HELPER_NAME=Proc%02d", i),
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				errs <- fmt.Errorf("process %d: %w: %s", i, err, output)
				return
			}
			errs <- nil
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var count int
	require.NoError(t, store.DB().QueryRowContext(context.Background(),
		"SELECT COUNT(DISTINCT name) FROM categories").Scan(&count))
	assert.Equal(t, processes, count)

	var integrity string
	require.NoError(t, store.DB().QueryRowContext(context.Background(),
		"PRAGMA integrity_check").Scan(&integrity))
	assert.Equal(t, "ok", integrity)
}

// TestHelperWriteProcess is not a real test. It is the subprocess entry point
// used by TestTwentyConcurrentWriteProcesses.
func TestHelperWriteProcess(t *testing.T) {
	if os.Getenv("TODO_HELPER_WRITE") != "1" {
		t.Skip("helper process only")
	}
	path := os.Getenv("TODO_HELPER_DB")
	name := os.Getenv("TODO_HELPER_NAME")
	store, err := Open(context.Background(), OpenOptions{
		Path:        path,
		DisplayPath: path,
		DisplayDir:  filepath.Dir(path),
		Clock:       testClock(),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := insertCategory(context.Background(), store, name); err != nil {
		fmt.Fprintln(os.Stderr, err)
		store.Close()
		os.Exit(2)
	}
	store.Close()
	os.Exit(0)
}
