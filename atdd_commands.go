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

	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/board"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/classify"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/diagram"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/driver"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/gates"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/override"
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

The driver loads the canonical process-flow YAML embedded in gh-optivem
and walks it node by node — dispatching service tasks inline (board
picks, classification, smoke tests, commits) and pausing at each
user-task node so the operator can launch the corresponding Claude Code
agent (atdd-story, atdd-test, atdd-dsl, …). When the agent's COMMIT
lands on HEAD, the operator presses Enter and the engine moves on.`,
	}
	cmd.AddCommand(
		newAtddImplementTicketCmd(),
		newAtddManageProjectCmd(),
		newAtddShowCmd(),
		newAtddDebugCmd(),
	)
	return cmd
}

// newAtddShowCmd builds the `gh optivem atdd show` parent. Children
// emit human-readable views of the embedded orchestration artifacts —
// today only the rendered process-flow diagram. The intent is the
// "view, don't generate" principle: consumers and CI alike can read
// the same artifact gh-optivem produces, with no per-repo tooling.
func newAtddShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print embedded orchestration artifacts (diagram, …)",
	}
	cmd.AddCommand(newAtddShowDiagramCmd())
	return cmd
}

// newAtddShowDiagramCmd renders the canonical process-flow Mermaid
// markdown to stdout. CI's regenerate-diagram workflow pipes this
// output to docs/process-diagram.md and commits any diff.
func newAtddShowDiagramCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diagram",
		Short: "Render the process-flow Mermaid diagram to stdout",
		Example: `  gh optivem atdd show diagram
  gh optivem atdd show diagram > docs/process-diagram.md`,
		Run: func(cmd *cobra.Command, args []string) {
			eng, err := statemachine.LoadDefault()
			exitOnError(err)
			fmt.Print(diagram.Render(eng))
		},
	}
	return cmd
}

// newAtddImplementTicketCmd implements `gh optivem atdd implement-ticket
// --issue N`. The driver pre-resolves the project item for the issue, seeds
// Context, and walks the main flow from MOVE_TO_IN_PROGRESS (skipping the
// PICK_TOP_READY picker).
func newAtddImplementTicketCmd() *cobra.Command {
	var (
		issueArg          string
		projectURL        string
		autonomous        bool
		manualAgents      bool
		interactive       bool
		extraPairs        []string
		replacePairs      []string
		yamlPath          string
		agentPromptPairs  []string
		configPath        string
	)
	cmd := &cobra.Command{
		Use:   "implement-ticket",
		Short: "Walk the ATDD pipeline for a specific GitHub issue",
		Example: `  gh optivem atdd implement-ticket --issue 42
  gh optivem atdd implement-ticket --issue https://github.com/optivem/shop/issues/42
  gh optivem atdd implement-ticket --issue 42 --project https://github.com/orgs/optivem/projects/3
  gh optivem atdd implement-ticket --issue 42 --extra AT_RED_DSL_WRITE="prefer record types"
  gh optivem atdd implement-ticket --issue 42 --yaml ./alt-process-flow.yaml
  gh optivem atdd implement-ticket --issue 42 --agent-prompt atdd-test=./prompts/atdd-test.md
  gh optivem atdd implement-ticket --issue 42 --config ./optivem-multitier.yaml`,
		Run: func(cmd *cobra.Command, args []string) {
			issue, err := parseIssueArg(issueArg)
			exitOnError(err)
			hooks, err := buildOverrideHooks(extraPairs, replacePairs, interactive)
			exitOnError(err)
			promptOverrides, err := parseAgentPromptPairs(agentPromptPairs)
			exitOnError(err)
			exitOnError(driver.Run(context.Background(), driver.Options{
				IssueNum:             issue,
				ProjectURL:           projectURL,
				Autonomous:           autonomous,
				ManualAgents:         manualAgents,
				Override:             hooks,
				YAMLPath:             yamlPath,
				AgentPromptOverrides: promptOverrides,
				ConfigPath:           configPath,
			}))
		},
	}
	cmd.Flags().StringVar(&issueArg, "issue", "", "GitHub issue number or URL (required; accepts e.g. 42 or https://github.com/owner/repo/issues/42)")
	cmd.Flags().StringVar(&projectURL, "project", "", "GitHub project URL (optional; defaults to README.md or git-remote lookup)")
	cmd.Flags().BoolVar(&autonomous, "autonomous", false, "Skip human-approval STOPs and run agent dispatches headless via `claude -p`")
	cmd.Flags().BoolVar(&manualAgents, "manual-agents", false, "Fall back to v1 manual dispatch: pause and let the operator launch each agent in a separate window")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "Before each user_task dispatch, print the constructed prompt and read stdin for last-minute additions")
	cmd.Flags().StringSliceVar(&extraPairs, "extra", nil, "Per-node extra prompt text, repeatable (e.g. --extra AT_RED_DSL_WRITE=\"prefer record types\")")
	cmd.Flags().StringSliceVar(&replacePairs, "replace", nil, "Per-node prompt replacement, repeatable (escape hatch — full prompt swap)")
	cmd.Flags().StringVar(&yamlPath, "yaml", "", "Path to a process-flow YAML override (defaults to the embedded canonical document)")
	cmd.Flags().StringSliceVar(&agentPromptPairs, "agent-prompt", nil, "Override one named agent prompt, repeatable (e.g. --agent-prompt atdd-test=./prompts/atdd-test.md)")
	cmd.Flags().StringVar(&configPath, "config", "", "Path to a project config override (defaults to <repoPath>/optivem.yaml)")
	return cmd
}

