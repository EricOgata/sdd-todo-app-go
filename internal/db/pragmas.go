package db

import (
	"context"
	"strings"

	"todo/internal/apperr"
)

var connectionPragmas = []string{
	"PRAGMA foreign_keys = ON;",
	"PRAGMA busy_timeout = 5000;",
	"PRAGMA synchronous = NORMAL;",
}

func (s *Store) applyPragmas(ctx context.Context) error {
	var mode string
	if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode = WAL;").Scan(&mode); err != nil {
		return err
	}
	if !strings.EqualFold(mode, "wal") {
		return apperr.Systemf("failed to enable WAL journal mode (got %q).", mode)
	}
	for _, stmt := range connectionPragmas {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
