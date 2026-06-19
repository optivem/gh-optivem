# Plan: fix the multitier `${system-path}` render crash in the GREEN/redesign/refactor system prompts ({20260619-1120})

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-06-19T09:55:12Z`

## TL;DR

**Why:** The GREEN/redesign/refactor system prompts hardcode the monolith-only `${system-path}`, which is empty on a multitier repo — so shop #72's multitier rehearsal crashes at prompt render (`unresolved placeholder ${system-path}`) after 27m of green phases.
**End result:** The driver resolves one architecture-agnostic `${system-surface}` placeholder per dispatch (monolith→`System.Path`; multitier `api`→backend, `ui`→frontend, whole-system→both tiers), the three prompts name `${system-surface}` instead, and a render-matrix unit test catches any future multitier-only prompt drift in <1s.

## ▶ Next executable step (resume here)

All four agent Items are landed and the test suite is green. **Only operator-driven verification remains** (see `## Verification` below): re-run the failing multitier shop #72 rehearsal and the rehearsal-loop to confirm `implement-system` renders past `RUN_AGENT`, and spot-check a rendered `system-implementer` prompt log shows `backend` on `api` / `frontend` on `ui`. Plus the post-landing memory follow-up in `## Notes`. No further agent edits.

✅ **DECISIONS RESOLVED — ready to execute.** Records the 2026-06-19 shop #72
multitier rehearsal crash and the agreed orchestrator-side fix (A1 + B1). All
fixes are in **`gh-optivem`** (driver + agent prompts + tests), not in `shop`.

## Target state

When this lands, the GREEN/redesign/refactor system prompts no longer hardcode
the monolith-only `${system-path}`. The driver resolves a single
**`${system-surface}`** placeholder per dispatch, so the same prompt renders
correctly on every architecture.

**End logic — how `${system-surface}` resolves (filled in `driver.go`, skipped if empty so a bad dispatch still fail-fasts):**

| Architecture | Dispatch | `${system-surface}` resolves to |
|---|---|---|
| monolith | any | `System.Path` (e.g. `system`) |
| multitier | `channel=api` | `System.Backend.Path` (e.g. `backend`) |
| multitier | `channel=ui` | `System.Frontend.Path` (e.g. `frontend`) |
| multitier | whole-system (no channel — updater/refactorer) | both tiers joined (e.g. `backend/ and frontend/`) |
| multitier | unknown channel | *(unfilled → `findUnfilledPlaceholders` fail-fast — never silent `""`)* |

The channel→tier mapping `{api: backend, ui: frontend}` is pinned in Go (A1); the
config schema is **unchanged**. The mapping reads off the existing
`System.Backend` / `System.Frontend` tiers already in `gh-optivem.yaml` — no new
config keys. Concretely, the multitier shop config the fix consumes is just
today's:

```yaml
system:
  architecture: multitier
  backend:  { path: backend,  repo: …, lang: … }   # ← api channel surface
  frontend: { path: frontend, repo: …, lang: … }   # ← ui  channel surface
  # system.path stays empty on multitier (monolith-only, by construction)
```

**What the user observes:** shop #72 (and any multitier story) renders the
`implement-system` / `update-system` / `refactor-system` prompts and proceeds
past `RUN_AGENT` instead of crashing with `unresolved placeholder ${system-path}`;
a rendered `system-implementer` prompt log shows `backend` on the `api` dispatch
and `frontend` on `ui`. A new render-matrix unit test (`[monolith, multitier] ×
every prompt`) makes any future multitier-only prompt drift fail in CI in <1s
instead of mid-rehearsal.

**Explicitly unchanged:** the `gh-optivem.yaml` schema; `PlaceholderMap`'s
existing `system-path` emission (monolith prompts still work); the empty-skip +
`findUnfilledPlaceholders` fail-fast behaviour (kept as the safety net);
`${system-db-migration-path}` (architecture-agnostic, untouched); all non-system
prompts (driver-adapter implementers already use Family B keys).

---

Sibling / cross-ref: [[reference_system_path_monolith_only_resolver]] recorded
the *scope-resolver* half of the monolith-only `system-path` assumption (layer/
path resolvers skip it on multitier). This plan addresses a **second, separate**
place the same assumption leaks: the **prompt-render** path. Closing the
resolver side did not cover prompt bodies.

