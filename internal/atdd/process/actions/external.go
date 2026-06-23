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

// touchedExternalSystemNames derives the set of external-system names this
// ticket touched, by PRECEDENCE (plan 20260620-1850): when the ticket declares
// External System Contract Criteria (ticket-has-escc), the explicit escc-systems
// names are authoritative — the contract/stub room may have opened with NO
// external-driver PORT file changed (the #65 list-shaped story), so the
// port-change paths would be empty and must not be the source. Otherwise fall
// back to the names the port change touched (the legacy 90% path). The explicit
// declaration is authoritative when present; an undeclared port change is never
// silently merged in.
//
// ESCC names are matched case-INSENSITIVELY against the registry: tickets read
// "External System: ERP" while the registry key is the lowercase "erp", so the
// declared names are lowercased here — matching the lowercase first path segment
// externalSystemNamesFromChangedPaths returns. Both sources therefore yield the
// same lowercase registry-key space.
func touchedExternalSystemNames(ctx *statemachine.Context, root string) map[string]struct{} {
	if hasESCC, _ := ctx.State["ticket-has-escc"].(bool); hasESCC {
		names := map[string]struct{}{}
		systems, _ := ctx.State["escc-systems"].([]string)
		for _, s := range systems {
			s = strings.ToLower(strings.TrimSpace(s))
			if s != "" {
				names[s] = struct{}{}
			}
		}
		return names
	}
	return externalSystemNamesFromChangedPaths(ctx.GetString("external-driver-port-changed-paths"), root)
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
	hasESCC, _ := ctx.State["ticket-has-escc"].(bool)
	names := touchedExternalSystemNames(ctx, roots[0])

	unregistered := map[string]struct{}{}
	for name := range names {
		if _, ok := cfg.ExternalSystems[name]; !ok {
			unregistered[name] = struct{}{}
		}
	}
	if len(unregistered) > 0 {
		source := "the external-driver-port change touches"
		if hasESCC {
			source = "the ticket's External System Contract Criteria name"
		}
		return statemachine.Outcome{Err: fmt.Errorf("validate-external-systems-registered: %s external system(s) %s not registered in gh-optivem.yaml external-systems: — onboard them (declaring their real-kind) before running the contract-test cycle", source, strings.Join(sortedSetKeys(unregistered), ", "))}
	}
	return statemachine.Outcome{}
}

// validateRedesignExternalRequiresESCC is the upfront no-silent-no-op guard for
// the redesign-external-system-structure cycle (plan 20260622-1739 Step 4c). The
// redesign path runs no AT/CT cascade, so external-driver-port-changed-paths is
// never populated; the ONLY selection source for which external system(s) the
// reshape targets is the ticket's External System Contract Criteria (escc-systems,
// stamped by parse-ticket). Without ESCC, touchedExternalSystemNames returns the
// empty set, every unrolled clone's external-system-touched is false, and the
// whole cycle silently no-ops (the #65-class bug). This guard hard-errors up front
// so a redesign-external ticket missing its ESCC section fails loud with an
// actionable message rather than completing as a no-op.
//
// parse-ticket always stamps ticket-has-escc (false when the section is absent),
// so a missing key reads as false and surfaces the same actionable error — never a
// silent default-yes. Deterministic — no agent. Mirrors
// validate-external-systems-registered (which runs immediately after and checks
// the ESCC-named systems against the registry).
func (a actions) validateRedesignExternalRequiresESCC(ctx *statemachine.Context) statemachine.Outcome {
	if hasESCC, _ := ctx.State["ticket-has-escc"].(bool); !hasESCC {
		return statemachine.Outcome{Err: fmt.Errorf("validate-redesign-external-requires-escc: a redesign-external-system ticket must declare an `## External System Contract Criteria` section naming the external system(s) whose response contract is being reshaped — without it the redesign has no target system and would silently no-op; add the section (with `External System: <name>`) and re-run")}
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
	names := touchedExternalSystemNames(ctx, roots[0])
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
