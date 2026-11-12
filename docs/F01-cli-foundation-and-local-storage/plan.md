# Implementation Plan: CLI Foundation and Local Storage

**Prerequisites:**
- Go 1.23 or newer
- GNU Make
- No C toolchain required; the SQLite driver is pure Go
- Network access to download Go modules
- Optional: `sqlite3` CLI for manual inspection
- Optional: `git` for version stamping; untagged builds fall back to `dev`

## Stage 1: Project Scaffolding and CLI Shell

**1. Go Module and Dependency Baseline** - Initialize the `todo` module and add the command framework, pure-Go SQLite driver, and terminal detection package. Pin the resolved versions in the module files. See the spec's decision log and component overview for the exact packages.

**2. Build Automation and Version Metadata** - Create the Makefile with the documented build, test, race, coverage, lint, format, install, and clean targets. Inject the version at build time and add the ignore rules for build and local database artifacts.

**3. Cobra Root Command and Global Flags** - Implement the root command with the persistent `--json`, `--db`, `--no-color`, and `--version` flags. Disable the framework's default error and usage printing so the application controls every line written to stderr.

**4. Command Group Scaffold** - Register the task, organization, and maintenance help groups with their documented identifiers and titles. Confirm that later commands can attach by setting a group identifier without changing the root structure.

**5. Error Taxonomy and Exit-Code Mapping** - Add the user and system error types, the unexpected-failure wrapper, and the single execution boundary that converts errors into the documented stderr lines and exit codes. Classify framework-originated flag and command errors as user errors.

## Stage 2: Configuration and Runtime Services

**6. Database Path Resolution** - Implement the full precedence chain, XDG and home fallbacks, tilde expansion, absolute-path normalization, empty-value handling, and display formatting for error messages.

**7. Clock Abstraction** - Add the clock interface and the system implementation, including local-day and UTC-timestamp helpers. Provide a controllable fake implementation so later date-sensitive features can use a deterministic clock.

**8. Output Printer and Color Policy** - Implement the shared printer for plain lines, JSON documents, notes, errors, and aligned tables. Add TTY detection and the `--no-color` override, and make sure JSON output is never colored.

**9. Runtime Context Wiring** - Build the per-command runtime object that carries configuration, the store, the printer, and the clock. Attach it to the command context so later features can retrieve it without re-resolving paths or reopening the database.

## Stage 3: SQLite Storage Bootstrap

**10. Database Open and Permissions** - Open or create the database file with the documented permissions, create the parent directory with the documented permissions, and apply the connection pragmas.

**11. Write Transaction Retry Helper** - Implement the write transaction wrapper as specified, including contention handling, application-error preservation, context cancellation, and the documented exhaustion error.

**12. Lazy Corruption Detection** - Detect corruption-class SQLite errors and run the confirmation check only when such an error appears. Surface the documented corrupt-database error and keep ordinary startup free of full integrity checks.

## Stage 4: Schema and Migration Runner

**13. Schema v1 Migration** - Create the initial migration with the category, tag, task, and task-tag tables, plus the documented indexes, uniqueness rules, and referential actions.

**14. Migration Runner and Version Check** - Embed the migration files, bootstrap the schema-version table, read the effective version, and refuse databases newer than the binary with the documented error.

**15. Transactional Application and First-Run Initialization** - Initialize a fresh database and apply pending upgrades during command startup so the first storage-using command initializes silently.

## Stage 5: Command Surface and Release Readiness

**16. Version Surface** - Implement the `--version` text output and its JSON variant. Ensure version handling bypasses path resolution and database access.

**17. Help and No-Argument Behavior** - Make `--help` and bare `todo` render the grouped help without opening the database, and ensure unknown commands and flags produce the documented single-line error with exit code 1.

**18. Build Targets, Documentation, and Contract Sweep** - Finalize the Makefile targets, add a short README with build and usage instructions, and verify every documented flag, exit code, error line, permission, and JSON convention against the spec before closing the feature.
