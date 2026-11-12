package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"todo/internal/apperr"
	"todo/internal/clock"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migration is one embedded SQL upgrade step.
type migration struct {
	Version int
	Name    string
	SQL     string
}

const createSchemaMigrations = `CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT    NOT NULL
);`

// SupportedVersion reports the highest embedded migration version. It is the
// schema version this binary was built to handle.
func SupportedVersion() int {
	migrations, err := loadMigrations()
	if err != nil || len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].Version
}

// Migrate creates a fresh schema or applies pending upgrades. It refuses
// databases newer than the binary and databases that are not ours.
func (s *Store) Migrate(ctx context.Context) error {
	migrations, err := loadMigrations()
	if err != nil {
		return apperr.Systemf("migration failed: %s. Database left unchanged.", fmtReason(err))
	}
	return s.migrate(ctx, migrations)
}

func (s *Store) migrate(ctx context.Context, migrations []migration) error {
	supported := 0
	if len(migrations) > 0 {
		supported = migrations[len(migrations)-1].Version
	}

	tables, err := s.userTables(ctx)
	if err != nil {
		return s.classify(err)
	}

	fresh := len(tables) == 0
	if !fresh {
		hasMigrations := false
		for _, table := range tables {
			if table == "schema_migrations" {
				hasMigrations = true
				break
			}
		}
		if !hasMigrations {
			return apperr.Systemf("database file at %s is not a todo database. Back up the file before retrying.", s.displayPath)
		}
		var count int
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
			return s.classify(err)
		}
		if count == 0 && len(tables) == 1 {
			fresh = true
		}
	}

	if fresh {
		return s.initFresh(ctx, migrations)
	}

	effective, err := s.effectiveVersion(ctx)
	if err != nil {
		return s.classify(err)
	}
	if effective > supported {
		return apperr.Userf("database schema v%d is newer than supported v%d. Upgrade todo before using this database.", effective, supported)
	}
	if effective == supported {
		s.schemaVersion = effective
		return nil
	}

	for _, m := range migrations {
		if m.Version <= effective {
			continue
		}
		if err := s.applyMigration(ctx, m); err != nil {
			return err
		}
	}
	s.schemaVersion = supported
	return nil
}

func (s *Store) initFresh(ctx context.Context, migrations []migration) error {
	failedVersion := 0
	if len(migrations) > 0 {
		failedVersion = migrations[0].Version
	}
	err := s.WriteTx(ctx, func(tx *sql.Tx) error {
		failedVersion = 0
		if len(migrations) > 0 {
			failedVersion = migrations[0].Version
		}
		if _, err := tx.ExecContext(ctx, createSchemaMigrations); err != nil {
			return err
		}
		for _, m := range migrations {
			if err := execMigrationSQL(ctx, tx, m, s.clk); err != nil {
				failedVersion = m.Version
				return err
			}
		}
		return nil
	})
	if err != nil {
		return s.migrationFailure(err, failedVersion)
	}
	if len(migrations) > 0 {
		s.schemaVersion = migrations[len(migrations)-1].Version
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, m migration) error {
	err := s.WriteTx(ctx, func(tx *sql.Tx) error {
		return execMigrationSQL(ctx, tx, m, s.clk)
	})
	if err != nil {
		return s.migrationFailure(err, m.Version)
	}
	s.schemaVersion = m.Version
	return nil
}

func execMigrationSQL(ctx context.Context, tx *sql.Tx, m migration, clk clock.Clock) error {
	for _, stmt := range splitStatements(m.SQL) {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
		m.Version, clock.Timestamp(clk.Now())); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", m.Version)); err != nil {
		return err
	}
	return nil
}

// migrationFailure converts a migration error into the documented message
// while preserving lock-exhaustion and corruption errors.
func (s *Store) migrationFailure(err error, version int) error {
	if err == errDatabaseLocked {
		return err
	}
	if interrupted(err) {
		return apperr.System("interrupted.")
	}
	if IsCorruption(err) {
		return s.classify(err)
	}
	if _, ok := apperr.As(err); ok {
		return err
	}
	return apperr.Systemf("migration to v%d failed: %s. Database left unchanged.", version, fmtReason(err))
}

func (s *Store) userTables(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

func (s *Store) effectiveVersion(ctx context.Context) (int, error) {
	var maxVersion sql.NullInt64
	if err := s.db.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&maxVersion); err != nil {
		return 0, err
	}
	effective := int(maxVersion.Int64)
	var userVersion int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&userVersion); err != nil {
		return 0, err
	}
	if userVersion > effective {
		effective = userVersion
	}
	return effective, nil
}

func loadMigrations() ([]migration, error) {
	names, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	result := make([]migration, 0, len(names))
	for _, name := range names {
		base := strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql")
		prefix, _, ok := strings.Cut(base, "_")
		if !ok {
			return nil, fmt.Errorf("migration %q has no version prefix", name)
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("migration %q has an invalid version: %w", name, err)
		}
		data, err := migrationFS.ReadFile(name)
		if err != nil {
			return nil, err
		}
		result = append(result, migration{
			Version: version,
			Name:    base,
			SQL:     string(data),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version < result[j].Version })
	return result, nil
}

// splitStatements splits a SQL script on semicolons while respecting string
// literals, quoted identifiers, and comments. The modernc driver prepares a
// single statement per call, so scripts must be executed one statement at a
// time.
func splitStatements(script string) []string {
	var statements []string
	var b strings.Builder

	var (
		inSingle bool
		inDouble bool
		inTick   bool
		inLine   bool
		inBlock  bool
	)

	runes := []rune(script)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]

		if inLine {
			b.WriteRune(ch)
			if ch == '\n' {
				inLine = false
			}
			continue
		}
		if inBlock {
			b.WriteRune(ch)
			if ch == '*' && i+1 < len(runes) && runes[i+1] == '/' {
				b.WriteRune(runes[i+1])
				i++
				inBlock = false
			}
			continue
		}
		if inSingle || inDouble || inTick {
			b.WriteRune(ch)
			if ch == '\'' && inSingle {
				if i+1 < len(runes) && runes[i+1] == '\'' {
					b.WriteRune(runes[i+1])
					i++
				} else {
					inSingle = false
				}
			} else if ch == '"' && inDouble {
				inDouble = false
			} else if ch == '`' && inTick {
				inTick = false
			}
			continue
		}

		switch ch {
		case '\'':
			inSingle = true
			b.WriteRune(ch)
		case '"':
			inDouble = true
			b.WriteRune(ch)
		case '`':
			inTick = true
			b.WriteRune(ch)
		case '-':
			if i+1 < len(runes) && runes[i+1] == '-' {
				inLine = true
				b.WriteString("--")
				i++
				continue
			}
			b.WriteRune(ch)
		case '/':
			if i+1 < len(runes) && runes[i+1] == '*' {
				inBlock = true
				b.WriteString("/*")
				i++
				continue
			}
			b.WriteRune(ch)
		case ';':
			if stmt := strings.TrimSpace(b.String()); stmt != "" {
				statements = append(statements, stmt)
			}
			b.Reset()
		default:
			b.WriteRune(ch)
		}
	}
	if stmt := strings.TrimSpace(b.String()); stmt != "" {
		statements = append(statements, stmt)
	}
	return statements
}
