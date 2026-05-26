> ⚠️ **DISCUSSION MODE ONLY — NOT READY FOR EXECUTION.**
> This file is a thinking space, not a work plan. There are no checked items, no scope decisions, no agent assignments. Do **not** run `/execute-plan` against it. The point is to surface options and pick a direction *before* anything gets written as an actionable plan. When the direction is locked, a fresh dated plan supersedes this one (per `feedback_new_plan_not_extend`).

# Structured BPMN execution trace — design discussion

## Why this conversation exists

We already have two trace channels for BPMN execution:

1. **Human-readable runtime stream** — `internal/atdd/runtime/trace/trace.go`. Per-node enter/exit banners (`[trace HH:MM:SS] > NODE_ID kind=… …`), `state:` deltas, `files:` deltas for `user_task` nodes, colored on TTY, mirrored to `--log-file`. Good for an operator watching a live `gh optivem implement`. Not parseable.

2. **Structured test-time spy** — `internal/atdd/runtime/statemachine/dispatch_spy_test.go` + `dispatch_expect_test.go`. Captures `[]DispatchEvent` with kind-specific fields (`Action` / `Agent` / `Binding` + `GateValue|GateBool` / `CallTarget` + `CallParams`) and a `ParamsIn` snapshot at every dispatch. Asserted via the `expectDispatch` fluent builder with `reflect.DeepEqual` and a `formatEvents` diff. Drives the structural- and behavioral-cycle tests.

The spy is **only wired with noop mocks**: `AgentFn` / `ActionFn` return `Outcome{}`, `GateFn` reads pre-seeded `ctx` state. By design — see `dispatch_spy_test.go:19-20` (*"Outcome.Err / Outputs are not captured: under noop mocks they carry no signal"*).

That leaves a gap worth thinking about:

- **What did a real action mutate in `ctx.State`?** Not captured anywhere structured.
- **What `Outcome.Value` / `Outcome.Outputs` did a real (non-mocked) dispatch produce?** Only visible in the human-readable trace's `state:` and `OK/RED/INFRA` words.
- **Cross-cycle invariants over a real run** (e.g. "every `COMMIT` is preceded by a successful `verify` in the same scope") — currently asserted ad hoc inside per-cycle expected-event lists.

## Three directions, not mutually exclusive

### Direction A — JSONL emit alongside the runtime trace

Add a `--trace-events <path>` driver option that writes one `DispatchEvent`-shaped JSON object per line to a file (in addition to the existing `--log-file`). Real `Outcome` populated; real `ctx.State` delta included.

Shape sketch:
```json
{"ts":"14:04:18.231","process":"red_phase_cycle","node":"WRITE","kind":"user_task","agent":"at-red-test","params_in":{"agent":"at-red-test","phase_id":"AT_RED_TEST",...},"outcome":{"value":"","bool":false},"state_delta":{"prompt_log":"/repo/.gh-optivem/runs/003-at-red-test.prompt.md"},"files":["tests/acceptance/checkout_story42_test.go"],"elapsed_ms":136421}
```

**Pros:**
- Parseable. A fixture-based golden test for a full pipeline becomes possible.
- The shape already exists (`DispatchEvent`) — reuse, don't redesign.
- Operator can `jq` over a real run for triage.

**Cons:**
- Emission belongs at the same decorator layer as `trace.go`, so any new field added later has to be added in two places (or one of the two refactored to derive from the other).
- File grows fast on long runs — pruning policy lives with `--keep-runs` already, so probably fine.

**Open question:** does the JSONL file *replace* the per-prompt `.prompt.md` files, or supplement them? They overlap in part (prompt-log path is referenced from both) but the per-prompt files are richer for agent forensics. Likely supplement, not replace.

### Direction B — Invariant DSL over `[]DispatchEvent`

Today, a test like "every `COMMIT` call_activity is preceded by a successful `verify` in the same scope" is asserted *by hand* via the expected-event list — every cycle test re-spells the sequence. A small DSL would let those invariants be expressed once and re-checked against any captured trail.

Sketch:
```go
invariants.Check(events,
    invariants.EveryCallTo("commit").IsPrecededBy(
        invariants.GatewayInSameScope("verify_class").WithValue("ok"),
    ),
    invariants.EveryUserTask().IsFollowedBy(
        invariants.ServiceTask("run_tests").InSameScope(),
    ),
)
```

**Pros:**
- Catches new-call-site regressions (the most common way a test misses something today is "I added another `COMMIT` site and forgot to assert the verify before it").
- Decouples *what must always be true* from *which test currently exercises it*.
- Works on both spy-captured and JSONL-captured events with no shape change.

**Cons:**
- Easy to over-engineer. A handful of helper funcs over a `[]DispatchEvent` slice probably gets us 80% there without a builder.
- Risks duplicating routing logic the gates already enforce. Need to be careful that invariants assert *external* expectations (e.g. doctrine, prior-incident rules), not just restate the YAML.

**Open question:** is the right shape a DSL, a set of plain test helpers, or actually a lint pass over the YAML itself (so violations show at `gh optivem` build time, not at test time)?

### Direction C — Opt-in state-mutation capture on selected service tasks

The spy mocks everything to `Outcome{}` for speed. But sometimes you really do want to assert "this service task wrote `issue_num=42` to `ctx.State`" without spinning up the full live driver.

Option: a per-node opt-in (`StateAware: true`) that lets the spy run the *real* registered action for that node only, then captures the post-state delta on the event.

**Pros:**
- Targeted — pay the cost only on the nodes you care about.
- Lets a single test cover orchestration + the action's contract without two layers of fixtures.

