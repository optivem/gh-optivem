# Guarantee test-harness dependencies before the first test run (add a setup-tests BPMN step)

## Motivation

Rehearsal `#61` (`redesigning-new-order-ui`, TypeScript) crashed in the GREEN
verification with a **missing-dependency** error, not a test-logic failure:

```
Error: Cannot find package '@playwright/test' imported from
  ...worktrees/rehearsal-...-61.../system-test/typescript/playwright.config.ts
  code: 'ERR_MODULE_NOT_FOUND'
```

`npx playwright test` ran inside the worktree's `system-test/typescript`, but
`@playwright/test` (a devDependency) was never installed there — `node_modules`
was absent. The fix loop then burned **both** of its attempts
(`FIX_LOOP_EXHAUSTED`) asking `system-updater` to fix *system code*, which can
never resolve a missing npm package.

Two distinct defects compound here:

1. **No path-independent guarantee that test deps are installed** before the
   first `run-tests`. Today it happens only incidentally, on test-authoring
   paths, and the redesign path skips it entirely.
2. **A missing-dependency runtime error is classified as a test `fail`** (→ fix
   loop) instead of an infra error (→ immediate halt), so two expensive
   `opus·high` fix passes are wasted on an unfixable condition.

## Current behaviour (verified against code)

**Dependency install only happens on test-authoring paths.** The only thing
that runs `npm ci` is the `compile-tests` MID process (`gh optivem test
compile` → `npm ci && tsc`). It is wired into exactly three places, all
test-authoring/refactor:

- after `WRITE_ACCEPTANCE_TESTS` — `process-flow.yaml:1054`
- after `REFACTOR_TESTS` — `process-flow.yaml:1502`
- after `RUN_ACTION` in the filtered-run path — `process-flow.yaml:1587`

