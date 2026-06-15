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

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/diagrams/diagram"
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

// newProcessScopeCmd prints per-phase allowed-paths from process-flow.yaml's
// inline node scope joined against the project's `gh-optivem.yaml paths:`.
// No arg lists every writing-agent MID; one positional arg narrows to that
// phase. When run outside a gh-optivem.yaml-rooted project, layer names
// print bare (still useful for navigating the process flow itself).
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
		Short: "Print per-phase allowed-paths from process-flow.yaml × gh-optivem.yaml",
		Example: `  gh optivem process scope
  gh optivem process scope write-acceptance-tests`,
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
	eng, err := statemachine.LoadDefault()
	if err != nil {
		return fmt.Errorf("process scope: load process-flow: %w", err)
	}

	// Project config is optional — without it, layer names print bare;
	// with it, layers resolve to physical paths.
	cfg := loadConfigIfPresent(configPath)

	if phaseArg != "" {
		return printOnePhase(out, phaseArg, eng, cfg)
	}
	return printAllPhases(out, eng, cfg)
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

// writingAgentMIDs returns the sorted set of writing-agent MID process
// names — every process containing an EXECUTE_AGENT call-activity that
// dispatches the LOW execute-agent primitive with a concrete (non-
// templated) task-name. The process name is the `task-name:` value on
// that node and the same identifier Engine.Scope keys on. The `fix`
// LOW is excluded because its task-name is templated
// (`fix-${failure-kind}`) — fix dispatches resolve a concrete MID at
// runtime, not here.
func writingAgentMIDs(eng *statemachine.Engine) []string {
	var ids []string
	for name, proc := range eng.Processes {
		for _, node := range proc.Nodes {
			if node.Kind != statemachine.CallActivity || node.Raw.Process != "execute-agent" {
				continue
			}
			task := node.Raw.Params["task-name"]
			if task == "" || strings.HasPrefix(task, "${") || strings.Contains(task, "${") {
				continue
			}
			ids = append(ids, name)
			break
		}
	}
	sort.Strings(ids)
	return ids
}

// taskNameOf returns the agent task-name dispatched by a writing-agent
// MID — read from the EXECUTE_AGENT call-activity's `params.task-name`.
// Defaults to the process name when the param is missing (matches the
// MID-name == task-name convention).
func taskNameOf(eng *statemachine.Engine, processName string) string {
	proc, ok := eng.Processes[processName]
	if !ok {
		return ""
	}
	for _, node := range proc.Nodes {
		if node.Kind != statemachine.CallActivity || node.Raw.Process != "execute-agent" {
			continue
		}
		if t := node.Raw.Params["task-name"]; t != "" {
			return t
		}
		return processName
	}
	return ""
}

// printAllPhases renders every writing-agent MID (sorted) — those with
// inline read/write scope, then those whose EXECUTE_AGENT node declares
// `scope: none` (the artifact-only doctrinal exemption — see
// runtime/shared/scope.md). The second group has no layers to resolve
// but still belongs in the operator's overview.
func printAllPhases(out io.Writer, eng *statemachine.Engine, cfg *projectconfig.Config) error {
	var noneIDs []string
	for _, id := range writingAgentMIDs(eng) {
		_, write, ok := eng.Scope(id)
		if ok {
			writePhaseBlock(out, id, write, taskNameOf(eng, id), cfg)
			fmt.Fprintln(out)
			continue
		}
		if eng.IsScopeNone(id) {
			noneIDs = append(noneIDs, id)
		}
	}
	for _, id := range noneIDs {
		writeNoneScopeBlock(out, id, taskNameOf(eng, id))
		fmt.Fprintln(out)
	}
	return nil
}

// printOnePhase renders a single phase. Routes through the same block
// renderer as printAllPhases. A phase id without inline read/write is
// only accepted if its EXECUTE_AGENT node declares `scope: none`;
// otherwise a typo'd or unscoped phase id fails loudly.
func printOnePhase(out io.Writer, phaseID string, eng *statemachine.Engine, cfg *projectconfig.Config) error {
	_, write, ok := eng.Scope(phaseID)
	if ok {
		writePhaseBlock(out, phaseID, write, taskNameOf(eng, phaseID), cfg)
		return nil
	}
	if eng.IsScopeNone(phaseID) {
		writeNoneScopeBlock(out, phaseID, taskNameOf(eng, phaseID))
		return nil
	}
	return fmt.Errorf("phase %q is not a scoped writing-agent MID in process-flow.yaml; run `gh optivem process scope` to list known phases", phaseID)
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

// resolveLayer maps a layer name to its physical path in the project.
// Family A `system-path` reads from `system.path` (monolith).
// Everything else is a Family B key under `system-test.paths:`.
//
// Multitier projects leave `system.path` empty — `system-path` returns
// "" there, which the renderer surfaces as "(not set in gh-optivem.yaml)".
// implement-system / refactor-system are monolith-only by construction;
// per-component fanout for multitier projects is deferred to a future
// plan.
func resolveLayer(cfg *projectconfig.Config, layer string) string {
	if layer == "system-path" {
		return cfg.System.Path
	}
	return cfg.SystemTest.Paths[layer]
}
