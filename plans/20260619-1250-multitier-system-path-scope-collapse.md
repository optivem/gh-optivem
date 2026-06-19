# Plan: fix the multitier `system-path` write-scope collapse in implement/update/refactor-system ({20260619-1250})

✅ **DECISIONS RESOLVED — ready to execute.** Sibling/follow-up to
`plans/20260619-1120-multitier-system-surface-placeholder-crash.md` (the
prompt-render leak, **landed** commit `7854e8f`). This plan closes the **second**
leak of the same monolith-only `system-path` assumption — the **scope-resolution**
side — surfaced by the very next node in the shop #72 multitier rehearsal once
the render crash was fixed. All fixes in **`gh-optivem`**.

## TL;DR

**Why:** On multitier, the writing-agent phases that scope `system-path`
(`implement-system`, `update-system`, `refactor-system`, `fix-unexpected-*-tests`)
have their production-surface write-scope silently collapse: `ResolveLayerPaths`
**skips** `system-path` on multitier (it's monolith-only), so the agent's correct
backend/frontend tier writes land **outside scope** → `scope-diff` → fix declined
→ run halts. Shop #72 dies here after ~27m, one node past the render crash we just
fixed.
**End result:** one kernel helper `cfg.SystemSurfacePaths(channel)` becomes the
single source of the architecture+channel→surface mapping (`api`→backend,
`ui`→frontend, whole-system→both); the scope check resolves `system-path` on
multitier through it (backend writes on an `api` dispatch are now in scope),
the landed `driver.resolveSystemSurface` is refactored onto it (mapping no longer
lives twice), and preflight reuses the *same* resolver to assert every declared
scope layer resolves to ≥1 path — turning this whole bug class into a <1s static
failure instead of a 27-minute live halt.

**What the user observes:** shop #72 (and any multitier story) passes
`validate-outputs-and-scopes` at `implement-system` and proceeds, instead of
halting on `scope-diff`; an `api` dispatch may write `backend/` (not `frontend/`),
a `ui` dispatch `frontend/`, and whole-system updater/refactorer both — precision
preserved. A misconfigured-or-collapsing multitier scope now fails at preflight in
<1s with `MID <name> <read|write> scope: resolves to no paths …`.

**Explicitly unchanged:** `ResolveLayerPaths`' `MonolithOnlyPathKeys` skip and its
drift tests (the tier surface is *appended* after the channel narrow, not folded
into the index-aligned base resolve); monolith scope resolution; the
`gh-optivem.yaml` schema. **Deferred (D-D):** a preflight prompt-*render* gate —
its own follow-up plan; the render-matrix unit test (`7854e8f`) remains the
near-term guard for prompt-placeholder drift.

## ▶ Next executable step (resume here)

**All agent items (1–5) are landed** — kernel helper `cfg.SystemSurfacePaths`,
`actions.AddSystemSurfaceScope` wired at both scope-check sites, the preflight
per-layer non-empty gate, the `driver.resolveSystemSurface` DRY refactor, and the
full test set (kernel table test, `AddSystemSurfaceScope` table test, #72
multitier scope regression, preflight collapse + real-engine-passes,
`TestResolveSystemSurface` still green). No mechanical edits remain.

What's left is **operator-driven** (see `## Verification`) and **follow-up
authoring**, not `/execute-plan` work:
1. Operator: re-run the #72 rehearsal + the rehearsal loop (Verification bullets).
2. Draft the D-D follow-up plan `plans/<ts>-preflight-dispatch-dry-run.md`
   (use `/create-plan`) — the unified "dispatch dry-run" preflight; cross-reference
   this plan.
3. Memory: update [[reference_system_path_monolith_only_resolver]] to record the
   scope-resolution leak as closed (third site) + the preflight non-empty gate as
   the static guard.

---

## The error (rehearsal `rehearsal-20260619-120336-72`, run `20260619-100352`)

The render crash is **gone** — `system-implementer` rendered, ran (4m41s, opus),
wrote correct multitier backend code, and **compiled**. The run now halts one node
later at `VALIDATE_OUTPUTS_AND_SCOPES`:

```
validate-outputs-and-scopes: 5 path(s) outside scope [system-path system-db-migration-path]:
  out-of-scope: system/multitier/backend-java/.../core/services/OrderService.java   (+4 more)
→ outputs-and-scopes-valid=false  failure-kind=scope-diff  failing-task-name=implement-system
→ FIX → human declined → "Fix Declined — Run Halted"
```

The migration file (`system/db/migrations/V…__add_shipping_fee_to_orders.sql`)
passed; the five backend-tier files did not.

## Root cause (the scope-resolution leak of the monolith-only `system-path`)

| # | Where | Gap |
|---|---|---|
| R1 | `actions/scope.go:163-165` `ResolveLayerPaths` | On multitier it **skips** any `MonolithOnlyPathKeys` layer (`system-path`) — returning success with **no path** for it. So `implement-system`'s `write: [system-path, system-db-migration-path]` collapses to just `[system/db/migrations]`. The production surface (the tier the agent must write) is **not in scope**. |
| R2 | No channel→tier scope resolution | `narrowAdapterScopeByChannel` (`scope.go:210`) already proves the channel is available at both checks (`ctx.Params["channel"]`) and that per-channel scope narrowing is an established pattern — but there is **no equivalent** that resolves `system-path` to the channel's tier. The mapping (`api→backend, ui→frontend`) exists now **only** in `driver.resolveSystemSurface` (for the prompt), not for the scope check. |
| R3 (latent) | `update-system` / `refactor-system` / `fix-unexpected-*-tests` | Same `system-path` write-scope, same collapse. These are whole-system (no channel) → their multitier surface is **both** tiers. They crash identically on the next multitier redesign/refactor/fix-loop dispatch. |
| R4 (detection gap) | `preflight.go:280` `runScopeResolutionChecks` | Already runs **every** writing-agent MID's `read:`/`write:` list through `ResolveLayerPaths` — but only flags a resolution **error**. The R1 skip turns the empty resolution into a non-error, so the sweep reports **success** on a scope that collapsed to nothing. A statically-determinable defect (config + MID scope + architecture matrix) was left to a 27-minute live dispatch. |

**Why so late (answering "could preflight have caught it?"):** yes — the sweep
machinery already exists; it missed this only because it asserts *"resolves
without error,"* and the monolith-only **skip** (added originally to fix a
preflight *false positive* — see [[reference_system_path_monolith_only_resolver]])
silently swallows the empty result. Two compounding reasons it escaped: (a) this
is a **second subsystem** (scope-resolution) carrying the same assumption the
[[project_multitier_system_surface_prompt_crash]] fix closed only in
prompt-render, so fixing one didn't reveal the other; (b) shop only recently
became multitier and #72 is the **only** story that drives
`change-system-behavior` all the way to `implement-system`
([[project_bpmn_full_coverage_story_and_realkind_gap]]), so nothing exercised the
multitier write-scope until a live run. Item 3 closes the detection gap.

