// architecture_commands.go wires the `gh optivem architecture` parent
// and its children. Mirrors `process_commands.go` — same "view, don't
// generate" principle: consumers and CI alike read the same artifact
// gh-optivem produces, with no per-repo tooling.
//
//	gh optivem architecture show           # render the Mermaid architecture diagram
//	gh optivem architecture show > docs/architecture-diagram.md
//
// The rendered output describes the layered ATDD architecture every
// scaffolded student repo follows. Keeping the canonical copy in
// gh-optivem's `docs/` (alongside `docs/process-diagram.md`) avoids
// drift between scaffolded repos and the source-of-truth template.
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/architecture"
)

// newArchitectureCmd builds the `gh optivem architecture` parent. The
// parent has no Run, so invoking it without a subcommand prints help.
func newArchitectureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "architecture",
		Short: "Inspect the ATDD layered-architecture diagram",
	}
	cmd.AddCommand(newArchitectureShowCmd())
	return cmd
}

// newArchitectureShowCmd renders the canonical architecture Mermaid
// markdown to stdout. CI's regenerate-architecture-diagram workflow
// pipes this output to docs/architecture-diagram.md and commits any
// diff.
func newArchitectureShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Render the architecture Mermaid diagram to stdout",
		Example: `  gh optivem architecture show
  gh optivem architecture show > docs/architecture-diagram.md`,
		Run: func(cmd *cobra.Command, args []string) {
			doc, err := architecture.LoadDefault()
			exitOnError(err)
			fmt.Print(architecture.Render(doc))
		},
	}
}
