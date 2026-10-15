# 2026-10-15 21:08 — Position gh-optivem as a spec-driven starter kit (spec-as-test)

## TL;DR

**Why:** "Spec-driven development" is the term developers search for when they look at AI-assisted coding (GitHub Spec Kit, AWS Kiro, Tessl, OpenSpec, BMAD). gh-optivem fits the category but out-ATDDs all of them: its specs are executable acceptance tests that must fail first and then pass. None of the existing tools checks that the code meets the spec. Today gh-optivem is presented as an academy/teaching tool, not as something a standalone developer would install.
**End result:** gh-optivem is pitched to standalone developers as **the spec-driven starter kit where specs are tests**. The README, docs and a comparison page all tell the same three-step story: **scaffold → adapt (by you) → drive**. A developer can try step 3 on a sample app before writing any drivers of their own.

## Decisions already made (from the 2026-10-15 discussion)

- **Audience:** standalone developers who install the tool themselves (not only academy students).
- **Keep the binary name `gh-optivem`.** It names the install route (a `gh` CLI extension), not the brand. Renaming it would break scaffolded repos and course references.
- **Brand:** "Optivem", with the category descriptor. No separate product name for now (fallback if one is ever wanted: "Assay", which is unchecked for trademark/domain).
- **Category we claim:** **Spec-as-Test**. It adds a fourth entry to Böckeler's (Thoughtworks) list of spec-first / spec-anchored / spec-as-source.
- **Tagline:** "Specs that fail first." Alternatives: "Spec-driven, test-proven." / "The spec is the test."
- **Product story (three steps):**
  1. **Scaffold:** gh-optivem generates the acceptance-test architecture (test layers, DSL, drivers, channels, CI pipeline), already filled in with **working eShop examples** (tests, DSL and drivers for the eShop sample app). It runs green out of the box. This is a *test architecture* that wraps an app, not an app template.
  2. **Adapt (by you):** the developer edits the eShop examples to fit their own app: reworks the drivers to target their UI and API, and replaces the eShop tests and DSL with their own domain. **The tool does not and will not claim to do this.** Present it as a deliberate choice: *"The agents never decide how your specs reach your system. You do, once."* The eShop examples are the worked reference the developer copies from.
  3. **Drive:** the BPMN-orchestrated agent workflow takes each ticket → acceptance criteria → failing acceptance test → code → green.
- **Main differentiators to name everywhere:**
  - **No drift:** a failing test is never out of date (versus markdown specs that go stale).
  - **Proof, not promises:** red → green is the evidence the code meets the spec (answers the "false sense of control" criticism).
  - **One thin slice at a time:** one ticket → a few criteria → a few tests (answers "Waterfall Strikes Back" and Spec Kit's 1,300-line spec for one feature).
  - **A defined process, not a chain of prompts:** the BPMN orchestrator decides which agent runs, which checks it must pass, and where it loops back on failure.
  - **Works on existing apps:** most SDD tools are strongest on new projects.
- **Business angle:** the free/open tool brings people to paid academy, workshop and consulting work, which is mostly help with the manual adapt step.

## Items

- [ ] **1. Rewrite the README's first screen** around the pitch: headline ("Executable specs for the app you already have" / "Specs that fail first"), install + init commands, the three steps scaffold → adapt (by you) → drive, and the differentiator bullets. Keep the existing reference content below it. Use `readme-updater` only if structural changes ripple further; otherwise edit directly.
- [ ] **2. Write a positioning page** (`docs/positioning.md` or similar): positioning statement, Spec-as-Test category explanation, the five differentiators, tagline. This becomes the single reference for all future copy.
- [ ] **3. Write a comparison page** "Spec Kit vs Kiro vs Tessl vs OpenSpec vs Optivem", built around the question **"What happens to your spec after the code ships?"** Columns: what the spec is, how the tool checks the code against it, new vs existing projects, how the process is orchestrated. Cite sources (list below) and re-check vendor docs for current status (Tessl's maturity was disputed in Oct 2026).
- [ ] **4. Make "run the eShop examples green, then drive one ticket on eShop" the documented try-it path**, so a developer sees step 3 (drive) working before adapting anything. The scaffold already ships the eShop examples. This item only makes the path explicit in the README/docs and fills any gap in how `gh optivem init` presents it.
- [ ] **5. Write a "From eShop to your app" guide** describing how to edit the scaffolded eShop examples into the developer's own:
  - which files are eShop-specific and must change (drivers, DSL, tests) and which are architecture to keep as is
  - one worked example per language (Java, .NET, TypeScript) of reworking an eShop driver against a different API
  - a "done" checklist (no eShop leftovers, suite green against your app)
  - a small first goal: "rework one driver for one endpoint → your first spec runs against your app"
- [ ] **6. Audit setup friction for a standalone developer.** List every prerequisite (`gh`, Docker, tokens, the `project` gh scope, org setup) and propose:
  - one install command
  - one preflight that reports **all** missing prerequisites at once
  - whether a local-only mode with no GitHub org setup is feasible

  Cross-reference existing work (clean-room VM loop, plan for the `project` scope, issue #58) instead of duplicating it. If it's large, write the result as a separate plan.
- [ ] **7. Check name availability** for "Optivem" in the SDD context and for the fallback "Assay": domains, GitHub orgs, obvious trademark conflicts. Report the findings and don't register anything.

## Verification (user-driven)

- Show the rewritten README first screen to two or three developers outside the academy and note where they get confused.
- Do a cold run of install → init → drive on the sample app on a clean machine, timed against a five-minute target.

## Sources (research 2026-10-15)

- https://www.augmentcode.com/tools/best-ai-tools-for-spec-driven-development
- https://particula.tech/blog/spec-driven-development-tools-spec-kit-vs-kiro-vs-tessl
- https://entalpa.com/en/vs
- https://github.com/cameronsjo/spec-compare
- https://codemyspec.com/blog/best-spec-driven-development-tools
- https://daniliants.com/insights/understanding-spec-driven-development-kiro-spec-kit-and-tessl/
- https://marmelab.com/blog/2025/11/12/spec-driven-development-waterfall-strikes-back
- https://glukhov.org/ai-devtools/vibe-coding/spec-driven-development-vs-vibe-coding/
- https://www.augmentcode.com/guides/spec-driven-development-vs-waterfall
- https://www.testmuai.com/blog/spec-driven-development/
- https://oreillyradar.substack.com/p/why-ai-coding-agents-still-need-clear
