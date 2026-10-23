// Package storage owns SQLite schema and persistence operations.
package storage

import "database/sql"

const schemaVersion = 14

type SQLite struct {
	db *sql.DB
}

// ConfigureConnectionPool allows dashboard reads to run while the bounded
// ingest worker writes to SQLite. WAL still serializes SQLite writers.
func ConfigureConnectionPool(db *sql.DB) {
	const connectionPoolSize = 8
	db.SetMaxOpenConns(connectionPoolSize)
	db.SetMaxIdleConns(connectionPoolSize)
}

func NewSQLite(db *sql.DB) *SQLite {
	return &SQLite{db: db}
}
