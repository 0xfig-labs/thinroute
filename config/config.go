// Package config provides configuration management for the application.
package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/0xfig-labs/thinroute/internal/storage"
)

// Config holds the application configuration.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Models     ModelsConfig     `yaml:"models"`
	Cache      CacheConfig      `yaml:"cache"`
	Storage    StorageConfig    `yaml:"storage"`
	Logging    LogConfig        `yaml:"logging"`
	Usage      UsageConfig      `yaml:"usage"`
	Metrics    MetricsConfig    `yaml:"metrics"`
	HTTP       HTTPConfig       `yaml:"http"`
	Control    ControlConfig    `yaml:"control"`
	Resilience ResilienceConfig `yaml:"resilience"`

	// VirtualModels declares redirects, load balancers, and access policies as
	// infrastructure-as-code. They are the sole virtual-model configuration source.
	VirtualModels []VirtualModelConfig `yaml:"virtual_models"`
}

// LoadResult is returned by Load and bundles the application config with the raw
// provider map parsed from YAML. Provider env vars and resolution are handled by
// the providers package.
type LoadResult struct {
	Config       *Config
	RawProviders map[string]RawProviderConfig
}

// buildDefaultConfig returns the single source of truth for all configuration defaults.
func buildDefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Listen:                  "127.0.0.1:52180",
			BasePath:                "/",
			UserPathHeader:          "X-thinroute-User-Path",
			PprofEnabled:            false,
			EnablePassthroughRoutes: true,
			AllowPassthroughV1Alias: true,
			RealtimeEnabled:         true,
			EnabledPassthroughProviders: []string{
				"openai",
				"anthropic",
				"openrouter",
				"zai",
				"vllm",
				"deepseek",
			},
		},
		Models: ModelsConfig{
			EnabledByDefault:                true,
			KeepOnlyAliasesAtModelsEndpoint: false,
			ConfiguredProviderModelsMode:    ConfiguredProviderModelsModeFallback,
		},
		Cache: CacheConfig{
			Model: ModelCacheConfig{
				RefreshInterval: 3600,
				RecheckInterval: 60,
				ModelList: ModelListConfig{
					URL: "",
				},
				Local: nil,
			},
			Response: ResponseCacheConfig{},
		},
		Storage: StorageConfig{
			Type: "sqlite",
			SQLite: SQLiteStorageConfig{
				Path: storage.DefaultSQLitePath,
			},
		},
		Logging: LogConfig{
			BufferSize:            1000,
			FlushInterval:         5,
			RetentionDays:         30,
			OnlyModelInteractions: true,
		},
		Usage: UsageConfig{
			Enabled:                   true,
			EnforceReturningUsageData: true,
			BufferSize:                1000,
			FlushInterval:             5,
			RetentionDays:             90,
		},
		Metrics: MetricsConfig{
			Endpoint: "/metrics",
		},
		HTTP: HTTPConfig{
			Timeout:               600,
			ResponseHeaderTimeout: 600,
		},

		Resilience: ResilienceConfig{
			Retry:          DefaultRetryConfig(),
			CircuitBreaker: DefaultCircuitBreakerConfig(),
		},
		Control: ControlConfig{
			Enabled: true,
			Listen:  "127.0.0.1:52181",
		},
	}
}

// Load reads configuration from file and environment.
//
// Priority: explicit configPath > THINROUTE_CONFIG env var > config.yaml > defaults.
// When configPath is empty, THINROUTE_CONFIG is checked; when both are empty,
// config.yaml in the working directory is tried. If no file is found, defaults
// and environment variables are used.
//
// The returned LoadResult contains the resolved application Config and the raw
// provider map parsed from YAML. Provider env var discovery, credential filtering,
// and resilience merging are handled by the providers package.
func Load(configPath ...string) (*LoadResult, error) {
	cfg := buildDefaultConfig()

	strict, err := resolveConfigStrict()
	if err != nil {
		return nil, err
	}

	rawProviders, err := applyYAML(cfg, resolveConfigPath(configPath...), strict)
	if err != nil {
		return nil, err
	}

	if err := applyResponseSimpleEnv(&cfg.Cache.Response); err != nil {
		return nil, err
	}
	if err := cfg.Control.Validate(); err != nil {
		return nil, err
	}
	if err := applyVirtualModelsEnv(cfg, strict); err != nil {
		return nil, err
	}
	cfg.Server.BasePath = NormalizeBasePath(cfg.Server.BasePath)
	cfg.Server.UserPathHeader, err = NormalizeHeaderName(cfg.Server.UserPathHeader, "X-thinroute-User-Path")
	if err != nil {
		return nil, fmt.Errorf("invalid server.user_path_header: %w", err)
	}
	cfg.Models.ConfiguredProviderModelsMode = ResolveConfiguredProviderModelsMode(cfg.Models.ConfiguredProviderModelsMode)
	if !cfg.Models.ConfiguredProviderModelsMode.Valid() {
		return nil, fmt.Errorf("models.configured_provider_models_mode must be one of: fallback, allowlist")
	}

	// When no model cache backend was specified, default to local.
	if cfg.Cache.Model.Local == nil {
		cfg.Cache.Model.Local = &LocalCacheConfig{}
	}

	if cfg.Server.BodySizeLimit != "" {
		if err := ValidateBodySizeLimit(cfg.Server.BodySizeLimit); err != nil {
			return nil, fmt.Errorf("invalid BODY_SIZE_LIMIT: %w", err)
		}
	}

	if err := ValidateCacheConfig(&cfg.Cache); err != nil {
		return nil, err
	}

	return &LoadResult{
		Config:       cfg,
		RawProviders: rawProviders,
	}, nil
}

