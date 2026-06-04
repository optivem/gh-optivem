# Scoped `implement`: layer/channel slices as the team-handoff seam

**Status:** 1702 landed (unblocked); mechanism + red-gate resolved against the code — see Item 2 / D-red-gate
**Created:** 2026-05-30 17:25 CEDT

> **Depends on `plans/20260530-1702-channels-field-channel-by-channel.md` — land
> that plan first; do NOT execute these two in parallel.** This plan is built on
> two of 1702's deliverables and cannot be executed independently:
>
> - the **`channels:` SSoT** (1702 Item 1 / D2) — the `<ch>` arg validates
>   against it (Item 4); it does not exist until 1702 lands.
> - the **`common` param** (1702 D5/D7, Items 4–5) — D-common / Item 5 reuse it
>   verbatim and must stay consistent with 1702's resolution.
>
> The two plans also write the same surfaces (`process-flow.yaml`,
> `internal/projectconfig/`, `system-implementer.md`,
> `statemachine/{transitions,phase_scopes}_test.go`), so concurrent runs would
> collide — and both carry the statemachine loopback/RAM hazard around those
> fixtures. Sequence: **1702 fully landed + committed → then this plan.**

## Problem

`gh optivem implement` walks the **whole** four-layer pipeline from `START` for
one `--issue`, single pass, no way to stop or resume mid-stack
(`implement_commands.go::newImplementCmd` → `driver.Run` from START). That is
exactly right for a **fullstack developer doing the whole ticket** — and that
default must stay untouched.

But an operator running **separate backend and frontend teams** wants to split
one ticket along the architecture's own seams:

- the whole team **mobs** the shared, channel-agnostic contract (acceptance
  tests + DSL + driver-port interfaces) — the "shared decisions" layer;
- the **API team** then implements the API channel (driver adapter, then
  system), the **UI team** the UI channel — the per-team "decisions for API /
  UI" layers.

There is no way today to invoke `implement` scoped to a slice of the stack, so
this team workflow cannot be expressed.

## Goal

Add **layer/channel-scoped invocation** to `implement`, so a ticket can be
produced in slices by different people across separate clones, while the
**no-arg form remains the whole pipeline**. The CLI scope becomes the
(informal, unenforced) ownership boundary — no CODEOWNERS, no permissions.

### Decided ergonomics (D-flags / D-positional resolved)

The scope is one enum-valued `--target` flag (+ `--channel` for channel-split
targets), not boolean verb flags — see D-flags for why.

```
# Whole team, mob-programming at product level — the SHARED red contract:
gh optivem implement 7 --target test               # Test + DSL Port + DSL Core + Driver Port

# API team (backend):
gh optivem implement 7 --target driver-adapter --channel api
gh optivem implement 7 --target system --channel api

# UI team (frontend):
gh optivem implement 7 --target driver-adapter --channel ui
gh optivem implement 7 --target system --channel ui

# Fullstack developer, start to end (UNCHANGED current behaviour):
gh optivem implement 7            # positional issue (D-positional) — or --issue 7
```

The scoped flag is a **refinement of the default**, never a separate mode:
omit `--target` and you get the full walk exactly as today.

## Why the slices fall on real seams (not invented ones)

The four-layer ATDD stack is **Test → DSL Port → DSL Core → Driver Port →
Driver Adapter → System**. Two of those layers are channel-agnostic (shared)
and two are channel-split:

| `--target` | Layers produced | Channel? | Owner | End state |
| --- | --- | --- | --- | --- |
| `test` | Test, DSL Port, DSL Core, Driver Port (+ external, see D-external) | agnostic | mob / whole team | **RED by design** (no system yet) |
| `driver-adapter --channel <ch>` | Driver Adapter `<ch>` | per-channel | that channel's team | compiles, still red |
| `system --channel <ch>` | System `<ch>` (+ the common layer on the first channel, see D-common) | per-channel | that channel's team | channel green |

