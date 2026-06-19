package projectconfig

import (
	"reflect"
	"testing"
)

// TestSystemSurfacePaths pins the single architecture+channel→surface mapping
// every consumer shares (the runtime scope check, preflight, and the driver
// prompt). The unknown-channel, microservices, unset, and nil-receiver rows
// assert the fail-fast contract: ok=false so no caller scopes or renders an
// empty surface.
func TestSystemSurfacePaths(t *testing.T) {
	monolith := &Config{
		System: System{Architecture: ArchMonolith, Path: "system/monolith"},
	}
	multitier := &Config{
		System: System{
			Architecture: ArchMultitier,
			Backend:      TierSpec{Path: "system/multitier/backend-java"},
			Frontend:     TierSpec{Path: "system/multitier/frontend-typescript"},
		},
	}
	microservices := &Config{System: System{Architecture: ArchMicroservices}}
	unset := &Config{}

	tests := []struct {
		name    string
		cfg     *Config
		channel string
		want    []string
		wantOK  bool
	}{
		{name: "monolith api → system path", cfg: monolith, channel: "api", want: []string{"system/monolith"}, wantOK: true},
		{name: "monolith ui → system path", cfg: monolith, channel: "ui", want: []string{"system/monolith"}, wantOK: true},
		{name: "monolith no channel → system path", cfg: monolith, channel: "", want: []string{"system/monolith"}, wantOK: true},
		{name: "multitier api → backend", cfg: multitier, channel: "api", want: []string{"system/multitier/backend-java"}, wantOK: true},
		{name: "multitier ui → frontend", cfg: multitier, channel: "ui", want: []string{"system/multitier/frontend-typescript"}, wantOK: true},
		{name: "multitier whole-system → both tiers", cfg: multitier, channel: "", want: []string{"system/multitier/backend-java", "system/multitier/frontend-typescript"}, wantOK: true},
		{name: "multitier unknown channel → fail-fast", cfg: multitier, channel: "mobile", want: nil, wantOK: false},
		{name: "microservices → fail-fast (out of scope)", cfg: microservices, channel: "api", want: nil, wantOK: false},
		{name: "unset architecture → fail-fast", cfg: unset, channel: "api", want: nil, wantOK: false},
		{name: "nil receiver → fail-fast", cfg: nil, channel: "api", want: nil, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.cfg.SystemSurfacePaths(tt.channel)
			if ok != tt.wantOK || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SystemSurfacePaths(%q) = (%v, %v); want (%v, %v)",
					tt.channel, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
