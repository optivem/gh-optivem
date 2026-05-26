// Package trace adds a per-node logging decorator to the engine, producing
// a chronological audit trail of every step the ATDD pipeline takes:
//
//   - service-task: action name, ctx.State keys mutated, Outcome.Value/Bool
//   - gateway:      binding name, evaluated value, ctx.State keys mutated
//   - user-task:    agent name, working-tree paths the agent touched
//   - call-activity:process name, params pushed
//   - start/end:    just the node id and kind
//
// The decorator sits at the outermost layer (after override.Wrap), so what
// it logs is exactly what the engine's RunProcess loop dispatches. Output is
// plain text with `[trace HH:MM:SS]` prefixes so the trace stream is easy
// to grep alongside clauderun's existing colored agent banners.
//
// File-list capture for user-task nodes works by taking a `git status
// --porcelain` snapshot before and after the wrapped NodeFn fires. The
// diff is the set of paths the agent introduced or modified. The
// subsequent commit_phase action will commit those paths, but at trace
// time we want them visible *as soon as the agent exits* — well before
// commit_phase runs — so the operator can see what landed even if the
// run halts before commit.
//
// trace is testable without shelling out: GitRunner is an injectable seam
// (default execGit) and Out is the writer the decorator writes to (default
// os.Stdout).
package trace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// GitRunner is the seam for `git show --name-only` lookups. Tests inject a
// fake; production falls back to execGit.
type GitRunner interface {
	Run(ctx context.Context, dir string, args ...string) ([]byte, error)
}

// Deps bundles trace's collaborators. A zero-value Deps writes to os.Stdout
// and shells out to the real `git`.
type Deps struct {
	Out      io.Writer
	Git      GitRunner
	RepoPath string

	// colorize is set by withDefaults when Out is an interactive stdout TTY.
	// MultiWriter (--log-file) and bytes.Buffer (tests) keep it false so the
	// log file and test fixtures stay ANSI-free.
	colorize bool
}

func (d Deps) withDefaults() Deps {
	if d.Out == nil {
		d.Out = os.Stdout
	}
	if d.Git == nil {
		d.Git = execGit{}
	}
	if f, ok := d.Out.(*os.File); ok && f == os.Stdout && isatty.IsTerminal(f.Fd()) {
		d.colorize = true
	}
	return d
}

// paint wraps s in c's ANSI escapes when colorize is true; otherwise returns
// s unchanged. Per-instance EnableColor avoids touching fatih/color's global
// NoColor flag — tests in other packages that share the process must not see
// our color choice flip on them.
func (d Deps) paint(s string, attrs ...color.Attribute) string {
	if !d.colorize {
		return s
	}
	c := color.New(attrs...)
	c.EnableColor()
	return c.Sprint(s)
}

// nowFn is the package-level seam for tests that need a stable timestamp.
// Production points at time.Now.
var nowFn = time.Now

// WrapAll decorates every node in every flow of eng with a trace
// decorator. Call this once during driver startup, after the other
// decorators (verify, override) have been applied — trace should sit at
// the outermost layer so the entry/exit lines bracket everything else.
func WrapAll(eng *statemachine.Engine, deps Deps) {
	deps = deps.withDefaults()
	for _, process := range eng.Processes {
		for id, node := range process.Nodes {
			node.Fn = wrap(node, deps)
			process.Nodes[id] = node
		}
	}
}

// wrap returns a NodeFn that logs entry/exit around inner. The closure
// captures the original Node (kind + raw) so it has the metadata it needs
// to render the entry banner without re-querying the engine. For
// user-task nodes the wrapper also snapshots the working tree (via
// `git status --porcelain`) on each side of the dispatch so the exit
// banner can list what the agent changed.
func wrap(node statemachine.Node, deps Deps) statemachine.NodeFn {
	inner := node.Fn
	return func(ctx *statemachine.Context) statemachine.Outcome {
		writeEnter(deps, node, ctx)
		preState := snapshotState(ctx.State)
		preDirty := snapshotDirty(deps, node.Kind)
		started := nowFn()
		out := inner(ctx)
		elapsed := nowFn().Sub(started).Round(time.Millisecond)
		postState := snapshotState(ctx.State)
		postDirty := snapshotDirty(deps, node.Kind)
		writeExit(deps, node, out, elapsed, preState, postState, preDirty, postDirty)
		return out
	}
}