// newAtddManageProjectCmd implements `gh optivem atdd manage-project`. The
// driver picks the top item from the Ready column and walks the main flow
// from START.
func newAtddManageProjectCmd() *cobra.Command {
	var (
		projectURL       string
		autonomous       bool
		manualAgents     bool
		interactive      bool
		extraPairs       []string
		replacePairs     []string
		yamlPath         string
		agentPromptPairs []string
		configPath       string
	)
	cmd := &cobra.Command{
		Use:   "manage-project",
		Short: "Pick the top Ready ticket and walk the ATDD pipeline",
		Example: `  gh optivem atdd manage-project
  gh optivem atdd manage-project --project https://github.com/orgs/optivem/projects/3
  gh optivem atdd manage-project --yaml ./alt-process-flow.yaml --config ./optivem-multitier.yaml`,
		Run: func(cmd *cobra.Command, args []string) {
			hooks, err := buildOverrideHooks(extraPairs, replacePairs, interactive)
			exitOnError(err)
			promptOverrides, err := parseAgentPromptPairs(agentPromptPairs)
			exitOnError(err)
			exitOnError(driver.Run(context.Background(), driver.Options{
				ProjectURL:           projectURL,
				Autonomous:           autonomous,
				ManualAgents:         manualAgents,
				Override:             hooks,
				YAMLPath:             yamlPath,
				AgentPromptOverrides: promptOverrides,
				ConfigPath:           configPath,
			}))
		},
	}
	cmd.Flags().StringVar(&projectURL, "project", "", "GitHub project URL (optional; defaults to README.md or git-remote lookup)")
	cmd.Flags().BoolVar(&autonomous, "autonomous", false, "Skip human-approval STOPs and run agent dispatches headless via `claude -p`")
	cmd.Flags().BoolVar(&manualAgents, "manual-agents", false, "Fall back to v1 manual dispatch: pause and let the operator launch each agent in a separate window")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "Before each user_task dispatch, print the constructed prompt and read stdin for last-minute additions")
	cmd.Flags().StringSliceVar(&extraPairs, "extra", nil, "Per-node extra prompt text, repeatable (e.g. --extra AT_RED_DSL_WRITE=\"prefer record types\")")
	cmd.Flags().StringSliceVar(&replacePairs, "replace", nil, "Per-node prompt replacement, repeatable (escape hatch — full prompt swap)")
	cmd.Flags().StringVar(&yamlPath, "yaml", "", "Path to a process-flow YAML override (defaults to the embedded canonical document)")
	cmd.Flags().StringSliceVar(&agentPromptPairs, "agent-prompt", nil, "Override one named agent prompt, repeatable (e.g. --agent-prompt atdd-test=./prompts/atdd-test.md)")
	cmd.Flags().StringVar(&configPath, "config", "", "Path to a project config override (defaults to <repoPath>/optivem.yaml)")
	return cmd
}

