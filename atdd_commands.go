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
//
// Process-flow / agent-prompt / per-node overrides are project-stable
// values, so they live in `gh-optivem.yaml` (process_flow:, agent_prompts:,
// node_extras:, node_replacements:) — not on flags. The cobra layer loads
// the config once per run, reads those fields, and threads them into
// driver.Options.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/board"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/classify"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/diagram"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/driver"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/gates"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/override"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/preflight"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/release"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/configinit"
	"github.com/optivem/gh-optivem/internal/projectconfig"
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
// Context, and walks the main process from MOVE_TICKET_IN_PROGRESS (skipping the
// PICK_TOP_READY picker).
func newAtddImplementTicketCmd() *cobra.Command {
	var (
		issueArg     string
		autonomous   bool
		manualAgents bool
		logFile      string
		workspace    string
		keepRuns     int
		showPrompt   bool
	)
	cmd := &cobra.Command{
		Use:   "implement-ticket",
		Short: "Walk the ATDD pipeline for a specific GitHub issue",
		Example: `  gh optivem atdd implement-ticket --issue 42
  gh optivem atdd implement-ticket --issue https://github.com/optivem/shop/issues/42
  gh optivem -c ./optivem-multitier.yaml atdd implement-ticket --issue 42
  gh optivem atdd implement-ticket --issue 42 --workspace /abs/path/to/workspace
  gh optivem atdd implement-ticket --issue 42 --log-file run.log
  gh optivem atdd implement-ticket --issue 42 --show-prompt
  gh optivem atdd implement-ticket --issue 42 --keep-runs 0   # never prune`,
		Run: func(cmd *cobra.Command, args []string) {
			issue, err := parseIssueArg(issueArg)
			exitOnError(err)
			exitOnError(validateKeepRuns(keepRuns))
			resolvedConfigPath, _ := projectconfig.ResolvePath(projectConfigPath)
			exportConfigForShellOuts(resolvedConfigPath)
			cfg, err := runImplementTicketPreflight(resolvedConfigPath, workspace)
			exitOnError(err)
			hooks, err := overrideHooksFromConfig(cfg)
			exitOnError(err)
			promptOverrides, err := agentPromptOverridesFromConfig(cfg)
			exitOnError(err)
			exitOnError(driver.Run(context.Background(), driver.Options{
				IssueNum:             issue,
				Autonomous:           autonomous,
				ManualAgents:         manualAgents,
				Override:             hooks,
				YAMLPath:             cfg.ProcessFlow,
				AgentPromptOverrides: promptOverrides,
				ConfigPath:           resolvedConfigPath,
				LogFile:              logFile,
				KeepRuns:             keepRuns,
				ShowPrompt:           showPrompt,
			}))
		},
	}
	cmd.Flags().StringVar(&issueArg, "issue", "", "GitHub issue number or URL (required; accepts e.g. 42 or https://github.com/owner/repo/issues/42)")
	cmd.Flags().BoolVar(&autonomous, "autonomous", false, "Skip human-approval STOPs and run agent dispatches headless via `claude -p`")
	cmd.Flags().BoolVar(&manualAgents, "manual-agents", false, "Fall back to v1 manual dispatch: pause and let the operator launch each agent in a separate window")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root containing one clone per repo (default: parent directory of CWD). Each clone dir must be named after the repo-name component of its slug; symlink outliers into place.")
	cmd.Flags().StringVar(&logFile, "log-file", "", "Mirror everything stdout/stderr emit during the run to this file (in addition to streaming live)")
	cmd.Flags().IntVar(&keepRuns, "keep-runs", 10, "Max prompt-log run directories to keep under .gh-optivem/runs/ at startup (0 = never prune)")
	cmd.Flags().BoolVar(&showPrompt, "show-prompt", false, "Dump each agent's full rendered prompt to stdout before dispatch (default: summary banner only)")
	return cmd
}

// validateKeepRuns surfaces the documented "negative values are rejected"
// rule at flag-parse time. 0 is allowed (means "never prune").
func validateKeepRuns(n int) error {
	if n < 0 {
		return fmt.Errorf("--keep-runs must be >= 0 (got %d); use 0 to disable pruning", n)
	}
	return nil
}

// exportConfigForShellOuts publishes the resolved gh-optivem.yaml path on
// $GH_OPTIVEM_CONFIG so every child `gh optivem ...` shell-out fired by
// ATDD bindings picks up the same project config without needing per-call
// --config flags. An absolute path is exported so the value still resolves
// after children chdir. A startup banner echoes the active config so the
// operator can reproduce shell-outs by hand. Failures from filepath.Abs
// are non-fatal — the runner's own ./gh-optivem.yaml fallback still
// applies.
func exportConfigForShellOuts(resolvedConfigPath string) {
	abs, err := filepath.Abs(resolvedConfigPath)
	if err != nil {
		return
	}
	_ = os.Setenv("GH_OPTIVEM_CONFIG", abs)
	fmt.Fprintf(os.Stdout, "Using gh-optivem config: %s\n", abs)
}