// resolveConfigPath returns the config file path using the priority:
// explicit arg > THINROUTE_CONFIG env var > config.yaml.
func resolveConfigPath(explicitPath ...string) string {
	if len(explicitPath) > 0 && explicitPath[0] != "" {
		return explicitPath[0]
	}
	if envPath := os.Getenv("THINROUTE_CONFIG"); envPath != "" {
		return envPath
	}
	return ""
}

// configFilePaths are searched in order when no explicit path is given;
// the first readable file wins.
var configFilePaths = []string{
	"config/config.yaml",
	"config.yaml",
}

// envConfigStrict is the env var name for CONFIG_STRICT.
const envConfigStrict = "CONFIG_STRICT"

func resolveConfigStrict() (bool, error) {
	raw := strings.TrimSpace(os.Getenv(envConfigStrict))
	if raw == "" {
		return true, nil
	}
	strict, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid %s: %q is not a boolean", envConfigStrict, raw)
	}
	if !strict {
		slog.Warn("CONFIG_STRICT=false: unknown config keys are ignored with a warning instead of aborting startup")
	}
	return strict, nil
}

// applyYAML reads an optional config file and overlays it onto cfg.
// Returns the raw provider map parsed from the providers: YAML section.
// If no config file is found, this is a no-op (not an error).
//
// When strict, an unknown key is an error rather than a silently ignored one. A
// misindented section — the classic `providers:` followed by entries at column
// zero — otherwise parses as a null section plus unknown top-level keys, and the
// applyYAML reads an optional config file and overlays it onto cfg.
// configPath is the explicit path; when empty, THINROUTE_CONFIG or configFilePaths are tried.
// Returns the raw provider map parsed from the providers: YAML section.
// If no config file is found, this is a no-op (not an error).
//
// When strict, an unknown key is an error rather than a silently ignored one.
func applyYAML(cfg *Config, configPath string, strict bool) (map[string]RawProviderConfig, error) {
	path, data, err := readConfigFile(configPath)
	if err != nil {
		return nil, err
	}
	if data == nil {
		slog.Info("no config file found; using defaults and environment", "searched", configFilePaths)
		return map[string]RawProviderConfig{}, nil
	}
	// yamlTarget is a local struct that mirrors Config for YAML unmarshaling,
	// using RawProviderConfig for providers so nullable resilience overrides are preserved.
	type yamlTarget struct {
		*Config      `yaml:",inline"`
		RawProviders map[string]RawProviderConfig `yaml:"providers"`
	}

	target := yamlTarget{Config: cfg}
	expanded := expandString(string(data))
	if err := validateNoUnexpandedVars(path, expanded); err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(strings.NewReader(expanded))
	// Unknown keys are always detected. Whether they are fatal is decided below,
	// so the lax mode can still name each one instead of dropping it in silence.
	decoder.KnownFields(true)
	// A file holding only comments decodes to nothing; that is an empty overlay,
	// not a failure.
	decodeErr := decoder.Decode(&target)
	if decodeErr != nil && !errors.Is(decodeErr, io.EOF) {
		if err := reportYAMLDecodeError(path, decodeErr, strict); err != nil {
			return nil, err
		}
	}
	if err := ensureSingleDocument(path, decoder); err != nil {
		return nil, err
	}

	slog.Info("config file loaded", "path", path, "providers", len(target.RawProviders))

	if target.RawProviders == nil {
		return map[string]RawProviderConfig{}, nil
	}
	return target.RawProviders, nil
}

