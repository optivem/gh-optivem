package actions

import (
	"fmt"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// externalSystemNamesFromChangedPaths derives the set of external-system names
// the external-driver-PORT change touched, SOLELY from the preserved port
// changed-paths (interface methods AND DTOs) — never from the driver-adapter
// files an impl step wrote (plan 20260613-1835). The adapter files are
// legitimately empty on a DTO-only port change (#65 view-product-list); the
// port change is the cycle's trigger and is always present (the entry gate
// GATE_EXTERNAL_DRIVER_PORTS_CHANGED is port-keyed).
//
// The external-system-driver-port layer root ends at the `external` segment;
// each system's files live under `<root>/<name>/...`, so <name> is the first
// path segment of every changed path relative to the root. Residual `shared`
// code directly under the root (no `<name>/` segment) is ignored.
//
// Shared by resolve-external-system (per-clone membership guard) and
// validate-external-systems-registered (upfront whole-set registration check),
// the two actions that replaced the retired identify-external-system.
func externalSystemNamesFromChangedPaths(changedPaths, root string) map[string]struct{} {
	names := map[string]struct{}{}
	for _, p := range strings.Split(changedPaths, "\n") {
		p = strings.TrimSpace(p)
		if !strings.HasPrefix(p, root+"/") {
			continue
		}
		rel := strings.TrimPrefix(p, root+"/")
		slash := strings.IndexByte(rel, '/')
		if slash <= 0 {
			continue
		}
		names[rel[:slash]] = struct{}{}
	}
	return names
}

// validateExternalSystemsRegistered is the upfront no-silent-skip guard for the
// unrolled external-system contract cycle (plan 20260615-0755 Item 3). It runs
// ONCE in shared-contract, before the per-system clones, and hard-errors if any
// external system the port change touched is absent from the external-systems
// registry (so it would have no clone and be silently skipped — the #65-class
// bug plan 20260613-1835 closed, now seeing the whole changed-set against the
// whole registry rather than one cycle at a time). The zero-names case is
// covered by the entry gate (GATE_EXTERNAL_DRIVER_PORTS_CHANGED), so a non-empty
// port change is guaranteed whenever this runs.
//
// Source is the preserved ctx.State["external-driver-port-changed-paths"] set,
// stashed by validate-outputs-and-scopes when the AT-cascade DSL phase landed
// external-driver-port-changed=true. Deterministic — no agent.
func (a actions) validateExternalSystemsRegistered(ctx *statemachine.Context) statemachine.Outcome {
	cfg := a.deps.Config
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-external-systems-registered: gh-optivem.yaml not loaded — driver must inject actions.Deps.Config")}
	}
	roots, err := ResolveLayerPaths([]string{"external-system-driver-port"}, cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-external-systems-registered: %w", err)}
	}
	names := externalSystemNamesFromChangedPaths(ctx.GetString("external-driver-port-changed-paths"), roots[0])

	unregistered := map[string]struct{}{}
	for name := range names {
		if _, ok := cfg.ExternalSystems[name]; !ok {
			unregistered[name] = struct{}{}
		}
	}
	if len(unregistered) > 0 {
		return statemachine.Outcome{Err: fmt.Errorf("validate-external-systems-registered: the external-driver-port change touches external system(s) %s not registered in gh-optivem.yaml external-systems: — onboard them (declaring their real-kind) before running the contract-test cycle", strings.Join(sortedSetKeys(unregistered), ", "))}
	}
	return statemachine.Outcome{}
}

// resolveExternalSystem runs at the START of each unrolled external-system
// contract-cycle clone (plan 20260615-0755 Items 2 + 4). Each clone bakes its
// own external-system-name + real-kind into ctx.Params at the call-activity
// entry; this action reads those baked params and:
//
//  1. Self-guards the clone — stamps ctx.State["external-system-touched"]
//     = (this clone's name ∈ the names-set derived from the port change). The
//     downstream GATE_EXTERNAL_SYSTEM_TOUCHED routes an untouched clone past
//     the whole cycle to its skip end-event, so a ticket runs only the cycles
//     for the external systems it actually touched.
//  2. Copies the baked real-kind param into gate-readable state — the
//     param→State shim (plan verify A): baked call-activity params land in
//     ctx.Params, but GATE_CONTRACT_REAL_RED_KIND reads ctx.State["real-kind"],
//     and the two spaces have no automatic bridge. The gate binding itself is
//     unchanged.
//
// Replaces the retired identify-external-system action: identity is now baked
// at load time (UnrollExternalSystems), so no runtime path→name resolution,
// no >1 collapse, and no dead external-system-name stamping survive — only this
// minimal guard + shim. Deterministic — no agent.
func (a actions) resolveExternalSystem(ctx *statemachine.Context) statemachine.Outcome {
	cfg := a.deps.Config
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("resolve-external-system: gh-optivem.yaml not loaded — driver must inject actions.Deps.Config")}
	}
	name := strings.TrimSpace(ctx.Params["external-system-name"])
	if name == "" {
		return statemachine.Outcome{Err: fmt.Errorf("resolve-external-system: external-system-name not baked into clone params — UnrollExternalSystems must bake it (run on a project with external-systems: declared)")}
	}
	// param→State shim for GATE_CONTRACT_REAL_RED_KIND (verify A). Stamped even
	// on an untouched clone (harmless — the gate is never reached after a skip).
	ctx.Set("real-kind", ctx.Params["real-kind"])

	roots, err := ResolveLayerPaths([]string{"external-system-driver-port"}, cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("resolve-external-system: %w", err)}
	}
	names := externalSystemNamesFromChangedPaths(ctx.GetString("external-driver-port-changed-paths"), roots[0])
	_, touched := names[name]
	ctx.Set("external-system-touched", touched)
	return statemachine.Outcome{}
}

// sortedSetKeys returns the keys of a string-set in deterministic order, so
// the external-system actions' error messages are stable across map-iteration
// order.
func sortedSetKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
