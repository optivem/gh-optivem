// implement_commands.go wires the `gh optivem implement` subcommand into the
// root Cobra command. The `implement` verb walks the configured pipeline
// against a specific issue identified by --issue.
//
//	gh optivem implement --issue 42     # walk the pipeline for a specific issue
//
// The handler is deliberately thin: it translates Cobra flags into
// internal/atdd/runtime/* calls and surfaces their errors via exitOnError
// (defined in runner_helpers.go) for consistency with the rest of the
// `optivem` binary.
//
// Why "implement" as a top-level verb (not under a methodology noun): the
// pipeline orchestrated here is the stable concept; *which* methodology runs
// (ATDD today, TDD/DDD or compositions later) is configuration read from
// `gh-optivem.yaml` (process_flow:, task_prompts:, node_extras:,
// node_replacements:). Hoisting `implement` to the root keeps the muscle
// memory ("implement an issue") even when a second methodology lands.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/kernel/approval"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/configcheck"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/driver"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/outlog"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/override"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/preflight"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
	"github.com/optivem/gh-optivem/internal/kernel/cmdctx"
	"github.com/optivem/gh-optivem/internal/configinit"
	"github.com/optivem/gh-optivem/internal/projectconfig"
	"github.com/optivem/gh-optivem/internal/build/runner"
)

