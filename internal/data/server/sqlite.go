package server

import (
	"context"
	"database/sql"

	_ "modernc.org/sqlite"
)

// SQLiteRepository persists server data using a SQLite database file.
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository wires a SQLite-backed implementation of Repository.
func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{
		db: db,
	}
}

// Bootstrap creates the initial schema and prepares the store for use.
// TODO: execute schema.sql (or an equivalent migration) once available.
func (r *SQLiteRepository) Bootstrap(ctx context.Context) error {
	return nil
}

var _ Repository = (*SQLiteRepository)(nil)
