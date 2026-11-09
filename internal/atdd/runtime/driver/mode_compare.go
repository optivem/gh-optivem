// mode_compare.go aggregates past runs by the inner-loop mode their component
// ran in, so the modes (none / test-after / tdd) can be compared on the numbers
// the inner-loop experiments need: tokens split into fresh vs cached, cost,
// fix-loop attempts and elapsed time. Each dispatch row in summary.jsonl is
// tagged with the run's inner-loop mode and ticket id (see dispatchRecord).
package driver

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// modeStats accumulates the comparison columns for one inner-loop mode.
type modeStats struct {
	runs         int
	dispatches   int
	fixAttempts  int // dispatches that ran inside a max-visits loop (attempt_max > 0)
	freshTokens  int // input + output + cache creation
	cachedTokens int // cache reads
	cost         float64
	elapsed      time.Duration
}

// runMode resolves a run's inner-loop mode from its tagged records: the first
// non-empty tag wins, and a run with no tagged record ran with no inner loop.
func runMode(records []dispatchRecord) string {
	for _, r := range records {
		if r.innerLoopMode != "" {
			return r.innerLoopMode
		}
	}
	return "none"
}

// CompareModes reads every run under <repoPath>/.gh-optivem/runs that has a
// summary sidecar, groups the runs by inner-loop mode, and renders one row per
// mode with per-run averages. Runs that pre-date the tagging count as "none".
func CompareModes(w io.Writer, repoPath string) error {
	root := filepath.Join(repoPath, ghOptivemDir, "runs")
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read runs dir %s: %w", root, err)
	}
	byMode := statsByMode(root, entries)
	if len(byMode) == 0 {
		return fmt.Errorf("no runs under %s have a summary.jsonl sidecar", root)
	}
	renderModeComparison(w, byMode)
	return nil
}

// statsByMode loads each run directory's summary sidecar and accumulates its
// records into the stats for the run's inner-loop mode.
func statsByMode(root string, entries []os.DirEntry) map[string]*modeStats {
	byMode := map[string]*modeStats{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		records, err := loadSummary(filepath.Join(root, e.Name(), "summary.jsonl"))
		if err != nil || len(records) == 0 {
			continue // run without a sidecar: nothing to compare
		}
		mode := runMode(records)
		st := byMode[mode]
		if st == nil {
			st = &modeStats{}
			byMode[mode] = st
		}
		st.addRun(records)
	}
	return byMode
}

// addRun folds one run's dispatch records into the mode's totals.
func (st *modeStats) addRun(records []dispatchRecord) {
	st.runs++
	for _, r := range records {
		st.dispatches++
		if r.attemptMax > 0 {
			st.fixAttempts++
		}
		st.elapsed += r.elapsed
		if r.usage != nil {
			st.freshTokens += r.usage.InputTokens + r.usage.OutputTokens + r.usage.CacheCreationInputTokens
			st.cachedTokens += r.usage.CacheReadInputTokens
			st.cost += r.usage.TotalCostUSD
		}
	}
}

// renderModeComparison writes the per-mode table of per-run averages, one row
// per mode in sorted order.
func renderModeComparison(w io.Writer, byMode map[string]*modeStats) {
	modes := make([]string, 0, len(byMode))
	for m := range byMode {
		modes = append(modes, m)
	}
	sort.Strings(modes)

	fmt.Fprintln(w, "Inner-loop mode comparison (per-run averages):")
	fmt.Fprintf(w, "  %-12s %5s %11s %13s %13s %9s %9s %10s\n", "mode", "runs", "dispatches", "fresh-tokens", "cached-tokens", "cost-usd", "fix-loops", "elapsed")
	fmt.Fprintln(w, "  "+strings.Repeat("-", 92))
	for _, m := range modes {
		s := byMode[m]
		n := float64(s.runs)
		fmt.Fprintf(w, "  %-12s %5d %11.1f %13.0f %13.0f %9.2f %9.1f %10s\n",
			m, s.runs,
			float64(s.dispatches)/n,
			float64(s.freshTokens)/n,
			float64(s.cachedTokens)/n,
			s.cost/n,
			float64(s.fixAttempts)/n,
			(s.elapsed / time.Duration(s.runs)).Round(time.Second),
		)
	}
}
