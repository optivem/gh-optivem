package actions

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// validateOutputsAndScopes is the LOW `execute-agent` primitive's
// post-RUN_AGENT validation step (BPMN Phase D Item 7, Q-D6).
//
// The agent emits structured outputs by invoking `gh optivem output
// write KEY=VAL` from its Bash tool; each call appends one JSON object
// to the per-dispatch JSONL file the driver pre-computes (path stashed
// in ctx.State["output-file-path"] before RUN_AGENT, env-exported as
// GH_OPTIVEM_OUTPUT_FILE). This action:
//
//  1. Walks that JSONL file (when present) applying last-write-wins per
//     key, and flattens the resulting map into ctx.State so downstream
//     actions and gates read the values the same way they always did.
//  2. Looks up the writing-agent MID's declared OutputSpec list via
//     Engine.Outputs(phaseTaskName) and presence-checks every
//     non-Optional key against the flattened state.
//  3. Diffs the working tree against the per-phase baseline stashed at
//     ctx.State[CtxKeyPreAgentFingerprint] by snapshot-working-tree and
//     flags any modified path outside the MID's `write:` scope.
//
// The JSONL channel replaces the older prose-YAML tail (parsed by the
// deleted clauderun.ParseOutputs). The new channel works uniformly in
// both interactive and autonomous modes — interactive mode used to
// silently fail every outputs validation because RunResult.ResultText
// was empty (claude-CLI prints to TTY, not envelope).
//
// Reads:
//   - ctx.State["output-file-path"]      — absolute path to the per-dispatch
//     JSONL file. Set by the driver
//     ONLY when the MID's BPMN
//     `outputs:` list is non-empty;
//     absent → no-op output read.
//   - ctx.Params["originating-task-name"] (preferred) or
//     ctx.Params["task-name"]            — the writing-agent MID name used
//     to look up scope (Engine.Scope)
//     AND outputs (Engine.Outputs).
//     The originating- prefix is set
//     by the `fix` LOW so the inner
//     execute-agent validation can
//     recover the outer MID's scope +
//     outputs after task-name is
//     shadowed to fix-${failure-kind}.
//   - ctx.State[CtxKeyPreAgentFingerprint] — WorkingTreeFingerprint
//     captured by snapshot-working-tree.
//     Required when the dispatching node
//     has a write scope; missing key is
//     a wiring bug and surfaces as
//     Outcome.Err.
//
// Writes:
//   - ctx.State["outputs-and-scopes-valid"]   — bool.
//   - ctx.State["failure-kind"]               — set on false; one of
//     missing-output | scope-diff
//     (priority: missing-output
//     wins when both fail).
//   - ctx.State["failing-task-name"]          — set on false; the OUTER
//     execute-agent's task-name
//     (e.g. "write-acceptance-tests")
//     captured from ctx.Params
//     before the inner `fix`
//     call-activity shadows it.
//     Consumed by the
//     fix-missing-output /
//     fix-scope-diff prompts.
//   - ctx.State["missing-outputs"]            — set on missing-output;
//     comma-separated list of
//     unemitted output keys.
//   - ctx.State["scope-violating-paths"]      — set on scope-diff;
//     comma-separated list of
//     working-tree paths
//     outside the declared
//     write scope.
//   - ctx.State["phase-changed-files"]        — set on every dispatch
//     (success and failure).
//     Newline-joined sorted list of
//     every path in the snapshot
//     delta (in-scope +
//     out-of-scope). Consumed by
//     fix-scope-diff (this MID's
//     own failure-kind),
//     fix-unexpected-failing-tests,
//     and fix-unexpected-passing-tests
//     to scope their reasoning to
//     "what the WRITE phase just
//     edited." Replaces the
//     previous live
//     `git status --porcelain`
//     shell-out at dispatch time,
//     which was fragile against
//     working-tree state (clean
//     tree at dispatch ≠ no
//     WRITE-phase changes).
//
// Does NOT surface as Outcome.Err — the gateway's false branch
// dispatches `fix-${failure-kind}` per Q-late-5. Hard errors
// (gh-optivem.yaml missing, git unusable, engine not wired, snapshot
// key missing) DO surface as Err since they indicate a wiring/infra
// problem, not an
// agent-output problem.
func (a actions) validateOutputsAndScopes(ctx *statemachine.Context) statemachine.Outcome {
	if a.deps.Engine == nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: engine not loaded — driver must inject actions.Deps.Engine")}
	}

	// 1. Read the per-dispatch JSONL outputs file (when the driver
	// stashed a path), apply last-write-wins per key, coerce values per
	// the BPMN-declared types, and flatten into ctx.State. The driver
	// only stashes output-file-path when the MID declared outputs, so
	// an absent stash skips this block entirely.
	taskName := phaseTaskName(ctx)
	declared, _ := a.deps.Engine.Outputs(taskName)
	outputFile, _ := ctx.State["output-file-path"].(string)
	if outputFile != "" {
		flattened, err := readOutputsJSONL(outputFile, declared)
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: %w", err)}
		}
		for k, v := range flattened {
			// Cascade-namespace the per-cascade outputs (plan 20260606-1525;
			// test-names added by plan 20260608-1231): the agent emits the
			// bare key, but namespacedLandingKeys members land under an
			// `at-`/`ct-` key chosen by the active test-category so the nested
			// contract excursion can't clobber the acceptance cascade's
			// verdict or test-name list. landingStateKey is the identity for
			// every other output.
			ctx.Set(landingStateKey(k, ctx.Params["test-category"]), v)
		}
	}

	// CT-path System-Driver fence (plan 20260527-1147 Item 4). A
	// dsl-implementer dispatched on the contract-test path (test-category=contract)
	// stimulates the External-System Driver only — the System Driver port is
	// conceptually out of scope. If it emits system-driver-port-changed=true,
	// that flag leaks up into the AT cycle's system-driver-adapter gate and
	// fires a spurious adapter cycle. This is a structural invariant
	// violation, not a recoverable agent-output problem (no fix-* pass can
	// correct "wrong test path"), so it halts with a diagnostic rather than
	// routing to the soft fix loop. The AT path (test-category=acceptance) emitting
	// the same flag is correct and untouched. `test-category` reaches here via the
	// wrapCallActivity param-merge from the IMPLEMENT_AND_VERIFY_DSL call site.
	if ctx.Params["test-category"] == "contract" {
		if changed, _ := ctx.State["ct-system-driver-port-changed"].(bool); changed {
			return statemachine.Outcome{Err: fmt.Errorf(
				"validate-outputs-and-scopes: CT-path dsl-implementer must not touch the System Driver port; contract tests stimulate the External-System Driver only (set system-driver-port-changed=false on the test-category=contract path)")}
		}
	}

	// 2. Output presence check — every non-Optional declared key must
	// be present in ctx.State after the flatten.
	var missing []string
	for _, spec := range declared {
		if spec.Optional {
			continue
		}
		// Check the cascade-namespaced landing key (plan 20260606-1525):
		// the port-changed verdicts are flattened under their `at-`/`ct-`
		// key, so the presence check must look there, not under the bare
		// declared key. The agent-facing diagnostic still names the bare key.
		if _, ok := ctx.State[landingStateKey(spec.Key, ctx.Params["test-category"])]; !ok {
			missing = append(missing, spec.Key)
		}
	}
	if len(missing) > 0 {
		ctx.Set("outputs-and-scopes-valid", false)
		ctx.Set("failure-kind", "missing-output")
		ctx.Set("failing-task-name", taskName)
		ctx.Set("missing-outputs", strings.Join(missing, ","))
		fmt.Fprintf(a.deps.Stderr,
			"validate-outputs-and-scopes: agent did not emit expected outputs: %s\n",
			strings.Join(missing, ", "))
		return statemachine.Outcome{}
	}

	// 3. Scope check (no-op when the dispatching node is not a
	// writing-agent MID — refine-acceptance-criteria declares
	// scope: none in prompt frontmatter and has no inline read/write,
	// so Engine.Scope returns ok=false and the scope check is skipped).
	_, write, ok := a.deps.Engine.Scope(taskName)
	if !ok {
		ctx.Set("outputs-and-scopes-valid", true)
		return statemachine.Outcome{}
	}

	cfg := a.deps.Config
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: gh-optivem.yaml not loaded — driver must inject actions.Deps.Config")}
	}
	allowed, err := ResolveLayerPaths(write, cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: %w", err)}
	}
	allowed, err = narrowAdapterScopeByChannel(write, allowed, ctx.Params["channel"], cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: %w", err)}
	}
	allowed, err = AddSystemSurfaceScope(write, allowed, ctx.Params["channel"], cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: %w", err)}
	}
	snapshot, ok := ctx.State[CtxKeyPreAgentFingerprint].(WorkingTreeFingerprint)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: pre-agent-fingerprint not set — execute-agent must run snapshot-working-tree before RUN_AGENT")}
	}
	modified, err := a.modifiedPathsSinceFingerprint(context.Background(), snapshot)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: %w", err)}
	}
	// Stash the full snapshot delta (in-scope + out-of-scope) on every
	// dispatch — success and failure. The fix-scope-diff prompt reads it
	// as ${changed-files} on the scope-diff failure branch, and the
	// fix-unexpected-{failing,passing}-tests prompts read it on the
	// verify-tests-fail branch downstream of a clean validate. Setting
	// it unconditionally replaces the live `git status --porcelain`
	// shell-out fixChangedFiles used to do at dispatch time, which was
	// fragile against working-tree state (a clean tree at dispatch ≠
	// no WRITE-phase changes — staged-and-committed paths drop out of
	// `git status` immediately).
	ctx.Set("phase-changed-files", strings.Join(modified, "\n"))

	// Preserve the external-driver-PORT changed-path subset for
	// IDENTIFY_EXTERNAL_SYSTEM (plan 20260613-1835). Identity is resolved
	// SOLELY from the external-driver-port change (interface methods AND its
	// DTOs) — never from the adapter files the impl step writes, which are
	// legitimately empty on a DTO-only change and crashed #65. The port
	// change happens in this (AT-cascade DSL) phase, so phase-changed-files
	// carries it now but is overwritten before IDENTIFY runs; stash the
	// under-port-root subset into a durable flat key. Guarded to write only
	// when the landed verdict is true AND the subset is non-empty, so the
	// compile-only CT DSL phase (which does not re-change the port) cannot
	// clobber the AT phase's list.
	if portChanged, _ := ctx.State[landingStateKey("external-driver-port-changed", ctx.Params["test-category"])].(bool); portChanged {
		if portRoots, perr := ResolveLayerPaths([]string{"external-system-driver-port"}, cfg); perr == nil && len(portRoots) > 0 {
			portRoot := portRoots[0]
			var portPaths []string
			for _, m := range modified {
				if strings.HasPrefix(m, portRoot+"/") {
					portPaths = append(portPaths, m)
				}
			}
			if len(portPaths) > 0 {
				ctx.Set("external-driver-port-changed-paths", strings.Join(portPaths, "\n"))
			}
		}
	}

	var violating []string
	for _, m := range modified {
		if !pathInScope(m, allowed) {
			violating = append(violating, m)
		}
	}
	if len(violating) > 0 {
		ctx.Set("outputs-and-scopes-valid", false)
		ctx.Set("failure-kind", "scope-diff")
		ctx.Set("failing-task-name", taskName)
		ctx.Set("scope-violating-paths", strings.Join(violating, ","))
		fmt.Fprintf(a.deps.Stderr,
			"validate-outputs-and-scopes: %d path(s) outside scope %v:\n",
			len(violating), write)
		for _, v := range violating {
			fmt.Fprintf(a.deps.Stderr, "  out-of-scope: %s\n", v)
		}
		return statemachine.Outcome{}
	}

	// Diagnostic keys owned by this action; clear on success so a later
	// success doesn't carry residue from an earlier failure into the
	// trace or into a downstream fix-* prompt via ExpandParams's
	// state-fallback. phase-changed-files is stamped unconditionally above
	// and is NOT a diagnostic key — left alone.
	ctx.Unset("failure-kind")
	ctx.Unset("failing-task-name")
	ctx.Unset("missing-outputs")
	ctx.Unset("scope-violating-paths")
	ctx.Set("outputs-and-scopes-valid", true)
	return statemachine.Outcome{}
}

