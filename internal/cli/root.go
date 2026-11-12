package cli

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"todo/internal/apperr"
	"todo/internal/buildinfo"
	"todo/internal/clock"
	"todo/internal/config"
	"todo/internal/db"
	"todo/internal/output"
)

// Options configures an App. Zero values fall back to process defaults.
type Options struct {
	// Out receives program output.
	Out io.Writer
	// Err receives diagnostics and errors.
	Err io.Writer
	// Version overrides the compiled-in build version. Empty uses
	// buildinfo.Version.
	Version string
	// IsTerminal overrides TTY detection for color policy.
	IsTerminal func(io.Writer) bool
	// Env overrides environment lookup used by path resolution.
	Env func(string) string
	// HomeDir overrides the user home lookup used by path resolution.
	HomeDir func() (string, error)
	// Clock overrides the time source. Nil uses the system clock.
	Clock clock.Clock
}

// App owns the root command and the execution boundary.
type App struct {
	root  *cobra.Command
	state *rootState
	opts  Options
}

type rootState struct {
	opts    Options
	root    *cobra.Command
	printer *output.Printer
	store   *db.Store
	runtime *Runtime
}

// NewApp builds the root command tree.
func NewApp(opts Options) *App {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.Err == nil {
		opts.Err = os.Stderr
	}
	if opts.Clock == nil {
		opts.Clock = clock.System{}
	}

	state := &rootState{opts: opts}

	root := &cobra.Command{
		Use:   "todo",
		Short: "A local-first task manager for the terminal",
		Long: "todo is a single-user, local-first task manager.\n" +
			"It stores tasks in a local SQLite database and never makes network calls.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetOut(opts.Out)
	root.SetErr(opts.Err)
	root.CompletionOptions.DisableDefaultCmd = true

	for _, group := range commandGroups() {
		root.AddGroup(group)
	}

	flags := root.PersistentFlags()
	flags.Bool("json", false, "emit one JSON document on stdout (read commands)")
	flags.String("db", "", "path to the SQLite database file")
	flags.Bool("no-color", false, "disable ANSI styling")
	flags.Bool("version", false, "print version information and exit")

	root.PersistentPreRunE = state.preRun
	state.root = root

	return &App{root: root, state: state, opts: opts}
}

// Root exposes the command tree so later features and tests can register
// additional commands.
func (a *App) Root() *cobra.Command { return a.root }

// preRun builds output, handles version and bare-root help, and initializes
// storage for subcommands.
func (s *rootState) preRun(cmd *cobra.Command, _ []string) error {
	jsonMode, _ := cmd.Flags().GetBool("json")
	noColor, _ := cmd.Flags().GetBool("no-color")
	s.printer = output.New(output.Options{
		Out:        s.opts.Out,
		Err:        s.opts.Err,
		JSON:       jsonMode,
		NoColor:    noColor,
		IsTerminal: s.opts.IsTerminal,
	})

	versionFlag, _ := cmd.Flags().GetBool("version")
	if versionFlag {
		return s.printVersion(jsonMode)
	}

	// An explicitly empty --db is a usage error even on the database-free
	// root/help path.
	if s.root.PersistentFlags().Changed("db") {
		if dbFlag, _ := cmd.Flags().GetString("db"); strings.TrimSpace(dbFlag) == "" {
			return apperr.User("--db requires a non-empty path.")
		}
	}

	if cmd == s.root {
		_ = cmd.Help()
		return errStop
	}
	// The auto-generated help command must not touch storage.
	if cmd.Name() == "help" && cmd.Parent() == s.root {
		return nil
	}

	ctx := cmd.Context()
	if err := ctx.Err(); err != nil {
		return apperr.System("interrupted.")
	}

	dbFlag, _ := cmd.Flags().GetString("db")
	dbFlagSet := s.root.PersistentFlags().Changed("db")

	env := s.opts.Env
	if env == nil {
		env = os.Getenv
	}
	homeDir := s.opts.HomeDir
	if homeDir == nil {
		homeDir = os.UserHomeDir
	}

	paths, err := config.Resolve(config.Input{
		DBFlag:    dbFlag,
		DBFlagSet: dbFlagSet,
		Env:       env,
		HomeDir:   homeDir,
	})
	if err != nil {
		return err
	}

	store, err := db.Open(ctx, db.OpenOptions{
		Path:        paths.DBPath,
		DisplayPath: paths.DisplayDB,
		DisplayDir:  paths.DisplayDir,
		Clock:       s.opts.Clock,
	})
	if err != nil {
		return err
	}
	if err := store.Migrate(ctx); err != nil {
		_ = store.Close()
		return err
	}
	s.store = store

	rt := &Runtime{
		Paths:   paths,
		Store:   store,
		Printer: s.printer,
		Clock:   s.opts.Clock,
	}
	s.runtime = rt
	cmd.SetContext(withRuntime(ctx, rt))
	return nil
}

// errStop short-circuits command execution for paths that have already
// produced their output (help, version). It is not an error.
var errStop = errors.New("cli: stop")

// schemaVersion is the schema version reported by the binary.
func (s *rootState) schemaVersion() int { return buildinfo.SchemaVersion }
