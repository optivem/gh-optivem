# 2026-11-10 11:30 — Legacy baseline: bulk process (implementation)

> 🤖 **Picked up by agent** — `Valentina_Desk` at `2026-11-10T17:19:23Z`

## TL;DR

**Why:** The published article "Acceptance test coverage with AI" (`substack/articles/published/2026-11-06-PAID-ATDD-acceptance-test-coverage-with-ai.md`) describes a legacy on-ramp, but gh-optivem only has the per-ticket `cover-system-behavior` (`task/legacy-coverage`). It writes passing tests for given criteria, but generates no diagram, narratives or criteria, and starts the system again on each ticket, which is too slow for a whole-system baseline.
**End result:** a separate, one-off **bulk baseline** process that baselines a whole legacy system layer by layer, external systems included (contract tests, external drivers, simulators and stubs), with the system under test started once and a guarantee that no system code changes. `cover-system-behavior` stays unchanged for later ad-hoc gaps.

## ▶ Next executable step (resume here)

Step 3b: split `legacy-use-case-writer` into the narrative writer and the criteria writer (see "Steps"); Step 3a (feedback on `approve`) is done. The flow was redesigned on 2026-11-10 to go use case by use case (see "Revised flow"); parts 1, 2, 2b, Step 3a and Step 4 are committed but wired layer by layer, so Step 3d rewires them. Steps 3b-3e build on each other and all touch `process-flow.yaml`: run them one after another, not in parallel.

**Execution mode (user, 2026-11-10):** autonomous. The user asked for autonomous work: incremental commits per completed step (after its tests pass, `go test -p 2`) are approved; no per-step gate.

Known limitations (accepted for now): TS parameterised test names may not match `remove-wip-gate`'s regex (existing); commit labels assume the entry point keeps the `- ${layer}` commit-message suffix (`use_case_state.go` matches ` - ACCEPTANCE TESTS (<id>)` / ` - DSL (<id>)`).

Observation (from Step 4): the external layer makes no commit of its own; its CT/adapters/stubs/simulator are swept into the next commit (normally `ACCEPTANCE TESTS (<id>)`). The per-ticket flow behaves the same way.

Wiring judgment calls (accepted, for the record): the `characterize` binding treats unset as false; new param binding `gated` (strict); new binding `stand-ins-changed` + action `check-stand-ins-changed` (restart only when the fixer touched a stub/simulator); new action `record-use-case-criteria`; new config key `use-cases-path` (default `docs/use-cases`); fixer category `test-agent`; the test-instance contract-real verify is a new node; the `channels.go` adapter-channel unroll also covers `characterize-adapter-layer`; 2 new commit-after-verify invariant exceptions in `invariants.go`.

- Step 3a: feedback is opt-in per approve call site (`ask-for-changes: "true"`), not on every approve: the approvals nested inside a redo (execute-agent / execute-command / fix PRE) would otherwise clear or overwrite it before the redone agent reads it. Pinned by `TestApproveSites_RedoGatesAskForChanges` (a site asks iff its "rejected" edge loops back to a step). A "no" there asks "What should change?" (blank re-asks; end of input stores ""), State `feedback`; a "yes" there unsets it. The dispatcher renders it into the always-registered `${feedback-section}` (empty when unset), so no node params or seeding; only the two legacy writers' prompts include it so far. Step 3d: add `${feedback-section}` to the agents the tests gate (A3) redoes (contract-test, AT, DSL, adapter writers) and pass `ask-for-changes` on the new gates. The refiner's own `refinement-feedback` loop is left as is.

### Part 1 handoff (done 2026-11-10)

- Runner: `gh optivem system start --restart --external-only` (`internal/build/runner/standins.go`, `SystemOptions.ExternalOnly` / `StandInPaths` / `StandInBaseDir`, `SelectStandInServices`; `--external-only` without `--restart`, no stand-in paths, or zero matched services → hard error). Not yet run against real Docker.
- Actions (`internal/atdd/process/actions/characterize_safety.go`, `use_case_loop.go`, registered in `register.go`):
  - `record-start-commits` → `system-start-commits` (JSON `[{repo, commit, paths}]`, keyed by repo slug), `stand-in-paths`, `assertion-checked-commit`; kept on resume; fails hard on microservices/unset, empty paths, stand-in/system overlap.
  - `check-system-unchanged` → `system-unchanged` (bool, binding `system-unchanged`), `system-changed-files`.
  - `check-no-assertion-removed` → `no-assertion-removed` (bool, binding `no-assertion-removed`), `removed-assertions`; advances `assertion-checked-commit` on pass. A removed line re-added verbatim in the same diff counts as a move.
  - `select-next-use-case` (params `use-cases`, `layer-process`) → `use-case-id`, `use-case-left` (bool, binding `use-case-left`), `use-case-cursors`, plus the ESCC keys (`ticket-has-escc`, `escc-systems`, `external-system-contract-criteria`) from the use case file's ESCC section (`intake.ExtractSection` + `intake.ParseSections`; no file yet → `ticket-has-escc=false`).
  - `restamp-current-use-case`: same-use-case mode, for the DSL layer's RESTAMP node (service tasks can't carry params).
  - Shared helper `stampESCC` extracted from `parse-ticket` (behaviour unchanged).

