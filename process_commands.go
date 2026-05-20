// process_commands.go wires the `gh optivem process` parent and its
// children. The intent is the "view, don't generate" principle: consumers
// and CI alike can read the same artifact gh-optivem produces, with no
// per-repo tooling.
//
//	gh optivem process show           # render the Mermaid process-flow diagram
//	gh optivem process show > docs/process-diagram.md
//	gh optivem process scope          # list every phase's allowed-paths layers
//	gh optivem process scope AT_RED_TEST   # one phase, resolved against gh-optivem.yaml
//
// `process` is a noun group, not a methodology namespace. The artifact
// described (the configured process flow) belongs to *the process*, not to
// *the methodology* — keeping it grouped under `process` ages better when
// TDD/DDD or compositions land, and matches the existing noun-first surface
// (`system`, `test`, `config`, `environment`).
package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/atdd"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/diagram"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// newProcessCmd builds the `gh optivem process` parent. The parent has no
// Run, so invoking it without a subcommand prints help.
func newProcessCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "process",
		Short: "Inspect the configured implementation process",
	}
	cmd.AddCommand(newProcessShowCmd())
	cmd.AddCommand(newProcessScopeCmd())
	return cmd
}

// newProcessShowCmd renders the canonical process-flow Mermaid markdown to
// stdout. CI's regenerate-diagram workflow pipes this output to
// docs/process-diagram.md and commits any diff. Today the diagram is the
// only thing `show` emits; if a second artifact is ever needed, it goes
// behind a flag or a noun argument rather than a fourth command level.
func newProcessShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Render the process-flow Mermaid diagram to stdout",
		Example: `  gh optivem process show
  gh optivem process show > docs/process-diagram.md`,
		Run: func(cmd *cobra.Command, args []string) {
			eng, err := statemachine.LoadDefault()
			exitOnError(err)
			fmt.Print(diagram.Render(eng))
		},
	}
}

// newProcessScopeCmd prints per-phase allowed-paths from the SSoT join of
// `internal/atdd/phase-scopes.yaml` (embedded) and the project's
// `gh-optivem.yaml paths:`, with the agent dispatched per phase surfaced
// from `process-flow.yaml`. No arg lists every phase; one positional arg
// narrows to that phase. When run outside a gh-optivem.yaml-rooted
// project, layer names print bare (still useful for navigating
// phase-scopes.yaml itself).
//
// Replaces the originally-planned scaffolder/sync projection of `scope:`
// into runtime-prompt frontmatter (item 4 of plans/20260518-1530-atdd-
// phase-scope-ssot.md, refined 2026-05-19): adding a parallel
// materialization layer was non-trivial machinery for a documentation-
// only payoff — runtime enforcement always lived in `check_phase_scope`
// (item 8). A CLI query satisfies the IDE-inspection use case with no
// disk write, no sidecar, no drift risk.
func newProcessScopeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scope [<phase>]",
		Short: "Print per-phase allowed-paths from phase-scopes.yaml × gh-optivem.yaml",
		Example: `  gh optivem process scope
  gh optivem process scope AT_RED_TEST`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			phaseArg := ""
			if len(args) == 1 {
				phaseArg = args[0]
			}
			exitOnError(runProcessScope(os.Stdout, phaseArg, projectConfigPath))
		},
	}
}

// runProcessScope is the testable body of newProcessScopeCmd. Splitting
// it out lets the test substitute an io.Writer and a config-path
// argument without spawning a subprocess or touching os.Stdout / globals.
func runProcessScope(out io.Writer, phaseArg, configPath string) error {
	scopes, err := atdd.LoadPhaseScopes()
	if err != nil {
		return fmt.Errorf("process scope: %w", err)
	}
	eng, err := statemachine.LoadDefault()
	if err != nil {
		return fmt.Errorf("process scope: load process-flow: %w", err)
	}

	// Project config is optional — without it, layer names print bare;
	// with it, layers resolve to physical paths.
	cfg := loadConfigIfPresent(configPath)

	agents := agentByPhase(eng)

	if phaseArg != "" {
		return printOnePhase(out, phaseArg, scopes, agents, cfg)
	}
	return printAllPhases(out, scopes, agents, cfg)
}

// loadConfigIfPresent resolves the gh-optivem.yaml path and loads it,
// returning nil on any failure. `process scope` is read-only and the
// project-context resolution is best-effort — a stale or missing config
// shouldn't break the bare-layer-name output.
func loadConfigIfPresent(configPath string) *projectconfig.Config {
	path, _ := projectconfig.ResolvePath(configPath)
	cfg, err := projectconfig.LoadFromPath(path)
	if err != nil {
		return nil
	}
	return cfg
}

