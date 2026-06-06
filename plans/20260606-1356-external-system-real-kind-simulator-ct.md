# Plan: `real-kind` per external system + simulator implementation in the CT pathway

## TL;DR

**Why:** The CT-HIGH real-side flow silently hard-codes `real-kind == test-instance`: it implements the Real client then expects contract-real GREEN, so the moment the real side is a simulator-we-author (no live test instance), `VERIFY_TESTS_PASS_CONTRACT_REAL` fails because nothing teaches the simulator the new contract.
**End result:** `external-systems` becomes a per-system map carrying an explicit `real-kind` (`test-instance` | `simulator`); the CT-HIGH real side identifies the system, gates on `real-kind`, and runs the missing simulator red→green branch — making the real and stub sides structurally symmetric.

## Why

The CT-HIGH `implement-and-verify-external-system-driver-adapters-contract-tests`
(`internal/atdd/runtime/statemachine/process-flow.yaml:946`) is **asymmetric** between
its real side and its stub side:

- **Stub side** (lines 1005–1038) runs a full red→green: `verify-tests-fail` (contract-stub)
  → `implement-external-system-stubs` (teach the **stub server** the new contract) →
  `verify-tests-pass` (contract-stub).
- **Real side** (lines 972–996) implements only the **Real driver (client)**
  (`implement-external-system-driver-adapters`), then jumps straight to
  `verify-tests-pass` (contract-real). **No step teaches the real *server* the new contract.**

That works **only when the real side is a live third-party test instance** — the vendor
already implements the contract, so fixing our client is enough for contract-real to go
green. The moment there is **no test instance** and the real side is a **simulator we
author**, `VERIFY_TESTS_PASS_CONTRACT_REAL` (line 989) fails: nobody taught the simulator
the new contract.

Root cause, stated precisely: **the current flow silently hard-codes
`real-kind == test-instance`.** This plan makes the kind explicit per external system and
adds the missing simulator-implementation branch.

## The model (locked in discussion 2026-06-06)

Per external system, the thing backing the **contract-real** suite is one of two kinds:

- **`test-instance`** — a live third-party sandbox that already honors the contract. After
  the Real **driver (client)** is implemented, contract-real is **expected GREEN**. Nothing
  to implement on the real server; we don't touch it.
- **`simulator`** — a stand-in **we author** (declared per system under
  `external-systems.<name>.simulator`). After the client is implemented, contract-real
  is **expected RED**, then we implement the simulator and re-verify GREEN — exactly the
  red→green the stub side already does.

Consequence: with `real-kind: simulator`, the real and stub branches are **structurally
symmetric** (red → implement → green). With `real-kind: test-instance`, the real branch
collapses to a single green check. The symmetry is the smell-test that the model is right.

## Decisions (locked)

- **Register it explicitly; do not infer.** Whether a test instance exists is an
  operational fact, not derivable from code. It is the gate signal for the new branch.
- **Per-system restructure (option 1B, locked).** `external-systems` becomes a map keyed by
  external-system name, each entry holding `real-kind` + its own `stub` and (when a simulator)
  `simulator` spec. This replaces today's two flat tiers (`external-systems.stubs`,
  `external-systems.simulators`, `config.go:336`). Chosen as the **cleanest end-state**: one
  entry = one system, every path explicit and operator-owned (no implicit `<name>/` subdir
  convention), and no SSoT join between a registry and shared tier locations. The cost is a
  one-time migration of `Repos()` / `Validate` / path resolution / shop template. Rejected:
  **1A** additive `systems:` map (keeps an implicit subdir-convention join between the map and
  the tier roots); **1C** project-global `real-kind` (breaks the moment two externals differ).
- **Field name `real-kind`** (YAML) / `RealKind` (Go); value enum **`test-instance` |
  `simulator`**. Rejected: `Mode` (names no subject), `ExternalSystemRealType` (redundant
  `ExternalSystem` prefix inside a per-system entry; `Type` is a weak discriminator suffix).
  "kind" is the idiomatic closed-enum discriminator and reads cleanly in scaffolded YAML
  students see.
- **Fidelity-ladder reading (Q2, locked).** `stub → simulator → test-instance` is one axis =
  what backs **contract-real**; `contract-stub` is always the stub. The `simulator` block is
  present **iff** `real-kind: simulator`; with `test-instance` it is **absent**. Absence *is*
  "test instance," structurally — so `real-kind` and the `simulator` block can never disagree
  (single SSoT, no cross-field reconciliation rule needed). The simulator writes into its own
  per-system `external-systems.<name>.simulator.path` — no new path key.
- **`IDENTIFY_EXTERNAL_SYSTEM` is deterministic (Q3, locked).** Resolve the system name from
  the changed external-driver file paths (`external/.../<name>/`), validated against the
  `external-systems` registry. No agent. Unrecognized name → **hard error, not a default** —
  identity must resolve before the `real-kind` gate. The error points at onboarding (the
  registration flow), which is where `real-kind` gets declared.

## Flow change (CT-HIGH real side)

Replace the straight-line real side at `process-flow.yaml:972–996` with an identify step,
a `real-kind` gate, and the simulator red→green branch:

```
IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS        (Real client — always)
  → IDENTIFY_EXTERNAL_SYSTEM        (resolve system from the adapter files just written; unknown → error → onboarding)
  → BUILD / START
  → GATE real-kind
       ├─ test-instance → VERIFY_TESTS_PASS_CONTRACT_REAL          (expect GREEN — done)
       └─ simulator     → VERIFY_TESTS_FAIL_CONTRACT_REAL          (expect RED)
                          → IMPLEMENT_EXTERNAL_SYSTEM_REAL_SIMULATOR   (NEW)
                          → BUILD / START
                          → VERIFY_TESTS_PASS_CONTRACT_REAL          (expect GREEN)
  → (stub side unchanged, lines 1005–1038)
```