## Decisions (user, 2026-11-10)
- **Redesign: use case by use case (user, 2026-11-10 afternoon; aligns the tool with the series article `substack/articles/published/2026-11-10-FREE-ATDD-new-series-acceptance-testing-in-legacy-code-with-ai.md`). Supersedes "Layer by layer, in bulk", "Hand-holding (Q8)" and the cross-use-case DSL reuse open question (it disappears: a later use case sees earlier use cases' DSL already implemented).**
  - Phase 0: one agent writes two diagrams: the use case diagram (actors + `UC-NNN` use cases) and a **separate dependency diagram** (`A --> B` = B needs data A creates, each arrow with a one-line reason; not UML include/extend), and proposes the most important use case with reasons. Commit. Gate: approve or corrections (feedback).
  - Pilot = the chosen use case + all its transitive dependencies; order dependencies first, ties by lower UC id; a dependency cycle halts with the cycle named. Computed deterministically (action `order-use-cases`), saved in State.
  - Per use case, 5 steps: (1) narrative (main + alternative scenarios, **Preconditions** naming the use cases it depends on; may add a newly found dependency to the dependency diagram) -> commit -> safety gate; (2) acceptance criteria (Gherkin with real values + ESCC) -> commit -> safety gate; (3) acceptance tests passing (external -> AT -> DSL -> drivers, unattended, commit + safety gate per layer; characterization fixer); (4) remove WIP gate for this use case -> commit; (5) delete this use case's criteria -> commit.
  - Pilot use cases: gates after steps 1, 2 and 3. Scaling use cases: no gates.
  - **No go-ahead gate** between pilot and scaling.
  - Scaling = all remaining use cases, same ordering rule. A newly found dependency on a not-yet-done use case moves that one ahead (re-order before continuing).
  - A use case that can't go green halts the flow (existing human halt); never skip, later use cases may depend on it.
  - End: final safety gate, stop system. Diagrams + narratives are kept (they are the requirements documentation).
  - **Feedback at gates:** every gate's "changed" answer carries free text, passed to the agent that redoes the step. Added to `approve` generally (other flows get it too). Supersedes the "Declined layer/diagram feedback" open question.
- **Vendor contract test fails during characterize (user, 2026-11-10):** route the red to `verify-tests-pass` (suite `contract-real`), so the characterization fixer handles it, with the existing human halt when it makes no progress. The per-ticket flow keeps `CONTRACT_REAL_UPSTREAM_GAP_HALT`.

- **Two paths.** A separate **bulk baseline** process (one-off, whole system) plus the existing ad-hoc per-ticket `cover-system-behavior` (unchanged). The bulk flow is its own entry point, **not** a kickoff mode; kickoff stays greenfield-only (nothing in `internal/kickoff/`, which `1121` also waits on).
- **System started once.** In the baseline the system is never modified, so the system is started once at the start and stopped once at the end; the flow does not loop `cover-system-behavior` (which brings Docker up each time). Inner writing/fixing nodes are reused via a "system already up" param, or the start is hoisted to the caller.
- **Layer by layer, in bulk.** Across all use cases: all DSL, then all drivers, etc. — not use case by use case. The **core use case goes first**, through every layer, to establish the architecture.
- **Hand-holding (Q8).** Human approve gates after each layer for the **core use case only**. Bulk layers run unattended. (Be careful with the core use case.)
- **Entry point name:** top-level `gh optivem characterize` (user's choice; beside `kickoff`, separate from it).
- **Core use case choice:** the agent proposes it from the generated use case diagram (with reasons); the human confirms or changes it at the first gate.
- **Deleting criteria at the end:** automatic, as its own commit (revertable with git; `gh optivem spec generate` regenerates readable criteria from tagged tests). Rationale: criteria left behind drift out of date; same as the existing AC policy.
- **Tickets are user stories.** The bulk baseline does not use tickets; the per-ticket path (story with criteria) is for later gaps only.
- **System-code safety gate (Q9, Q8) — mandatory, 100% guarantee.** Two lines: (1) the existing per-agent scope check (`validate-outputs-and-scopes`, snapshot/diff, per-phase allowed paths) with baseline-flow agent scopes that exclude `system-path`; (2) a new **hard** check, `git diff <start-commit> -- <system paths>`, run after each layer's commit and at the end; any non-empty diff HALTS the flow (no fix-agent routing). The start commit is recorded in the flow state at start and kept across resumes. Reason: the existing check is soft (a violation routes to a `fix-scope-diff` agent), per-dispatch only, covers only scoped writing agents, and may not see changes an agent commits itself (verify in Step 1). This is not conditional on what `green-when-complete` already does. Also guard "never weaken an assertion" (assertion-removal diff check).
- **Design review answers (2026-11-10).** All recommendations accepted except the external-systems part:
  - Per-use-case State: `select-next-use-case` re-derives it from git (tests tagged `UC-NNN`, port changes from that use case's commits, ESCC from its criteria file).
  - Use case loop: runtime loop (`select-next-use-case` + gateway), not a load-time unroll.
  - Skipped gates: `VALIDATE_CHANNELS_REGISTERED`, `GATE_DSL_PORT_CHANGED`, `GATE_SYSTEM_DRIVER_PORTS_CHANGED` move into the AT / DSL / adapter layer wrappers. **Overridden:** `EXTERNAL_DRIVER_PORT_CHANGED_HALT` is not kept; a late external-port change routes into the external layer (see 2b).
  - Fixer selection: State flag `characterize` (preset `false` on `main`, `true` on `characterize`), one new gateway per verify process.
  - Commits: one per (layer, use case); safety gate after each.
  - Assertion check: per-language regex over removed lines in test paths since the last checked commit; any hit halts.
  - `apply-ac-policy`: the criteria layer appends each criterion id to `merged-criteria`.
  - System on HALT: left running (confirmed).
  - Declined core layer: redo it with the feedback, as a new commit on top (confirmed).
- **External systems are IN SCOPE for v1** (user overrode the recommendation). The baseline also characterizes external systems: contract tests (CT), external driver ports/adapters, simulators and stubs, via the existing ESCC / contract path.

## Source (article steps)

1. Generate the use case diagram (source code wins over the docs). Human confirms it and picks the core use case.
2. Core use case: narrative + business rules + Gherkin acceptance criteria.
3. Acceptance test architecture.
4. The core use case's tests pass against the existing system. They are characterization tests: on failure, fix the test, criterion and narrative, never the system, and never weaken an assertion.
5. Repeat for the other use cases, one commit per layer (bulk).
6. Delete the criteria.

Updated by the series article (`substack/articles/published/2026-11-10-FREE-ATDD-new-series-acceptance-testing-in-legacy-code-with-ai.md`), which wins where they differ: phase 0 = use case diagram incl. dependencies, pick the most important use case; phase 1 pilot = that use case and the ones it depends on, in order, each through narrative -> acceptance criteria -> acceptance tests passing; phase 2 scaling = all others, one at a time, in dependency order. See "Revised flow".

## Step 1 findings

All lines in `internal/atdd/process/process-flow.yaml` unless noted.

- **Start/stop (Q1).** No ATDD process ever stops the system (no `system stop` / `runner.Down` in `internal/atdd`). In the cover path the system is started by:
  - `write-and-verify-acceptance-test-code` → `START_SYSTEM` (`start-system`, :1389-1391), reached via `write-acceptance-tests-and-dsl` (:1204).
  - `implement-test-layer` → `START_SYSTEM` (`start-system`, :2192-2194), used by `implement-and-verify-dsl` (:1448) and `implement-and-verify-system-driver-adapters` (:1471).
  - ESCC branch only: `implement-external-drivers-if-needed` (:1136, gate `ticket-has-escc`) → `implement-and-verify-external-system-driver-adapters-contract-tests` (:1533) → `build-system` + `start-system-restart` (:1681-1843).
  - `start-system` runs `gh optivem system start` (:3485-3505). `runner.Up` skips when any health URL is up (`internal/build/runner/system.go:135-140`). So a plain start on a running system is a cheap health probe, not a Docker restart. Only `start-system-restart` (`--restart`, :3507) recreates containers.
- **Q2 recommendation: hoist the start to the caller, no param.** The inner starts are already idempotent no-ops on a running system, and the engine has no per-param defaulting (:3457-3459), so a "system already up" param would need a new gateway in 3 processes and a pinned value at every other caller. The bulk flow adds one start + one stop. The "started once" test (Step 4) asserts no `build-system` / `start-system-restart` dispatch, a single cold start, and only `restart-external-systems` recreates (see "External systems" below).
- **System code paths (Q3).** Not language-specific; read from `gh-optivem.yaml`.
  - Resolver: `projectconfig.Config.SystemSurfacePaths(channel)` (`internal/kernel/projectconfig/config.go:719-760`). Monolith → `[system.path]`. Multitier → `[system.backend.path, system.frontend.path]` for channel `""` (api → backend, ui → frontend). Microservices / unset → `ok=false`.
  - `system.path` is empty on multitier by construction. `ResolveLayerPaths` skips it there (`internal/atdd/phase_scopes.go:37-50`, `MonolithOnlyPathKeys`). `AddSystemSurfaceScope` appends the tier paths instead (`internal/atdd/process/actions/scope.go:285-318`).
  - Also system code: `system.db-migration-path` (default `system/db/migrations`, `internal/kernel/projectconfig/paths_defaults.go:17`; `scope.go:238`).
  - Multirepo: system tiers can live in other repos (`internal/config/optivemyaml/optivemyaml.go:215-240`). A `git diff` in the test repo will not see them. **Decided (user, 2026-11-10):** the gate records a start commit per system repo and diffs each one against it. Microservices (`ok=false`) fails hard.
- **Fix loop can write system code (Q4): yes.** There is no `phase-scopes.yaml`; scopes are inline `read:`/`write:` on each node.
  - `verify-tests-pass` → `fix-unexpected-failing-tests` (:2280) has `write: [... system-path ...]` (:3092).
  - `verify-tests-fail` → `fix-unexpected-passing-tests` (:2403) has `system-path` too (:3067).
  - On multitier, `AddSystemSurfaceScope` with channel `""` widens this to both tiers.
  - The scope check is also soft: `execute-agent` routes violations to `fix-scope-diff` or the scope-exception branch (:3744-3800).
  - The bulk flow needs fixer nodes without `system-path`.
- **Snapshot vs agent commits (Q5): no.** `snapshotWorkingTree` (`outputs.go:337`) stores SHA-256 hashes of `git status --porcelain` paths only. No HEAD is recorded (`internal/atdd/process/actions/worktree.go:22-101`). `modifiedPathsSinceFingerprint` (`worktree.go:115-137`) diffs that against the current dirty set. If an agent edits a clean file and commits it, the file is clean again and never shows up. This confirms the need for the hard `git diff <start-commit>` gate.
- **Contradicts the plan:** "brings Docker up each time" is overstated. Without ESCC, each cover run only runs idempotent `start-system` probes. The real cost is the per-ticket loop, not repeated Docker restarts. The decision (start once, hoist) still holds.

### External systems findings (2026-11-10)

- **What the contract path writes.** Per external system (load-time unroll, `UnrollExternalSystems`, `internal/engine/statemachine/channels.go:225`), `implement-and-verify-external-system-driver-adapters-contract-tests` (:1533) runs: `write-contract-tests` (ct-test, dsl-port, dsl-core; :2531-2532), `write-stub-fidelity-tests`, DSL, `implement-external-system-driver-adapters` (external-system-driver-adapter, testkit side; :2938-2939), then `reconcile-external-contract-producer` (:1668), which may run `implement-external-system-real-simulator` (writes `external-system-simulator`, :3048-3049) and `implement-external-system-stubs` (writes `external-system-stub`, :3006-3007).
- **Rebuild/restart today recreates the system under test.** `reconcile-external-contract-producer` runs `build-system` + `start-system-restart` four times (:1679/1684, :1727/1732, :1779/1784, :1836/1841). `runner.Build` builds every compose service (`internal/build/runner/system.go:107-119`). `--restart` force-recreates **every non-persistent service** (`up -d --build --force-recreate --no-deps <all but postgres>`, `system.go:239-265`), so the SUT container is recreated too. There is no per-service option today (`system_commands.go:90`).
- **A stub/simulator change needs only its own container recreated.** In shop the SUT and the stand-ins are separate compose services: `system` builds from `system/monolith/java`, `external-system-simulators` builds from `external-systems/simulators`, `external-system-stubs` is WireMock with `external-systems/stubs` bind-mounted locally and `COPY`-baked in the pipeline (`shop/docker/java/monolith/docker-compose.local.real.yml:5-9,91-93`, `docker-compose.local.stub.yml:91-96`, `shop/external-systems/stubs/Dockerfile:2`). The SUT image does not contain stub or simulator code, and `--no-deps` leaves dependents running. So `up -d --build --force-recreate --no-deps <stand-in services>` picks up the change (the runner's own comment: force-recreate "reloads WireMock's mounted mappings", `system.go:261-263`) without touching the SUT. External driver adapters and contract tests are testkit code: no container change at all.
- **Stand-in paths are not system paths.** The registry declares `stub.path` / `simulator.path` per system (`internal/kernel/projectconfig/config.go:445-470`, resolved by `ExternalRegistryPaths`, :500); shop uses `external-systems/stubs` and `external-systems/simulators`, outside `system/` (`shop/gh-optivem-monolith-java.yaml:37-45`). `SystemSurfacePaths` + `db-migration-path` do not include them.
- **Tickets feed the ESCC keys; the baseline must feed them itself.** `parse-ticket` stamps `ticket-has-escc`, `escc-systems`, `external-system-contract-criteria` from the `## External System Contract Criteria` section via `intake.Parse` (`internal/atdd/process/actions/tracker.go:114-130`). With `ticket-has-escc` true, `touchedExternalSystemNames` takes `escc-systems` as authoritative and ignores port-change paths (`actions/external.go:63-75`).
- **Order today: external first.** `implement-external-drivers-if-needed` runs before the AT step "so stubs are in place before acceptance tests run" (:1127-1128, :1188-1189), and the cover-path `verify-pending-on` values leave external ports out because they are already built (:1226-1229, :1262-1265; `internal/atdd/process/gates/bindings.go:614-630`).

## Step 2: flow design

Design only. Nothing in `process-flow.yaml` or Go changed yet.

**Colour key.** Fill and shape follow the legend in `docs/process-diagram.md` (`serviceNode` white, `agentNode` dark blue, `humanNode` yellow, `errorEndNode` red border, call activities unfilled). The legend has no marker for new or changed nodes, so this design adds an overlay, plus a text tag in each label:

- `newNode`: thick purple border, label tag `NEW`.
- `changedNode`: thick orange border, label tag `CHANGED`.
- No overlay, label tag `REUSE <process-flow.yaml id>`: reused as is.

The overlay overrides the TDD-stage border. It is for this plan only; the generated diagrams keep the legend as is.

**Shape of the design.**

- `characterize` (NEW): the top-level flow behind `gh optivem characterize`.
- `characterize-layer` (NEW): one layer over a list of use cases. Called 5 times for the core use case (gated) and 5 times for the rest (unattended). This gives "layer by layer, in bulk".
- Layer wrappers (NEW): `write-use-case-criteria`, `characterize-at-layer`, `characterize-dsl-layer`, `characterize-adapter-layer`; the external layer is REUSE `implement-external-drivers-if-needed`.
- `verify-system-unchanged` (NEW): the hard safety gate, called after every commit and at the end.
- `refresh-external-producer` (NEW) inside CHANGED `reconcile-external-contract-producer`: restarts only the stand-ins when characterizing.
- `verify-tests-pass` / `verify-tests-fail` (CHANGED): the characterization fix loop.

**Layer order per use case:** criteria (incl. ESCC) → external → acceptance tests → DSL → system driver adapters. The external layer sits **before** the AT layer, not after the adapter layer, because the AT layer runs against the stub stack and its green-when-complete prediction assumes the stubs already exist (evidence above). The core use case goes through all five first.

**Restart rule (precise).** While `characterize == true`, the system under test is started once (by `characterize`) and never rebuilt or recreated. Any external-layer step that changes a producer-side stand-in (simulator or stub) is followed by `restart-external-systems` = `gh optivem system start --restart --external-only`, which runs `up -d --build --force-recreate --no-deps` on the stand-in services only, then health-waits. The stand-in services are found deterministically from `docker compose config --format json`: every service whose build context or bind-mount source resolves under a registry `stub.path` / `simulator.path`. Zero matches fails hard (never a silent no-op). With `characterize == false`, `refresh-external-producer` runs `build-system` + `start-system-restart` exactly as today.

### Revised flow (2026-11-10, supersedes the old layer-by-layer 2a/2b)

```mermaid
flowchart TD
    START(( )) --> RECORD[["record-start-commits"]] --> START_SYS["start-system (once)"]
    START_SYS --> DIAGRAMS["legacy-use-case-diagram-writer<br/>use case diagram + dependency diagram<br/>+ proposed most important use case"]
    DIAGRAMS --> COMMIT_D["commit"] --> GATE_D["approve (with feedback)"]
    GATE_D -- "changed: feedback" --> DIAGRAMS
    GATE_D -- approved --> ORDER_P[["order-use-cases: pilot"]]
    ORDER_P --> PILOT["characterize-use-cases<br/>gated: true"]
    PILOT --> ORDER_S[["order-use-cases: scaling"]] --> SCALE["characterize-use-cases<br/>gated: false"]
    SCALE --> FINAL["verify-system-unchanged"] --> STOP["stop-system"] --> END(( ))
```

`characterize-use-cases` (runtime loop: `select-next-use-case` + gateway) runs `characterize-use-case` per use case:

```mermaid
flowchart TD
    S(( )) --> NARR["legacy-use-case-narrative-writer"] --> C1["commit + verify-system-unchanged"] --> G1{"gated?"}
    G1 -- true --> A1["approve (feedback)"]
    A1 -- changed --> NARR
    A1 -- approved --> REORDER
    G1 -- false --> REORDER[["order-use-cases: re-order if a new dependency"]]
    REORDER --> CRIT["legacy-use-case-criteria-writer (+ ESCC)"] --> C2["commit + verify-system-unchanged"] --> G2{"gated?"}
    G2 -- true --> A2["approve (feedback)"]
    A2 -- changed --> CRIT
    A2 -- approved --> TESTS
    G2 -- false --> TESTS["external -> AT -> DSL -> adapter layers<br/>(existing wrappers, commit + safety gate each)"]
    TESTS --> G3{"gated?"}
    G3 -- true --> A3["approve (feedback)"]
    A3 -- changed --> TESTS
    A3 -- approved --> WIP
    G3 -- false --> WIP[["remove-wip-gate (this use case)"]] --> CW["commit"] --> DEL[["delete this use case's criteria"]] --> CD["commit"] --> E(( ))
```

- If re-ordering puts an unfinished dependency ahead of the current use case, the current one goes back in the list behind it (its narrative commit stays; resume re-derives state from git).
- The external, AT, DSL and adapter wrappers (`implement-external-drivers-if-needed`, `characterize-at-layer`, `characterize-dsl-layer`, `characterize-adapter-layer`) stay as built; only the outer loop changes. `characterize-layer` is replaced.

### 2c. `verify-system-unchanged` (hard safety gate)

```mermaid
flowchart TD
    S_START(( ))
    DIFF_SYSTEM[["NEW check-system-unchanged<br/>per system repo: git diff start-commit, system paths only<br/>+ untracked files under system paths"]]
    GW_SYS{"System code<br/>unchanged?"}
    HALT_SYS((⚡))
    CHECK_ASSERT[["NEW check-no-assertion-removed<br/>removed assertion lines in test paths<br/>since the last checked commit"]]
    GW_ASSERT{"No assertion<br/>removed?"}
    HALT_ASSERT((⚡))
    S_END(( ))

    S_START --> DIFF_SYSTEM --> GW_SYS
    GW_SYS -- "no: HALT, no fix agent" --> HALT_SYS
    GW_SYS -- yes --> CHECK_ASSERT --> GW_ASSERT
    GW_ASSERT -- "no: HALT, no fix agent" --> HALT_ASSERT
    GW_ASSERT -- yes --> S_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    classDef errorEndNode fill:#ffffff,stroke:#dc3545,stroke-width:2px,color:#000000
    classDef newNode stroke:#6f42c1,stroke-width:4px

    class DIFF_SYSTEM,CHECK_ASSERT serviceNode
    class HALT_SYS,HALT_ASSERT errorEndNode
    class DIFF_SYSTEM,CHECK_ASSERT newNode
```

- System paths = `cfg.SystemSurfacePaths("")` + `system.db-migration-path`, resolved once by `record-start-commits`. **Stand-in paths (`stub.path`, `simulator.path`) are excluded**: they are test doubles the baseline may write. The SUT's own outbound client code to external systems lives under the system paths and stays protected. A vendor test instance is not in any repo, so there is nothing to diff.
- `git diff <start-commit>` (no `HEAD`) compares the working tree, so it sees committed and uncommitted edits. It does not see new untracked files, so the gate also runs `git status --porcelain -- <system paths>`.
- Test paths for the assertion check include `ct-test` (contract tests), so a weakened contract assertion halts too.
- Both checks are error ends: no `fix-scope-diff`, no scope-exception branch.

### 2d. Characterization fix loop (CHANGED `verify-tests-pass`; `verify-tests-fail` mirrors it)

```mermaid
flowchart TD
    V_START(( ))
    RUN_TESTS["Run Tests<br/>REUSE run-tests"]
    GW_OUTCOME{"Test Outcome?<br/>CHANGED GATE_TESTS_OUTCOME<br/>fail edge rerouted"}
    GW_CHAR{"Characterizing?<br/>NEW gateway, State characterize"}
    FIX_SYSTEM["Fix Unexpected Test Failures<br/>REUSE fix-unexpected-failing-tests<br/>writes system-path"]
    FIX_CHAR["NEW fix-characterization-failing-tests<br/>fix the test, criterion, narrative or stand-in<br/>never the system, never weaken an assertion"]
    CHECK_PROGRESS["Check Fix Progress<br/>REUSE CHECK_FIX_PROGRESS + its human halt"]
    V_END(( ))

    V_START --> RUN_TESTS --> GW_OUTCOME
    GW_OUTCOME -- pass --> V_END
    GW_OUTCOME -- fail --> GW_CHAR
    GW_CHAR -- "false (other flows)" --> FIX_SYSTEM --> CHECK_PROGRESS
    GW_CHAR -- "true (characterize)" --> FIX_CHAR --> CHECK_PROGRESS
    CHECK_PROGRESS --> RUN_TESTS

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    classDef newNode stroke:#6f42c1,stroke-width:4px
    classDef changedNode stroke:#fd7e14,stroke-width:4px

    class FIX_CHAR agentNode
    class FIX_CHAR,GW_CHAR newNode
    class GW_OUTCOME changedNode
```

- NEW `fix-characterization-failing-tests` / `fix-characterization-passing-tests`: one NEW agent, `characterization-test-fixer`. Write scope: test paths (at-test, ct-test, DSL, system and external driver adapters), the use-cases dir, and `external-system-stub` / `external-system-simulator`. No `system-path`, no tier paths from `AddSystemSurfaceScope`, no `system.db-migration-path`.
- The same loop serves the contract-suite verifies inside `reconcile-external-contract-producer`. If the fixer edits a stand-in, the loop's next `RUN_TESTS` would run against a stale container, so `fix-characterization-*` ends with NEW `restart-external-systems` when its diff touches a stand-in path (cheap; SUT untouched; not drawn).
- `verify-tests-fail` gets the same gateway in front of `fix-unexpected-passing-tests`.
- `characterize` sets `characterize: true` in `preset-state`; `main` presets `false`.

### 2e. External producer refresh (CHANGED `reconcile-external-contract-producer`)

Each of the four `BUILD_SYSTEM_AFTER_*` + `START_SYSTEM_AFTER_*` pairs (:1679/1684, :1727/1732, :1779/1784, :1836/1841) becomes one call to NEW `refresh-external-producer`:

```mermaid
flowchart TD
    R_START(( ))
    GW_CHAR{"Characterizing?<br/>NEW gateway, State characterize"}
    BUILD["Build System<br/>REUSE build-system"]
    RESTART_ALL["Start System<br/>REUSE start-system-restart"]
    RESTART_EXT["Restart External Stand-ins Only<br/>NEW restart-external-systems<br/>system start --restart --external-only"]
    R_END(( ))

    R_START --> GW_CHAR
    GW_CHAR -- "false (per-ticket flows, unchanged)" --> BUILD --> RESTART_ALL --> R_END
    GW_CHAR -- "true (characterize: SUT stays up)" --> RESTART_EXT --> R_END

    classDef newNode stroke:#6f42c1,stroke-width:4px

    class GW_CHAR,RESTART_EXT newNode
```

Where it sits in the reconcile leg (only the changed parts are new; the rest is REUSE):

```mermaid
flowchart TD
    P_START(( ))
    REFRESH_1["Refresh After Driver<br/>NEW refresh-external-producer"]
    PROBE_REAL["Probe Contract-Real<br/>REUSE run-tests"]
    GW_REAL{"Contract-Real Outcome?<br/>REUSE GATE_CONTRACT_REAL_OUTCOME"}
    GW_KIND{"Real Kind?<br/>CHANGED GATE_CONTRACT_REAL_RED_KIND"}
    IMPL_SIM["Implement Simulator<br/>REUSE implement-external-system-real-simulator"]
    REFRESH_2["Refresh After Simulator<br/>NEW refresh-external-producer"]
    VERIFY_REAL["Verify Contract-Real Pass<br/>REUSE verify-tests-pass"]
    UPSTREAM_HALT((⚡))
    STUB_LEG["Stub + stub-fidelity legs<br/>REUSE, with refresh-external-producer<br/>after each stub implement"]
    P_END(( ))

    P_START --> REFRESH_1 --> PROBE_REAL --> GW_REAL
    GW_REAL -- pass --> STUB_LEG
    GW_REAL -- fail --> GW_KIND
    GW_KIND -- simulator --> IMPL_SIM --> REFRESH_2 --> VERIFY_REAL --> STUB_LEG
    GW_KIND -- "test-instance, not characterizing" --> UPSTREAM_HALT
    GW_KIND -- "test-instance, characterizing: see open question" --> VERIFY_REAL
    STUB_LEG --> P_END

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    classDef errorEndNode fill:#ffffff,stroke:#dc3545,stroke-width:2px,color:#000000
    classDef newNode stroke:#6f42c1,stroke-width:4px
    classDef changedNode stroke:#fd7e14,stroke-width:4px

    class UPSTREAM_HALT errorEndNode
    class REFRESH_1,REFRESH_2 newNode
    class GW_KIND changedNode
```

- At `REFRESH_1` (after the driver adapter) nothing container-side changed in characterize; the external-only restart is redundant but kept so there is one rule and the four sites stay uniform (cost: one stand-in recreate).
- `START_SYSTEM_BEFORE_STUB_PROBE` (plain `start-system`) stays: a no-op probe on the running stack.
- The `test-instance` edge depends on the open question; drawn with the recommended answer.

### Node table

| Node | Reuse (`process-flow.yaml` id) or new |
|---|---|
| Record start commits + resolve system and stand-in paths, overlap check | NEW action `record-start-commits` |
| Start system once | REUSE `start-system` |
| Use case diagram + proposed core use case | NEW agent `legacy-use-case-diagram-writer` (variant of kickoff `use-case-diagram-writer`; reads source, not a prototype) |
| Confirm diagram + core use case, approve each core layer | REUSE `approve` |
| Commit diagram, criteria, WIP removal, criteria deletion | REUSE `commit` |
| Layer over use cases | NEW `characterize-layer` + NEW action `select-next-use-case` (stamps ESCC keys via REUSE `intake.Parse`) |
| Criteria + ESCC layer | NEW `write-use-case-criteria` + NEW agent `legacy-use-case-writer` (variant of kickoff `use-case-writer`; also writes ESCC sections) |
| External layer (CT, external DSL, external driver adapters, simulator, stubs) | REUSE `implement-external-drivers-if-needed` → REUSE `implement-and-verify-external-system-driver-adapters-contract-tests` (unrolled per system) |
| External producer reconcile | CHANGED `reconcile-external-contract-producer` (4 build+restart pairs → `refresh-external-producer`; `GATE_CONTRACT_REAL_RED_KIND` test-instance edge) |
| External producer refresh | NEW `refresh-external-producer` (gateway on `characterize`) + NEW `restart-external-systems` (`gh optivem system start --restart --external-only` via REUSE `execute-command`) |
| Runner support for external-only restart | CHANGED `internal/build/runner/system.go` (`SystemOptions.ExternalOnly`, stand-in service selection from compose config) + CHANGED `system_commands.go` (flag) |
| Acceptance test layer | NEW `characterize-at-layer` = REUSE `write-and-verify-acceptance-test-code` + REUSE action `validate-channels-registered` |
| DSL layer | NEW `characterize-dsl-layer` = REUSE `implement-and-verify-dsl` + REUSE bindings `dsl-port-changed`, `external-driver-port-changed`; late discovery → `write-use-case-criteria` + `select-next-use-case` + REUSE `implement-external-drivers-if-needed` + REUSE `verify-tests-pass` |
| Driver adapter layer | NEW `characterize-adapter-layer` = REUSE binding `system-driver-port-changed` + REUSE `implement-and-verify-system-driver-adapters` |
| Per-layer test run + commit | REUSE `implement-test-layer` (inner `START_SYSTEM` = no-op probe) |
| Safety gate | NEW `verify-system-unchanged` + NEW actions `check-system-unchanged`, `check-no-assertion-removed` |
| Fix loop | CHANGED `verify-tests-pass`, `verify-tests-fail` (new gateway); NEW `fix-characterization-failing-tests`, `fix-characterization-passing-tests`, agent `characterization-test-fixer` |
| Remove WIP gate | REUSE action `remove-wip-gate` |
| Stop system | NEW `stop-system` (`gh optivem system stop` via REUSE `execute-command`) |
| Delete criteria | REUSE action `apply-ac-policy` |
| Entry point | NEW `gh optivem characterize` command |
| No longer reached | `EXTERNAL_DRIVER_PORT_CHANGED_HALT` (lives in `write-acceptance-tests-and-dsl`, which `characterize` does not call; unchanged for per-ticket flows) |

## Steps

- [ ] Step 3b: Split `legacy-use-case-writer` into `legacy-use-case-narrative-writer` (narrative + Preconditions naming dependency UC ids; may add a found dependency to the dependency diagram) and `legacy-use-case-criteria-writer` (Gherkin `UC-NNN.M` criteria + ESCC section). Both take `feedback`.
- [ ] Step 3c: `legacy-use-case-diagram-writer` also writes the dependency diagram (Mermaid, under `use-cases-path`) and takes `feedback`; NEW action `order-use-cases` (modes: pilot = chosen + transitive deps; scaling = the rest; re-order after a narrative adds a dependency): topological, ties by UC id, cycle -> hard error. Unit tests.
- [ ] Step 3d: Rewire `characterize` in `process-flow.yaml` to the revised flow (use case by use case, gates on pilot only, per-use-case WIP removal + criteria deletion, no go-ahead gate, final safety gate + stop at the end); update `characterize_flow_test.go` and any affected tests.
- [ ] Step 3e: `gh optivem characterize` entry point (top level, beside `kickoff`): seed `ticket-id` / `issue-title` for commit messages (keep the `- ${layer}` suffix), add `characterize` to the driver's `topProcesses` and the diagram's `processOrder`.
- [ ] Step 5: Docs: `docs/bpmn-process-design.md`, README/CLI reference for `characterize` and `system start --external-only`.

Edits `process-flow.yaml`: never run in parallel with another plan editing it. Diagrams under `docs/process-diagram*` are regenerated by CI; don't regenerate locally.

## Out of scope

- Parallel per-use-case test runs (possible later; tests are cheap against a running system).
- Narrowing test runs after refactors (see `plans/20261110-1125-consider-narrow-test-runs-after-refactor.md`).
- Multi-stack SUT DNS caching after a stand-in recreate (SUT may hold stale connections to a recreated container; revisit only if a rehearsal shows it).
