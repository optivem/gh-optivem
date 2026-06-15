package trace

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// TreeWriter renders the human-readable execution tree to flow.txt — the
// decolored, indented sibling of the live colored stream (D3/D5). It is fed
// the same per-dispatch Event records the live banner consumes (D2): wrap()
// calls enter/push/pop to maintain run-scoped depth, then writeEnter /
// writeExit to emit each node's lines as it fires, so a halted run still
// leaves the partial tree on disk.
//
// Nesting works because sub-processes run synchronously inside their
// call-activity NodeFn (statemachine.wrapCallActivity calls runProcess
// inline): wrap() pushes before inner() and pops after, so every child node
// records depth+1 and indents under its parent. Each push also opens a fresh
// scope-instance id, so a node id that recurs in two distinct sub-process
// invocations is not mistaken for a loop-back; only a re-fire within the
// same scope instance (Occurrence > 1) is annotated `↻ retry N`.
//
// All methods lock t.mu. The engine is single-threaded today, but the writer
// is shared across every wrapped node and nothing structurally rules out a
// future concurrent walk — the lock keeps depth, the scope stack, and the
// interleaved writes consistent if that ever changes (same defensive posture
// as driver.runState).
//
// flow.txt is an *os.File; Go writes it straight through to the OS without
// userspace buffering, so each Fprintln is durable the moment it returns —
// no explicit per-step flush is needed for a halt to leave a usable file.
type TreeWriter struct {
	w  io.Writer
	mu sync.Mutex

	// depth is the current sub-process nesting level; scopeStack holds the
	// id of each enclosing scope-instance (top = current), assigned from the
	// monotonic nextScope on push. visits counts node fires keyed by
	// "<scope>:<nodeID>" so a re-fire within one scope instance reads as a
	// retry. steps is the running total of node exits, surfaced in the footer.
	depth      int
	scopeStack []int
	nextScope  int
	visits     map[string]int
	steps      int
}

// NewTreeWriter returns a TreeWriter that emits to w (typically the open
// <run-ts>/flow.txt file). The caller owns w's lifecycle (open + close).
func NewTreeWriter(w io.Writer) *TreeWriter {
	return &TreeWriter{w: w, visits: map[string]int{}}
}

// enter records a node fire and returns the depth + 1-based occurrence the
// Event should carry. Called by wrap() before inner() (and before any
// call-activity push), so the node is attributed to its own enclosing scope.
func (t *TreeWriter) enter(nodeID string) (depth, occurrence int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	scope := 0
	if n := len(t.scopeStack); n > 0 {
		scope = t.scopeStack[n-1]
	}
	key := strconv.Itoa(scope) + ":" + nodeID
	t.visits[key]++
	return t.depth, t.visits[key]
}

// push opens a new sub-process scope: children fired before the matching pop
// record one greater depth and key their occurrence counts under a fresh
// scope-instance id. Called by wrap() for call-activity nodes immediately
// before inner() runs the sub-process inline.
func (t *TreeWriter) push() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.depth++
	t.nextScope++
	t.scopeStack = append(t.scopeStack, t.nextScope)
}

// pop closes the most recent scope opened by push. Called by wrap() after the
// call-activity's inner() returns. Defensive against underflow so a stray pop
// can never panic the run.
func (t *TreeWriter) pop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.depth > 0 {
		t.depth--
	}
	if n := len(t.scopeStack); n > 0 {
		t.scopeStack = t.scopeStack[:n-1]
	}
}

// writeEnter emits the node's header line (indented by ev.Depth) plus an `in`
// line carrying the ctx.Params it received, when any. The selector vocabulary
// (`kind=… action=… / agent=… / process=… params=…`) is the same enterParts
// the live banner uses, so the two views name things identically.
func (t *TreeWriter) writeEnter(ev *Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	indent := indentFor(ev.Depth)
	line := fmt.Sprintf("%s> %s  %s", indent, ev.Node.ID, strings.Join(enterParts(ev), " "))
	if ev.Occurrence > 1 {
		line += fmt.Sprintf("  ↻ retry %d", ev.Occurrence)
	}
	fmt.Fprintln(t.w, line)
	if len(ev.Params) > 0 {
		fmt.Fprintf(t.w, "%s  in: %s\n", indent, formatParams(ev.Params))
	}
}

// writeExit emits the node's `out` line and any leaf pointers — the agent
// name (user-tasks), the classified command line (service-tasks that shelled
// out), and the working-tree files an agent touched. It does NOT inline
// command stdout or agent response text (D4): those live in the prompt-log /
// events files the footer points to. The one inline exception is the
// infra-halt stderr tail, kept because that is the moment the output is
// needed in place.
func (t *TreeWriter) writeExit(ev *Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.steps++
	indent := indentFor(ev.Depth)
	node := ev.Node
	switch {
	case ev.Outcome.Err != nil:
		fmt.Fprintf(t.w, "%s  out: ERROR -> %v  (%s)\n", indent, ev.Outcome.Err, ev.Elapsed)
	case node.ID == "TESTS_INFRA_HALT":
		t.writeInfraHalt(ev, indent)
	default:
		label, _, detail, _, _ := outcomeDetail(ev)
		if detail != "" {
			fmt.Fprintf(t.w, "%s  out: %s %s  (%s)\n", indent, label, detail, ev.Elapsed)
		} else {
			fmt.Fprintf(t.w, "%s  out: %s  (%s)\n", indent, label, ev.Elapsed)
		}
	}
	if node.Kind == statemachine.UserTask && ev.AgentExpanded != "" {
		fmt.Fprintf(t.w, "%s  agent: %s\n", indent, ev.AgentExpanded)
	}
	if cmd := ev.PostState["command-line"]; cmd != "" {
		if cls := commandClass(ev); cls != "" {
			fmt.Fprintf(t.w, "%s  cmd: %s  [%s]\n", indent, cmd, cls)
		} else {
			fmt.Fprintf(t.w, "%s  cmd: %s\n", indent, cmd)
		}
	}
	if node.Kind == statemachine.UserTask {
		if files := dirtyDelta(ev.PreDirty, ev.PostDirty); len(files) > 0 {
			fmt.Fprintf(t.w, "%s  files: %s\n", indent, strings.Join(files, ", "))
		}
	}
}