A `--target` is a **slice** — a contiguous run of phases that may span several
layers (`test` covers four), which is why the flag is `--target`/slice, not
`--layer`. This is the **same channel axis** as the open channels plan
(`plans/20260530-1702-channels-field-channel-by-channel.md`), and the same
per-slice-commit philosophy. `--target system --channel <ch>` is the **manual,
per-team** counterpart to that plan's **automatic** channel-by-channel unroll:
same decomposition, two drivers.

## What already exists (reuse, do not reinvent)

- **The phases the slices map onto already exist as distinct writing-agent
  MIDs.** `acceptance-test-writer`/`contract-test-writer` (tests + DSL stubs),
  `dsl-implementer` (writes `dsl-core`, `driver-port`,
  `external-system-driver-port`), `system-driver-adapter-implementer`,
  `system-implementer` (system code). A slice is a contiguous run of these, not a
  new pipeline.
  - **CORRECTION (verified against the code 2026-05-30).** An earlier draft of
    this bullet claimed `system-driver-adapter-implementer` is already "per
    channel". **It is not.** `process-flow.yaml:1535`
    (`implement-system-driver-adapters`) is a **single, channel-agnostic
    dispatch** (`read:[driver-port,driver-adapter] write:[driver-adapter]`, no
    `channel` param), and it lives **inside the RED `write-and-verify-acceptance-
    tests` cascade** (gated on `system-driver-port-changed`). 1702's
    `UnrollSystemChannels` (`channels.go:54`) unrolls **only** the GREEN
    `IMPLEMENT_AND_VERIFY_SYSTEM` step per channel — it does **not** touch the
    adapter step. So `--target driver-adapter --channel <ch>` is **net-new
    decomposition**, not reuse — see D-adapter-ownership.
- **`channels:` SSoT** (the 1702 plan) supplies the `<ch>` token vocabulary and
  its lowercase canon + validation. The `<ch>` arg here MUST reuse it, not
  hardcode `api`/`ui` per flag.
- **`gh optivem process scope [<phase>]`** (`process_commands.go:78`) already
  introspects per-phase read/write scope — proof the phase set is addressable
  by name from the CLI.
- **Per-phase scope sets** (`phase-scopes.yaml`, `internal/atdd/phase_scopes_test.go`)
  bound what each slice may write — the scoped run can assert it stayed within
  the owning team's layer.
- **`--manual-agents`, node-extras/replacements, task-prompt overrides** —
  existing per-run customization seams in `implement_commands.go`.

## Decisions resolved

- **D-flags — RESOLVED: one enum-valued `--target` flag + `--channel`.** Not
  boolean verb flags. `--target test|driver-adapter|system`, plus `--channel
  <ch>` for the two channel-split targets. Chosen over the verb form (`--test` /
  `--driver-adapter <ch>` / `--system <ch>`) because: (1) mutual exclusion is
  **structural** — `--target` takes one value, so two slices can't be requested
  at once, no hand-written exclusion check; (2) `--channel` validates against the
  `channels:` SSoT in **one** place, reused by both channel-split targets (the
  verb form would wire that check into two flags); (3) a future slice is a new
  enum value, not a new flag symbol. The word is **`--target`** (not `--layer`):
  a slice spans several layers (`test` spans four), so "layer" would mislead;
  "target" reads naturally ("aim the run at the system slice") and matches the
  mainstream `make`/`cargo`/`msbuild` value-flag convention. Value for the shared
  slice is **`test`** (kept from the sketch). Rules: `--channel` is **required**
  for `driver-adapter`/`system` and **rejected** for `test` (channel-agnostic);
  unknown `--channel` token → channels-validation error.
- **D-positional — RESOLVED: accept a positional issue, additive.** `gh optivem
  implement 7` and `gh optivem implement --issue 7` both work; `--issue` stays
  valid. Matches the operator sketch, better ergonomics, zero risk.
