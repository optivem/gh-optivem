# 2026-11-17 15:25:00 UTC — META: product roadmap for the plans in `plans/product/`

> Coordination plan only. It holds no implementation work of its own; it records the order, the dependencies and the business decisions behind the sibling plans in this folder. Update it whenever a sibling plan is added, finished or re-sequenced.

## TL;DR

**Why:** three plans written on 2026-11-17 together form one product direction for gh-optivem: help people build fast and safe with AI, and be able to maintain it later. That direction comes with the intent to sell it. Each plan on its own does not show the order or the reasons, so a later session could build them in the wrong order or put paid-tier features in the free CLI.
**End result:** one place that says which plan runs next, what blocks what, and which commercial decisions every sibling plan must respect.

## Plans in this folder

| Plan | What it delivers | Status | Blocked by |
|---|---|---|---|
| `20261117-1547-local-html-visualizations.md` | Run view, docs site, coverage (local HTML) | Step 1b mockup v1 published at https://claude.ai/artifact/6jwoEeEG1HFEzSKSt5BZZg, waiting for design approval; Steps 2–7 deferred | characterize proven end-to-end (Steps 2–7 only) |
| `20261117-1547-greenfield-use-case-modelling.md` | `envision`-style entry point: product idea → use case model → UX/UI mockups → narratives | Design only; needs `/refine-plan` | characterize proven end-to-end |
| `20261117-1612-source-available-licence-switch.md` | Source-available licence, commercial licence path, CLA | Needs `/refine-plan` (licence choice) | none; must land **before** the first release that ships the HTML views |

## Order

1. **Licence switch**: refine, then execute. It is cheap and blocks nothing, but it must happen before any HTML view is released, since a version released under AGPL stays AGPL forever.
2. **Mockup approval** (visualizations Step 1b): design only, so it can run in parallel with item 1.
3. *(outside this folder)* Credential rotation, then the characterize rehearsal on real Docker. These gate everything below.
4. **HTML visualizations** Steps 1, 2–7: run view, then docs site, then coverage, then cross-links.
5. **Greenfield use case modelling**: reuses characterize's output format, and the docs site renders its output.
6. **Paid hosted tier**: not planned yet. Write a plan only once free users ask to share views with their team.

## Commercial decisions (binding on every sibling plan)

- **Inspiration: Lovable, but with acceptance tests.** Lovable's strength is the first minute: idea to running app. Its weakness is month two: changes break old behaviour, and the only spec is a chat history. Our flow keeps the fast first minute and adds the parts that last: idea, approved use case model, UX/UI mockups, running app, acceptance tests proving every use case, then docs and coverage that stay current.
- **Implication:** if we follow Lovable's playbook, the hosted product (subscription plus usage credits, agents run on our servers) becomes the main product after validation, not a late add-on. The local CLI stays the free entry point and is validated first.
- **Positioning:** "AI builds it fast. Acceptance tests prove it's right. Docs and coverage keep it maintainable." The differentiators against Spec Kit, Kiro and Tessl are a failing acceptance test first, support for legacy systems via `characterize`, and the link from use case to test to coverage.
- **Model:** open core with a source-available licence. The local CLI, including the local HTML views, is free for individuals, education and small teams; companies above the size limit buy a commercial licence.
- **Reserved for the closed paid service, never added to the CLI:**
  - publishing the docs site from CI
  - coverage and cost history across runs
  - dashboards across repos and teams
  - private access, SSO and comments
  - hosted agent runs
  - audit evidence reports
- **The CLI's HTML output stays a local, one-repo, point-in-time snapshot.** Copying it into CI is allowed and expected; the value people pay for is what a snapshot cannot do.
- **No licence-key check in the CLI** at first. Enforcement is by licence terms; reconsider only if abuse is seen.
- **Revenue order:** method and training (courses, Optivem Circle) first, then team workshops and adoption help, then the hosted tier, then an enterprise or regulated tier.

## Open questions

- **Target audience:** developers and small teams building a product they must keep running, or founders and non-developers (Lovable's audience)? Recommended: developers and small teams, because Optivem's ATDD training credibility lands there and they already understand why tests matter.
- **Hosted product timing:** start a hosted-product plan right after mockup validation, or only after the local CLI has free users? Recommended: after the 5–10 mockup demos, if at least a few say their team would pay.

## ▶ Next executable step (resume here)

`/refine-plan plans/product/20261117-1612-source-available-licence-switch.md`

## Verification (operator, not agent work)

- Demo the mockup to 5–10 people from the Optivem audience. Ask whether their team would pay for the hosted version.
- A lawyer reviews the licence text before the first source-available release.
