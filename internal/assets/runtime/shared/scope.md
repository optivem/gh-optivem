# Scope rule

Every ATDD phase has a **scope**: the set of paths its agent may modify.
The phase's runtime-prompt file (under `internal/assets/runtime/prompts/atdd/`)
carries a `scope:` frontmatter listing those paths, fully resolved, so a
human (or IDE) can see the scope without consulting the doctrinal source.

**The rule:** only modify files under paths listed in the phase's `scope:`.
If the task appears to require touching paths outside scope, **stop and
alert the user** rather than expanding silently — scope creep usually
signals a wrong phase boundary, wrong test scope, or a missing refactor.

## How the rule is enforced

Two layers, in order:

**Layer 1 — agent-signalled.** If the agent recognises mid-phase that
it cannot finish without editing files outside `scope:`, it does **not**
proceed silently or ask inline. It emits a structured `scope_exception`
block in its final output and exits; the runtime presents the exception
to the user, who decides whether to accept the out-of-scope change,
rewind to an upstream phase, revert and rerun, or abort.

The block shape (consumed by the runtime's `scope_exception_requested`
gate):

```
scope_exception:
  files:
    - path/to/out-of-scope.go
  reason: <one-line rationale>
```

Absence of the block means the phase ran within scope.

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

## `scope: none` — artifact-only agents

Some writing agents mutate **only** inter-phase artifacts (e.g. the
parsed-concepts scratch object passed between BPMN nodes) or external
systems (e.g. the GitHub / Jira tracker) — never a file in the repo
working tree. These agents declare `scope: none` in their prompt
frontmatter and are deliberately absent from `phase-scopes.yaml`; `none`
is a doctrinal category, not a "TBD later" placeholder.

**Contract:** under the canonical GitHub / Jira backends, a `scope: none`
agent modifies NO file in the repo working tree — no config, no docs,
no scripts. A working-tree write outside an escape-hatch adapter is a
contract violation.

Markdown / file adapters are escape hatches (per the
naming-by-primary-backend principle); their repo writes are
out-of-doctrine and do not invalidate `scope: none`.

Distinct from `scope: {}` — the documentation-only frontmatter shape
used by layer-pinned phases, where the real scope lives in
`phase-scopes.yaml`. `scope: none` is the only frontmatter shape that
asserts a doctrinal exemption from `phase-scopes.yaml`; the reverse-FK
drift guard reads the frontmatter to decide whether to require an entry.
