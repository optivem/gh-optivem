# 2026-06-22 11:32:57 CEST — `domain-value-types`: a shared, universally-writable domain-vocabulary layer

> **Status: code-complete (2026-06-22).** All agent work shipped in two commits — shop `325979f1` (Steps 1–2: OrderStatus relocation + 12 config keys) and gh-optivem `07f594e` (Steps 3–4: process-flow scopes, CanonicalPathKeys/pathStems, fixtures, path-keys.md). Only the operator rehearsal re-runs in `## Verification` remain.

## TL;DR

**Why:** Rehearsal #70 ("Return a delivered order") crashed at `STOP_SCOPE_VIOLATION` because `OrderStatus` — a **domain** value type (business vocabulary the system models and tests assert on) — is lumped under `driver/port/dtos`, a layer owned solely by the `dsl-implementer`. So the `acceptance-test-writer` (and any other agent) cannot add a new status value its test references without a fatal scope-exception. Root cause: a **classification error** — domain vocabulary is gated like implementation.
**End result:** A new `domain-value-types` scope layer — sibling of `common`, universally readable/writable like `common` — holds the system's domain value types (enums + value objects), relocated out of `driver/port/dtos`. Any agent can extend domain vocabulary; acceptance tests stay idiomatically typed (`hasStatus(OrderStatus.RETURNED)`); the #70-class halt is gone **for every agent**.

## Outcomes

What we get out of this — the goals and deliverables:

- A first-class `domain-value-types` Family B layer in every `gh-optivem*.yaml` config, resolved by `ResolveLayerPaths` with **no** special-casing (an ordinary `cfg.SystemTest.Paths` key).
- The system's **domain** value types (today: the `OrderStatus` enum; future: `Money`/`Sku`/`Quantity` VOs) live in `testkit/.../domainvaluetypes`, a sibling of `common`.
- **Harness/infrastructure enums stay out** — `ChannelMode` and `ExternalSystemMode` remain in `dsl-port` (they answer *"how is the test run?"*, not *"what does the system model?"*, and `dsl-port` is already writable). The domain-scoped name makes this boundary self-enforcing: a channel/external-system enum is obviously not domain vocabulary.
- `domain-value-types` sits in the read+write scope of every test-side agent (it **travels with `common`**), so adding or extending a domain value type never triggers a scope-exception or reaches `STOP_SCOPE_VIOLATION`.
- Acceptance/contract tests reference domain values idiomatically and compile + run red **without** invented string overloads (no more `hasStatus(String)`).
- **No new routing**: no `*-changed` flag, no gateway, no Go change — domain value types are shared vocabulary nobody routes on.
- Docs define `domain-value-types` and contrast it with the harness enums and with the C# CLR / JVM "value type" language features.

## ▶ Next executable step (resume here)

**No agent steps remain — operator verification only.** All five steps shipped (shop `325979f1`, gh-optivem `07f594e`). The next action is the operator's: run the rehearsals in `## Verification` below to prove the #70-class halt is gone and nothing regressed. There is nothing left for `/execute-plan` to do.

## Verification (operator, not agent steps)

- Re-run the #70 rehearsal (`return-a-delivered-order`). Confirm `write-acceptance-tests` completes **RED without** `STOP_SCOPE_VIOLATION`: the AT-writer adds `RETURNED` to `domainvaluetypes/OrderStatus` (now in scope), writes `hasStatus(OrderStatus.RETURNED)` against the existing typed assertion, and the cascade proceeds into `IMPLEMENT_AND_VERIFY_DSL`.
- Re-run the full rehearsal loop (#72/#71/#69/#76/#61/#65/#68) to confirm the scope-layer addition didn't regress any existing phase.

## Resolved decisions

- **Name = `domain-value-types`** (operator's call). A **domain-scoped** name (not the generic `value-types`/`value-objects`) so the boundary is self-enforcing: only the system's business vocabulary belongs there. Rejected `value-types` (too generic — `ChannelMode` is *also* a value type, inviting harness enums to drift in), `value-objects` (DDD domain-model term; risks reading as the system's domain layer), `primitives` (collides with the BPMN "LOW primitive" term).
- **Boundary = domain only.** Domain value types move (`OrderStatus`); harness/infra enums (`ChannelMode`, `ExternalSystemMode`) stay in `dsl-port` — different kind (how-the-test-runs vs what-the-system-models), different change driver, and already in a writable layer. Adapter-internal enums and Request/Response DTOs stay too.
- **Scope rule = "travels with `common`"** — read+write wherever `common` appears; the lone exception (`implement-system`, read-only) is resolved in Step 3.
- **No routing** — domain value types are shared vocabulary; no `domain-value-types-changed` flag/gateway, no Go logic change.
- **Supersedes** the agent-prompt approach (drafted then dropped: a "compile-safe string pattern" would have institutionalized a `hasStatus(String)` divergence from every existing typed-enum test, degrading the keystone artifact, and would have left the same wall for the contract-test-writer).
- **`dtos/error/SystemError` stays in `dtos/`.** It's a generic error-response envelope (message + field errors), value-semantic but not the system's business vocabulary and not per-story-extensible like `OrderStatus` — the same domain-scoped boundary that excludes `ChannelMode` excludes it.
- **Single plan in gh-optivem.** No copy/pointer in the shop repo — the scope mechanism is gh-optivem's, the shop-repo work is driven by `/execute-plan` reading this file, and a duplicate would only drift.