// ensureSingleDocument rejects a config file holding more than one YAML document.
// The decoder reads only the first, so everything after a `---` separator would be
// applied nowhere — the same silent loss a misindented section causes. Decoding into
// a yaml.Node accepts any shape, so this detects a second document without
// re-triggering the unknown-key check. A structural fault, fatal regardless of
// CONFIG_STRICT.
func ensureSingleDocument(path string, decoder *yaml.Decoder) error {
	var extra yaml.Node
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return formatYAMLError(path, err)
	}
	return fmt.Errorf("failed to parse %s: only one YAML document is supported, found another after a '---' separator", path)
}

// reportYAMLDecodeError decides the fate of a decode error. Unknown keys are fatal
// when strict and warnings otherwise; every other problem — a malformed value, a
// syntax error — is fatal regardless, because CONFIG_STRICT relaxes what the schema
// accepts, not whether the file makes sense. Returns nil when nothing is fatal.
func reportYAMLDecodeError(path string, err error, strict bool) error {
	var typeErr *yaml.TypeError
	if strict || !errors.As(err, &typeErr) {
		return formatYAMLError(path, err)
	}

	var fatal []string
	for _, message := range typeErr.Errors {
		line, field, ok := parseUnknownFieldMessage(message)
		if !ok {
			fatal = append(fatal, message)
			continue
		}
		slog.Warn("unknown config key ignored; it has no effect",
			"path", path, "line", line, "field", field)
	}
	if len(fatal) > 0 {
		return formatYAMLError(path, &yaml.TypeError{Errors: fatal})
	}
	return nil
}

// unknownFieldMessage matches yaml.v3's unknown-key message, the only decode error
// CONFIG_STRICT=false is allowed to downgrade.
var unknownFieldMessage = regexp.MustCompile(`^line (\d+): field (\S+) not found in type \S+$`)

func parseUnknownFieldMessage(message string) (line int, field string, ok bool) {
	match := unknownFieldMessage.FindStringSubmatch(message)
	if match == nil {
		return 0, "", false
	}
	line, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, "", false
	}
	return line, match[2], true
}

// readConfigFile returns the first config file that exists and its contents, or an
// empty path and nil contents when none does. A file that exists but cannot be read
// is an error, not a missing file.
func readConfigFile(explicitPath string) (string, []byte, error) {
	if explicitPath != "" {
		data, err := os.ReadFile(explicitPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", nil, fmt.Errorf("config file not found: %s", explicitPath)
			}
			return "", nil, fmt.Errorf("failed to read %s: %w", explicitPath, err)
		}
		return explicitPath, data, nil
	}
	for _, path := range configFilePaths {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			return path, data, nil
		case errors.Is(err, fs.ErrNotExist):
			continue
		default:
			return "", nil, fmt.Errorf("failed to read %s: %w", path, err)
		}
	}
	return "", nil, nil
}

// yamlTypeSuffix matches the Go type name yaml.v3 appends to unknown-field errors
// ("field foo not found in type config.yamlTarget"). It names an internal struct
// the operator cannot act on, so it is stripped.
var yamlTypeSuffix = regexp.MustCompile(` in type \S+`)

// formatYAMLError rewrites a yaml.v3 decode error into a single actionable line
// prefixed with the offending file.
func formatYAMLError(path string, err error) error {
	msg := yamlTypeSuffix.ReplaceAllString(err.Error(), "")
	msg = strings.TrimPrefix(msg, "yaml: unmarshal errors:\n")
	msg = strings.TrimPrefix(msg, "yaml: ")
	msg = strings.ReplaceAll(msg, "\n  ", "; ")
	return fmt.Errorf("failed to parse %s: %s", path, strings.TrimSpace(msg))
}

// unexpandedVarPattern matches ${VAR} references that remain after expansion.
var unexpandedVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// validateNoUnexpandedVars checks that no unexpanded ${VAR} references remain
// in the YAML content. Each unresolved variable is reported with its line number.
func validateNoUnexpandedVars(path, content string) error {
	matches := unexpandedVarPattern.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return nil
	}
	var msgs []string
	for _, m := range matches {
		varName := content[m[2]:m[3]]
		line := strings.Count(content[:m[0]], "\n") + 1
		msgs = append(msgs, fmt.Sprintf("line %d: ${%s}", line, varName))
	}
	return fmt.Errorf("unresolved environment variable(s) in %s:\n  %s",
		path, strings.Join(msgs, "\n  "))
}