// writeInfraHalt renders the TESTS_INFRA_HALT node's diagnostic payload
// inline — the infra label, failing command, and stderr tail — the D4
// exception to "pointers only". Unset fields render as "(unset)" so contract
// drift surfaces visibly rather than as a blank line. Mirrors the live
// banner's writeInfraHaltBanner content, decolored.
func (t *TreeWriter) writeInfraHalt(ev *Event, indent string) {
	label := orUnset(ev.PostState["test-infra-label"])
	cmd := orUnset(ev.PostState["command-line"])
	tail := orUnset(ev.PostState["command-stderr-tail"])
	fmt.Fprintf(t.w, "%s  out: HALT — infra failure: %s  (%s)\n", indent, label, ev.Elapsed)
	fmt.Fprintf(t.w, "%s  command: %s\n", indent, cmd)
	lines := strings.Split(tail, "\n")
	fmt.Fprintf(t.w, "%s  stderr tail: %s\n", indent, lines[0])
	for _, l := range lines[1:] {
		fmt.Fprintf(t.w, "%s               %s\n", indent, l)
	}
}

// TreeHeader is the run metadata emitted at the top of flow.txt before the
// first node fires.
type TreeHeader struct {
	RunTimestamp string
	RepoPath     string
	Process      string
	IssueNum     int
}

// WriteHeader emits the run-metadata header. Called once at driver startup,
// right after the file is opened.
func (t *TreeWriter) WriteHeader(h TreeHeader) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Fprintf(t.w, "=== execution flow: %s ===\n", h.RunTimestamp)
	if h.IssueNum > 0 {
		fmt.Fprintf(t.w, "issue:   #%d\n", h.IssueNum)
	}
	if h.RepoPath != "" {
		fmt.Fprintf(t.w, "repo:    %s\n", h.RepoPath)
	}
	if h.Process != "" {
		fmt.Fprintf(t.w, "process: %s\n", h.Process)
	}
	fmt.Fprintln(t.w)
}

// TreeFooter is the at-a-glance run summary emitted at the bottom of flow.txt.
// Dispatches is supplied by the driver from runState's recorded dispatches —
// the same tally printAgentSummary renders — so the footer count cannot
// diverge from the agent summary (Item 3). Steps comes from the writer's own
// per-exit counter (a quantity printAgentSummary does not track, so no
// divergence is possible). RunDir + LogFile point the operator at the
// per-dispatch *.prompt.md and the colored --log-file for the detail this
// tree deliberately omits (D4).
type TreeFooter struct {
	Result     error
	WallClock  time.Duration
	CommitSHA  string
	Dispatches int
	RunDir     string
	LogFile    string
}

// WriteFooter emits the result/wall-clock/commit/counts/pointers footer.
// Called from the driver's deferred run-end tail so it fires on success and
// on any halt, after the partial tree is already on disk.
func (t *TreeWriter) WriteFooter(f TreeFooter) {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := "ok"
	if f.Result != nil {
		result = "halted — " + f.Result.Error()
	}
	fmt.Fprintln(t.w)
	fmt.Fprintf(t.w, "=== result: %s ===\n", result)
	fmt.Fprintf(t.w, "wall-clock: %s\n", f.WallClock.Round(time.Millisecond))
	if f.CommitSHA != "" {
		fmt.Fprintf(t.w, "commit:     %s\n", f.CommitSHA)
	}
	fmt.Fprintf(t.w, "steps:      %d\n", t.steps)
	fmt.Fprintf(t.w, "dispatches: %d\n", f.Dispatches)
	if f.RunDir != "" {
		fmt.Fprintf(t.w, "prompt logs: %s\n", filepath.Join(f.RunDir, "*.prompt.md"))
	}
	if f.LogFile != "" {
		fmt.Fprintf(t.w, "log file:   %s\n", f.LogFile)
	}
}

// commandClass classifies a service-task's shell-out result as PASS / RED /
// INFRA from the outcome + state runCommand already recorded (Item 3) — so
// the `cmd` line carries the verdict without the operator re-deriving it. The
// verify-action Outcome.Value wins when present; otherwise the test-outcome /
// test-infra-label / command-succeeded state keys (set by
// actions.runCommand) are consulted. Returns "" when none apply so the caller
// emits a bare `cmd` line.
func commandClass(ev *Event) string {
	switch ev.Outcome.Value {
	case "infra":
		return "INFRA"
	case "red":
		return "RED"
	}
	switch ev.PostState["test-outcome"] {
	case "infra":
		return "INFRA"
	case "fail":
		return "RED"
	case "pass":
		return "PASS"
	}
	if ev.PostState["test-infra-label"] != "" {
		return "INFRA"
	}
	switch ev.PostState["command-succeeded"] {
	case "true":
		return "PASS"
	case "false":
		return "RED"
	}
	return ""
}

// indentFor returns the two-spaces-per-level indent for a tree depth.
func indentFor(depth int) string {
	return strings.Repeat("  ", depth)
}

// orUnset renders the empty string as the explicit "(unset)" sentinel so a
// missing diagnostic field surfaces visibly in the infra-halt block.
func orUnset(s string) string {
	if s == "" {
		return "(unset)"
	}
	return s
}
