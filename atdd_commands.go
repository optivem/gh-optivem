// atdd_commands.go wires the `gh optivem atdd …` subcommands into the root
// Cobra command. Two public commands mirror today's slash commands:
//
//	gh optivem atdd implement-ticket --issue N
//	gh optivem atdd manage-project
//
// A hidden `debug` parent groups the diagnostic helpers — pick-top-ready,
// classify, next-phase, gate, release — so each underlying runtime package
// can be exercised standalone without rerunning the whole pipeline. The
// hidden flag (Cobra's `Hidden: true`) keeps these out of the default help
// text; `gh optivem atdd debug --help` still works for anyone who knows
// they exist.
//
// The handlers are deliberately thin: they translate Cobra flags into
// internal/atdd/runtime/* calls and surface their errors via exitOnError
// (defined in runner_commands.go) for consistency with the rest of the
// `optivem` binary.
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/board"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/classify"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/driver"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/gates"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/release"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// newAtddCmd builds the `gh optivem atdd` parent. The parent has no Run, so
// invoking it without a subcommand prints help. Children are added in the
// order they should appear in --help: implement-ticket and manage-project
// first (the public surface), debug last (hidden — shown only with
// --help-all).
func newAtddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "atdd",
		Short: "Run the ATDD pipeline driver",
		Long: `Run the ATDD pipeline driver.

The driver loads docs/atdd/process/process-flow.yaml from the consumer's
working directory and walks it node by node — dispatching service tasks
inline (board picks, classification, smoke tests, commits) and pausing at
each user-task node so the operator can launch the corresponding Claude
Code agent (atdd-story, atdd-test, atdd-dsl, …). When the agent's COMMIT
lands on HEAD, the operator presses Enter and the engine moves on.`,
	}
	cmd.AddCommand(
		newAtddImplementTicketCmd(),
		newAtddManageProjectCmd(),
		newAtddDebugCmd(),
	)
	return cmd
}

// newAtddImplementTicketCmd implements `gh optivem atdd implement-ticket
// --issue N`. The driver pre-resolves the project item for the issue, seeds
// Context, and walks the main flow from MOVE_TO_IN_PROGRESS (skipping the
// PICK_TOP_READY picker).
func newAtddImplementTicketCmd() *cobra.Command {
	var (
		issueArg   string
		projectURL string
		autonomous bool
	)
	cmd := &cobra.Command{
		Use:   "implement-ticket",
		Short: "Walk the ATDD pipeline for a specific GitHub issue",
		Example: `  gh optivem atdd implement-ticket --issue 42
  gh optivem atdd implement-ticket --issue https://github.com/optivem/shop/issues/42
  gh optivem atdd implement-ticket --issue 42 --project https://github.com/orgs/optivem/projects/3`,
		Run: func(cmd *cobra.Command, args []string) {
			issue, err := parseIssueArg(issueArg)
			exitOnError(err)
			exitOnError(driver.Run(context.Background(), driver.Options{
				IssueNum:   issue,
				ProjectURL: projectURL,
				Autonomous: autonomous,
			}))
		},
	}
	cmd.Flags().StringVar(&issueArg, "issue", "", "GitHub issue number or URL (required; accepts e.g. 42 or https://github.com/owner/repo/issues/42)")
	cmd.Flags().StringVar(&projectURL, "project", "", "GitHub project URL (optional; defaults to README.md or git-remote lookup)")
	cmd.Flags().BoolVar(&autonomous, "autonomous", false, "Skip human-approval STOPs (agent-dispatch pauses still apply in v1)")
	return cmd
}

// newAtddManageProjectCmd implements `gh optivem atdd manage-project`. The
// driver picks the top item from the Ready column and walks the main flow
// from START.
func newAtddManageProjectCmd() *cobra.Command {
	var (
		projectURL string
		autonomous bool
	)
	cmd := &cobra.Command{
		Use:   "manage-project",
		Short: "Pick the top Ready ticket and walk the ATDD pipeline",
		Example: `  gh optivem atdd manage-project
  gh optivem atdd manage-project --project https://github.com/orgs/optivem/projects/3`,
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(driver.Run(context.Background(), driver.Options{
				ProjectURL: projectURL,
				Autonomous: autonomous,
			}))
		},
	}
	cmd.Flags().StringVar(&projectURL, "project", "", "GitHub project URL (optional; defaults to README.md or git-remote lookup)")
	cmd.Flags().BoolVar(&autonomous, "autonomous", false, "Skip human-approval STOPs (agent-dispatch pauses still apply in v1)")
	return cmd
}