- **D-common — RESOLVED: option (b), first channel carries `common: true`.** The
  channel-agnostic **common** layer (DTO / entity / service / migration; never
  "core" — that collides with **DSL Core**) is built in the **first** channel's
  `--target system` dispatch (`common: true`); later channels are `common:
  false` deltas. Chosen over (a) "mob builds common in `--target test`" because
  this is a teaching repo for **ATDD/outside-in**: option (a) would write
  production code (entities, services, **migration**) speculatively in the still-
  RED shared slice — big-design-up-front for the domain model, the exact thing
  outside-in prevents. Option (b) lets the common layer **emerge as the first
  channel goes green**, which is methodologically correct, and is **consistent
  with 1702's D5/D7 as-built** (reuses the `common` boolean param verbatim — no
  rework of the in-flight plan). The resulting "UI can't `--target system` until
  the first channel landed common" is a **natural, correct** ATDD ordering, not a
  bug — and it is made **safe and explicit** by the git-state resume gate (a UI
  `system` run detects common DONE before entering). See Resume mechanism.
- **D-red-gate — RESOLVED: two-tier expected-red gate = the resume DONE
  predicate.** The driver adapter (`MyShopApiDriver` / `MyShopUiDriver`) is
  **inherently channel-specific** — there is no channel-agnostic adapter the mob
  could build (confirmed against the cascade: the existing RED slice builds
  adapters *inside* RED precisely so the AT can run-and-fail for an assertion
  reason). The shared `--target test` slice stops at the driver **port**, so it
  **cannot reach assertion-red** on its own — per channel the AT is not yet
  runnable (no adapter). The gate is therefore **two-tier**, NOT one criterion:
  - **`--target test` — "port-deep, adapter-pending" red.** Succeeds when it
    **compiles through the driver port + its write-scope files are present + the
    per-channel ATs are *pending* (present, wired to the port, not yet runnable
    because no adapter)**. This is a *weaker, well-defined* red than assertion-red
    — deliberately, because the mob owns only the channel-agnostic contract.
  - **`--target driver-adapter <ch>` — assertion-red.** Advances that channel to
    the strong red: **compiles + adapter write-scope present + that channel's ATs
    fail for the *right* reason** (assertion/runtime, not compile/wiring).

  This is **the same predicate the resume detector uses to classify an upstream
  slice DONE** (see "Resume mechanism") — build it once (two-tier), evaluate on
  *this* slice (the success gate) and on *upstream* slices (resume detection). The
  no-arg full run and `--target system <ch>` keep the normal end-green gate
  (channel green). *Chosen over folding adapters into `--target test`: that would
  make the mob touch per-channel adapter code, breaking the "shared, channel-
  agnostic contract" ownership story the whole plan exists to express.*
- **D-external — RESOLVED: external-system rides in `--target test`.** The
  external-system driver ports/adapters (clock/erp/tax) + contract tests are
  channel-agnostic shared contract — conceptually identical to DSL/driver-port,
  which already ride in the shared slice. The mob owns them; this keeps
  `driver-adapter`/`system` purely about the API/UI channels. No separate target.
- **D-resume — RESOLVED: git-state-derived; see "Resume mechanism" below.** The
  detection contract is fully specified there (write-scope-as-footprint +
  ABSENT/DIRTY/DONE + first-non-DONE entry resolver). Code confirms the pivot:
  `driver.Run`→`RunProcess` always enters at the process `.Start` node (no
  start-at-node option), builds a fresh empty `Context` per run, and the
  `.gh-optivem/runs/<ts>/summary.jsonl` journal is *written but never read back*
  (forensic, machine-local). **Per-channel layout knob resolved:** channels are
  distinguished by **file/class naming** (e.g. `MyShopApiDriver` /
  `MyShopUiDriver`, per `channels.go`), not a `/api` `/ui` subdirectory, so
  channel-narrowing keys on filename within the layer's write-scope dir. Confirm
  against the `shop` testkit tree at execution time.
