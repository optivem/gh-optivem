package driver

import (
	"bytes"
	"fmt"
	"os/exec"
	"path"
	"slices"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// scoped.go implements scoped `gh optivem implement --target <slice>` entry
// (plan 20260530-1725 Items 2b–2d, 3): compose-by-name slice selection plus a
// git-state-derived resume guard.
//
// The handoff between teams crosses clones, so the only durable cross-machine
// signal is the COMMIT (the .gh-optivem/runs/ journal is forensic and
// machine-local). There is therefore NO status file: a slice's "done-ness" is
// read back out of its committed write-scope files each run, content-addressed
// via `git ls-files` / `git status` on resolved paths — never by parsing commit
// messages, branches, or tags (plan "Resume mechanism" Non-goal). resolveScoped
// Entry below is the single place that classifies upstream slices and refuses a
// premature one.
//
// D-red-gate (Item 3) is NOT a new gate here: the slice's expected-red /
// expected-green outcome is expressed as the `expected-test-result` param the
// slice is entered with (Target.ExpectedTestResult), which the slice's existing
// verify nodes enforce. The same content-addressed predicate the resume guard
// uses to classify an UPSTREAM slice DONE is what proves THIS slice finished
// when it committed — so DONE folds the gate in by construction: a slice only
// reaches a commit by passing its own verify nodes, and the commit is what the
// downstream run reads back. (See plan: "implement once, evaluate on this slice
// (gate) and on upstream slices (resume).")

// artifactState is the three-state classification of a slice's resolved
// write-scope footprint against the committed tree (plan Resume mechanism
// point 2). DIRTY is deliberately NOT a handoff point: the cross-clone artifact
// is the commit, so present-but-uncommitted ≠ done.
type artifactState int

const (
	stateAbsent artifactState = iota // paths empty/untracked → slice not started
	stateDirty                       // present but uncommitted → in progress on THIS clone
	stateDone                        // committed clean → handoff-ready
)

// gitRunFn shells `git -C <repoPath> <args…>`. A package var (like nowFn) so
// scoped_test.go can swap in a fake tree-state without spawning real git.
var gitRunFn = func(repoPath string, args ...string) ([]byte, error) {
	full := append([]string{"-C", repoPath}, args...)
	cmd := exec.Command("git", full...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("git %s: %w (stderr: %s)", strings.Join(full, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// classifyFootprint resolves a slice's footprint dirs to one artifactState.
// DIRTY wins over DONE (an in-progress edit on top of a prior commit is not a
// clean handoff point); ABSENT is "nothing committed and nothing dirty".
//
// paths are repo-relative dirs (already resolved + channel-narrowed). An empty
// list is a programming error — callers guard it so a missing path key cannot
// degrade into a bare `git ls-files` that lists the whole tree.
func classifyFootprint(repoPath string, paths []string) (artifactState, error) {
	if len(paths) == 0 {
		return stateAbsent, fmt.Errorf("classifyFootprint: empty path list (unresolved footprint)")
	}
	dirty, err := footprintNonEmpty(repoPath, append([]string{"status", "--porcelain", "--"}, paths...))
	if err != nil {
		return stateAbsent, err
	}
	if dirty {
		return stateDirty, nil
	}
	committed, err := footprintNonEmpty(repoPath, append([]string{"ls-files", "--"}, paths...))
	if err != nil {
		return stateAbsent, err
	}
	if committed {
		return stateDone, nil
	}
	return stateAbsent, nil
}

// footprintNonEmpty runs a git command scoped to pathspecs and reports whether
// it produced any output (tracked files for `ls-files`, change records for
// `status --porcelain`).
func footprintNonEmpty(repoPath string, args []string) (bool, error) {
	out, err := gitRunFn(repoPath, args...)
	if err != nil {
		return false, err
	}
	return len(bytes.TrimSpace(out)) > 0, nil
}

// resolveScopedEntry validates a scoped --target request, enforces the
// upstream-slice resume preconditions, seeds the slice's call-activity params
// onto sCtx, and returns the named sub-process Run should RunProcess.
//
// It is the single validation point shared by the eventual flag path (Item 4)
// and any other caller: the --channel rule (required for channel-split targets,
// rejected for the agnostic `test` slice) and channel-membership against the
// project channels: SSoT are both enforced here, so the flag layer stays a thin
// parse wrapper (parity per the 1702 plan's D2).
func resolveScopedEntry(target Target, channel string, cfg *projectconfig.Config, repoPath string, sCtx *statemachine.Context) (string, error) {
	process, requiresChannel, ok := target.SliceProcess()
	if !ok {
		return "", fmt.Errorf("scoped --target: %q is not a slice (the no-arg full run is not a slice)", target)
	}

	// --channel rule (structural, D-flags). Mirrors the flag-layer check.
	switch {
	case requiresChannel && channel == "":
		return "", fmt.Errorf("--target %s requires --channel (channel-split slice)", target)
	case !requiresChannel && channel != "":
		return "", fmt.Errorf("--target %s is channel-agnostic; --channel is not allowed", target)
	}

	if cfg == nil {
		return "", fmt.Errorf("scoped --target needs a gh-optivem.yaml (channel + write-scope path resolution)")
	}
	if requiresChannel && !slices.Contains(cfg.Channels, channel) {
		return "", fmt.Errorf("--channel %q is not declared in this project's channels: %v", channel, cfg.Channels)
	}

	// Resume guard (Item 2d): refuse a slice whose upstream slice is not DONE.
	if err := checkUpstreamDone(target, channel, cfg, repoPath); err != nil {
		return "", err
	}

	// Slice params (Item 2b / 3). expected-test-result is the per-slice gate;
	// the channel-split slices additionally need the params the per-channel
	// unrolls (channels.go) bind on their cloned nodes — channel/common/suite/
	// tests — since a direct RunProcess enters the callee without traversing the
	// unrolled anchor that would otherwise push them.
	if etr, ok := target.ExpectedTestResult(); ok {
		sCtx.Params["expected-test-result"] = etr
	}
	if requiresChannel {
		sCtx.Params["channel"] = channel
		sCtx.Params["tests"] = "acceptance"
		if target == TargetSystem {
			// D-common option b: the FIRST channel carries the channel-agnostic
			// common layer; later channels are common:false deltas. Same
			// first-channel rule UnrollSystemChannels applies (i == 0).
			sCtx.Params["common"] = boolStr(channel == cfg.Channels[0])
			sCtx.Params["suite"] = "acceptance-" + channel
		}
	}

	return process, nil
}

// checkUpstreamDone enforces the resume preconditions for a scoped slice
// (plan payoff: ordering constraints become *checked*, not positional). Each
// precondition is one content-addressed footprint classification:
//
//   - test            → no upstream.
//   - driver-adapter ch → the shared contract (--target test) must be DONE.
//   - system ch       → the shared contract AND that channel's driver adapter
//     must be DONE; a non-first channel additionally needs the common layer
//     (built on the first channel's --target system, D-common option b) DONE.
func checkUpstreamDone(target Target, channel string, cfg *projectconfig.Config, repoPath string) error {
	switch target {
	case TargetTest:
		return nil

	case TargetDriverAdapter:
		return requireDone(repoPath, "the shared contract (`--target test`)",
			sharedContractFootprint(cfg),
			"run `gh optivem implement --target test` and commit it first")

	case TargetSystem:
		if err := requireDone(repoPath, "the shared contract (`--target test`)",
			sharedContractFootprint(cfg),
			"run `gh optivem implement --target test` and commit it first"); err != nil {
			return err
		}
		if err := requireDone(repoPath, fmt.Sprintf("the %s driver adapter (`--target driver-adapter --channel %s`)", channel, channel),
			driverAdapterFootprint(cfg, channel),
			fmt.Sprintf("run `gh optivem implement --target driver-adapter --channel %s` and commit it first", channel)); err != nil {
			return err
		}
		if first := cfg.Channels[0]; channel != first {
			if err := requireDone(repoPath, fmt.Sprintf("the common layer (built on the first channel %q)", first),
				commonFootprint(cfg),
				fmt.Sprintf("run `gh optivem implement --target system --channel %s` first so the common layer lands", first)); err != nil {
				return err
			}
		}
		return nil
	}
	return nil
}

// requireDone classifies one upstream footprint and turns a non-DONE state into
// an operator-actionable refusal. An unresolved (empty) footprint is itself an
// error — a scoped run cannot reason about a layer the config does not place.
func requireDone(repoPath, label string, paths []string, remedy string) error {
	if len(paths) == 0 {
		return fmt.Errorf("scoped run blocked: cannot locate %s — its write-scope path is not set in gh-optivem.yaml", label)
	}
	state, err := classifyFootprint(repoPath, paths)
	if err != nil {
		return fmt.Errorf("scoped run: classify %s: %w", label, err)
	}
	switch state {
	case stateDone:
		return nil
	case stateDirty:
		return fmt.Errorf("scoped run blocked: %s is present but uncommitted on this clone — the cross-clone handoff is the commit, so commit it (or %s)", label, remedy)
	default: // stateAbsent
		return fmt.Errorf("scoped run blocked: %s is not done — %s", label, remedy)
	}
}

// sharedContractFootprint is the irreducible artifact of the `--target test`
// slice: the acceptance tests. Every story/bug ticket writes them, so their
// committed presence is the robust "shared contract ran" signal. (The slice's
// DSL / driver-port / external writes are conditional on port-change gateways,
// so they are deliberately NOT in the detection footprint — an unchanged port
// would read as falsely-ABSENT.)
func sharedContractFootprint(cfg *projectconfig.Config) []string {
	return nonEmptyPaths(layerPath(cfg, "at-test"))
}

// driverAdapterFootprint narrows the System Driver adapter layer to one
// channel's subtree. Per the 2026-06-04 operator clarification the adapters
// split by subdirectory (`<driver-adapter>/{api,ui,external,shared}`), so a
// channel's adapter is `<driver-adapter>/<channel>` — distinct from the
// `shared` stubs the test slice writes, which is what keeps the two slices'
// footprints from colliding. (Making these folders configurable rather than a
// path-join convention is plans/20260604-0955-configurable-per-channel-adapter-
// folders.md.)
func driverAdapterFootprint(cfg *projectconfig.Config, channel string) []string {
	base := layerPath(cfg, "driver-adapter")
	if base == "" {
		return nil
	}
	return []string{path.Join(base, channel)}
}

// commonFootprint is the channel-agnostic common layer's irreducible artifact:
// the shared DB migrations (system-db-migration-path), written only when the
// first channel's --target system runs with common:true (D-common option b).
func commonFootprint(cfg *projectconfig.Config) []string {
	return nonEmptyPaths(layerPath(cfg, "system-db-migration-path"))
}

// layerPath resolves a write-scope layer key to its physical repo-relative path
// via the same flat Family A+B map phase-doc substitution uses — no parallel
// path table (plan Resume mechanism point 1: reuse the scope SSoT's resolution).
func layerPath(cfg *projectconfig.Config, key string) string {
	return cfg.PlaceholderMap()[key]
}

// nonEmptyPaths wraps a single resolved path in a slice, dropping it when empty
// so callers get nil (which requireDone reports as "path not set") rather than
// a [""] that would widen a git pathspec to the whole tree.
func nonEmptyPaths(p string) []string {
	if p == "" {
		return nil
	}
	return []string{p}
}

// boolStr renders a Go bool as the lowercase string the BPMN param layer carries
// (params are map[string]string), matching statemachine's unexported boolParam.
func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
