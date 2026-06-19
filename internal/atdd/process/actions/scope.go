package actions

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// Context keys consumed by the check-phase-scope action. Centralised so the
// downstream phase-scope-clean gate and the STOP_SCOPE_VIOLATION user-task
// reference one canonical declaration.
const (
	// CtxKeyPhaseScopeClean is the bool check-phase-scope writes to record
	// whether every modified path in the phase fell within the allowed-paths
	// join (process-flow.yaml MID `write:` scope ∘ gh-optivem.yaml paths:).
	// Read by the phase-scope-clean gate.
	CtxKeyPhaseScopeClean = "phase-scope-clean"

	// CtxKeyPhaseScopeViolatingPaths is the []string of modified paths
	// check-phase-scope found outside scope. Populated only on violations;
	// consumed by the STOP_SCOPE_VIOLATION user-task to render the
	// human-review payload.
	CtxKeyPhaseScopeViolatingPaths = "phase-scope-violating-paths"

	// CtxKeyPreAgentFingerprint is the snapshot of the working tree
	// captured by the snapshot-working-tree action immediately before
	// RUN_AGENT. It is the per-phase baseline downstream scope-checking
	// actions (validate-outputs-and-scopes, check-phase-scope) diff
	// against — replaces the previous HEAD-relative baseline, which
	// attributed upstream phases' uncommitted edits to whichever phase
	// happened to be running. Value type: WorkingTreeFingerprint.
	CtxKeyPreAgentFingerprint = "pre-agent-fingerprint"
)

// ---------------------------------------------------------------------------
// Phase-scope enforcement Layer 2 (post-phase scripted check)
// ---------------------------------------------------------------------------

// checkPhaseScope is Layer 2 of phase-scope enforcement (plan
// 20260518-1144 item 5, retargeted at process-flow.yaml node scope per
// plan 20260526-1536). After the agent commits, the action joins:
//
//   - process-flow.yaml's inline per-node scope (the writing-agent MID's
//     EXECUTE_AGENT carries `read:` / `write:` lists; this action checks
//     the write list — paths the agent may modify).
//   - the project's gh-optivem.yaml paths: (layer name → resolved path)
//     plus Family A path-shaped keys in FamilyAPathKeysInScope (system-path
//     today).
//
// It then enumerates the working-tree changes this phase produced and
// checks each modified path against the allowed set with directory-aware
// prefix matching: diffPath ∈ scope iff
// diffPath == P || diffPath.startsWith(P + "/").
//
// Baseline: prefers the per-phase snapshot stashed in
// ctx.State[CtxKeyPreAgentFingerprint] by an upstream snapshot-working-tree
// step (the same baseline validate-outputs-and-scopes uses), so an
// upstream phase's uncommitted edits are not re-attributed to whichever
// phase is currently running. When the snapshot is absent — check-phase-scope
// is dormant in process-flow.yaml today and may be re-wired in a context
// where no per-phase snapshot exists — the action falls back to the full
// dirty-tree set (`git status --porcelain`) and emits a debug line via
// a.deps.Stderr so the re-wiring is loud.
//
// Phase id source: ctx.Params["phase-id"] — the writing-agent MID's name
// (e.g. "write-acceptance-tests"). Resolved via Engine.Scope.
//
// Writes:
//   - CtxKeyPhaseScopeClean (bool)            — false on violation
//   - CtxKeyPhaseScopeViolatingPaths ([]string) — populated on violation
//
// The phase-scope-clean gate reads the boolean; the STOP_SCOPE_VIOLATION
// user-task reads the slice to render the human-review payload.
func (a actions) checkPhaseScope(ctx *statemachine.Context) statemachine.Outcome {
	phaseID := ctx.Params["phase-id"]
	if phaseID == "" {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: phase-id not set in Params — the call-activity invoking the writing-agent MID must pass phase-id")}
	}

	if a.deps.Engine == nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: engine not loaded — driver must inject actions.Deps.Engine")}
	}
	_, write, ok := a.deps.Engine.Scope(phaseID)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf(
			"check_phase_scope: phase id %q not a writing-agent MID in process-flow.yaml — add inline read: / write: scope to its EXECUTE_AGENT node", phaseID)}
	}

	cfg := a.deps.Config
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: gh-optivem.yaml not loaded — driver must inject actions.Deps.Config")}
	}

	allowed, err := ResolveLayerPaths(write, cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope (%s): %w", phaseID, err)}
	}
	allowed, err = narrowAdapterScopeByChannel(write, allowed, ctx.Params["channel"], cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope (%s): %w", phaseID, err)}
	}
	allowed, err = AddSystemSurfaceScope(write, allowed, ctx.Params["channel"], cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope (%s): %w", phaseID, err)}
	}

	var modified []string
	if snapshot, ok := ctx.State[CtxKeyPreAgentFingerprint].(WorkingTreeFingerprint); ok {
		modified, err = a.modifiedPathsSinceFingerprint(context.Background(), snapshot)
	} else {
		// HEAD-equivalent fallback: this action is dormant in
		// process-flow.yaml today and may be re-wired in a context
		// without an upstream snapshot. Log loudly so the operator
		// notices, then enumerate the full dirty tree.
		fmt.Fprintln(a.deps.Stderr,
			"check_phase_scope: no pre-agent-fingerprint in state — falling back to current dirty tree; re-wire with snapshot-working-tree upstream for per-phase semantics")
		modified, err = a.dirtyTreePaths(context.Background())
	}
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: %w", err)}
	}

	var violating []string
	for _, m := range modified {
		if !pathInScope(m, allowed) {
			violating = append(violating, m)
		}
	}
	if len(violating) > 0 {
		ctx.Set(CtxKeyPhaseScopeClean, false)
		ctx.State[CtxKeyPhaseScopeViolatingPaths] = violating
		fmt.Fprintf(a.deps.Stderr,
			"check_phase_scope: %s scope violation — %d path(s) outside scope.\nResolve by: (1) accept the diff if intentional; (2) rewind to an upstream phase; (3) revert and rerun; (4) abort the cycle.\n",
			phaseID, len(violating))
		for _, v := range violating {
			fmt.Fprintf(a.deps.Stderr, "  out-of-scope: %s\n", v)
		}
		return statemachine.Outcome{Bool: false}
	}
	ctx.Set(CtxKeyPhaseScopeClean, true)
	return statemachine.Outcome{Bool: true}
}

