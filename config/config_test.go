package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// clearProviderEnvVars unsets all known provider-related environment variables.
func clearProviderEnvVars(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_MODELS",
		"ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "ANTHROPIC_MODELS",
		"GEMINI_API_KEY", "GEMINI_BASE_URL", "GEMINI_MODELS",
		"DEEPSEEK_API_KEY", "DEEPSEEK_BASE_URL", "DEEPSEEK_MODELS",
		"XAI_API_KEY", "XAI_BASE_URL", "XAI_MODELS",
		"GROQ_API_KEY", "GROQ_BASE_URL", "GROQ_MODELS",
		"OPENROUTER_API_KEY", "OPENROUTER_BASE_URL", "OPENROUTER_MODELS", "OPENROUTER_SITE_URL", "OPENROUTER_APP_NAME",
		"ZAI_API_KEY", "ZAI_BASE_URL", "ZAI_MODELS",
		"AZURE_API_KEY", "AZURE_BASE_URL", "AZURE_API_VERSION", "AZURE_MODELS",
		"ORACLE_API_KEY", "ORACLE_BASE_URL", "ORACLE_MODELS",
		"VLLM_API_KEY", "VLLM_BASE_URL", "VLLM_MODELS",
		"OLLAMA_API_KEY", "OLLAMA_BASE_URL", "OLLAMA_MODELS",
	} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

// clearAllConfigEnvVars unsets all config-related environment variables.
func clearAllConfigEnvVars(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"CONFIG_STRICT",
		"PORT", "BASE_PATH", "BODY_SIZE_LIMIT", "PPROF_ENABLED", "ENABLE_PASSTHROUGH_ROUTES", "ALLOW_PASSTHROUGH_V1_ALIAS", "USER_PATH_HEADER", "ENABLED_PASSTHROUGH_PROVIDERS",
		"THINROUTE_CACHE_DIR", "CACHE_REFRESH_INTERVAL",
		"RESPONSE_CACHE_SIMPLE_ENABLED",
		"STORAGE_TYPE", "SQLITE_PATH",
		"METRICS_ENABLED", "METRICS_ENDPOINT",
		"LOGGING_ENABLED", "LOGGING_BUFFER_SIZE",
		"LOGGING_ONLY_MODEL_INTERACTIONS",
		"LOGGING_FLUSH_INTERVAL", "LOGGING_RETENTION_DAYS",
		"USAGE_ENABLED", "ENFORCE_RETURNING_USAGE_DATA",
		"USAGE_BUFFER_SIZE", "USAGE_FLUSH_INTERVAL", "USAGE_RETENTION_DAYS",
		"MODELS_ENABLED_BY_DEFAULT", "KEEP_ONLY_ALIASES_AT_MODELS_ENDPOINT", "CONFIGURED_PROVIDER_MODELS_MODE",
		"HTTP_TIMEOUT", "HTTP_RESPONSE_HEADER_TIMEOUT",
	} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
	clearProviderEnvVars(t)
}

// withTempDir runs fn in a temporary directory, restoring the original working directory afterward.
func withTempDir(t *testing.T, fn func(dir string)) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()
	fn(tempDir)
}

