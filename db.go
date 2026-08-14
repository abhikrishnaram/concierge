package main

import (
	"database/sql"
	"os"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS webhook_routes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	path TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	script TEXT NOT NULL,
	token TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS cron_jobs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	schedule TEXT NOT NULL,
	script TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS runs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	source_type TEXT NOT NULL,
	source_id INTEGER NOT NULL,
	started_at DATETIME NOT NULL,
	exit_code INTEGER NOT NULL,
	stdout TEXT NOT NULL,
	stderr TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_runs_source ON runs(source_type, source_id, started_at DESC);

CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	created_at DATETIME NOT NULL,
	expires_at DATETIME NOT NULL
);
`

func openDB() (*sql.DB, error) {
	if err := os.MkdirAll(dataDir(), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath()+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	// ponytail: single writer avoids "database is locked" from concurrent cron+HTTP writes; move to a connection pool if throughput ever demands it
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
