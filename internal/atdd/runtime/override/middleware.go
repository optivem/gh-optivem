// Package override implements the per-step override-hook decorator.
//
// Every NodeFn the engine dispatches is wrapped through this package, even
// in v1 where no overrides are exposed yet on the CLI. The wrap is in place
// from day one so v2 only has to add `--extra <NODE>="..."`,
// `--replace <NODE>="..."`, and `--interactive` flags to the
// `implement-ticket` subcommand — no engine surgery required.
//
// Override semantics (planned):
//   - Extra: appended to the agent prompt at dispatch time.
//   - Replace: swaps the dispatcher's prompt entirely.
//   - Interactive: prompts the user for extra instructions before dispatch.
//   - Mechanical: replaces the action's Go function with a shell snippet
//     (escape hatch for local experimentation; discouraged).
//
// In v1, the wrap is a no-op pass-through.
package override

import (
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// Hooks holds the override configuration loaded from CLI flags. Empty in
// v1; populated in v2.
type Hooks struct {
	// Extra: per-node-ID extra prompt text, appended at dispatch.
	Extra map[string]string
	// Replace: per-node-ID full prompt replacement.
	Replace map[string]string
	// Interactive: when true, prompt for extra instructions before every
	// user_task dispatch.
	Interactive bool
}

// Wrap decorates a NodeFn with override-hook handling. v1 is a pass-through:
// the function returned is identical in behaviour to orig, but the wrap is
// in place so v2 just has to fill in the body.
func Wrap(orig statemachine.NodeFn, nodeID string, hooks *Hooks) statemachine.NodeFn {
	return func(ctx *statemachine.Context) statemachine.Outcome {
		// v2: read hooks.Extra[nodeID] / Replace[nodeID] / Interactive,
		// adjust the dispatch context (prompt text) accordingly, then call
		// orig(ctx).
		_ = hooks
		_ = nodeID
		return orig(ctx)
	}
}