// runImplementTicketPreflight loads the consumer's gh-optivem.yaml at the
// resolved configPath (always non-empty after projectconfig.ResolvePath:
// flag > env > <cwd>/gh-optivem.yaml) and runs the on-disk preflight before
// any board, classify, or agent dispatch happens. Missing file is a hard
// error — atdd is config-driven and there's nothing useful to preflight
// without it. A failure prints one error block listing every missing repo
// or path and exits non-zero — see preflight.Run.
//
// Returns the loaded cfg so the cobra layer can read process_flow:,
// agent_prompts:, node_extras:, and node_replacements: without paying
// for a second LoadFromPath. The driver still re-loads internally via
// loadDriverConfig — the double load is deliberate and a config file is
// small enough that the second read is free.
func runImplementTicketPreflight(configPath string, workspace string) (*projectconfig.Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("preflight: getwd: %w", err)
	}
	if err := configinit.EnsureExists(configPath); err != nil {
		return nil, err
	}
	cfg, err := projectconfig.LoadFromPath(configPath)
	if err != nil {
		return nil, err
	}
	opts, err := defaultPreflightOptions(cfg, workspace, cwd)
	if err != nil {
		return nil, err
	}
	if err := preflight.Run(context.Background(), cfg, opts); err != nil {
		return nil, err
	}
	return cfg, nil
}

// newAtddManageProjectCmd implements `gh optivem atdd manage-project`. The
// driver picks the top item from the Ready column and walks the main process
// from START.
func newAtddManageProjectCmd() *cobra.Command {
	var (
		autonomous   bool
		manualAgents bool
		logFile      string
		keepRuns     int
		showPrompt   bool
	)
	cmd := &cobra.Command{
		Use:   "manage-project",
		Short: "Pick the top Ready ticket and walk the ATDD pipeline",
		Example: `  gh optivem atdd manage-project
  gh optivem -c ./optivem-multitier.yaml atdd manage-project
  gh optivem atdd manage-project --log-file run.log`,
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(validateKeepRuns(keepRuns))
			resolvedConfigPath, _ := projectconfig.ResolvePath(projectConfigPath)
			exportConfigForShellOuts(resolvedConfigPath)
			cfg, err := projectconfig.LoadFromPath(resolvedConfigPath)
			exitOnError(err)
			hooks, err := overrideHooksFromConfig(cfg)
			exitOnError(err)
			promptOverrides, err := agentPromptOverridesFromConfig(cfg)
			exitOnError(err)
			exitOnError(driver.Run(context.Background(), driver.Options{
				Autonomous:           autonomous,
				ManualAgents:         manualAgents,
				Override:             hooks,
				YAMLPath:             cfg.ProcessFlow,
				AgentPromptOverrides: promptOverrides,
				ConfigPath:           resolvedConfigPath,
				LogFile:              logFile,
				KeepRuns:             keepRuns,
				ShowPrompt:           showPrompt,
			}))
		},
	}
	cmd.Flags().BoolVar(&autonomous, "autonomous", false, "Skip human-approval STOPs and run agent dispatches headless via `claude -p`")
	cmd.Flags().BoolVar(&manualAgents, "manual-agents", false, "Fall back to v1 manual dispatch: pause and let the operator launch each agent in a separate window")
	cmd.Flags().StringVar(&logFile, "log-file", "", "Mirror everything stdout/stderr emit during the run to this file (in addition to streaming live)")
	cmd.Flags().IntVar(&keepRuns, "keep-runs", 10, "Max prompt-log run directories to keep under .gh-optivem/runs/ at startup (0 = never prune)")
	cmd.Flags().BoolVar(&showPrompt, "show-prompt", false, "Dump each agent's full rendered prompt to stdout before dispatch (default: summary banner only)")
	return cmd
}

// overrideHooksFromConfig builds the per-node override hooks the driver
// passes to the dispatcher, sourced from cfg.NodeExtras (inline text,
// appended at dispatch) and cfg.NodeReplacements (paths whose file body
// replaces the prompt verbatim). Returns (nil, nil) when both maps are
// empty so the driver's wrapOverride sees a no-op decorator rather than
// an empty-but-allocated struct. Node-ID validity against the resolved
// process flow is enforced by the driver at startup; this layer only
// reads the file bodies.
func overrideHooksFromConfig(cfg *projectconfig.Config) (*override.Hooks, error) {
	if cfg == nil || (len(cfg.NodeExtras) == 0 && len(cfg.NodeReplacements) == 0) {
		return nil, nil
	}
	hooks := &override.Hooks{}
	if len(cfg.NodeExtras) > 0 {
		hooks.Extra = cfg.NodeExtras
	}
	if len(cfg.NodeReplacements) > 0 {
		replace := make(map[string]string, len(cfg.NodeReplacements))
		for node, path := range cfg.NodeReplacements {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("node_replacements[%s]: read %s: %w", node, path, err)
			}
			replace[node] = string(data)
		}
		hooks.Replace = replace
	}
	return hooks, nil
}

