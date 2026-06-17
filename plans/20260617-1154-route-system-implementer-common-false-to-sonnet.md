# Route system-implementer `common:false` dispatches to sonnet

## Motivation

Analysis of the rehearsal run on shop #72 ("Charge shipping based on product weight from ERP") showed the agent fleet is the cost center (79% of wall-clock, $5.97 of the $6.97 on opus). A first pass downgraded three uniformly-shallow, hard-gated agents from opus to sonnet (commit `488adc5`): both driver-adapter implementers and the dsl-implementer.

`system-implementer` was deliberately left on opus — but it is **not a uniform workload**. It bifurcates on the `common` flag:

- **`common: true`** (first channel of the cycle) → builds the channel-agnostic **common** layer (DTOs, entities, service logic) **and** authors a forward-only Flyway migration shared across all 6 SUTs (3 langs × 2 architectures). Deep design + irreversible, high-blast-radius output that the single channel suite does **not** fully verify → genuinely needs opus.
- **`common: false`** (every later channel) → *"implement only the `${channel}` adapter delta that wires this channel to the already-built common layer"* (`system-implementer.md` step 2). Shallow translation, no migration, no common-layer change, hard-gated by `acceptance-${channel}`. This is the **same quadrant as the adapter agents already downgraded** — sonnet-grade.

In the #72 run this was visible: `implement-system` api (`common: true`) cost $1.71 / 4m38s; ui (`common: false`) cost $0.67 / 1m38s. The later-channel delta is cheap work paying the opus tax. Routing it to sonnet banks ~$0.67/run **per extra channel** with zero added migration risk, because the migration-bearing first-channel dispatch stays on opus.

## Current behaviour (verified against code)

Model is resolved **once per agent** from the agent's frontmatter and threaded unchanged into each dispatch:

- The dispatcher sets `cOpts.Model = tuning.Model` at the single dispatch site (`internal/atdd/runtime/driver/driver.go:1336`), where `tuning` is the frontmatter-derived tuning for the resolved agent.
- `clauderun` turns a non-empty `opts.Model` into the `--model` flag on the `claude` subprocess (`internal/atdd/process/clauderun/clauderun.go` → `claudeTuningArgs`, ~line 1865). Empty → no flag, inherits session default.

So `Model` is already a **per-dispatch field**, not a static constant — nothing structurally prevents a per-dispatch override.

The `common` value is already present at that exact site:

- Scoped (per-channel) path: `sCtx.Params["common"] = boolStr(channel == cfg.Channels[0])` (`internal/atdd/runtime/driver/scoped.go:152`).
- Full no-arg run: bound per channel by `UnrollSystemChannels` (same first-channel rule, `driver.go` ~line 341).
- Either way it flows into `nodeParams` (`driver.go:1111–1120`), which is in scope where `cOpts.Model` is set.

**Conclusion: the routing is wireable with the data already on hand at `driver.go:1336` — no new plumbing, no new param.**

## Why this is safe

`common: false` dispatches touch only the per-channel adapter delta and are gated by `acceptance-${channel}` — a wrong adapter fails its suite loudly and immediately, with no spillover to other channels or SUTs. The thing that makes the *first-channel* dispatch risky (the shared, forward-only, under-verified migration) is **absent** on `common: false` by construction (`system-implementer.md` step 3 authors the migration only when `common: true`). So the verifier fully covers the downgraded path.

## Mechanism decision (resolved)

The rule lives as a **small named policy function in code**, not a config field. Three options were weighed; the rejected two are recorded under *Alternatives*.

The deciding reasons for the code function:

1. **It is an N=1 rule today.** `system-implementer` is the only agent whose difficulty bifurcates on a param. A frontmatter "conditional model" grammar would be exercised by exactly one agent — schema that doesn't earn its slot.
2. **Single source of truth for the default model stays in frontmatter.** The override is one named, commented exception next to where the model is already chosen — not a second file answering "what model does this agent use."
3. **The data is already at the dispatch site.** Both `tuning.Model` and `nodeParams["common"]` are in scope at `driver.go:1336`.
4. **Pure and testable**, mirroring the existing `common`-value tests in `scoped_test.go`.

## Items

### 1. Add `resolveDispatchModel`

**File:** `internal/atdd/runtime/driver/driver.go` (near the dispatch construction, ~line 1336).

Add a pure function:

