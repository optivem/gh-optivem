// Package trace adds a per-node logging decorator to the engine, producing
// a chronological audit trail of every step the ATDD pipeline takes:
//
//   - service_task: action name, ctx.State keys mutated, Outcome.Value/Bool
//   - gateway:      binding name, evaluated value, ctx.State keys mutated
//   - user_task:    agent name, Outcome.Commit, files in that commit
//   - call_activity:flow name, params pushed
//   - start/end:    just the node id and kind
//
// The decorator sits at the outermost layer (after override.Wrap), so what
// it logs is exactly what the engine's RunFlow loop dispatches. Output is
// plain text with `[trace HH:MM:SS]` prefixes so the trace stream is easy
// to grep alongside clauderun's existing colored agent banners.
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
}

func (d Deps) withDefaults() Deps {
	if d.Out == nil {
		d.Out = os.Stdout
	}
	if d.Git == nil {
		d.Git = execGit{}
	}
	return d
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
	for _, flow := range eng.Flows {
		for id, node := range flow.Nodes {
			node.Fn = wrap(node, deps)
			flow.Nodes[id] = node
		}
	}
}

// wrap returns a NodeFn that logs entry/exit around inner. The closure
// captures the original Node (kind + raw) so it has the metadata it needs
// to render the entry banner without re-querying the engine.
func wrap(node statemachine.Node, deps Deps) statemachine.NodeFn {
	inner := node.Fn
	return func(ctx *statemachine.Context) statemachine.Outcome {
		writeEnter(deps.Out, node, ctx)
		preState := snapshotState(ctx.State)
		started := nowFn()
		out := inner(ctx)
		elapsed := nowFn().Sub(started).Round(time.Millisecond)
		postState := snapshotState(ctx.State)
		writeExit(deps, node, out, elapsed, preState, postState)
		return out
	}
}

// writeEnter prints the per-node entry banner. The format is:
//
//	[trace HH:MM:SS] > NODE_ID  kind=<kind> <selector>=<name>
//
// where <selector> is action / agent / binding / flow depending on Kind.
// Templated fields (e.g. ${agent} on structural_cycle nodes) are expanded
// against ctx.Params so the operator sees the substituted name rather
// than the literal placeholder.
func writeEnter(out io.Writer, node statemachine.Node, ctx *statemachine.Context) {
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
				statemachine.ExpandParams(node.Raw.Agent, ctx.Params)))
		}
	case statemachine.Gateway:
		if node.Raw.Binding != "" {
			parts = append(parts, fmt.Sprintf("binding=%s", node.Raw.Binding))
		}
	case statemachine.CallActivity:
		if node.Raw.Flow != "" {
			parts = append(parts, fmt.Sprintf("flow=%s", node.Raw.Flow))
		}
		if len(node.Raw.Params) > 0 {
			parts = append(parts, fmt.Sprintf("params=%s", formatParams(node.Raw.Params)))
		}
	}
	fmt.Fprintf(out, "%s > %s  %s\n", tracePrefix(), node.ID, strings.Join(parts, " "))
}

// writeExit prints the per-node exit banner and any follow-on detail
// (state-delta keys, files-in-commit). The format is:
//
//	[trace HH:MM:SS] OK NODE_ID -> <outcome>  (<elapsed>)
//	[trace HH:MM:SS]    state: key=value, …
//	[trace HH:MM:SS]    files: path, …
//
// On Outcome.Err the first line becomes:
//
//	[trace HH:MM:SS] FAIL NODE_ID -> <error>  (<elapsed>)
//
// and no follow-on lines are emitted (the engine halts the run anyway).
func writeExit(deps Deps, node statemachine.Node, out statemachine.Outcome, elapsed time.Duration, pre, post map[string]string) {
	w := deps.Out
	if out.Err != nil {
		fmt.Fprintf(w, "%s FAIL %s -> %v  (%s)\n", tracePrefix(), node.ID, out.Err, elapsed)
		return
	}
	fmt.Fprintf(w, "%s OK %s -> %s  (%s)\n", tracePrefix(), node.ID, formatOutcome(out), elapsed)
	if delta := stateDelta(pre, post); delta != "" {
		fmt.Fprintf(w, "%s    state: %s\n", tracePrefix(), delta)
	}
	if node.Kind == statemachine.UserTask && out.Commit != "" {
		paths, err := filesInCommit(deps, out.Commit)
		switch {
		case err != nil:
			fmt.Fprintf(w, "%s    files: (lookup failed: %v)\n", tracePrefix(), err)
		case len(paths) > 0:
			fmt.Fprintf(w, "%s    files: %s\n", tracePrefix(), strings.Join(paths, ", "))
		}
	}
}

// kindLabel maps NodeKind to the YAML vocabulary the operator already
// knows from the process-flow document.
func kindLabel(k statemachine.NodeKind) string {
	switch k {
	case statemachine.StartEvent:
		return "start_event"
	case statemachine.EndEvent:
		return "end_event"
	case statemachine.ServiceTask:
		return "service_task"
	case statemachine.UserTask:
		return "user_task"
	case statemachine.Gateway:
		return "gateway"
	case statemachine.CallActivity:
		return "call_activity"
	default:
		return fmt.Sprintf("kind%d", k)
	}
}

// formatOutcome renders the populated Outcome field as `key=value` for
// the exit banner. Empty Outcome returns "(no result)".
func formatOutcome(out statemachine.Outcome) string {
	switch {
	case out.Commit != "":
		return fmt.Sprintf("commit=%s", shortSHA(out.Commit))
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

// filesInCommit returns the paths touched by `git show --name-only --format= sha`.
// Returns an empty slice (not an error) when the SHA is empty so callers can
// uniformly skip the "files:" line.
func filesInCommit(deps Deps, sha string) ([]string, error) {
	if sha == "" {
		return nil, nil
	}
	out, err := deps.Git.Run(context.Background(), deps.RepoPath, "show", "--name-only", "--format=", sha)
	if err != nil {
		return nil, err
	}
	var paths []string
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

func tracePrefix() string {
	return fmt.Sprintf("[trace %s]", nowFn().Format("15:04:05"))
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
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
