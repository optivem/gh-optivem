# Automation Ideas: Sandbox Project Setup via gh-optivem

## Context

Analysis of the [sandbox project setup page](https://github.com/optivem/academy/blob/main/courses/02-atdd-accelerator/course/01-getting-started/04-sandbox-project.md) to identify manual tasks that could be automated by `gh-optivem`, reducing human effort during project setup.

## All Issues

### Already Tracked (#1-#9)

| Issue | Page Section | What it automates |
|-------|-------------|-------------------|
| ~~#1~~ ✅ | Scaffold | LICENSE file copyright holder + year |
| #2 | Task 1: Architecture Diagram | Mermaid architecture diagram based on `--arch` |
| ~~#3~~ ✅ | Task 2: Use Case Diagram | Pre-populated use case diagram with system name |
| #4 | Task 2: External Systems | `--external-systems` flag to populate diagrams with real names |
| ~~#5~~ ✅ | Task 2: GitHub Docs | Scaffold `/docs` structure (GitHub Pages removed — README links to `docs/*.md` instead) |
| ~~#6~~ ✅ | Scaffold | Replace system name in class/method/string across all langs |
| ~~#7~~ ✅ | E2E Tests | Remove `-Architecture` flag requirement from test runner |
| ~~#8~~ ✅ | Scaffold | Rename template test files to student's system name |
| #9 | Implement | `add-use-case` command to scaffold method stubs across DSL/driver/client |

### New Ideas (#10-#16)

| Issue | Page Section | What it automates | Priority |
|-------|-------------|-------------------|----------|
| #10 | Implement + Task 7 (Business Logic) | Scaffold primary use case (entity, endpoints, UI, validation, status) | High |
| #11 | Task 5 (External API Dependency) | Scaffold mock external API server + docker-compose entry | High |
| #12 | Task 8 (E2E Tests) | Scaffold E2E test stubs (positive, negative, CRUD) | High |
| ~~#13~~ ✅ | Test Manually + Task 3 (API CRUD) | Scaffold Postman/HTTP collection for manual testing | Medium |
| ~~#14~~ ✅ | Task 2, Step 5 (Use Case Narrative) | Scaffold use case narrative template in docs | Medium |
| ~~#15~~ ✅ | Milestone 1 (Project Registration) | Auto-generate project registration info after scaffold | Low |
| #16 | Teaching Agent | Tutor agent that guides students through the course interactively | Medium |

### Not Needed

| Idea | Page Section | Reason |
|------|-------------|--------|
| Clock abstraction | Task 6 (Clock Dependency) | Already handled by shop template ("Clock is already wired in") |

## Priority Summary

| Priority | Issues | Effort Saved |
|----------|--------|-------------|
| **High** | #10, #11, #12 | Large — covers the bulk of manual implementation work (Tasks 3-8) |
| **Medium** | #13, #14, #16 | Small-Medium — reduces manual testing, documentation setup, and project migration effort |
| **Low** | #15 | Small — one-time task per student |

## Spec-coverage gate open items

- TODO: `gh optivem spec check` lists every untagged test as an orphan ("no criterion ID"); on a repo where only one use case is tagged this drowns the report. Found in the shop real run (`docs/spec-gate/trial/gh-optivem-run.md`). Consider hiding the untagged list by default and showing only unknown IDs, or put it behind a flag.
- TODO: match tests to criteria by ID in test names (for example `@DisplayName`) as a fallback when tags cannot reach the XML. Tags remain the primary link (many-to-many, rename-proof).

Later ideas for the spec-coverage gate:

- Needs Clarification lifecycle state.
- Stale-criterion warning (spec changed after its test) and smell warning (one test tagged with many criteria).
- `gh optivem spec next --json`: tells an AI agent which criterion is next and what "done" means.
- Gherkin as a second spec source.
- MCP wrapper (thin, read-only: `status`, `next`, `explain`, `validate`).
- Wire the gate into the ATDD verify steps.
- Extract the ATDD runner (`internal/atdd`) from gh-optivem; remove the `kernel` and `diagrams` back-edges first.
