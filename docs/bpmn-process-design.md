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

## Open explorations

Deferred during design; not yet settled:

- **`spike` ticket type** — no gateway mapping today; a `spike` would hard-exit. Open
  question whether it maps to a research cycle or sits outside `implement-ticket`.
- **Cover subtype split** — whether `task/cover-legacy` should split into
  `-acceptance` / `-contract` to make the test-layer explicit, vs. `cover-system-behavior`
  handling both internally.
- **`fix-*` inventory** — the closed set of failure kinds and their `fix-<kind>.md`
  prompts need enumerating; the convention assumes the prompt exists.
