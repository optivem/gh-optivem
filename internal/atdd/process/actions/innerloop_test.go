package actions

import (
	"testing"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

func innerLoopMultitier(backend projectconfig.InnerLoop) *projectconfig.Config {
	return &projectconfig.Config{
		System: projectconfig.System{
			Architecture: projectconfig.ArchMultitier,
			Backend:      projectconfig.TierSpec{InnerLoop: backend},
		},
	}
}

func TestResolveInnerLoop(t *testing.T) {
	cases := []struct {
		name          string
		cfg           *projectconfig.Config
		channel       string
		wantMode      string
		wantComponent string
	}{
		{"api channel with test-after backend", innerLoopMultitier(projectconfig.InnerLoop{Mode: projectconfig.InnerLoopTestAfter}), "api", "test-after", "backend"},
		{"ui channel, frontend not opted in", innerLoopMultitier(projectconfig.InnerLoop{Mode: projectconfig.InnerLoopTestAfter}), "ui", "none", "frontend"},
		{"whole-system dispatch has no channel", innerLoopMultitier(projectconfig.InnerLoop{Mode: projectconfig.InnerLoopTestAfter}), "", "none", ""},
		{"absent block is none", innerLoopMultitier(projectconfig.InnerLoop{}), "api", "none", "backend"},
		{"nil config is none", nil, "api", "none", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newActions(Deps{Config: tc.cfg})
			ctx := statemachine.NewContext()
			ctx.Params = map[string]string{"channel": tc.channel}
			if out := a.resolveInnerLoop(ctx); out.Err != nil {
				t.Fatalf("resolve-inner-loop: %v", out.Err)
			}
			if got := ctx.State[CtxKeyInnerLoopMode]; got != tc.wantMode {
				t.Errorf("mode = %v, want %q", got, tc.wantMode)
			}
			if got := ctx.State[CtxKeyInnerLoopComponent]; got != tc.wantComponent {
				t.Errorf("component = %v, want %q", got, tc.wantComponent)
			}
		})
	}
}
