CREATE TABLE backups_replacement (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER REFERENCES subscriptions(id) ON DELETE SET NULL,
    path TEXT NOT NULL UNIQUE,
    size_bytes INTEGER,
    status TEXT NOT NULL CHECK (status IN ('running', 'complete', 'failed')),
    started_at TEXT NOT NULL,
    finished_at TEXT,
    error TEXT
);

INSERT INTO backups_replacement (id, subscription_id, path, size_bytes, status, started_at, finished_at, error)
SELECT id, subscription_id, path, size_bytes, status, started_at, finished_at, error FROM backups;

DROP TABLE backups;
ALTER TABLE backups_replacement RENAME TO backups;
