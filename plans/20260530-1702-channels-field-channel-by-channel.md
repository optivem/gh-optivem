# Project-declared `channels:` + channel-by-channel system implementation

**Status:** proposed
**Created:** 2026-05-30 17:02 CEDT

## Problem

`system-implementer` implements **every channel at once** (for `shop`: API +
UI) in a single dispatch. Observed in rehearsal-71: one long dispatch with a
wide blast radius, a single large diff, no per-channel commit checkpoint, and
the full cross-channel context re-paid on any re-run.

Channels are **project-dependent** — `shop` is API+UI, but another scaffolded
project could be API-only, or API+CLI+gRPC. Yet today the channel set has **no
single source of truth**: it is triplicated as hardcoded constants across the
three language testkits, e.g. `shop/system-test/dotnet/Channel/ChannelType.cs`:

```csharp
public static class ChannelType
{
    public const string UI = "UI";
    public const string API = "API";
}
```

with Java/TS equivalents, and the acceptance tests `@Channel`-parameterized over
them. There is nowhere to declare "this project has these channels," so there is
nothing the process can iterate to implement one channel at a time.

## Goal

1. Make the channel set a **project-declared list** in `gh-optivem.yaml`, as the
   single source of truth the scaffold and the runtime both read.
2. Use it to drive **channel-by-channel** system implementation: green +
   commit one channel, then the next — bounding each dispatch's blast radius and
   giving a per-channel checkpoint.

## What already exists (reuse, do not reinvent)

- **Channel constants** per language (`ChannelType.{cs,java,ts}`) + AT
  `@Channel` parameterization — the channel concept already exists, just
  un-centralized.
- **`implement-and-verify-system`** HIGH process
  (`internal/atdd/runtime/statemachine/process-flow.yaml` ~L1024): linear
  `RUN_ACTION → BUILD_SYSTEM → START_SYSTEM → VERIFY_TESTS_PASS → COMMIT_SYSTEM`.
  `VERIFY_TESTS_PASS` is **already param-scoped** (`suite` / `test-names`), so a
  per-channel scope param has an established shape to follow.
- **`projectconfig.Validate` Rule 22a** — the established "hard-error on
  non-canonical key, do not back-fill" pattern (see `CLAUDE.md` and
  `internal/projectconfig/path-keys.md`). The `channels:` validation rule
  mirrors it.
- **Scaffold-authoritative `init`** — `projectconfig.DefaultPaths` →
  `internal/steps/optivem_yaml.go::BuildOptivemYAML` is the **only** place the
  binary writes a config block, and it owns the just-created tree. `channels:`
  is written here the same way (authoritative initial value matching the
  scaffolded testkit), operator-owned afterwards.
- **Scaffold templates** under `internal/templates/` — where `ChannelType.*`
  generation lives / will live.
- **`${action}` templating** on `RUN_ACTION` (`process: ${action}`) — proof the
  engine already resolves process selection at dispatch time; the per-channel
  param follows the same dispatch-time-resolution model.

## Decisions resolved (so execution doesn't stall)

- **D1 — `channels:` is a list of lowercase canonical tokens**, e.g.
  `channels: [api, ui]`. **Lowercase**, because the tokens are identity for the
  runtime selectors derived from them — `acceptance-${channel}` →
  `acceptance-api` / `acceptance-ui` (and any sibling suite/path slugs). Only the
  `ChannelType` constant generation needs a transform (idiomatic uppercase),
  done deterministically at the scaffold layer. List **order = implementation
  order** (api first → green the cheap/headless channel before UI).
- **D2 — case-sensitive, single canon, hard-validated.** No case-insensitive
  matching / fold layer (it would have to be repeated in every consumer —
  codegen, AT param, verify filter — and a missed fold is a drift bug). Instead a
  single `projectconfig.Validate` rule (mirroring Rule 22a) rejects non-canonical
  casing with a did-you-mean hint: `"channels: tokens must be lowercase; got
  'API', did you mean 'api'?"`. Interactive/flag validation parity required (no
  duplicate validator copies).
- **D3 — `channels:` is the SSoT; scaffold generates `ChannelType.*` from it.**
  Collapses the three hand-maintained constant copies into one declared list.
  Backfill `optivem/shop`'s `gh-optivem.yaml` with `channels: [api, ui]`.
- **D4 — iteration = static unroll in the CALLER, NO loopback edge.** Because
  `channels:` is a small static list known at process-construction time,
  synthesize a sequential per-channel chain rather than a runtime loop. The
  unroll lives **one level up, in the caller** (`change-system-behavior`), which
  invokes the *unchanged* static `implement-and-verify-system` chain **once per
  channel** — rather than synthesizing N×5 nodes inside that inner process. This
  is the **token-optimal** shape (see D7): the inner chain and its
  `VERIFY`/`COMMIT` nodes + params are reused verbatim, so the only synthesized
  surface is N single call-activity nodes — minimal fixture churn. It is also
  deliberate on safety: new loopback edges in `process-flow.yaml` have previously
  deadlocked the statemachine tests and consumed 20GB+ RAM — a static unroll keeps
  the process a terminating DAG.
