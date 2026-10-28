# 2026-10-28 14:42:10 UTC — Spec-coverage gate as a gh-optivem feature

## TL;DR

**Why:** Teams (and AI agents) can't answer "which acceptance criteria are verified by passing tests?". Trials of rsaglobaltech/specgate and reqcov on shop (Place Order, 14 criteria) showed neither fits: specgate needs Gherkin and reads only the suite exit code; reqcov lacks Planned / in-progress states, waivers and stage-based gating. Decided 2026-10-28: build the gate inside gh-optivem, not as a standalone product.
**End result:** A gh-optivem command checks acceptance criteria (with IDs, in plain markdown) against tagged tests and Gradle JUnit XML, reports a lifecycle per criterion, and gates leniently on branches and strictly on `main`, with the convention independent of the Optivem test architecture so it can be split out later.

## Outcomes

- Criteria (`UC-001.3`) live in plain markdown under use cases; no Gherkin required.
- A test links to a criterion by a tag on the top-layer test only; no change to the user's test architecture.
- Gradle JUnit XML is read, including `@TestTemplate` per-channel results (`method [Channel: UI]` mapped back to the method).
- Each criterion shows a lifecycle state: Planned, In progress (`wip`), Verified, Failing, Waived (waiver has reason, approver, expiry). Possibly also Needs Clarification.
- Gate is lenient on branches (only Failing blocks) and strict on `main` (coverage threshold).
- Orphan tests, unknown IDs and duplicate IDs are reported.
- An adoption baseline lets an existing repo adopt the gate without fixing old gaps.
- Output: CI job summary, sticky PR comment, and an HTML report a product owner can read; `--json` is the stable contract.
- The convention (IDs, tags, lifecycle) is documented independently of gh-optivem, so the gate can be split out if demand appears (5 unprompted external requests).

## ▶ Next executable step (resume here)

Design work, not a mechanical edit: settle the open questions below (command name, ID convention, use case source format) via `/refine-plan plans/20261028-1442-spec-coverage-gate.md`, then Step 1 becomes executable. Background and trial evidence are in the hq repo: `plans/20261028-091856-sdd-coverage-tool.md` ("Trial results" and the decision paragraph).

## Steps

- [ ] Step 1: Write the convention: criterion ID format, markdown layout (use case with criteria list), tag syntax on tests, lifecycle states and `wip` tag, waiver format. Architecture-neutral.
- [ ] Step 2: Decide where the code lives in gh-optivem (a package with no dependency on `internal/atdd`, so it can be extracted later) and the command name.
- [ ] Step 3: Implement parsing (markdown criteria, test tags, JUnit XML incl. `@TestTemplate` suffix handling) and the `--json` report.
- [ ] Step 4: Implement lifecycle computation and validation (orphan, unknown, duplicate IDs).
- [ ] Step 5: Implement lenient vs strict gating by stage, plus the adoption baseline.
- [ ] Step 6: Outputs: job summary, sticky PR comment, HTML report for the product owner.
- [ ] Step 7: Try it on shop (Java monolith, Place Order, 14 criteria; known shape: criteria 3 and 4 share tests, criterion 14 has no test, `legacy/` duplicates excluded).
- [ ] Step 8: Wire into the ATDD process at the verify steps ("verify tests fail" = In progress, "verify tests pass" = Verified, strict gate on `main`), and add a `next`-style status and an `adopt` command.
- [ ] Step 9: Docs, and update the demo canvas and course references.

## Background carried over from the hq plan (`plans/20261028-091856-sdd-coverage-tool.md`)

Wording below still says "specgate" for the gate; read it as the gh-optivem feature.

**Decided earlier:** acceptance criteria are first-class (criterion → acceptance test is the core link); use cases + criteria are the living documentation; user stories are throw-away and not modeled; lifecycle Planned / In progress / Verified / Failing / Waived; metric "spec coverage" (always shown with its definition, "21 of 27 acceptance criteria verified"); multi-language parity is not the gate's concern (one run reads one stack's results); the gate stays out of the build process (it guides and verifies; "verify tests fail" = In progress, "verify tests pass" = Verified, "mark IN ACCEPTANCE" = strict gate on main); validation fails on duplicate IDs, a criterion ID whose prefix does not match its use case, tests referencing unknown IDs, and malformed front matter.

