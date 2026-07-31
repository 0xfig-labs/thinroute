package config

import (
	"fmt"
	"os"
)

// CacheConfig holds model and response cache configuration.
type CacheConfig struct {
	Model    ModelCacheConfig    `yaml:"model"`
	Response ResponseCacheConfig `yaml:"response"`
}

// ModelCacheConfig holds cache configuration for model registry.
// Uses local file cache.
type ModelCacheConfig struct {
	RefreshInterval int               `yaml:"refresh_interval" env:"CACHE_REFRESH_INTERVAL"`
	RecheckInterval int               `yaml:"recheck_interval" env:"PROVIDER_RECHECK_INTERVAL"`
	ModelList       ModelListConfig   `yaml:"model_list"`
	Local           *LocalCacheConfig `yaml:"local"`
}

// ModelListConfig holds configuration for fetching the external model metadata registry.
type ModelListConfig struct {
	// URL is the HTTP(S) URL to fetch models.json from (empty = disabled)
	URL string `yaml:"url" env:"MODEL_LIST_URL"`
}

// LocalCacheConfig holds local file cache configuration.
type LocalCacheConfig struct {
	CacheDir string `yaml:"cache_dir" env:"THINROUTE_CACHE_DIR"`
}

// ResponseCacheConfig holds configuration for response cache middleware.
type ResponseCacheConfig struct {
	Simple *SimpleCacheConfig `yaml:"simple"`
}

// SimpleCacheConfig holds configuration for exact-match response caching (in-memory).
type SimpleCacheConfig struct {
	Enabled *bool `yaml:"enabled"`
	TTL     int   `yaml:"ttl"` // cache TTL in seconds; 0 = default (1 hour)
}

// ValidateCacheConfig validates the cache configuration in c.
func ValidateCacheConfig(c *CacheConfig) error {
	if c == nil {
		return fmt.Errorf("cache: configuration is required")
	}
	if c.Model.Local == nil {
		return fmt.Errorf("cache.model.local: must be configured")
	}
	return nil
}

// SimpleCacheEnabled reports whether the exact-match response cache layer is
// allowed to run for a non-nil simple config. Omitted enabled means true.
func SimpleCacheEnabled(s *SimpleCacheConfig) bool {
	if s == nil {
		return false
	}
	if s.Enabled != nil && !*s.Enabled {
		return false
	}
	return true
}

func applyResponseSimpleEnv(resp *ResponseCacheConfig) error {
	v, ok := os.LookupEnv("RESPONSE_CACHE_SIMPLE_ENABLED")
	if ok && !parseBool(v) {
		resp.Simple = nil
		return nil
	}
	if resp.Simple == nil {
		if ok && parseBool(v) {
			resp.Simple = &SimpleCacheConfig{}
		} else {
			return nil
		}
	}
	simple := resp.Simple
	if ok {
		b := parseBool(v)
		simple.Enabled = &b
	}
	return nil
}