func TestBuildDefaultConfig(t *testing.T) {
	cfg := buildDefaultConfig()

	if cfg.Server.Listen != "127.0.0.1:52180" {
		t.Errorf("expected Server.Listen=127.0.0.1:52180, got %s", cfg.Server.Listen)
	}
	if cfg.Server.BasePath != "/" {
		t.Errorf("expected Server.BasePath=/, got %s", cfg.Server.BasePath)
	}
	if cfg.Server.UserPathHeader != "X-thinroute-User-Path" {
		t.Errorf("expected Server.UserPathHeader=X-thinroute-User-Path, got %s", cfg.Server.UserPathHeader)
	}
	if cfg.Server.PprofEnabled {
		t.Error("expected Server.PprofEnabled=false")
	}
	if !cfg.Server.EnablePassthroughRoutes {
		t.Error("expected Server.EnablePassthroughRoutes=true")
	}
	if !cfg.Server.AllowPassthroughV1Alias {
		t.Error("expected Server.AllowPassthroughV1Alias=true")
	}
	if got, want := cfg.Server.EnabledPassthroughProviders, []string{"openai", "anthropic", "openrouter", "zai", "vllm", "deepseek"}; !reflect.DeepEqual(got, want) {
		t.Errorf("expected Server.EnabledPassthroughProviders=%v, got %v", want, got)
	}
	if cfg.Models.ConfiguredProviderModelsMode != ConfiguredProviderModelsModeFallback {
		t.Errorf("expected Models.ConfiguredProviderModelsMode=fallback, got %q", cfg.Models.ConfiguredProviderModelsMode)
	}
	if cfg.Cache.Model.Local != nil {
		t.Error("expected Cache.Model.Local to be nil in raw defaults")
	}
	if cfg.Cache.Model.RefreshInterval != 3600 {
		t.Errorf("expected Cache.Model.RefreshInterval=3600, got %d", cfg.Cache.Model.RefreshInterval)
	}
	if cfg.Storage.Type != "sqlite" {
		t.Errorf("expected Storage.Type=sqlite, got %s", cfg.Storage.Type)
	}
	if cfg.Storage.SQLite.Path != "data/thinroute.db" {
		t.Errorf("expected Storage.SQLite.Path=data/thinroute.db, got %s", cfg.Storage.SQLite.Path)
	}

	if cfg.Logging.BufferSize != 1000 {
		t.Errorf("expected Logging.BufferSize=1000, got %d", cfg.Logging.BufferSize)
	}
	if cfg.Logging.FlushInterval != 5 {
		t.Errorf("expected Logging.FlushInterval=5, got %d", cfg.Logging.FlushInterval)
	}
	if cfg.Logging.RetentionDays != 30 {
		t.Errorf("expected Logging.RetentionDays=30, got %d", cfg.Logging.RetentionDays)
	}
	if !cfg.Logging.OnlyModelInteractions {
		t.Error("expected Logging.OnlyModelInteractions=true")
	}
	if cfg.Logging.Enabled {
		t.Error("expected Logging.Enabled=false")
	}
	if !cfg.Usage.Enabled {
		t.Error("expected Usage.Enabled=true")
	}
	if !cfg.Usage.EnforceReturningUsageData {
		t.Error("expected Usage.EnforceReturningUsageData=true")
	}
	if cfg.Usage.BufferSize != 1000 {
		t.Errorf("expected Usage.BufferSize=1000, got %d", cfg.Usage.BufferSize)
	}
	if cfg.Usage.FlushInterval != 5 {
		t.Errorf("expected Usage.FlushInterval=5, got %d", cfg.Usage.FlushInterval)
	}
	if cfg.Usage.RetentionDays != 90 {
		t.Errorf("expected Usage.RetentionDays=90, got %d", cfg.Usage.RetentionDays)
	}
	if cfg.Metrics.Endpoint != "/metrics" {
		t.Errorf("expected Metrics.Endpoint=/metrics, got %s", cfg.Metrics.Endpoint)
	}
	if cfg.Metrics.Enabled {
		t.Error("expected Metrics.Enabled=false")
	}
	if cfg.HTTP.Timeout != 600 {
		t.Errorf("expected HTTP.Timeout=600, got %d", cfg.HTTP.Timeout)
	}
	if cfg.HTTP.ResponseHeaderTimeout != 600 {
		t.Errorf("expected HTTP.ResponseHeaderTimeout=600, got %d", cfg.HTTP.ResponseHeaderTimeout)
	}
	if !cfg.Control.Enabled {
		t.Error("expected Admin.EndpointsEnabled=true")
	}
	if !cfg.Models.EnabledByDefault {
		t.Error("expected Models.EnabledByDefault=true")
	}
	if cfg.Models.KeepOnlyAliasesAtModelsEndpoint {
		t.Error("expected Models.KeepOnlyAliasesAtModelsEndpoint=false")
	}
	if cfg.Cache.Response.Simple != nil {
		t.Errorf("expected Cache.Response.Simple=nil in defaults, got %+v", cfg.Cache.Response.Simple)
	}

	expectedRetry := DefaultRetryConfig()
	if cfg.Resilience.Retry != expectedRetry {
		t.Errorf("expected Resilience.Retry=%+v, got %+v", expectedRetry, cfg.Resilience.Retry)
	}

	expectedCB := DefaultCircuitBreakerConfig()
	if cfg.Resilience.CircuitBreaker != expectedCB {
		t.Errorf("expected Resilience.CircuitBreaker=%+v, got %+v", expectedCB, cfg.Resilience.CircuitBreaker)
	}
}