- **D-adapter-ownership — RESOLVED (2026-05-30): option A, channel team owns its
  driver adapter.** The mob owns only the **channel-agnostic** contract (tests +
  DSL core + driver **ports** + external ports); each channel team owns its own
  driver **adapter** *and* system. Chosen over option C (mob owns all adapters as
  test-harness; channel teams own only `system`) to keep the ownership boundary
  strict — a channel team owns *everything* channel-shaped, test-side adapter
  included — matching the Problem statement ("the API team then implements the
  API channel (driver adapter, then system)").
  - **Consequence — net-new adapter decomposition is in scope (the plan's biggest
    single piece).** Because the adapter step is today a single channel-agnostic
    node inside the RED cascade (see CORRECTION under "What already exists"),
    option A requires a 1702-style decomposition of the adapter step:
    1. **Make `implement-system-driver-adapters` channel-aware** — add a
       `channel` param and make the `system-driver-adapter-implementer` agent
       prompt write only that channel's adapter (mirrors how 1702 made
       `system-implementer` channel-aware).
    2. **Per-channel unroll for the adapter step** — extend/parallel
       `UnrollSystemChannels` so each channel gets its own adapter dispatch
       (linear, no loopback, same DAG discipline).
    3. **Move the adapter step out of the mob's RED `test` cascade** — so
       `--target test` stops at the driver **port** (ends port-deep / adapter-
       pending red per D-red-gate), and the per-channel adapter runs in the
       channel team's slice ahead of that channel's system.
  - This is acknowledged to be substantially larger than the original "small YAML
    seam-extraction" framing; the decomposition is sequenced **first** (it is the
    structural foundation the `--target` selector and resume detector build on).

## Resume mechanism (git-state-derived) — resolves D-resume

Resume is **not a new state store**; it is *computed from the committed tree on
every scoped run*. The handoff crosses clones, so the only durable cross-machine
signal is the **commit** — the `.gh-optivem/runs/` journal is forensic and
machine-local (records *what ran*, never *where to resume*, and never leaves the
mob's machine). Four parts, each reusing an existing primitive:

> **Non-goal — there is NO status file.** Do not introduce a `progress.json`, a
> `.gh-optivem/state`, a resume cursor, or any persisted "current phase" marker.
> The status store **is the git repository**: a phase's "done-ness" is read back
> out of its committed write-scope files each run (see below). A sidecar status
> file would be *wrong*, not just redundant — it lives on one clone and the
> handoff crosses clones, so a teammate pulling on another machine would see it
> absent or stale. The commit is the only thing that travels; therefore the
> commit is the only thing that may carry status. The `.gh-optivem/runs/`
> journal is written but **must stay read-only for resume** — it is diagnostics,
> not a cursor.

> **Content-addressed, not name-addressed.** Resolve DONE from the *committed
> write-scope files and their build/test state* — never by parsing commit
> messages, branch names, or tags. `git log --grep "system(api)"` is **forbidden**:
> it is a tempting shortcut that shatters the moment the operator changes commit
> conventions. Detection reads *which files are committed-clean and whether they
> compile/pass*, via `git ls-files` / `git status` on resolved paths — not *what
> the commit was called*. Commit **naming** is irrelevant to detection by design;
> commit **granularity** (one big commit vs per-slice) affects only the
> checkpoint/revert ergonomics (1702 D7), never correctness.

**1. A phase's `write:` scope IS its artifact footprint (reuse — do not invent a
map).** Every writing-agent MID already declares the layers it may modify in its
inline `write:` set on the `EXECUTE_AGENT` node, accessed via
`Engine.Scope(phase)` and resolved to physical paths against
`system-test.paths:` — exactly what `gh optivem process scope [<phase>]` already
prints. That resolved write-set *is* the set of files whose committed presence
proves the phase ran. No parallel phase→artifact table: the scope SSoT already
enumerates each phase's footprint. (Footprint by layer key: `--test` →
`at-test`, `dsl-port`, `dsl-core`, `driver-port` [+ `ct-test`,
`external-system-driver-port` if external rides here, per D-external];
`--driver-adapter <ch>` → `driver-adapter`; `--system <ch>` → `system-path`
[+ `system-db-migration-path` when `common: true`].)

