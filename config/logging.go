package config

// LogConfig holds audit logging configuration
type LogConfig struct {
	// Enabled controls whether audit logging is active
	// Default: false
	Enabled bool `yaml:"enabled" env:"LOGGING_ENABLED"`


	// BufferSize is the number of log entries to buffer before flushing
	// Default: 1000
	BufferSize int `yaml:"buffer_size" env:"LOGGING_BUFFER_SIZE"`

	// FlushInterval is how often to flush buffered logs (in seconds)
	// Default: 5
	FlushInterval int `yaml:"flush_interval" env:"LOGGING_FLUSH_INTERVAL"`

	// RetentionDays is how long to keep logs (0 = forever)
	// Default: 30
	RetentionDays int `yaml:"retention_days" env:"LOGGING_RETENTION_DAYS"`

	// OnlyModelInteractions limits audit logging to AI model endpoints only
	// When true, only /v1/chat/completions, /v1/responses, /v1/embeddings, /v1/files, and /v1/batches are logged
	// Endpoints like /health, /metrics, /v1/models are skipped
	// Default: true
	OnlyModelInteractions bool `yaml:"only_model_interactions" env:"LOGGING_ONLY_MODEL_INTERACTIONS"`
}
