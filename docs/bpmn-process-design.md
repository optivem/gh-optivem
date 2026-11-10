# BPMN process design

Design rationale behind the five-level process model encoded in
[`internal/atdd/process/process-flow.yaml`](../internal/atdd/process/process-flow.yaml)
and rendered into [process-diagram.md](process-diagram.md). The YAML is the single
source of truth for *what* the process does; this document records the *why* behind
its shape, so the decisions survive even though the original design plan does not.

## Five levels

The process model is structured as five nested levels. Each level is built by
*calling* (BPMN call-activity semantics) the level below — never by inheritance.

| Level | Holds | Examples |
|-------|-------|----------|
| **TOP** | Operator entry points. | `refine-ticket`, `implement-ticket`, `refactor` |
| **CYCLE** | Per-ticket sub-processes — one classification maps to exactly one cycle. | `change-system-behavior`, `cover-system-behavior`, `redesign-system-structure`, `redesign-external-system-structure`, `refactor-system-structure`, `refactor-test-structure`, `onboard-external-system`, `refine-backlog` |
| **HIGH** | Orchestrations that compose MID tasks with compile/verify discipline. | `write-and-verify-acceptance-tests`, `implement-and-verify-system`, `refactor-and-verify-tests` |
| **MID** | Concrete tasks; each calls a LOW primitive. | `write-acceptance-tests`, `implement-dsl`, `run-tests`, `compile`, `commit` |
| **LOW** | The four reusable primitives. | `execute-agent`, `execute-command`, `approve`, `fix` |

`implement-ticket` is the spine: mark ticket IN PROGRESS → a classification gateway
that maps ticket type/subtype 1:1 to a CYCLE → call that CYCLE → mark IN ACCEPTANCE.

## LOW primitives

Four primitives, each with its own contract:

- **`execute-agent`** — runs an agent for a task. The prompt file is derived from the
  task name (`<task-name>.md`), not configured. Has PRE and POST `approve` gates
  (agent output needs human review). On a failed output/scope check it calls `fix`
  unless `fix-on-failure: false`.
- **`execute-command`** — runs a CLI command. PRE `approve` only — command success is
  machine-checkable, so no POST review gate (deliberately asymmetric with `execute-agent`).
- **`approve`** — a pure human gate. YES returns to the caller (`END`); NO returns
  `END ERROR`. It is exit-only; the *caller* owns any retry/NO-branch behaviour, which
  keeps `approve` reusable from inside `execute-agent` and `fix`.
- **`fix`** — bounded remediation. A thin wrapper that calls `execute-agent` with
  `fix-on-failure: false`, so it makes a single attempt and cannot recurse. PRE
  `approve` only (it performs destructive edits); it does **not** re-validate its own
  output — the caller re-runs validation after `fix` returns (no duplicated contract).

## Doctrine decisions

- **Full replacement.** The five-level structure fully superseded the previous
  21-diagram BPMN. Every prior concern was either absorbed into a level or dropped with
  a recorded reason — nothing survived by accident.
- **Legacy cycles collapse.** There is no first-class "legacy" path. Writing tests for
  existing behaviour is `cover-system-behavior`, which pins the expected test result to
  *success*; the change path pins it to *failure*. The expected-result is a parameter,
  not a structural fork, and there is no separate "run legacy tests" operation —
  `run-tests` runs whatever is in the suite. (Legacy test *artifacts* are
  indistinguishable from change-cycle artifacts: no folder, annotation, or filename
  suffix.)
- **"Write" tests, "Implement" code.** Canonical TDD vocabulary, applied uniformly
  across HIGH, MID, and CYCLE names.
- **kebab-case everywhere.** Every process-model identifier — YAML keys, doc headings,
  prompt filenames, in-prose references, anchor slugs, Go struct tags — is kebab-case.
  One rule, no per-layer split.
- **No `agent-name:` field.** The runtime derives the prompt path from the task name
  (`prompt_path(task) = task + ".md"`) and errors at startup if the file is missing.
  Convention over configuration; no double-data, no layer-coding in YAML field names.
- **Contracts live in the YAML.** Each task's `scopes:` and `outputs:` live once in
  `process-flow.yaml`. Both consumers read them from there: the agent invocation (prompt
  context + permitted file scope) and the post-execute verify step (required outputs
  present? scope diff clean?). Single source of truth, no drift.
- **`fix` routing by convention.** The failure payload carries a `kind`; the fix prompt
  is derived as `fix-<kind>.md` (e.g. `fix-missing-output`, `fix-scope-diff`,
  `fix-command-failed`, `fix-unexpected-passing-tests`, `fix-unexpected-failing-tests`).
  No routing table, no caller-supplied task name.

## Ticket type → cycle mapping

The `implement-ticket` gateway is purely mechanical — it reads ticket type plus an
optional `task` subtype and looks up one cycle. Unknown subtypes hard-exit (the operator
must re-classify or refine). Multi-cycle work is split into separate tickets during
refinement, never dispatched as one.

