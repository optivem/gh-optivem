# 2026-06-19 17:53:42 UTC — Preflight dispatch dry-run: eliminate `(unresolved)` from scope-block render

## TL;DR

**Why:** Rehearsal run #71 (`gift-wrap-an-order`) halted ~20 min in at `BUILD_SYSTEM` (ui) on a `tsc` error, but the true cause was upstream: the ui `system-implementer` was dispatched with a scope block that rendered `system-path: (unresolved)` in both its read and write sets on multitier. The agent never saw the real frontend surface (incl. `src/test/`) as in-scope, worked off the prose `${system-surface}` instead, and missed a sibling `OrderFormData` constructor in a template smoke test. The engine is already strict-fail on unresolved `${}` placeholders everywhere else; `renderScopeBlock`'s soft `(unresolved)` fallback is the lone outlier.

**End result:** The `(unresolved)` token is eliminated from the codebase — a scope key resolves to a real path, is omitted as not-applicable to the architecture, or the render hard-errors. Preflight renders **every** dispatched writing-MID's scope block up front, so a run carrying any unresolvable key fails **before `IMPLEMENT_TICKET`, in <1s** — never leaking a malformed write-contract into a live dispatch.

## Outcomes

What we get out of this — the goals and deliverables:

- The literal `(unresolved)` is **never rendered anywhere**. Its only two emit sites (`clauderun.go:1059`, `:1064`, both in `renderScopeBlock`) are removed; the function's sole caller is the dispatch substitution (`clauderun.go:813`), so there is no display/debug case to keep.
- On multitier, a dispatched writing-agent's scope block (`read:` + `write:`) names the real tier surface (e.g. `system/multitier/frontend-react`) — including `src/test/` — instead of `system-path: (unresolved)`.
- The agent's *visible* contract matches the *enforced* one — scope-diff + preflight already resolve via `AddSystemSurfaceScope`; render parity closes the gap between what the agent is told and what is enforced.
- A genuinely-unresolvable scope key halts at **preflight, before the BPMN `main` process / `IMPLEMENT_TICKET`, in <1s**, with an operator-actionable message — not 20 minutes later as a downstream `tsc`/build or scope-diff failure.
- `renderScopeBlock` is brought into line with the engine's existing strict-placeholder contract (`statemachine/run.go:408`, `load.go:77,327`) — resolve or fail, never a soft sentinel.

## ▶ Next executable step (resume here)

Step 1 — Render parity. In `internal/atdd/process/clauderun/clauderun.go`, route the scope-block render path (`renderScopeBlock` ~line 1056 and its caller at `:813` that supplies `opts.Placeholders`) through `actions.AddSystemSurfaceScope` (`internal/atdd/process/actions/scope.go:253`), keyed on the dispatch's architecture + channel, so multitier `system-path` resolves to the concrete tier surface (api→backend, ui→frontend, whole-system→both) before rendering. This must land before Step 2 so removing the `(unresolved)` fallback doesn't hard-error a *correct* multitier config. Stop after this + its render test; it unblocks Steps 2a/2b.

## Steps

