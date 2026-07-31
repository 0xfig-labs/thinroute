package auditlog

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Logger provides async buffered logging with batch writes.
// It collects log entries in a channel and flushes them to storage
// either when the buffer is full or at regular intervals.
type Logger struct {
	config        Config
	buffer        chan *LogEntry
	done          chan struct{}
	wg            sync.WaitGroup
	writes        sync.WaitGroup
	flushInterval time.Duration
	closed        atomic.Bool
}

// NewLogger creates a new async buffered Logger.
// The logger starts a background goroutine for flushing entries.
func NewLogger(cfg Config) *Logger {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 1000
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 5 * time.Second
	}

	l := &Logger{
		config:        cfg,
		buffer:        make(chan *LogEntry, cfg.BufferSize),
		done:          make(chan struct{}),
		flushInterval: cfg.FlushInterval,
	}

	l.wg.Add(1)
	go l.flushLoop()

	return l
}

// Write queues a log entry for async writing.
// This method is non-blocking. If the buffer is full or the logger is closed,
// the entry is dropped and a warning is logged.
func (l *Logger) Write(entry *LogEntry) {
	if entry == nil {
		return
	}

	// Check if logger is shut down to avoid sending on closed channel
	if l.closed.Load() {
		return
	}

	// Track this write to prevent Close from closing buffer while we're sending
	l.writes.Add(1)
	defer l.writes.Done()

	// Double-check after registering - Close() may have set closed between first check and Add(1)
	if l.closed.Load() {
		return
	}

	select {
	case l.buffer <- entry:
	default:
		slog.Warn("audit log buffer full, dropping entry",
			"request_id", entry.RequestID,
			"requested_model", entry.RequestedModel,
		)
	}
}

// Config returns the logger configuration
func (l *Logger) Config() Config {
	return l.config
}

// Close stops the logger and flushes remaining entries.
// This should be called during graceful shutdown.
// Close is idempotent - calling it multiple times is safe.
func (l *Logger) Close() error {
	// Make Close idempotent - if already closed, return immediately
	if l.closed.Swap(true) {
		return nil
	}

	// Wait for any in-flight Write calls to complete
	l.writes.Wait()

	// Signal the flush loop to stop
	close(l.done)

	// Wait for the flush loop to finish
	l.wg.Wait()

	return nil
}

// flushLoop runs in the background and periodically flushes the buffer.
func (l *Logger) flushLoop() {
	defer l.wg.Done()

	ticker := time.NewTicker(l.flushInterval)
	defer ticker.Stop()

	batch := make([]*LogEntry, 0, BatchFlushThreshold)

	for {
		select {
		case entry := <-l.buffer:
			batch = append(batch, entry)
			// Flush when batch reaches threshold
			if len(batch) >= BatchFlushThreshold {
				l.flushBatch(batch)
				batch = make([]*LogEntry, 0, BatchFlushThreshold)
			}

		case <-ticker.C:
			// Periodic flush
			if len(batch) > 0 {
				l.flushBatch(batch)
				batch = make([]*LogEntry, 0, BatchFlushThreshold)
			}

		case <-l.done:
			// Shutdown: drain remaining entries from buffer using non-blocking loop.
			// Note: l.closed is already set by Close() before sending on l.done.
			// We do NOT close(l.buffer) — closing is unnecessary since flushLoop
			// exits via l.done, and closing creates a race with concurrent Write() calls.
			for {
				select {
				case entry := <-l.buffer:
					batch = append(batch, entry)
				default:
					goto drainComplete
				}
			}
		drainComplete:
			// Final flush
			if len(batch) > 0 {
				l.flushBatch(batch)
			}
			return
		}
	}
}

// flushBatch discards the batch (no persistent storage).
func (l *Logger) flushBatch(batch []*LogEntry) {
	// No-op: audit log is in-memory only; usage package handles persistence.
}

// NoopLogger is a logger that does nothing (used when logging is disabled)
type NoopLogger struct{}

// Write does nothing
func (l *NoopLogger) Write(_ *LogEntry) {}

// Config returns an empty config
func (l *NoopLogger) Config() Config {
	return Config{Enabled: false}
}

// Close does nothing
func (l *NoopLogger) Close() error {
	return nil
}

// LoggerInterface defines the interface for loggers (both real and noop)
type LoggerInterface interface {
	Write(entry *LogEntry)
	Config() Config
	Close() error
}
