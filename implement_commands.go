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

	"github.com/optivem/gh-optivem/internal/approval"
	assetsync "github.com/optivem/gh-optivem/internal/assets/sync"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/driver"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/override"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/preflight"
	"github.com/optivem/gh-optivem/internal/cmdctx"
	"github.com/optivem/gh-optivem/internal/configinit"
	"github.com/optivem/gh-optivem/internal/projectconfig"
	"github.com/optivem/gh-optivem/internal/runner"
	"github.com/optivem/gh-optivem/internal/version"
)

// newImplementCmd implements `gh optivem implement --issue N|URL`. The
// driver pre-resolves the project item and walks the main process from
// START, which dispatches to the implement-ticket sub-process. --issue is
// required; the on-disk preflight runs before the driver starts.
func newImplementCmd() *cobra.Command {
	var (
		issueArg              string
		headless              bool
		autonomousDeprecated  bool
		manualAgents          bool
		logFile               string
		workspace             string
		keepRuns              int
		showPrompt            bool
	)
	cmd := &cobra.Command{
		Use:   "implement",
		Short: "Run the configured implementation pipeline on an issue",
		Long: `Run the implementation pipeline against a GitHub issue.

The pipeline is configured per-project via gh-optivem.yaml (process_flow:,
task_prompts:, node_extras:, node_replacements:). Today the bundled flow
is ATDD; future flows (TDD, DDD, or compositions) plug into the same
command.

--issue is required; the pipeline targets that specific issue.`,
		Example: `  gh optivem implement --issue 42
  gh optivem implement --issue https://github.com/myorg/myrepo/issues/42
  gh optivem -c ./optivem-multitier.yaml implement --issue 42
  gh optivem implement --issue 42 --workspace /abs/path/to/workspace
  gh optivem implement --issue 42 --log-file run.log
  gh optivem implement --issue 42 --show-prompt
  gh optivem implement --issue 42 --keep-runs 0   # never prune
  gh optivem --auto implement --issue 42 --headless   # auto-approve everything except commit/fix; run claude -p`,
		Run: func(cmd *cobra.Command, args []string) {
			// ATDD-consuming command: when the auto-sync escape hatch is
			// set, the pipeline needs assets at ~/.gh-optivem/references/
			// that the startup auto-sync would normally provide. Fail
			// fast rather than dispatch agents whose prompts reference
			// files that may be missing or out of date.
			exitOnError(requireFreshAssetsWhenEscapeHatchSet())

			if strings.TrimSpace(issueArg) == "" {
				exitOnError(errors.New("--issue is required"))
			}
			issue, err := parseIssueArg(issueArg)
			exitOnError(err)
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
			runErr := driver.Run(context.Background(), driver.Options{
				IssueNum:            issue,
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
			})
			if runErr == nil {
				printSystemEndpointsBanner(cmd.ErrOrStderr(), cfg)
			}
			exitOnError(runErr)
		},
	}
	cmd.Flags().StringVar(&issueArg, "issue", "", "GitHub issue number or URL (required)")
	cmd.Flags().BoolVar(&headless, "headless", false, "Run the claude subprocess in headless `claude -p` mode (no interactive UI; structured JSON envelope captured for the exit banner)")
	cmd.Flags().BoolVar(&autonomousDeprecated, "autonomous", false, "[Deprecated] Equivalent to --auto --headless. Will be removed in a future release.")
	cmd.Flags().BoolVar(&manualAgents, "manual-agents", false, "Fall back to v1 manual dispatch: pause and let the operator launch each agent in a separate window")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root containing one clone per repo (default: parent directory of CWD). Each clone dir must be named after the repo-name component of its slug; symlink outliers into place.")
	cmd.Flags().StringVar(&logFile, "log-file", "", "Mirror everything stdout/stderr emit during the run to this file (in addition to streaming live)")
	cmd.Flags().IntVar(&keepRuns, "keep-runs", 10, "Max prompt-log run directories to keep under .gh-optivem/runs/ at startup (0 = never prune)")
	cmd.Flags().BoolVar(&showPrompt, "show-prompt", false, "Dump each agent's full rendered prompt to stdout before dispatch (default: summary banner only)")
	return cmd
}

// printSystemEndpointsBanner prints the system endpoints and OK/DOWN verdict
// to w as a final banner after a successful implement run. Best-effort:
// missing system.config, an unreadable systems.yaml, or an empty configured
// path is silently skipped — failing to print URLs must never fail the
// implement command itself. The banner goes to stderr (operator-facing UI),
// not stdout — matches the rest of implement's exit-banner content.
func printSystemEndpointsBanner(w io.Writer, cfg *projectconfig.Config) {
	if cfg == nil || cfg.System.Config == "" {
		return
	}
	sys, err := runner.LoadSystem(cfg.System.Config)
	if err != nil {
		return
	}
	fmt.Fprintln(w, "\n=== System endpoints ===")
	_ = runner.Status(w, sys, runner.StatusOptions{})
}

// requireFreshAssetsWhenEscapeHatchSet returns the documented staleness
// error when GH_OPTIVEM_NO_AUTO_SYNC disables the startup auto-sync AND
// the stamp on disk does not match the binary version. With the escape
// hatch unset the startup hook has already synced (or attempted to), so
// this check is a no-op.
func requireFreshAssetsWhenEscapeHatchSet() error {
	if !assetsync.IsEscapeHatchSet() {
		return nil
	}
	stale, err := assetsync.Stale(version.Version)
	if err != nil {
		return err
	}
	if stale {
		return errors.New(assetsync.EscapeHatchHint)
	}
	return nil
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
	cfg, err := projectconfig.LoadFromPath(configPath)
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