- [ ] **Step 1 — Render parity (`renderScopeBlock`).** Resolve multitier `system-path` to the tier surface via `actions.AddSystemSurfaceScope` (`scope.go:253`) on the render path, so the rendered `read:`/`write:` sets name the real surface. Add/extend a `clauderun` render test: a multitier ui dispatch's scope block contains `system/multitier/frontend-react` and no `(unresolved)`.
- [ ] **Step 2a — Preflight dispatch dry-run (primary; fails before `IMPLEMENT_TICKET`).** Preflight's existing `runScopeResolutionChecks` (`preflight.go:256`) walks the *resolved* path view, so it never sees the `(unresolved)` render token. Add a preflight pass that exercises the **render** path: for **every** dispatched writing-MID, render its scope block (via the Step-1 path) and hard-fail if any key would emit `(unresolved)`. Runs inside `preflight.Run` (`implement_commands.go:313`, before the BPMN `main` process), so it aborts at second 0 with an aggregated failure line. *(Thorough by decision — render all writing-MIDs; the static render is <1s.)*
- [ ] **Step 2b — Remove the `(unresolved)` fallback from `renderScopeBlock`.** Delete the two `return "(unresolved)"` lines (`clauderun.go:1059`, `:1064`) and the closure's soft-fallback behaviour. A key resolves to a non-empty path, is omitted as not-applicable to this architecture, or `renderScopeBlock` returns an error that its caller (`:813`) surfaces. No dispatch-path gating, no debug carve-out — the token does not exist in render output. Update the doc-comment (`:1042-1055`) that currently documents the soft `(unresolved)` behaviour.
- [ ] **Step 3 — Tests.** (a) Preflight: a multitier config whose render would leak `(unresolved)` fails preflight with the new message; healthy multitier **and** monolith configs pass (no false-trip on the by-construction-empty multitier `system-path`). (b) `renderScopeBlock` returns an error (not a sentinel) when a key can't resolve. (c) Reproduce the #71 shape: a multitier ui `system-implementer` dispatch renders the frontend surface in both scope sets. Run scoped — never unbounded `go test ./...` on Windows (`-p 2` / `scripts/test.sh`, or scope to the two packages).
- [ ] **Step 4 — Verify.** Dry-render a multitier ui dispatch: scope block names `system/multitier/frontend-react` incl. `src/test/`; a deliberately-broken scope key halts at preflight before `IMPLEMENT_TICKET`.

## Context / provenance

- Postmortem of rehearsal run `.gh-optivem/runs/20260619-163836/` (#71). Halt path: `IMPLEMENT_AND_VERIFY_SYSTEM_UI → BUILD_SYSTEM → EXECUTE_COMMAND → FIX → FIX_REJECTED_END`; proximate `tsc` TS2741 at `system/multitier/frontend-react/src/test/harness.test.tsx:12` (missing `giftWrap` on an `OrderFormData` literal after the type gained a required field at `form.types.ts:9`).
- Leaked scope block visible at `006-system-implementer.prompt.md:75` (read) and `:80` (write): `system-path: (unresolved)`.
- **Builds on** `plans/20260619-1250-multitier-system-path-scope-collapse.md` and commits `ffede25` (prose `${system-surface}`) + `3ca0b8b` (`ResolveLayerPaths` skip + preflight non-empty gate). Those fixed the **prose** and the **enforcement/preflight resolver**; this plan closes the remaining **render** leak and unifies detection at preflight. The existing collapse guard (`preflight.go:306-315`) is total-collapse-only (fires only when *zero* non-empty paths remain), so a partial `(unresolved)` alongside a valid `system-db-migration-path` slipped through.
- **Search confirms** (2026-06-19): `(unresolved)` is emitted only at `clauderun.go:1059` & `:1064`; `renderScopeBlock`'s only caller is the dispatch substitution at `:813` — no display/debug consumer. The engine already strict-fails on unresolved placeholders (`run.go:408`, `load.go:77,327`), so removing the soft fallback aligns the outlier with existing policy.

## Decisions resolved (were open questions)

- **Never `(unresolved)`, no gating.** `renderScopeBlock` has a single caller (the dispatch), so there is no non-dispatch surface that wants to display the token — the fallback is removed outright rather than gated to the dispatch path.
- **Preflight breadth: thorough.** Render every dispatched writing-MID's scope block at preflight, not just `system-path`-bearing ones — the static render is <1s, so cost is negligible.

## Out of scope (explicitly deselected during postmortem)

- **No agent-prompt change.** `system-implementer.md` (the "update every constructor incl. `src/test/` fixtures" net) was considered and deselected.
- **No BPMN / `process-flow.yaml` change.** The flow behaved correctly (detected the failure, routed to FIX).
- **No shop-template `tsconfig`/`package.json` change.** Splitting prod typecheck from `src/test/` was rejected as masking the failure class.
