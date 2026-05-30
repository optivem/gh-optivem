# Stop writing agents delegating discovery to a scout subagent (and over-reading the port tree)

> **Working style: token-efficient.** Execute in the cheapest form that still produces a quality result; if a costlier workflow is proposed where a cheaper one suffices, surface the cheaper option (memory: `feedback_flag_non_token_efficient`).

> **Prose-only, no diagram regen.** This plan edits prompt bodies under `internal/assets/runtime/...` only — no `process-flow.yaml`, no schema, no diagram. The frontmatter is untouched.

> **Accuracy is the priority — speed is not bought with correctness.** The acceptance tests this agent writes are the keystone artifact of the whole ATDD process: every downstream cycle (DSL impl, system impl, refactor) is driven by them, so an under-informed or wrong test poisons everything after it. Fewer reads is a *consequence* of discovering the right files directly, **never a target**. Where reading more is what produces a correct 1:1 translation, the agent reads more. This plan only removes *wasteful* discovery (a scout that returned a false map; blind full-tree sweeps), not *load-bearing* reads.

## Context

Triggered by rehearsal `rehearsal-69-reject-order-with-line-quantity-of-100-20260530-160041`, where the `acceptance-test-writer` (model `sonnet`, effort `low`) took ≈ **2m 26s** to add a single negative test — roughly half of it wasted. Evidence from the run's `001-acceptance-test-writer.events.jsonl`:

| Milestone | Time (UTC) |
|---|---|
| agent starts | `14:00:57` |
| **dispatches an `Explore` subagent** (+6s) | `14:01:03` |
| tries paths from Explore's summary → `File does not exist` ×2 | `14:01:59` |
| distrusts summary, re-globs + re-reads the real tree | `14:02:15`–`14:02:50` |
| the two actual `Edit`s | `14:03:06`–`14:03:12` |
| `output write`, done | `14:03:23` |

Two distinct inefficiencies, **both pure speed — no contamination** (zero `dsl-core` reads; the implementation was never opened, exactly as mandated):

1. **Scout-subagent detour.** The writer spawned one `Explore` subagent to map the test + DSL surface. Explore returned a tidy prose summary describing the interfaces at **flat** paths (`dsl/port/WhenPlaceOrder.java`, `dsl/port/ThenFailure.java`) that do not exist — the real layout nests them under `port/when/steps/`, `port/then/steps/`, `port/given/steps/` (**17 of the 30** port files live in those subdirs). Every path lifted from the summary 404'd; after two misses the writer threw the whole summary away and rediscovered the tree itself. The entire Explore phase was negative-value: it didn't help, it added a false map the writer had to detect and discard. That round-trip is ≈ the first 60s of the run.

2. **Exhaustive port sweep.** `dsl/port` holds **30** `.java` files; the writer read **22** (~73%). A 1:1 negative-test translation needs ≈ 5–6 surface types (`GivenStage`+`GivenProduct`, `WhenStage`+`WhenPlaceOrder`, `ThenResultStage`+`ThenFailure`) plus the existing sibling test to copy. The rest was "load the whole DSL to be safe" — defensible for correctness, wasteful on `effort: low`.

### The twin: `contract-test-writer`

`contract-test-writer.md` is a structural mirror of the acceptance writer (`model: sonnet`, identical Step 2, writes tests against the existing `${dsl-port}` surface). It must discover the same DSL surface and a sibling test to model on, so it carries the **identical** scout-subagent + over-read risk. It is the *other* keystone test artifact, so it gets the same accuracy-first targeted-discovery guidance. (The shared-preamble fix in Step 1 already covers its scout loophole; Step 2 below extends the positive guidance to it.)

### Other agents — audited, no accuracy fix needed

Swept all 18 ATDD agents for accuracy-negative instructions (model/effort tuning + read-restriction language):

- **Model/effort tuning is sane:** `sonnet` only on the two test-writers + the two refactorers; every implementer/updater and all five fixers run `opus` at `medium`/`high`. No accuracy-critical authoring is under-powered.
- **The remaining read restrictions are intentional, not accuracy bugs**, and are left untouched:
  - The `dsl-core` "do not read existing method bodies" clause in both test-writers is a deliberate guard against leaking implementation context into test design — preserved.
  - `external-system-driver-adapter-implementer`'s "do NOT read external-system source code; rely on the contract tests + published API contract" is contract-driven-by-design (the source is out of scope anyway) — preserved.
  - `system-implementer` Step 1 already instructs *wide* reading (AT → DSL port → DSL core → driver pair → external driver → CT) — accuracy-positive, no change.
- The scout-subagent prohibition (Step 1, shared preamble) covers the scout loophole for **all** of them at once.

### Root cause

`internal/assets/runtime/shared/preamble.md` ("Scope-bound reads", line 12) already says: *"Targeted greps for prompt-named symbols are fine; open-ended exploration is a scope violation."* The agent obeyed the **letter** — it did not browse open-endedly *itself* — while routing around the **intent** by delegating that exploration to a subagent. The preamble never closes the delegation loophole, and the delegated scout is exactly what produced the hallucinated paths. This loophole is identically exploitable by every one-shot writing agent, not just this one.

## Decisions (resolved upfront)

