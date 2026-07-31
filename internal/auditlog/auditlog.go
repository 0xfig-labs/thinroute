// Package auditlog provides audit logging for the AI gateway.
package auditlog

import (
	"strings"
	"time"

	"github.com/0xfig-labs/thinroute/internal/core"
)

const (
	CacheTypeExact = "exact"
)

const (
	AttemptKindPrimary  = "primary"
	AttemptKindFailover = "failover"
	AttemptKindRetry    = "retry"
)

// LogEntry represents a single audit log entry.
type LogEntry struct {
	// ID is a unique identifier for this log entry (UUID)
	ID string `json:"id" bson:"_id"`

	// Timestamp is when the request started
	Timestamp time.Time `json:"timestamp" bson:"timestamp"`

	// DurationNs is the request duration in nanoseconds
	DurationNs int64 `json:"duration_ns" bson:"duration_ns"`

	// Core fields
	RequestedModel    string `json:"requested_model" bson:"requested_model,omitempty"`
	ResolvedModel     string `json:"resolved_model,omitempty" bson:"resolved_model,omitempty"`
	Provider          string `json:"provider" bson:"provider"`
	ProviderName      string `json:"provider_name,omitempty" bson:"provider_name,omitempty"`
	AliasUsed         bool   `json:"alias_used,omitempty" bson:"alias_used,omitempty"`
	WorkflowVersionID string `json:"workflow_version_id,omitempty" bson:"workflow_version_id,omitempty"`
	CacheType         string `json:"cache_type,omitempty" bson:"cache_type,omitempty"`
	StatusCode        int    `json:"status_code" bson:"status_code"`

	// Request metadata
	RequestID string `json:"request_id,omitempty" bson:"request_id,omitempty"`
	ClientIP  string `json:"client_ip,omitempty" bson:"client_ip,omitempty"`
	Method    string `json:"method,omitempty" bson:"method,omitempty"`
	Path      string `json:"path,omitempty" bson:"path,omitempty"`
	Stream    bool   `json:"stream,omitempty" bson:"stream,omitempty"`
	ErrorType string `json:"error_type,omitempty" bson:"error_type,omitempty"`
	// ponytail: retained for server compatibility.
	UserPath string `json:"user_path,omitempty" bson:"user_path,omitempty"`

	// Data contains flexible request/response information as JSON
	Data *LogData `json:"data,omitempty" bson:"data,omitempty"`
}

// LogData contains lightweight request/response metadata.
type LogData struct {
	UserAgent   string   `json:"user_agent,omitempty" bson:"user_agent,omitempty"`
	Temperature *float64 `json:"temperature,omitempty" bson:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty" bson:"max_tokens,omitempty"`

	// Error details
	ErrorMessage string `json:"error_message,omitempty" bson:"error_message,omitempty"`
	ErrorCode    string `json:"error_code,omitempty" bson:"error_code,omitempty"`

	// WorkflowFeatures captures the request-time effective workflow features
	WorkflowFeatures *WorkflowFeaturesSnapshot `json:"workflow_features,omitempty" bson:"workflow_features,omitempty"`

	// Failover captures runtime redirect details
	Failover *FailoverSnapshot `json:"failover,omitempty" bson:"failover,omitempty"`

	// Attempts captures provider calls made for this logical request
	Attempts []AttemptSnapshot `json:"attempts,omitempty" bson:"attempts,omitempty"`
	// ponytail: retained for server compatibility.
	Labels []string `json:"labels,omitempty" bson:"labels,omitempty"`
}

// WorkflowFeaturesSnapshot stores the effective workflow feature state.
type WorkflowFeaturesSnapshot struct {
	Cache      bool `json:"cache" bson:"cache"`
	Audit      bool `json:"audit" bson:"audit"`
	Usage      bool `json:"usage" bson:"usage"`
	Budget     bool `json:"budget" bson:"budget"`
	Guardrails bool `json:"guardrails" bson:"guardrails"`
	Failover   bool `json:"failover" bson:"failover"`
}

// FailoverSnapshot stores the runtime failover selection for one request.
type FailoverSnapshot struct {
	TargetModel string `json:"target_model,omitempty" bson:"target_model,omitempty"`
}

// AttemptSnapshot stores a provider attempt with structured error info.
type AttemptSnapshot struct {
	Seq          int       `json:"seq" bson:"seq"`
	Kind         string    `json:"kind" bson:"kind"`
	ProviderType string    `json:"provider_type,omitempty" bson:"provider_type,omitempty"`
	ProviderName string    `json:"provider_name,omitempty" bson:"provider_name,omitempty"`
	Model        string    `json:"model,omitempty" bson:"model,omitempty"`
	StatusCode   int       `json:"status_code,omitempty" bson:"status_code,omitempty"`
	Success      bool      `json:"success" bson:"success"`
	ErrorType    string    `json:"error_type,omitempty" bson:"error_type,omitempty"`
	ErrorCode    string    `json:"error_code,omitempty" bson:"error_code,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty" bson:"error_message,omitempty"`
	StartedAt    time.Time `json:"started_at,omitempty" bson:"started_at,omitempty"`
	DurationNs   int64     `json:"duration_ns,omitempty" bson:"duration_ns,omitempty"`
}

// isCredentialHeader reports whether a header holds credentials.
// ponytail: re-export from core to avoid multi-package dependency for this one helper.
var isCredentialHeader = core.IsCredentialHeader

// RedactHeaders redacts credential headers. The original map is not modified.
func RedactHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	result := make(map[string]string, len(headers))
	for key, value := range headers {
		if isCredentialHeader(key) {
			result[key] = "[REDACTED]"
		} else {
			result[key] = value
		}
	}
	return result
}

// normalizeAttemptSnapshots normalizes and filters attempt snapshots.
func normalizeAttemptSnapshots(attempts []AttemptSnapshot) []AttemptSnapshot {
	if len(attempts) == 0 {
		return nil
	}
	normalized := make([]AttemptSnapshot, 0, len(attempts))
	for _, attempt := range attempts {
		attempt.Kind = normalizeAttemptKind(attempt.Kind)
		if attempt.Kind == "" {
			continue
		}
		if attempt.Seq <= 0 {
			attempt.Seq = len(normalized) + 1
		}
		attempt.ProviderType = strings.TrimSpace(attempt.ProviderType)
		attempt.ProviderName = strings.TrimSpace(attempt.ProviderName)
		attempt.Model = strings.TrimSpace(attempt.Model)
		attempt.ErrorType = strings.TrimSpace(attempt.ErrorType)
		attempt.ErrorCode = strings.TrimSpace(attempt.ErrorCode)
		attempt.ErrorMessage = strings.TrimSpace(attempt.ErrorMessage)
		normalized = append(normalized, attempt)
	}
	return normalized
}

func normalizeAttemptKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case AttemptKindPrimary:
		return AttemptKindPrimary
	case AttemptKindFailover:
		return AttemptKindFailover
	case AttemptKindRetry:
		return AttemptKindRetry
	default:
		return ""
	}
}

// Config holds audit logging configuration.
type Config struct {
	Enabled               bool
	BufferSize            int
	FlushInterval         time.Duration
	RetentionDays         int
	OnlyModelInteractions bool
}
