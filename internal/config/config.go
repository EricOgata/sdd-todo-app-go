// Package config resolves the on-disk location of the task database and
// formats paths for human-facing error messages.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"todo/internal/apperr"
)

// Input carries the sources that participate in path resolution. The
// functions exist so tests can run without touching the process environment.
type Input struct {
	// DBFlag is the value of the global --db flag.
	DBFlag string
	// DBFlagSet reports whether --db was explicitly supplied.
	DBFlagSet bool
	// Env is the environment lookup. Nil means os.Getenv.
	Env func(string) string
	// HomeDir resolves the user's home directory. Nil means os.UserHomeDir.
	HomeDir func() (string, error)
}

// Paths is the resolved database location.
type Paths struct {
	// DBPath is the absolute database file path.
	DBPath string
	// DataDir is the parent directory of DBPath.
	DataDir string
	// DisplayDB is the path rendered in user-facing messages.
	DisplayDB string
	// DisplayDir is the parent directory rendered in user-facing messages.
	DisplayDir string
}

// Resolve applies the documented precedence chain:
//
//	--db > TODO_DB_PATH > $XDG_DATA_HOME/todo/todo.db > $HOME/.local/share/todo/todo.db
func Resolve(in Input) (Paths, error) {
	switch {
	case in.DBFlagSet:
		if strings.TrimSpace(in.DBFlag) == "" {
			return Paths{}, apperr.User("--db requires a non-empty path.")
		}
	case in.env("TODO_DB_PATH") != "":
		in.DBFlag = in.env("TODO_DB_PATH")
	default:
		home, homeErr := in.home()
		if xdg := in.env("XDG_DATA_HOME"); filepath.IsAbs(xdg) {
			in.DBFlag = filepath.Join(xdg, "todo", "todo.db")
		} else if homeErr == nil && home != "" {
			in.DBFlag = filepath.Join(home, ".local", "share", "todo", "todo.db")
		} else {
			return Paths{}, apperr.User("cannot resolve data directory: neither XDG_DATA_HOME nor HOME is set. Set TODO_DB_PATH or use --db.")
		}
	}

	abs, err := normalize(in.DBFlag, in)
	if err != nil {
		return Paths{}, err
	}

	home, _ := in.home()
	dir := filepath.Dir(abs)
	return Paths{
		DBPath:     abs,
		DataDir:    dir,
		DisplayDB:  DisplayPath(abs, home),
		DisplayDir: DisplayPath(dir, home),
	}, nil
}

// DisplayPath renders path with a leading ~ when it lives under home.
func DisplayPath(path, home string) string {
	if home == "" {
		return path
	}
	home = filepath.Clean(home)
	path = filepath.Clean(path)
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}

func normalize(path string, in Input) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := in.home()
		if err != nil || home == "" {
			return "", apperr.User("cannot resolve data directory: neither XDG_DATA_HOME nor HOME is set. Set TODO_DB_PATH or use --db.")
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", apperr.Userf("cannot resolve database path %q: %v.", path, err)
		}
		path = abs
	}
	return filepath.Clean(path), nil
}

func (in Input) env(key string) string {
	if in.Env == nil {
		return os.Getenv(key)
	}
	return in.Env(key)
}

func (in Input) home() (string, error) {
	if in.HomeDir == nil {
		return os.UserHomeDir()
	}
	return in.HomeDir()
}

// ErrPermission is re-exported so callers can compare directory failures
// without importing io/fs directly.
var ErrPermission = os.ErrPermission

// IsPermission reports whether err is a permission failure.
func IsPermission(err error) bool { return errors.Is(err, ErrPermission) }
