package config

import "testing"

func TestControlConfigRejectsNonLoopback(t *testing.T) {
	for _, test := range []struct {
		cfg     ControlConfig
		wantErr bool
	}{
		{cfg: ControlConfig{Enabled: true, Listen: "127.0.0.1:52181"}},
		{cfg: ControlConfig{Enabled: true, Listen: "[::1]:52181"}},
		{cfg: ControlConfig{Enabled: true, Listen: "0.0.0.0:52181"}, wantErr: true},
	} {
		if err := test.cfg.Validate(); (err != nil) != test.wantErr {
			t.Fatalf("Validate(%+v) error = %v, wantErr %v", test.cfg, err, test.wantErr)
		}
	}
}
