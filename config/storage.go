package config

import "github.com/0xfig-labs/thinroute/internal/storage"

// StorageConfig holds database storage configuration (used by audit logging, usage tracking, future IAM, etc.)
type StorageConfig struct {
	// Type specifies the storage backend: "sqlite" (default)
	Type string `yaml:"type" env:"STORAGE_TYPE"`

	// SQLite configuration
	SQLite SQLiteStorageConfig `yaml:"sqlite"`
}

// SQLiteStorageConfig holds SQLite-specific storage configuration
type SQLiteStorageConfig struct {
	// Path is the database file path (default: data/thinroute.db)
	Path string `yaml:"path" env:"SQLITE_PATH"`
}

// BackendConfig converts the application storage config into the internal storage config.
func (c StorageConfig) BackendConfig() storage.Config {
	cfg := storage.Config{
		Type: c.Type,
		SQLite: storage.SQLiteConfig{
			Path: c.SQLite.Path,
		},
	}
	if cfg.Type == "" {
		cfg.Type = storage.TypeSQLite
	}
	if cfg.SQLite.Path == "" {
		cfg.SQLite.Path = storage.DefaultSQLitePath
	}
	return cfg
}
