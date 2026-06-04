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

	// Colorize controls whether banners are wrapped in ANSI colour escapes.
	// Callers set it explicitly because Out is often a wrapped writer
	// (io.MultiWriter for --log-file, bytes.Buffer in tests) — the package
	// cannot infer TTY-ness from Out alone in those cases. driver.go computes
	// it from isatty.IsTerminal(os.Stdout.Fd()) before installLogFileMirror
	// swaps Stdout for the multi-writer, so the terminal still gets colour
	// even when the run is being mirrored to a log file (the mirrored file
	// gains the same ANSI bytes; less -R or NO_COLOR=1 strip them, matching
	// the LogFile contract documented on driver.Options). When left at the
	// zero value, withDefaults() upgrades to true if Out is a direct
	// os.Stdout TTY — the convenience for ad-hoc callers and the no-log-file
	// rehearsal path.
	Colorize bool

	// Tree, when non-nil, receives a decolored, indented execution tree
	// written live as each node completes — the readable per-run sibling of
	// the colored Out stream (D3/D5). Both render from the same per-dispatch
	// Event record (D2), so they cannot drift. driver.Run opens
	// <run-ts>/flow.txt and passes a *TreeWriter here; tests and ad-hoc
	// callers leave it nil, in which case no tree is emitted and the live
	// stream behaves exactly as before. The pointer is shared across every
	// wrapped node so the run-scoped depth counter brackets sub-processes.
	Tree *TreeWriter
}

func (d Deps) withDefaults() Deps {
	if d.Out == nil {
		d.Out = os.Stdout
	}
	if d.Git == nil {
		d.Git = execGit{}
	}
	if !d.Colorize {
		if f, ok := d.Out.(*os.File); ok && f == os.Stdout && isatty.IsTerminal(f.Fd()) {
			d.Colorize = true
		}
	}
	return d
}

