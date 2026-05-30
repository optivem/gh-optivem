# Scoped `implement`: layer/channel slices as the team-handoff seam

**Status:** proposed
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

### Sketched ergonomics (operator's vision)

```
# Whole team, mob-programming at product level — the SHARED red contract:
gh optivem implement 7 --test               # Test + DSL Port + DSL Core + Driver Port

# API team (backend):
gh optivem implement 7 --driver-adapter api
gh optivem implement 7 --system api

# UI team (frontend):
gh optivem implement 7 --driver-adapter ui
gh optivem implement 7 --system ui

# Fullstack developer, start to end (UNCHANGED current behaviour):
gh optivem implement 7
```

The scoped flags are **refinements of the default**, never a separate mode:
omit them and you get the full walk exactly as today.

## Why the slices fall on real seams (not invented ones)

The four-layer ATDD stack is **Test → DSL Port → DSL Core → Driver Port →
Driver Adapter → System**. Two of those layers are channel-agnostic (shared)
and two are channel-split:

| Slice | Layers produced | Channel? | Owner | End state |
| --- | --- | --- | --- | --- |
| `--test` | Test, DSL Port, DSL Core, Driver Port | agnostic | mob / whole team | **RED by design** (no system yet) |
| `--driver-adapter <ch>` | Driver Adapter `/<ch>` | per-channel | that channel's team | compiles, still red |
| `--system <ch>` | System `/<ch>` (+ the common layer, see D-common) | per-channel | that channel's team | channel green |

This is the **same channel axis** as the open channels plan
(`plans/20260530-1702-channels-field-channel-by-channel.md`), and the same
per-slice-commit philosophy. `--system <ch>` is the **manual, per-team**
counterpart to that plan's **automatic** channel-by-channel unroll: same
decomposition, two drivers.

## What already exists (reuse, do not reinvent)

- **The phases the slices map onto already exist as distinct writing-agent
  MIDs.** `acceptance-test-writer`/`contract-test-writer` (tests + DSL stubs),
  `dsl-implementer` (writes `dsl-core`, `driver-port`,
  `external-system-driver-port`), `system-driver-adapter-implementer` (per
  channel), `system-implementer` (system code). A slice is a contiguous run of
  these, not a new pipeline.
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

## Open decisions (the real design work — resolve before/at execution)

