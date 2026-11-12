# todo

A single-user, local-first task manager for the terminal. Tasks live in a local
SQLite database; there are no accounts, no network calls, and no setup step.

This repository currently contains **F01 — CLI Foundation and Local Storage**:
the command shell, global flags, exit-code contract, output conventions, and
the versioned SQLite store that later task-management features build on.

## Requirements

- Go 1.23 or newer
- GNU Make (optional)
- No C toolchain; the SQLite driver is pure Go

## Build

```sh
make build          # writes ./bin/todo
make install        # installs into $GOBIN or $GOPATH/bin
```

Version metadata is injected at build time from `git describe`; untagged builds
report `dev`.

## Usage

```sh
todo --version                 # todo v0.1.0 (schema 1)
todo --version --json          # {"version":"v0.1.0","schema_version":1}
todo --help
```

Global flags: `--json`, `--db`, `--no-color`, `--help`, `--version`.

The first command that needs storage creates the data directory (`0700`) and
database (`0600`) silently:

1. `--db <path>`
2. `TODO_DB_PATH`
3. `$XDG_DATA_HOME/todo/todo.db`
4. `$HOME/.local/share/todo/todo.db`

A leading `~/` is expanded and relative paths are resolved to absolute paths.

## Exit codes

| Code | Meaning |
|------|---------|
| `0`  | Success |
| `1`  | User or validation error |
| `2`  | Unexpected or system error |

Failures write exactly one line to stderr (`error: <message>`) and leave stdout
empty. `--json` affects successful stdout only; errors remain plain text.

## Storage

- SQLite 3.35+, WAL journal mode, foreign keys on, 5-second busy timeout.
- Schema v1: `categories`, `tags`, `tasks`, `task_tags`, and
  `schema_migrations`.
- Migrations are embedded and applied transactionally.
- A database newer than the binary is refused; a corrupt database is reported
  without running a full integrity check on every startup.
- Write transactions retry busy/locked failures up to three times with a 100 ms
  backoff.

## Development

```sh
make test           # go test ./...
make test-race      # go test -race ./...
make cover          # coverage.out + coverage.html
make lint           # go vet ./...
make fmt            # gofmt
```

Tests use real temporary SQLite files; the CLI tests run the real Cobra tree
in-process, and `test/integration` builds and exercises the binary.