| Ticket type / subtype | Cycle |
|---|---|
| `story`, `bug` | `change-system-behavior` |
| `task/cover-legacy` | `cover-system-behavior` |
| `task/redesign-system` | `redesign-system-structure` |
| `task/redesign-external-system` | `redesign-external-system-structure` |
| `task/refactor-system` | `refactor-system-structure` |
| `task/refactor-tests` | `refactor-test-structure` |
| `task/onboard-external-system` | `onboard-external-system` |

## Three refactor surfaces

Refactoring is reachable at three ceremony levels, all calling the same refactor CYCLEs:

1. **Ticket-driven** — `task/refactor-system` (etc.) through the `implement-ticket` gateway.
2. **Opportunistic** — the red-green-**refactor** triad: `change-system-behavior` has a
   loopable step 3 that calls the refactor CYCLEs in *opportunistic mode* (no checklist;
   the cycle degrades to "look at the just-landed patch").
3. **Ad-hoc** — a `refactor` TOP process for refactoring without ticket overhead.

Only `change-system-behavior` gets the opportunistic step — the other cycles have no
GREEN moment that triggers a follow-on refactor.

## Redesign-external reshapes both sides of the contract

A `redesign-external-system-structure` ticket reshapes an external response
*contract* (e.g. restructuring ERP's `GetProductResponse`) without changing
behaviour. The contract is one interface seen from three sides — the consumer
driver-adapter, the simulated producer, and the stub — so reshaping only the
consumer leaves the simulated producer emitting the old shape and the
real-contract test goes red. The cycle therefore reshapes **both** sides, per
registered external system:

1. **ESCC is the selection source, required up front.** The redesign path runs no
   acceptance/contract cascade, so the file-change proxy (`external-driver-port-changed-paths`)
   that the change cascade populates is empty here. The only signal for *which*
   external system the reshape targets is the ticket's `## External System
   Contract Criteria` (the same `escc-systems` the contract room keys on). A
   `validate-redesign-external-requires-escc` guard hard-errors when ESCC is
   absent — without it every per-system clone's touched-guard is false and the
   cycle would silently no-op.
2. **Per-system unroll, reusing the CT cycle's machinery.** The per-system body is
   a single call-activity anchor that `UnrollExternalSystems` clones once per
   registered system at load time — the *second* anchor that transform rewrites,
   alongside the shared-contract contract cycle — baking `external-system-name` +
   `real-kind` into each clone exactly as the contract cycle does. Each clone runs
   the same `resolve-external-system` + `external-system-touched` self-guard, so a
   system this ticket did not touch skips.
3. **Consumer reshape, then probe-driven producer reconcile.** A touched clone
   reshapes the consumer adapter, then reuses the extracted
   `reconcile-external-contract-producer` leg verbatim: it probes contract-real and
   only acts on a red. `real-kind: simulator` (we own the mock-server) reshapes the
   simulator; `real-kind: test-instance` (a vendor sandbox we cannot restructure)
   halts on an upstream contract gap. A behaviour-preserving reshape that the
   in-process stub already honours passes the probe at zero cost. The redesign path
   writes no contract tests, so the reused leg's test-name selector is pinned empty
   — its probes run the whole contract suite for the touched system.
4. **Full regression last.** After the unrolled clones, the cycle re-greens the
   whole system via `implement-and-verify-system` (`action: update-system`), because
   either side of the boundary can shift the system's port surface.

## Legacy baseline (`characterize`)

`characterize` is a TOP process for a one-off baseline of a legacy system: it writes
acceptance tests (and the external layer: contract tests, external drivers, stubs,
simulators) that pin what the system does today, use case by use case, with no ticket
and no system code change. `cover-system-behavior` stays the per-ticket cycle for later
gaps. Entry point: `gh optivem characterize`.

1. **Started once, stopped once.** `record-start-commits` records each system repo's
   start commit and the protected system paths, then the system starts; it is stopped
   only at the very end. Inner `START_SYSTEM` nodes are no-op probes.
2. **Phase 0, diagrams.** `legacy-use-case-diagram-writer` writes the use case diagram,
   the dependency diagram (`${use-cases-path}/dependencies.md`: a `flowchart LR` whose
   `UC-A --> UC-B` edges mean B needs data A creates) and proposes the most important
   use case. The human approves it or gives feedback.
3. **Pilot, gated.** `order-pilot-use-cases`: the proposed use case plus its transitive
   dependencies, in dependency order. Every step of every pilot use case is approved.
4. **Scaling, ungated.** `order-scaling-use-cases`: every use case not done yet, same
   steps, no approvals. Order is topological (ties by lower UC number); a cycle, an
   edge to an undeclared id or a missing diagram is a hard error.
5. **Per use case** (`characterize-use-case`): narrative (`legacy-use-case-narrative-writer`)
   → `reorder-use-cases` (a newly found dependency that is not done yet goes first and
   the current use case is deferred behind it) → criteria with ESCC
   (`legacy-use-case-criteria-writer`) → `characterize-test-layers` (external → AT →
   DSL → driver adapter, the existing wrappers, each run against the running system)
   → `remove-wip-gate` → `delete-use-case-criteria`. Every step commits
   (`USE CASE NARRATIVE (<id>)`, `USE CASE CRITERIA (<id>)`, layer commits,
   `WIP GATE REMOVAL (<id>)`, `CRITERIA DELETION (<id>)`). The diagrams and narratives
   stay as requirements documentation; the tagged tests become the spec.
6. **Safety gate after every commit, and once at the end.** `verify-system-unchanged`
   runs `check-system-unchanged` (diff plus untracked files under the system paths
   since the start commit) and `check-no-assertion-removed` (test paths, contract tests
   included). Either failure is an error end: a hard halt, no fix agent, no scope
   exception. Stub and simulator paths are not protected: they are test doubles the
   baseline may write.
7. **Tests adapt to the system, never the reverse.** In `characterize` mode,
   `verify-tests-pass` / `verify-tests-fail` route failures to
   `characterization-test-fixer`, whose write scope covers tests, DSL, drivers, stubs
   and simulators but not system code. When the fixer (or the external producer
   refresh) changed a stand-in, only the stand-ins are restarted:
   `gh optivem system start --restart --external-only`.
8. **Feedback on approve.** Approve sites whose "rejected" edge loops back to a step set
   `ask-for-changes: "true"`: a "no" asks "What should change?" and stores the answer as
   State `feedback`, which the dispatcher renders into `${feedback-section}` of the redone
   agent's prompt (empty when unset; a "yes" clears it). Approvals nested inside a redo
   do not ask, so they leave the feedback alone.
9. **Resume.** Re-running the command continues: done use cases are re-derived from the
   files (a tagged acceptance test exists and the use case holds no criteria), and the
   resume unit id of a layer is keyed per use case (`<parent>@<node>@UC-002`), so one
   use case's completed layer does not skip another's.

## Spec-driven stages: executor, reviewer, approver

The 2026-11 redesign (from the "Spec-Driven Development for QA" talk, the 2026-11-06
articles and the 2026-10-30 Circle Q&A) gives every stage the same three roles: an
**executor** agent does the work, a **reviewer** agent checks it adversarially against
the spec without changing anything, and a **human approver** decides. Humans review what
agents cannot judge; every issue a human catches becomes a rule the reviewer applies next
time. Current state per stage:

| Stage | Today (in `process-flow.yaml`) | Still target |
|---|---|---|
| Align | `refine-backlog-item` loops: refiner drafts criteria (concrete example per business rule, contradictions with the use cases, shortest main success scenario), the human answers "Three Amigos agreed?", a "no" feeds back into the next round; 5 rounds max (`AC_REFINEMENT_EXHAUSTED`). | — |
| Merge into use cases | `use-case-merger` writes narrative + `BR-n` business rules + Gherkin criteria tagged `@UC-NNN.M @narrative-Xy`; `VERIFY_STORY_MERGE` checks tags and rule references; `LINK_STORY_CRITERIA` writes the touched ids into the ticket's "Use Case Criteria" section. | — |
| Red | `execute-agent` takes an optional `reviewer`: the acceptance-test, DSL and driver-adapter writers are each checked by a read-only `*-reviewer`; blocking findings return to the writer (2 passes max); the approval names the approver role and lists findings. Channels run API before UI. | — |
| Green | Once the last channel is green, `remove-wip-gate` strips `GH_OPTIVEM_RUN_WIP_TESTS` from the story's tests (also in `cover-system-behavior`), so CI runs them from then on. For a story, `APPLY_AC_POLICY` then deletes its `merged-criteria` from the use cases (deterministic, no agent; a criterion missing from the use cases fails the run) and `COMMIT_AC_POLICY` commits the deletion: the tagged tests are the spec, and `gh optivem spec generate` renders them back as Gherkin for POs; `spec check` keeps gating them as delivered criteria (from their tests). Spec fingerprints are retired. | — |
| Refactor & review | The `refactor` menu offers `review-code-and-architecture`: reviewer proposes a plan, human approves, refactorers execute it with the full suite, human lists issues, `guidelines-updater` records rules under `${guidelines-path}` and fixes other occurrences (3 rounds max). | UX/UI review ([plan](../plans/20261109-1302-ux-ui-review.md)); running only the touched use case's tests after refactor ([discussion](../plans/20261109-1250-refactor-touched-use-case-tests.md)). |
| Legacy | `cover-system-behavior` pins the expected result to success (per ticket). `characterize` baselines a whole legacy system in one run (see [Legacy baseline](#legacy-baseline-characterize)). | — |

Statuses are unchanged: gh-optivem ends at IN ACCEPTANCE; manual QA (quick functional,
exploratory and usability testing) happens outside it, and a human sets DONE.

## Open explorations

Deferred during design; not yet settled:

- **`spike` ticket type** — no gateway mapping today; a `spike` would hard-exit. Open
  question whether it maps to a research cycle or sits outside `implement-ticket`.
- **Cover subtype split** — whether `task/cover-legacy` should split into
  `-acceptance` / `-contract` to make the test-layer explicit, vs. `cover-system-behavior`
  handling both internally.
- **`fix-*` inventory** — the closed set of failure kinds and their `fix-<kind>.md`
  prompts need enumerating; the convention assumes the prompt exists.
