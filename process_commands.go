// process_commands.go wires the `gh optivem process` parent and its
// children. The intent is the "view, don't generate" principle: consumers
// and CI alike can read the same artifact gh-optivem produces, with no
// per-repo tooling.
//
//	gh optivem process show           # render the Mermaid process-flow diagram
//	gh optivem process show > docs/process-diagram.md
//
// `process` is a noun group, not a methodology namespace. The artifact
// described (the configured process flow) belongs to *the process*, not to
// *the methodology* — keeping it grouped under `process` ages better when
// TDD/DDD or compositions land, and matches the existing noun-first surface
// (`system`, `test`, `config`, `environment`).
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/diagram"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// newProcessCmd builds the `gh optivem process` parent. The parent has no
// Run, so invoking it without a subcommand prints help.
func newProcessCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "process",
		Short: "Inspect the configured implementation process",
	}
	cmd.AddCommand(newProcessShowCmd())
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
