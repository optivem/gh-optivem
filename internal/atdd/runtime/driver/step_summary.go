// step_summary.go owns the end-of-run step-execution summary — the sibling
// of the agent-summary table that spans BOTH halves of a run: agent
// dispatches AND shell commands. The agent summary answers "what did each
// agent cost?"; the step summary answers "which BPMN steps ran, in what
// order, and where did the wall-clock go?".
//
// One stepRecord is appended per executed MID-level atomic step:
//
//   - agent steps   — recorded at dispatch time in newClaudeRunDispatcher
//     (the LOW execute-agent RUN_AGENT fire), named by the MID task-name.
//   - command steps — recorded by wrapStepRecorders (driver.go), which wraps
//     every LOW execute-command run-command service task, named by the
//     resolved command line.
//
// Both append in execution order to runState.steps (the engine walks the BPMN
// graph single-threaded, so append order IS execution order). A fix loop or a
// max-visits retry that re-runs a step appends another record, so repeats show
// as their own rows — matching the "one row per execution" contract.
//
// Durability mirrors the agent sidecar: each step is also appended to
// steps.jsonl as it completes, so a binary crash mid-run still leaves every
// completed step on disk for `gh optivem run summary` to replay.
package driver

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// stepKind classifies one executed BPMN step. Only the two kinds that do real
// work and carry a wall-clock cost are tracked — agent dispatches and shell
// commands. Human-approval gates are sub-steps of execute-agent /
// execute-command (every agent and command is fronted by one), so surfacing
// them here would pair a noise row with every real step; they are deliberately
// omitted.
type stepKind string

const (
	stepKindAgent   stepKind = "agent"
	stepKindCommand stepKind = "command"
)

// stepRecord is one row in the step-execution summary: a MID-level atomic step
// that actually ran, its kind, how long it took, and whether it failed. err is
// set when the step's NodeFn returned an Outcome with a non-nil Err so the
// table can mark the row with a "✗" prefix, mirroring renderAgentSummary.
type stepRecord struct {
	name    string
	kind    stepKind
	elapsed time.Duration
	err     error
}

// appendStep records one completed step. Safe with nil rs (test fixtures that
// bypass the driver-managed runState), mirroring appendRecord. Guarded by the
// same mutex as records/result.
func (rs *runState) appendStep(s stepRecord) {
	if rs == nil {
		return
	}
	rs.mu.Lock()
	rs.steps = append(rs.steps, s)
	rs.mu.Unlock()
}

// snapshotSteps returns a copy of the recorded steps so the summary printer
// can iterate without holding the mutex. Safe with nil rs (returns nil).
func (rs *runState) snapshotSteps() []stepRecord {
	if rs == nil {
		return nil
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	out := make([]stepRecord, len(rs.steps))
	copy(out, rs.steps)
	return out
}

// wallClock returns the elapsed time since Run started — the headline total
// for the step summary. Zero when rs is nil or started was never stamped
// (test fixtures), so the renderer falls back to the sum-of-steps total.
func (rs *runState) wallClock() time.Duration {
	if rs == nil || rs.started.IsZero() {
		return 0
	}
	return nowFn().Sub(rs.started)
}

// stepsPath returns the absolute path to this run's step sidecar, beside the
// agent summary.jsonl. Empty string when rs is nil; appendStepLine treats an
// empty path as "skip the sidecar", same contract as summaryPath.
func (rs *runState) stepsPath() string {
	if rs == nil {
		return ""
	}
	return filepath.Join(rs.repoPath, ".gh-optivem", "runs", rs.runTimestamp, "steps.jsonl")
}

// recordStep is the single entry point both record sites (agent dispatcher and
// command wrapper) call: append to the in-memory list AND mirror to the
// sidecar. Best-effort on the sidecar — a write failure warns to stderr and
// never blocks the step, mirroring appendSummaryLine's stance.
func (rs *runState) recordStep(s stepRecord, stderr io.Writer) {
	rs.appendStep(s)
	if err := appendStepLine(rs.stepsPath(), s); err != nil && stderr != nil {
		fmt.Fprintf(stderr, "driver: warning: append step sidecar: %v\n", err)
	}
}

// printStepSummary writes rs's recorded steps via the package renderer. No-op
// when rs is nil or has no recorded steps. Called from Run's deferred tail so
// it fires on success AND on any error path, mirroring printAgentSummary.
func (rs *runState) printStepSummary(w io.Writer) {
	renderStepSummary(w, rs.snapshotSteps(), rs.wallClock())
}

// renderStepSummary writes a per-step table + totals to w. One row per
// executed step, in execution order. Columns:
//
//	#  step  kind  elapsed
//
// The totals row carries the sum of per-step elapsed times in the elapsed
// column (mirroring renderAgentSummary). When wallClock > 0 a final line gives
// it as the headline total — wall-clock and the step-time sum differ because
// gateways, state plumbing, and inter-step overhead fall outside any step.
// No-op when w is nil or there are no steps.
//
// Single source of truth for the table shape — both the live banner
// (printStepSummary) and the historical replay (PrintStepSummaryFile) route
// through here so the two views stay byte-identical.
func renderStepSummary(w io.Writer, steps []stepRecord, wallClock time.Duration) {
	if w == nil || len(steps) == 0 {
		return
	}

	stepW := len("step")
	kindW := len("kind")
	for _, s := range steps {
		if n := len(s.name) + 2; n > stepW { // +2 for the "✗ " marker on failed rows
			stepW = n
		}
		if n := len(string(s.kind)); n > kindW {
			kindW = n
		}
	}

	var sum time.Duration

	fmt.Fprintln(w)
	fmt.Fprintln(w, "=== Step summary ===")
	fmt.Fprintf(w, "  #  %-*s  %-*s  %10s\n", stepW, "step", kindW, "kind", "elapsed")

	for i, s := range steps {
		name := s.name
		if s.err != nil {
			name = "✗ " + name
		}
		sum += s.elapsed
		fmt.Fprintf(w, "%3d  %-*s  %-*s  %10s\n",
			i+1, stepW, name, kindW, string(s.kind), s.elapsed.Round(time.Second).String())
	}

	fmt.Fprintf(w, "%3s  %-*s  %-*s  %10s\n",
		"", stepW, "", kindW, "totals", sum.Round(time.Second).String())
	if wallClock > 0 {
		fmt.Fprintf(w, "Total execution time (wall-clock): %s\n", wallClock.Round(time.Second).String())
	}
}

// stepRecordJSON is the on-disk shape of one row in steps.jsonl. ElapsedNS is
// the raw nanoseconds (machine-readable; the renderer rounds at print time);
// Error is the step's error string when non-empty, absence meaning success.
type stepRecordJSON struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	ElapsedNS int64  `json:"elapsed_ns"`
	Error     string `json:"error,omitempty"`
}

