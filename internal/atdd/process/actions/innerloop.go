package actions

import (
	"fmt"
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// CtxKeyInnerLoopMode is the string resolveInnerLoop stamps recording the
// inner-loop mode (none | test-after | tdd) of the component this system
// dispatch targets; GATE_INNER_LOOP routes on it.
const CtxKeyInnerLoopMode = "inner-loop-mode"

// CtxKeyInnerLoopComponent is the logical component name (backend | frontend |
// monolith) the inner-loop verify commands run against, as the component-test
// runner names it.
const CtxKeyInnerLoopComponent = "inner-loop-component"

// resolveInnerLoop runs at the start of implement-and-verify-system and stamps
// ctx.State["inner-loop-mode"] from the project config: the baked `channel`
// maps to its tier (api -> backend, ui -> frontend; a monolith uses the
// system-level block). A dispatch with no channel (the whole-system
// redesign / refactor cycles) stamps none: in those cycles the existing tests
// are the only check, so no inner-loop gate applies. Deterministic, no agent.
func (a actions) resolveInnerLoop(ctx *statemachine.Context) statemachine.Outcome {
	mode := projectconfig.InnerLoopNone
	component := ""
	if a.deps.Config != nil {
		il, name := a.deps.Config.InnerLoopForChannel(strings.TrimSpace(ctx.Params["channel"]))
		mode, component = il.EffectiveMode(), name
	}
	if err := projectconfig.ValidateInnerLoopMode(string(mode)); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("resolve-inner-loop: %w", err)}
	}
	ctx.Set(CtxKeyInnerLoopMode, string(mode))
	ctx.Set(CtxKeyInnerLoopComponent, component)
	return statemachine.Outcome{}
}
