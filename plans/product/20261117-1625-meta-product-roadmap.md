# 2026-11-17 15:25:00 UTC — META: product roadmap for the plans in `plans/product/`

> Coordination plan only. It holds no implementation work of its own; it records the order, the dependencies and the scope rules for the sibling plans in this folder. Update it whenever a sibling plan is added, finished or re-sequenced.

## TL;DR

**Why:** two plans written on 2026-11-17 together form one product direction for gh-optivem: help people build fast and safe with AI, and be able to maintain it later. Each plan on its own does not show the order or the scope limits, so a later session could build them in the wrong order or grow the CLI beyond its scope.
**End result:** one place that says which plan runs next, what blocks what, and which scope rules every sibling plan must respect.

## Plans in this folder

| Plan | What it delivers | Status | Blocked by |
|---|---|---|---|
| `20261117-1547-local-html-visualizations.md` | Run view, docs site, coverage (local HTML) | Step 1b mockup v1 published at https://claude.ai/artifact/6jwoEeEG1HFEzSKSt5BZZg, waiting for design approval; Steps 2–7 deferred | characterize proven end-to-end (Steps 2–7 only) |
| `20261117-1547-greenfield-use-case-modelling.md` | `envision`-style entry point: product idea → use case model → UX/UI mockups → narratives | Design only; needs `/refine-plan` | characterize proven end-to-end |
| `20261125-1636-client-adoption-two-phase.md` | Spec-only repo against a client's existing system + legacy coverage (Phase 1); `implement` on client tickets with self-fixing regressions (Phase 2) | Design only; needs `/refine-plan` | characterize proven end-to-end (Phase 1 step 3) |

## Order

1. **Mockup approval** (visualizations Step 1b): design only, can run now.
2. *(outside this folder)* Credential rotation, then the characterize rehearsal on real Docker. These gate everything below.
3. **HTML visualizations** Steps 1, 2–7: run view, then docs site, then coverage, then cross-links.
4. **Greenfield use case modelling**: reuses characterize's output format, and the docs site renders its output.

## Scope rules (binding on every sibling plan)

- **Out of scope for the CLI** (keep these out of every plan here):
  - publishing the docs site from CI
  - coverage and cost history across runs
  - dashboards across repos and teams
  - private access, SSO and comments
  - running agents on a hosted service
  - audit evidence reports
- **The CLI's HTML output stays a local, one-repo, point-in-time snapshot.**
- **No licence-key check in the CLI.**

## ▶ Next executable step (resume here)

Approve or iterate the mockup (visualizations Step 1b): `/execute-plan plans/product/20261117-1547-local-html-visualizations.md`.

## Verification (operator, not agent work)

- Demo the mockup to a few people from the Optivem audience and note what they find unclear.
