// step_summary.go owns the end-of-run step-execution summary — the sibling
// of the agent-summary table that spans all three kinds of timed work in a
// run: agent dispatches, shell commands, AND lifecycle/orchestration
// service-tasks. The agent summary answers "what did each agent cost?"; the
// step summary answers "which BPMN steps ran, in what order, and where did the
// wall-clock go?".
//
// One stepRecord is appended per executed MID-level atomic step:
//
//   - agent steps   — recorded at dispatch time in newClaudeRunDispatcher
//     (the LOW execute-agent RUN_AGENT fire), named by the MID task-name.
//   - command steps — recorded by wrapStepRecorders (driver.go), which wraps
//     every LOW execute-command run-command service task; the bpmn-step traces
//     to the MID via task-name and the detail carries the resolved command line.
//   - service steps — also recorded by wrapStepRecorders, for the curated
//     serviceStepActions (ticket parse, status transitions, external-system
//     resolution), named by the action.
//
// All append in execution order to runState.steps (the engine walks the BPMN
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

// stepKind classifies one executed BPMN step. Three kinds carry real
// wall-clock cost and are tracked: agent dispatches, shell commands, and a
// curated set of lifecycle/orchestration service-tasks (ticket parse, status
// transitions, external-system resolution — see serviceStepActions).
// Per-dispatch plumbing service-tasks (output validation, working-tree
// snapshots) and human-approval gates are deliberately omitted: they fire
// after every real step and would pair a noise row with each one. The
// aggregate time they consume is surfaced instead as the "untracked overhead"
// reconciliation line in renderStepSummary.
type stepKind string

const (
	stepKindAgent   stepKind = "agent"
	stepKindCommand stepKind = "command"
	stepKindService stepKind = "service"
)

// stepRecord is one row in the step-execution summary: a MID-level atomic step
// that actually ran, its kind, how long it took, and whether it failed. err is
// set when the step's NodeFn returned an Outcome with a non-nil Err so the
// table can mark the row with a "✗" prefix, mirroring renderAgentSummary.
//
// bpmnStep is the diagram node that traces 1:1 to process-flow.yaml (the agent
// task-name, the command's MID process name, or the service action). detail
// carries the concrete command line for command rows (blank otherwise — an
// agent's name already IS its bpmnStep). channel disambiguates per-channel
// unrolled rows (e.g. implement-system × api/ui); blank when the step isn't
// channel-split. name is retained as the legacy primary identity so old
// sidecars (which predate bpmnStep) still render — the renderer falls back to
// it when bpmnStep is empty.
type stepRecord struct {
	name     string
	bpmnStep string
	detail   string
	channel  string
	kind     stepKind
	elapsed  time.Duration
	err      error
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

// stepBpmnLabel is the bpmn-step column's value for one row: the diagram node
// the step traces to. Falls back to the legacy name field for pre-bpmnStep
// sidecars so old runs still render a non-empty first column.
func stepBpmnLabel(s stepRecord) string {
	if s.bpmnStep != "" {
		return s.bpmnStep
	}
	return s.name
}

// renderStepSummary writes a per-step table + per-kind breakdown + totals to w.
// One row per executed step, in execution order. Columns:
//
//	#  bpmn-step  detail  kind  channel  elapsed
//
// bpmn-step traces 1:1 to process-flow.yaml; detail carries the concrete
// command line for command rows; channel disambiguates per-channel unrolled
// rows. After the rows, one subtotal per kind (agent / command / service) gives
// its elapsed and integer percentage of the step-sum, then a totals row carries
// the step-sum. When wallClock > 0 two reconciliation lines follow: "untracked
// overhead" (= wall-clock − step-sum, the time spent in gateways, param
// plumbing, and un-recorded service-tasks) and "wall-clock" itself — so the
// headline visibly adds up. No-op when w is nil or there are no steps.
//
// Single source of truth for the table shape — the live banner
// (printStepSummary), the historical replay (PrintStepSummaryFile), and the
// Markdown run digest all route through here so the views stay byte-identical.
func renderStepSummary(w io.Writer, steps []stepRecord, wallClock time.Duration) {
	if w == nil || len(steps) == 0 {
		return
	}

	cw := stepColumnWidths(steps)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "=== Step summary ===")
	fmt.Fprintf(w, "  #  %-*s  %-*s  %-*s  %-*s  %10s\n",
		cw.bpmn, "bpmn-step", cw.detail, "detail", cw.kind, "kind", cw.channel, "channel", "elapsed")

	sum := time.Duration(0)
	byKind := map[stepKind]time.Duration{}
	for i, s := range steps {
		bpmn := stepBpmnLabel(s)
		if s.err != nil {
			bpmn = "✗ " + bpmn
		}
		sum += s.elapsed
		byKind[s.kind] += s.elapsed
		fmt.Fprintf(w, "%3d  %-*s  %-*s  %-*s  %-*s  %10s\n",
			i+1, cw.bpmn, bpmn, cw.detail, s.detail, cw.kind, string(s.kind), cw.channel, s.channel,
			s.elapsed.Round(time.Second).String())
	}

	renderStepBreakdown(w, cw, byKind, sum)
	renderStepReconciliation(w, cw, sum, wallClock)
}

