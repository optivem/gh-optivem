# 2026-11-25 15:36:00 UTC — Client adoption: make gh-optivem support the two-phase rollout on a client's existing system

> Sibling of `20261117-1625-meta-product-roadmap.md`. Product work only: what the tool must do so the two-phase rollout below works on a client's real system. Sales material (pricing, the proposal email, engagement terms) stays out of this repo, the same as the licence plan that moved to hq.

## TL;DR

**Why:** the rollout we offer clients has two phases. In Phase 1 we set up executable specifications in a separate repo and cover the existing behaviour. In Phase 2 the client's developers run every new ticket through the workflow engine. Today gh-optivem assumes it scaffolded the system itself, on GitHub, in our shop layout. A client's system was built by someone else, may use a tracker and a CI that aren't ours, and must not be modified in Phase 1.
**End result:** with a client's existing repo and tracker, an operator can:
- (Phase 1) scaffold a separate executable-spec repo that tests the client's running system as a black box, then run `characterize` to get legacy coverage.
- (Phase 2) run `implement` on a client ticket so that it writes the spec, tests and code, commits and pushes, and fixes its own regressions when the client's pipeline goes red.

## Product name

**Recommended: Optivem Flow.** It says what the product does (it runs the whole flow from ticket to production) in words a non-technical buyer understands, and it puts the Optivem brand in front.
- The CLI command stays `gh optivem`. The name is for the product and the docs, not a rename of the binary.
- Alternatives: *Optivem Pilot* (clashes with "Copilot"), *Optivem Relay* (good for the hand-offs between AI and humans, but less obvious).
- Before using it publicly, check the trademark and domain for the chosen name.

## Phase 1: Executable Specification Architecture & Legacy Coverage

Goal: the client's code is not touched. All spec work lives in a separate repo, and the client's only job is to add one job to their pipeline.

1. **Spec-only scaffold against an existing system.** `init` mode that creates only the system-test tier (acceptance tests + DSL + drivers) and points it at an external system by URL and credentials, with no system code, no system Dockerfile and no system build.
   - Verify first: how much of this the current `--overwrite` / multi-repo `repo-strategy` already covers.
2. **Black-box system config.** `gh-optivem.yaml` describes a system we don't build: base URLs per channel (UI, API), auth, how to reset or seed test data, and which external systems to stub.
3. **`characterize` on a read-only client codebase.** Derive the use case model from the client repo, cloned read-only, and write the use cases plus the legacy acceptance tests into the spec repo. Do not write to the client repo.
   - Depends on characterize being proven end-to-end (see the META roadmap).
4. **Coverage scope selection.** Let the operator choose the key use cases to cover first, instead of characterizing the whole system in one go.
5. **Pipeline hand-off artefact.** Generate a ready-to-paste pipeline step that runs the spec suite against the client's deployed environment, for GitHub Actions first, plus a documented generic shell entry point (`gh optivem system-test run ...`) for any other CI.
6. **Phase 1 done-check.** A command that reports which use cases are covered and green against the client environment, so the client can see that Phase 1 is finished.

## Phase 2: Workflow engine on every new ticket

Goal: a developer starts one command per ticket. The engine tells the AI and the humans what to do next, and in what order.

1. **Client tracker as ticket source.** Confirm that `implement` reads tickets from Jira as well as GitHub Issues (a Jira tracker adapter exists in `internal/atdd/runtime/tracker`; check how complete it is).
2. **Human gates as named roles.** Approval steps (spec, tests, code) name a role (developer, product owner) instead of "the operator", so the right person is asked at each gate.
   - Example: the AI writes the ticket spec, the developer checks it, the product owner approves it.
3. **Work in the client repo layout.** `implement` writes code in the client's repo while the tests live in the spec repo from Phase 1. Two repos in one run, each with its own commit and push.
4. **Client's process as config.** Teams can switch gates on or off and reorder optional steps in config without forking the BPMN.
5. **Regression self-fix from the pipeline.** When the client pipeline's spec job fails after an AI change, the engine picks up the failure and runs the fix loop (the same root-cause-and-fix flow as `/fix-bug`), then pushes the fix.
6. **Per-ticket run report.** A summary of what was done, which gates were passed, tokens used and the commits made, which the developer can attach to the ticket.

## Open questions (resolve with `/refine-plan`)

- Phase 1 step 1: extend `init` with a new mode, or add a separate command (e.g. `adopt`)?
- Which CI comes after GitHub Actions: GitLab CI or Azure DevOps?
- Test data on a system we don't own: is there a reset/seed API, or are tests written to be data-independent?
- Does the spec repo need access to the client's source code beyond Phase 1 (e.g. for the Phase 2 code-writing steps)?

## Scope rules

- Every scope rule in the META roadmap still applies (no hosted agents, no cross-team dashboards, no licence check).
- Nothing in Phase 1 writes to the client's repo.
