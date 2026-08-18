// kind_guards.go owns the single refusal path for the verbs that need a
// booted system under test. A `kind: component` project declares one unit of
// code and nothing else — no compose stack, no system-test tier, no process
// flow whose outer loop it can walk — so those verbs have nothing to operate
// on.
//
// The guard is one helper, not a copy per command: the message must stay
// identical across every refusal site, and a per-command copy is exactly the
// kind of thing that drifts. Each call site supplies only the verb it is
// refusing and the one-clause reason specific to that verb.
//
// Why refuse rather than degrade: the alternative is running a half-applicable
// flow. `gh optivem system start` against a project with no compose file would
// fail deep inside the runner with "no systems.yaml", which reads as a
// misconfiguration rather than as "this verb does not apply to this project."
// Naming the kind up front is the honest answer.
package main

import (
	"fmt"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// requireSystemKind refuses the verb when the active gh-optivem.yaml declares
// `kind: component`. verb is the user-facing command ("gh optivem system
// start"); why is the reason clause, phrased to complete the sentence "…
// because <why>".
//
// A config that cannot be read is NOT this guard's error to report: the verb's
// own config handling owns that message (a missing gh-optivem.yaml already has
// a canonical recovery error, and an invalid one already reports its
// validation failure). This helper answers exactly one question — "is this
// project a component?" — and returns nil for every answer that is not a
// definitive yes, leaving the real error to surface downstream.
func requireSystemKind(verb, why string) error {
	path, _ := projectconfig.ResolvePath(projectConfigPath)
	cfg, err := projectconfig.LoadFromPath(path)
	if err != nil || cfg == nil || !cfg.IsComponent() {
		return nil
	}
	return fmt.Errorf("%s: %s declares kind: %s, so this command does not apply — %s.\n"+
		"A component project runs its inner loop with `gh optivem compile`, `gh optivem component-test compile`, and `gh optivem component-test run`.",
		verb, path, projectconfig.KindComponent, why)
}

// exitIfComponentKind is requireSystemKind wired to the process-exit path used
// by every Cobra Run body in this package.
func exitIfComponentKind(verb, why string) {
	exitOnError(requireSystemKind(verb, why))
}
