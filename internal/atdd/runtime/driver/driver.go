// Package driver wires together the ATDD pipeline runtime: it loads the
// process-flow YAML, registers gates / actions / agents, applies override and
// verify decorators, and walks the named flow end to end.
//
// The driver is deliberately thin — the heavy lifting lives in the runtime
// sub-packages (statemachine, gates, actions, verify, override, board,
// classify, release, clauderun). This file's job is to compose them and
// expose two run modes:
//
//   - Run with Options.IssueNum > 0 → implement-ticket mode: pre-resolve the
//     project item for the given issue, seed Context, and skip the picker by
//     starting the main flow at MOVE_TO_IN_PROGRESS.
//   - Run with Options.IssueNum == 0 → manage-project mode: the YAML's main
//     flow runs from START, picking the top Ready ticket from the project
//     board.
//
// Agent dispatch (v2): every user_task whose `agent:` value is something
// other than `human` shells out to the `claude` CLI via the clauderun
// package. clauderun reads the embedded per-agent prompt (from
// internal/atdd/runtime/agents/prompts/), substitutes ${name} placeholders
// from the live Context, and hands the rendered string to `claude -p` as
// the agent's full one-shot input — there is no parent-claude harness or
// Task-tool indirection. Success is detected by HEAD diff (a fresh commit
// on the same branch). v1's "pause and let the operator launch the agent
// in a second window" behaviour is preserved as a fallback under
// Options.ManualAgents — it lets us bisect "did v2 misroute the agent?"
// against "did v1 see the commit?" without two parallel binaries.
package driver

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/actions"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/board"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/clauderun"
	"github.com/optivem/gh-optivem/internal/projectconfig"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/gates"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/override"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/verify"
)

// DefaultFlowName is the entry flow loaded by every public CLI command.
const DefaultFlowName = "main"

// Options bundles every driver knob that callers (the `gh optivem atdd …`
// commands and tests) might want to set. Zero values yield a usable
// configuration: load the embedded canonical YAML, enter DefaultFlowName,
// no overrides, real shell-outs.
type Options struct {
	// YAMLPath, when non-empty, points the driver at an on-disk YAML file
	// instead of the canonical embedded document (statemachine.DefaultYAML).
	// Empty → load the embedded YAML via statemachine.LoadDefault.
	YAMLPath string

	// FlowName is the entry flow. Empty → DefaultFlowName.
	FlowName string

	// IssueNum, when > 0, switches the driver into implement-ticket mode:
	// the picker (PICK_TOP_READY) is bypassed and the driver pre-resolves
	// the project item for the given issue.
	IssueNum int

	// ProjectURL overrides config-based project resolution. Optional; when
	// empty, board.ResolveProjectURL loads RepoPath/gh-optivem.yaml (or the
	// file passed via ConfigPath, if set) and reads `project.url`.
	ProjectURL string

	// RepoPath overrides the working directory used for project resolution
	// and for shell-outs. Optional; defaults to cwd.
	RepoPath string

	// Autonomous skips human-approval STOPs. In v2 it also flips agent
	// dispatch into headless `claude -p` mode. Default (false) runs
	// `claude` interactively so the operator can observe / interject.
	Autonomous bool

	// CLICommits flips dispatch into "CLI owns the commit" mode: instead
	// of polling HEAD for an agent-produced commit, the CLI stages and
	// commits the working-tree delta after the subprocess exits. Default
	// off; gated rollout per
	// plans/20260430-171111-cli-owns-commit-not-agent.md.
	CLICommits bool

	// ManualAgents falls back to the v1 "pause and let the operator
	// launch the agent in a second window" behaviour at every user_task
	// dispatch. Default (false) shells out to the `claude` CLI via the
	// clauderun package. ManualAgents is mutually exclusive with the
	// override hooks (the prompt-construction layer is what consumes
	// them — bypass that and they have nothing to attach to).
	ManualAgents bool

	// Override holds the per-node override hooks (Extra / Replace /
	// Interactive). v2 wires CLI flags into this struct; v1 leaves it nil.
	Override *override.Hooks

	// AgentPromptOverrides is a map from embedded-agent name (e.g.
	// "atdd-test") to a prompt body that replaces the canonical embedded
	// prompt for that agent. Wired from `--agent-prompt name=path` on
	// the CLI; the values are the file contents, not the file paths
	// (the CLI reads at parse time so missing-file failures surface at
	// startup). Unrecognised agent names are rejected at parse time.
	AgentPromptOverrides map[string]string

	// ConfigPath, when non-empty, overrides the default
	// `<repoPath>/gh-optivem.yaml` lookup with an explicit file path. Wired
	// from `--config <path>`; missing-file is an error (unlike the default
	// lookup, where absence is OK).
	ConfigPath string

	// ClaudeRunDeps lets tests inject fake `claude` and `git` runners
	// without spawning real subprocesses. Production callers leave this
	// zero-valued; clauderun falls back to real execClaude / execGit.
	ClaudeRunDeps clauderun.Deps

	// Stdout / Stderr are the diagnostic targets. nil → os.Stdout / os.Stderr.
	Stdout io.Writer
	Stderr io.Writer

	// Stdin is the agent-dispatch pause reader. nil → os.Stdin.
	Stdin io.Reader
}

