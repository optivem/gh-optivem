// Package override implements the per-step override-hook decorator.
//
// Every NodeFn the engine dispatches is wrapped through this package. The
// decorator sits at the outermost layer (after the agent dispatcher and
// verify decorators are applied), so it sees the live Context first and
// can publish per-node hints that the inner agent dispatcher then reads.
//
// Override semantics:
//   - Extra: appended to the agent prompt at dispatch time.
//   - Replace: swaps the dispatcher's prompt entirely.
//   - Interactive: prompts the operator for extra instructions before
//     each user_task dispatch (handled by the agent dispatcher itself —
//     this layer only flips the flag in Context).
//
// The hints are exposed through the Context state map under reserved
// keys (see KeyExtra / KeyReplace / KeyInteractive) so the inner
// dispatcher does not need to import this package or hold a hooks
// pointer. v1 ships these decorators as no-ops; v2 fills in the body
// here without touching the dispatcher API.
package override

import (
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// Reserved Context keys. These are consumed by the agent dispatcher in
// internal/atdd/runtime/driver/driver.go. They are namespaced with a
// leading underscore to make accidental collision with action / gate
// state keys unlikely.
const (
	KeyExtra       = "_override_extra"
	KeyReplace     = "_override_replace"
	KeyInteractive = "_override_interactive"
)

// Hooks holds the override configuration loaded from CLI flags. Empty
// in v1; populated in v2 by the --extra / --replace / --interactive
// flags on `gh optivem atdd implement-ticket` and `manage-project`.
type Hooks struct {
	// Extra: per-node-ID extra prompt text, appended at dispatch.
	Extra map[string]string
	// Replace: per-node-ID full prompt replacement. When set for a node,
	// the dispatcher uses it verbatim instead of the templated prompt.
	Replace map[string]string
	// Interactive: when true, the dispatcher prompts the operator for
	// last-minute additional text before every user_task dispatch.
	Interactive bool
}

// Wrap decorates a NodeFn with override-hook handling. For every node
// dispatched, Wrap publishes the per-node Extra / Replace strings (if
// any) and the global Interactive flag into the Context state map under
// KeyExtra / KeyReplace / KeyInteractive, then calls orig. Inner
// dispatchers (the agent dispatcher in particular) read these hints and
// adjust prompt construction accordingly.
//
// The hints are republished on every node dispatch — including nodes
// without an entry in Extra / Replace — so a previous user_task's hints
// do not leak into a later, unrelated dispatch.
func Wrap(orig statemachine.NodeFn, nodeID string, hooks *Hooks) statemachine.NodeFn {
	return func(ctx *statemachine.Context) statemachine.Outcome {
		if hooks == nil {
			ctx.Set(KeyExtra, "")
			ctx.Set(KeyReplace, "")
			ctx.Set(KeyInteractive, false)
			return orig(ctx)
		}
		ctx.Set(KeyExtra, hooks.Extra[nodeID])
		ctx.Set(KeyReplace, hooks.Replace[nodeID])
		ctx.Set(KeyInteractive, hooks.Interactive)
		return orig(ctx)
	}
}