// newAtddDebugCmd builds the hidden `gh optivem atdd debug` parent. The
// children expose runtime sub-packages standalone — useful for reproducing a
// single phase in isolation without spinning up the whole pipeline.
func newAtddDebugCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "debug",
		Short:  "Diagnostic helpers (porcelain not stable; flags can change without notice)",
		Hidden: true,
	}
	cmd.AddCommand(
		newAtddDebugPickTopReadyCmd(),
		newAtddDebugClassifyCmd(),
		newAtddDebugNextPhaseCmd(),
		newAtddDebugGateCmd(),
		newAtddDebugReleaseCmd(),
	)
	return cmd
}

// newAtddDebugPickTopReadyCmd runs board.PickTopReady standalone and prints
// the picked item. No move; useful for "what would manage-project pick?".
func newAtddDebugPickTopReadyCmd() *cobra.Command {
	var projectURL string
	cmd := &cobra.Command{
		Use:   "pick-top-ready",
		Short: "Print the top Ready item without moving it",
		Run: func(cmd *cobra.Command, args []string) {
			pick, err := board.PickTopReady(context.Background(), board.Options{
				ProjectURL: projectURL,
			})
			exitOnError(err)
			fmt.Printf("issue:    #%d\n", pick.IssueNum)
			fmt.Printf("title:    %s\n", pick.Title)
			fmt.Printf("repo:     %s\n", pick.Repo)
			fmt.Printf("url:      %s\n", pick.IssueURL)
			fmt.Printf("project:  %s\n", pick.ProjectID)
			fmt.Printf("item:     %s\n", pick.ItemID)
		},
	}
	cmd.Flags().StringVar(&projectURL, "project", "", "GitHub project URL (optional; defaults to README.md or git-remote lookup)")
	return cmd
}

// newAtddDebugClassifyCmd runs classify.Classify and prints the result. No
// orchestration side-effects.
func newAtddDebugClassifyCmd() *cobra.Command {
	var (
		issueArg string
		repo     string
	)
	cmd := &cobra.Command{
		Use:   "classify",
		Short: "Classify a ticket via the deterministic fast path",
		Run: func(cmd *cobra.Command, args []string) {
			issue, err := parseIssueArg(issueArg)
			exitOnError(err)
			res, err := classify.Classify(context.Background(), issue, classify.Options{Repo: repo})
			exitOnError(err)
			fmt.Printf("issue:           #%d\n", res.IssueNum)
			fmt.Printf("classification:  %s\n", res.Classification)
			fmt.Printf("route:           %s\n", res.Route)
			fmt.Printf("type_field:      %s\n", res.TypeField)
			fmt.Printf("labels:          %s\n", strings.Join(res.LabelsSeen, ","))
			fmt.Printf("reasoning:       %s\n", res.Reasoning)
		},
	}
	cmd.Flags().StringVar(&issueArg, "issue", "", "GitHub issue number or URL (required)")
	cmd.Flags().StringVar(&repo, "repo", "", "owner/repo override for `gh issue view`")
	return cmd
}

// newAtddDebugNextPhaseCmd loads the YAML, builds an unbound Engine, and
// prints the outgoing edge that nextEdge would pick from a given node under
// a synthetic state. Useful for "why did the driver follow the No edge?".
func newAtddDebugNextPhaseCmd() *cobra.Command {
	var (
		yamlPath string
		flowName string
		nodeID   string
		stateRaw string
	)
	cmd := &cobra.Command{
		Use:   "next-phase",
		Short: "Print the next node nextEdge would pick from --node under --state",
		Example: `  gh optivem atdd debug next-phase --node GATE_DSL --state dsl_interface_changed=true
  gh optivem atdd debug next-phase --flow at_cycle --node AT_RED_TEST_COMMIT`,
		Run: func(cmd *cobra.Command, args []string) {
			if yamlPath == "" {
				yamlPath = driver.DefaultYAMLPath
			}
			if flowName == "" {
				flowName = driver.DefaultFlowName
			}
			if nodeID == "" {
				exitOnError(fmt.Errorf("--node is required"))
			}
			eng, err := statemachine.LoadFile(yamlPath)
			exitOnError(err)
			sCtx := statemachine.NewContext()
			for _, kv := range strings.Split(stateRaw, ",") {
				kv = strings.TrimSpace(kv)
				if kv == "" {
					continue
				}
				k, v, ok := strings.Cut(kv, "=")
				if !ok {
					exitOnError(fmt.Errorf("bad --state pair %q (want key=value)", kv))
				}
				sCtx.Set(strings.TrimSpace(k), strings.TrimSpace(v))
			}
			next, err := eng.NextEdge(flowName, nodeID, sCtx)
			exitOnError(err)
			fmt.Printf("from:  %s\n", nodeID)
			fmt.Printf("to:    %s\n", next)
		},
	}
	cmd.Flags().StringVar(&yamlPath, "yaml", "", "Path to process-flow.yaml (defaults to docs/atdd/process/process-flow.yaml)")
	cmd.Flags().StringVar(&flowName, "flow", "", "Flow name (defaults to main)")
	cmd.Flags().StringVar(&nodeID, "node", "", "Source node ID (required)")
	cmd.Flags().StringVar(&stateRaw, "state", "", "Comma-separated key=value pairs to seed Context (e.g. dsl_interface_changed=true,ticket_type=story)")
	return cmd
}

