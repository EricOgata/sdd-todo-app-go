package config

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"todo/internal/apperr"
)

func envFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func homeFrom(dir string) func() (string, error) {
	return func() (string, error) { return dir, nil }
}

func TestResolveFlagWins(t *testing.T) {
	home := t.TempDir()
	paths, err := Resolve(Input{
		DBFlag:    filepath.Join(home, "flag.db"),
		DBFlagSet: true,
		Env: envFrom(map[string]string{
			"TODO_DB_PATH":  filepath.Join(home, "env.db"),
			"XDG_DATA_HOME": filepath.Join(home, "xdg"),
			"HOME":          home,
		}),
		HomeDir: homeFrom(home),
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "flag.db"), paths.DBPath)
}

func TestResolveEnvWinsOverXDG(t *testing.T) {
	home := t.TempDir()
	paths, err := Resolve(Input{
		Env: envFrom(map[string]string{
			"TODO_DB_PATH":  filepath.Join(home, "env.db"),
			"XDG_DATA_HOME": filepath.Join(home, "xdg"),
		}),
		HomeDir: homeFrom(home),
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "env.db"), paths.DBPath)
}

func TestResolveXDG(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, "xdg")
	paths, err := Resolve(Input{
		Env:     envFrom(map[string]string{"XDG_DATA_HOME": xdg}),
		HomeDir: homeFrom(home),
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(xdg, "todo", "todo.db"), paths.DBPath)
}

func TestResolveHomeFallback(t *testing.T) {
	home := t.TempDir()
	paths, err := Resolve(Input{
		Env:     envFrom(map[string]string{}),
		HomeDir: homeFrom(home),
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".local", "share", "todo", "todo.db"), paths.DBPath)
}

func TestResolveRelativeXDGIgnored(t *testing.T) {
	home := t.TempDir()
	paths, err := Resolve(Input{
		Env:     envFrom(map[string]string{"XDG_DATA_HOME": "relative/xdg"}),
		HomeDir: homeFrom(home),
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".local", "share", "todo", "todo.db"), paths.DBPath)
}

func TestResolveTildeExpansion(t *testing.T) {
	home := t.TempDir()
	paths, err := Resolve(Input{
		DBFlag:    "~/tasks/todo.db",
		DBFlagSet: true,
		Env:       envFrom(map[string]string{}),
		HomeDir:   homeFrom(home),
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "tasks", "todo.db"), paths.DBPath)
}

func TestResolveRelativeToAbsolute(t *testing.T) {
	paths, err := Resolve(Input{
		DBFlag:    "relative/todo.db",
		DBFlagSet: true,
		Env:       envFrom(map[string]string{}),
		HomeDir:   homeFrom(t.TempDir()),
	})
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(paths.DBPath))
	assert.True(t, strings.HasSuffix(paths.DBPath, filepath.Join("relative", "todo.db")))
}

func TestResolveExplicitEmptyFlag(t *testing.T) {
	_, err := Resolve(Input{
		DBFlag:    "",
		DBFlagSet: true,
		Env:       envFrom(map[string]string{}),
		HomeDir:   homeFrom(t.TempDir()),
	})
	require.Error(t, err)
	assert.True(t, apperr.IsUser(err))
	assert.Equal(t, "--db requires a non-empty path.", apperr.Message(err))
	assert.Equal(t, 1, apperr.ExitCode(err))
}

func TestResolveEmptyEnvIsUnset(t *testing.T) {
	home := t.TempDir()
	paths, err := Resolve(Input{
		Env:     envFrom(map[string]string{"TODO_DB_PATH": ""}),
		HomeDir: homeFrom(home),
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".local", "share", "todo", "todo.db"), paths.DBPath)
}

func TestResolveMissingHome(t *testing.T) {
	_, err := Resolve(Input{
		Env: envFrom(map[string]string{}),
		HomeDir: func() (string, error) {
			return "", errors.New("no home")
		},
	})
	require.Error(t, err)
	assert.True(t, apperr.IsUser(err))
	assert.Equal(t, 1, apperr.ExitCode(err))
	assert.Equal(t,
		"cannot resolve data directory: neither XDG_DATA_HOME nor HOME is set. Set TODO_DB_PATH or use --db.",
		apperr.Message(err))
}

func TestDisplayPathUsesTilde(t *testing.T) {
	home := t.TempDir()
	got := DisplayPath(filepath.Join(home, ".local", "share", "todo", "todo.db"), home)
	assert.Equal(t, "~/.local/share/todo/todo.db", got)
}

func TestDisplayPathOutsideHome(t *testing.T) {
	home := t.TempDir()
	other := filepath.Join(t.TempDir(), "todo.db")
	assert.Equal(t, other, DisplayPath(other, home))
}
