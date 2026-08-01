package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigPathUsesXDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	if got, want := DefaultConfigPath(), filepath.Join(dir, "thinroute", "config.yaml"); got != want {
		t.Fatalf("DefaultConfigPath() = %q, want %q", got, want)
	}
}

func TestInitDefaultConfigDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path, err := InitDefaultConfig()
	if err != nil {
		t.Fatalf("InitDefaultConfig() error = %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || len(data) == 0 {
		t.Fatalf("created config = %q, read error = %v", data, err)
	}
	if _, err := InitDefaultConfig(); err == nil {
		t.Fatal("second InitDefaultConfig() succeeded, want existing-file error")
	}
}