// newImplementCmd implements `gh optivem implement --issue N|URL`. The
// driver pre-resolves the project item and walks the main process from
// START, which dispatches to the implement-ticket sub-process. --issue is
// required; the on-disk preflight runs before the driver starts.
func newImplementCmd() *cobra.Command {
	var (
		issueArg              string
		targetArg             string
		channelArg            string
		headless              bool
		autonomousDeprecated  bool
		manualAgents          bool
		logFile               string
		logLevelArg           string
		verbose               bool
		workspace             string
		keepRuns              int
		showPrompt            bool
	)
	cmd := &cobra.Command{
		Use:   "implement [issue]",
		Short: "Run the configured implementation pipeline on an issue",
		Long: `Run the implementation pipeline against a GitHub issue.

The pipeline is configured per-project via gh-optivem.yaml (process_flow:,
task_prompts:, node_extras:, node_replacements:). Today the bundled flow
is ATDD; future flows (TDD, DDD, or compositions) plug into the same
command.

Identify the issue with a positional argument or --issue (either a bare
number or a full issue URL); supply exactly one. With no --target the
command walks the WHOLE pipeline for that issue, start to end — the
fullstack-developer default.

Team handoff (--target): a ticket can instead be produced in slices that
different teams own, handed off via commit. --target names a contiguous
slice of the pipeline:

  test               shared, channel-agnostic contract (acceptance tests +
                     DSL + driver ports + external system) the whole team
                     mobs. Ends RED by design. --channel is rejected.
  driver-adapter     one channel's test-side System Driver adapter.
  system             one channel's system (the first channel also builds
                     the channel-agnostic common layer).

--channel <ch> selects which channel for the two channel-split slices
(required there, rejected for test); the token is validated against the
project's channels: list. There is no resume status file — a slice reads
how far the ticket got from the committed tree, so it refuses to start
until its upstream slice is committed.

Recovery from crashed runs: if a run is force-cancelled mid-dispatch
(Ctrl+C in the parent terminal, terminal closed, kernel kill, panic in a
child), orphan headless claude subprocesses may survive the parent's
exit. Run 'gh optivem doctor --orphans' to list them and interactively
kill them.`,
		Example: `  gh optivem implement 42                              # full pipeline (positional issue)
  gh optivem implement --issue 42                      # full pipeline (flag form)
  gh optivem implement --issue https://github.com/myorg/myrepo/issues/42
  gh optivem -c ./optivem-multitier.yaml implement 42

  # Team handoff — mob the shared contract, then each channel team owns its channel:
  gh optivem implement 42 --target test                          # whole team: shared RED contract
  gh optivem implement 42 --target driver-adapter --channel api  # API team: its driver adapter
  gh optivem implement 42 --target system --channel api          # API team: its system (channel green)
  gh optivem implement 42 --target driver-adapter --channel ui   # UI team: its driver adapter
  gh optivem implement 42 --target system --channel ui           # UI team: its system

  gh optivem implement 42 --workspace /abs/path/to/workspace
  gh optivem implement 42 --log-file run.log
  gh optivem implement 42 --verbose                    # stream full firehose to terminal
  gh optivem implement 42 --log-file run.log --log-level phase  # quiet log
  gh optivem implement 42 --show-prompt
  gh optivem implement 42 --keep-runs 0   # never prune
  gh optivem --auto implement 42 --headless   # auto-approve everything except commit/fix; run claude -p`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			issueSource, err := resolveIssueSource(issueArg, args)
			exitOnError(err)
			issue, err := parseIssueArg(issueSource)
			exitOnError(err)
			// --target enum validation lives in ParseTarget (the SSoT shared with
			// the driver); the --channel rule (required/rejected per slice,
			// membership against channels:) is enforced once in the driver's
			// resolveScopedEntry, so the flag layer stays a thin parse wrapper.
			// The one check only the flag layer can make is coherence: a
			// --channel with no --target would otherwise be silently dropped.
			target, err := driver.ParseTarget(targetArg)
			exitOnError(err)
			if target == driver.TargetUnset && strings.TrimSpace(channelArg) != "" {
				exitOnError(errors.New("--channel requires --target (channel applies to a scoped slice)"))
			}
			exitOnError(validateKeepRuns(keepRuns))

			// --autonomous is a deprecated alias for --auto --headless.
			// Emit one stderr warning, then turn on headless and (if not
			// already auto) replace the resolved Approval with one in
			// which Auto=true. The aliasing happens here, post-flag-parse
			// but pre-driver, so the rest of the command sees the same
			// policy it would under the new flags.
			if autonomousDeprecated {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"gh optivem: --autonomous is deprecated; use --auto --headless")
				headless = true
				if existing := cmdctx.Approval(cmd); !existing.Auto {
					r, err := approval.Resolve(true, true, "", false, os.Getenv)
					exitOnError(err)
					cmd.SetContext(cmdctx.WithApproval(cmd.Context(), r))
				}
			}

			resolvedConfigPath, _ := projectconfig.ResolvePath(projectConfigPath)
			exportConfigForShellOuts(resolvedConfigPath)
			cfg, err := runImplementPreflight(resolvedConfigPath, workspace, manualAgents)
			exitOnError(err)
			hooks, err := overrideHooksFromConfig(cfg)
			exitOnError(err)
			promptOverrides, err := taskPromptOverridesFromConfig(cfg)
			exitOnError(err)
			logFileLevel, err := outlog.ParseLevel(logLevelArg)
			exitOnError(err)
			terminalLevel := outlog.Phase
			if verbose {
				terminalLevel = outlog.Detail
			}
			var resolved tracker.Issue
			runErr := driver.Run(context.Background(), driver.Options{
				IssueNum:            issue,
				Target:              target,
				Channel:             strings.TrimSpace(channelArg),
				ResolvedIssue:       &resolved,
				Headless:            headless,
				ManualAgents:        manualAgents,
				Override:            hooks,
				YAMLPath:            cfg.ProcessFlow,
				TaskPromptOverrides: promptOverrides,
				ConfigPath:          resolvedConfigPath,
				LogFile:             logFile,
				KeepRuns:            keepRuns,
				ShowPrompt:          showPrompt,
				Approval:            cmdctx.Approval(cmd),
				TerminalLevel:       terminalLevel,
				LogFileLevel:        logFileLevel,
			})
			if runErr == nil {
				printRunEndBanner(cmd.ErrOrStderr(), cfg, resolved.URL, resolved.Title)
			}
			exitOnError(runErr)
		},
	}
	cmd.Flags().StringVar(&issueArg, "issue", "", "GitHub issue number or URL (or pass it positionally)")
	cmd.Flags().StringVar(&targetArg, "target", "", "Scope the run to one pipeline slice: test | driver-adapter | system (default: walk the whole pipeline)")
	cmd.Flags().StringVar(&channelArg, "channel", "", "Channel for a channel-split --target (driver-adapter / system); validated against the project's channels:. Rejected for --target test")
	cmd.Flags().BoolVar(&headless, "headless", false, "Run the claude subprocess in headless `claude -p` mode (no interactive UI; structured JSON envelope captured for the exit banner)")
	cmd.Flags().BoolVar(&autonomousDeprecated, "autonomous", false, "[Deprecated] Equivalent to --auto --headless. Will be removed in a future release.")
	cmd.Flags().BoolVar(&manualAgents, "manual-agents", false, "Fall back to v1 manual dispatch: pause and let the operator launch each agent in a separate window")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root containing one clone per repo (default: parent directory of CWD). Each clone dir must be named after the repo-name component of its slug; symlink outliers into place.")
	cmd.Flags().StringVar(&logFile, "log-file", "", "Mirror everything stdout/stderr emit during the run to this file (in addition to streaming live)")
	cmd.Flags().StringVar(&logLevelArg, "log-level", "detail", "Verbosity of --log-file: phase (BPMN trace + prompts only) or detail (everything, default)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Stream the full firehose (subprocess output, agent body, prompt-prep banners) to the terminal. Default: terminal shows only BPMN trace lines, agent enter/exit banners, and prompts")
	cmd.Flags().IntVar(&keepRuns, "keep-runs", 10, "Max prompt-log run directories to keep under .gh-optivem/runs/ at startup (0 = never prune)")
	cmd.Flags().BoolVar(&showPrompt, "show-prompt", false, "Dump each agent's full rendered prompt to stdout before dispatch (default: summary banner only)")
	return cmd
}

