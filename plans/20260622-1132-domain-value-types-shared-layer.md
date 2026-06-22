# 2026-06-22 11:32:57 CEST — `domain-value-types`: a shared, universally-writable domain-vocabulary layer

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

Start with **Step 1 (shop repo) — relocate domain value types**. In the shop repo's testkit, the domain value type trapped in the unwritable `driver-port` is `driver/port/dtos/OrderStatus` (audit for any other **domain** enums/value objects; the only enums present are `OrderStatus` [domain → move], `dsl/port/ChannelMode` + `dsl/port/ExternalSystemMode` [harness → **stay**], and a private enum in `driver/adapter/ui/MyShopUiDriver` [implementation → **stay**]). Create a new `testkit/.../domainvaluetypes` directory as a sibling of the existing `common` directory, move `OrderStatus` (+ any other domain value types) there, update package/namespace + every import/usage, and confirm the testkit compiles. Structural move across the java / dotnet / typescript scaffolds — do it per language, deriving each `domainvaluetypes` location from that language's existing `common` location. Stop at a review gate before committing. It unblocks Steps 2–3 (the config key + scope wiring that make `domain-value-types` a recognized layer).

## Steps

> **Repo split:** Steps 1–2 land in the **shop repo** (testkit code + `gh-optivem*.yaml` configs live there — gh-optivem has no copy). Steps 3–4 land in **gh-optivem** (process-flow scopes, docs, test fixtures). Step 5 is the cross-repo review+commit gate.

- [ ] **Step 1 — (shop) Relocate domain value types.** Per language scaffold (java, dotnet, typescript): identify the system's **domain** value types and move only those. Classify each type explicitly:
  - **Move** (domain vocabulary): `driver/port/dtos/OrderStatus`, plus any other domain enums/value objects the audit finds.
  - **Stay** (harness/infra config): `dsl/port/ChannelMode`, `dsl/port/ExternalSystemMode` — a different kind, already in writable `dsl-port`.
  - **Stay** (implementation detail): private enums inside adapters, e.g. the nested enum in `driver/adapter/ui/MyShopUiDriver`.
  - **Stay** (call contract): Request/Response DTOs in `driver/port/dtos`.

  Move the domain value types to a new `testkit/.../domainvaluetypes` dir (sibling of `common`), update package/namespace + all imports, confirm compile.
- [ ] **Step 2 — (shop) Add the `domain-value-types` config key.** In every `gh-optivem*.yaml` (monolith/multitier × java/dotnet/typescript × legacy — ~12 files), add one `system-test.paths:` entry. **Deterministic rule:** `domain-value-types` = that file's existing `common:` path with the trailing `common` segment replaced by `domainvaluetypes`. (E.g. multitier-java: `system-test/java/src/main/java/com/mycompany/myshop/testkit/domainvaluetypes`.)
- [ ] **Step 3 — (gh-optivem) Wire `domain-value-types` into `process-flow.yaml` scopes.** Add `domain-value-types` alongside `common` in both `read:` and `write:` of every phase that lists `common`: `write-acceptance-tests` (2043-2044), `write-contract-tests`, `write-stub-fidelity-tests`, `implement-dsl`, the system/external driver-adapter implementers, the fix passes, and `refactor-tests`. **Resolve the one edge:** `implement-system` reads `system-driver-port` (where `OrderStatus` lived) but has no `common` — verify whether the production implementer must open a domain value type that moved out of `system-driver-port`; if so, add `domain-value-types` to its `read:` only (it never writes test-kit types), otherwise leave it out. No `*-changed` flag, no gateway.
- [ ] **Step 4 — (gh-optivem) Docs + test fixtures.** (a) Define `domain-value-types` in the live testkit-architecture docs (`docs/atdd/...` — verify exact file vs the archived `archive/references/code/testkit-architecture-rules.md`) and add a `domainvaluetypes` row to the `language-equivalents` tables (java/csharp/typescript): *the system's domain value types (enums + value objects) — universally writable like `common`; **not** harness enums (those live in `dsl-port`), **not** the C# CLR value type / JVM value class*. (b) Update any gh-optivem test that pins exact scope lists or resolves layers against a fixture config — `internal/atdd/phase_scopes_test.go`, `internal/atdd/process/transitions_test.go`, and any preflight/clauderun fixture that must now know the `domain-value-types` key.
- [ ] **Step 5 — Review gate, then commit (per repo).** Surface diffs for review before committing — structural content change, gate per [[feedback_renames_autonomous_content_gated]] / [[feedback_no_commit_without_approval]]. Commit the shop-repo changes (Steps 1–2) and the gh-optivem changes (Steps 3–4) as **separate, per-repo commits** ([[feedback_never_create_patches]], [[reference_commit_sh_path_stale]] — raw git for surgical commits).

## Verification (operator, not agent steps)

- Re-run the #70 rehearsal (`return-a-delivered-order`). Confirm `write-acceptance-tests` completes **RED without** `STOP_SCOPE_VIOLATION`: the AT-writer adds `RETURNED` to `domainvaluetypes/OrderStatus` (now in scope), writes `hasStatus(OrderStatus.RETURNED)` against the existing typed assertion, and the cascade proceeds into `IMPLEMENT_AND_VERIFY_DSL`.
- Re-run the full rehearsal loop (#72/#71/#69/#76/#61/#65/#68) to confirm the scope-layer addition didn't regress any existing phase.

## Resolved decisions

- **Name = `domain-value-types`** (operator's call). A **domain-scoped** name (not the generic `value-types`/`value-objects`) so the boundary is self-enforcing: only the system's business vocabulary belongs there. Rejected `value-types` (too generic — `ChannelMode` is *also* a value type, inviting harness enums to drift in), `value-objects` (DDD domain-model term; risks reading as the system's domain layer), `primitives` (collides with the BPMN "LOW primitive" term).
- **Boundary = domain only.** Domain value types move (`OrderStatus`); harness/infra enums (`ChannelMode`, `ExternalSystemMode`) stay in `dsl-port` — different kind (how-the-test-runs vs what-the-system-models), different change driver, and already in a writable layer. Adapter-internal enums and Request/Response DTOs stay too.
- **Scope rule = "travels with `common`"** — read+write wherever `common` appears; the lone exception (`implement-system`, read-only) is resolved in Step 3.
- **No routing** — domain value types are shared vocabulary; no `domain-value-types-changed` flag/gateway, no Go logic change.
- **Supersedes** the agent-prompt approach (drafted then dropped: a "compile-safe string pattern" would have institutionalized a `hasStatus(String)` divergence from every existing typed-enum test, degrading the keystone artifact, and would have left the same wall for the contract-test-writer).

## Open questions

- **`error/` types:** are the DTOs under `dtos/error/` domain value types (relocate) or response-shaped DTOs (stay)? Resolve during the Step 1 audit per-type.
- **Plan home:** the plan lives in gh-optivem but most edits are shop-repo. Acceptable (the scope mechanism is gh-optivem's), but flag if the shop repo should carry a cross-reference copy.