func TestLoad_ZeroConfig(t *testing.T) {
	clearAllConfigEnvVars(t)

	withTempDir(t, func(_ string) {
		result, err := Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		if result.Config.Server.Listen != "127.0.0.1:52180" {
			t.Errorf("expected default listen 127.0.0.1:52180, got %s", result.Config.Server.Listen)
		}
		if len(result.RawProviders) != 0 {
			t.Errorf("expected no raw providers, got %d", len(result.RawProviders))
		}
	})
}

func TestLoad_YAMLOverridesDefaults(t *testing.T) {
	clearAllConfigEnvVars(t)

	withTempDir(t, func(dir string) {
		yaml := `
server:
  listen: "127.0.0.1:3000"
  pprof_enabled: true
models:
  enabled_by_default: false
  keep_only_aliases_at_models_endpoint: true
  configured_provider_models_mode: allowlist
logging:
  enabled: true
  buffer_size: 500
`
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		result, err := Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}
		cfg := result.Config

		if cfg.Server.Listen != "127.0.0.1:3000" {
			t.Errorf("expected listen 127.0.0.1:3000, got %s", cfg.Server.Listen)
		}
		if !cfg.Server.PprofEnabled {
			t.Error("expected Server.PprofEnabled=true from YAML")
		}
		if cfg.Models.EnabledByDefault {
			t.Error("expected Models.EnabledByDefault=false from YAML")
		}
		if !cfg.Models.KeepOnlyAliasesAtModelsEndpoint {
			t.Error("expected Models.KeepOnlyAliasesAtModelsEndpoint=true from YAML")
		}
		if cfg.Models.ConfiguredProviderModelsMode != ConfiguredProviderModelsModeAllowlist {
			t.Errorf("expected Models.ConfiguredProviderModelsMode=allowlist from YAML, got %q", cfg.Models.ConfiguredProviderModelsMode)
		}
		if !cfg.Logging.Enabled {
			t.Error("expected Logging.Enabled=true from YAML")
		}
		if cfg.Logging.BufferSize != 500 {
			t.Errorf("expected Logging.BufferSize=500, got %d", cfg.Logging.BufferSize)
		}
		if cfg.Logging.FlushInterval != 5 {
			t.Errorf("expected Logging.FlushInterval=5 (default), got %d", cfg.Logging.FlushInterval)
		}
		if cfg.Storage.Type != "sqlite" {
			t.Errorf("expected Storage.Type=sqlite (default), got %s", cfg.Storage.Type)
		}
	})
}

func TestLoad_InvalidConfiguredProviderModelsMode(t *testing.T) {
	clearAllConfigEnvVars(t)

	withTempDir(t, func(dir string) {
		yaml := `
models:
  configured_provider_models_mode: strict
`
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		_, err := Load()
		if err == nil {
			t.Fatal("expected Load() to fail for invalid configured provider models mode")
		}
		if !strings.Contains(err.Error(), "models.configured_provider_models_mode must be one of") {
			t.Fatalf("Load() error = %v, want configured provider models mode validation error", err)
		}
	})
}

func TestLoad_InvalidConfiguredProviderModelsMode_BadKey(t *testing.T) {
	clearAllConfigEnvVars(t)

	withTempDir(t, func(dir string) {
		yaml := `
models:
  configured_provider_models_mode: strict
`
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		_, err := Load()
		if err == nil {
			t.Fatal("expected Load() to fail for invalid configured provider models mode")
		}
		if !strings.Contains(err.Error(), "models.configured_provider_models_mode must be one of") {
			t.Fatalf("Load() error = %v, want configured provider models mode validation error", err)
		}
	})
}

func TestLoad_PassthroughFlags_YAMLExpansion(t *testing.T) {
	withTempDir(t, func(dir string) {
		clearAllConfigEnvVars(t)
		t.Setenv("PASSTHROUGH_ENABLED_FROM_YAML", "false")
		t.Setenv("PASSTHROUGH_NORMALIZE_FROM_YAML", "")

		yaml := `
server:
  enable_passthrough_routes: ${PASSTHROUGH_ENABLED_FROM_YAML}
  allow_passthrough_v1_alias: ${PASSTHROUGH_NORMALIZE_FROM_YAML:-false}
`
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		result, err := Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}
		if result.Config.Server.EnablePassthroughRoutes {
			t.Fatal("expected YAML ${VAR} expansion to set EnablePassthroughRoutes=false")
		}
		if result.Config.Server.AllowPassthroughV1Alias {
			t.Fatal("expected YAML ${VAR:-default} expansion to set AllowPassthroughV1Alias=false")
		}
	})
}