// agentByPhase returns a phase-id → agent-name map derived from
// process-flow.yaml. UserTask nodes carry the agent directly; templated
// call_activity nodes carry the concrete agent in Params["agent"]. Skips
// non-writing agents (human / fix-verify) and templated `${agent}`
// references whose concrete value lives on the parent.
func agentByPhase(eng *statemachine.Engine) map[string]string {
	out := map[string]string{}
	for _, proc := range eng.Processes {
		for _, node := range proc.Nodes {
			agent := ""
			switch node.Kind {
			case statemachine.UserTask:
				agent = node.Raw.Agent
			case statemachine.CallActivity:
				agent = node.Raw.Params["agent"]
			}
			if agent == "" || atdd.NonWritingAgents[agent] || strings.HasPrefix(agent, "${") {
				continue
			}
			out[node.ID] = agent
		}
	}
	return out
}

// printAllPhases renders every phase in phase-scopes.yaml (sorted) and
// every PhasesDeferredByPlan entry under a separate "Deferred" header.
func printAllPhases(out io.Writer, scopes atdd.PhaseScopes, agents map[string]string, cfg *projectconfig.Config) error {
	phaseIDs := make([]string, 0, len(scopes.Phases))
	for id := range scopes.Phases {
		phaseIDs = append(phaseIDs, id)
	}
	sort.Strings(phaseIDs)

	for _, id := range phaseIDs {
		writePhaseBlock(out, id, scopes.Phases[id], agents[id], cfg)
		fmt.Fprintln(out)
	}

	deferredIDs := make([]string, 0, len(atdd.PhasesDeferredByPlan))
	for id := range atdd.PhasesDeferredByPlan {
		deferredIDs = append(deferredIDs, id)
	}
	sort.Strings(deferredIDs)

	if len(deferredIDs) == 0 {
		return nil
	}
	fmt.Fprintln(out, "Deferred — scope not yet declared (cited plan picks up the work):")
	for _, id := range deferredIDs {
		fmt.Fprintf(out, "  %-44s %s\n", id, atdd.PhasesDeferredByPlan[id])
	}
	return nil
}

// printOnePhase renders a single phase. Routes through the same block
// renderer as printAllPhases. Unknown phases (not in phase-scopes.yaml,
// not on the deferred allowlist) are a hard error so a typo'd phase id
// fails loudly instead of silently emitting nothing.
func printOnePhase(out io.Writer, phaseID string, scopes atdd.PhaseScopes, agents map[string]string, cfg *projectconfig.Config) error {
	if layers, ok := scopes.Phases[phaseID]; ok {
		writePhaseBlock(out, phaseID, layers, agents[phaseID], cfg)
		return nil
	}
	if plan, ok := atdd.PhasesDeferredByPlan[phaseID]; ok {
		fmt.Fprintf(out, "Phase:  %s\n", phaseID)
		fmt.Fprintf(out, "Status: deferred — scope not yet declared\n")
		fmt.Fprintf(out, "Plan:   %s\n", plan)
		return nil
	}
	return fmt.Errorf("phase %q not in phase-scopes.yaml or PhasesDeferredByPlan; run `gh optivem process scope` to list known phases", phaseID)
}

// writePhaseBlock renders one phase's header (phase + agent) and its
// layer list. When cfg is non-nil, each layer prints alongside its
// resolved physical path; when nil, layers print bare.
func writePhaseBlock(out io.Writer, phaseID string, layers []string, agent string, cfg *projectconfig.Config) {
	fmt.Fprintf(out, "Phase:  %s\n", phaseID)
	if agent != "" {
		fmt.Fprintf(out, "Agent:  %s\n", agent)
	}
	fmt.Fprintln(out, "Layers:")
	for _, layer := range layers {
		if cfg == nil {
			fmt.Fprintf(out, "  %s\n", layer)
			continue
		}
		path := resolveLayer(cfg, layer)
		if path == "" {
			fmt.Fprintf(out, "  %-24s (not set in gh-optivem.yaml)\n", layer)
			continue
		}
		fmt.Fprintf(out, "  %-24s %s\n", layer, path)
	}
}

// resolveLayer maps a phase-scopes.yaml layer name to its physical path
// in the project. Family A `system_path` reads from `system.path`
// (monolith). Everything else is a Family B key under `paths:`.
//
// Multitier projects leave `system.path` empty — `system_path` returns
// "" there, which the renderer surfaces as "(not set in gh-optivem.yaml)".
// AT_GREEN is monolith-only by construction; per-component fanout for
// multitier projects is deferred to a future plan.
func resolveLayer(cfg *projectconfig.Config, layer string) string {
	if layer == "system_path" {
		return cfg.System.Path
	}
	return cfg.Paths[layer]
}
