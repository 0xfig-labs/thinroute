// Package storage provides shared database connections for all features.
// This abstraction allows multiple features (audit logging, IAM, guardrails)
// to share a single database connection.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Type constants for storage backends
const TypeSQLite = "sqlite"

// DefaultSQLitePath is the default file path for the SQLite database.
const DefaultSQLitePath = "data/thinroute.db"

// Config holds storage configuration
type Config struct {
	// Type specifies the storage backend: must be "sqlite"
	Type string

	// SQLite configuration
	SQLite SQLiteConfig
}

// SQLiteConfig holds SQLite-specific configuration
type SQLiteConfig struct {
	// Path is the database file path (default: data/gomodel.db)
	Path string
}

// Storage manages the lifecycle of a shared storage backend.
type Storage interface {
	Close() error
}

// HealthChecker is implemented by storage backends that can verify
// connectivity to the underlying database. All concrete backends satisfy it;
// readiness checks type-assert against this interface.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// SQLiteStorage exposes a SQLite database handle.
type SQLiteStorage interface {
	Storage
	DB() *sql.DB
}

// ResolveBackend dispatches to the callback matching the concrete storage backend.
func ResolveBackend[T any](
	store Storage,
	sqlite func(*sql.DB) (T, error),
) (T, error) {
	var zero T

	switch store := store.(type) {
	case SQLiteStorage:
		if sqlite == nil {
			return zero, fmt.Errorf("sqlite handler is nil")
		}
		return sqlite(store.DB())
	default:
		return zero, fmt.Errorf("unsupported storage backend %T", store)
	}
}

// New creates a new Storage based on the configuration.
// It validates the configuration and establishes the database connection.
func New(ctx context.Context, cfg Config) (Storage, error) {
	switch cfg.Type {
	case TypeSQLite:
		return NewSQLite(cfg.SQLite)
	default:
		return nil, fmt.Errorf("unknown storage type: %s (valid: sqlite)", cfg.Type)
	}
}

// RowScanner is the single-row result shape shared by database/sql.
type RowScanner interface {
	Scan(dest ...any) error
}

// UnixTime converts a unix-seconds retention column into a time, mapping the
// 0 "never expires" sentinel to the zero time.
func UnixTime(sec int64) time.Time {
	if sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}

// UnixOrZero converts a time into a unix-seconds retention column value,
// mapping the zero time to the 0 "never expires" sentinel.
func UnixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
