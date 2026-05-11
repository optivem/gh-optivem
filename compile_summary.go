// compile_summary.go collects per-tier outcomes during a `gh optivem compile`
// run and prints a compact tail summary, so the user can scan success/failure
// without scrolling the (often long) compile output.
//
// One row per `compiler.Compile` call (i.e. per tier). Per-command timing
// inside the compiler package is deliberately not threaded through — the
// compile_commands.go level is the right granularity, and keeping the
// `compiler` package's `error`-only API intact preserves its testable seam.
package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"

	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// compileSummary collects rows during the compile sweep and prints a single
// tail block. Construction starts the wall-clock timer so the total duration
// includes phase headers and any pre-flight work, matching what the user
// actually waited for.
type compileSummary struct {
	start   time.Time
	rows    []compileRow
	skipped []string
}

type compileRow struct {
	phase string
	tier  string
	lang  string
	path  string
	dur   time.Duration
	err   error
}

func newCompileSummary() *compileSummary {
	return &compileSummary{start: time.Now()}
}

// Record appends a per-tier outcome. err == nil means OK.
func (s *compileSummary) Record(phase, tier, lang, path string, dur time.Duration, err error) {
	s.rows = append(s.rows, compileRow{
		phase: phase,
		tier:  tier,
		lang:  lang,
		path:  path,
		dur:   dur,
		err:   err,
	})
}

// MarkSkipped flags a phase that did not run because an earlier phase failed.
// Surfaced in the summary so the reader knows the run halted early.
func (s *compileSummary) MarkSkipped(phase string) {
	s.skipped = append(s.skipped, phase)
}

// recordCompile times fn, records the outcome, and returns fn's error
// unchanged so callers preserve today's first-error-halts behaviour.
func recordCompile(sum *compileSummary, phase, tier string, spec projectconfig.TierSpec, fn func() error) error {
	start := time.Now()
	err := fn()
	sum.Record(phase, tier, spec.Lang, spec.Path, time.Since(start), err)
	return err
}

// Print writes the summary block to stdout. Safe to call once at the end of
// every compile invocation (bare, system, or system-tests); produces nothing
// if no rows were recorded (e.g. config load failed before any compile ran).
func (s *compileSummary) Print() {
	if len(s.rows) == 0 && len(s.skipped) == 0 {
		return
	}

	var ok, fail int
	for _, r := range s.rows {
		if r.err == nil {
			ok++
		} else {
			fail++
		}
	}

	title := "Compile summary"
	rule := strings.Repeat("─", len([]rune(title)))

	fmt.Println()
	color.New(color.FgCyan, color.Bold).Println(title)
	color.New(color.Faint).Println(rule)

	// Group rows by phase, preserving first-seen order.
	var phaseOrder []string
	byPhase := map[string][]compileRow{}
	for _, r := range s.rows {
		if _, seen := byPhase[r.phase]; !seen {
			phaseOrder = append(phaseOrder, r.phase)
		}
		byPhase[r.phase] = append(byPhase[r.phase], r)
	}

	for _, phase := range phaseOrder {
		fmt.Printf("  %s\n", phase)
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, r := range byPhase[phase] {
			status := color.New(color.FgGreen, color.Bold).Sprint("OK  ")
			if r.err != nil {
				status = color.New(color.FgRed, color.Bold).Sprint("FAIL")
			}
			dur := color.New(color.Faint).Sprint(formatDuration(r.dur))
			fmt.Fprintf(tw, "    %s\t%s\t(%s)\t%s\t%s\n",
				status, r.tier, r.lang, r.path, dur)
		}
		_ = tw.Flush()
		// Print the error message under each failing row, indented to align
		// with the tier column. Done after Flush so tabwriter doesn't try to
		// align the multi-character error line into the table.
		for _, r := range byPhase[phase] {
			if r.err != nil {
				fmt.Printf("          %s %s\n",
					color.New(color.Faint).Sprint("└─"),
					color.New(color.Faint).Sprint(r.err.Error()))
			}
		}
	}

	for _, phase := range s.skipped {
		fmt.Printf("  %s  %s\n", phase, color.New(color.Faint).Sprint("(skipped)"))
	}

	fmt.Println()
	parts := []string{}
	if ok > 0 {
		parts = append(parts, color.New(color.FgGreen, color.Bold).Sprintf("%d OK", ok))
	}
	if fail > 0 {
		parts = append(parts, color.New(color.FgRed, color.Bold).Sprintf("%d FAIL", fail))
	}
	if len(s.skipped) > 0 {
		parts = append(parts, color.New(color.Faint).Sprintf("%d skipped", len(s.skipped)))
	}
	parts = append(parts, color.New(color.Faint).Sprintf("%s total", formatDuration(time.Since(s.start))))
	fmt.Println(strings.Join(parts, color.New(color.Faint).Sprint(" · ")))
}

// Wall-clock durations use the package-level formatDuration in main.go.