**Cons:**
- Smells like the start of "integration test" creep into orchestration tests. The current split (orchestration tests = routing + params; action tests = state + outputs) is clean *because* it forbids this.
- One more knob in the spy.

**Open question:** is there even a current test that wants this, or is it speculative?

## What we are *not* discussing

- Replacing the human-readable runtime trace. Operators read it, agents read it (via `--log-file` excerpts in bug reports), and it would not be improved by structured emission.
- Adding OpenTelemetry / spans / external telemetry sinks. No external consumer asked for it; the audience is local operators and tests.
- Per-event timestamps in the spy. The spy is deterministic by design; timestamps would defeat `reflect.DeepEqual`.

## Discussion notes

### Direction A — DECIDED: ship it

Locked in (2026-05-25 walk). A is worth doing on its own merits — lowest-risk, highest-leverage of the three. Rationale:

- Reuses the existing `DispatchEvent` shape — no schema design needed.
- Operator can `jq` over a real run for triage (parseable artefact, not just colored stream).
- Enables fixture-based golden tests for full pipelines later.

Settled sub-decisions:

- **Supplement, not replace** the per-prompt `.prompt.md` files. They cover different forensic needs (prompt/response text vs. routing/state). JSONL does not subsume them.
- **Single source of truth for event shape.** Either extract a shared "build event record" helper that both `trace.go` formatting and the JSONL serializer consume, or pick one as authoritative and derive the other. Decide which at plan-drafting time, not now.
- File-growth pruning rides on the existing `--keep-runs` machinery.

Carry-forward for the fresh plan: name the driver flag (`--trace-events <path>` is the sketched name), specify the JSON shape vs. the in-memory `DispatchEvent` struct, and decide the shared-emit-path refactor up front so we don't ship two divergent emitters.

### Direction B — DECIDED: ship as plain helpers, not a DSL

Locked in (2026-05-25 walk). Adopt the diagnosis (the "new call site forgot the upstream gate" failure mode is real and recurring) but reject the fluent-builder shape from the sketch.

Settled sub-decisions:

- **Plain Go helpers over `[]DispatchEvent`** — e.g. `assertCallPrecededByGate(events, "commit", "verify_class", "ok")`. No `EveryCallTo(...).IsPrecededBy(...)` chain syntax.
- The "easy to over-engineer" con in the original sketch is the reason: a handful of helpers gets ~80% of the value with none of the chain-shape lock-in, and the third invariant that doesn't fit a builder shape always shows up.
- B's helpers should be **callable from both spy-captured and JSONL-captured event slices** (same shape). This dovetails with Direction A — without A, helpers only ever run against spy fixtures and just restate what per-cycle expected-event lists already cover.
- The "YAML lint pass" option was rejected as B's primary shape, but if specific invariants turn out to be purely structural (graph-level, not runtime-level), they can be added to the YAML loader separately — not as part of B.

Carry-forward for the fresh plan: enumerate the 2-3 invariants we'd write helpers for first (concrete enough to stress-test the helper shape), and decide where they live (`internal/atdd/runtime/statemachine/invariants_test.go`? a non-test package so they're reusable from JSONL post-mortems?).

### Direction C — DECIDED: reject

Locked in (2026-05-25 walk). The current orchestration/action test split is clean **because** it forbids state-mutation capture inside orchestration spies; weakening that forbiddance via a `StateAware: true` per-node knob invites exactly the slippery-slope the original cons section names ("once one node has it, the next reviewer asks why not this one").

The use cases C tried to serve are already covered:

- "Did this action mutate state correctly?" → action-level test calling the action directly with a seeded `ctx`. No spy, no orchestration glue.
- "Did orchestration + action together produce the right end-state?" → end-to-end / driver-level tests, with Direction A's JSONL output as the parseable artefact.

The open question in the original proposal ("is there even a current test that wants this?") is itself the answer: no concrete test was named, and the two cheaper alternatives above cover the named pros.

No carry-forward — C does not appear in the fresh plan.

---

## Summary of decisions

| Direction | Outcome | Shape |
|-----------|---------|-------|
| A — JSONL emit | **Ship** | `--trace-events` driver flag; supplements `.prompt.md`; shared event-record source between trace formatter and JSONL serializer |
| B — Invariant checks | **Ship** | Plain Go helpers over `[]DispatchEvent`; no fluent DSL; reusable from A's JSONL captures |
| C — Spy state capture | **Reject** | Load-bearing split; use action-level tests + A's JSONL instead |

Next: write a fresh `plans/YYYYMMDD-HHMM-<slug>.md` that turns A and B into actionable items (per `feedback_new_plan_not_extend`). This file gets archived to `plans/deferred/` once the fresh plan is drafted.

## Decision criteria (working list)

When this discussion converts into an actionable plan, the chosen direction should answer:

1. **Is there a current test (or recurring failure pattern) that's painful today?** If A/B/C doesn't make at least one existing test smaller or one bug-class harder to ship, it's not earning its weight.
2. **Does it add a second source of truth?** If yes, name which one is authoritative and how the other is derived.
3. **Who is the consumer?** Operator-during-triage, test author, or external tool — the answer changes the shape (color stream / Go helper / JSONL).

## When this file is superseded

When direction is locked, archive this file under `plans/deferred/` (or delete) and write a fresh `plans/YYYYMMDD-HHMM-<slug>.md` with the actionable steps. Do **not** edit this file into an executable plan — the warning at the top is load-bearing and removing it mid-flight is exactly the failure mode `feedback_new_plan_not_extend` exists to prevent.