---

## Resolved decisions

**D-A → A1 (append the tier surface in the scope check, channel-aware).** A new
`addSystemSurfaceScope(write, allowed, channel, cfg)` runs **after**
`narrowAdapterScopeByChannel` at both checks: on multitier, when `system-path` ∈
`write`, it **appends** the channel's tier path (`api→System.Backend.Path`,
`ui→System.Frontend.Path`) or **both** tier paths when no channel (whole-system).
Unknown channel on multitier → hard error (mirrors `narrowAdapterScopeByChannel`'s
strict "no silent widen"). Monolith → no-op (`system-path` already resolved by
`ResolveLayerPaths`); `system-path` absent from `write` → no-op.

- *Chosen over folding the tier resolution into `ResolveLayerPaths` itself*
  because that resolver is **channel-blind** (shared with preflight) and its
  output is **index-aligned** with the `write` list — making one layer resolve to
  *two* tier paths would break the alignment `narrowAdapterScopeByChannel`
  depends on. Appending after the narrow is alignment-safe and keeps
  `ResolveLayerPaths` + the existing `MonolithOnlyPathKeys` skip (and their drift
  tests) untouched.
- *Chosen over rewriting the MID `write:` lists in `process-flow.yaml`* (e.g.
  `write: [backend-path]`) because the channel→tier choice is **per-dispatch** on
  a **shared** node (`api` and `ui` reuse one `implement-system` MID), so it
  cannot be expressed statically; and the tier keys aren't Family A scope keys.
- A1 deliberately mirrors `driver.resolveSystemSurface` (same `{api:backend,
  ui:frontend}` table, same whole-system→both rule) so the prompt surface and the
  scope surface stay in lockstep.

