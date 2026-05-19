# Scope rule

Every ATDD phase has a **scope**: the set of paths the agent for that
phase is allowed to modify.

The phase's runtime-prompt file (under `internal/assets/runtime/prompts/atdd/`)
carries a `scope:` frontmatter listing the allowed paths, fully resolved.
That frontmatter is documentation — it lets a human (or IDE) see the
scope without cross-referencing the doctrinal source.

**The rule:** in a given phase, only modify files under paths listed in
the phase's `scope:`. If the task appears to require touching paths
outside scope, **stop and alert the user** rather than expanding scope
silently — scope creep is usually a sign that the phase boundary is
wrong, the test scope is wrong, or a refactor is needed first.

## How the rule is enforced

The agent does not self-police via the frontmatter. Scope is enforced at
WRITE-time by the `check_phase_scope` action in the BPMN runtime: after
the agent commits, the action diffs the working tree against the
phase's allowed paths and fails the run if anything fell outside.

## Where the per-phase scope comes from

Two doctrinal inputs are joined to produce each phase's `scope:`:

- **`internal/atdd/phase-scopes.yaml`** (inside gh-optivem) maps phase
  ids to layer names (e.g. `AT_RED_TEST: [at_test, dsl_port, dsl_core]`).
  Layer names are canonical ATDD vocabulary — the same in every
  gh-optivem project.
- **`gh-optivem.yaml paths:`** (inside the user's project) maps each
  layer name to a fully-resolved physical path
  (e.g. `at_test: system-test/typescript/tests/latest/acceptance`).

The runtime prompt's `scope:` frontmatter is the join of these two,
projected at scaffold and `gh optivem sync` time.
