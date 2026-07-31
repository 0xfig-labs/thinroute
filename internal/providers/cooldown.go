package providers

import (
	"sync"
	"time"
)

// CooldownKey identifies what scope a cooldown applies to.
type CooldownKey struct {
	ProviderName string
	KeyIndex     int
	Model        string
	Scope        string // "key" | "model"
}

// CooldownStatus describes the current cooldown state for a key.
type CooldownStatus struct {
	CoolingUntil time.Time
	Active       bool
	Terminal     bool // true when the key is permanently banned/expired
}

// cooldownEntry tracks failures and backoff state for a key.
type cooldownEntry struct {
	until       time.Time
	failures    int
	lastFailure time.Time
	terminal    bool // banned/expired — never retry
}

// CooldownTracker manages in-memory cooldown state with exponential backoff,
// anti-thundering-herd protection, and terminal-state tracking.
// Cooldowns are NOT persisted across restarts.
type CooldownTracker struct {
	mu      sync.Mutex
	entries map[CooldownKey]*cooldownEntry

	// Configurable parameters.
	baseCooldown  time.Duration // initial cooldown on first failure (default 5s)
	maxCooldown   time.Duration // maximum cooldown cap (default 5m)
	backoffFactor float64       // multiply cooldown by this on each failure (default 2.0)
	// antiHerdWindow prevents concurrent failures from extending cooldown.
	// Failures within this window of the last failure don't increment the counter.
	antiHerdWindow time.Duration // default 5s
}

// CooldownConfig tunes the tracker's backoff behaviour.
type CooldownConfig struct {
	BaseCooldown   time.Duration
	MaxCooldown    time.Duration
	BackoffFactor  float64
	AntiHerdWindow time.Duration
}

// DefaultCooldownConfig returns sensible defaults for the tracker.
func DefaultCooldownConfig() CooldownConfig {
	return CooldownConfig{
		BaseCooldown:   5 * time.Second,
		MaxCooldown:    5 * time.Minute,
		BackoffFactor:  2.0,
		AntiHerdWindow: 5 * time.Second,
	}
}

// NewCooldownTracker creates an empty cooldown tracker with defaults.
func NewCooldownTracker() *CooldownTracker {
	return NewCooldownTrackerWithConfig(DefaultCooldownConfig())
}

// NewCooldownTrackerWithConfig creates a tracker with custom backoff config.
func NewCooldownTrackerWithConfig(cfg CooldownConfig) *CooldownTracker {
	return &CooldownTracker{
		entries:        make(map[CooldownKey]*cooldownEntry),
		baseCooldown:   cfg.BaseCooldown,
		maxCooldown:    cfg.MaxCooldown,
		backoffFactor:  cfg.BackoffFactor,
		antiHerdWindow: cfg.AntiHerdWindow,
	}
}

// Check returns the cooldown status for the given key.
func (ct *CooldownTracker) Check(key CooldownKey) CooldownStatus {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	e, ok := ct.entries[key]
	if !ok {
		return CooldownStatus{Active: false}
	}
	if e.terminal {
		return CooldownStatus{CoolingUntil: e.until, Active: true, Terminal: true}
	}
	if time.Now().Before(e.until) {
		return CooldownStatus{CoolingUntil: e.until, Active: true}
	}
	// Expired — clean up
	delete(ct.entries, key)
	return CooldownStatus{Active: false}
}

// RecordFailure marks a key as cooling down. The duration is computed with
// exponential backoff: baseCooldown * backoffFactor^failures, capped at maxCooldown.
// Anti-thundering-herd: failures within antiHerdWindow of the last failure
// don't increment the counter and don't extend the cooldown.
func (ct *CooldownTracker) RecordFailure(key CooldownKey, duration time.Duration) CooldownStatus {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	now := time.Now()
	e, ok := ct.entries[key]
	if !ok {
		e = &cooldownEntry{}
		ct.entries[key] = e
	}

	// Terminal state — never overwrite.
	if e.terminal {
		return CooldownStatus{CoolingUntil: e.until, Active: true, Terminal: true}
	}

	// Anti-thundering-herd: if we failed within the herd window, don't escalate.
	if now.Sub(e.lastFailure) < ct.antiHerdWindow {
		// Still refresh the cooldown expiry so it doesn't expire mid-storm.
		if now.After(e.until) {
			e.until = now.Add(duration)
		}
		return CooldownStatus{CoolingUntil: e.until, Active: true}
	}

	e.failures++
	e.lastFailure = now

	// Exponential backoff.
	computed := float64(duration)
	if ct.backoffFactor > 0 && e.failures > 1 {
		for i := 1; i < e.failures; i++ {
			computed *= ct.backoffFactor
		}
		if computed > float64(ct.maxCooldown) {
			computed = float64(ct.maxCooldown)
		}
	}

	e.until = now.Add(time.Duration(computed))
	return CooldownStatus{CoolingUntil: e.until, Active: true}
}