func stepToJSON(s stepRecord) stepRecordJSON {
	j := stepRecordJSON{
		Name:      s.name,
		Kind:      string(s.kind),
		ElapsedNS: s.elapsed.Nanoseconds(),
	}
	if s.err != nil {
		j.Error = s.err.Error()
	}
	return j
}

func stepFromJSON(j stepRecordJSON) stepRecord {
	s := stepRecord{
		name:    j.Name,
		kind:    stepKind(j.Kind),
		elapsed: time.Duration(j.ElapsedNS),
	}
	if j.Error != "" {
		s.err = errors.New(j.Error)
	}
	return s
}

// appendStepLine writes one JSON record to the run's step sidecar. Best-effort:
// MkdirAll the parent on first write, append-open, write a single line, close.
// Empty path is a no-op (rs was nil). Mirrors appendSummaryLine.
func appendStepLine(path string, s stepRecord) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create step sidecar dir: %w", err)
	}
	data, err := json.Marshal(stepToJSON(s))
	if err != nil {
		return fmt.Errorf("marshal step record: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open step sidecar: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append step line: %w", err)
	}
	return nil
}

// loadSteps reads one record per line from a step sidecar in file (execution)
// order. Malformed lines are skipped silently, mirroring loadSummary.
func loadSteps(path string) ([]stepRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []stepRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var j stepRecordJSON
		if err := json.Unmarshal(line, &j); err != nil {
			continue
		}
		out = append(out, stepFromJSON(j))
	}
	if err := scanner.Err(); err != nil {
		return out, fmt.Errorf("read step sidecar: %w", err)
	}
	return out, nil
}

// PrintStepSummaryFile loads the records from the step sidecar at path and
// renders the step-execution table to w. Exported so the cobra layer can wire
// `gh optivem run summary` to replay it alongside the agent table. The run's
// start time isn't persisted, so the replayed totals show the sum-of-steps
// total only (no wall-clock headline). Missing/unreadable file → error; an
// empty file no-ops (the renderer skips empty input).
func PrintStepSummaryFile(w io.Writer, path string) error {
	steps, err := loadSteps(path)
	if err != nil {
		return err
	}
	renderStepSummary(w, steps, 0)
	return nil
}

// commandStepName resolves the display name for a command step from the live
// call-activity params at run-command execution: the resolved command line is
// the most descriptive (e.g. "gh optivem test compile"), falling back to the
// MID task-name and finally a generic label.
func commandStepName(ctx *statemachine.Context) string {
	if cmd := ctx.Params["command"]; cmd != "" {
		return cmd
	}
	if t := ctx.Params["task-name"]; t != "" {
		return t
	}
	return "command"
}

// wrapStepRecorders decorates every LOW execute-command run-command service
// task with a timer that records one command stepRecord per execution. Agent
// steps are recorded at dispatch time in newClaudeRunDispatcher; this covers
// the command half so the step summary spans both kinds. Mirrors
// wrapPhaseBoundaries' node-wrap shape (measure elapsed locally around the
// inner NodeFn). No-op when rs is nil.
func wrapStepRecorders(eng *statemachine.Engine, rs *runState, stderr io.Writer) {
	if rs == nil {
		return
	}
	for _, process := range eng.Processes {
		for id, node := range process.Nodes {
			if node.Kind != statemachine.ServiceTask || node.Raw.Action != "run-command" {
				continue
			}
			inner := node.Fn
			node.Fn = func(ctx *statemachine.Context) statemachine.Outcome {
				t0 := nowFn()
				out := inner(ctx)
				rs.recordStep(stepRecord{
					name:    commandStepName(ctx),
					kind:    stepKindCommand,
					elapsed: nowFn().Sub(t0),
					err:     out.Err,
				}, stderr)
				return out
			}
			process.Nodes[id] = node
		}
	}
}