**The redesign path touches no tests, so it never installs deps.** The crash
trace's call chain was `main → implement-ticket → redesign-system-structure →
implement-and-verify-system → verify-tests-pass`:

- `redesign-system-structure` (`process-flow.yaml:516`):
  `UPDATE_SYSTEM_DRIVER_ADAPTERS → IMPLEMENT_AND_VERIFY_SYSTEM`.
- `implement-and-verify-system` (`:1397`): `RUN_ACTION → BUILD_SYSTEM →
  START_SYSTEM → VERIFY_TESTS_PASS → COMMIT_SYSTEM`. **No `COMPILE_TESTS`, no
  setup.**
- `verify-tests-pass` (`:1603`): `RUN_TESTS` = `gh optivem test run`, which
  **does not** run `setupCommands` or `npm ci`.

So a fresh worktree (`node_modules` is gitignored, never copied in) reaches
`run-tests` with no `node_modules` → `ERR_MODULE_NOT_FOUND`.

**`setupCommands` is explicit-invoke only.** `gh optivem test setup` runs
`setupCommands` from `tests.yaml`; the runner never runs it implicitly
(`test_commands.go:110`, `:45` — documented lifecycle is `test setup` (once) →
`system start` → `test run`). There is **no `setup-tests` BPMN process** in
`process-flow.yaml` today — only `compile-tests` and `run-tests`.

**This is TypeScript-specific.** The other tiers self-bootstrap from the suite
command, so they would not have failed on the redesign path:

- Java suite is `gradlew test …`; `test` depends on `compileTestJava`, which
  resolves deps from the Gradle/Maven cache and compiles test sources on its
  own.
- .NET `dotnet test` implicitly restores + builds first.
- TypeScript `npx playwright test` only *consumes* an already-present
  `node_modules` — it never installs the project's devDependencies. (npx fetches
  the bare `playwright` package into its global cache, which is why a command
  ran at all, but the project's `@playwright/test` is never installed.)

**The infra branch already exists but wasn't taken.** `verify-tests-pass`'s
`GATE_TESTS_OUTCOME` has a `test-outcome == infra → TESTS_INFRA_HALT` branch
(`process-flow.yaml:1687`). The module-not-found error was classified `fail`,
not `infra`, so it entered the fixer loop instead of halting.

## Test setup is idempotent — the real invariant

`gh optivem test setup` is **safe to run once or many times**: TS `npm ci` does
a clean reinstall, Java `gradlew clean compile…`, .NET `restore` — N runs leave
the same state as one. So the goal is **not** "exactly once." It is:

- **At least once before the *first* `run-tests`** — the defect we hit.
- **Re-run after any package manifest could have changed** (an authoring agent
  edits `package.json` / `build.gradle`). This already happens via the `npm ci`
  inside `compile-tests` on the authoring paths — so that install is a
  deliberate re-sync, **not** dead redundancy (revises Open Question C).
- **Never on the fix-loop back-edge.** `verify-tests-pass` re-enters `RUN_TESTS`
  directly via `FIX_UNEXPECTED_FAILING_TESTS → RUN_TESTS`
  (`process-flow.yaml:1692`). Setup must sit *before* that loop, or every fix
  iteration pays a fresh `npm ci`. This is the main placement constraint.

Because repeats are free of correctness risk, the design can lean on
"install up front + idempotent re-sync where manifests change" rather than
threading an exactly-once guarantee through every path.

## Proposed approach

Two fixes, primary + defense-in-depth:

1. **Primary — establish the invariant structurally.** Guarantee deps are
   installed **before any path can reach the first `run-tests`** (idempotent, so
   a later re-sync is free). Add a `setup-tests` MID process wrapping
   `gh optivem test setup` and dispatch it near the top of `implement-ticket`,
   before the ticket-kind gateway and before the fix loop — so every ticket kind
   (story / bug / all task subtypes), not just redesign, is covered, and the
   guarantee no longer rides on the incidental `compile-tests` on the authoring
   paths.

   `gh optivem test setup` is language-agnostic at the BPMN layer: the runner
   resolves the active language's `setupCommands` (npm ci for TS, gradle clean
   compile for Java, restore for .NET). One step, correct for every tier.

2. **Defense-in-depth — classify dependency/module-resolution failures as
   `infra`.** A `run-tests` invocation that fails because the harness can't
   resolve its own packages (`ERR_MODULE_NOT_FOUND`, missing `node_modules`,
   unresolved gradle/nuget deps) is an infra failure, not a red test. Route it
   to `TESTS_INFRA_HALT` so it halts immediately with a clear message instead of
   burning two `opus·high` fix passes. This makes the system fail loud at the
   true cause even if the install step is ever skipped or fails.

## Items

1. **Add the `setup-tests` MID process.** Mirror `compile-tests`
   (`process-flow.yaml:2261`): a single `execute-command` node dispatching
   `gh optivem test setup`, `task-name: setup-tests`, `category: command`
   (cheap, no AI cost). Add its end-event and sequence-flow.

2. **Dispatch it in `implement-ticket`, before the loop.** Insert a
   `SETUP_TESTS` call-activity between `PARSE_TICKET` and `GATE_TICKET_KIND`
   (`process-flow.yaml:266–271`): re-point `PARSE_TICKET → SETUP_TESTS` and add
   `SETUP_TESTS → GATE_TICKET_KIND`. Ticket-kind-independent placement = one
   install before every cycle, and crucially *outside* `verify-tests-pass`, so
   the `FIX_* → RUN_TESTS` back-edge never re-triggers it. **Do not** place
   setup on the `run-tests` / `verify-tests-pass` path itself. (Alternative
   placement: top of `main`; see Open Question A.)

3. **Classify dependency/module-resolution errors as `infra` in the runner.**
   Locate where `gh optivem test run` maps a suite's non-zero exit to the
   `test-outcome` binding (`pass` / `fail` / `infra`) and add detection for
   harness-resolution failures — `ERR_MODULE_NOT_FOUND` / missing
   `node_modules`, and the gradle/nuget equivalents — so they yield `infra`, not
   `fail`. Fail loud with the true cause in the halt message (e.g. "test harness
   dependencies not installed — run `gh optivem test setup`"). Verify the exact
   classifier location before editing.

4. **Regenerate the process diagram from the YAML.** `docs/process-diagram.md`
   and any diagram SVGs are generated — do **not** hand-edit. Let the normal
   regeneration path (CI) pick up the new node; do not run regeneration locally.

5. **Tests.**
   - BPMN: a static-structure / reachability check that `setup-tests` is on the
     path to `run-tests` for every ticket kind (the `bpmn-logic-audit` surface),
     and that `gh optivem test setup` dispatches before any `gh optivem test
     run`.
   - Runner: a unit test that a module-not-found / missing-`node_modules`
     stderr is classified `infra`, not `fail`.

6. **Docs.** Update the lifecycle note in `test_commands.go` (`:45`) and any
   ATDD process doc that describes the run order to reflect that `setup-tests`
   is now an orchestrated step, not only a manual one.

## Open questions

- **A. Placement: `implement-ticket` vs `main`.** **(Recommended)** Put
  `SETUP_TESTS` in `implement-ticket` after `PARSE_TICKET` — it's the narrowest
  scope that still covers every ticket kind, and it sits next to the other
  per-ticket bookends (`MARK_IN_PROGRESS`, `MARK_IN_ACCEPTANCE`). Alternative:
  top of `main` (`:157`) if there are non-`implement-ticket` entry paths that
  also reach `run-tests` — check `main`'s other branches (`MARK_IN_REFINEMENT`,
  refine-backlog) before deciding. Recommend `implement-ticket` unless such a
  path exists.

- **B. Reuse cost only (correctness is settled by idempotence).** Running
  `setup-tests` unconditionally is always *correct* — the only question is the
  few seconds `npm ci` costs on a real-repo run whose `node_modules` already
  exists. **(Recommended)** Keep it unconditional (simple, always-correct); if
  that cost ever matters, add an "already-present" short-circuit **in the
  runner**, not in the BPMN, so the idempotence guarantee stays intact.

- **C. Keep `compile-tests`' embedded `npm ci`.** **(Recommended)** Leave it.
  Per the idempotence section, that install is the deliberate re-sync after an
  authoring agent may have edited `package.json` — removing it would reintroduce
  a manifest-drift gap on the authoring paths. Decoupling install-from-`tsc` is
  a separate refactor and not needed here.

- **D. Should Item 3 (infra classification) ship even if Item 1/2 fully fix the
  path?** **(Recommended)** Yes — it's defense-in-depth and a general
  correctness fix (a red test should never be reported for an unrunnable
  harness, regardless of cause). Keep both; they're independent.

## Out of scope

- Splitting `compile-tests` into separate install vs `tsc` steps (Open
  Question C follow-up).
- Any change to how `node_modules` is gitignored or to the rehearsal harness's
  worktree creation/teardown.
- Per-language `setupCommands` content in `tests.yaml` — this plan orchestrates
  *when* setup runs, not *what* it does.