- **D5 — common layer done once; later channels are deltas.** The
  channel-agnostic **common** layer (DTO / entity / service / migration) is
  implemented in the **first** channel's dispatch; subsequent channel dispatches
  add only that channel's adapter/template wiring. Avoids re-paying the common
  layer's context and re-running the migration. The distinction is carried by a
  **`common` boolean param** the caller binds per channel — `common: true` on the
  first channel (build the common layer + this channel's adapter), `common: false`
  after (adapter delta only). **Named `common`, not `core`**, to avoid conflation
  with the **DSL core** (`IMPLEMENT_AND_VERIFY_DSL` / `dsl-port-changed`).
- **D6 — cumulative verify.** Channel dispatch *K* verifies channels `0..K` all
  pass (not just channel *K*), so greening a later channel cannot silently regress
  an earlier one.
- **D7 — engine-loop in the caller (A2), chosen as the token-optimal model.**
  The channel loop lives in the **process engine**, dispatching `system-implementer`
  **once per channel** — *not* a single launch where the agent loops internally.
  Rejected alternatives and why:
  - **Single launch, agent loops internally (B):** cheapest to build (no engine
    change) but fails every goal — the per-channel commit checkpoint is lost (the
    lone `COMMIT_SYSTEM` fires once at the end), the blast radius is the same wide
    dispatch, and a re-run re-pays the *entire* cross-channel context. It
    relocates rehearsal-71's problem rather than solving it.
  - **Unroll N×5 nodes inside `implement-and-verify-system` (A1):** meets the
    goals but duplicates the `verify`/`commit` nodes per channel, bakes the
    first-channel `common` asymmetry into the inner chain's *shape*, and balloons
    the `transitions_test.go` / `phase_scopes_test.go` fixture surface.
  - **Unroll in the caller, reuse the static inner chain (A2 — chosen):**
    token-optimal on **both** axes. *Runtime:* each dispatch holds only its
    channel (+ the common layer on the first), retries are incremental, the common
    layer is built once (D5). *Maintenance:* the inner 5-node chain and its
    `suite`/`test-names`/`commit` params are reused verbatim; only N single
    call-activity nodes are synthesized, so fixture churn is minimal. `common` and
    `channel` are passed as caller-bound params through the existing strict-mode
    `ExpandParams` path (process-flow.yaml ~L1054).

## Open knob (the one thing to confirm during execution)

- **`ChannelType` constant *value* casing.** Today the value is `"API"`/`"UI"`
  (uppercase) and the AT `@Channel` parameterization compares against it. For a
  clean end-to-end single token, regenerate the value as the canonical
  `"api"`/`"ui"` too — but this **changes the channel string the existing tests
  carry**. Acceptable in a teaching repo (no legacy-alias machinery; configs are
  regenerated), but it is a real behaviour change. Confirm before flipping the
  value; the constant *name* stays idiomatic uppercase (`API`) regardless.

## Items

1. **Add `channels:` to the config schema + validation.** In
   `internal/projectconfig/`: parse the `channels:` list, add the Rule-22a-style
   `Validate` rule (lowercase-canon, hard-error + did-you-mean), with
   interactive/flag validation parity.
2. **Write `channels:` at `init`** via `BuildOptivemYAML` (scaffold-authoritative,
   alongside `DefaultPaths`) — the authoritative initial value matching the
   scaffolded testkit. Do **not** add a migrate-time or validate-time back-fill.
3. **Generate `ChannelType.{cs,java,ts}` from `channels:`** in the scaffold
   templates (`internal/templates/`) — the SSoT codegen that retires the three
   hand-maintained copies. Resolve the D-open-knob constant-value casing here.
4. **Static-unroll the channel loop in the caller (A2 — per D7)**
   (`process-flow.yaml` + the engine/loader): in `change-system-behavior`, read
   `channels:` at process-construction time and synthesize **one call-activity
   node per channel** invoking the *unchanged* static `implement-and-verify-system`
   chain, with **no loopback edge**. Bind per channel: `channel` (the channel
   token), `common` (true on the first channel only, per D5), and the cumulative
   verify scope (channels `0..K`, per D6) — all through the existing strict-mode
   `ExpandParams` path, following the established `suite`/`test-names` param shape.
   Do **not** duplicate the inner `verify`/`commit` nodes (that is the rejected
   A1 shape).
5. **Make `system-implementer.md` channel-aware.** The dispatch is told which
   channel to green (`channel` param) and reads the `common` param for the
   layer-vs-delta guidance: when `common: true`, implement the channel-agnostic
   common layer (DTO / entity / service / migration) **and** this channel's
   adapter; when `common: false`, implement only this channel's adapter delta.
   Describe the layer as "the channel-agnostic **common** layer" — never "core"
   (DSL-core collision).
6. **Tests + fixture audit.** Update `statemachine/transitions_test.go` and
   `phase_scopes_test.go` — under A2 the new node surface is confined to the N
   per-channel call-activity nodes in the caller (the inner chain is unchanged),
   so fixture churn is minimal. **Audit the gate fixtures before running the
   statemachine tests** and watch RAM — even though the static unroll has no
   loopback by design, the statemachine loop hazard warrants the check.

## Cross-repo note

- Item 3's backfill (`channels: [api, ui]`) lands in the **`optivem/shop`**
  repo's `gh-optivem.yaml`, not in `gh-optivem`. Treat as a separate, gated
  commit in that repo.

## Do NOT

- **Do not add a diagram-regeneration step.** Editing `process-flow.yaml`
  triggers the regenerate-diagram GH Actions workflow on push to main, which
  auto-regenerates `docs/process-diagram.md` + `docs/images/*.svg`. A local regen
  step races it and produces merge conflicts.
- **Do not introduce a runtime loopback** over channels (see D4).

## Sibling plan

- `plans/20260530-1701-headless-no-ask-clause.md` — the other fix from the same
  rehearsal-71 investigation (stopping headless agents from burning turns on
  un-answerable `AskUserQuestion` calls).

## Verification (operator)

- Re-run rehearsal-71: confirm the run produces **one commit per channel** (an
  api-green commit, then a ui-green commit), each channel verified incrementally,
  and that a single project with `channels: [api]` only runs the API slice with
  no UI work attempted.
