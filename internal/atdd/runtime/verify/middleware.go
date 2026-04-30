// Package verify implements pre/post-condition decorators that wrap every
// NodeFn in the engine. Verifications guard against authoring bugs in the
// YAML / agent dispatch loop — for example, "before AT_RED_DSL_WRITE the
// HEAD message must match `^AT - RED - TEST - COMMIT`" — by failing the run
// fast with a clear error.
//
// Verifications are middleware decorators rather than separate nodes
// because they're cross-cutting: every user_task wants the "HEAD matches
// expected commit-message prefix" check, but we don't want to spam the YAML
// with an extra node before each one.
//
// v1 ships the decorator scaffolding with a single no-op verification so
// the wiring is exercised end to end. Real pre/post-condition checks are
// added in Session 2 alongside the action implementations.
package verify

import (
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// PreFn runs before a NodeFn fires. Returning a non-nil error halts the
// run with that error.
type PreFn func(nodeID string, ctx *statemachine.Context) error

// PostFn runs after a NodeFn fires successfully. Returning a non-nil error
// halts the run with that error.
type PostFn func(nodeID string, ctx *statemachine.Context, out statemachine.Outcome) error

// Wrap returns a NodeFn that calls pre, then orig, then post. Any nil hook
// is skipped, so callers can pass only the side they care about.
//
// Errors short-circuit the chain: a failing pre prevents orig from running,
// and a failing orig prevents post from running. Post is not called when
// orig returns an Outcome with a non-nil Err.
func Wrap(orig statemachine.NodeFn, nodeID string, pre PreFn, post PostFn) statemachine.NodeFn {
	return func(ctx *statemachine.Context) statemachine.Outcome {
		if pre != nil {
			if err := pre(nodeID, ctx); err != nil {
				return statemachine.Outcome{Err: err}
			}
		}
		out := orig(ctx)
		if out.Err != nil {
			return out
		}
		if post != nil {
			if err := post(nodeID, ctx, out); err != nil {
				return statemachine.Outcome{Err: err}
			}
		}
		return out
	}
}