// writeEnter prints the per-node entry banner. The format is:
//
//	[trace HH:MM:SS] > NODE_ID  kind=<kind> <selector>=<name>
//
// where <selector> is action / agent / binding / process depending on Kind.
// Templated fields (e.g. ${agent} on structural_cycle nodes) are expanded
// against ctx.Params so the operator sees the substituted name rather
// than the literal placeholder.
//
// On a TTY: `[trace …]` faint, `>` cyan, node ID bold (cyan-bold for
// call-activity so process boundaries stand out as the "phase" markers above
// their service-task / gateway / user-task children).
func writeEnter(deps Deps, node statemachine.Node, ctx *statemachine.Context) {
	parts := []string{
		fmt.Sprintf("kind=%s", kindLabel(node.Kind)),
	}
	switch node.Kind {
	case statemachine.ServiceTask:
		if node.Raw.Action != "" {
			parts = append(parts, fmt.Sprintf("action=%s", node.Raw.Action))
		}
	case statemachine.UserTask:
		if node.Raw.Agent != "" {
			parts = append(parts, fmt.Sprintf("agent=%s",
				statemachine.ExpandParams(node.Raw.Agent, ctx.Params, ctx.State)))
		}
	case statemachine.Gateway:
		if node.Raw.Binding != "" {
			parts = append(parts, fmt.Sprintf("binding=%s", node.Raw.Binding))
		}
	case statemachine.CallActivity:
		if node.Raw.Process != "" {
			parts = append(parts, fmt.Sprintf("process=%s", node.Raw.Process))
		}
		if len(node.Raw.Params) > 0 {
			parts = append(parts, fmt.Sprintf("params=%s", formatParams(node.Raw.Params)))
		}
	}
	fmt.Fprintf(deps.Out, "%s %s %s  %s\n",
		deps.tracePrefix(),
		deps.paint(">", color.FgCyan),
		deps.nodeIDPaint(node),
		strings.Join(parts, " "))
}

// writeExit prints the per-node exit banner and any follow-on detail
// (state-delta keys, working-tree-delta paths). The format is:
//
//	[trace HH:MM:SS] OK NODE_ID -> <outcome>  (<elapsed>)
//	[trace HH:MM:SS]    state: key=value, …
//	[trace HH:MM:SS]    files: path, …
//
// The status word is normally OK, but verify-style actions stamp
// Outcome.Value with a failure class so the banner can render `RED
// NODE_ID` or `INFRA NODE_ID` instead — see outcomeStatusLabel. The
// previous "OK RUN_TESTS -> (no result)" line was the most misleading
// thing in the trace; it directly contradicted the inline "(test run
// failed: ... — continuing)" the same node had just printed.
//
// When the node returns an empty Outcome but did write to ctx.State
// (e.g. a `user-task` `agent: human` that records `approval-outcome`,
// or a gateway whose binding evaluated to a `Bool: false` that
// formatOutcome can't distinguish from "no result"), the state delta is
// hoisted into the banner — so the line reads `OK ASK_HUMAN ->
// approval-outcome=rejected` rather than the misleading `OK ASK_HUMAN ->
// (no result)` followed by a separate `state:` line. The follow-on
// `state:` line is suppressed in that case to avoid duplication.
//
// On Outcome.Err the first line becomes:
//
//	[trace HH:MM:SS] FAIL NODE_ID -> <error>  (<elapsed>)
//
// and no follow-on lines are emitted (the engine halts the run anyway).
func writeExit(deps Deps, node statemachine.Node, out statemachine.Outcome, elapsed time.Duration, pre, post map[string]string, preDirty, postDirty map[string]bool) {
	w := deps.Out
	if out.Err != nil {
		fmt.Fprintf(w, "%s %s %s -> %v  (%s)\n",
			deps.tracePrefix(),
			deps.paint("FAIL", color.FgRed),
			deps.nodeIDPaint(node),
			out.Err, elapsed)
		return
	}
	label, attr := outcomeStatusLabel(out)
	detail := formatOutcome(out)
	delta := stateDelta(pre, post)
	hoistedDelta := detail == "(no result)" && delta != ""
	if hoistedDelta {
		detail = delta
	}
	if detail != "" {
		fmt.Fprintf(w, "%s %s %s -> %s  (%s)\n",
			deps.tracePrefix(),
			deps.paint(label, attr),
			deps.nodeIDPaint(node),
			detail, elapsed)
	} else {
		fmt.Fprintf(w, "%s %s %s  (%s)\n",
			deps.tracePrefix(),
			deps.paint(label, attr),
			deps.nodeIDPaint(node),
			elapsed)
	}
	if delta != "" && !hoistedDelta {
		fmt.Fprintf(w, "%s    %s %s\n", deps.tracePrefix(), deps.paint("state:", color.Faint), delta)
	}
	if node.Kind == statemachine.UserTask {
		if files := dirtyDelta(preDirty, postDirty); len(files) > 0 {
			fmt.Fprintf(w, "%s    %s %s\n", deps.tracePrefix(), deps.paint("files:", color.Faint), strings.Join(files, ", "))
		}
	}
}