// Run loads the YAML, wires the registries, applies decorators, optionally
// pre-resolves an issue, and walks the chosen flow.
func Run(ctx context.Context, opts Options) error {
	opts = opts.withDefaults()

	// Pre-flight the `claude` CLI when subprocess dispatch is enabled,
	// so missing-binary or missing-credentials failures surface at
	// startup instead of after several service-task spinners scroll by.
	// Skipped under --manual-agents (the v1 fallback that doesn't need
	// the CLI at all).
	if !opts.ManualAgents {
		if err := preflightFn(ctx); err != nil {
			return fmt.Errorf("driver: %w", err)
		}
	}

	var eng *statemachine.Engine
	var err error
	if opts.YAMLPath == "" {
		eng, err = statemachine.LoadDefault()
		if err != nil {
			return fmt.Errorf("driver: load embedded YAML: %w", err)
		}
	} else {
		eng, err = statemachine.LoadFile(opts.YAMLPath)
		if err != nil {
			return fmt.Errorf("driver: load YAML %q: %w", opts.YAMLPath, err)
		}
	}

	flow, ok := eng.Flows[opts.FlowName]
	if !ok {
		source := opts.YAMLPath
		if source == "" {
			source = "embedded"
		}
		return fmt.Errorf("driver: flow %q not in YAML %q", opts.FlowName, source)
	}

	gateReg := gates.New()
	gates.RegisterAll(gateReg, gates.Deps{})

	actionReg := actions.New()
	actions.RegisterAll(actionReg, actions.Deps{
		ProjectURL: opts.ProjectURL,
		RepoPath:   opts.RepoPath,
	})

	agentReg := agents.New()
	registerAgentDispatchers(agentReg)

	eng.GateFn = gateReg.Lookup
	eng.ActionFn = actionReg.Lookup
	eng.AgentFn = agentReg.Lookup

	if err := eng.Bind(); err != nil {
		return fmt.Errorf("driver: bind engine: %w", err)
	}

	// Post-Bind decoration order matters:
	//   1. Wrap user_task agent dispatch with per-node info-printer (uses
	//      RawNode metadata only available after Bind).
	//   2. Apply verify pre/post-condition decorators (commit-message HEAD
	//      checks).
	//   3. Apply override hooks last — they sit at the outermost layer so a
	//      v2 --replace short-circuits both the verify check and the agent
	//      dispatcher (which is the documented escape-hatch behaviour).
	wrapAgentDispatchers(eng, opts)
	verify.WrapAll(eng, verify.Deps{})
	wrapOverride(eng, opts.Override)

	repoPath, err := resolveRepoPath(opts.RepoPath)
	if err != nil {
		return fmt.Errorf("driver: %w", err)
	}
	cfg, err := loadDriverConfig(opts.ConfigPath, repoPath)
	if err != nil {
		return err
	}
	printConfig(opts.Stdout, opts, cfg, repoPath)

	sCtx := statemachine.NewContext()
	seedScopeParams(sCtx, cfg)
	if opts.ProjectURL != "" {
		sCtx.Set("project_url", opts.ProjectURL)
	}

	if opts.IssueNum > 0 {
		if err := preResolveIssue(ctx, opts, sCtx, cfg); err != nil {
			return fmt.Errorf("driver: pre-resolve issue #%d: %w", opts.IssueNum, err)
		}
		// Skip START → PICK_TOP_READY when running main. The pre-resolution
		// has already populated everything PICK_TOP_READY would have set;
		// MOVE_TO_IN_PROGRESS is the next service task downstream.
		if opts.FlowName == DefaultFlowName {
			flow.Start = "MOVE_TO_IN_PROGRESS"
		}
	}

	return eng.RunFlow(opts.FlowName, sCtx)
}