**2. Three-state detection per phase (not a boolean).** Classify a phase's
resolved write-scope paths:
- **ABSENT** — paths empty/missing → phase not started.
- **DIRTY** — files present but uncommitted → in-progress *on this clone*; NOT a
  handoff point (the cross-clone artifact is the commit, so dirty ≠ done).
- **DONE** — files present, committed, **and** in the slice's expected
  build/test state (next point).

**3. Detection predicate = write-scope committed ∧ the slice's D-red-gate state —
one predicate, two directions.** "Committed + present" is too weak (a
half-written DSL port file is present but compiles-not), so DONE folds in the
slice's **D-red-gate** criterion: `--test` DONE = committed + compiles + tests
RED *for the right reason*; `--system <ch>` DONE = committed + channel green.
**D-red-gate and D-resume are therefore the same predicate evaluated on
different slices** — the gate asks "did *this* slice finish in its expected
state?", resume asks "did the *upstream* slice finish in its expected state?".
Implement it once.

**4. Entry resolver — "where we got up to" = first non-DONE phase.** Walk the
phase sequence (Test → DSL Port → DSL Core → Driver Port → Driver Adapter/`<ch>`
→ System/`<ch>`) in order; the resume entry point is the **first phase not
DONE**. Everything upstream is skipped because its outputs are committed in the
expected state. That is the entire "status" — there is no stored cursor, it is
derived each run.

**Channel narrowing.** For channel-split phases (`driver-adapter`, `system`) the
detection set is the layer's write-scope **narrowed to the channel's subtree**,
reusing the same channel→path-segment derivation as the 1702 codegen. *Confirm
the physical per-channel layout against the scaffolded testkit tree* — the one
genuinely-open detail.

