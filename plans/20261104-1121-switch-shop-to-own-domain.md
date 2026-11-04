# 2026-11-04 11:21 — Switch a scaffolded repo from the shop sample to the user's own domain

## TL;DR

**Why:** After `gh optivem init`, the repo runs green on the `shop` sample (Place Order, ERP, clock, ...). A user who wants to build *their own* product has no guided path from there: today they must hand-rewrite the use cases, DSL, drivers, tests and system code. The eparhija-system-test project showed a flow that works: describe → approved clickable mockups → use cases traced to screens → acceptance tests written test-first.
**End result:** A user writes a short informal product description; a gh-optivem agent turns it into a clickable HTML prototype they iterate on until approved; the approved screens are turned into `UC-NNN` use cases; the shop examples are removed, leaving the test architecture green and empty; and the existing ATDD flow drives the first stories test-first from those use cases.

## Outcomes

- The user writes a short informal product description (a few sentences) as the single input, e.g. in `docs/product.md`.
- A gh-optivem agent generates a **clickable UX/UI prototype** from it: static HTML/CSS screens linked into a click-through (Figma-like), opened in any browser, with no Figma account needed. Stored in the repo (e.g. `docs/prototype/`).
- The prototype is a **throwaway spec**, not the start of the real frontend: the real UI is built by the ATDD agents in the scaffolded frontend tech (decided 2026-11-04). Static HTML was chosen over Figma because it needs no account/API and lives next to the use cases it traces to.
- The user iterates on the prototype by giving feedback in plain language until they approve it. The approval is recorded (so later steps can refuse to run on an unapproved prototype).
- From the approved prototype, an agent writes:
  - a screen → use case table
  - one `docs/use-cases/UC-NNN-<slug>.md` per use case, with `UC-NNN.M` acceptance criteria that pass `gh optivem spec validate`
  - a use case diagram in `docs/use-cases.md`
- The user reviews the use cases before anything is removed or generated.
- Language (decided 2026-11-04): screen labels follow the description's language; use-case structure, criteria and tests are in English.
- The shop examples (UC-001 Place Order, shop DSL, tests, system code, shop-only external systems) are removed. The test architecture (layers, channels, CI pipeline) stays, and the repo is green with no tests.
- Each use case becomes input to the existing ATDD flow (ticket → acceptance criteria → failing acceptance test → code → green), so the first real stories are built test-first.
- **Two build modes, both through ATDD** (decided 2026-11-04): *attended* (default; stories run one at a time and the user reviews each) and *autopilot* (startup speed; after use-case approval all stories are queued and run unattended end to end, still red → green; the user steps in only on failures). Code is never written outside ATDD, so the red → green proof is kept.
- **CLI shape** (decided 2026-11-04): **one public command with resume, `gh optivem kickoff`** (the step after `init`; `start` was rejected because `system start` exists). It reads the recorded state (description written? prototype approved? use cases approved? shop removed? stories created?) and continues at the next phase; each phase ends at a user checkpoint, so the user just re-runs the command. The phases (prototype, use cases, remove sample, create stories) are **internal**: hidden subcommands or plain functions, usable for tests and debugging but not documented to users. A `--redo <phase>` flag reopens an earlier phase (e.g. change the prototype after use cases were approved) and marks every later phase as needing re-approval.
- Works for every architecture × language combination init supports (monolith/multitier × Java/.NET/TypeScript).

## ▶ Next executable step (resume here)

Step 1: inventory the shop-specific surface of a freshly scaffolded repo (per architecture × language) and write the remove / keep / rename table into this plan.
## Steps