// agentPromptOverridesFromConfig reads cfg.AgentPrompts (agent-name → path)
// and returns the agent-name → prompt-body map the driver passes through to
// the dispatcher. Files are read at startup so missing paths surface there,
// not deep inside a pipeline run. Agent-name validity is enforced by
// projectconfig.Validate (Rule 11) — this layer only reads the files.
// Returns (nil, nil) when AgentPrompts is empty.
func agentPromptOverridesFromConfig(cfg *projectconfig.Config) (map[string]string, error) {
	if cfg == nil || len(cfg.AgentPrompts) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(cfg.AgentPrompts))
	for name, path := range cfg.AgentPrompts {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("agent_prompts[%s]: read %s: %w", name, path, err)
		}
		out[name] = string(data)
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
	cmd := &cobra.Command{
		Use:   "pick-top-ready",
		Short: "Print the top Ready item without moving it",
		Run: func(cmd *cobra.Command, args []string) {
			pick, err := board.PickTopReady(context.Background(), board.Options{})
			exitOnError(err)
			fmt.Printf("issue:    #%d\n", pick.IssueNum)
			fmt.Printf("title:    %s\n", pick.Title)
			fmt.Printf("repo:     %s\n", pick.Repo)
			fmt.Printf("url:      %s\n", pick.IssueURL)
			fmt.Printf("project:  %s\n", pick.ProjectID)
			fmt.Printf("item:     %s\n", pick.ItemID)
		},
	}
	return cmd
}

// newAtddDebugClassifyCmd runs classify.Classify and prints the result. No
// orchestration side-effects. Falls back to gh's CWD git-remote inference
// for the repo — operators debugging classify must cd into a clone of the
// target repo first.
func newAtddDebugClassifyCmd() *cobra.Command {
	var issueArg string
	cmd := &cobra.Command{
		Use:   "classify",
		Short: "Classify a ticket via the deterministic fast path",
		Run: func(cmd *cobra.Command, args []string) {
			issue, err := parseIssueArg(issueArg)
			exitOnError(err)
			res, err := classify.Classify(context.Background(), issue, classify.Options{})
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
	return cmd
}

// newAtddDebugNextPhaseCmd loads the YAML, builds an unbound Engine, and
// prints the outgoing edge that nextEdge would pick from a given node under
// a synthetic state. Useful for "why did the driver follow the No edge?".
// Operates against the configured process flow (gh-optivem.yaml's
// process_flow:) when set, otherwise the embedded canonical document. To
// poke an alternate flow, point --config at a gh-optivem.yaml whose
// process_flow: names it.
func newAtddDebugNextPhaseCmd() *cobra.Command {
	var (
		processName string
		nodeID      string
		stateRaw    string
	)
	cmd := &cobra.Command{
		Use:   "next-phase",
		Short: "Print the next node nextEdge would pick from --node under --state",
		Example: `  gh optivem atdd debug next-phase --node GATE_DSL --state dsl_interface_changed=true
  gh optivem atdd debug next-phase --process at_cycle --node AT_RED_TEST_COMMIT`,
		Run: func(cmd *cobra.Command, args []string) {
			if processName == "" {
				processName = driver.DefaultProcessName
			}
			if nodeID == "" {
				exitOnError(fmt.Errorf("--node is required"))
			}
			cfg, err := loadDebugConfig()
			exitOnError(err)
			var eng *statemachine.Engine
			if cfg != nil && cfg.ProcessFlow != "" {
				eng, err = statemachine.LoadFile(cfg.ProcessFlow)
			} else {
				eng, err = statemachine.LoadDefault()
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
			next, err := eng.NextEdge(processName, nodeID, sCtx)
			exitOnError(err)
			fmt.Printf("from:  %s\n", nodeID)
			fmt.Printf("to:    %s\n", next)
		},
	}
	cmd.Flags().StringVar(&processName, "process", "", "Process name (defaults to main)")
	cmd.Flags().StringVar(&nodeID, "node", "", "Source node ID (required)")
	cmd.Flags().StringVar(&stateRaw, "state", "", "Comma-separated key=value pairs to seed Context (e.g. dsl_interface_changed=true,ticket_type=story)")
	return cmd
}

// loadDebugConfig soft-loads gh-optivem.yaml via flag > env > cwd default
// for the debug commands. Missing default-location file returns (nil, nil)
// so the debug commands keep working in repos without one; a missing file
// at an *explicit* --config / $GH_OPTIVEM_CONFIG target is an error.
func loadDebugConfig() (*projectconfig.Config, error) {
	path, explicit := projectconfig.ResolvePath(projectConfigPath)
	cfg, err := projectconfig.LoadFromPath(path)
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, fs.ErrNotExist) && !explicit {
		return nil, nil
	}
	return nil, err
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