// outcomeStatusLabel returns the status word and its color for the
// trace banner. Most outcomes render as green "OK", but the verify
// action stamps Outcome.Value with one of {ok, red, infra} so a
// structural-cycle test failure (red) or an orchestrator-side blow-up
// (infra) shows up as RED / INFRA in the banner. INFRA is yellow rather
// than red because the SUT itself isn't proven broken — the runner
// never got that far.
//
// "ok" still reads as OK but is treated as a known status so
// formatOutcome can suppress the redundant `-> value=ok` suffix that
// would otherwise repeat the same signal.
func outcomeStatusLabel(out statemachine.Outcome) (string, color.Attribute) {
	switch out.Value {
	case "red":
		return "RED", color.FgRed
	case "infra":
		return "INFRA", color.FgYellow
	}
	return "OK", color.FgGreen
}

// kindLabel maps NodeKind to the YAML vocabulary the operator already
// knows from the process-flow document.
func kindLabel(k statemachine.NodeKind) string {
	switch k {
	case statemachine.StartEvent:
		return "start-event"
	case statemachine.EndEvent:
		return "end-event"
	case statemachine.ServiceTask:
		return "service-task"
	case statemachine.UserTask:
		return "user-task"
	case statemachine.Gateway:
		return "gateway"
	case statemachine.CallActivity:
		return "call-activity"
	default:
		return fmt.Sprintf("kind%d", k)
	}
}

// formatOutcome renders the populated Outcome field as `key=value` for
// the exit banner. Returns the empty string when the status word
// already conveys the outcome (verify classes — see outcomeStatusLabel),
// so the caller can drop the "-> ..." suffix entirely. Otherwise empty
// Outcome returns "(no result)".
func formatOutcome(out statemachine.Outcome) string {
	switch out.Value {
	case "ok", "red", "infra":
		// Status word already shows this; "-> value=red" would just be
		// noise. The empty return tells writeExit to skip the suffix.
		return ""
	}
	switch {
	case out.Value != "":
		return fmt.Sprintf("value=%s", out.Value)
	case out.Bool:
		return "bool=true"
	default:
		// Bool false is indistinguishable from "no result" via Outcome alone,
		// but for gateways the wrapGateway decorator records the boolean in
		// ctx.State under the binding name — so the state-delta line will
		// show it. (no result) is the honest label here.
		return "(no result)"
	}
}

// snapshotState copies ctx.State into a new map keyed only by string values
// (plus best-effort string coercion for bool and other types) so we can diff
// it cheaply afterward. We don't deep-copy the values — they're treated as
// opaque strings for delta detection.
func snapshotState(state map[string]any) map[string]string {
	if len(state) == 0 {
		return nil
	}
	out := make(map[string]string, len(state))
	for k, v := range state {
		out[k] = fmt.Sprint(v)
	}
	return out
}