---

## The error, and how we found it

**Symptom (rehearsal-loop, worktree
`rehearsal-20260619-102123-72-charge-shipping-based-on-product-weight`, run
`20260619-082131`):** after 27m of green upstream phases, the very next agent
crashed at prompt render — a runtime error, **not** a test failure:

```
process "main" → IMPLEMENT_TICKET → CHANGE_SYSTEM_BEHAVIOR
  → IMPLEMENT_AND_VERIFY_SYSTEM_API → implement-system → EXECUTE_AGENT → RUN_AGENT
FAIL RUN_AGENT -> clauderun: render prompt: unresolved placeholder ${system-path}
```

The shop repo is now **multitier** (backend + frontend tiers), and the crash is
on the `api` channel dispatch of `system-implementer`.

**How we traced it (the read trail):**

1. **The prompt** — `internal/atdd/assets/runtime/agents/atdd/system-implementer.md:8,27`
   names the production surface as `${system-path}` ("production code under the
   system surface (`${system-path}`)"). The prompt was authored monolith-first.
2. **The fill source** — `${system-path}` has exactly one source,
   `internal/kernel/projectconfig/config.go:521`:
   `out["system-path"] = c.System.Path`.
3. **The empty value** — `c.System.Path` is **monolith-only by construction**
   (`config.go:244-245,269-271`; field doc: "Monolith-only"). On multitier the
   system is split into `System.Backend` / `System.Frontend` and `System.Path`
   is `""`.
4. **The skip→crash** — the renderer deliberately **skips empty placeholder
   values** (`internal/atdd/runtime/driver/driver.go:771-773`,
   `if v != "" { … }`), so `system-path` never registers. The literal
   `${system-path}` survives substitution and `findUnfilledPlaceholders`
   (`internal/atdd/process/clauderun/clauderun.go:596`) turns it into the
   fail-fast error. **This is working as designed** — it refused to write code to
   an empty path rather than corrupting the tree.

**Why upstream phases passed:** the driver-adapter implementers reference
**Family B** keys (`${system-driver-adapter}`, …) that `PlaceholderMap` fills on
both architectures. Only the GREEN/redesign/refactor *system* prompts reach for
the monolith-only `${system-path}`.

**Why it surfaced now (not earlier):** the trigger needs **multitier** ×
**a dispatch that reaches the GREEN `implement-system` step**. Shop #72 is the
only story that fully drives `change-system-behavior` to that step (cross-ref
[[project_bpmn_full_coverage_story_and_realkind_gap]]), and shop only recently
became multitier. Every prior exercise was monolith, where `${system-path}`
resolves. The bug is fully determined by static artifacts (prompt body +
`PlaceholderMap` + the architecture matrix) yet was left to a 27-minute live
dispatch to discover — see Item 4.

---

## Root cause (orchestrator-side)

| # | Where | Gap |
|---|---|---|
| R1 | `system-implementer.md:8,27` | Hardcodes monolith-only `${system-path}` to name the per-channel production surface. On multitier the surface for a dispatch is the **channel's tier** (`api → backend`, `ui → frontend`), which the prompt has no way to name. |
| R2 | `config.go:507-531` `PlaceholderMap()` | Emits `system-path` (empty on multitier) but **no `backend-path` / `frontend-path` keys at all** — there is currently no placeholder the prompt *could* use for a tier surface, even if it wanted to. |
| R3 | No channel→tier mapping anywhere | `api→backend / ui→frontend` is pure convention; nothing in `config.go` (`System.Backend` / `System.Frontend` are singular `TierSpec`s) or `process-flow.yaml` encodes which tier a channel targets. A driver-side resolver needs this mapping as input. |
| R4 | `system-updater.md:6,25,30` + `system-refactorer.md:8` | Same monolith-only `${system-path}` assumption (8 references combined). These are **whole-system** agents (updater walks the Checklist across *all* channels), so on multitier their surface is **both tiers**, not one — a different shape from the per-channel implementer. Latent: they crash identically on the next multitier redesign/refactor dispatch. |
| R5 (test gap) | `clauderun_test.go:125-129` `newOpts()` | Every render test seeds a **monolith** Java config, so `${system-path}` always resolves and no test renders any prompt against a multitier `PlaceholderMap`. The only thing exercising body-level placeholder resolution on multitier is a live dispatch. |

---

## Resolved decisions

**D-A → A1 (driver-resolved `${system-surface}`, channel→tier table pinned in Go).**
The driver fills one `${system-surface}` placeholder per dispatch from `cfg` +
`nodeParams["channel"]`: monolith → `System.Path`; multitier + channel → that
channel's tier path via an explicit `{api: backend, ui: frontend}` table in the
driver. Prompts stay architecture-agnostic. Chosen over A2 (config-encoded
mapping) because the channel set is fixed for this teaching repo, so the schema +
validation + scaffolder + `config migrate` cost isn't justified; chosen over A3
(prompt-aware branching) because that lets the agent guess the wrong tier. A1
mirrors the existing driver-resolved-per-phase `Language` pattern. *If the
channel set later grows (mobile, admin, microservices), revisit A2.*

**D-B → B1 (fix the whole-system agents now).** `system-updater` and
`system-refactorer` carry the same monolith-only `${system-path}` assumption and
share the root cause, so they get the same `${system-surface}` token in the same
change. For the no-channel (whole-system) dispatch, multitier resolves to **both**
tier paths joined in reader-friendly form (e.g. `backend/ and frontend/`).
Deferring (B2) would leave two known landmines for the next redesign/refactor story.

---

## Items (A1 + B1) — ✅ all landed

All four agent Items are implemented and committed; the test suite is green
(`go test -p 2 ./internal/atdd/... ./internal/kernel/projectconfig/`). Summary:

1. **[driver]** `resolveSystemSurface(cfg, channel)` in `driver.go` + `${system-surface}`
   registered on the placeholder map at the `cfg.PlaceholderMap()` site.
2. **[agent]** `${system-path}`→`${system-surface}` in `system-implementer.md` (×2),
   `system-updater.md` (×3), `system-refactorer.md` (×1); `${system-db-migration-path}` untouched.
3. **[test]** `TestRenderMatrix_NoUnfilledPlaceholders` in `clauderun_test.go` — every agent ×
   `[monolith, multitier]` × `[channelled, whole-system]`, asserts no unfilled `${…}`.
4. **[driver/test]** `TestResolveSystemSurface` in `driver_test.go` — monolith / multitier
   api→backend / ui→frontend / whole-system→both / unknown→unfilled / nil → unfilled.

Only operator-driven verification + the memory follow-up remain.

---

## Verification

(Operator-driven — not agent `## Items` work.)

- `bash scripts/test.sh` (or `-p 2`) green; the new render-matrix + resolver tests pass.
- Re-run the failing rehearsal on the multitier shop config:
  `bash scripts/atdd-rehearsal.sh 72` against the multitier config — `implement-system`
  renders and proceeds past `RUN_AGENT` instead of crashing on `${system-path}`.
- Spot-check a rendered `system-implementer` prompt log: `${system-surface}`
  shows the backend path on the `api` dispatch, frontend on `ui`.
- Re-run `bash scripts/atdd-rehearsal-loop.sh` so #72 no longer stops the corpus.

---

## Notes (resolved + carry-forward)

- **Naming → `${system-surface}`.** Reads on both architectures (the "surface"
  the production code lands on, monolith or tier); not layer-coded. Cross-ref
  [[feedback_no_layer_coding_in_names]] and [[feedback_substitutable_paths_in_docs]].
- **Microservices (carry-forward guard).** `System.BackendServices` (name-keyed
  map) is a third shape. Out of scope for the fix, but the resolver's default
  branch must **fail-fast cleanly, not panic** — pinned as an assertion in Item 1
  / Item 4 (unknown-channel-on-multitier → unfilled → `findUnfilledPlaceholders`).
- **Memory follow-up.** After landing, update
  [[reference_system_path_monolith_only_resolver]] to note the prompt-render path
  is a second leak site now closed, and save the render-matrix-test gap.
