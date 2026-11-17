# 2026-11-17 14:47:43 UTC — Local HTML visualizations: run view, docs site, coverage

> **DEFERRED.** Do not start until `gh optivem characterize` has been proven end-to-end on a real legacy project (real Docker). Its output (`docs/use-cases.md`, `${use-cases-path}/dependencies.md`, narratives, tagged acceptance tests) is the input of slices 2–3; building the viewer before that format is stable risks a rebuild. Higher-priority work ahead of this: credential rotation, then the characterize rehearsal.

## TL;DR

**Why:** gh-optivem's results are only visible as markdown files, `.jsonl` sidecars and terminal output. A long autonomous run is a black box, and a legacy system's use case model and test coverage are hard to show to students, product owners or a demo audience.
**End result:** three local, open-in-a-browser HTML views sharing one style. A live run view of the BPMN process, a docs site of the use case model linked to the acceptance tests, and per-use-case coverage colors from the latest test results.

## Outcomes

- **Run view** (`gh optivem run view`): the process-flow diagram with the run drawn on it. Done, current, not-reached and skipped nodes are distinguished, and each node carries badges for attempts, tokens, cost and elapsed time. A side panel shows totals and a dispatch timeline. The page refreshes itself while a run is in progress and is the record of the run once it ends. It shows the steps left on the expected path, not an exact count, because of gateways and fix loops.
- **Docs site** (`gh optivem docs build`): a static folder whose `index.html` opens locally with no server. The use case diagram and the dependency diagram link to a page per use case with its narrative and criteria. Each criterion links to the acceptance test file and line that pins it. Criteria text comes from the use case files while it exists there and from `spec generate` once it has been deleted. It works for legacy projects (after the characterize diagram step) and for new ones.
- **Coverage**: verified, failing or not covered per criterion and per channel, rolled up per use case, using the `spec check` status logic and the latest on-disk acceptance reports. The use case diagram is colored by status, with a headline line such as "14/20 use cases fully covered, 87/102 criteria verified".
- **Cross-links**: the run view links to the use case a run changes, and each use case page lists the runs that touched it.

## ▶ Next executable step (resume here)

**Step 1b is in progress: mockup v1 is published and waiting for the user's approval.** It is at https://claude.ai/artifact/6jwoEeEG1HFEzSKSt5BZZg. The source file is in a session scratchpad, so read it back from the artifact before iterating. On the user's request (2026-11-17), v1 tells one greenfield story with a hotel-booking example: Envision (from the sibling `20261117-1547-greenfield-use-case-modelling.md`), then the live run view, then the docs site and a use case page, then coverage. Each view is framed by the goal "build fast and safe with AI, and be able to maintain it later". Iterate on feedback; once the user explicitly approves, delete Step 1b and point this block back at the DEFERRED note.

Original brief: **Step 1b, the UX/UI mockup, can run now.** It is design only, with no code changes, so the DEFERRED note does not block it. Build the clickable HTML mockup of the three views with sample data, using realistic shapes from `summary.jsonl` and `steps.jsonl` (read a few `internal/atdd/runtime/driver/summary_sidecar*.go` structs for the fields) and from `docs/use-cases.md` as written by `legacy-use-case-diagram-writer`. Publish it as an Artifact, iterate on feedback, and **stop at the user's explicit approval**. Everything after Step 1b stays deferred until characterize is proven.

## Steps

- [ ] Step 1: Inventory the run-view data. Check what `summary.jsonl` and `steps.jsonl` record per dispatch and per step (node id, attempt number, status, timestamps, usage), and whether every BPMN node in `internal/atdd/process/process-flow.yaml` can be mapped to its rows. List any field the sidecars need to add.
- [ ] Step 1b: **UX/UI mockup, approval gate.** Build a clickable static HTML mockup of all three views with realistic sample data:
  - the run view (mid-run, with a fix loop retried)
  - the docs site home and one use case page
  - the coverage-colored diagram

  Publish it as an Artifact so the user can open it on any device. Iterate on the user's feedback and **stop until the user explicitly approves the design**. Steps 2–6 implement the approved mockup, not a fresh design.
- [ ] Step 2: Define the shared HTML shell: one embedded template and stylesheet, Mermaid rendering (inline for offline use, or a pinned CDN script), and a light/dark theme. All three slices use it.
- [ ] Step 3: Build slice 1, the run view. Reuse the `process show` Mermaid generation with per-node state classes and badges, add live refresh, the expected-path remainder and tests.
- [ ] Step 4: Build slice 2, the docs site. Parse `docs/use-cases.md`, `dependencies.md` and the narratives, resolve criteria from the use cases or `spec generate`, map criteria to test file and line, and add tests.
- [ ] Step 5: Build slice 3, coverage. Read the latest acceptance reports, reuse the `spec check` status computation, add per use case and per channel rollups and color the diagram.
- [ ] Step 6: Add the cross-links between runs and use cases.
- [ ] Step 7: Document the new commands in `docs/cli-reference.md` and the `--help` text.

## Out of scope (later)

- Publishing the docs site from CI (e.g. GitHub Pages).
- Coverage and cost history or trends across runs (needs results stored across runs).

## Open questions

- Command names: `run view` and `docs build` are my suggestions. Should the docs site sit under the existing `spec` command (e.g. `spec site`) instead?
- Live run view: is a self-refreshing static page that re-reads the `.jsonl` files enough, or is a small local server with `--serve` wanted? Browsers restrict reading local files from `file://` pages, so live refresh may in practice need the server or a regenerate loop.
- Mermaid offline: should it be embedded (a large binary asset) or loaded from a pinned CDN (needs internet, which the always-online rule allows)?
- Which test report format do the acceptance runs write per language (Java, .NET, TypeScript), and can criterion IDs (`UC-NNN.M`) be read from it directly?
- Slice order: run view first (cheapest, data already on disk) is inferred from the discussion. Confirm.
