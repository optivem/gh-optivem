# Plan: `real-kind` per external system + simulator implementation in the CT pathway

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-06-06T12:19:15Z`

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
IDENTIFY_EXTERNAL_SYSTEM            (resolve which external system; unknown → error → onboarding)
  → IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS        (Real client — always)
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

## Edits

### 1. Config schema — `internal/projectconfig/config.go` (option 1B)
- Replace `ExternalSystems{Stubs, Simulators}` (`:336`) with a name-keyed map:
  ```go
  type ExternalSystems map[string]ExternalSystem

  type ExternalSystem struct {
      RealKind  RealKind     `yaml:"real-kind"`           // test-instance | simulator
      Stub      ExternalSpec `yaml:"stub"`                // contract-stub backing (always present)
      Simulator ExternalSpec `yaml:"simulator,omitempty"` // present iff RealKind == simulator
  }

  type RealKind string
  const (
      RealKindTestInstance RealKind = "test-instance"
      RealKindSimulator    RealKind = "simulator"
  )
  ```
- `Repos()` (`:380`) — replace the two `add(c.ExternalSystems.Stubs/Simulators.Repo)` calls
  with iteration over the map, adding each entry's `Stub.Repo` and (if present) `Simulator.Repo`.
- Validation (`:531`, `:582`): per entry — `real-kind` required and ∈ enum; `stub` always
  required (full `ExternalSpec`); `simulator` **present iff** `real-kind: simulator`
  (absent-iff-test-instance is the SSoT — no cross-field reconciliation beyond this iff).

### 2. Config docs — `internal/projectconfig/path-keys.md` (+ `config.go` doc comments)
- Document the per-system `external-systems.<name>` shape, `real-kind` + enum, and the
  "simulator present iff simulator" rule.

### 3. Process flow — `internal/atdd/runtime/statemachine/process-flow.yaml`
- In CT-HIGH (`:946`): add `IDENTIFY_EXTERNAL_SYSTEM`, the `real-kind` gateway, the
  `VERIFY_TESTS_FAIL_CONTRACT_REAL` + `IMPLEMENT_EXTERNAL_SYSTEM_REAL_SIMULATOR` +
  build/start + `VERIFY_TESTS_PASS_CONTRACT_REAL` simulator branch, and the test-instance
  green-only branch. Wire sequence-flows.
- New MID `implement-external-system-real-simulator` (mirror of
  `implement-external-system-stubs`, `:1014`): `read`/`write` scoped to the identified
  system's `external-systems.<name>.simulator.path` (+ whatever the stub MID reads).

### 4. Gate binding — `internal/atdd/runtime/gates/bindings.go` (+ test)
- `real-kind` gateway binding: promote the identified system's `real-kind` from config into
  `ctx.State` for the predicate evaluator (Q6 port-change wiring precedent).
- `IDENTIFY_EXTERNAL_SYSTEM`: resolve the system name deterministically from changed
  external-driver file paths (`external/.../<name>/`), validate against the `external-systems`
  registry; unrecognized → error routing to onboarding.

### 5. New agent — `internal/assets/runtime/agents/atdd/external-system-real-simulator-implementer.md`
- Mirror `external-system-stub-implementer.md`: implement the simulator
  (`external-systems.<name>.simulator.path`) so contract-real passes. Real-server fidelity:
  same shapes / status codes / error semantics as the published contract (the stub
  implementer's "reflect the real Test Instance's contract" line is the test-instance analogue).

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

Residual to confirm at execution (not blocking design):
- **Existing-config migration.** The shop template's flat `stubs`/`simulators` tiers must be
  rewritten to the per-system map; confirm whether any `migrate`/validation back-fill is
  expected or operators hand-edit (mirrors the Rule-22a "operator adds the lines" posture).

## Tests to update / add
- `internal/projectconfig/config_test.go` — `real-kind` parse, enum validation, required-per-
  system, `simulator` present-iff-simulator, `stub` always required.
- `internal/steps/optivem_yaml_test.go` — emitted YAML carries `real-kind`.
- `internal/atdd/runtime/gates/bindings_test.go` — `real-kind` gateway promotes the right
  value; unrecognized system errors.
- `internal/atdd/runtime/statemachine/transitions_test.go` — CT-HIGH new nodes + both gate
  branches; simulator branch red→green ordering.
- `internal/atdd/runtime/clauderun/clauderun_test.go` — dispatch of
  `implement-external-system-real-simulator`.
- Shop template config — add `real-kind` to the checked-in `gh-optivem-<arch>-<lang>.yaml`
  so parity/validation stays green; coordinate with shop CI.

## Verification
- `go build ./...`
- `go test ./internal/projectconfig/... ./internal/atdd/... ./internal/steps/...`
- `gh optivem process scope implement-external-system-real-simulator` shows the simulator
  path in the resolved write set.
- Diagram regenerates on push to main (do **not** regen locally — the workflow owns it).

## Cross-references
- Bug site: `internal/atdd/runtime/statemachine/process-flow.yaml:946` (CT-HIGH).
- Existing config tiers: `internal/projectconfig/config.go:336` (`ExternalSystems`).
- Mirror agent: `internal/assets/runtime/agents/atdd/external-system-stub-implementer.md`.
- Forward dependency: `plans/backlog/20260526-1746-rebuild-onboard-external-system.md`
  (onboarding declares `real-kind`; receives the unrecognized-system error branch).
- Design rationale: `docs/bpmn-process-design.md`.
