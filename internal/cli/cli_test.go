package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"todo/internal/apperr"
)

type testApp struct {
	app    *App
	out    *bytes.Buffer
	errOut *bytes.Buffer
	env    map[string]string
}

func newTestApp(t *testing.T, version string) *testApp {
	t.Helper()
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	env := map[string]string{}
	home := t.TempDir()
	app := NewApp(Options{
		Out:        out,
		Err:        errOut,
		Version:    version,
		Env:        func(key string) string { return env[key] },
		HomeDir:    func() (string, error) { return home, nil },
		IsTerminal: func(io.Writer) bool { return false },
	})
	return &testApp{app: app, out: out, errOut: errOut, env: env}
}

func (a *testApp) probe() {
	a.app.Root().AddCommand(&cobra.Command{
		Use:     "probe",
		Short:   "storage probe",
		GroupID: GroupTask,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := RuntimeFrom(cmd)
			if err != nil {
				return err
			}
			if rt.Printer.JSONMode() {
				return rt.Printer.WriteJSON(map[string]any{
					"db":     rt.Paths.DBPath,
					"schema": rt.Store.SchemaVersion(),
				})
			}
			rt.Printer.Line("ok %s", rt.Paths.DBPath)
			return nil
		},
	})
}

func (a *testApp) failing() {
	a.app.Root().AddCommand(&cobra.Command{
		Use:     "fail",
		GroupID: GroupTask,
		RunE: func(*cobra.Command, []string) error {
			return apperr.User("boom")
		},
	})
}

func TestFirstCommandCreatesDataDirAndDB(t *testing.T) {
	ctx := context.Background()
	ta := newTestApp(t, "v0.1.0")
	ta.probe()
	dir := filepath.Join(t.TempDir(), "nested", "data")
	path := filepath.Join(dir, "todo.db")
	ta.app.Root().SetArgs([]string{"probe", "--db", path})

	code := ta.app.Execute(ctx)
	require.Equal(t, 0, code, "stderr: %s", ta.errOut.String())
	assert.Equal(t, "ok "+path+"\n", ta.out.String())
	assert.Empty(t, ta.errOut.String())

	dirInfo, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())

	fileInfo, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())

	require.NotNil(t, ta.app.state.runtime)
	assert.Equal(t, 1, ta.app.state.runtime.Store.SchemaVersion())
}

func TestVersionText(t *testing.T) {
	ta := newTestApp(t, "v0.1.0")
	ta.app.Root().SetArgs([]string{"--version"})

	code := ta.app.Execute(context.Background())
	assert.Equal(t, 0, code)
	assert.Equal(t, "todo v0.1.0 (schema 1)\n", ta.out.String())
	assert.Empty(t, ta.errOut.String())
}

func TestVersionJSON(t *testing.T) {
	ta := newTestApp(t, "v0.1.0")
	ta.app.Root().SetArgs([]string{"--version", "--json"})

	code := ta.app.Execute(context.Background())
	assert.Equal(t, 0, code)
	assert.Equal(t, "{\"version\":\"v0.1.0\",\"schema_version\":1}\n", ta.out.String())
	assert.Empty(t, ta.errOut.String())
}

func TestVersionDoesNotOpenDatabase(t *testing.T) {
	ta := newTestApp(t, "v0.1.0")
	missing := filepath.Join(t.TempDir(), "missing", "todo.db")
	ta.app.Root().SetArgs([]string{"--version", "--db", missing})

	code := ta.app.Execute(context.Background())
	assert.Equal(t, 0, code)
	assert.Equal(t, "todo v0.1.0 (schema 1)\n", ta.out.String())
	_, err := os.Stat(missing)
	assert.True(t, os.IsNotExist(err))
}

func TestRootNoArgsPrintsHelp(t *testing.T) {
	ta := newTestApp(t, "v0.1.0")
	dbPath := filepath.Join(t.TempDir(), "todo.db")
	ta.env["TODO_DB_PATH"] = dbPath
	ta.app.Root().SetArgs([]string{})

	code := ta.app.Execute(context.Background())
	assert.Equal(t, 0, code)
	assert.Contains(t, ta.out.String(), "Usage:")
	assert.Contains(t, ta.out.String(), "todo")
	assert.Contains(t, ta.out.String(), "--json")
	assert.Contains(t, ta.out.String(), "--db")
	assert.Contains(t, ta.out.String(), "--no-color")
	assert.Contains(t, ta.out.String(), "--version")
	_, err := os.Stat(dbPath)
	assert.True(t, os.IsNotExist(err), "bare todo must not touch the database")
}

func TestHelpGroupsRegistered(t *testing.T) {
	ta := newTestApp(t, "v0.1.0")
	groups := ta.app.Root().Groups()
	require.Len(t, groups, 3)
	assert.Equal(t, GroupTask, groups[0].ID)
	assert.Equal(t, "Task commands", groups[0].Title)
	assert.Equal(t, GroupOrganization, groups[1].ID)
	assert.Equal(t, "Organization commands", groups[1].Title)
	assert.Equal(t, GroupMaintenance, groups[2].ID)
	assert.Equal(t, "Maintenance commands", groups[2].Title)
}

func TestUnknownCommandExit1(t *testing.T) {
	ta := newTestApp(t, "v0.1.0")
	ta.app.Root().SetArgs([]string{"bogus"})

	code := ta.app.Execute(context.Background())
	assert.Equal(t, 1, code)
	assert.Empty(t, ta.out.String())
	assert.Equal(t, "error: unknown command \"bogus\" for \"todo\"\n", ta.errOut.String())
}