// stepWidths holds the per-column widths the renderer aligns to.
type stepWidths struct{ bpmn, detail, kind, channel int }

// stepColumnWidths sizes each column to its widest value. The kind column also
// hosts the breakdown/totals labels, so it accounts for those ("commands" is 8
// chars — wider than any kind value).
func stepColumnWidths(steps []stepRecord) stepWidths {
	cw := stepWidths{bpmn: len("bpmn-step"), detail: len("detail"), kind: len("kind"), channel: len("channel")}
	for _, lbl := range []string{"agents", "commands", "service", "totals"} {
		if n := len(lbl); n > cw.kind {
			cw.kind = n
		}
	}
	for _, s := range steps {
		if n := len(stepBpmnLabel(s)) + 2; n > cw.bpmn { // +2 for the "✗ " marker on failed rows
			cw.bpmn = n
		}
		if n := len(s.detail); n > cw.detail {
			cw.detail = n
		}
		if n := len(s.channel); n > cw.channel {
			cw.channel = n
		}
		if n := len(string(s.kind)); n > cw.kind {
			cw.kind = n
		}
	}
	return cw
}

// renderStepBreakdown writes one subtotal row per kind (fixed agent → command →
// service order, skipping a kind with no rows) plus the totals row. Each kind's
// percentage is integer-rounded against the step-sum so the subtotals sum to
// ~100%.
func renderStepBreakdown(w io.Writer, cw stepWidths, byKind map[stepKind]time.Duration, sum time.Duration) {
	for _, k := range []stepKind{stepKindAgent, stepKindCommand, stepKindService} {
		d, ok := byKind[k]
		if !ok {
			continue
		}
		pct := int64(0)
		if sum > 0 {
			pct = (int64(d)*100 + int64(sum)/2) / int64(sum)
		}
		fmt.Fprintf(w, "%3s  %-*s  %-*s  %-*s  %-*s  %10s   %d%%\n",
			"", cw.bpmn, "", cw.detail, "", cw.kind, breakdownLabel(k), cw.channel, "",
			d.Round(time.Second).String(), pct)
	}
	fmt.Fprintf(w, "%3s  %-*s  %-*s  %-*s  %-*s  %10s\n",
		"", cw.bpmn, "", cw.detail, "", cw.kind, "totals", cw.channel, "", sum.Round(time.Second).String())
}

// renderStepReconciliation writes the "untracked overhead" (= wall-clock −
// step-sum) and "wall-clock" lines so the headline visibly adds up. No-op when
// wallClock is zero (the replay path doesn't persist the run start time). The
// label spans the bpmn-step+detail+kind+channel columns so the elapsed value
// lands under the elapsed column.
func renderStepReconciliation(w io.Writer, cw stepWidths, sum, wallClock time.Duration) {
	if wallClock <= 0 {
		return
	}
	labelSpan := cw.bpmn + 2 + cw.detail + 2 + cw.kind + 2 + cw.channel
	if overhead := wallClock - sum; overhead > 0 {
		fmt.Fprintf(w, "%3s  %-*s  %10s\n",
			"", labelSpan, "untracked overhead", overhead.Round(time.Second).String())
	}
	fmt.Fprintf(w, "%3s  %-*s  %10s\n",
		"", labelSpan, "wall-clock", wallClock.Round(time.Second).String())
}

// breakdownLabel maps a stepKind to its plural breakdown-row label. agent and
// command pluralize; service is a mass noun and stays singular.
func breakdownLabel(k stepKind) string {
	switch k {
	case stepKindAgent:
		return "agents"
	case stepKindCommand:
		return "commands"
	default:
		return "service"
	}
}

