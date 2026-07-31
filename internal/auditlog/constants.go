package auditlog

// Buffer limit for audit logging.
const (
	// BatchFlushThreshold is the number of entries that triggers an immediate flush.
	// When the batch reaches this size, it's written to storage without waiting for the timer.
	BatchFlushThreshold = 100

	// MaxBodyCapture is the maximum size of request bodies to buffer for request snapshot (1MB).
	MaxBodyCapture = 1024 * 1024
)

// Context keys for storing audit log data in request context.
type contextKey string

const (
	// LogEntryKey is the context key for storing the log entry.
	LogEntryKey contextKey = "auditlog_entry"

	// LogEntryStreamingKey is the context key for marking a request as streaming.
	LogEntryStreamingKey contextKey = "auditlog_entry_streaming"
)
