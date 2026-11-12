package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustExec(t *testing.T, store *Store, query string, args ...any) {
	t.Helper()
	_, err := store.DB().ExecContext(context.Background(), query, args...)
	require.NoError(t, err)
}

func insertTask(t *testing.T, store *Store, title string, categoryID sql.NullInt64, due sql.NullString) int64 {
	t.Helper()
	res, err := store.DB().ExecContext(context.Background(),
		`INSERT INTO tasks (title, notes, due_date, category_id, created_at, updated_at)
		 VALUES (?, '', ?, ?, '2026-11-12T09:00:00Z', '2026-11-12T09:00:00Z')`,
		title, due, categoryID)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

func TestSchemaV1TablesAndIndexes(t *testing.T) {
	store := newTestStore(t)
	tables := listNames(t, store, "table")
	assert.ElementsMatch(t,
		[]string{"categories", "tags", "tasks", "task_tags", "schema_migrations"},
		tables)

	indexes := listNames(t, store, "index")
	for _, want := range []string{
		"idx_tasks_due_date",
		"idx_tasks_completed_at",
		"idx_tasks_deleted_at",
		"idx_tasks_category_id",
		"idx_task_tags_tag_id",
	} {
		assert.Contains(t, indexes, want)
	}
}

func TestCategoryUniqueNOCASE(t *testing.T) {
	store := newTestStore(t)
	insertCategory(context.Background(), store, "Work")
	err := insertCategory(context.Background(), store, "work")
	require.Error(t, err)
}

func TestCategoryDeleteRestricted(t *testing.T) {
	store := newTestStore(t)
	insertCategory(context.Background(), store, "Work")
	var categoryID int64
	require.NoError(t, store.DB().QueryRowContext(context.Background(),
		"SELECT id FROM categories WHERE name = 'Work'").Scan(&categoryID))
	insertTask(t, store, "Task", sql.NullInt64{Int64: categoryID, Valid: true}, sql.NullString{})

	_, err := store.DB().ExecContext(context.Background(), "DELETE FROM categories WHERE id = ?", categoryID)
	require.Error(t, err)
}

func TestCategoryRenameKeepsTaskReference(t *testing.T) {
	store := newTestStore(t)
	insertCategory(context.Background(), store, "Work")
	var categoryID int64
	require.NoError(t, store.DB().QueryRowContext(context.Background(),
		"SELECT id FROM categories WHERE name = 'Work'").Scan(&categoryID))
	taskID := insertTask(t, store, "Task", sql.NullInt64{Int64: categoryID, Valid: true}, sql.NullString{})

	mustExec(t, store, "UPDATE categories SET name = 'Job' WHERE id = ?", categoryID)

	var joinedCategoryID int64
	require.NoError(t, store.DB().QueryRowContext(context.Background(),
		`SELECT c.id FROM tasks t JOIN categories c ON c.id = t.category_id WHERE t.id = ?`, taskID).Scan(&joinedCategoryID))
	assert.Equal(t, categoryID, joinedCategoryID)

	var name string
	require.NoError(t, store.DB().QueryRowContext(context.Background(),
		"SELECT name FROM categories WHERE id = ?", joinedCategoryID).Scan(&name))
	assert.Equal(t, "Job", name)
}

func insertTag(t *testing.T, store *Store, name string) int64 {
	t.Helper()
	res, err := store.DB().ExecContext(context.Background(),
		"INSERT INTO tags (name, created_at) VALUES (?, '2026-11-12T09:00:00Z')", name)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

func TestTagUnique(t *testing.T) {
	store := newTestStore(t)
	insertTag(t, store, "urgent")
	_, err := store.DB().ExecContext(context.Background(),
		"INSERT INTO tags (name, created_at) VALUES ('urgent', 'now')")
	require.Error(t, err)
}

func TestTagRenameKeepsTaskAssociation(t *testing.T) {
	store := newTestStore(t)
	taskID := insertTask(t, store, "Task", sql.NullInt64{}, sql.NullString{})
	tagID := insertTag(t, store, "urgent")
	mustExec(t, store, "INSERT INTO task_tags (task_id, tag_id) VALUES (?, ?)", taskID, tagID)

	mustExec(t, store, "UPDATE tags SET name = 'priority' WHERE id = ?", tagID)

	var count int
	require.NoError(t, store.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM task_tags tt JOIN tags g ON g.id = tt.tag_id
		 WHERE tt.task_id = ? AND g.name = 'priority'`, taskID).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestTaskTagsCascadeOnTagDelete(t *testing.T) {
	store := newTestStore(t)
	taskID := insertTask(t, store, "Task", sql.NullInt64{}, sql.NullString{})
	tagID := insertTag(t, store, "urgent")
	mustExec(t, store, "INSERT INTO task_tags (task_id, tag_id) VALUES (?, ?)", taskID, tagID)

	mustExec(t, store, "DELETE FROM tags WHERE id = ?", tagID)

	var joinCount int
	require.NoError(t, store.DB().QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM task_tags").Scan(&joinCount))
	assert.Zero(t, joinCount)

	var taskCount int
	require.NoError(t, store.DB().QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM tasks WHERE id = ?", taskID).Scan(&taskCount))
	assert.Equal(t, 1, taskCount)
}

func TestTaskTagsCascadeOnTaskPurge(t *testing.T) {
	store := newTestStore(t)
	taskID := insertTask(t, store, "Task", sql.NullInt64{}, sql.NullString{})
	tagID := insertTag(t, store, "urgent")
	mustExec(t, store, "INSERT INTO task_tags (task_id, tag_id) VALUES (?, ?)", taskID, tagID)

	mustExec(t, store, "DELETE FROM tasks WHERE id = ?", taskID)

	var joinCount int
	require.NoError(t, store.DB().QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM task_tags").Scan(&joinCount))
	assert.Zero(t, joinCount)

	var tagCount int
	require.NoError(t, store.DB().QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM tags WHERE id = ?", tagID).Scan(&tagCount))
	assert.Equal(t, 1, tagCount)
}

func TestTaskIDsNeverReused(t *testing.T) {
	store := newTestStore(t)
	first := insertTask(t, store, "First", sql.NullInt64{}, sql.NullString{})
	mustExec(t, store, "DELETE FROM tasks WHERE id = ?", first)
	second := insertTask(t, store, "Second", sql.NullInt64{}, sql.NullString{})
	assert.Greater(t, second, first)
}

func TestSchemaV1LifecycleColumns(t *testing.T) {
	store := newTestStore(t)
	rows, err := store.DB().QueryContext(context.Background(), "PRAGMA table_info(tasks)")
	require.NoError(t, err)
	defer rows.Close()

	type column struct {
		name    string
		notNull bool
	}
	found := map[string]column{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt sql.NullString
		require.NoError(t, rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk))
		found[name] = column{name: name, notNull: notNull != 0}
	}
	require.NoError(t, rows.Err())

	for _, want := range []string{"completed_at", "deleted_at"} {
		col, ok := found[want]
		require.True(t, ok, "missing column %s", want)
		assert.False(t, col.notNull, "%s must be nullable", want)
	}
}

func TestDueDateStoredAsISODate(t *testing.T) {
	store := newTestStore(t)
	insertTask(t, store, "Late", sql.NullInt64{}, sql.NullString{String: "2026-10-01", Valid: true})
	insertTask(t, store, "Later", sql.NullInt64{}, sql.NullString{String: "2026-12-01", Valid: true})

	rows, err := store.DB().QueryContext(context.Background(),
		"SELECT due_date FROM tasks ORDER BY due_date")
	require.NoError(t, err)
	defer rows.Close()
	var dates []string
	for rows.Next() {
		var due string
		require.NoError(t, rows.Scan(&due))
		dates = append(dates, due)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"2026-10-01", "2026-12-01"}, dates)
}

func TestDerivedStatus(t *testing.T) {
	store := newTestStore(t)
	openID := insertTask(t, store, "Open", sql.NullInt64{}, sql.NullString{})
	completedID := insertTask(t, store, "Completed", sql.NullInt64{}, sql.NullString{})
	mustExec(t, store, "UPDATE tasks SET completed_at = '2026-11-12T09:00:00Z' WHERE id = ?", completedID)

	status := func(id int64) string {
		var got string
		require.NoError(t, store.DB().QueryRowContext(context.Background(),
			"SELECT CASE WHEN completed_at IS NULL THEN 'open' ELSE 'completed' END FROM tasks WHERE id = ?", id).Scan(&got))
		return got
	}
	assert.Equal(t, "open", status(openID))
	assert.Equal(t, "completed", status(completedID))
}
