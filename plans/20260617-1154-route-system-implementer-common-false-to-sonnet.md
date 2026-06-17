# Route system-implementer `common:false` dispatches to sonnet

## TL;DR

**Why:** `system-implementer` runs on opus for every channel, but later-channel (`common:false`) dispatches do only a shallow adapter delta hard-gated by `acceptance-${channel}` — the same sonnet-grade work already downgraded for the adapter agents. Paying the opus tax there banks ~$0.67/run per extra channel.
**End result:** A small `resolveDispatchModel` policy function routes `system-implementer` + `common:false` dispatches to sonnet at `driver.go:1336`, leaving the migration-bearing first channel (`common:true`) on opus; frontmatter is annotated and the rule is unit-tested.

## ▶ Next executable step (resume here)

**All agent items are implemented and committed** (the `model-later-channel` scalar + `resolveDispatchModel` + frontmatter + table test; build green, driver/agents packages green). What remains is **user-driven** — no agent edits left. Run the rehearsal corpus (see *Verification* below) across several tickets and let the green/red + delta correctness decide whether the sonnet downgrade stays or is reverted (delete the `model-later-channel:` line). If you keep it, this plan file can be deleted.

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

## Mechanism decision (resolved — revised during execution)

**Chosen: an optional frontmatter scalar `model-later-channel:`, read by an agent-agnostic policy function `resolveDispatchModel` in code.** The original plan chose a hardcoded code function (no config field); during execution the author reversed this in favour of putting the *tunable value* in config. Both the original code-function design and the heavier conditional-map alternative are recorded under *Alternatives*.

The deciding reasons for the scalar:

1. **Logic in code, tunable value in config — the right split.** The *condition* ("`common:false` ⇒ later, delta-only channel") is genuine program logic and stays in `resolveDispatchModel`. The *model value* (`sonnet`) is a tuning knob and belongs in frontmatter next to `model:`/`effort:`. The hardcoded code function buried that knob in Go — backwards.
2. **Transparency + no-rebuild configurability.** The whole model story for the agent lives in its frontmatter (`model: opus` default, `model-later-channel: sonnet`). Changing, retuning, or disabling the downgrade is a one-line YAML edit — no Go change, no rebuild of routing logic.
3. **Single source of truth.** "What model does this agent run?" is answered in one file (frontmatter), not split frontmatter-says-opus / code-says-sonnet.
4. **Reversible experiment, not a prediction.** The field defaults the whole fleet to opus-everywhere when absent; setting it to `sonnet` makes the downgrade an experiment the rehearsal corpus validates per the *accuracy over speed* gate. If sonnet regresses, delete one line → back to opus everywhere.
5. **No DSL.** One optional scalar, branched on in code (it earns its slot), not a `when:`-condition match-list grammar serving one agent.
6. **Pure and testable.** `resolveDispatchModel(tuning, nodeParams)` is a pure function with a table test.

## Verification

(User-driven — not agent steps.)

- Rebuild the `gh optivem` binary if `internal/atdd/assets/` is embedded, then run the rehearsal corpus across **several** tickets (not just #72) and confirm: the `implement-system` later-channel dispatch reports `sonnet` in the agent summary's model column, the first channel still reports `opus`, and all suites stay green.
- Spot-check one multi-channel ticket's later-channel commit to confirm the sonnet delta is correct, per the "accuracy over speed" gate — a passing channel suite plus a correct-looking adapter delta.
- **If the rehearsal disappoints** (a later-channel suite regresses, or the sonnet delta is wrong): delete the `model-later-channel: sonnet` line in `system-implementer.md` and rebuild — the whole fleet is back to opus-everywhere with no code change.

## Alternatives considered

### Hardcoded code function, no config field (original plan — reversed during execution)

A `resolveDispatchModel(agentName, frontmatterModel, nodeParams)` that hardcoded `agentName == "system-implementer" && common == "false" → "sonnet"` in Go. Rejected on execution: it buries the one tunable knob (the target model `sonnet`) in code, splits the "what model does this agent run" answer across frontmatter + driver.go, and forces a Go edit + rebuild to retune or disable. The scalar keeps the same agent-agnostic evaluator but moves the value to frontmatter — see *Mechanism decision*.

### Conditional-model frontmatter *map* (rejected)

A grammar like `model-overrides: [{when: {common: "false"}, model: sonnet}]`. Rejected: it is a mini condition→model DSL (match-list, condition keys, evaluator) serving exactly one agent today — schema speculating about multi-param conditional routing that doesn't exist yet. The single scalar covers the N=1 reality; revisit the map under the rule of three if a second agent needs multi-condition routing.

### Process-flow param override (rejected hardest)

Express a per-node model override in `process-flow.yaml`. Rejected: `process-flow.yaml` owns orchestration topology; model tuning deliberately lives in agent frontmatter (the rich model-choice comments are there for a reason). A model override in process-flow creates a two-file SSoT join for "what model does this agent use" — frontmatter says opus, process-flow silently says sonnet — exactly the drift source to avoid.

## Out of scope

- Any change to the three agents already downgraded in `488adc5` (adapters + dsl-implementer) — validated separately.
- Generalizing the routing into a declarative mechanism — deferred until a second param-conditional agent earns it.
- The orthogonal wall-clock lever (parallelizing api/ui channel pairs) — separate concern, separate plan.
- Diagram/flow regeneration — none; this change touches neither `process-flow.yaml` topology nor any rendered node.

## Estimated effort

~1–2 hours: one pure function + call-site swap + frontmatter comment + table test. Low risk — additive, defaults to today's behaviour for every path except the one documented, fully unit-testable without a live dispatch.