- [ ] Step 1: Inventory what is shop-specific in a freshly scaffolded repo, per architecture × language: `docs/use-cases/`, `docs/design/`, system code, DSL, drivers/ports, acceptance and contract tests, simulators/stubs, the `external-systems` and paths in `gh-optivem.yaml`. Classify each as *remove*, *keep as architecture*, or *rename*. Output a table in this plan.
- [ ] Step 2: Product description input: define where it lives (`docs/product.md`) and how the command captures it (prompt interactively, or read the file). Same validation for flag and interactive input.
- [ ] Step 3: Prototype agent: new runtime agent prompt (e.g. `ux-prototyper.md`) that writes linked HTML screens + an index page to `docs/prototype/`, covering normal and edge/empty states. Feedback loop: the user describes changes, the agent edits the screens. Record approval in `gh-optivem-progress.yaml` (see Step 6c).
- [ ] Step 4: Use case agent, in two checkpoints (decided 2026-11-04):
  - 4a: from the approved prototype + description, write the screen → use case table and the **use case diagram** in `docs/use-cases.md`: actors, use cases, and the **external systems as secondary actors** (as shop's diagram shows Clock and ERP). External systems are decided here, up front, not discovered later. Stop for user approval.
  - 4b: from the approved diagram, write one narrative per use case (`docs/use-cases/UC-NNN-<slug>.md`, `UC-NNN.M` criteria), then run `gh optivem spec validate`. Stop for user approval.
  - External systems are recorded **only in the approved diagram** (decided 2026-11-04): this phase does not write them into `gh-optivem.yaml` or scaffold drivers/simulators; ATDD sets each one up when a story first needs it, reading the diagram as the source of truth.
  - ATDD's `IDENTIFY_EXTERNAL_SYSTEM` must agree with the approved diagram (check whether it needs to read it; flag a mismatch rather than inventing a new system).
- [ ] Step 5: Shop removal, run only after the use cases are approved (so the repo is never left without a direction): delete (not archive; the shop repo stays the reference) the *remove* items from Step 1 and adjust config so build + CI stay green with empty suites. Must be deterministic code, not an agent.
- [ ] Step 6: Hand-off to ATDD: generate the initial user stories from the approved use cases, one per slice (e.g. "UC-001 main scenario", "UC-001 extension 4a"), as tickets in the existing ATDD intake format (Description + Gherkin Acceptance Criteria, plus External System Contract Criteria where the slice crosses an external system), each referencing the `UC-NNN.M` ids it covers. Later stories are merged back into the use cases by `plans/20261104-1227-merge-stories-into-use-cases.md` (one-way, one-time; stories are closed after building, never re-edited). The initial stories already match the use cases, so their merge is a no-op and must pass **without** a human approval (otherwise autopilot stalls on the first story).
- [ ] Step 6b: Autopilot mode: queue all generated tickets and run the ATDD flow over them unattended, in dependency order, stopping and surfacing only on failures. Attended mode stays the default.
- [ ] Step 6c: The public command: resume at the next incomplete phase, `--redo <phase>` invalidating later phases; phases implemented as hidden subcommands (or functions) so tests can drive each one directly. Progress (decided 2026-11-04):
  - **Derived from the files wherever possible:** description written = `docs/product.md` exists; sample removed = shop files absent; stories created = issues exist linked to the use cases.
  - **Only approvals are stored**, in a small committed file at the repo root, `gh-optivem-progress.yaml` (not in `gh-optivem.yaml`, which is user configuration, and not in `.gh-optivem/`, which is gitignored local run data). Each approval records who, when, and an **approval fingerprint** of the approved content (prototype, use case diagram, narratives). Not to be confused with the **criterion fingerprint** in the merge plan (marks a changed `UC-NNN.M` unverified); they may share hashing code but are named distinctly.
  - **Edit after approval = re-approval:** a fingerprint mismatch marks that phase and every later one as needing re-approval. `--redo` uses the same invalidation logic.
- [ ] Step 6d: Update the parked plan `plans/backlog/parked/20261015-2108-spec-driven-positioning-and-branding.md`: its "the tool will not adapt" decision now applies only to existing apps (drivers targeting a real UI/API); greenfield products are adapted via this plan's flow.
- [ ] Step 7: Docs: README + getting-started describe the new path (init → describe → prototype → use cases → stories).
- [ ] Step 8: Tests: unit tests for the removal step on every architecture × language; render tests for the new agent prompts; one attended end-to-end run on a throwaway repo.

## Related

- `plans/20261104-1227-merge-stories-into-use-cases.md`: kept as a separate plan because it changes ATDD intake for every repo (including existing apps), not only after a domain switch. Neither plan blocks the other: without it, the initial stories simply run through today's ATDD intake.