// namespacedLandingKeys are the bare agent outputs the landing layer rewrites
// to a cascade-namespaced key — an `at-`/`ct-` prefix chosen by the active
// test-category — so a nested contract excursion can't clobber the acceptance
// cascade's value (plan 20260606-1525; extended to test-names by plan
// 20260608-1231). The three port-changed verdicts (dsl-port-changed by the
// test-code writers; the two driver-port flags by the DSL implementer) gate
// downstream re-reads; test-names is the per-cascade list the GREEN verify
// selects on. The map's meaning is "outputs that namespace by cascade tag," so
// any future per-cascade output joins this one set rather than growing a
// second allowlist.
//
// isolated-test-names is the stub-fidelity-test-writer's own per-cascade list
// (plan 20260620-2348): it lands under `ct-isolated-test-names`, a key DISTINCT
// from `ct-test-names`, so the stub-only fidelity register the new writer emits
// never leaks into the shared-register PROBE_CONTRACT_REAL / PROBE_CONTRACT_STUB
// (which select `${ct-test-names}`). The dedicated stub-only probe/verify pair
// selects `${ct-isolated-test-names}` instead, keeping clean failure
// localization (shared red ⇒ stub/real disagree; stub-only red ⇒ stub broken)
// and keeping the never-resettable real driver away from exact-set/empty asserts.
var namespacedLandingKeys = map[string]bool{
	"dsl-port-changed":             true,
	"system-driver-port-changed":   true,
	"external-driver-port-changed": true,
	"test-names":                   true,
	"isolated-test-names":          true,
}