- **D1 — Close the loophole in the shared preamble, not per-agent.** The "no open-ended exploration" rule lives in `preamble.md`, which is prepended to every one-shot dispatch; one edit there covers all writing/fixing agents and avoids N drifting copies (memory: `feedback_drop_dont_relocate`). The acceptance-test-writer is where it surfaced, but the fix is general.
- **D2 — Scope the prohibition to *discovery delegation*, not a blanket subagent ban.** Wording: do not dispatch a scouting/exploration subagent (`Explore`/`Task`/`general-purpose`) to discover files or structure — the scope is already given; do your own targeted greps within it. Keeping it about discovery (rather than "never spawn a subagent") avoids over-reaching and stays aligned with the existing line-12 rule it extends.
- **D3 — Add *accuracy-first* targeted-discovery guidance to both test-writers** (`acceptance-test-writer.md` and `contract-test-writer.md`). Tell each to model the new test on the existing sibling test in its test dir (`${at-test}` / `${ct-test}`) and reach the DSL surface via targeted greps for the methods the scenario/contract exercises — so it lands on the *right* interfaces directly instead of a blind sweep. Framed as guidance with an explicit "read whatever you need for a correct translation" escape, **not** a read cap: the goal is to discover accurately, and reduced reads are the byproduct. The two test-writers are the keystone-artifact authors and share the same discovery shape; the other agents have different shapes, so the positive recipe is not templated into them here (possible follow-up).
- **D6 — No accuracy regression is the acceptance bar.** The steer is correct only if the test it produces is as faithful as before (same shape, correct builders, correct assertions/intent). If targeted discovery would ever leave the agent guessing at the DSL chain, it reads the interface — full stop. Verification gates on test fidelity, not on read count or wall-clock (those are reported, not required to drop).
- **D4 — Frontmatter untouched.** `embed_test.go:74` pins `acceptance-test-writer`'s `model: sonnet` / `effort: low` via `LoadTuning`; edits are prose-body only.
- **D5 — Not adopting the orchestrator-substitutes-the-DSL-surface approach here.** Injecting a precomputed port-surface index into the prompt (so the writer does *zero* discovery) is the stronger fix but a bigger change (the dispatcher must compute and substitute it). Tracked as a follow-up; this plan is the cheap prompt-level steer that removes the detour today.

## Steps

### Step 1 — Close the scout-subagent loophole in the shared preamble

In `internal/assets/runtime/shared/preamble.md`, under **`## Scope-bound reads`** (after the existing "Targeted greps … open-ended exploration is a scope violation." sentence), add a sentence to the effect of:

> Do this discovery yourself with targeted greps and reads inside scope — **do not dispatch a scouting subagent** (`Explore`, `Task`, `general-purpose`) to map files or structure. A delegated scout returns a summary you cannot trust against the real tree and routes around this rule.

Keep it one or two sentences; match the preamble's terse register.

### Step 2 — Add accuracy-first discovery guidance to both test-writers

In `acceptance-test-writer.md` Step 1 (the 1:1-translation step), add a short clause steering discovery — **accuracy-first**:

- Model each new test on the **existing sibling test** in `${at-test}` (same feature/area) — read that one, copy its shape, annotations, and DSL chain.
- Reach the DSL builder surface in `${dsl-port}` via **targeted greps for the methods named in the Acceptance Criteria** (e.g. the product/order/assertion builders the scenario implies), to land on the right interfaces directly rather than reading the whole port tree.
- Add the explicit escape: **read whatever you genuinely need to get the translation right** — when the correct builder/assertion or the chain shape is unclear, read the interface. A faithful test matters more than a short read list.

In `contract-test-writer.md` Step 1, add the mirror clause: model on the **existing sibling contract test** in `${ct-test}`, reach `${dsl-port}` via targeted greps for the methods the contract exercises, with the same "read whatever you need for fidelity" escape.

Phrase as guidance, not a hard cap, in both. Do not touch frontmatter, Step 2, Outputs, or Notes in either file.

### Step 3 — Build + targeted tests

- `go build ./...`
- `go test ./internal/atdd/runtime/agents/... ./internal/atdd/runtime/clauderun/...` — scoped, never unbounded `go test ./...` on Windows (memory: `feedback_go_test_windows`). Confirms `embed_test.go` (frontmatter pin + preamble-prepend assertions) and the prompt-rendering tests still pass after the prose edits.

## Verification

- **Primary gate — test fidelity (must hold).** Re-run rehearsal 69 (or any single-AC ticket) off a fresh build and diff the produced acceptance test against the pre-change baseline: same test shape, same builders, same assertions/intent, WIP gate + channel annotations intact, `dsl-core` still untouched. The steer is accepted only if the test is at least as faithful as before. If targeted discovery yields a worse test on any ticket, the guidance is too aggressive — soften it before landing.
- **Secondary — efficiency (reported, not required).** From the new `acceptance-test-writer.events.jsonl`: confirm **no `Explore`/`Task` dispatch** and note the port-read count and wall-clock (expected to fall from ~22 reads / ≈ 2m 26s). These are observed improvements, not pass/fail criteria — never let a read-count target override the fidelity gate above.

## References

- Diagnosis of rehearsal-69 (event-log timeline, read counts, hallucinated-path evidence): this conversation (2026-05-30).
- Rule extended: `internal/assets/runtime/shared/preamble.md` "Scope-bound reads".
- Agents edited: `internal/assets/runtime/agents/atdd/acceptance-test-writer.md`, `internal/assets/runtime/agents/atdd/contract-test-writer.md`.
- Audit basis (all 18 agents swept for accuracy-negative tuning/read restrictions): this conversation (2026-05-30).
- Frontmatter pin: `internal/atdd/runtime/agents/embed_test.go` (`LoadTuning("acceptance-test-writer")`).
- Follow-up (not in this plan): orchestrator-substituted DSL-surface index to eliminate writer-side discovery entirely (D5).
