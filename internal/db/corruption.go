package db

// Corruption handling is intentionally lazy.
//
// The open sequence never runs an integrity check: doing so would threaten the
// startup budget on large databases. When SQLite raises a corruption-class
// error during open, pragma setup, migration, or a command query, Store.classify
// (see store.go) runs PRAGMA quick_check once to confirm and then returns the
// documented error. Confirmations that report "ok" preserve the original
// error, so a one-off driver quirk never masks a real failure.