// resolveRepoPath returns the absolute path the driver treats as the
// consumer repo. Empty input falls back to the process CWD; non-empty
// is returned as-is (no canonicalisation, since callers and tests may
// pass either absolute or relative paths).
func resolveRepoPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return cwd, nil
}

// loadDriverConfig returns the parsed config for the run. When configPath
// is non-empty (operator passed `--config`), it must exist; missing-file
// is an error to catch typos. Empty configPath falls back to the default
// `<repoPath>/gh-optivem.yaml` lookup, where absence is OK (returns nil).
func loadDriverConfig(configPath, repoPath string) (*projectconfig.Config, error) {
	if configPath != "" {
		cfg, err := projectconfig.LoadFromPath(configPath)
		if err != nil {
			return nil, fmt.Errorf("driver: %w", err)
		}
		return cfg, nil
	}
	cfg, err := projectconfig.Load(repoPath)
	if err != nil {
		return nil, fmt.Errorf("driver: %w", err)
	}
	return cfg, nil
}

// printConfig writes a one-shot configuration banner to w so the operator
// sees the resolved consumer repo, config source, project URL/name, repo
// strategy, and scope axes at startup — well before the first agent
// dispatch (or pre-resolve failure) would otherwise reveal them.
//
// Script-specific values (worktree path, rehearsal branch, build source
// dir) deliberately stay in the rehearsal wrapper — those have no meaning
// outside the rehearsal scenario, so the binary doesn't know about them.
func printConfig(w io.Writer, opts Options, cfg *projectconfig.Config, repoPath string) {
	source := configSourceLabel(opts.ConfigPath, cfg, repoPath)
	fmt.Fprintln(w, "Configuration:")
	fmt.Fprintf(w, "  consumer repo: %s\n", repoPath)
	fmt.Fprintf(w, "  config file:   %s\n", source)
	if opts.IssueNum > 0 {
		fmt.Fprintf(w, "  issue:         #%d\n", opts.IssueNum)
	}
	projectURL := opts.ProjectURL
	projectURLNote := ""
	if projectURL == "" && cfg != nil {
		projectURL = cfg.Project.URL
	}
	if opts.ProjectURL != "" {
		projectURLNote = " (from --project)"
	}
	fmt.Fprintf(w, "  project URL:   %s%s\n", orPlaceholder(projectURL, "(unset — pre-resolve will fail)"), projectURLNote)
	if cfg != nil {
		fmt.Fprintf(w, "  repo strategy: %s\n", orPlaceholder(cfg.Project.RepoStrategy, "(unset)"))
		if len(cfg.Project.Repos) > 0 {
			fmt.Fprintf(w, "  repos:         %s\n", strings.Join(cfg.Project.Repos, ", "))
		}
		fmt.Fprintf(w, "  scope:         architecture=%s system_lang=%s test_lang=%s\n",
			orPlaceholder(cfg.Scope.Architecture, "-"),
			orPlaceholder(cfg.Scope.SystemLang, "-"),
			orPlaceholder(cfg.Scope.TestLang, "-"))
	}
}

// configSourceLabel returns a human-readable description of where the
// driver loaded its config from, suitable for the printConfig banner.
func configSourceLabel(explicitPath string, cfg *projectconfig.Config, repoPath string) string {
	if explicitPath != "" {
		return explicitPath + " (--config)"
	}
	if cfg != nil {
		return filepath.Join(repoPath, projectconfig.Path)
	}
	return "(none — no gh-optivem.yaml at repo root)"
}