```go
// resolveDispatchModel returns the frontmatter model unchanged, except for the
// one documented exception: a system-implementer dispatch on a later channel
// (common:false) does only the per-channel adapter delta — shallow translation
// hard-gated by the acceptance-${channel} suite — so it is routed to sonnet.
// The first channel (common:true) authors the shared forward-only migration and
// stays on the frontmatter model (opus). N=1 rule; promote to a declarative
// mechanism only when a second param-conditional agent earns it (rule of three).
func resolveDispatchModel(agentName, frontmatterModel string, nodeParams map[string]string) string
```

- Default: return `frontmatterModel`.
- Exception: when this is the system-implementer dispatch **and** `nodeParams["common"] == "false"` → return `"sonnet"`.
- Call it at the dispatch site: `Model: resolveDispatchModel(agentName, tuning.Model, nodeParams)`.

**Sub-task — confirm the agent identifier.** The frontmatter file is `system-implementer.md` but the BPMN task is `implement-system` (`driver_test.go:640`). Key the rule on the exact value `agentName` holds at `driver.go:1336` — confirm the literal token before hardcoding the comparison, and use a named constant if one already exists for this task rather than a bare string literal.

### 2. Cross-reference comment in the agent frontmatter

**File:** `internal/atdd/assets/runtime/agents/atdd/system-implementer.md` (frontmatter comment).

Because the override lives in code, `model: opus` is now a half-truth — `common:false` dispatches run on sonnet. The frontmatter comment **must** point to the exception so a reader of the frontmatter is not misled:

> `common:false` (later-channel adapter delta) dispatches are routed to sonnet in `resolveDispatchModel` (driver.go); the `common:true` first channel authors the shared migration and stays on opus.

Keep `model: opus` as the declared default — it is correct for the worst-case path the frontmatter must size for.

### 3. Tests

**File:** `internal/atdd/runtime/driver/driver_test.go` (or a focused `resolveDispatchModel` table test).

Pure-function table test:
- system-implementer + `common: "false"` → `"sonnet"`.
- system-implementer + `common: "true"` → frontmatter model (e.g. `"opus"`) unchanged.
- system-implementer + `common` absent/empty → frontmatter model unchanged (no silent downgrade on a malformed dispatch).
- a different agent + `common: "false"` → frontmatter model unchanged (the rule is system-implementer-only).
- empty frontmatter model + non-matching case → empty (session default preserved).

## Verification

(User-driven — not agent steps.)

- Rebuild the `gh optivem` binary if `internal/atdd/assets/` is embedded, then run the rehearsal corpus across **several** tickets (not just #72) and confirm: the `implement-system` later-channel dispatch reports `sonnet` in the agent summary's model column, the first channel still reports `opus`, and all suites stay green.
- Spot-check one multi-channel ticket's later-channel commit to confirm the sonnet delta is correct, per the "accuracy over speed" gate — a passing channel suite plus a correct-looking adapter delta.

## Alternatives considered

### Conditional-model frontmatter field (rejected)

Add a frontmatter grammar like `model-when: {common=false: sonnet}`. Rejected: it splits routing across two places — a condition→model *map* in frontmatter plus a condition *evaluator* in the dispatcher — for zero second consumer. It is the clean option only if future param-conditional agents are imagined into existence. Revisit under the rule of three.

### Process-flow param override (rejected hardest)

Express a per-node model override in `process-flow.yaml`. Rejected: `process-flow.yaml` owns orchestration topology; model tuning deliberately lives in agent frontmatter (the rich model-choice comments are there for a reason). A model override in process-flow creates a two-file SSoT join for "what model does this agent use" — frontmatter says opus, process-flow silently says sonnet — exactly the drift source to avoid.

## Out of scope

- Any change to the three agents already downgraded in `488adc5` (adapters + dsl-implementer) — validated separately.
- Generalizing the routing into a declarative mechanism — deferred until a second param-conditional agent earns it.
- The orthogonal wall-clock lever (parallelizing api/ui channel pairs) — separate concern, separate plan.
- Diagram/flow regeneration — none; this change touches neither `process-flow.yaml` topology nor any rendered node.

## Estimated effort

~1–2 hours: one pure function + call-site swap + frontmatter comment + table test. Low risk — additive, defaults to today's behaviour for every path except the one documented, fully unit-testable without a live dispatch.
