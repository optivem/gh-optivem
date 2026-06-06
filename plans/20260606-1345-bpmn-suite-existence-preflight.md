# Plan: Preflight that the BPMN-required test suites exist in `tests.yaml`

## Why

The BPMN process flow (`internal/atdd/runtime/statemachine/process-flow.yaml`) hard-codes the
test-suite ids it will request via `gh optivem test run --suite=…` call-activities:

- `suite: acceptance` — a **group alias** expanding to `acceptance-<channel>`
  (`internal/atdd/runtime/testselect/suite.go` → `[acceptance-api, acceptance-ui]`).
- `suite: contract-real`
- `suite: contract-stub`

These literals are only checked against the project's `tests.yaml` **when the driver actually
reaches the node** — the runner resolves them in `runner.selectSuites`
(`internal/runner/tests.go:118`), which errors `suite(s) not found: <id>. Available: …`. If a
teacher/student renamed a suite in their `tests.yaml`, the run fails **deep inside the pipeline,
after agents have already done work and committed** — the costly, late failure this plan moves
forward.

The fix: validate suite existence **before any agent dispatches**, in the existing runtime
preflight that already runs on `gh optivem implement`.

## Where this belongs (no new mechanism)

`internal/atdd/runtime/preflight/preflight.go` is the runtime backstop that runs on
`gh optivem implement` (via `runImplementPreflight`, `implement_commands.go:282`) *before* board,
classify, or agent dispatch. It already sweeps the loaded `Engine` for each writing-agent MID's
`read:`/`write:` scope (`runScopeResolutionChecks`) and folds failures into one aggregated error
block. The suite check is a **new check class of the same shape** — engine sweep + cfg-derived
expectation + membership check — gated on `opts.Engine != nil` exactly like the scope sweep.

## Relationship to other plans

- `plans/20260606-1116-slim-red-layer-acceptance-verify.md` and
  `plans/20260606-1111-decouple-verify-suite-from-tests-discriminator.md` may **change which
  literal `suite:` values appear** in `process-flow.yaml` (e.g. replacing the `acceptance` alias
  with explicit `acceptance-api` / `acceptance-ui` per-channel bindings). This plan's check reads
  whatever literals exist at run time, so it stays correct under either outcome — but **execute
  those plans first if they are in flight**, since they touch the same suite-resolution surface.
  The expander (Item 2) must handle both the alias form and explicit per-channel ids.

## Design — derive expected suites deterministically, don't guess

Per `[[feedback_paths_deterministic_no_guessing]]`: the expected suite set is computed from
`cfg` + the engine's literals, never invented.

**Inputs available in `preflight.Run`:** `eng` (process-flow), `cfg` (has `cfg.Channels`,
`cfg.ExternalSystems`, `cfg.SystemTest.Config` = the `tests.yaml` path).

**Resolution rules:**

1. **Collect literals.** Sweep every node's `Raw.Params["suite"]` across `eng.Processes`. Skip
   `""` (the explicit-empty verify-noop sentinel) and any value containing `${` (runtime-resolved
   placeholders like `${suite}`, whose concrete value originates at some literal call site that
   the sweep already sees). Distinct literals today: `acceptance`, `contract-real`,
   `contract-stub`.
2. **Filter conditional literals.** `contract-real` / `contract-stub` are only exercised when the
   project does contract testing — gate them on external systems being configured:
   `!cfg.ExternalSystems.Stubs.IsEmpty() || !cfg.ExternalSystems.Simulators.IsEmpty()` (same
   condition `collectTiers` uses for its external-system tiers). A project with no
   `external-systems:` never takes the `GATE_EXTERNAL_DRIVER_PORTS_CHANGED` branch, so requiring
   the contract suites there would be a false positive — drop them from the expected set.
   *(Executor: confirm the gate predicate against the actual `shared-contract` gateway semantics
   — stub-only vs stub-or-sim — before pinning it.)*
