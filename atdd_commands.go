// atdd_commands.go wires the `gh optivem atdd …` subcommands into the root
// Cobra command. Two public commands mirror today's slash commands:
//
//	gh optivem atdd implement-ticket --issue N
//	gh optivem atdd manage-project
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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/diagram"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/driver"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/override"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/preflight"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/configinit"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// newAtddCmd builds the `gh optivem atdd` parent. The parent has no Run, so
// invoking it without a subcommand prints help.
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