func orPlaceholder(s, placeholder string) string {
	if s == "" {
		return placeholder
	}
	return s
}

// seedScopeParams copies repo-strategy and scope axes from a loaded config
// into Context.Params so agent prompts can substitute ${repo_strategy},
// ${repos}, ${architecture}, ${system_lang}, ${test_lang}. Empty values
// are left absent (the template var expands to ""). nil cfg is a no-op.
func seedScopeParams(sCtx *statemachine.Context, cfg *projectconfig.Config) {
	if cfg == nil {
		return
	}
	if cfg.Project.RepoStrategy != "" {
		sCtx.Params["repo_strategy"] = cfg.Project.RepoStrategy
	}
	if len(cfg.Project.Repos) > 0 {
		sCtx.Params["repos"] = strings.Join(cfg.Project.Repos, ",")
	}
	if cfg.Scope.Architecture != "" {
		sCtx.Params["architecture"] = cfg.Scope.Architecture
	}
	if cfg.Scope.SystemLang != "" {
		sCtx.Params["system_lang"] = cfg.Scope.SystemLang
	}
	if cfg.Scope.TestLang != "" {
		sCtx.Params["test_lang"] = cfg.Scope.TestLang
	}
}

// preflightFn is the per-Run function that verifies the `claude` CLI
// is on PATH and authenticated. Production points at preflightClaude;
// tests can swap it for a no-op or canned-error stub. The seam is a
// package-level var rather than an Options field because pre-flight is
// a startup-time concern, not part of the per-run dispatch surface.
var preflightFn = preflightClaude

// preflightClaude runs `claude --no-update-check --version` as a cheap
// health check at driver startup. Failure surfaces with operator
// guidance pointing at the auth bootstrap doc — without this, missing
// credentials manifest as a confusing "exited non-zero" several
// service-task spinners deep into the run.
func preflightClaude(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "claude", "--no-update-check", "--version")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		tail := strings.TrimSpace(stderr.String())
		if tail == "" {
			tail = err.Error()
		}
		return fmt.Errorf(
			"claude CLI pre-flight failed: %s\n  Ensure `claude` is on PATH and authenticated via `claude /login` (credentials live in ~/.claude/).\n  Use --manual-agents to fall back to the v1 two-window workflow without the CLI.",
			tail)
	}
	return nil
}