3. **Expand to concrete ids.**
   - `acceptance` → `acceptance-<ch>` for each `ch ∈ cfg.Channels`. This is channel-correct and
     matches `UnrollSystemChannels`; it avoids the false positive of expanding the static group
     (which hard-codes both channels) against an api-only project. **Fallback** when
     `cfg.Channels` is empty (the no-`channels:` static path): expand via
     `testselect.ExpandSuiteGroups(["acceptance"], tests.SuiteGroups)` so behaviour matches the
     runtime's own resolution.
   - Explicit per-channel ids (`acceptance-api`, …) and the contract ids pass through unchanged
     (still run through `ExpandSuiteGroups` in case a project declared them as a group alias).
4. **Membership check.** Load `tests.yaml` (`cfg.SystemTest.Config`) via `runner.LoadTests`; build
   the set from `tests.SuiteIDs()`. For each expected concrete id absent from the set, emit one
   failure line: `tests suite "<id>" is required by the ATDD process flow but not declared in
   <tests.yaml path>; available: <ids>`.

**Edge cases:**
- `cfg.SystemTest.Config == ""` → skip the check (nothing declared to validate; other layers
  already hard-error on a missing system-test config). Return no failure.
- `cfg.SystemTest.Config != ""` but `LoadTests` fails → emit one failure line naming the path and
  the load error (a missing/corrupt `tests.yaml` is itself worth surfacing here).

## Items

1. **Add `runSuiteExistenceChecks(cfg, eng) []string` to the preflight package.** Implements the
   resolution rules above. Mirror `runScopeResolutionChecks`: nil-guard on `eng`/`cfg`, return a
   `[]string` of failure lines, deterministic (sorted) output. Wire it into `preflight.Run`
   alongside the existing `runScopeResolutionChecks` append (`preflight.go:189`).
   - Files: `internal/atdd/runtime/preflight/preflight.go` (new func + one append line).
   - New dependency: the preflight package will import `internal/runner` (`LoadTests`) and
     `internal/atdd/runtime/testselect` (`ExpandSuiteGroups`). Confirm no import cycle
     (`runner` and `testselect` do not depend on `preflight`).
2. **Decide the literal-sweep helper placement.** The `suite:` literal collection mirrors the
   call-activity walk in `runScopeResolutionChecks`. Either inline a second walk or factor a
   shared node-iterator. Executor's discretion; keep it readable per the surrounding style.
3. **Wire the engine into `config preflight` for parity.** `defaultPreflightOptions`
   (`preflight_helpers.go:40`) does not set `opts.Engine`, so `gh optivem config preflight`
   currently skips both the scope sweep **and** (after Item 1) this new suite check. Load the
   state machine there (same `statemachine.LoadDefault()` the implement path uses at
   `implement_commands.go:306`) and set `opts.Engine` so the "stronger contract" surface
   (`config_commands.go:204` doc comment) validates suites too. *Gate decision for the executor:*
   if wiring the engine into `config preflight` would surface the scope sweep's failures on a
   surface that intentionally validates "YAML shape without committing to a state-machine
   version" (see the `Engine` field doc in `preflight.go:78`), raise it before changing
   `config preflight` — Item 1's `implement`-time check is the must-have; this item is parity.

## Tests (executor's discretion on exact coverage)

Add to `internal/atdd/runtime/preflight/preflight_test.go`, following the existing engine-wired
fixtures (`preflight_test.go:676`):

- Renamed acceptance suite (`acceptance-api` → `acceptance`) → failure naming the missing id.
- Renamed contract suite with external systems configured → failure.
- Renamed contract suite with **no** external systems configured → **no** failure (conditional
  gate holds).
- api-only project (`channels: [api]`) whose `tests.yaml` declares only `acceptance-api` → no
  false positive for `acceptance-ui`.
- All suites present → passes.
- `cfg.SystemTest.Config` pointing at a missing file → failure naming the path.

## Verification

- `scripts/test.sh` (or `go test -p 2 ./internal/atdd/runtime/preflight/...`) green — never
  unbounded `go test ./...` on Windows (`[[feedback_go_test_windows]]`).
- Manual smoke: in a scaffolded project, rename a suite id in `tests.yaml`, run
  `gh optivem implement <issue>`, and confirm it fails at preflight with the new message **before**
  any agent dispatch — not mid-run.
- Manual smoke: `gh optivem config preflight` reports the same missing suite (if Item 3 lands).