// printRunEndBanner prints the post-run exit banner: a "Ticket: <url>" line
// (with the issue title appended in quotes when known) followed by the
// per-system status block emitted by runner.Status. Best-effort for the
// system block: missing system.config, an unreadable systems.yaml, or an
// empty configured path is silently skipped — failing to print URLs must
// never fail the implement command itself. The ticket line is independent
// and still prints when the system block is skipped. The banner goes to
// stderr (operator-facing UI), not stdout — matches the rest of implement's
// exit-banner content.
func printRunEndBanner(w io.Writer, cfg *projectconfig.Config, ticketURL, ticketTitle string) {
	if ticketURL != "" {
		if ticketTitle != "" {
			fmt.Fprintf(w, "\nTicket: %s %q\n", ticketURL, ticketTitle)
		} else {
			fmt.Fprintf(w, "\nTicket: %s\n", ticketURL)
		}
	}
	if cfg == nil || cfg.System.Config == "" {
		return
	}
	sys, err := runner.LoadSystem(cfg.System.Config)
	if err != nil {
		return
	}
	fmt.Fprintln(w)
	_ = runner.Status(w, sys, runner.StatusOptions{})
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
// $GH_OPTIVEM_CONFIG so every child `gh optivem ...` shell-out fired by the
// pipeline bindings picks up the same project config without needing per-call
// --config flags. An absolute path is exported so the value still resolves
// after children chdir. A startup banner echoes the active config so the
// operator can reproduce shell-outs by hand. Failures from filepath.Abs are
// non-fatal — the runner's own ./gh-optivem.yaml fallback still applies.
func exportConfigForShellOuts(resolvedConfigPath string) {
	abs, err := filepath.Abs(resolvedConfigPath)
	if err != nil {
		return
	}
	_ = os.Setenv("GH_OPTIVEM_CONFIG", abs)
	fmt.Fprintf(os.Stdout, "Using gh-optivem config: %s\n", abs)
}

// runImplementPreflight loads the consumer's gh-optivem.yaml at the resolved
// configPath (always non-empty after projectconfig.ResolvePath: flag > env >
// <cwd>/gh-optivem.yaml) and runs the on-disk preflight before any board,
// classify, or agent dispatch happens. Missing file is a hard error — the
// pipeline is config-driven and there's nothing useful to preflight without
// it. A failure prints one error block listing every missing repo or path
// and exits non-zero — see preflight.Run.
//
// manualAgents gates the claude-CLI presence check: when true (the v1
// two-window fallback) the check is skipped, so the operator can drive
// the pipeline without `claude` installed. When false, preflight.Run
// runs claude alongside its structural checks and a missing-or-broken
// CLI shows up in the same aggregated error block as any missing repos.
//
// Returns the loaded cfg so the cobra layer can read process_flow:,
// task_prompts:, node_extras:, and node_replacements: without paying for a
// second LoadFromPath. The driver still re-loads internally via
// loadDriverConfig — the double load is deliberate and a config file is
// small enough that the second read is free.
func runImplementPreflight(configPath string, workspace string, manualAgents bool) (*projectconfig.Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("preflight: getwd: %w", err)
	}
	if err := configinit.EnsureExists(configPath); err != nil {
		return nil, err
	}
	cfg, err := configcheck.LoadFromPath(configPath)
	if err != nil {
		return nil, err
	}
	opts, err := defaultPreflightOptions(cfg, workspace, cwd)
	if err != nil {
		return nil, err
	}
	if !manualAgents {
		opts.ClaudeCheck = preflight.VerifyClaude
	}
	// opts.Engine is wired by defaultPreflightOptions, so the scope and
	// suite-existence sweeps run here exactly as they do on
	// `gh optivem config preflight`.
	if err := preflight.Run(context.Background(), cfg, opts); err != nil {
		return nil, err
	}
	return cfg, nil
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
				return nil, fmt.Errorf("node-replacements[%s]: read %s: %w", node, path, err)
			}
			replace[node] = string(data)
		}
		hooks.Replace = replace
	}
	return hooks, nil
}