// buildOverrideHooks parses the --extra / --replace NODE=text pairs into
// override.Hooks. Returns nil (no overrides) when every list is empty and
// --interactive is off, so the driver's wrapOverride sees a no-op decorator
// rather than an empty-but-allocated struct. NODE keys are case-sensitive
// to match the YAML node ID convention (UPPER_SNAKE_CASE).
func buildOverrideHooks(extraPairs, replacePairs []string, interactive bool) (*override.Hooks, error) {
	if len(extraPairs) == 0 && len(replacePairs) == 0 && !interactive {
		return nil, nil
	}
	hooks := &override.Hooks{Interactive: interactive}
	if len(extraPairs) > 0 {
		extra, err := parseNodeKVPairs("--extra", extraPairs)
		if err != nil {
			return nil, err
		}
		hooks.Extra = extra
	}
	if len(replacePairs) > 0 {
		replace, err := parseNodeKVPairs("--replace", replacePairs)
		if err != nil {
			return nil, err
		}
		hooks.Replace = replace
	}
	return hooks, nil
}

// parseAgentPromptPairs splits each `name=path` pair, validates that name
// is a known embedded agent (typos surface at startup, not deep inside a
// pipeline run), reads the file at startup (so missing files also surface
// at startup), and returns the agent-name → prompt-body map the driver
// passes through to the dispatcher. Returns (nil, nil) when pairs is
// empty so the driver sees a clean nil map rather than an empty one.
func parseAgentPromptPairs(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	known := map[string]bool{}
	for _, n := range agents.Names() {
		known[n] = true
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		name, path, ok := strings.Cut(p, "=")
		if !ok {
			return nil, fmt.Errorf("--agent-prompt value %q must be name=path", p)
		}
		name = strings.TrimSpace(name)
		path = strings.TrimSpace(path)
		if name == "" {
			return nil, fmt.Errorf("--agent-prompt value %q has empty name", p)
		}
		if path == "" {
			return nil, fmt.Errorf("--agent-prompt %q has empty path", name)
		}
		if !known[name] {
			return nil, fmt.Errorf("--agent-prompt name %q is not a known embedded agent", name)
		}
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("--agent-prompt name %q specified more than once", name)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("--agent-prompt %s: read %s: %w", name, path, err)
		}
		out[name] = string(data)
	}
	return out, nil
}

// parseNodeKVPairs splits each `NODE=text` pair on the first `=`. Empty
// node IDs and duplicate node IDs are user errors — they almost always
// indicate a typo in the flag invocation, and silently merging would lead
// to "why did my override not apply?" debugging.
func parseNodeKVPairs(flag string, pairs []string) (map[string]string, error) {
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return nil, fmt.Errorf("%s value %q must be NODE=text", flag, p)
		}
		k = strings.TrimSpace(k)
		if k == "" {
			return nil, fmt.Errorf("%s value %q has empty NODE", flag, p)
		}
		if _, dup := out[k]; dup {
			return nil, fmt.Errorf("%s NODE %q specified more than once", flag, k)
		}
		out[k] = v
	}
	return out, nil
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
			if flowName == "" {
				flowName = driver.DefaultFlowName
			}
			if nodeID == "" {
				exitOnError(fmt.Errorf("--node is required"))
			}
			var eng *statemachine.Engine
			var err error
			if yamlPath == "" {
				eng, err = statemachine.LoadDefault()
			} else {
				eng, err = statemachine.LoadFile(yamlPath)
			}
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
	cmd.Flags().StringVar(&yamlPath, "yaml", "", "Path to a process-flow YAML override (defaults to the embedded canonical document)")
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