**Convention details.** A use case has a main scenario plus extensions (alternative and exception flows); each criterion is one path through it and needs at least one acceptance test, so criteria coverage is path coverage. The gate cannot know whether the list of paths is complete; that is spec quality (courses and gh-optivem review). Use case state is derived from its criteria: Planned (no criterion has a test), In progress (some verified, or a linked test tagged `wip`), Verified (all verified). The gate threshold applies to criteria coverage. In a layered test architecture (test, DSL, driver, system) the criterion tag sits only on the top-layer test. `--json` output is the versioned contract that every other integration consumes.

**Feature scope.**
- v1: parse markdown use cases (front matter, scenarios, criterion IDs); link tests to criteria by tag in JUnit XML; lifecycle states; use case state derived from criteria; lenient and strict modes with a threshold; `validate`; orphan tests; JSON report; static HTML dashboard; exit codes; job summary, PR comment, badge; `next --json` telling an AI agent which criterion is next and what "done" means.
- v2: stale-criterion warning (spec changed after its test); smell warning (one test tagged with many criteria); expiring waivers in the config file (reason, approver, until date); coverage diff vs `main`; Gherkin as a second spec source; `init`; optional EARS wording; criterion → test → PR click-through.
- Out of scope: generating code (Spec Kit, Kiro, Tessl), authoring or running tests, formal verification, task management, user story modeling, multi-language parity.
- Later and only with demand: hosted dashboard, Jira / Azure DevOps traceability (a story cannot move to Done until its criteria are Verified), MCP wrapper (thin, read-only: `status`, `next`, `explain`, `validate`).

**Trunk-based development (no branch or PR assumption).** The gate takes a mode `lenient|strict`; the pipeline stage chooses it, not the branch. Commit stage (every push to trunk): lenient, blocks only validate errors and Failing criteria. Acceptance stage: lenient, blocks Failing, reports Planned and In progress. Release stage / promotion: strict, anything in the release scope that is not Verified, or coverage below the threshold, blocks. Unfinished work on trunk sits behind feature flags: a criterion can carry `flag: <name>`; the strict gate checks only criteria whose flag is on or that have none (a flag is deliberate dark work with no expiry or approver; a waiver is an expiring exception). Outputs without PRs: commit status and job summary are primary; the PR comment is optional. See `docs/tbd.md`.

**Working with POs and Jira teams.** Stories stay with the PO and the backlog; a story is a delta against the living documentation (adds or changes criteria, or adds a use case). PO flow: writes the story; agrees the criteria with the team; reads the use case markdown diff in the PR; sees criteria turn Verified before accepting. Each story lists the criterion IDs it adds or changes (custom field or description); the ID is the link, no Jira connection needed. With AI agents, criteria matter more than stories: an agent can be held to precise criteria with tests.

**Ticket-to-criteria contract (for the ATDD runner).** The PO's ticket lists criterion IDs; the Refine step writes or updates criteria in the use case markdown; the runner then reads criteria from markdown, not the ticket text. Ticket kinds: Story (business reason + IDs), Bug (links the violated or missing criterion), Task (refactor / redesign, may have no criteria). Build paths: freestyle (any test architecture; needs only a criterion tag on each top-layer test and JUnit XML) or gh-optivem (fixed ATDD sequence). Idea: split gh-optivem's ATDD into a core discipline (criteria, red then green then refactor, tests tied to IDs) and architecture profiles (Optivem DSL / driver profile, plain tests, Cucumber, page objects); read `process-flow.yaml` first to see how hard.

**Landscape.** Generate side is crowded (Kiro, GitHub Spec Kit, Tessl, OpenSpec, BMAD, fromSpec); none independently gates on test evidence. Verify side is thin (BMAD TEA `trace`, spec-trace, Doorstop, Jira test-management plugins, ReqProof). Gap: markdown use cases with criteria, state derived from test evidence, lenient-on-branch / strict-at-release lifecycle, free GitHub-native gate with PR comment, test-architecture agnostic, usable after Spec Kit, Kiro or gh-optivem. Still unread: SpecLint, tracey, spec-trace, BMAD TEA `trace`.