// MarkTerminal marks a key as permanently disabled (banned, expired, credits_exhausted).
// Terminal states are never automatically cleared.
func (ct *CooldownTracker) MarkTerminal(key CooldownKey) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.entries[key] = &cooldownEntry{
		until:    time.Now().Add(365 * 24 * time.Hour), // effectively forever
		terminal: true,
	}
}

// Reset removes any cooldown for the given key.
func (ct *CooldownTracker) Reset(key CooldownKey) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	delete(ct.entries, key)
}

// ActiveCount returns the number of keys currently in cooldown.
func (ct *CooldownTracker) ActiveCount() int {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	count := 0
	now := time.Now()
	for key, e := range ct.entries {
		if e.terminal || now.Before(e.until) {
			count++
		} else {
			delete(ct.entries, key) // cleanup expired
		}
	}
	return count
}

// RecordFailureFromStatusCode records a cooldown based on an HTTP status code.
//
// Status code → behaviour:
//
//	401/403 → key-level cooldown with exponential backoff (5s base)
//	429     → key-level cooldown based on Retry-After (default 5s)
//	404     → model-level cooldown (30s) — the model doesn't exist on this provider
//
// Terminal states (banned, expired, credits_exhausted) detected in the error
// message are marked permanent.
func RecordFailureFromStatusCode(ct *CooldownTracker, providerName string, keyIndex int, model string, statusCode int, retryAfter time.Duration, errorMessage string) {
	switch {
	case statusCode == 401 || statusCode == 403:
		key := CooldownKey{ProviderName: providerName, KeyIndex: keyIndex, Scope: "key"}
		// Check for terminal states in the error body.
		if isTerminalError(errorMessage) {
			ct.MarkTerminal(key)
			return
		}
		ct.RecordFailure(key, 5*time.Second) // base, backoff will grow it

	case statusCode == 429:
		if retryAfter <= 0 {
			retryAfter = 5 * time.Second
		}
		key := CooldownKey{ProviderName: providerName, KeyIndex: keyIndex, Scope: "key"}
		ct.RecordFailure(key, retryAfter)

	case statusCode == 404 && model != "":
		// Model-level lockout: the specific model doesn't exist on this provider.
		// Other models on the same connection remain usable.
		key := CooldownKey{ProviderName: providerName, KeyIndex: keyIndex, Model: model, Scope: "model"}
		ct.RecordFailure(key, 30*time.Second)
	}
}

// isTerminalError checks if an error message indicates a permanent state.
func isTerminalError(msg string) bool {
	terminals := []string{
		"banned",
		"expired",
		"credits_exhausted",
		"insufficient_quota",
		"account_deactivated",
		"invalid_api_key",
	}
	for _, t := range terminals {
		if len(msg) > 0 && containsAny(msg, t) {
			return true
		}
	}
	return false
}

func containsAny(s, substr string) bool {
	// Case-insensitive substring match.
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			c1 := s[i+j]
			c2 := substr[j]
			if c1 >= 'A' && c1 <= 'Z' {
				c1 += 32
			}
			if c2 >= 'A' && c2 <= 'Z' {
				c2 += 32
			}
			if c1 != c2 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// CooldownSnapshot is a read-only view of an active cooldown entry,
// suitable for control API responses.
type CooldownSnapshot struct {
	ProviderName string    `json:"provider"`
	KeyIndex     int       `json:"key_index,omitempty"`
	Model        string    `json:"model,omitempty"`
	Scope        string    `json:"scope"`
	Active       bool      `json:"active"`
	Terminal     bool      `json:"terminal"`
	CoolingUntil time.Time `json:"cooling_until"`
	Failures     int       `json:"failures"`
}

// Snapshot returns all cooldown entries currently tracked, both active
// and terminal. Expired non-terminal entries are excluded.
func (ct *CooldownTracker) Snapshot() []CooldownSnapshot {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	now := time.Now()
	snapshots := make([]CooldownSnapshot, 0, len(ct.entries))
	for key, entry := range ct.entries {
		// Skip expired non-terminal entries.
		if !entry.terminal && !entry.until.After(now) {
			continue
		}
		snapshots = append(snapshots, CooldownSnapshot{
			ProviderName: key.ProviderName,
			KeyIndex:     key.KeyIndex,
			Model:        key.Model,
			Scope:        key.Scope,
			Active:       entry.terminal || entry.until.After(now),
			Terminal:     entry.terminal,
			CoolingUntil: entry.until,
			Failures:     entry.failures,
		})
	}
	return snapshots
}
