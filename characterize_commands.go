// characterize_commands.go wires the top-level `gh optivem characterize`
// command: the one-off legacy baseline (plan 20261110-1130). It walks the
// `characterize` process of the ATDD flow with no ticket, writing tests that
// pin the system's current behaviour, use case by use case, without changing
// the system code.
package main

import (
	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/driver"
)

// newCharacterizeCmd implements `gh optivem characterize`. It shares the
// driver flags of `implement` and enters driver.CharacterizeProcessName,
// whose preset State sets `characterize: true` and the ticket-less commit
// message fields.
func newCharacterizeCmd() *cobra.Command {
	var f driverRunFlags
	cmd := &cobra.Command{
		Use:   "characterize",
		Short: "Write characterization tests for an existing (legacy) system, use case by use case",
		Long: `Write a baseline of acceptance tests that pin how an existing system behaves
today, without changing the system code. No ticket is needed.

The run starts the system once, then:

  1. An agent writes the use case diagram and the use case dependency
     diagram (docs/use-cases by default, config key use-cases-path) and
     proposes the most important use case. You approve or give feedback.
  2. Pilot: the proposed use case and the use cases it depends on, one at a
     time. Per use case: narrative, acceptance criteria, then the acceptance
     tests (external systems, acceptance tests, DSL, drivers) until they pass
     against the running system. You approve after each of the three steps;
     a "no" asks what should change and redoes the step with your feedback.
  3. Scaling: every other use case, the same steps, unattended.

Each use case then gets its WIP gate removed and its acceptance criteria
deleted (the tagged tests are the specification; 'gh optivem spec generate'
renders them readable). The diagrams and narratives stay as the requirements
documentation. Every step commits, with messages like
"#characterize Characterize legacy system - ACCEPTANCE TESTS (UC-001)".

Safety gate: after every commit the run checks that no system code changed
since the run started and that no test assertion was removed; either one
halts the run. Stubs and simulators of external systems may change; the
run restarts only those (system start --restart --external-only).

Resume: run the same command again on the same branch. Completed use cases
are read back from the committed tests, and completed layers from their
commit trailers.

Monolith and multitier projects only.`,
		Example: `  gh optivem characterize
  gh optivem characterize --log-file characterize.log
  gh optivem characterize --workspace /abs/path/to/workspace
  gh optivem -c ./myrepo-multitier.yaml characterize
  gh optivem characterize --headless          # run each agent as claude -p`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitIfComponentKind("gh optivem characterize", "it writes acceptance tests against a running system, and a component project has no system")
			exitOnError(f.validate())
			runDriverCommand(cmd, &f, driver.Options{ProcessName: driver.CharacterizeProcessName}, nil)
		},
	}
	f.register(cmd)
	return cmd
}
