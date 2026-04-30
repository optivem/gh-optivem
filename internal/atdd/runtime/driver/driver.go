// Package driver wires together the ATDD pipeline runtime: it loads the
// process-flow YAML, registers gates / actions / agents, applies override and
// verify decorators, and walks the named flow end to end.
//
// The driver is deliberately thin — the heavy lifting lives in the runtime
// sub-packages (statemachine, gates, actions, verify, override, board,
// classify, release). This file's job is to compose them and expose two
// run modes:
//
//   - Run with Options.IssueNum > 0 → implement-ticket mode: pre-resolve the
//     project item for the given issue, seed Context, and skip the picker by
//     starting the main flow at MOVE_TO_IN_PROGRESS.
//   - Run with Options.IssueNum == 0 → manage-project mode: the YAML's main
//     flow runs from START, picking the top Ready ticket from the project
//     board.
//
// Agent dispatch in v1: every user_task whose `agent:` value is something
// other than `human` registers a "pause-and-prompt" dispatcher. The driver
// prints the YAML node's description / phase_doc and blocks on stdin while
// the operator launches the corresponding Claude Code agent (e.g. via the
// Task tool). When the agent's COMMIT lands on HEAD the operator presses
// Enter, the post-condition verify check kicks in (commit-message HEAD
// match), and the engine moves on. Auto-dispatch via the Agent SDK is a v2
// concern and out of scope for this session.
package driver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/actions"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/board"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/gates"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/override"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/verify"
)

// DefaultYAMLPath is the canonical relative location of the process-flow
// document inside a consumer repo. Read from the consumer's CWD — gh-optivem
// is repo-agnostic by design.
const DefaultYAMLPath = "docs/atdd/process/process-flow.yaml"

// DefaultFlowName is the entry flow loaded by every public CLI command.
const DefaultFlowName = "main"

// Options bundles every driver knob that callers (the `gh optivem atdd …`
// commands and tests) might want to set. Zero values yield a usable
// configuration: load DefaultYAMLPath, enter DefaultFlowName, no overrides,
// real shell-outs.
type Options struct {
	// YAMLPath is the process-flow file to load. Empty → DefaultYAMLPath.
	YAMLPath string

	// FlowName is the entry flow. Empty → DefaultFlowName.
	FlowName string

	// IssueNum, when > 0, switches the driver into implement-ticket mode:
	// the picker (PICK_TOP_READY) is bypassed and the driver pre-resolves
	// the project item for the given issue.
	IssueNum int

	// ProjectURL overrides README/git-remote project resolution. Optional;
	// when empty, board.ResolveProjectURL handles the lookup against
	// RepoPath / cwd.
	ProjectURL string

	// RepoPath overrides the working directory used for project resolution
	// and for shell-outs. Optional; defaults to cwd.
	RepoPath string

	// Autonomous skips human-approval STOPs. v1 still pauses at agent
	// dispatch points (the Go driver does not auto-launch agents); the
	// flag is propagated to gates / actions for their own decisions.
	Autonomous bool

	// Override holds the per-node override hooks (Extra / Replace /
	// Interactive). v2 wires CLI flags into this struct; v1 leaves it nil.
	Override *override.Hooks

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

	eng, err := statemachine.LoadFile(opts.YAMLPath)
	if err != nil {
		return fmt.Errorf("driver: load YAML %q: %w", opts.YAMLPath, err)
	}

	flow, ok := eng.Flows[opts.FlowName]
	if !ok {
		return fmt.Errorf("driver: flow %q not in YAML %q", opts.FlowName, opts.YAMLPath)
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

	sCtx := statemachine.NewContext()
	if opts.ProjectURL != "" {
		sCtx.Set("project_url", opts.ProjectURL)
	}

	if opts.IssueNum > 0 {
		if err := preResolveIssue(ctx, opts, sCtx); err != nil {
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

func (o Options) withDefaults() Options {
	if o.YAMLPath == "" {
		o.YAMLPath = DefaultYAMLPath
	}
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
func preResolveIssue(ctx context.Context, opts Options, sCtx *statemachine.Context) error {
	projectURL := opts.ProjectURL
	if projectURL == "" {
		repoPath := opts.RepoPath
		if repoPath == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			repoPath = cwd
		}
		resolved, err := board.ResolveProjectURL(repoPath, nil)
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
	sCtx.Set("issue_num", strconv.Itoa(pick.IssueNum))
	sCtx.Set("issue_url", pick.IssueURL)
	sCtx.Set("issue_title", pick.Title)
	sCtx.Set("issue_repo", pick.Repo)
	sCtx.Set("project_id", pick.ProjectID)
	sCtx.Set("item_id", pick.ItemID)

	fmt.Fprintf(opts.Stdout, "Resolved issue #%d %q (%s) on project %s.\n",
		pick.IssueNum, pick.Title, pick.Repo, projectURL)
	return nil
}

// agentNames lists every Claude Code agent referenced by a user_task in
// docs/atdd/process/process-flow.yaml. Adding a new agent to the YAML
// requires adding its name here so the dispatch registry resolves; the
// engine refuses to start with an unknown binding.
var agentNames = []string{
	"atdd-story",
	"atdd-bug",
	"atdd-task",
	"atdd-chore",
	"atdd-test",
	"atdd-dsl",
	"atdd-driver",
	"atdd-backend",
	"atdd-frontend",
	"atdd-stubs",
	"atdd-release",
}

// registerAgentDispatchers registers a no-op base dispatcher for every
// known agent name. The substantive prompt-and-pause behaviour is layered
// on after Bind by wrapAgentDispatchers, which has access to per-node
// RawNode metadata (description, phase_doc).
func registerAgentDispatchers(r *agents.Registry) {
	noop := func(ctx *statemachine.Context) statemachine.Outcome {
		return statemachine.Outcome{}
	}
	for _, name := range agentNames {
		r.Register(name, noop)
	}
}

// wrapAgentDispatchers replaces every non-human user_task NodeFn with a
// pause-and-prompt wrapper that tells the operator to launch the named
// agent. v1 cannot auto-launch agents from a Go binary — the operator
// drives the Claude Code session — so we serialise the operator's role at
// each dispatch.
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
			inner := node.Fn
			node.Fn = func(ctx *statemachine.Context) statemachine.Outcome {
				if err := promptForAgent(opts, raw); err != nil {
					return statemachine.Outcome{Err: err}
				}
				return inner(ctx)
			}
			flow.Nodes[id] = node
		}
	}
}

// promptForAgent prints the per-node dispatch banner and blocks on stdin
// until the operator types Enter (continue) or `abort` (halt).
func promptForAgent(opts Options, raw statemachine.RawNode) error {
	fmt.Fprintln(opts.Stdout)
	fmt.Fprintf(opts.Stdout, "DISPATCH: %s\n", raw.Agent)
	if raw.Description != "" {
		fmt.Fprintf(opts.Stdout, "  Phase: %s\n", raw.Description)
	}
	if raw.PhaseDoc != "" {
		fmt.Fprintf(opts.Stdout, "  Phase doc: %s\n", raw.PhaseDoc)
	}
	fmt.Fprintf(opts.Stdout, "  Launch the %s agent now (e.g. via the Task tool in Claude Code).\n", raw.Agent)
	fmt.Fprintln(opts.Stdout, "  When the agent's COMMIT lands on HEAD, press Enter to continue. Type 'abort' to halt.")

	r := bufio.NewReader(opts.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read agent-dispatch confirmation: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(line), "abort") {
		return fmt.Errorf("operator aborted at %s dispatch", raw.Agent)
	}
	return nil
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
