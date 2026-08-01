package config

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed example.yaml
var exampleConfig []byte

// DefaultConfigDir returns the user configuration directory.
func DefaultConfigDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "thinroute")
}

// DefaultConfigPath returns the default user configuration file path.
func DefaultConfigPath() string {
	if dir := DefaultConfigDir(); dir != "" {
		return filepath.Join(dir, "config.yaml")
	}
	return ""
}

// ExampleConfig returns a copy of the built-in configuration example.
func ExampleConfig() []byte {
	return append([]byte(nil), exampleConfig...)
}

// InitDefaultConfig creates the default configuration without overwriting it.
func InitDefaultConfig() (string, error) {
	path := DefaultConfigPath()
	if path == "" {
		return "", errors.New("cannot determine user config directory")
	}
	if _, err := os.Stat(path); err == nil {
		return path, fmt.Errorf("config file already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("check %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, exampleConfig, 0o600); err != nil {
		return "", fmt.Errorf("create config file: %w", err)
	}
	return path, nil
}