**D-B → B1 (tighten preflight to per-layer non-emptiness).**
`runScopeResolutionChecks` resolves each declared `read:`/`write:` layer and, on
the loaded config (channel-blind: apply `addSystemSurfaceScope` with `channel=""`),
fails any layer that collapses to **zero** paths. After A1, `system-path` on
multitier resolves to both tiers (non-empty) and passes; a future re-introduced
skip / misconfigured tier fails in <1s. This is the scope-side analogue of the
render-matrix test landed in `7854e8f`. *Deferring detection (B2) would leave the
next architecture-shaped scope collapse to another live rehearsal.*

**D-C → single kernel resolver (no duplicated channel→tier mapping).** The
`{api:backend, ui:frontend}` + whole-system→both mapping becomes **one** kernel
helper `projectconfig.Config.SystemSurfacePaths(channel string) ([]string, bool)`
returning the structured surface path list (`ok=false` for unknown channel /
unsupported architecture). Every consumer formats for its own need: the **scope
check** (`addSystemSurfaceScope`) appends the paths; **preflight** checks
non-empty; the **driver prompt** (`resolveSystemSurface`, landed `7854e8f`) is
refactored into a thin formatter that joins the list to the `"backend/ and
frontend/"` display string. Chosen over per-package copies because the mapping is
domain knowledge about the config and belongs with it (kernel is the lowest
layer, no import cycle); kills the "mapping lives twice" drift. Preflight and the
runtime scope check already share `ResolveLayerPaths` — this extends that
"one resolver, many callers" rule to the surface mapping too.

**D-D → render-preflight is a separate follow-up, not bundled here.** A preflight
gate that *renders* every agent prompt against the real loaded config (catching
any unfilled placeholder early, the original `${system-path}` render-crash class)
is worthwhile but is a distinct subsystem (prompt-render, not scope-resolution)
and a larger build (it must share the unit test's per-dispatch seed harness to
avoid duplication). The long-term-clean shape is a **unified "dispatch dry-run"
preflight** that runs *both* checks per writing-agent MID — scope resolves
non-empty **and** prompt renders with no unfilled placeholder — through the same
shared resolver + seed harness the render-matrix unit test uses. Captured as a
carry-forward follow-up plan (see Notes); the render-matrix unit test landed in
`7854e8f` remains the near-term guard for prompt-placeholder-vs-architecture
drift (its correct, cheapest home — that class is a prompt-authoring bug, not an
operator-config one). This plan stays scoped to the scope-collapse + scope-detection.

---


## Verification

(Operator-driven — not agent `## Items` work.)

- `bash scripts/test.sh ./internal/atdd/... ./internal/kernel/projectconfig/` green; new resolver/regression/preflight tests pass; existing `ResolveLayerPaths` / `narrowAdapterScopeByChannel` drift tests still pass.
- Re-run the failing rehearsal: `bash scripts/atdd-rehearsal.sh 72` against the multitier shop config — `implement-system` passes `validate-outputs-and-scopes` (backend writes in scope) and proceeds instead of halting on `scope-diff`.
- Re-run `bash scripts/atdd-rehearsal-loop.sh` so #72 clears the corpus through GREEN.

---

## Notes (resolved + carry-forward)

- **Mapping lives once (D-C).** The `{api:backend, ui:frontend}` + whole-system→both
  mapping is the single kernel helper `cfg.SystemSurfacePaths`; the driver prompt
  (Item 4), the scope check (Item 2), and preflight (Item 3) all call it. No
  per-package copy.
- **Follow-up: unified "dispatch dry-run" preflight (D-D).** A future plan should
  add a preflight pass that, per writing-agent MID, runs **both** static checks —
  scope resolves non-empty (this plan) **and** the prompt renders with no unfilled
  placeholder — through the same shared resolver + the render-matrix unit test's
  seed harness (one harness, two callers). It would catch the original
  `${system-path}` render-crash class against the operator's *real* config at
  preflight, generalising "discover early" to any agent placeholder. Out of scope
  here (distinct subsystem + larger build); the render-matrix unit test landed in
  `7854e8f` is the near-term guard. Write it as `plans/<ts>-preflight-dispatch-dry-run.md`
  cross-referencing this plan.
- **Microservices (carry-forward).** `System.BackendServices` is a third surface
  shape; the unknown-channel→error branch fail-fasts cleanly rather than
  panicking. Out of scope for the fix.
- **Memory follow-up.** After landing, update
  [[reference_system_path_monolith_only_resolver]] to record the scope-resolution
  leak as closed (third closed leak site), and note the preflight per-layer
  non-empty gate as the static guard.