// newAtddDebugGateCmd runs a single gateway binding standalone and prints
// the resulting Outcome. Useful for "what would GATE_DSL return right now?".
func newAtddDebugGateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gate <binding>",
		Short: "Evaluate one gateway binding and print the Outcome",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			reg := gates.New()
			gates.RegisterAll(reg, gates.Deps{})
			fn := reg.Lookup(args[0])
			if fn == nil {
				exitOnError(fmt.Errorf("unknown gate binding %q", args[0]))
			}
			out := fn(statemachine.NewContext())
			if out.Err != nil {
				exitOnError(out.Err)
			}
			if out.Value != "" {
				fmt.Printf("value: %s\n", out.Value)
			} else {
				fmt.Printf("bool:  %t\n", out.Bool)
			}
		},
	}
	return cmd
}

// newAtddDebugReleaseCmd runs the release primitives end-to-end against a
// caller-supplied set of test roots. Useful for replaying a release on a
// dirty working tree without re-walking the pipeline.
func newAtddDebugReleaseCmd() *cobra.Command {
	var (
		issueArg string
		roots    []string
		message  string
		noClose  bool
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "release",
		Short: "Remove @Disabled markers, commit, and (optionally) close the issue",
		Run: func(cmd *cobra.Command, args []string) {
			issue, err := parseIssueArg(issueArg)
			exitOnError(err)
			if message == "" {
				message = fmt.Sprintf("Release | issue #%d", issue)
			}
			if len(roots) == 0 {
				roots = []string{
					"system-test/java",
					"system-test/dotnet",
					"system-test/typescript",
				}
			}
			changes, err := release.RemoveDisabledMarkers(context.Background(), release.RemoveOptions{
				Roots:  roots,
				DryRun: dryRun,
			})
			exitOnError(err)
			for _, c := range changes.Files {
				fmt.Printf("edited: %s (pattern=%s, removed=%v, edited=%v)\n",
					c.Path, c.PatternName, c.LinesRemoved, c.LinesEdited)
			}
			if dryRun {
				fmt.Println("dry-run: no commit / close")
				return
			}
			confirm := release.InteractiveConfirmer(os.Stdin, os.Stdout)
			exitOnError(release.Commit(context.Background(), release.CommitOptions{
				Message: message,
				Confirm: confirm,
			}))
			if !noClose {
				exitOnError(release.CloseIssue(context.Background(), issue, nil))
			}
		},
	}
	cmd.Flags().StringVar(&issueArg, "issue", "", "GitHub issue number or URL (required)")
	cmd.Flags().StringSliceVar(&roots, "root", nil, "Test root to walk (repeatable; defaults to system-test/{java,dotnet,typescript})")
	cmd.Flags().StringVar(&message, "message", "", "Commit message (defaults to 'Release | issue #N')")
	cmd.Flags().BoolVar(&noClose, "no-close", false, "Skip the gh issue close step")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print what would change without writing files or committing")
	return cmd
}

// parseIssueArg accepts either a bare issue number ("42") or a GitHub issue
// URL ("https://github.com/owner/repo/issues/42") and returns the integer
// issue number. Mirrors `gh issue view`, which accepts both forms.
func parseIssueArg(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("--issue is required")
	}
	s = strings.TrimRight(s, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimPrefix(s, "#")
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("cannot parse issue number from %q", s)
	}
	if n <= 0 {
		return 0, fmt.Errorf("--issue must be positive (got %d)", n)
	}
	return n, nil
}