// landingStateKey returns the ctx.State key a flattened agent output lands
// under (plan 20260606-1525; test-names added by plan 20260608-1231). The
// namespacedLandingKeys outputs are prefixed by the active test-category —
// acceptance → `at-`, contract → `ct-` — so the nested contract excursion
// writes only `ct-*` and can't overwrite the acceptance cascade's `at-*`
// value the parent re-gates / GREEN verify read. Every other output is the
// identity. In production the emitters always carry a test-category (threaded
// at their call sites); an unrecognised category falls back to the bare key,
// where the gate's strict "not set" check surfaces the wiring bug loudly
// rather than mis-routing.
func landingStateKey(key, testCategory string) string {
	if !namespacedLandingKeys[key] {
		return key
	}
	switch testCategory {
	case "acceptance":
		return "at-" + key
	case "contract":
		return "ct-" + key
	default:
		return key
	}
}

// phaseTaskName returns the writing-agent MID name to look up scope by.
// Prefers ctx.Params["originating-task-name"] (set by the `fix` LOW so
// fix dispatches retain the outer MID's scope identity) and falls back
// to ctx.Params["task-name"] for the normal path.
func phaseTaskName(ctx *statemachine.Context) string {
	if orig := ctx.Params["originating-task-name"]; orig != "" {
		return orig
	}
	return ctx.Params["task-name"]
}

