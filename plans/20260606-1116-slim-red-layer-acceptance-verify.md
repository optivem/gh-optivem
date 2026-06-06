# Plan: Slim the RED-layer acceptance verify (re-evaluate the `acceptance` alias / `ExpandSuiteGroups`)

## Relationship to other plans

Follow-up to `plans/20260606-1111-decouple-verify-suite-from-tests-discriminator.md`
("the bugfix plan"). That plan decouples the verify `suite` from the `tests` discriminator and
binds explicit suites at every leaf, **retaining `suite: acceptance` (the union alias) at the
two AT leaves** to stay behaviour-preserving. This plan re-opens that retained decision.

**Execute the bugfix plan first.** Both touch the same AT leaf nodes; sequencing avoids a
collision and lets this plan reason about explicit `suite:` bindings rather than `${tests}`.

## Why

The `acceptance` group alias (`internal/atdd/runtime/testselect/suite.go` →
`[acceptance-api, acceptance-ui]`) makes the DSL-layer and System-Driver-adapter-layer verifies
run **both channels**. Per `channels.go`:

- `UnrollSystemChannels` rewrites the **GREEN system-impl** step to per-channel `acceptance-<ch>`
  — genuinely per-channel, does not use the alias post-unroll.
- `UnrollSystemDriverAdapterChannels` overrides only `channel` and **keeps the inherited suite**,
  so each per-channel adapter node still runs the **union**.
- The **DSL-layer** verify also runs the union.
- The **no-`channels:` fallback** keeps the single static `suite: acceptance`.

Observation driving this plan: an acceptance test's **behaviour is channel-agnostic** (same
business assertion); only the **driver path** differs per channel (API HTTP vs UI Playwright).
So running both channels buys real coverage **only when a green result is expected** (a broken
UI adapter can fail while API passes). When a **red** result is expected, one representative
channel already proves "wired and failing for the right reason" — the second channel is
duplicated system-test cost for no added signal.

## The core question

For each layer that currently binds `suite: acceptance`, is the union justified, or should it be
a single channel?

| Verify site | Expected result on its paths | Union earns coverage? |
|---|---|---|
| GREEN system-impl (`implement-and-verify-system`) | success | already per-channel (`acceptance-<ch>`) — N/A |
| DSL-layer (`implement-and-verify-dsl` → `implement-test-layer`) | **mixed** — `failure` on the RED cascade, `success` on AT-pass paths (e.g. legacy coverage) | only on the `success` paths |
| System-Driver-adapter-layer (`implement-and-verify-system-driver-adapters`) | **mixed**, same as above | only on the `success` paths |

The nuance that blocks a naive "always single channel": these two layers are reached with
`expected-test-result: ${expected-test-result}`, so they are **not always RED**. On an AT-pass
path the union is a genuine per-channel green gate.

## Investigation items (resolve before any mechanical edit)

1. **Enumerate the expected-result on every path** reaching the DSL-layer and adapter-layer
   verifies. Confirm which callers forward `failure` vs `success` (trace `write-and-verify-
   acceptance-tests-fail` vs `-pass`, and the legacy-coverage `cover-system-behavior` path).
2. **Confirm GREEN coverage is not lost.** Determine whether, on the `success` paths, the
   per-channel green is *also* covered by the system-impl step's `acceptance-<ch>` unroll — i.e.
   is the DSL/adapter union green redundant with a later per-channel green, or the only one?
3. **Measure the cost** the union adds at the RED layers (extra system-test suite runs per
   cycle) to confirm the change is worth making.

## Candidate directions (decide after investigation)

- **Option A — condition the suite on expected result.** RED layers (`expected-test-result ==
  failure`) bind a single representative channel; success paths keep the union (or per-channel).
  Preserves all green coverage, removes only the redundant RED second-channel run. *Likely
  recommendation*, pending item 2.
- **Option B — single channel everywhere at these layers**, relying on the system-impl
  `acceptance-<ch>` step for all per-channel green. Simplest; only safe if item 2 proves the
  green is covered elsewhere.
- **Option C — status quo.** If item 2 shows the union green is the *only* per-channel gate on
  the AT-pass paths, the union stays and this plan closes as "no change, documented".

## Alias / `ExpandSuiteGroups` re-evaluation

Fold into the decision: if the surviving union usages drop to just the no-`channels:` fallback,
re-examine whether the `acceptance` alias + `ExpandSuiteGroups` still earns its slot, or whether
explicit per-channel suite ids (and a plain fallback) are simpler. Do **not** pre-decide;
`ExpandSuiteGroups` is also the documented expansion point for future `contract`/`e2e` groups.

## Items (mechanical — gated on the decision above)

> Left as TBD until the investigation lands and a direction is signed off; the mechanical edits
> (which leaf binds which suite, any `channels.go` unroll-suite change, any `suite.go` removal)
> follow directly from the chosen option. This plan is **not** autonomous past the investigation
> — the direction is a maintainer call (behaviour-affecting).

## Verification (once implemented)

- Scoped `go test ./internal/atdd/runtime/statemachine/ ./internal/atdd/runtime/testselect/ -p 2`
  (never unbounded `go test ./...` on Windows).
- A rehearsal slice that exercises both a RED AT cascade and an AT-pass path, confirming the
  intended per-channel vs single-channel suite is emitted at each layer and no green per-channel
  coverage was dropped.
