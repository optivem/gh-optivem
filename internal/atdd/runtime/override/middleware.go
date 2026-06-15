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
//
// The hints are exposed through the Context state map under reserved
// keys (see KeyExtra / KeyReplace) so the inner dispatcher does not need
// to import this package or hold a hooks pointer. Hooks are populated
// from gh-optivem.yaml's node_extras: / node_replacements: by the cobra
// layer in atdd_commands.go.
package override

import (
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// Reserved Context keys. These are consumed by the agent dispatcher in
// internal/atdd/runtime/driver/driver.go. They are namespaced with a
// leading underscore to make accidental collision with action / gate
// state keys unlikely.
const (
	KeyExtra   = "_override_extra"
	KeyReplace = "_override_replace"
)

// Hooks holds the override configuration loaded from gh-optivem.yaml's
// node_extras: (inline text appended at dispatch) and node_replacements:
// (file bodies that swap the prompt verbatim) fields. nil is the
// "no overrides" case.
type Hooks struct {
	// Extra: per-node-ID extra prompt text, appended at dispatch.
	Extra map[string]string
	// Replace: per-node-ID full prompt replacement. When set for a node,
	// the dispatcher uses it verbatim instead of the templated prompt.
	Replace map[string]string
}

// Wrap decorates a NodeFn with override-hook handling. For every node
// dispatched, Wrap publishes the per-node Extra / Replace strings (if
// any) into the Context state map under KeyExtra / KeyReplace, then
// calls orig. Inner dispatchers (the agent dispatcher in particular)
// read these hints and adjust prompt construction accordingly.
//
// The hints are republished on every node dispatch — including nodes
// without an entry in Extra / Replace — so a previous user-task's hints
// do not leak into a later, unrelated dispatch.
func Wrap(orig statemachine.NodeFn, nodeID string, hooks *Hooks) statemachine.NodeFn {
	return func(ctx *statemachine.Context) statemachine.Outcome {
		if hooks == nil {
			ctx.Set(KeyExtra, "")
			ctx.Set(KeyReplace, "")
			return orig(ctx)
		}
		ctx.Set(KeyExtra, hooks.Extra[nodeID])
		ctx.Set(KeyReplace, hooks.Replace[nodeID])
		return orig(ctx)
	}
}