// snapshotWorkingTree is the body of the BPMN Phase D
// execute-agent.SNAPSHOT_WORKING_TREE service task. It captures a
// WorkingTreeFingerprint of every dirty path and stashes it in
// ctx.State[CtxKeyPreAgentFingerprint] for the post-RUN_AGENT
// validate-outputs-and-scopes step to diff against.
//
// Failure to enumerate the dirty set (e.g. `git` not on PATH, repo
// path invalid) is a wiring problem, not an agent-output problem, so
// it surfaces as Outcome.Err — same shape as
// validate-outputs-and-scopes' hard-error path.
func (a actions) snapshotWorkingTree(ctx *statemachine.Context) statemachine.Outcome {
	fp, err := a.captureWorkingTreeFingerprint(context.Background())
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("snapshot-working-tree: %w", err)}
	}
	ctx.State[CtxKeyPreAgentFingerprint] = fp
	return statemachine.Outcome{}
}

// readOutputsJSONL reads the per-dispatch JSONL outputs file the agent
// appended to via `gh optivem output write`, applies last-write-wins per
// key across lines, and coerces each value to the Go shape declared in
// the BPMN OutputSpec list (bool → bool, string → string, string-list →
// []string). Returns an empty map when the file is missing — the
// dispatcher stashed a path but the agent emitted no writes (or the run
// terminated early). Tolerates blank/whitespace lines but treats any
// malformed JSON line as a hard error so the cycle stops with a clear
// "agent emitted malformed output line" message rather than silently
// dropping the agent's intent.
//
// Unknown keys (not in declared) are tolerated and pass through
// unchanged — the CLI side already rejects them at write-time, so a key
// reaching this reader past the allow-list is either a test fixture or
// a deliberately-permissive caller. We don't double-enforce.
func readOutputsJSONL(path string, declared []statemachine.OutputSpec) (map[string]any, error) {
	out := map[string]any{}
	typeByKey := make(map[string]string, len(declared)+2)
	for _, o := range declared {
		typeByKey[o.Key] = o.Type
	}
	// Universal envelope keys (plan 20260528-1150). When the MID declares
	// no outputs but the dispatcher seeded the envelope channel (because
	// category: prod-agent), the JSONL may carry scope-exception-* lines
	// the coercer must shape correctly — coerceJSONOutputValue's default
	// branch returns the JSON-decoded []any as-is, but
	// scopeExceptionRequested gate type-asserts []string, so an
	// uncoerced value mis-routes the cycle to FIX. A MID-declared entry
	// for the same key always wins (we only fill gaps).
	for _, o := range statemachine.EnvelopeOutputSpecs() {
		if _, ok := typeByKey[o.Key]; !ok {
			typeByKey[o.Key] = o.Type
		}
	}

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return nil, fmt.Errorf("open outputs file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, fmt.Errorf("agent emitted malformed output line: %q: %w", string(line), err)
		}
		if row == nil {
			return nil, fmt.Errorf("agent emitted malformed output line: %q (not a JSON object)", string(line))
		}
		for k, v := range row {
			coerced, err := coerceJSONOutputValue(k, v, typeByKey[k])
			if err != nil {
				return nil, err
			}
			out[k] = coerced
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read outputs file: %w", err)
	}
	return out, nil
}