The simulator branch is step-for-step the existing stub branch, so it reuses the existing
`verify-tests-fail` / `verify-tests-pass` MID processes — no new primitives.

**IDENTIFY ordering (resolved 2026-06-06, session 2).** IDENTIFY runs **after**
`IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS`, not before. The only deterministic,
timing-stable source of the `<name>` is `ctx.State["phase-changed-files"]`, which
`validate-outputs-and-scopes` populates **only for an agent with a write scope**. The
driver-adapter impl (`write: [external-system-driver-adapter]`) always runs on the real
side and writes under `.../adapter/external/<name>/…`, so reading `phase-changed-files`
immediately after it is reliable. Running IDENTIFY *first* (as this pseudocode originally
drew it) would read a **stale** `phase-changed-files` from whatever agent ran last — e.g.
the contract-test author on the `dsl-port-changed == false` branch, which writes `ct-test`,
not external-driver files. Identity is not needed until the downstream `real-kind` gate, so
the later placement costs nothing.

## Status

**Session 1 (2026-06-06) — DONE: Go-side schema foundation (Edits #1 + #2).**
The schema is migrated to the per-system map.

**Session 2 (2026-06-06) — DONE: process-flow + gateway + action + new agent (Edits #3–#5)
and all in-repo tests.** CT-HIGH now runs IDENTIFY → `real-kind` gate → simulator red→green
/ test-instance green-only; `identify-external-system` action + `real-kind` gateway wired and
tested; `external-system-real-simulator-implementer` agent added. Full affected suite green.
What remains: the forward onboarding dependency (#6, separate plan) and the cross-repo shop
template config + inert-flag retirement (follow-ups below).

## Edits

### 6. Onboarding dependency (forward) — `plans/backlog/20260526-1746-rebuild-onboard-external-system.md`
- The rebuilt `onboard-external-system` flow is where `real-kind` is **declared** for a new
  system, and where `IDENTIFY_EXTERNAL_SYSTEM`'s unrecognized-system error routes. This plan
  does **not** build onboarding; it records the dependency. The error branch can land as a
  hard stop ("external system not onboarded — register it") until onboarding exists.

## Resolved forks (locked 2026-06-06)

- **Config shape → 1B** (per-system map). See Decisions.
- **`real-kind` vs `simulator` block → fidelity ladder** (simulator present iff
  `real-kind: simulator`; absent ⇒ test-instance). See Decisions.
- **`IDENTIFY_EXTERNAL_SYSTEM` → deterministic** from changed-file paths. See Decisions.
- **Scaffold emission → omit at `init`, operator-owns (fork #1, locked session 1).**
  `gh optivem init` writes **no** `external-systems:` block. The flat `--stubs-path` /
  `--simulators-path` scaffold flags carry no system *name* to key the per-system map on,
  and a teaching repo regenerates its configs — so operators hand-add per-system entries
  (Rule-22a "operator adds the lines" posture). The flat flags + `RawFlags.StubsPath` /
  `SimulatorsPath` are left **inert** this session (not removed — that CLI-surface cleanup
  is a separate follow-up). `buildExternals` and `externalsRepoSlug` were deleted;
  `FillRawFlagsFromYAML` no longer reads external-systems.
- **New simulator MID scope → mirror the stub MID's `external-system-driver-adapter`
  (fork #2, locked session 1).** See Edit #3 / #5.

Residual / follow-ups — **spun out into their own plans (2026-06-06):**
- **Shop template config (cross-repo).** →
  `plans/20260606-1520-shop-template-external-systems-per-system-map.md`
- **Retire the inert scaffold flags.** →
  `plans/20260606-1521-retire-inert-stub-simulator-scaffold-flags.md`

## Tests (remaining)
Done in session 1: `config_test.go`, `optivem_yaml_test.go`, `config_commands_test.go`,
`yaml_input_test.go`, `preflight_test.go`, `driver_test.go`.
Done in session 2: `actions/bindings_test.go` (IDENTIFY action — name + real-kind stamp,
shared-residual ignore, unknown/ambiguous/zero/nil-config hard errors, RegisterAll),
`gates/bindings_test.go` (`real-kind` gateway enum + unset/wrong-type halts, RegisterAll),
`statemachine/transitions_test.go` (CT-HIGH IDENTIFY + `real-kind` gate + both branches +
simulator red→green ordering + new MID scope/agent), `clauderun_test.go`
(`external-system-real-simulator-implementer` renders + dispatch).

- Shop template config (cross-repo) — spun out:
  `plans/20260606-1520-shop-template-external-systems-per-system-map.md`.

## Verification (session 2 — all green)
- `go build ./...` ✓
- `go test ./internal/projectconfig/... ./internal/atdd/... ./internal/steps/...` ✓
- `gh optivem process scope implement-external-system-real-simulator` → resolves the
  `external-system-driver-adapter` layer (fork #2 write set) ✓
- Diagram regenerates on push to main (do **not** regen locally — the workflow owns it).

## Cross-references
- Bug site: `internal/atdd/runtime/statemachine/process-flow.yaml:946` (CT-HIGH).
- Existing config tiers: `internal/projectconfig/config.go:336` (`ExternalSystems`).
- Mirror agent: `internal/assets/runtime/agents/atdd/external-system-stub-implementer.md`.
- Forward dependency: `plans/backlog/20260526-1746-rebuild-onboard-external-system.md`
  (onboarding declares `real-kind`; receives the unrecognized-system error branch).
- Design rationale: `docs/bpmn-process-design.md`.
