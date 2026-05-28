package actions

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fatih/color"
)

// WriteBpmnTaskTiming prints a one-line BPMN-level duration banner of the
// form "⏱  BPMN TASK <task-id> (<kind>): <elapsed>" in cyan. Called by the
// driver around clauderun.Dispatch and by the run-command action around
// a.runShell — the two subprocess-backed BPMN task kinds the operator
// wants wall-clock visibility on. The clauderun layer's own AGENT EXIT
// banner is unaffected; this prints after it.
//
// Rounding to second matches clauderun.elapsedRound so operators see
// consistent grain across both banner sources.
//
// taskID empty → the "<task-id>" segment is suppressed, yielding
// "⏱  BPMN TASK (<kind>): <elapsed>". Empty w is a no-op so test
// fixtures that nil out stdout don't crash.
func WriteBpmnTaskTiming(w io.Writer, taskID, kind string, elapsed time.Duration) {
	if w == nil {
		return
	}
	cyan := color.New(color.FgCyan)
	prefix := ""
	if taskID != "" {
		prefix = " " + taskID
	}
	fmt.Fprintln(w, cyan.Sprintf("⏱  BPMN TASK%s (%s): %s", prefix, kind, elapsed.Round(time.Second)))
}

// TruncateForBanner returns s capped to maxLen chars, appending "…" when
// truncated, and replaces any embedded newline with a space so a
// multi-line command renders cleanly on one banner line. Used by the
// run-command timing line to keep long composed command strings
// ("gh optivem test run --suite=… --test=…") inside the visual budget.
func TruncateForBanner(s string, maxLen int) string {
	flat := strings.ReplaceAll(s, "\n", " ")
	if len(flat) <= maxLen {
		return flat
	}
	return flat[:maxLen] + "…"
}