// coerceJSONOutputValue normalises a value decoded from one JSONL line to
// the Go shape downstream consumers expect. The `output write` CLI
// already encodes values per the declared type, so the input is
// already-shaped JSON (bool / string / []string-as-JSON-array). The job
// here is to flatten the `[]any` that json.Unmarshal returns for arrays
// into a `[]string` so the existing []string-typed readers (e.g. the
// scope_exception_requested gate, the runCommand --test=… joiner) keep
// working unchanged.
//
// An undeclared key (declaredType == "") passes through untouched; the
// CLI rejects unknown keys at write time, so this branch is just
// defensive for test fixtures that hand-craft JSONL outside the CLI.
func coerceJSONOutputValue(key string, v any, declaredType string) (any, error) {
	switch declaredType {
	case statemachine.OutputTypeStringList:
		raw, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("output key %q: declared string-list but emitted %T", key, v)
		}
		out := make([]string, 0, len(raw))
		for i, e := range raw {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("output key %q: string-list element [%d] is %T, want string", key, i, e)
			}
			out = append(out, s)
		}
		return out, nil
	case statemachine.OutputTypeBool:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("output key %q: declared bool but emitted %T", key, v)
		}
		return b, nil
	case statemachine.OutputTypeString:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("output key %q: declared string but emitted %T", key, v)
		}
		return s, nil
	default:
		// Undeclared key — pass through as-is.
		return v, nil
	}
}