func TestUnknownFlagExit1(t *testing.T) {
	ta := newTestApp(t, "v0.1.0")
	ta.app.Root().SetArgs([]string{"--bogus"})

	code := ta.app.Execute(context.Background())
	assert.Equal(t, 1, code)
	assert.Empty(t, ta.out.String())
	assert.Equal(t, "error: unknown flag: --bogus\n", ta.errOut.String())
}

func TestUnwritableDataDirExit1(t *testing.T) {
	base := t.TempDir()
	readonly := filepath.Join(base, "readonly")
	require.NoError(t, os.Mkdir(readonly, 0o500))
	t.Cleanup(func() { _ = os.Chmod(readonly, 0o700) })

	ta := newTestApp(t, "v0.1.0")
	ta.probe()
	ta.app.Root().SetArgs([]string{"probe", "--db", filepath.Join(readonly, "sub", "todo.db")})

	code := ta.app.Execute(context.Background())
	assert.Equal(t, 1, code)
	assert.Empty(t, ta.out.String())
	assert.Contains(t, ta.errOut.String(), "error: cannot create data directory ")
	assert.Contains(t, ta.errOut.String(), "permission denied. Set TODO_DB_PATH to a writable location.")
}

func TestDBFlagPrecedence(t *testing.T) {
	ta := newTestApp(t, "v0.1.0")
	ta.probe()
	flagPath := filepath.Join(t.TempDir(), "flag.db")
	envPath := filepath.Join(t.TempDir(), "env.db")
	xdgPath := t.TempDir()
	ta.env["TODO_DB_PATH"] = envPath
	ta.env["XDG_DATA_HOME"] = xdgPath
	ta.app.Root().SetArgs([]string{"probe", "--db", flagPath})

	code := ta.app.Execute(context.Background())
	require.Equal(t, 0, code, "stderr: %s", ta.errOut.String())
	require.NotNil(t, ta.app.state.runtime)
	assert.Equal(t, flagPath, ta.app.state.runtime.Paths.DBPath)
	_, err := os.Stat(flagPath)
	assert.NoError(t, err)
	_, err = os.Stat(envPath)
	assert.True(t, os.IsNotExist(err))
}

func TestJSONSuccess(t *testing.T) {
	ta := newTestApp(t, "v0.1.0")
	ta.probe()
	path := filepath.Join(t.TempDir(), "todo.db")
	ta.app.Root().SetArgs([]string{"probe", "--json", "--db", path})

	code := ta.app.Execute(context.Background())
	require.Equal(t, 0, code, "stderr: %s", ta.errOut.String())
	assert.Contains(t, ta.out.String(), `"db":"`+path+`"`)
	assert.Contains(t, ta.out.String(), `"schema":1`)
}

func TestJSONErrorStaysText(t *testing.T) {
	ta := newTestApp(t, "v0.1.0")
	ta.failing()
	path := filepath.Join(t.TempDir(), "todo.db")
	ta.app.Root().SetArgs([]string{"fail", "--json", "--db", path})

	code := ta.app.Execute(context.Background())
	assert.Equal(t, 1, code)
	assert.Empty(t, ta.out.String())
	assert.Equal(t, "error: boom\n", ta.errOut.String())
}

func TestCancelledContextExit2(t *testing.T) {
	ta := newTestApp(t, "v0.1.0")
	ta.probe()
	path := filepath.Join(t.TempDir(), "todo.db")
	ta.app.Root().SetArgs([]string{"probe", "--db", path})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	code := ta.app.Execute(ctx)
	assert.Equal(t, 2, code)
	assert.Empty(t, ta.out.String())
	assert.Equal(t, "error: interrupted.\n", ta.errOut.String())
}

func TestVersionOnSubcommand(t *testing.T) {
	ta := newTestApp(t, "v0.1.0")
	ta.probe()
	ta.app.Root().SetArgs([]string{"probe", "--version", "--db", filepath.Join(t.TempDir(), "todo.db")})

	code := ta.app.Execute(context.Background())
	assert.Equal(t, 0, code)
	assert.Equal(t, "todo v0.1.0 (schema 1)\n", ta.out.String())
}

func TestExplicitEmptyDBFlagExit1(t *testing.T) {
	ta := newTestApp(t, "v0.1.0")
	ta.probe()
	ta.app.Root().SetArgs([]string{"probe", "--db", ""})

	code := ta.app.Execute(context.Background())
	assert.Equal(t, 1, code)
	assert.Empty(t, ta.out.String())
	assert.Equal(t, "error: --db requires a non-empty path.\n", ta.errOut.String())
}

func TestHelpCommandDoesNotOpenDatabase(t *testing.T) {
	ta := newTestApp(t, "v0.1.0")
	ta.probe()
	dbPath := filepath.Join(t.TempDir(), "todo.db")
	ta.env["TODO_DB_PATH"] = dbPath
	ta.app.Root().SetArgs([]string{"help"})

	code := ta.app.Execute(context.Background())
	assert.Equal(t, 0, code)
	assert.Contains(t, ta.out.String(), "Usage:")
	_, err := os.Stat(dbPath)
	assert.True(t, os.IsNotExist(err), "todo help must not touch the database")
}

func TestEmptyDBFlagOnRootExit1(t *testing.T) {
	ta := newTestApp(t, "v0.1.0")
	ta.app.Root().SetArgs([]string{"--db", ""})

	code := ta.app.Execute(context.Background())
	assert.Equal(t, 1, code)
	assert.Empty(t, ta.out.String())
	assert.Equal(t, "error: --db requires a non-empty path.\n", ta.errOut.String())
}