func TestLoad_ConfigExample_UsesNestedModelCacheSettings(t *testing.T) {
	clearAllConfigEnvVars(t)

	examplePath, err := filepath.Abs("../config.example.yaml")
	if err != nil {
		t.Fatalf("Failed to resolve config.example.yaml path: %v", err)
	}
	exampleData, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("Failed to read config.example.yaml: %v", err)
	}

	withTempDir(t, func(dir string) {
		if err := os.MkdirAll(filepath.Join(dir, "config"), 0755); err != nil {
			t.Fatalf("Failed to create config directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config", "config.yaml"), exampleData, 0644); err != nil {
			t.Fatalf("Failed to write config/config.yaml: %v", err)
		}

		result, err := Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		if result.Config.Cache.Model.RefreshInterval != 3600 {
			t.Fatalf("Cache.Model.RefreshInterval = %d, want 3600", result.Config.Cache.Model.RefreshInterval)
		}
		if result.Config.Cache.Model.Local == nil {
			t.Fatal("expected Cache.Model.Local to be configured from example config")
		}
		if result.Config.Cache.Model.Local.CacheDir != ".cache" {
			t.Fatalf("Cache.Model.Local.CacheDir = %q, want .cache", result.Config.Cache.Model.Local.CacheDir)
		}
		gotProviders := result.Config.Server.EnabledPassthroughProviders
		wantProviders := []string{"openai", "anthropic", "openrouter", "zai", "vllm", "deepseek", "bailian"}
		if !reflect.DeepEqual(gotProviders, wantProviders) {
			t.Fatalf("Server.EnabledPassthroughProviders = %v, want %v", gotProviders, wantProviders)
		}
	})
}

func TestLoad_ProviderFromYAML(t *testing.T) {
	clearAllConfigEnvVars(t)

	withTempDir(t, func(dir string) {
		yaml := `
providers:
  openai:
    type: openai
    api_key: "sk-yaml-key"
    base_url: "https://custom.openai.com"
`
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		result, err := Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		provider, exists := result.RawProviders["openai"]
		if !exists {
			t.Fatal("expected 'openai' raw provider to exist")
		}
		if provider.APIKey != "sk-yaml-key" {
			t.Errorf("expected API key sk-yaml-key, got %s", provider.APIKey)
		}
		if provider.BaseURL != "https://custom.openai.com" {
			t.Errorf("expected base URL https://custom.openai.com, got %s", provider.BaseURL)
		}
	})
}

func TestLoad_ProviderResilienceInRawProviders(t *testing.T) {
	clearAllConfigEnvVars(t)

	withTempDir(t, func(dir string) {
		yamlContent := `
resilience:
  retry:
    max_retries: 5
providers:
  openai:
    type: openai
    api_key: "sk-yaml-key"
    resilience:
      retry:
        max_retries: 10
  anthropic:
    type: anthropic
    api_key: "sk-ant-key"
`
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yamlContent), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		result, err := Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		if result.Config.Resilience.Retry.MaxRetries != 5 {
			t.Errorf("expected global MaxRetries=5, got %d", result.Config.Resilience.Retry.MaxRetries)
		}

		openai, exists := result.RawProviders["openai"]
		if !exists {
			t.Fatal("expected openai in raw providers")
		}
		if openai.Resilience == nil || openai.Resilience.Retry == nil || *openai.Resilience.Retry.MaxRetries != 10 {
			t.Error("expected openai raw provider to have MaxRetries override of 10")
		}

		_, exists = result.RawProviders["anthropic"]
		if !exists {
			t.Fatal("expected anthropic in raw providers")
		}
	})
}

func TestLoad_LoggingOnlyModelInteractionsDefault(t *testing.T) {
	clearAllConfigEnvVars(t)

	withTempDir(t, func(_ string) {
		result, err := Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		if !result.Config.Logging.OnlyModelInteractions {
			t.Error("expected OnlyModelInteractions to default to true")
		}
	})
}