// taskPromptOverridesFromConfig reads cfg.TaskPrompts (task-name → path)
// and returns the task-name → prompt-body map the driver passes through to
// the dispatcher. Files are read at startup so missing paths surface there,
// not deep inside a pipeline run. Task-name validity is enforced by
// projectconfig.Validate (Rule 11) — this layer only reads the files.
// Returns (nil, nil) when TaskPrompts is empty.
func taskPromptOverridesFromConfig(cfg *projectconfig.Config) (map[string]string, error) {
	if cfg == nil || len(cfg.TaskPrompts) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(cfg.TaskPrompts))
	for name, path := range cfg.TaskPrompts {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("task-prompts[%s]: read %s: %w", name, path, err)
		}
		out[name] = string(data)
	}
	return out, nil
}

// resolveIssueSource reconciles the two ways to name an issue (D-positional):
// a positional argument (args[0]) or the --issue flag. Exactly one must be
// supplied. Both set is a conflict (the operator would not know which wins);
// neither is the "no issue" error. The Args constraint (MaximumNArgs(1))
// guarantees len(args) <= 1, so a non-empty args[0] is the positional form.
func resolveIssueSource(issueFlag string, args []string) (string, error) {
	flag := strings.TrimSpace(issueFlag)
	var positional string
	if len(args) == 1 {
		positional = strings.TrimSpace(args[0])
	}
	switch {
	case flag != "" && positional != "":
		return "", errors.New("specify the issue once: as a positional argument OR --issue, not both")
	case flag != "":
		return flag, nil
	case positional != "":
		return positional, nil
	default:
		return "", errors.New("provide an issue: a positional argument or --issue (a number or issue URL)")
	}
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
