package outlog

import (
	"fmt"
	"io"
	"time"

	"github.com/fatih/color"
)

// WritePhaseBoundary prints a one-line phase-boundary banner of the form
// "[phase]  <edge>  <phaseName>" (on `start`) or
// "[phase]  end  <phaseName>  <elapsed>" (on `end`), in bold bright cyan.
// Called by the driver's phase-decoration wrap around outermost-call-
// activity boundaries — the CYCLE-level processes the operator's mental
// model treats as "phases". The trace stream's `[trace …] > / OK …` pair
// continues to bracket every node fire at finer grain.
//
// A blank line is emitted BEFORE `start` and AFTER `end` so each phase
// block is visually bracketed without separator rules. No blank line at
// the inverse edges keeps the phase block tight.
//
// Rounding to second matches clauderun.elapsedRound so operators see
// consistent grain across both banner sources. Empty w is a no-op so
// test fixtures that nil out stdout don't crash.
func WritePhaseBoundary(w io.Writer, edge, phaseName string, elapsed time.Duration) {
	if w == nil {
		return
	}
	c := color.New(color.FgHiCyan, color.Bold)
	switch edge {
	case "start":
		fmt.Fprintln(w)
		fmt.Fprintln(w, c.Sprintf("[phase]  start  %s", phaseName))
	case "end":
		fmt.Fprintln(w, c.Sprintf("[phase]  end    %s  %s", phaseName, elapsed.Round(time.Second)))
		fmt.Fprintln(w)
	}
}
