-- 0001_initial.sql
CREATE TABLE categories (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL COLLATE NOCASE UNIQUE,
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL
);

CREATE TABLE tags (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL UNIQUE,
    created_at  TEXT    NOT NULL
);

CREATE TABLE tasks (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    title        TEXT    NOT NULL,
    notes        TEXT    NOT NULL DEFAULT '',
    due_date     TEXT,
    category_id  INTEGER REFERENCES categories(id) ON DELETE RESTRICT,
    completed_at TEXT,
    deleted_at   TEXT,
    created_at   TEXT    NOT NULL,
    updated_at   TEXT    NOT NULL
);

CREATE TABLE task_tags (
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    tag_id  INTEGER NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
    PRIMARY KEY (task_id, tag_id)
);

CREATE INDEX idx_tasks_due_date     ON tasks(due_date);
CREATE INDEX idx_tasks_completed_at ON tasks(completed_at);
CREATE INDEX idx_tasks_deleted_at   ON tasks(deleted_at);
CREATE INDEX idx_tasks_category_id  ON tasks(category_id);
CREATE INDEX idx_task_tags_tag_id   ON task_tags(tag_id);