// stepRecordJSON is the on-disk shape of one row in steps.jsonl. ElapsedNS is
// the raw nanoseconds (machine-readable; the renderer rounds at print time);
// Error is the step's error string when non-empty, absence meaning success.
// BpmnStep / Detail / Channel are all omitempty so pre-feature sidecars (which
// lack them) decode cleanly — an absent BpmnStep falls back to Name at render
// time (see stepBpmnLabel).
type stepRecordJSON struct {
	Name      string `json:"name"`
	BpmnStep  string `json:"bpmn_step,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Channel   string `json:"channel,omitempty"`
	Kind      string `json:"kind"`
	ElapsedNS int64  `json:"elapsed_ns"`
	Error     string `json:"error,omitempty"`
}

func stepToJSON(s stepRecord) stepRecordJSON {
	j := stepRecordJSON{
		Name:      s.name,
		BpmnStep:  s.bpmnStep,
		Detail:    s.detail,
		Channel:   s.channel,
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
		name:     j.Name,
		bpmnStep: j.BpmnStep,
		detail:   j.Detail,
		channel:  j.Channel,
		kind:     stepKind(j.Kind),
		elapsed:  time.Duration(j.ElapsedNS),
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
// the most descriptive (e.g. "gh optivem system-test compile"), falling back to the
// MID task-name and finally a generic label.
//
// When a `suite` param is present and non-empty, it is appended as
// `--suite=<suite>` — the `run-tests` MID carries the suite as a separate param
// (process-flow.yaml), not baked into the static `command` literal, so without
// this every `gh optivem system-test run` row would render identically regardless of
// whether it ran the acceptance, contract-real, or contract-stub suite. An
// empty suite means "all" (the strict-mode `""` convention) and stays bare.
// test-names is deliberately not appended — it can be a long list and the suite
// alone is enough to tell the rows apart at a glance.
func commandStepName(ctx *statemachine.Context) string {
	if cmd := ctx.Params["command"]; cmd != "" {
		if suite := ctx.Params["suite"]; suite != "" {
			return cmd + " --suite=" + suite
		}
		return cmd
	}
	if t := ctx.Params["task-name"]; t != "" {
		return t
	}
	return "command"
}

// commandBpmnStep resolves the bpmn-step (diagram node) for a command step: the
// MID process node carries a `task-name` param (process-flow.yaml), so prefer
// it; fall back to the command line for pre-feature replay where no task-name
// was threaded.
func commandBpmnStep(ctx *statemachine.Context) string {
	if t := ctx.Params["task-name"]; t != "" {
		return t
	}
	return commandStepName(ctx)
}

// serviceStepActions is the curated allowlist of lifecycle/orchestration
// service-task actions recorded as stepKindService — the ones that carry
// semantic meaning and measurable wall-clock (reading the ticket, status
// transitions, external-system resolution). Per-dispatch plumbing actions
// (validate-outputs-and-scopes, snapshot-working-tree, …) are deliberately
// absent: recording them would pair a noise row with every agent dispatch. The
// time they consume surfaces in the renderer's "untracked overhead" line.
var serviceStepActions = map[string]bool{
	"parse-ticket":                         true,
	"move-to-in-refinement":                true,
	"move-to-ready":                        true,
	"move-to-in-progress":                  true,
	"move-to-in-acceptance":                true,
	"validate-external-systems-registered": true,
	"resolve-external-system":              true,
}

// wrapStepRecorders decorates the timed service-tasks with a recorder so the
// step summary spans all three kinds. run-command tasks record as
// stepKindCommand (named by the command line, traced to their MID via
// task-name); the curated serviceStepActions record as stepKindService (named
// by the action). Agent steps are recorded separately at dispatch time in
// newClaudeRunDispatcher (RUN_AGENT is a user-task, not a service-task, so it
// never matches here — no double-count). Mirrors wrapPhaseBoundaries' node-wrap
// shape (measure elapsed locally around the inner NodeFn). No-op when rs is nil.
func wrapStepRecorders(eng *statemachine.Engine, rs *runState, stderr io.Writer) {
	if rs == nil {
		return
	}
	for _, process := range eng.Processes {
		for id, node := range process.Nodes {
			if node.Kind != statemachine.ServiceTask {
				continue
			}
			action := node.Raw.Action
			isCommand := action == "run-command"
			if !isCommand && !serviceStepActions[action] {
				continue
			}
			inner := node.Fn
			act := action
			node.Fn = func(ctx *statemachine.Context) statemachine.Outcome {
				t0 := nowFn()
				out := inner(ctx)
				rs.recordStep(newStepRecord(ctx, act, isCommand, nowFn().Sub(t0), out.Err), stderr)
				return out
			}
			process.Nodes[id] = node
		}
	}
}

// newStepRecord builds the stepRecord for a wrapped service-task. Command tasks
// carry the concrete command line as detail and trace to their MID via
// task-name; service tasks are named by their action and carry no detail or
// channel.
func newStepRecord(ctx *statemachine.Context, action string, isCommand bool, elapsed time.Duration, err error) stepRecord {
	if isCommand {
		return stepRecord{
			name:     commandStepName(ctx),
			bpmnStep: commandBpmnStep(ctx),
			detail:   commandStepName(ctx),
			channel:  ctx.Params["channel"],
			kind:     stepKindCommand,
			elapsed:  elapsed,
			err:      err,
		}
	}
	return stepRecord{
		name:     action,
		bpmnStep: action,
		kind:     stepKindService,
		elapsed:  elapsed,
		err:      err,
	}
}
