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
	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
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

	phaseAgents := agentByPhase(eng)

	if phaseArg != "" {
		return printOnePhase(out, phaseArg, scopes, phaseAgents, cfg)
	}
	return printAllPhases(out, scopes, phaseAgents, cfg)
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

// agentByPhase returns a phase-id → task-name map derived from
// process-flow.yaml. UserTask nodes carry the task name on the `agent:`
// field directly; templated call_activity nodes carry the concrete
// task name in Params["agent"]. Skips non-writing agents (human) and
// templated `${agent}` references whose concrete value lives on the
// parent.
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

// printAllPhases renders every phase in phase-scopes.yaml (sorted),
// followed by every writing-agent phase whose prompt frontmatter
// declares `scope: none` (the artifact-only doctrinal exemption — see
// runtime/shared/scope.md). The second group has no layers to resolve
// but still belongs in the operator's overview.
func printAllPhases(out io.Writer, scopes atdd.PhaseScopes, phaseAgents map[string]string, cfg *projectconfig.Config) error {
	phaseIDs := make([]string, 0, len(scopes.Phases))
	for id := range scopes.Phases {
		phaseIDs = append(phaseIDs, id)
	}
	sort.Strings(phaseIDs)

	for _, id := range phaseIDs {
		writePhaseBlock(out, id, scopes.Phases[id], phaseAgents[id], cfg)
		fmt.Fprintln(out)
	}

	noneIDs, err := noneScopedPhaseIDs(scopes, phaseAgents)
	if err != nil {
		return err
	}
	for _, id := range noneIDs {
		writeNoneScopeBlock(out, id, phaseAgents[id])
		fmt.Fprintln(out)
	}
	return nil
}

// printOnePhase renders a single phase. Routes through the same block
// renderer as printAllPhases. A phase id not in phase-scopes.yaml is
// only accepted if it corresponds to a writing-agent node in
// process-flow.yaml whose prompt frontmatter declares `scope: none`;
// otherwise a typo'd or unscoped phase id fails loudly.
func printOnePhase(out io.Writer, phaseID string, scopes atdd.PhaseScopes, phaseAgents map[string]string, cfg *projectconfig.Config) error {
	if layers, ok := scopes.Phases[phaseID]; ok {
		writePhaseBlock(out, phaseID, layers, phaseAgents[phaseID], cfg)
		return nil
	}
	if agent, ok := phaseAgents[phaseID]; ok {
		none, err := agents.HasNoneScope(agent)
		if err != nil {
			return fmt.Errorf("process scope: %w", err)
		}
		if none {
			writeNoneScopeBlock(out, phaseID, agent)
			return nil
		}
	}
	return fmt.Errorf("phase %q not in phase-scopes.yaml; run `gh optivem process scope` to list known phases", phaseID)
}

// noneScopedPhaseIDs returns the sorted set of writing-agent phase ids
// not in phase-scopes.yaml whose prompt frontmatter declares
// `scope: none`. Surfaced from printAllPhases so the doctrinal
// exemption is visible in the operator's overview, not hidden.
func noneScopedPhaseIDs(scopes atdd.PhaseScopes, phaseAgents map[string]string) ([]string, error) {
	var out []string
	for id, agent := range phaseAgents {
		if _, inScopes := scopes.Phases[id]; inScopes {
			continue
		}
		none, err := agents.HasNoneScope(agent)
		if err != nil {
			return nil, fmt.Errorf("process scope: %w", err)
		}
		if none {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

// writeNoneScopeBlock renders the `scope: none` sentinel for an
// artifact-only / external-system-only writing-agent phase. No layers,
// no resolved paths — the contract is "no working-tree writes" full
// stop, so the block intentionally lacks the Layers: section.
func writeNoneScopeBlock(out io.Writer, phaseID, agent string) {
	fmt.Fprintf(out, "Phase:  %s\n", phaseID)
	fmt.Fprintf(out, "Agent:  %s\n", agent)
	fmt.Fprintln(out, "Scope:  none (artifact-only — no working-tree writes)")
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
// in the project. Family A `system-path` reads from `system.path`
// (monolith). Everything else is a Family B key under `system-test.paths:`.
//
// Multitier projects leave `system.path` empty — `system-path` returns
// "" there, which the renderer surfaces as "(not set in gh-optivem.yaml)".
// AT_GREEN is monolith-only by construction; per-component fanout for
// multitier projects is deferred to a future plan.
func resolveLayer(cfg *projectconfig.Config, layer string) string {
	if layer == "system-path" {
		return cfg.System.Path
	}
	return cfg.SystemTest.Paths[layer]
}
