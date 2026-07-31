package config

import (
	"os"
	"testing"
)

func TestLoad_FromEnvironment(t *testing.T) {
	_ = os.Unsetenv("PORT")

	result, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Config.Server.Listen != "127.0.0.1:52180" {
		t.Errorf("expected default listen 127.0.0.1:52180, got %s", result.Config.Server.Listen)
	}
}