// ResolveLayerPaths joins a phase's layer list against the project's
// configured paths: Family A path-shaped keys go through their dedicated
// Config accessor (system-path → cfg.System.Path); everything else is a
// Family B key in cfg.SystemTest.Paths. Missing values surface as errors
// rather than silently shrinking the allowed set — except for a
// monolith-only key (atdd.MonolithOnlyPathKeys) on a multitier config,
// where the empty backing field is expected by construction, not drift:
// the layer is not applicable and is skipped. This is the one resolver
// the preflight scope sweep and the runtime scope check both use, so the
// architecture polymorphism is honoured identically in both.
func ResolveLayerPaths(layers []string, cfg *projectconfig.Config) ([]string, error) {
	out := make([]string, 0, len(layers))
	for _, layer := range layers {
		// Monolith-only key on a multitier config: not applicable. The
		// phases that scope it are monolith-only by construction and never
		// dispatched here, so the empty backing field (projectconfig.System
		// polymorphism) is correct, not a misconfiguration. Skip before the
		// switch so the monolith empty-path error below still fires for an
		// actually-broken monolith config.
		if atdd.MonolithOnlyPathKeys[layer] && cfg.System.Architecture == projectconfig.ArchMultitier {
			continue
		}
		if atdd.FamilyAPathKeysInScope[layer] {
			switch layer {
			case "system-path":
				if cfg.System.Path == "" {
					return nil, fmt.Errorf("layer %q resolves to empty system.path in gh-optivem.yaml", layer)
				}
				out = append(out, cfg.System.Path)
			case "system-db-migration-path":
				if cfg.System.DbMigrationPath == "" {
					return nil, fmt.Errorf("layer %q resolves to empty system.db-migration-path in gh-optivem.yaml", layer)
				}
				out = append(out, cfg.System.DbMigrationPath)
			default:
				return nil, fmt.Errorf("layer %q is in FamilyAPathKeysInScope but has no Config accessor", layer)
			}
			continue
		}
		v, ok := cfg.SystemTest.Paths[layer]
		if !ok || v == "" {
			return nil, fmt.Errorf("layer %q not present in gh-optivem.yaml system-test.paths:", layer)
		}
		out = append(out, v)
	}
	return out, nil
}