// stateDelta returns a human-readable description of keys whose values
// differ between pre and post (added, modified, or removed). The empty
// string means "no change" so the caller can suppress the line.
//
// Reserved override keys (those starting with `_override_`) are excluded
// because override.Wrap republishes them on every node, which would make
// the delta line noisy without adding signal.
func stateDelta(pre, post map[string]string) string {
	keys := map[string]bool{}
	for k := range pre {
		keys[k] = true
	}
	for k := range post {
		keys[k] = true
	}
	var changed []string
	for k := range keys {
		if strings.HasPrefix(k, "_override_") {
			continue
		}
		if pre[k] != post[k] {
			changed = append(changed, k)
		}
	}
	sort.Strings(changed)
	if len(changed) == 0 {
		return ""
	}
	parts := make([]string, 0, len(changed))
	for _, k := range changed {
		switch {
		case post[k] == "":
			parts = append(parts, fmt.Sprintf("-%s", k))
		case pre[k] == "":
			parts = append(parts, fmt.Sprintf("%s=%s", k, post[k]))
		default:
			parts = append(parts, fmt.Sprintf("%s=%s", k, post[k]))
		}
	}
	return strings.Join(parts, ", ")
}

// formatParams renders the raw params map sorted by key for stable output.
func formatParams(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, params[k]))
	}
	return strings.Join(parts, ",")
}

// snapshotDirty captures the set of working-tree paths that `git status
// --porcelain` reports right now. We only call git for user-task nodes
// because those are the ones an agent might mutate the working tree
// from — service tasks and gateways are pure-Go and have no need for
// the snapshot (skipping them avoids one git shell-out per service node,
// which adds up fast on a full pipeline run).
//
// Failures (e.g. `git` missing, not in a repo) return nil rather than
// propagating: file listing is informational, not load-bearing for
// transitions tests or sandbox dry-runs.
func snapshotDirty(deps Deps, kind statemachine.NodeKind) map[string]bool {
	if kind != statemachine.UserTask {
		return nil
	}
	out, err := deps.Git.Run(context.Background(), deps.RepoPath, "status", "--porcelain")
	if err != nil {
		return nil
	}
	return parseDirty(out)
}

// parseDirty mirrors clauderun.parseDirty: every path mentioned in
// `git status --porcelain` regardless of status code, with rename
// arrows (`R old -> new`) collapsed to the new path. Lines too short
// to hold "XY path" are skipped silently (defensive against trailing
// blank lines).
func parseDirty(porcelain []byte) map[string]bool {
	m := map[string]bool{}
	for line := range strings.SplitSeq(string(porcelain), "\n") {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = path[idx+len(" -> "):]
		}
		if path != "" {
			m[path] = true
		}
	}
	return m
}

// dirtyDelta returns paths present in post but not pre, sorted for
// stable output. The intent is "what did the agent introduce or modify
// during this user-task" — pre-existing dirty paths are excluded so the
// listing isn't polluted by the operator's unrelated work.
func dirtyDelta(pre, post map[string]bool) []string {
	var out []string
	for p := range post {
		if !pre[p] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func (d Deps) tracePrefix() string {
	return d.paint(fmt.Sprintf("[trace %s]", nowFn().Format("15:04:05")), color.Faint)
}

// nodeIDPaint renders the node ID with the kind-appropriate emphasis. On
// a TTY: cyan-bold for call-activity (so process boundaries pop as the
// "phase" markers), plain bold for everything else, plain text off-TTY.
func (d Deps) nodeIDPaint(node statemachine.Node) string {
	if node.Kind == statemachine.CallActivity {
		return d.paint(node.ID, color.FgCyan, color.Bold)
	}
	return d.paint(node.ID, color.Bold)
}

// execGit is the production GitRunner. Mirrors the implementation pattern
// used by clauderun's execGit so behavior is consistent across packages.
type execGit struct{}

func (execGit) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("git %s: %w (stderr: %s)",
				strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return out, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}
