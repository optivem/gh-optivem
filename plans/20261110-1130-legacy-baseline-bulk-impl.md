# 2026-11-10 11:30 — Legacy baseline: bulk process (implementation)

## TL;DR

**Why:** The published article "Acceptance test coverage with AI" (`substack/articles/published/2026-11-06-PAID-ATDD-acceptance-test-coverage-with-ai.md`) describes a legacy on-ramp, but gh-optivem only has the per-ticket `cover-system-behavior` (`task/legacy-coverage`). It writes passing tests for given criteria, but generates no diagram, narratives or criteria, and starts the system again on each ticket, which is too slow for a whole-system baseline.
**End result:** a separate, one-off **bulk baseline** process that baselines a whole legacy system layer by layer, with the system started once and a guarantee that no system code changes. `cover-system-behavior` stays unchanged for later ad-hoc gaps.

## ▶ Next executable step (resume here)

Step 1: verify where the current flow starts/stops the system, then draw the flow (design). Use `/refine-plan` or `/execute-plan` on this file.

## Decisions (user, 2026-11-10)

- **Two paths.** A separate **bulk baseline** process (one-off, whole system) plus the existing ad-hoc per-ticket `cover-system-behavior` (unchanged). The bulk flow is its own entry point, **not** a kickoff mode; kickoff stays greenfield-only (nothing in `internal/kickoff/`, which `1121` also waits on).
- **System started once.** In the baseline the system is never modified, so the system is started once at the start and stopped once at the end; the flow does not loop `cover-system-behavior` (which brings Docker up each time). Inner writing/fixing nodes are reused via a "system already up" param, or the start is hoisted to the caller.
- **Layer by layer, in bulk.** Across all use cases: all DSL, then all drivers, etc. — not use case by use case. The **core use case goes first**, through every layer, to establish the architecture.
- **Hand-holding (Q8).** Human approve gates after each layer for the **core use case only**. Bulk layers run unattended. (Be careful with the core use case.)
- **Entry point name:** top-level `gh optivem characterize` (user's choice; beside `kickoff`, separate from it).
- **Core use case choice:** the agent proposes it from the generated use case diagram (with reasons); the human confirms or changes it at the first gate.
- **Deleting criteria at the end:** automatic, as its own commit (revertable with git; `gh optivem spec generate` regenerates readable criteria from tagged tests). Rationale: criteria left behind drift out of date; same as the existing AC policy.
- **Tickets are user stories.** The bulk baseline does not use tickets; the per-ticket path (story with criteria) is for later gaps only.
- **System-code safety gate (Q9, Q8) — mandatory, 100% guarantee.** Two lines: (1) the existing per-agent scope check (`validate-outputs-and-scopes`, snapshot/diff, per-phase allowed paths) with baseline-flow agent scopes that exclude `system-path`; (2) a new **hard** check, `git diff <start-commit> -- <system paths>`, run after each layer's commit and at the end; any non-empty diff HALTS the flow (no fix-agent routing). The start commit is recorded in the flow state at start and kept across resumes. Reason: the existing check is soft (a violation routes to a `fix-scope-diff` agent), per-dispatch only, covers only scoped writing agents, and may not see changes an agent commits itself (verify in Step 1). This is not conditional on what `green-when-complete` already does. Also guard "never weaken an assertion" (assertion-removal diff check).

## Source (article steps)

1. Generate the use case diagram (source code wins over the docs). Human confirms it and picks the core use case.
2. Core use case: narrative + business rules + Gherkin acceptance criteria.
3. Acceptance test architecture.
4. The core use case's tests pass against the existing system. They are characterization tests: on failure, fix the test, criterion and narrative, never the system, and never weaken an assertion.
5. Repeat for the other use cases, one commit per layer (bulk).
6. Delete the criteria.

## Steps

- [ ] Step 1: Verify where `write-acceptance-tests-and-system-adapters` / `cover-system-behavior` start and stop the system; decide between a "system already up" param and hoisting the start to the caller. Verify which paths count as "system code" per language/layout (needed by the safety gate) and whether the `green-when-complete` fix loop can write them. Also verify whether the pre-agent working-tree snapshot sees changes an agent commits itself (`internal/atdd/process/actions/outputs.go`, `modifiedPathsSinceFingerprint`).
- [ ] Step 2: Draw the flow in the BPMN colour legend (new/changed nodes coloured): start system once → diagram + core use case choice (human) → core use case through every layer with human gates → bulk layers over remaining use cases (no gates) → safety gate → stop system → delete criteria.
- [ ] Step 3: Implement in `internal/atdd/process/process-flow.yaml` (+ Go actions for the safety gate and entry point). Entry point is separate from kickoff and from the per-ticket flow. Resumable: state is in git, one commit per layer, so a failure resumes at that layer.
- [ ] Step 4: Tests: process transitions test, safety gate (system path changed → fails; assertion removed → fails), "system started once" invariant.
- [ ] Step 5: Docs: `docs/bpmn-process-design.md`, README/CLI reference for the new entry point.

Edits `process-flow.yaml`: never run in parallel with another plan editing it. Diagrams under `docs/process-diagram*` are regenerated by CI; don't regenerate locally.

## Out of scope

- Parallel per-use-case test runs (possible later; tests are cheap against a running system).
- Narrowing test runs after refactors (see `plans/20261110-1125-consider-narrow-test-runs-after-refactor.md`).
