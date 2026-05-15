# Open work triage: priorities across gh-optivem issues + plans

> ⚠️ **STATUS: NEEDS HUMAN DECISION.** This plan does not execute anything. It captures the current state of open issues + open plans in `gh-optivem` and proposes a priority ordering. The "Decisions needed" section lists the questions a human must answer before any execution begins. Do not start work from this plan without explicit go-ahead per item.

## Context

Snapshot taken 2026-05-14 against `optivem/gh-optivem`:

- **38 open issues** (`gh issue list --state open`, `--limit 100`).
- **2 open plans** under `plans/`:
  - `20260513-1530-shop-canonical-db-schema-via-migrations.md` — agent-picked-up today (2026-05-14 08:52Z), decisions locked, execution underway.
  - `20260514-0914-migrate-workspace-scripts-to-gh-optivem.md` — explicitly DEFERRED at the top of the file.

The 38 issues are not all live work. Many are old enhancement stubs from early April; many are auto-filed scaffold-failure reports with overlapping symptoms. The goal here is to bucket them honestly and surface only the items that warrant near-term attention.

## Current snapshot

### In flight (do not interrupt)

| Item | Why |
|---|---|
| `plans/20260513-1530-shop-canonical-db-schema-via-migrations.md` | Agent picked up 2026-05-14 08:52Z. 10 decisions locked, Phases 1+2 are an inseparable single PR across 6 stacks / 24 compose files. |

### P0 candidates (real bugs biting current flows)

| # | Title | Signal |
|---|---|---|
| #55 | Scaffolding failure: Veg Finder (monolith, monorepo) — TS | Phase 6 build: `exit status 125` (docker build). 2026-05-13. |
| #54 | Same as #55 | Duplicate filing. |
| #53 | Scaffolding failure: Page Turner (monolith, monorepo) — Java | Phase 6: acceptance suite (stub mode) fails. 2026-05-13. |
| #52 | Scaffolding failure: Page Turner (multitier, monorepo) — Java | Phase 6: `gradlew.bat compileJava` exits 1 on Windows. 2026-05-13. |
| #42 | Meta-prerelease vs auto-bump-patch chicken-and-egg | Structural bug. Has full root-cause writeup. Manual `VERSION` bump required to unblock today. |
| #27 | Mirror base images to GHCR | MCR `403 Forbidden` on `dotnet/aspnet:8.0` manifest. Plausibly the same root cause as #55's `exit status 125`. Implementation sketch already written. |

### P1 candidates (real but lower urgency)

| # | Title | Notes |
|---|---|---|
| #32 | Runner port-clash: `IsAnyURLUp` matches unrelated stack | Silent correctness bug — tests pass against the wrong system. Local-mode only. |
| #40 | Scaffolding failure: "nothing to commit" git error | Reproducible — repo already had matching content. Small fix in commit-and-push step. |

### P2 / roadmap (not bugs)

- **ATDD cluster: #43, #44, #45, #46, #47, #48, #49, #51.** Mostly title-only. Reads as a planned workstream, not eight separate tasks. #51 (language-agnostic ATDD, e.g. python) is the most concrete.
- **#16** Teaching agent — speculative feature, depends on private course content.
- **#31** `--json` flag for `gh optivem init` — small DX win.

### Older auto-filed scaffold failures (likely look-alikes of P0)

#21, #22, #23, #24, #25, #28, #29, #30, #38. All title-only or near-empty bodies, all from April. Almost certainly the same handful of underlying causes as the recent ones. Worth a single triage pass that closes duplicates and merges signal into the live P0 tickets.

### Deferred / skip

- `plans/20260514-0914-migrate-workspace-scripts-to-gh-optivem.md` — author marked DEFERRED.
- **#33** Evaluate moving github-utils scripts into gh-optivem — covered by the deferred plan above.
- **#17** Fix SonarCloud warnings — generic hygiene, no specific failing project.
- **#2, #4, #9, #10, #11, #12, #18, #19** — vague enhancement notes from early April. Recommend bulk-close unless a human flags specific ones to keep.

## Proposed sequencing (subject to decisions below)

1. Let the Flyway plan finish — do not start anything else in the same area.
2. **Triage the scaffold-failure cluster as one unit.** Pull logs for #52/#53/#55, classify by root cause (network / docker / windows-gradle / acceptance-suite), close #54 as duplicate, fold older April look-alikes (#21–25, #28–30, #38) into whichever live ticket matches.
3. **Do #27 (GHCR mirror) before re-investigating #55.** If the `exit status 125` is the MCR 403, mirroring eliminates the bug; if not, the remaining failure mode is clearer.
4. **Then #42** (meta-prerelease deadlock). Has the cleanest root-cause writeup of the bugs; needs a structural decision (split top-level `VERSION` per flavor vs. unify the two signals) before code.
5. **P1 next:** #32 and #40 are small enough to bundle.
6. **ATDD cluster:** treat #43–49 + #51 as one planning task — produce a single plan that sequences them, instead of executing them ad hoc.
7. **Stale issues:** bulk-close with a one-line comment, unless a human pulls specific ones out.

## Decisions needed (human)

These are the questions blocking execution. Each one is genuinely open — do not pick a default.

1. **Scaffold-failure triage scope.** Pull logs and classify roots for *all* open scaffold-failure tickets in one pass (#21, #22, #23, #24, #25, #28, #29, #30, #38, #40, #52, #53, #54, #55), or only the four recent ones (#52–55)?
2. **Order between #27 and the scaffold failures.** Do #27 first (might eliminate one P0 outright), or investigate the failure logs first to confirm the link before investing in the mirror?
3. **#42 direction.** Two possibilities surface from the ticket: (a) unify the two signals (image-presence and git tag) so both check the same thing; (b) acknowledge per-flavor independence and split the top-level `VERSION` per flavor — which matches the reality that Java's multitier VERSION already diverges. Which direction?
4. **ATDD cluster shape.** Bundle #43–49 + #51 into a single multi-step plan, or keep them as separate tickets and just pick one to start?
5. **Stale-issue disposition.** OK to bulk-close #2, #4, #9, #10, #11, #12, #17, #18, #19 with a "stale, re-open if still relevant" comment, or keep them?
6. **Duplicate handling.** Close #54 as duplicate of #55 now, or leave both until triage step 2 is run?

## Out of scope

- Executing any of the buckets above.
- Reading individual ticket bodies in detail — this plan is a snapshot based on titles + recent-ticket bodies + the two plan files. A real execution plan for any P0 item must read the full log and the relevant code paths first.
- Cross-repo work (shop, github-utils, courses). The Flyway plan already covers shop schema; this triage is for `gh-optivem` itself.

## References

- Issue list source: `gh issue list --repo optivem/gh-optivem --state open --limit 100` on 2026-05-14.
- Open plans: `plans/20260513-1530-shop-canonical-db-schema-via-migrations.md`, `plans/20260514-0914-migrate-workspace-scripts-to-gh-optivem.md`.
- Conversation that produced this triage: 2026-05-14, single session.