// narrowAdapterScopeByChannel tightens a resolved write-scope so a
// channel-split dispatch can only write its own System Driver adapter
// folder. The phase node `implement-system-driver-adapters` is shared:
// the UnrollSystemDriverAdapterChannels clone runs it once per channel
// (ctx.Params["channel"] set), and the no-`channels:` full run reuses
// the same node with no channel param. The single static
// `write: [system-driver-adapter]` resolves to the whole adapter layer;
// when a channel is present we replace that entry with the channel's
// configured member (read verbatim from
// cfg.SystemTest.SystemDriverAdapterChannels[channel], the same member
// the resume guard keys on). No channel → whole layer, unchanged.
//
// write and allowed are index-aligned (ResolveLayerPaths builds allowed
// in layer order), so the substitution targets exactly the
// system-driver-adapter entries. A channel param on a task whose scope
// does not include system-driver-adapter is a no-op. A channel with no
// configured member is a hard error rather than a silent widen to the
// whole layer — the permissive fallback is the very gap this closes.
func narrowAdapterScopeByChannel(write, allowed []string, channel string, cfg *projectconfig.Config) ([]string, error) {
	if channel == "" {
		return allowed, nil
	}
	out := make([]string, len(allowed))
	copy(out, allowed)
	for i, layer := range write {
		if layer != "system-driver-adapter" {
			continue
		}
		member := cfg.SystemTest.SystemDriverAdapterChannels[channel]
		if member == "" {
			return nil, fmt.Errorf("channel %q has no system-driver-adapter member in gh-optivem.yaml system-test.system-driver-adapter-channels: — cannot narrow write-scope to the channel folder", channel)
		}
		out[i] = member
	}
	return out, nil
}

// AddSystemSurfaceScope resolves the monolith-only system-path layer to the
// concrete tier surface a multitier dispatch writes, and appends it to the
// allowed set. It closes the multitier write-scope collapse: ResolveLayerPaths
// skips system-path on multitier (it's monolith-only by construction), so a
// writing-agent's `write: [system-path, …]` would otherwise resolve with the
// production surface absent — every correct backend/frontend write then lands
// out of scope.
//
// It acts only when the architecture is multitier AND system-path is a
// declared write layer (monolith already resolves system-path via
// ResolveLayerPaths; an absent layer is a no-op). It calls the kernel resolver
// cfg.SystemSurfacePaths(channel) — the single home for the channel→tier
// mapping (api→backend, ui→frontend, whole-system→both) — and appends the
// returned paths. An unknown channel (ok==false) is a hard error, mirroring
// narrowAdapterScopeByChannel's "no silent widen".
//
// Append (not index-rewrite) keeps it independent of the write/allowed
// index-alignment narrowAdapterScopeByChannel relies on, so the two narrows
// compose in either order.
func AddSystemSurfaceScope(write, allowed []string, channel string, cfg *projectconfig.Config) ([]string, error) {
	if cfg.System.Architecture != projectconfig.ArchMultitier {
		return allowed, nil
	}
	if !slices.Contains(write, "system-path") {
		return allowed, nil
	}
	surface, ok := cfg.SystemSurfacePaths(channel)
	if !ok {
		return nil, fmt.Errorf("channel %q has no system-path surface on this multitier config (expected one of api, ui, or whole-system) — cannot resolve write-scope to the tier folder", channel)
	}
	return append(allowed, surface...), nil
}

// pathInScope returns true if diffPath is within any allowed path P
// with directory-aware prefix matching: diffPath == P, or diffPath
// starts with P + "/". Raw HasPrefix(P) is wrong — it would let
// ".../shop2/..." match ".../shop". This contract is shared with the
// `gh optivem process scope` CLI projection.
func pathInScope(diffPath string, allowed []string) bool {
	for _, p := range allowed {
		if diffPath == p || strings.HasPrefix(diffPath, p+"/") {
			return true
		}
	}
	return false
}
