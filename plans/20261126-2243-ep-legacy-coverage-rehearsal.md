# 2026-11-26 21:43 UTC — Legacy coverage on EP: first real `characterize` run, use cases first

> Follows the Substack series *acceptance-testing-legacy-code-ai* (`substack/articles/published/2026-11-11-*-pipeline-use-case-diagram.md`, `...-use-case-narrative-business-rules.md`, draft `2026-11-26-*-acceptance-criteria.md`). Is the "characterize proven end-to-end" proof that `plans/product/20261117-1625-meta-product-roadmap.md`, `20261117-1547-greenfield-use-case-modelling.md` and `20261125-1636-client-adoption-two-phase.md` (Phase 1, item 3) are all sequenced behind.

## TL;DR

**Why:** `gh optivem characterize` is code-complete (2026-11-10) but has never run end to end on a real system. EP is a real legacy system we own, so it is the cheapest honest proof, and it is the same flow the Substack series teaches: minimal pipeline → use case diagram → core use case + dependencies → narrative + business rules → acceptance criteria → tests.
**End result:** EP has an approved use case diagram, dependency diagram, narratives, business rules and green tagged acceptance tests for the core use case and its dependencies (pilot), produced by `characterize` with no system code changed; every gap between the tool and the series is either fixed or written up as its own plan.

## Outcomes

- **Use case model for EP** under `${use-cases-path}`: use case diagram, `dependencies.md`, a proposed core use case approved by the operator.
- **Pilot coverage:** for the core use case and the use cases it depends on: narrative, business rules, `UC-NNN.M` acceptance criteria, then acceptance tests + DSL + drivers green against the running system, each step approved.
- **Safety gate proven:** the run shows zero system-code changes and no removed assertions.
- **Series ↔ tool gap list:** a concrete table of where `characterize` differs from the series (seeded from Decisions: PO interview, `php` lang), each row closed by a fix or a new plan.
- **Scaling run (optional, after pilot):** remaining use cases characterized unattended.
- `characterize` and `system start --restart --external-only` are proven against real Docker, unblocking the greenfield and client-adoption plans.

## ▶ Next executable step (resume here)

Design-only step remains first: all questions are resolved (see Decisions). Write the Step 0 config-blocker plan; after it ships, Step 1 (readiness check) is a read-only inspection of the EP repo: `gh-optivem.yaml` (`system-test.paths.use-cases-path`, `external-systems:`, system paths), `gh optivem system start` + `gh optivem system-test run --suite smoke` green.

## Steps

- [ ] Step 0: **Config blocker (known before running).** The EP config leaves (as does the EP system-test repo's) `system.architecture` and `system.lang` unset (comment in the yaml: gh-optivem has no `php` lang, and `architecture` requires `lang`), and set no `system.db-migration-path`. `characterize`'s safety gate (`systemRepoPaths`) errors on an unset architecture and on an empty `db-migration-path`, so the run would stop at its first commit check. Decide the fix (e.g. allow `architecture: monolith` without a supported `lang` for characterize/black-box systems, and make `db-migration-path` optional) and write it as its own gh-optivem plan; this plan waits on it.
- [ ] Step 1: **Readiness (series part 1, Minimal Pipeline).** In the target repo confirm Dockerfiles, docker-compose, smoke tests, commit/acceptance stages exist and pass locally (`gh optivem system start`, smoke suite green). Fix only config/test-side gaps; system code stays untouched. Record what was missing.
- [ ] Step 1b: **Move the old tests out (cut-paste, not delete).** On a fresh `characterize` branch of the EP repo, offer the operator to cut-paste the existing system tests to a folder outside the repo (default: a sibling `EP-old-system-tests/` next to the EP repo), keeping their paths: `system-test/tests/latest/acceptance`, `system-test/tests/latest/contract`, and the DSL/driver code under `system-test/src/testkit/dsl` and `system-test/src/testkit/driver`. Smoke tests stay (Step 1 needs them). Show the exact move commands, wait for the operator's yes (or the operator runs them), then commit the removal before `characterize` starts so its safety gate baseline begins clean.
- [ ] Step 2: **Diagrams (series part 2).** Build the use case model from scratch: no existing use case doc is used as input (the EP system-test repo's `docs/use-cases.md` included) (decided 2026-11-26); after approval, diff the result against it as a quality check only. Run `gh optivem characterize --log-file characterize.log` on a fresh branch. Review the use case diagram + dependency diagram + proposed core use case against the series' checklist (actors, external systems as secondary actors, review with the PO). Approve or redo.
- [ ] Step 3: **Pilot narrative + business rules (series part 3).** Per pilot use case: review narrative (main success scenario, extensions) and business rules; approve or redo.
- [ ] Step 4: **Pilot acceptance criteria (series part 4).** Review criteria for main, alternative, exception scenarios and business rules (incl. edge numerics such as rounding); approve or redo.
- [ ] Step 5: **Pilot tests.** Let the run write external-system stand-ins, acceptance tests, DSL, drivers until green; confirm the safety gate passed and the per-layer commits exist (`#characterize ... (UC-NNN)`).
- [ ] Step 6: **Postmortem per failure.** Any crash or false-RED goes through `/atdd-postmortem` into its own fix plan; resume with the same command on the same branch.
- [ ] Step 7: **Gap table.** Write the series ↔ tool comparison (step by step, enumerated both sides) into this plan; spin off a fresh plan per real gap.
- [ ] Step 8 (optional): **Scaling** — let the run continue over the remaining use cases unattended; then `gh optivem spec generate` to render readable Gherkin and `spec check` for coverage.

## Decisions (2026-11-26)

- **Target: the EP repo** (system in `site/` and `system-test/` in the same repo, `repo-strategy: mono-repo`). Repo placement comes from `gh-optivem.yaml` anyway; no same-repo assumption in `characterize`.
- Use case model is built from scratch (existing `docs/use-cases.md` only for a comparison afterwards).
- **The operator acts as the product owner** at every approve/redo gate; the "What should change?" answer stands in for the series' PO interview. The gap table records whether a real interview step is still needed.
- Business rules are already a separate artifact (`business-rules.md` beside `use-case-narrative.md`, `legacy-use-case-narrative-writer.md`), matching the series.
- Minimal pipeline already exists in the EP repo (`site/Dockerfile`, `site/docker-compose.yml`, commit/acceptance/deploy stage workflows, smoke tests), so Step 1 is a check, not a build.
- `system-test.lang: typescript`; the PHP system only affects Step 0 (`system.lang`/`architecture`).
