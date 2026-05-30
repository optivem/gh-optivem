// run_commands.go wires the `gh optivem run` parent and its children.
// `run` is the noun group for per-pipeline-run artefacts produced under
// .gh-optivem/runs/<ts>/ — a dispatcher per-prompt log, the streamed
// claude events JSONL, the agent's declared-outputs JSONL, and the
// per-dispatch agent-summary sidecar. The first child, `summary`,
// replays any run's agent-summary table from its sidecar.
//
//	gh optivem run summary           # most recent run
//	gh optivem run summary 20260528-150000   # a specific run
//
// `run` is grouped under "Other" — a diagnostic-noun verb, not a project
// op (no gh-optivem.yaml is touched) and not a cross-repo op (it reads
// only the local cwd's .gh-optivem/runs/ tree).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/driver"
)

// newRunCmd builds the `gh optivem run` parent. The parent has no Run,
// so invoking it without a subcommand prints help.
func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Inspect artefacts from past `gh optivem implement` runs",
	}
	cmd.AddCommand(newRunSummaryCmd())
	return cmd
}

// newRunSummaryCmd replays the per-agent summary table for one
// pipeline run. No positional arg → most-recently-modified run under
// <cwd>/.gh-optivem/runs/. One positional arg → that run-timestamp
// directory (e.g. 20260528-150000).
//
// Reads from .gh-optivem/runs/<ts>/summary.jsonl, which the driver
// writes one line per dispatch as the run progresses. A binary crash
// mid-run still leaves on disk every row that completed before the
// bust — that is the use case this subcommand exists to surface.
func newRunSummaryCmd() *cobra.Command {
	var markdown bool
	cmd := &cobra.Command{
		Use:   "summary [<run-ts>]",
		Short: "Print the agent-summary table for a past `gh optivem implement` run",
		Long: `Replay the per-agent summary table from the run's on-disk sidecar.

No arg → most-recently-modified run under <cwd>/.gh-optivem/runs/.
One positional arg → that run-ts directory (e.g. 20260528-150000).

Columns: agent, model, effort, elapsed, in / out tokens, $ cost. Rows
whose dispatch ran in interactive mode (no stream-json envelope) show — for
the token + cost columns.

--markdown prints the human run digest (summary.md) instead: the ticket
header, overall verdict, and the same table fenced inside Markdown. It
renders inline on github.com.`,
		Example: `  gh optivem run summary
  gh optivem run summary 20260528-150000
  gh optivem run summary --markdown`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cwd, err := os.Getwd()
			exitOnError(err)
			var runDir string
			if len(args) == 0 {
				// Skip pre-feature runs and runs that bombed before
				// any dispatch — they have no sidecar, so picking
				// the strict mtime-latest would error noisily and
				// point at the wrong run. LatestRunWithSummary lands
				// on the newest run with actual content instead.
				runDir, err = driver.LatestRunWithSummary(cwd)
				exitOnError(err)
			} else {
				runDir = filepath.Join(cwd, ".gh-optivem", "runs", args[0])
			}
			if markdown {
				// Prefer the emitted file over re-rendering from
				// records so the replay is byte-identical with what
				// the run wrote at exit.
				digestPath := filepath.Join(runDir, "summary.md")
				if _, err := os.Stat(digestPath); err != nil {
					if os.IsNotExist(err) {
						exitOnError(fmt.Errorf("no run digest at %s; this run pre-dates the summary.md feature", digestPath))
					}
					exitOnError(fmt.Errorf("stat %s: %w", digestPath, err))
				}
				exitOnError(driver.PrintRunDigestFile(cmd.OutOrStdout(), digestPath))
				return
			}
			summaryPath := filepath.Join(runDir, "summary.jsonl")
			if _, err := os.Stat(summaryPath); err != nil {
				if os.IsNotExist(err) {
					exitOnError(fmt.Errorf("no summary sidecar at %s; this run pre-dates the summary feature or no agents dispatched", summaryPath))
				}
				exitOnError(fmt.Errorf("stat %s: %w", summaryPath, err))
			}
			exitOnError(driver.PrintSummaryFile(cmd.OutOrStdout(), summaryPath))
		},
	}
	cmd.Flags().BoolVar(&markdown, "markdown", false, "Print the human run digest (summary.md) instead of the table")
	return cmd
}