// paint wraps s in c's ANSI escapes when Colorize is true; otherwise returns
// s unchanged. Per-instance EnableColor avoids touching fatih/color's global
// NoColor flag — tests in other packages that share the process must not see
// our color choice flip on them.
func (d Deps) paint(s string, attrs ...color.Attribute) string {
	if !d.Colorize {
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

// Event is the per-dispatch record both the live colored stream and the
// flow.txt tree renderer consume (D2). wrap() populates it once per node
// fire: the enter-time fields (node metadata, the expanded selector, the
// ctx.Params snapshot, the tree depth/occurrence) before inner() runs, and
// the exit-time fields (outcome, elapsed, state + working-tree deltas)
// after. Neither renderer re-derives any of this from the live Context —
// the record is the single source of truth, so the two views cannot drift.
type Event struct {
	// Node carries kind + Raw metadata for both the enter selector and the
	// exit verdict/name fallbacks.
	Node statemachine.Node

	// Enter-time fields — known before inner() fires.

	// Params is a snapshot of ctx.Params at entry: what this node *receives*
	// from the enclosing call-activity scope. Captured for all node kinds so
	// the tree's `in` line is populated for service-/user-tasks too, not just
	// call-activities (D2). Distinct from the call-activity's `params=` chip
	// (CallParams below), which is what it *pushes* to its sub-process.
	Params map[string]string
	// AgentExpanded is node.Raw.Agent with ${…} resolved against the live
	// scope (user-tasks only; empty otherwise). On strict-mode expansion
	// error the raw template is kept — the dispatcher surfaces the error
	// authoritatively, trace must not block on it.
	AgentExpanded string
	// CallParams is node.Raw.Params template-expanded against the live scope
	// (call-activities only): the values pushed onto the sub-process, shown
	// as the `params=` chip on the enter line.
	CallParams map[string]string
	// Depth is the sub-process nesting level for tree indentation (0 at the
	// run root). Occurrence is the 1-based fire count of this node id within
	// its enclosing scope-instance; >1 marks a loop-back re-dispatch the tree
	// annotates as `↻ retry N`. Both are zero unless a TreeWriter is wired.
	Depth      int
	Occurrence int

	// Exit-time fields — filled after inner() returns.

	Outcome   statemachine.Outcome
	Elapsed   time.Duration
	PreState  map[string]string
	PostState map[string]string
	PreDirty  map[string]bool
	PostDirty map[string]bool
}

// wrap returns a NodeFn that records an Event around inner and renders it to
// both the live colored stream and (when wired) the flow.txt tree. The
// closure captures the original Node (kind + raw) so it has the metadata it
// needs to render without re-querying the engine. For user-task nodes the
// wrapper also snapshots the working tree (via `git status --porcelain`) on
// each side of the dispatch so the exit record can list what the agent
// changed.
//
// For call-activity nodes the wrapper brackets inner() with TreeWriter
// push/pop so the sub-process's children — which run synchronously inside
// the call-activity NodeFn (statemachine.wrapCallActivity calls runProcess
// inline) — record one greater depth and nest under this node in the tree.
func wrap(node statemachine.Node, deps Deps) statemachine.NodeFn {
	inner := node.Fn
	return func(ctx *statemachine.Context) statemachine.Outcome {
		ev := &Event{
			Node:          node,
			Params:        snapshotParams(ctx.Params),
			AgentExpanded: expandedAgent(node, ctx),
			CallParams:    callParams(node, ctx),
		}
		if deps.Tree != nil {
			ev.Depth, ev.Occurrence = deps.Tree.enter(node.ID)
		}
		writeEnter(deps, ev)
		if deps.Tree != nil {
			deps.Tree.writeEnter(ev)
		}
		ev.PreState = snapshotState(ctx.State)
		ev.PreDirty = snapshotDirty(deps, node.Kind)
		if node.Kind == statemachine.CallActivity && deps.Tree != nil {
			deps.Tree.push()
		}
		started := nowFn()
		out := inner(ctx)
		ev.Elapsed = nowFn().Sub(started).Round(time.Millisecond)
		if node.Kind == statemachine.CallActivity && deps.Tree != nil {
			deps.Tree.pop()
		}
		ev.PostState = snapshotState(ctx.State)
		ev.PostDirty = snapshotDirty(deps, node.Kind)
		ev.Outcome = out
		writeExit(deps, ev)
		if deps.Tree != nil {
			deps.Tree.writeExit(ev)
		}
		return out
	}
}

// snapshotParams copies ctx.Params so the `in` line stays stable even after
// a call-activity push mutates the live map. Returns nil for an empty scope
// so the renderers can cheaply test "no params".
func snapshotParams(params map[string]string) map[string]string {
	if len(params) == 0 {
		return nil
	}
	out := make(map[string]string, len(params))
	for k, v := range params {
		out[k] = v
	}
	return out
}

// expandedAgent returns node.Raw.Agent with ${…} resolved against the live
// scope for user-task nodes, the empty string otherwise. On strict-mode
// expansion error the raw template is kept (same contract as the dispatcher's
// own about-to-fire expansion, which surfaces the error authoritatively).
func expandedAgent(node statemachine.Node, ctx *statemachine.Context) string {
	if node.Kind != statemachine.UserTask || node.Raw.Agent == "" {
		return ""
	}
	expanded, err := statemachine.ExpandParams(node.Raw.Agent, ctx.Params, ctx.State)
	if err != nil {
		return node.Raw.Agent
	}
	return expanded
}

// callParams returns node.Raw.Params expanded against the live scope for
// call-activity nodes (nil otherwise) — the values pushed onto the
// sub-process, rendered as the `params=` chip.
func callParams(node statemachine.Node, ctx *statemachine.Context) map[string]string {
	if node.Kind != statemachine.CallActivity || len(node.Raw.Params) == 0 {
		return nil
	}
	return expandParamValues(node.Raw.Params, ctx)
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
//
// Renders from the Event's enter-time fields (D2): the expanded agent name
// (ev.AgentExpanded) and the pushed call-activity params (ev.CallParams) are
// pre-resolved by wrap() against the live scope, so this stays a pure
// formatter.
func writeEnter(deps Deps, ev *Event) {
	node := ev.Node
	fmt.Fprintf(deps.Out, "%s %s %s  %s\n",
		deps.tracePrefix(),
		deps.paint(">", color.FgCyan),
		deps.nodeIDPaint(node),
		strings.Join(enterParts(ev), " "))
}

// enterParts builds the `kind=… <selector>=…` field list shared by the live
// banner and the tree's enter line. Pure function of the Event so both views
// agree on the selector vocabulary.
func enterParts(ev *Event) []string {
	node := ev.Node
	parts := []string{fmt.Sprintf("kind=%s", kindLabel(node.Kind))}
	switch node.Kind {
	case statemachine.ServiceTask:
		if node.Raw.Action != "" {
			parts = append(parts, fmt.Sprintf("action=%s", node.Raw.Action))
		}
	case statemachine.UserTask:
		if node.Raw.Agent != "" {
			parts = append(parts, fmt.Sprintf("agent=%s", ev.AgentExpanded))
		}
	case statemachine.Gateway:
		if node.Raw.Binding != "" {
			parts = append(parts, fmt.Sprintf("binding=%s", node.Raw.Binding))
		}
	case statemachine.CallActivity:
		if node.Raw.Process != "" {
			parts = append(parts, fmt.Sprintf("process=%s", node.Raw.Process))
		}
		if len(ev.CallParams) > 0 {
			parts = append(parts, fmt.Sprintf("params=%s", formatParams(ev.CallParams)))
		}
	}
	return parts
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
// When the hoist doesn't fire because there is no delta — a gateway
// re-affirms an existing binding value, so pre and post agree — a
// gateway-specific fallback substitutes `bool=false` for `(no result)`.
// wrapGateway guarantees gateways return a meaningful bool, so the
// `(no result)` label is never honest for that kind; without the
// fallback the line would contradict the symmetric `bool=true` case and
// hide the gate's actual decision.
//
// On Outcome.Err the first line becomes:
//
//	[trace HH:MM:SS] FAIL NODE_ID -> <error>  (<elapsed>)
//
// and no follow-on lines are emitted (the engine halts the run anyway).
func writeExit(deps Deps, ev *Event) {
	w := deps.Out
	node := ev.Node
	if ev.Outcome.Err != nil {
		fmt.Fprintf(w, "%s %s %s -> %v  (%s)\n",
			deps.tracePrefix(),
			deps.paint("FAIL", color.FgRed),
			deps.nodeIDPaint(node),
			ev.Outcome.Err, ev.Elapsed)
		return
	}
	if node.ID == "TESTS_INFRA_HALT" {
		writeInfraHaltBanner(deps, node, ev.PostState, ev.Elapsed)
		return
	}
	label, attr, detail, delta, hoisted := outcomeDetail(ev)
	if detail != "" {
		fmt.Fprintf(w, "%s %s %s -> %s  (%s)\n",
			deps.tracePrefix(),
			deps.paint(label, attr),
			deps.nodeIDPaint(node),
			detail, ev.Elapsed)
	} else {
		fmt.Fprintf(w, "%s %s %s  (%s)\n",
			deps.tracePrefix(),
			deps.paint(label, attr),
			deps.nodeIDPaint(node),
			ev.Elapsed)
	}
	if delta != "" && !hoisted {
		fmt.Fprintf(w, "%s    %s %s\n", deps.tracePrefix(), deps.paint("state:", color.Faint), delta)
	}
	if node.Kind == statemachine.UserTask {
		if files := dirtyDelta(ev.PreDirty, ev.PostDirty); len(files) > 0 {
			fmt.Fprintf(w, "%s    %s %s\n", deps.tracePrefix(), deps.paint("files:", color.Faint), strings.Join(files, ", "))
		}
	}
}

// outcomeDetail computes the semantic exit detail for an Event — the `-> X`
// payload — applying the hoist / gateway-bool / end-event-name /
// call-activity-verdict rules in one place so the live banner (writeExit)
// and the flow.txt tree agree on what an exit *means* even though they lay
// it out differently (D2). Returns the status label + colour, the detail
// string, the raw state delta, and whether that delta was folded into the
// detail (so the caller can suppress a separate `state:` line).
//
//   - detail "" means the status word alone conveys the outcome (verify
//     classes ok/red/infra — see formatOutcome); the caller drops the
//     `-> …` suffix.
//   - A gateway whose Bool:false left no delta to hoist renders `bool=false`
//     explicitly, because gateways are contractually bool-valued
//     (wrapGateway) and `(no result)` would be misleading.
//   - end-/error-end-events surface their YAML `name:` in place of the
//     content-free `(no result)` placeholder.
//   - call-activities lead with a derived `verdict=` chip (the cycle's
//     test-outcome × expected-test-result classification), prepended ahead
//     of any hoisted delta so intent reads before mechanics.
func outcomeDetail(ev *Event) (label string, attr color.Attribute, detail, delta string, hoisted bool) {
	node := ev.Node
	label, attr = outcomeStatusLabel(ev.Outcome)
	detail = formatOutcome(ev.Outcome)
	delta = stateDelta(ev.PreState, ev.PostState)
	hoisted = detail == "(no result)" && delta != ""
	if hoisted {
		detail = delta
	}
	if !hoisted && detail == "(no result)" && node.Kind == statemachine.Gateway {
		detail = "bool=false"
	}
	if detail == "(no result)" && (node.Kind == statemachine.EndEvent || node.Kind == statemachine.ErrorEndEvent) && node.Raw.Name != "" {
		detail = fmt.Sprintf("%q", node.Raw.Name)
	}
	if node.Kind == statemachine.CallActivity {
		if v := callActivityVerdict(ev.PostState); v != "" {
			if detail == "(no result)" {
				detail = "verdict=" + v
			} else {
				detail = "verdict=" + v + ", " + detail
			}
		}
	}
	return label, attr, detail, delta, hoisted
}

// writeInfraHaltBanner replaces the generic exit line for the
// TESTS_INFRA_HALT error-end-event with a human-readable halt banner
// that quotes the infra label, the failing command, and the stderr
// tail — the three pieces of evidence the operator needs to diagnose
// "the runner could not start" without reading the raw state dump.
//
// The values come from the per-node post-state snapshot, which is the
// shared ctx.State at the moment TESTS_INFRA_HALT fired:
//
//   - test-infra-label  set by runCommand on infra classification
//                       (see internal/atdd/runtime/actions/bindings.go)
//   - command-line      set by runCommand on the failing test-run shell-out
//   - command-stderr-tail  ditto; the last N lines of the runner's stderr
//
// Empty fields render as "(unset)" rather than being skipped so contract
// drift surfaces visibly. Halt colour is yellow (matching the INFRA
// label in outcomeStatusLabel) because an infra failure does not prove
// the SUT broken — the runner never got that far.
func writeInfraHaltBanner(deps Deps, node statemachine.Node, post map[string]string, elapsed time.Duration) {
	w := deps.Out
	label := post["test-infra-label"]
	if label == "" {
		label = "(unset)"
	}
	cmd := post["command-line"]
	if cmd == "" {
		cmd = "(unset)"
	}
	tail := post["command-stderr-tail"]
	if tail == "" {
		tail = "(unset)"
	}
	fmt.Fprintf(w, "%s %s %s — infra failure: %s  (%s)\n",
		deps.tracePrefix(),
		deps.paint("HALT", color.FgYellow, color.Bold),
		deps.nodeIDPaint(node),
		label, elapsed)
	fmt.Fprintf(w, "%s    %s %s\n", deps.tracePrefix(), deps.paint("command:", color.Faint), cmd)
	// Indent multi-line stderr tail so subsequent lines align with the
	// first line of stderr, not with the trace prefix — keeps the visual
	// "this is one chunk" grouping intact on multi-line tails.
	stderrLines := strings.Split(tail, "\n")
	fmt.Fprintf(w, "%s    %s %s\n", deps.tracePrefix(), deps.paint("stderr tail:", color.Faint), stderrLines[0])
	for _, line := range stderrLines[1:] {
		fmt.Fprintf(w, "%s                 %s\n", deps.tracePrefix(), line)
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

// callActivityVerdict classifies the sub-process's terminal test state
// against the call-site's expectation, returning the short label that the
// trace exit line surfaces as a leading `verdict=` chip. The chip lets the
// operator read the cycle's *intent* — "this red wrapper expected fail and
// got fail" — rather than re-deriving it from `expected-test-result=… +
// test-outcome=…` buried in the comma-separated state delta.
//
// Returns the empty string when the sub-process is not a test-bearing
// phase (neither field set, e.g. refine-ticket / refactor / commit
// sub-processes). The caller treats empty as "omit the chip" so we don't
// litter every non-test call-activity exit with `verdict=n/a`.
//
// `infra` short-circuits the expectation comparison: an infra-classified
// failure means the runner could not start, so the cycle never reached
// the state where matching expectations is meaningful — surface the
// classification directly.
func callActivityVerdict(post map[string]string) string {
	outcome := post["test-outcome"]
	if outcome == "infra" {
		return "infra"
	}
	expected := post["expected-test-result"]
	if outcome == "" || expected == "" {
		return ""
	}
	switch {
	case expected == "success" && outcome == "pass":
		return "green-as-expected"
	case expected == "failure" && outcome == "fail":
		return "red-as-expected"
	case expected == "success" && outcome == "fail":
		return "unexpected-fail"
	case expected == "failure" && outcome == "pass":
		return "unexpected-pass"
	}
	return ""
}

// kindLabel maps NodeKind to the YAML vocabulary the operator already
// knows from the process-flow document.
func kindLabel(k statemachine.NodeKind) string {
	switch k {
	case statemachine.StartEvent:
		return "start-event"
	case statemachine.EndEvent:
		return "end-event"
	case statemachine.ErrorEndEvent:
		return "error-end-event"
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
		// Bool false is indistinguishable from "no result" via Outcome alone.
		// For gateways, writeExit substitutes "bool=false" because wrapGateway
		// guarantees gateways return a meaningful bool. For other node kinds,
		// "(no result)" is the honest label — they may legitimately return
		// Outcome{} (and any state they wrote will be hoisted by writeExit).
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
//
// Entries sort by post-value length ascending (alphabetical tiebreaker)
// so short scalars render first and long blobs — e.g. phase-changed-files'
// newline-joined path list — trail at the end of the line. Reading from
// left to right then matches a "primitive → composite" gradient instead
// of being broken up by a multi-line value sandwiched between two
// scalars.
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
	sort.SliceStable(changed, func(i, j int) bool {
		li, lj := len(post[changed[i]]), len(post[changed[j]])
		if li != lj {
			return li < lj
		}
		return changed[i] < changed[j]
	})
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

// expandParamValues returns a copy of raw with each value template-expanded
// against ctx.Params + ctx.State, mirroring the substitution wrapCallActivity
// will apply when pushing these params onto the sub-process scope (run.go
// `for k, v := range raw.Params { ... ExpandParams(v, prev, ctx.State) ... }`).
// We expand here so the trace banner shows the operator what value the called
// sub-process actually sees — without this, `question=Do you approve fix to
// attempt remediation for ${failure-kind} ?` leaks the literal placeholder
// into the trace even though the downstream ASK_HUMAN prompt resolves it
// correctly.
//
// On strict-mode expansion error the raw template is kept — the about-to-fire
// wrapCallActivity will surface the same error authoritatively. Trace must
// not block on diagnostic output (same contract as the UserTask agent branch
// in writeEnter).
func expandParamValues(raw map[string]string, ctx *statemachine.Context) map[string]string {
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		expanded, err := statemachine.ExpandParams(v, ctx.Params, ctx.State)
		if err != nil {
			out[k] = v
			continue
		}
		out[k] = expanded
	}
	return out
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