**Mechanism (resolved against the code — supersedes the earlier "enter at an
arbitrary node" sketch).** An earlier draft proposed `Options.StartPhase` + an
arbitrary-node `RunProcess` + a state re-seed for skipped phases, and called the
re-seed "the main implementation risk." Reading the engine showed a simpler,
lower-risk path that the sketch missed:

- `RunProcess(name, ctx)` **already enters any process by name** — the pipeline
  is a *tree of named sub-processes*, not one flat node graph. `Context.State` is
  shared across the whole run; `Context.Params` are merged on call-activity entry
  and restored on exit (`statemachine/run.go` `wrapCallActivity`). The top-level
  `main` is a thin bootstrap (`START → IMPLEMENT_TICKET → END`) with no phase
  picker; the phase ladder lives inside `write-and-verify-acceptance-tests`.
- So a `--target` slice is **composed by name**: extract the plan's slices as
  named sub-processes at the seams the plan wants (a small `process-flow.yaml`
  refactor — see Item 2a), then call `RunProcess` on the selected one. No
  start-node knob, **no stop-node knob** (the old sketch's arbitrary-node entry
  would have *also* needed a stop-node to truncate `--target test` before the
  adapter gates — it never scoped one).
- **The re-seed risk evaporates.** Entering a slice by name never traverses the
  upstream gates, so the `*-changed` gateway flags those gates read are simply
  not in play — nothing to reconstruct. A slice's only inputs are config-derived
  (`channel`, `common`, scope) + the issue, all seeded today by `seedScopeState`
  / `preResolveIssue` / call-activity params.

The remaining genuinely-net-new work is the **YAML seam-extraction** plus the
**tree-state resume detector + entry resolver** — both additive, neither touching
the single-linear-walk internals the old sketch would have rewritten.

**Payoff — the ordering constraints become *checked*, not positional.** Today
"system can't start until DSL + driver-port exist" and "UI can't start until API
landed" hold only because one continuous walk visits phases in order. With
git-state detection + the entry resolver, the resolver **refuses** to enter
`System/<ch>` unless DSL + driver-port detect DONE, and refuses `System/ui`
unless the common layer (built on the first channel per D-common option b)
detects DONE. Preconditions are verified against the committed tree, not assumed
from pipeline position. (Under D-common option a the common layer lands in the
`--test` slice, so the System-channel slices have no inter-channel precondition
to check — only the shared upstream one.)

## Items

1702 has landed (unblocked). All design decisions are resolved, including
**D-adapter-ownership = option A** (channel team owns its driver adapter), which
makes Item 1 substantially larger than first framed. Execute in order:

0. **Adapter decomposition (D-adapter-ownership option A) — FOUNDATION, do
   first.** The structural prerequisite for everything below. Per D-adapter-
   ownership: (0a) add a `channel` param to `implement-system-driver-adapters` and
   make the `system-driver-adapter-implementer` agent prompt write only that
   channel's adapter; (0b) per-channel unroll of the adapter step
   (extend/parallel `UnrollSystemChannels`, linear DAG, no loopback); (0c) move
   the adapter step out of the RED `write-and-verify-acceptance-tests` cascade so
   `--target test` stops at the driver port. Audit statemachine fixtures + watch
   RAM (loop hazard) since `channels.go` + `process-flow.yaml` both change.
1. **`--target` → slice mapping.** Define, per `--target` value, the named
   sub-process(es) it runs: `test` → the shared-contract slice (tests + DSL
   port/core + driver port + external, D-external; adapter step now removed per
   item 0c); `driver-adapter` → that channel's (now per-channel) adapter slice;
   `system` → that channel's system slice (+ common on the first channel).
   Encode the channel-agnostic vs channel-split split.
2. **Scoped entry via compose-by-name + git-state resume (D-resume).**
   **Mechanism resolved against the code (supersedes the earlier "arbitrary-node
   entry" sketch — see "Mechanism" below).** The pipeline is already a tree of
   named sub-processes that `RunProcess(name, ctx)` enters cleanly (`State`
   shared across the run, `Params` scoped/restored per call-activity in
   `wrapCallActivity`). So a `--target` slice is **selected by name**, not by
   starting a single linear walk partway through. Build:
   (a) a **small YAML seam-extraction** in `process-flow.yaml` exposing the
   plan's slices as named sub-processes at the seams the plan wants — a
   `shared-contract` slice (test → DSL core → driver port + external, the
   `write-and-verify-acceptance-tests` cascade truncated *before* the adapter
   gates), and the already-per-channel `driver-adapter-<ch>` / system slices
   (the 1702 unroll already splits the system step per channel);
   (b) a thin **`--target` → sub-process selector** that calls `RunProcess` on
   the chosen slice (no `Options.StartPhase`, no arbitrary-node entry);
   (c) the per-phase write-scope→footprint detector (reuse `Engine.Scope` /
   `process scope`, no new artifact map);
   (d) the ABSENT/DIRTY/DONE classifier + first-non-DONE entry resolver that
   refuses a slice whose upstream slice is not DONE.
   The brittle "re-seed skipped phases' gateway flags" step from the old sketch
   is **gone**: entering a slice by name does not traverse the upstream gates, so
   there is nothing to fake. The only state a slice needs is config-derived
   (`channel`, `common`, scope) + the issue, all already seeded by
   `seedScopeState` / `preResolveIssue` / the call-activity params. Inspect
   committed tree state only — never the machine-local `.gh-optivem/runs/`
   journal.
3. **Per-slice success gate (D-red-gate).** Add the "expected-red" success
   criterion for `--target test` and `--target driver-adapter <ch>`; keep
   end-green for the no-arg full run and for `--target system <ch>` (channel
   green). **This is the same predicate Item 2's detector uses to classify a
   phase DONE** — implement once, evaluate on this slice (gate) and on upstream
   slices (resume).
4. **Flag surface (D-flags, D-positional).** Wire `--target
   test|driver-adapter|system` + `--channel <ch>` into `implement_commands.go`:
   `--channel` required for `driver-adapter`/`system`, rejected for `test`,
   validated against `channels:` (parity with flag/interactive validation per the
   1702 plan's D2). Accept a positional issue arg (additive; `--issue` still
   works). Preserve the no-arg full-walk default.
5. **Common-layer ownership wiring (D-common option b).** First channel's
   `--target system` carries `common: true`, later channels `common: false` —
   reusing the 1702 plan's `common` param verbatim, consistent with its D5/D7.
6. **`--help` + Example refresh.** Update the `implement` `Long`/`Example`
   strings to show the team workflow and the unchanged no-arg default. Use
   `myorg/myrepo` placeholders, never `shop`-specific repos.
7. **Tests.** Slice→phase mapping, git-state resume detection, expected-red
   gate, `<ch>` validation. Audit gate/statemachine fixtures before running the
   statemachine tests and watch RAM (statemachine loop hazard).

## Do NOT

- **Do not change the no-arg `implement <issue>` behaviour.** The `--target`
  flag is a refinement; omitting it must walk the full pipeline exactly as today.
- **Do not add CODEOWNERS / permission enforcement here.** The ownership split
  is the *informal agreement* (who runs which scoped command). Enforcement is
  the separately-deferred `plans/20260530-1721-codeowners-channel-team-ownership.md`.
- **Do not hardcode `api`/`ui`.** The `--channel` token comes from the
  `channels:` SSoT (1702 plan), validated in one place.
- **Do not add a diagram-regeneration step** if `process-flow.yaml` is touched
  (the regenerate-diagram GH Actions workflow races a local regen).
- **Do not persist a resume status file.** No `progress.json`, no
  `.gh-optivem/state`, no "current phase" cursor — the status store IS the git
  tree (committed write-scope files), read back each run. A sidecar marker lives
  on one clone and breaks the cross-clone handoff. The `.gh-optivem/runs/`
  journal stays read-only for resume (diagnostics only). See the "Resume
  mechanism" Non-goal.
- **Do not detect status by parsing commit messages, branches, or tags.**
  Detection is **content-addressed**: resolve DONE from committed write-scope
  files + their build/test state (`git ls-files` / `git status` on resolved
  paths), never `git log --grep`. Commit naming is the operator's to change
  freely; resume must not depend on it.

## Related

- `plans/20260530-1702-channels-field-channel-by-channel.md` — `channels:` SSoT
  + automatic channel-by-channel system implementation. **Shares the channel
  axis and the common-layer-ownership decision (D5/D7 ↔ D-common), including the
  `common` param.** `--target system --channel <ch>` is the manual per-team
  counterpart of that plan's caller-side auto-unroll; the two agree on
  common-layer ownership (both use option (b): common on the first channel).
- `plans/20260530-1721-codeowners-channel-team-ownership.md` — deferred
  enforcement variant of the same team split.

## Verification (operator)

- `gh optivem implement 7` (no `--target`) walks the full pipeline and ends
  green — unchanged from today.
- `gh optivem implement 7 --target test` produces tests + DSL + driver-port, ends
  RED by design, one commit; a second clone can pull that commit and run
  `--target driver-adapter --channel api` / `--target system --channel api`
  without redoing the shared slice.
- A project with `channels: [api]` rejects `--target system --channel ui` with a
  channels-validation error (token not declared).
- `--target system` with no `--channel` errors; `--target test --channel api`
  errors (channel-agnostic slice).
