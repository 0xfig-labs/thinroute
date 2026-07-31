package config

import (
	"fmt"
	"net"
	"strings"
)

// ControlConfig configures the local runtime control plane. It is intentionally
// separate from the public inference listener.
type ControlConfig struct {
	Enabled bool   `yaml:"enabled" env:"CONTROL_ENABLED"`
	Listen  string `yaml:"listen"`
}

// Validate rejects accidental exposure of an unauthenticated control plane.
func (c ControlConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(c.Listen))
	if err != nil {
		return fmt.Errorf("control.listen: %w", err)
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("control.listen must be a loopback address")
	}
	return nil
}