func TestLoad_YAMLWithEnvVarExpansion(t *testing.T) {
	clearAllConfigEnvVars(t)

	withTempDir(t, func(dir string) {
		yaml := `
server:
  listen: "${TEST_PORT_CFG:-127.0.0.1:9999}"
providers:
  openai:
    type: "openai"
    api_key: "${TEST_KEY_CFG:-default-key}"
`
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		result, err := Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		if result.Config.Server.Listen != "127.0.0.1:9999" {
			t.Errorf("expected listen 127.0.0.1:9999 (YAML default), got %s", result.Config.Server.Listen)
		}
		provider, exists := result.RawProviders["openai"]
		if !exists {
			t.Fatal("expected openai in raw providers")
		}
		if provider.APIKey != "default-key" {
			t.Errorf("expected API key 'default-key', got %s", provider.APIKey)
		}
	})
}

func TestLoad_YAMLWithEnvVarOverride(t *testing.T) {
	clearAllConfigEnvVars(t)

	withTempDir(t, func(dir string) {
		yaml := `
server:
  listen: "${TEST_PORT_CFG:-127.0.0.1:9999}"
providers:
  openai:
    type: "openai"
    api_key: "${TEST_KEY_CFG:-default-key}"
`
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		t.Setenv("TEST_PORT_CFG", "127.0.0.1:1111")
		t.Setenv("TEST_KEY_CFG", "real-key")

		result, err := Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		if result.Config.Server.Listen != "127.0.0.1:1111" {
			t.Errorf("expected listen 127.0.0.1:1111 (env override), got %s", result.Config.Server.Listen)
		}
		provider, exists := result.RawProviders["openai"]
		if !exists {
			t.Fatal("expected openai in raw providers")
		}
		if provider.APIKey != "real-key" {
			t.Errorf("expected API key 'real-key', got %s", provider.APIKey)
		}
	})
}

func TestLoad_YAMLInConfigSubdir(t *testing.T) {
	clearAllConfigEnvVars(t)

	withTempDir(t, func(dir string) {
		configDir := filepath.Join(dir, "config")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatalf("Failed to create config dir: %v", err)
		}

		yaml := `
server:
  listen: "127.0.0.1:4444"
`
		if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(yaml), 0644); err != nil {
			t.Fatalf("Failed to write config/config.yaml: %v", err)
		}

		result, err := Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		if result.Config.Server.Listen != "127.0.0.1:4444" {
			t.Errorf("expected listen 127.0.0.1:4444 from config/config.yaml, got %s", result.Config.Server.Listen)
		}
	})
}

func TestValidateBodySizeLimit(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"empty string is valid", "", false},
		{"plain number", "1048576", false},
		{"kilobytes lowercase", "100k", false},
		{"kilobytes uppercase", "100K", false},
		{"kilobytes with B suffix", "100KB", false},
		{"megabytes lowercase", "10m", false},
		{"megabytes uppercase", "10M", false},
		{"megabytes with B suffix", "10MB", false},
		{"whitespace trimmed", "  10M  ", false},
		{"minimum valid (1KB)", "1K", false},
		{"maximum valid (100MB)", "100M", false},
		{"invalid format with letters", "abc", true},
		{"invalid unit", "10X", true},
		{"negative number", "-10M", true},
		{"decimal number", "10.5M", true},
		{"empty unit with B", "10B", true},
		{"below minimum (100 bytes)", "100", true},
		{"above maximum (200MB)", "200M", true},
		{"above maximum (1GB)", "1G", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBodySizeLimit(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error for input %q, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for input %q: %v", tt.input, err)
				}
			}
		})
	}
}

func TestLoad_ResponseSimpleOptInViaEnvWithoutYAML(t *testing.T) {
	clearAllConfigEnvVars(t)

	withTempDir(t, func(_ string) {
		t.Setenv("RESPONSE_CACHE_SIMPLE_ENABLED", "true")

		result, err := Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}
		cfg := result.Config
		if cfg.Cache.Response.Simple == nil {
			t.Fatal("expected simple from env opt-in")
		}
	})
}
func TestParseBodySizeLimitBytes(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    int64
		expectError bool
	}{
		{"empty string", "", 0, false},
		{"plain number", "1048576", 1048576, false},
		{"kilobytes", "2K", 2 * 1024, false},
		{"megabytes", "10MB", 10 * 1024 * 1024, false},
		{"whitespace trimmed", " 1M ", 1024 * 1024, false},
		{"invalid format", "10B", 0, true},
		{"below minimum", "100", 0, true},
		{"above maximum", "1G", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBodySizeLimitBytes(tt.input)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error for input %q, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tt.input, err)
			}
			if got != tt.expected {
				t.Fatalf("ParseBodySizeLimitBytes(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}