func (o Options) withDefaults() Options {
	if o.FlowName == "" {
		o.FlowName = DefaultFlowName
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if o.Stdin == nil {
		o.Stdin = os.Stdin
	}
	return o
}

// preResolveIssue populates Context with everything PICK_TOP_READY would
// have set, reading from `gh project view` + `gh project item-list` (via
// board.FindIssue). Called once at driver startup in implement-ticket mode.
// cfg is the pre-loaded project config (may be nil if no gh-optivem.yaml and
// no --config); supplied by Run so the load happens once per driver
// invocation.
func preResolveIssue(ctx context.Context, opts Options, sCtx *statemachine.Context, cfg *projectconfig.Config) error {
	projectURL := opts.ProjectURL
	if projectURL == "" {
		resolved, err := board.ResolveProjectURLFromConfig(cfg)
		if err != nil {
			return fmt.Errorf("resolve project URL: %w", err)
		}
		projectURL = resolved
	}
	sCtx.Set("project_url", projectURL)

	pick, err := board.FindIssue(ctx, opts.IssueNum, board.Options{
		ProjectURL: projectURL,
		RepoPath:   opts.RepoPath,
	})
	if err != nil {
		return err
	}
	projectName := pick.ProjectTitle

	sCtx.Set("issue_num", strconv.Itoa(pick.IssueNum))
	sCtx.Set("issue_url", pick.IssueURL)
	sCtx.Set("issue_title", pick.Title)
	sCtx.Set("issue_repo", pick.Repo)
	sCtx.Set("project_id", pick.ProjectID)
	sCtx.Set("project_title", projectName)
	sCtx.Set("item_id", pick.ItemID)

	projectLabel := projectURL
	if projectName != "" {
		projectLabel = fmt.Sprintf("%s (%s)", projectName, projectURL)
	}
	fmt.Fprintf(opts.Stdout, "Resolved issue #%d %q (%s) on project %s.\n",
		pick.IssueNum, pick.Title, pick.Repo, projectLabel)
	return nil
}

// registerAgentDispatchers registers a no-op base dispatcher for every
// agent that has an embedded prompt (filesystem walk via agents.Names).
// The substantive prompt-and-pause behaviour is layered on after Bind by
// wrapAgentDispatchers, which has access to per-node RawNode metadata
// (description, phase_doc). Adding a new agent is now: drop a prompt
// under internal/atdd/runtime/agents/prompts/, recompile.
func registerAgentDispatchers(r *agents.Registry) {
	noop := func(ctx *statemachine.Context) statemachine.Outcome {
		return statemachine.Outcome{}
	}
	for _, name := range agents.Names() {
		r.Register(name, noop)
	}
}

// wrapAgentDispatchers replaces every non-human user_task NodeFn with
// either a clauderun-based dispatcher (the v2 default — auto-launches
// the named Claude Code subagent via the `claude` CLI) or, when
// opts.ManualAgents is true, the v1 pause-and-prompt fallback. The
// dispatcher closure has access to the YAML node's Raw metadata
// (description, phase_doc, agent name) and the per-run Options, which
// together provide everything the prompt template needs.
func wrapAgentDispatchers(eng *statemachine.Engine, opts Options) {
	for _, flow := range eng.Flows {
		for id, node := range flow.Nodes {
			if node.Kind != statemachine.UserTask {
				continue
			}
			if node.Raw.Agent == "human" || node.Raw.Agent == "" {
				continue
			}
			raw := node.Raw
			nodeID := id
			inner := node.Fn
			if opts.ManualAgents {
				node.Fn = newManualAgentDispatcher(opts, raw, inner)
			} else {
				node.Fn = newClaudeRunDispatcher(opts, raw, nodeID, inner)
			}
			flow.Nodes[id] = node
		}
	}
}

// newManualAgentDispatcher returns the v1 pause-and-prompt wrapper. The
// operator launches the named agent in a second window (typically the
// Task tool in a separate Claude Code session), commits, then presses
// Enter at the driver's prompt to advance.
func newManualAgentDispatcher(opts Options, raw statemachine.RawNode, inner statemachine.NodeFn) statemachine.NodeFn {
	return func(ctx *statemachine.Context) statemachine.Outcome {
		if err := promptForAgent(opts, raw, ctx.Params); err != nil {
			return statemachine.Outcome{Err: err}
		}
		return inner(ctx)
	}
}

// newClaudeRunDispatcher returns the v2 dispatcher. It reads the override
// hints written to the Context state by override.Wrap, pulls the ticket
// fields populated by preResolveIssue / PICK_TOP_READY, hands the lot to
// clauderun.Dispatch, and surfaces the resulting commit SHA via
// Outcome.Commit (which the verify post-condition decorator keys off).
func newClaudeRunDispatcher(opts Options, raw statemachine.RawNode, nodeID string, inner statemachine.NodeFn) statemachine.NodeFn {
	return func(ctx *statemachine.Context) statemachine.Outcome {
		extraText := ctx.GetString(override.KeyExtra)
		replaceText := ctx.GetString(override.KeyReplace)
		interactive, _ := ctx.Get(override.KeyInteractive).(bool)

		issueNum, _ := strconv.Atoi(ctx.GetString("issue_num"))

		agentName := statemachine.ExpandParams(raw.Agent, ctx.Params)
		cOpts := clauderun.Options{
			Agent:           agentName,
			PhaseDoc:        statemachine.ExpandParams(raw.PhaseDoc, ctx.Params),
			NodeDescription: statemachine.ExpandParams(raw.Description, ctx.Params),
			IssueNum:        issueNum,
			IssueTitle:      ctx.GetString("issue_title"),
			IssueRepo:       ctx.GetString("issue_repo"),
			ProjectTitle:    ctx.GetString("project_title"),
			ProjectURL:      ctx.GetString("project_url"),
			Architecture:    ctx.GetString("architecture"),
			SystemLang:      ctx.GetString("system_lang"),
			TestLang:        ctx.GetString("test_lang"),
			OverrideText:    extraText,
			RawPrompt:       replaceText,
			PromptOverride:  opts.AgentPromptOverrides[agentName],
			Autonomous:      opts.Autonomous,
			CLICommits:      opts.CLICommits,
			RepoPath:        opts.RepoPath,
			Stdout:          opts.Stdout,
			Stderr:          opts.Stderr,
			Stdin:           opts.Stdin,
		}

		if interactive {
			additional, err := promptForInteractiveExtra(opts, cOpts, nodeID)
			if err != nil {
				return statemachine.Outcome{Err: err}
			}
			if additional != "" {
				if cOpts.OverrideText == "" {
					cOpts.OverrideText = additional
				} else {
					cOpts.OverrideText = cOpts.OverrideText + "\n" + additional
				}
			}
		}

		info, err := clauderun.Dispatch(context.Background(), opts.ClaudeRunDeps, cOpts)
		if err != nil {
			return statemachine.Outcome{Err: err}
		}
		out := inner(ctx)
		if out.Err != nil {
			return out
		}
		out.Commit = info.SHA
		return out
	}
}

// promptForAgent prints the per-node dispatch banner and blocks on stdin
// until the operator types Enter (continue) or `abort` (halt). v1 / fallback path.
//
// params is the live Context.Params for the call_activity scope; templated
// fields in raw (e.g. ${agent} / ${phase_doc} / ${change_type} inside the shared
// structural_cycle) are expanded against it so the operator sees the
// substituted name in the banner instead of the literal placeholder.
func promptForAgent(opts Options, raw statemachine.RawNode, params map[string]string) error {
	agent := statemachine.ExpandParams(raw.Agent, params)
	phaseDoc := statemachine.ExpandParams(raw.PhaseDoc, params)
	description := statemachine.ExpandParams(raw.Description, params)

	fmt.Fprintln(opts.Stdout)
	fmt.Fprintf(opts.Stdout, "DISPATCH: %s\n", agent)
	if description != "" {
		fmt.Fprintf(opts.Stdout, "  Phase: %s\n", description)
	}
	if phaseDoc != "" {
		fmt.Fprintf(opts.Stdout, "  Phase doc: %s\n", phaseDoc)
	}
	fmt.Fprintf(opts.Stdout, "  Launch the %s agent now (e.g. via the Task tool in Claude Code).\n", agent)
	fmt.Fprintln(opts.Stdout, "  When the agent's COMMIT lands on HEAD, press Enter to continue. Type 'abort' to halt.")

	r := bufio.NewReader(opts.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read agent-dispatch confirmation: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(line), "abort") {
		return fmt.Errorf("operator aborted at %s dispatch", agent)
	}
	return nil
}

// promptForInteractiveExtra implements the --interactive override hook:
// render the prompt clauderun.Dispatch would build, print it for review,
// and read one trailing line from stdin to append. An empty line is the
// "no addition" signal.
func promptForInteractiveExtra(opts Options, cOpts clauderun.Options, nodeID string) (string, error) {
	rendered, err := clauderun.RenderPrompt(cOpts)
	if err != nil {
		return "", fmt.Errorf("render prompt for review: %w", err)
	}
	fmt.Fprintln(opts.Stdout)
	fmt.Fprintf(opts.Stdout, "── DISPATCH PREVIEW: %s (%s) ──\n", cOpts.Agent, nodeID)
	fmt.Fprintln(opts.Stdout, rendered)
	fmt.Fprintln(opts.Stdout, "──")
	fmt.Fprintln(opts.Stdout, "Additional text to append (single line; press Enter to skip):")
	r := bufio.NewReader(opts.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read interactive extra: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// wrapOverride applies the override.Wrap decorator to every node. Wrapping
// happens for every node regardless of kind — the v1 hook is a pass-through
// so there is no measurable cost, and v2 (which adds CLI surface for
// --extra / --replace / --interactive) only has to fill in the body.
func wrapOverride(eng *statemachine.Engine, hooks *override.Hooks) {
	if hooks == nil {
		hooks = &override.Hooks{}
	}
	for _, flow := range eng.Flows {
		for id, node := range flow.Nodes {
			node.Fn = override.Wrap(node.Fn, id, hooks)
			flow.Nodes[id] = node
		}
	}
}