**Competitors.** rsaglobaltech/specgate (v0.27.2, MIT, Node, very active; "No test, no done"; REQ-NNN + Gherkin; 41 commands; MCP; `propose`, `harness`, `onboard`; already uses the phrase "spec coverage"). reqcov (Python, alpha 0.3.1, MIT; "Codecov for requirements"). thanhtung2693/specgate (Go, governance layer, different job). Ideas worth taking: "Needs Clarification" state, adoption baseline, product-owner report page, `onboard` / `init` for existing repos, publishing defects found per pilot.

**Naming (no longer blocking).** The standalone name `specgate` collided with rsaglobaltech/specgate and about 10 other GitHub repos; clean candidates found were specvouch, specbind, specvet, specattest, sddgate. Needed only if the gate is split out as a product. Category and tagline: SDD as the category, "verified SDD" with ATDD as the method. The public placeholder repo `optivem/gh-specgate` (README only) can be deleted or left unused.

**Splitting the ATDD runner out of gh-optivem (separate, not started).** `internal/atdd/` is 88 Go files, about 16,000 of about 38,000 non-test lines (42%). It imports `engine` (42), `kernel` (33), `build` (5), `userstate` (3), `expand` (1). Back-edges to remove first: `kernel` and `diagrams` import `atdd`. About 13 root command files use it. Sequence: remove the two back-edges; extract `engine` and `kernel` into a shared module; move `internal/atdd` and its commands into a new repo (old command names forward for a while); update docs, skills, course references. Keeping the gate in its own package without a dependency on `internal/atdd` (Step 2) keeps this option open.

**Trial results (shop Java monolith, Place Order, 14 criteria).** Full table in the hq plan. Summary: rsaglobaltech/specgate (V = run) needs Gherkin and REQ IDs, links per file, reads only the suite exit code, lifecycle Draft/Implemented only. reqcov (run): custom `id_pattern: UC-\d+\.\d+` works, plain markdown works, `// @req UC-001.3, UC-001.4` comments on 36 methods link many-to-many, Gradle XML read but `@TestTemplate` names (`method [Channel: UI]`) did not match (all 29 became orphans until the suffix was stripped), uncovered/covered/verified/failing/skipped states, lenient vs strict through two config files, per-criterion baseline, orphan/unknown/duplicate reports, sticky-comment marker and step summary (not posting itself), no Planned/wip, no waiver reason/approver/expiry. Windows note: Python missed files under paths over 260 characters. Shop findings: criteria 3 and 4 share tests; criterion 14 (SAP/ERP unavailable) has no test; the narrative has 3 extensions while tests cover about 9 more behaviours; `legacy/` holds about 25 duplicate PlaceOrder tests to exclude; tests are `@TestTemplate` (one method yields several JUnit results per channel). Use cases are in `shop/docs/design/shared/use-cases.md` and `use-case-narrative.md`.

**Dogfooding.** Level 1: the gate's own use cases and criteria live as markdown with tagged acceptance tests, and CI runs a pinned released version. Level 2: shop, one stack (Java monolith), Place Order. Level 3: a new product (the meetup app) built SDD-first with gh-optivem and the gate, as the greenfield case study and Substack series material.

**Demo.** The UX mockup is saved in `docs/spec-gate/mockup/` (see its README); live canvas https://claude.ai/artifact/CfvTQoqjbsLV1c3mCDVDec. It shows the flow from story to dashboard to bug and feedback flows. To update: command name, trunk-based flow, four-layer test example.

## Open questions

- Command name (the standalone name `specgate` is no longer needed; options: `gh optivem spec ...`, `gh optivem verify-spec`, ...). Recommendation: a `spec` subcommand group (`check`, `status`, `adopt`), because it groups related commands and avoids a product name.
- ID convention: `UC-001.3` (use case dot criterion) as in the trial. Recommendation: keep it; the trial showed reqcov-style custom patterns work and the prefix ties a criterion to its use case.
- Use case source format: plain markdown files in the repo (the trial used `- UC-001.1: text` list lines). Recommendation: markdown only in v1.
- Does the lifecycle include Needs Clarification? Recommendation: add it later; not needed for v1.
- Test-link tag syntax per language (comment marker vs annotation). Recommendation: a language-agnostic marker that appears in JUnit XML where possible, with per-language annotations as optional sugar.
