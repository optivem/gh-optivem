# 2026-06-15 10:47 UTC — Process module follow-up: swappable agent set + deep-untangle (CHILD #8)

> **Sequel to** `20260615-0549-child1-modularize-gh-optivem-engine-process.md` (its deferred Steps 7–10) and grandchild of `20260615-0548-gh-optivem-modular-monolith-parent.md`. The first-pass engine↔definition carve is **DONE and on main**: `internal/engine/statemachine` exposes only `LoadBytes` (embeds no process); `internal/atdd/process` owns `process-flow.yaml` + `Load()`. This child finishes the carve: a separately-bound/**swappable agent set**, both swap axes proven end-to-end, the reuse path documented, and the `atdd/runtime` internals deep-untangled — **but only after** an import-coupling analysis tells us how deep the cut goes.

## TL;DR

**Why:** The engine↔definition seam is broken, but the **agent set is still welded to the ATDD process**: the binding dir `runtime/agents/atdd` is hardcoded in `internal/atdd/runtime/agents/embed.go`, so a third party can't bind an alternate agent set without forking. And the process-specific `actions`/`gates`/`verify`/`driver` still live under `internal/atdd/runtime/` with no clean "process-definition home", despite 25 files there importing the engine core.

**End result:** The agent set is a **parameterised, swappable layer** bound at load time; **both swap axes are proven end-to-end** (a second minimal BPMN via `statemachine.LoadBytes`, *and* the existing process rebound to an alternate agent set); the reuse path is documented ("bring your own BPMN and/or agents, run them on `internal/engine/statemachine`"); and the `atdd/runtime` definition internals are relocated into a process-definition home — guided by a coupling analysis, with **zero behaviour change** to the existing ATDD flow.

## Outcomes

What we get out of this — the goals and deliverables:

- **A swappable agent-set binding.** The hardcoded `agentsDir = "runtime/agents/atdd"` (and the sibling shared-asset paths) in `internal/atdd/runtime/agents/embed.go` are parameterised so an alternate agent set can bind at load time, instead of being fixed at package-init. The ATDD set remains the default.
- **Both swap axes proven end-to-end** (as runnable tests, not just prose):
  - *Process axis* — a second, minimal BPMN loaded via `statemachine.LoadBytes` and driven through the engine, with no engine changes.
  - *Agent axis* — the **existing** ATDD process rebound to an alternate agent set (e.g. a stub/fixture set) and exercised, with no process-flow changes.
- **A documented reuse path** — a short guide: "bring your own BPMN (`LoadBytes`) and/or agents (alternate agent set), run them on `internal/engine/statemachine`" — naming the two contracts and the two swap points.
- **An import-coupling analysis** of `actions`/`gates`/`verify`/`driver` (and their siblings) against `internal/engine/statemachine`: which packages bind to the *generic engine contract* vs. which encode *this specific process*, so the cut line for the move is evidence-based, not guessed.
- **The `atdd/runtime` definition internals relocated into a process-definition home** (the depth driven by the analysis) — `actions`/`gates`/`verify`/`driver` no longer sitting at the generic `runtime` level if they encode the ATDD process.
- **No behaviour change.** The existing `process`/`run`/`implement`/`test` commands behave identically; `go build ./...` + full `go test ./...` stay green at every step (regression-safe refactor).

## ▶ Next executable step (resume here)

**Step 7 — parameterise the agent-set binding dir in `internal/atdd/runtime/agents/embed.go`.** This is a ready-to-execute mechanical edit and the natural first move (it's self-contained and unblocks the agent-axis proof in Step 8). Currently `agentsDir` is a package-level `const "runtime/agents/atdd"` baked into `Prompt`, `LoadTuning`, and `Names`, and the five shared-asset paths (`preamble`/`scope`/`fixer-preamble`/`interactive-suffix`/`headless-suffix`) load once into package-level `var`s at init. Introduce a binding that carries the agent-set root (default `"runtime/agents/atdd"`) so an alternate set can be selected at load time rather than fixed at init — without changing the default ATDD behaviour. Consumers to keep green: `driver`, `clauderun`, and `agents/registry.go` (the five exported funcs `Prompt`/`Names`/`LoadTuning`/`InteractiveSuffix`/`HeadlessSuffix` are called from there). Decide the binding shape (constructor/struct vs. functional option) as the first sub-decision — see Open questions. Stop at: `go build ./...` + `go test ./...` green, ATDD default unchanged.

## Steps

- [ ] **Step 7: Make the agent set a separately-bound, swappable layer.** Parameterise the hardcoded `agentsDir = "runtime/agents/atdd"` in `internal/atdd/runtime/agents/embed.go` so the binding root is supplied at load time, not fixed at package-init. Note the init-time coupling: `sharedPreamble`/`sharedScope`/`fixerPreamble`/`interactiveSuffix`/`headlessSuffix` are `mustReadAsset(...)` package `var`s evaluated once — decide whether the shared chunks stay global or move under the binding too (an alternate agent set may want its own preamble/scope). Keep the ATDD set as the zero-config default. Update consumers (`driver`, `clauderun`, `agents/registry.go`) and keep `go build`/`go test` green.
- [ ] **Step 8: Prove both swap axes end-to-end (as tests).**
  - *Process axis:* author a second minimal BPMN (a tiny fixture flow), load it via `statemachine.LoadBytes`, and drive it through the engine — asserting the engine needs no change. Co-locate as a test fixture (don't ship it as a real process).
  - *Agent axis:* bind the existing ATDD process to an alternate agent set (a stub/fixture set via the Step-7 binding) and exercise dispatch — asserting `process-flow.yaml` needs no change. Reuse the existing `embedded_smoke_test.go` pattern in `driver/` as the harness shape.
- [ ] **Step 9: Document the reuse path.** Write a short guide (placement TBD — see Open questions: `docs/` vs. a package doc.go vs. the process module README) covering the two contracts (`statemachine.LoadBytes` for the flow; the agent-set binding for the roles) and the two swap points, with the Step-8 fixtures as worked examples. Keep it self-contained (no cross-language cross-references).
- [ ] **Step 10a: Import-coupling analysis (DO THIS BEFORE ANY MOVE).** For each of `actions`, `gates`, `verify`, `driver` (and siblings `override`, `trace`, `clauderun`, `preflight`, `configcheck`, `tracker/**`, `intake`, `outlog`, `testselect`, `release`, `repolocator`, `diagram`), classify its dependency on `internal/engine/statemachine`: does it use only the **generic engine contract** (reusable by any process) or does it encode **this ATDD process** (process-definition-specific)? Ground it in actual imports — 25 files under `internal/atdd/runtime` import the engine today. Output: a coupling table + a proposed cut line (what moves into the process-definition home, what — if anything — is generic enough to stay engine-side or go to a shared spot). This step gates Step 10b; consider running it via the `code-auditor` or an `Explore` agent to keep the analysis out of the main context.
- [ ] **Step 10b: Relocate the definition internals into a process-definition home.** Execute the move the analysis prescribes — physically nest the process-specific packages under a process-definition home (candidate: `internal/atdd/process/...` alongside the already-moved `process-flow.yaml`, or a sibling under `internal/atdd/`), updating import paths only. Pure moves, one isolated subagent per move (per parent Resume notes), `go build`/`go test` green after each, commit via the commit skill.

## Resolved decisions (inherited — confirmed in the seed plans)

- **Engine home:** `internal/engine/statemachine`, generic contract `LoadBytes([]byte) (*Engine, error)`, embeds no concrete process. *(done)*
- **Process-definition home (current):** `internal/atdd/process` owns `process-flow.yaml` + `Load()`/`DefaultYAML`. *(done)*
- **Role→agent contract:** YAML `agent:` name → `internal/assets/runtime/agents/atdd/<name>.md`, resolved by the `agents` registry. The *contract* exists; this child makes the *binding* swappable.
- **One repo, internal package boundaries; dependencies point down to the engine; regression-safe (no behaviour change).**

## Open questions

- **Binding shape (Step 7).** Constructor returning a bound `AgentSet` value (methods replace today's package-level funcs) vs. a functional-option / variable default vs. an interface the driver depends on? The driver + clauderun + registry are the call sites — pick the smallest change that makes the root swappable without churning every caller. *Lean: a small bound struct with the five methods, ATDD constructed as the default.*
- **Shared chunks under the binding?** Do `preamble`/`scope`/`fixer-preamble`/`interactive-suffix`/`headless-suffix` belong to the *engine/dispatch* (stay global, shared by all agent sets) or to the *agent set* (move under the binding, so an alternate set brings its own)? Affects how clean the agent-axis swap in Step 8 is.
- **Reuse-doc placement (Step 9).** `docs/` (operator-facing, but watch the no-cross-language rule), a `doc.go` on the process/engine package (developer-facing, lives with the code), or a README in the process module? *Lean: package `doc.go` for the contract + a short `docs/` reuse note if operator-facing is wanted.*
- **Cut line for the move (Step 10b)** — entirely determined by Step 10a. Don't pre-commit to a target dir until the coupling table exists; the candidate homes above are hypotheses.
- **Fixture vs. real second process (Step 8).** The second BPMN is a test fixture, not a shipped process — confirm it stays under test scope and isn't wired into any CLI command.