- **D-common — who owns the channel-agnostic *common* layer under
  parallelism?** (the hard one) Driver adapters and system code are
  channel-separated, so API/UI work in parallel without merge conflict. But the
  **common layer** (DTO / entity / service / migration) is channel-agnostic and
  gets written during `--system`. (Term is **common**, never "core" — "core"
  collides with **DSL Core**; this matches the 1702 plan's D5 naming.) The 1702
  plan's D5 resolves this for the *sequential* auto-unroll with a **`common`
  boolean param**: `common: true` on the **first** channel (build the common
  layer + that channel's adapter), `common: false` after (adapter delta only).
  That assumes sequential channels — truly parallel `--system api` /
  `--system ui` runs would either both set `common: true` (conflict + double
  migration) or force a soft ordering. Two candidate resolutions — **CONFIRM
  with operator**:
  - (a) **Pull the common-layer skeleton into the `--test`/mob slice** — the mob
    agrees DTO/entity/service/migration too, so both `--system <ch>` runs are
    `common: false` channel deltas and genuinely parallel. (Widens `--test`
    beyond "test side".)
  - (b) **`--system <first-channel>` carries `common: true`**, the other is a
    `common: false` delta with a documented soft dependency on it. (Keeps
    `--test` test-only; sacrifices full parallelism. Reuses 1702's `common`
    param verbatim.)
  This decision must stay **consistent with the 1702 plan's D5/D7** — the two
  plans share the common-layer-ownership question and the `common` param.
- **D-resume — resume is git-state-derived, NOT run-log-derived.** The mob, API
  team, and UI team are on different machines/clones. The handoff artifact is
  the **committed branch**, so a scoped run must infer "tests + DSL +
  driver-port already exist and are committed" by **inspecting the working
  tree**, not by reading a local `.gh-optivem/runs/` journal (which lives only
  on the mob's machine). Today `implement` always walks from START with no
  resume — confirmed in code: `driver.Run`→`RunProcess` always enters at the
  process `.Start` node (no start-at-node option), builds a fresh empty
  `Context` per run, and the `.gh-optivem/runs/<ts>/summary.jsonl` journal is
  *written but never read back* (forensic, machine-local). This is a real pivot.
  **The detection contract is now specified in the "Resume mechanism" section
  below** (write-scope-as-footprint + ABSENT/DIRTY/DONE + first-non-DONE entry
  resolver); the one remaining knob is the per-channel physical subtree layout.
- **D-red-gate — a scoped slice can succeed while RED.** `--test` ends red by
  design; `--driver-adapter <ch>` alone cannot go green (no system). These
  slices need a success criterion other than "tests green": compiles + expected
  wiring present + failing for the *right* reason (not a compile error). This
  criterion is **the same predicate the resume detector uses to classify an
  upstream slice DONE** (see "Resume mechanism") — build it once, evaluate it on
  *this* slice (the success gate) and on *upstream* slices (resume detection).
  The default no-arg run keeps the normal end-green gate.
- **D-flags — flag taxonomy.** Operator sketch is three verbs (`--test`,
  `--driver-adapter <ch>`, `--system <ch>`). Alternative: one selector
  `--layer test|driver-adapter|system` + `--channel <ch>`, which composes with
  the `channels:` SSoT and adds fewer surface symbols. The verb form reads more
  naturally for humans. CONFIRM preferred shape. (Either way `--channel`/`<ch>`
  validates against `channels:`.)
- **D-positional — `implement 7` vs `implement --issue 7`.** The sketch uses a
  positional issue number; today `--issue` is a required flag. Decide whether to
  accept a positional issue arg (additive, `--issue` still works) as part of
  this ergonomics pass or keep it out of scope.
- **D-external — external-system slice.** The stack also has external-system
  driver adapters (clock/erp/tax) + contract tests, mostly channel-agnostic.
  Decide whether they ride in the `--test`/mob slice or get their own scope;
  don't leave them unaddressed.

## Resume mechanism (git-state-derived) — resolves D-resume

Resume is **not a new state store**; it is *computed from the committed tree on
every scoped run*. The handoff crosses clones, so the only durable cross-machine
signal is the **commit** — the `.gh-optivem/runs/` journal is forensic and
machine-local (records *what ran*, never *where to resume*, and never leaves the
mob's machine). Four parts, each reusing an existing primitive:

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

**Driver change required (net-new — confirmed absent today).** `RunProcess`
always enters at `.Start`. Resume needs: (a) an `Options.StartPhase` (resume
entry) field; (b) `RunProcess` able to enter at an arbitrary node; (c) **state
re-seed** — the skipped upstream phases never set their in-run `Context`
state/params (`channel`, `common`, the cumulative-verify scope, any gateway flag
the entry phase reads), so those must be seeded from config + the tree-detection
result instead of from a prior phase's execution. (c) is the fiddly part and the
main implementation risk.

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

## Items (sequence once decisions land)

1. **Slice → phase-range mapping.** Define, per slice flag, the contiguous set
   of writing-agent MIDs it runs (reuse existing MIDs; no new pipeline).
   Encode the channel-agnostic vs channel-split split explicitly.
2. **Scoped entry + git-state resume (D-resume).** Build the four-part mechanism
   in the "Resume mechanism" section: (a) the per-phase write-scope→footprint
   detector (reuse `Engine.Scope` / `process scope`, no new artifact map); (b)
   the ABSENT/DIRTY/DONE classifier + first-non-DONE entry resolver; (c) the
   driver `Options.StartPhase` entry + arbitrary-node `RunProcess`; (d) the
   upstream-state re-seed for skipped phases (`channel`, `common`,
   cumulative-verify scope, gateway flags) from config + detection result.
   Inspect committed tree state only — never the machine-local
   `.gh-optivem/runs/` journal.
3. **Per-slice success gate (D-red-gate).** Add the "expected-red" success
   criterion for `--test` and `--driver-adapter <ch>`; keep end-green for the
   no-arg full run and for `--system <ch>` (channel green). **This is the same
   predicate Item 2's detector uses to classify a phase DONE** — implement once,
   evaluate on this slice (gate) and on upstream slices (resume).
4. **Flag surface (D-flags, D-positional).** Wire the chosen flag taxonomy into
   `implement_commands.go`; validate `<ch>` against `channels:` (parity with
   flag/interactive validation per the 1702 plan's D2). Preserve the no-arg
   full-walk default.
5. **Common-layer ownership wiring (D-common).** Implement whichever resolution
   the operator picks, reusing the 1702 plan's `common` param and kept
   consistent with its D5/D7.
6. **`--help` + Example refresh.** Update the `implement` `Long`/`Example`
   strings to show the team workflow and the unchanged no-arg default. Use
   `myorg/myrepo` placeholders, never `shop`-specific repos.
7. **Tests.** Slice→phase mapping, git-state resume detection, expected-red
   gate, `<ch>` validation. Audit gate/statemachine fixtures before running the
   statemachine tests and watch RAM (statemachine loop hazard).

## Do NOT

- **Do not change the no-arg `implement <issue>` behaviour.** Scoped flags are
  refinements; omitting them must walk the full pipeline exactly as today.
- **Do not add CODEOWNERS / permission enforcement here.** The ownership split
  is the *informal agreement* (who runs which scoped command). Enforcement is
  the separately-deferred `plans/20260530-1721-codeowners-channel-team-ownership.md`.
- **Do not hardcode `api`/`ui` per flag.** The channel token comes from the
  `channels:` SSoT (1702 plan).
- **Do not add a diagram-regeneration step** if `process-flow.yaml` is touched
  (the regenerate-diagram GH Actions workflow races a local regen).

## Related

- `plans/20260530-1702-channels-field-channel-by-channel.md` — `channels:` SSoT
  + automatic channel-by-channel system implementation. **Shares the channel
  axis and the common-layer-ownership decision (D5/D7 ↔ D-common), including the
  `common` param.** `--system <ch>` is the manual per-team counterpart of that
  plan's caller-side auto-unroll; the two must agree on common-layer ownership.
- `plans/20260530-1721-codeowners-channel-team-ownership.md` — deferred
  enforcement variant of the same team split.

## Verification (operator)

- `gh optivem implement 7` (no flags) walks the full pipeline and ends green —
  unchanged from today.
- `gh optivem implement 7 --test` produces tests + DSL + driver-port, ends RED
  by design, one commit; a second clone can pull that commit and run
  `--driver-adapter api` / `--system api` without redoing the shared slice.
- A project with `channels: [api]` rejects `--system ui` with a
  channels-validation error (token not declared).
