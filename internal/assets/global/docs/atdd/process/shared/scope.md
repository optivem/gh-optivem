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

Two layers, in order:

**Layer 1 — agent-signalled.** If the agent recognises mid-phase that it
cannot finish without editing files outside `scope:`, it does **not**
proceed silently and does **not** ask the user inline. It emits a
structured `scope_exception` block in its final output and exits — the
runtime presents the exception to the user, who decides whether to
accept the out-of-scope change, rewind to an upstream phase, revert and
rerun, or abort.

The `scope_exception` block shape (consumed by the runtime's
`scope_exception_requested` gate):

```
scope_exception:
  files:
    - path/to/out-of-scope.go
  reason: <one-line rationale>
```

When the block is absent, the phase ran within scope. No new IPC
mechanism — the block is one structured field of the existing
agent→runtime COMMIT-output channel.

**Layer 2 — post-phase scripted check.** Catches what Layer 1 missed.
After the agent commits, the `check_phase_scope` action in the BPMN
runtime diffs the working tree against the phase's allowed paths and
fails the run if anything fell outside; the cycle routes to the same
human-task page as Layer 1.

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
