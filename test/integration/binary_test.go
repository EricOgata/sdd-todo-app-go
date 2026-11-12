package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var binaryPath string

func TestMain(m *testing.M) {
	root, err := moduleRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "locate module root:", err)
		os.Exit(1)
	}
	dir, err := os.MkdirTemp("", "todo-integration-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create temp dir:", err)
		os.Exit(1)
	}
	binaryPath = filepath.Join(dir, "todo")

	build := exec.Command(goTool(), "build", "-trimpath",
		"-ldflags", "-X todo/internal/buildinfo.Version=v0.1.0",
		"-o", binaryPath, "./cmd/todo")
	build.Dir = root
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build binary:", err)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func moduleRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot locate test file")
	}
	return filepath.Abs(filepath.Join(filepath.Dir(file), "..", ".."))
}

func goTool() string {
	return filepath.Join(runtime.GOROOT(), "bin", "go")
}

type runResult struct {
	stdout string
	stderr string
	code   int
}

func runTodo(t *testing.T, args ...string) runResult {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	home := t.TempDir()
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"XDG_DATA_HOME="+filepath.Join(home, "xdg"),
		"TODO_DB_PATH="+filepath.Join(home, "todo.db"),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		code = exitErr.ExitCode()
	}
	return runResult{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func TestBinaryVersion(t *testing.T) {
	got := runTodo(t, "--version")
	assert.Equal(t, 0, got.code)
	assert.Equal(t, "todo v0.1.0 (schema 1)\n", got.stdout)
	assert.Empty(t, got.stderr)
}

func TestBinaryVersionJSON(t *testing.T) {
	got := runTodo(t, "--version", "--json")
	assert.Equal(t, 0, got.code)
	assert.Equal(t, "{\"version\":\"v0.1.0\",\"schema_version\":1}\n", got.stdout)
	assert.Empty(t, got.stderr)
}

func TestBinaryHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {}} {
		got := runTodo(t, args...)
		assert.Equal(t, 0, got.code, "args %v", args)
		assert.Contains(t, got.stdout, "Usage:")
		assert.Contains(t, got.stdout, "--json")
		assert.Empty(t, got.stderr)
	}
}

func TestBinaryUnknownCommand(t *testing.T) {
	got := runTodo(t, "bogus")
	assert.Equal(t, 1, got.code)
	assert.Empty(t, got.stdout)
	assert.True(t, strings.HasPrefix(got.stderr, "error: "), "stderr = %q", got.stderr)
	assert.Contains(t, got.stderr, `unknown command "bogus" for "todo"`)
}

func TestBinaryUnknownFlag(t *testing.T) {
	got := runTodo(t, "--bogus")
	assert.Equal(t, 1, got.code)
	assert.Empty(t, got.stdout)
	assert.True(t, strings.HasPrefix(got.stderr, "error: "), "stderr = %q", got.stderr)
	assert.Contains(t, got.stderr, "unknown flag: --bogus")
}

func TestBinaryEmptyDBFlag(t *testing.T) {
	got := runTodo(t, "--db", "")
	assert.Equal(t, 1, got.code)
	assert.Empty(t, got.stdout)
	assert.Equal(t, "error: --db requires a non-empty path.\n", got.stderr)
}

func TestBinaryExitCodes(t *testing.T) {
	assert.Equal(t, 0, runTodo(t, "--version").code)
	assert.Equal(t, 1, runTodo(t, "does-not-exist").code)
}

func TestBinaryStdoutStderrDiscipline(t *testing.T) {
	got := runTodo(t, "--version")
	assert.NotEmpty(t, got.stdout)
	assert.Empty(t, got.stderr)

	failed := runTodo(t, "bogus")
	assert.Empty(t, failed.stdout)
	assert.NotEmpty(t, failed.stderr)
}
