# 2026-06-24 14:14:00 UTC — Fix scaffolder config-name rewrite for commit-stage workflows (polyglot)

## TL;DR

**Why:** In polyglot scaffolds (backend/system language ≠ test language), the scaffolder leaves `GH_OPTIVEM_CONFIG:` in the commit-stage workflows pointing at a per-flavor config name (`gh-optivem-multitier-<backendLang>.yaml`) that does not exist in the scaffolded repo — because the config-name rewrite is keyed only on `testLang`. The scaffolded `backend-commit-stage` / `frontend-commit-stage` workflows then fail at CI runtime, and the scaffolder's own guardrail never catches it.

**Monolith second hop (observed in CI, run 28113696564 / `monolith monorepo java typescript`):** the surviving monolith config name `gh-optivem-monolith-<lang>.yaml` is then **collaterally mangled** by the later Sonar-key pass. `monolithSonarKeyReplacements(lang)` rewrites `-monolith-<lang>` → `-system` across **all** text files (including the workflow, via `FixupAllTextFiles` at `apply_template.go:258`, which runs *after* the workflow-content pass at line 253). Since `gh-optivem-monolith-java.yaml` contains the substring `-monolith-java`, the reference becomes `gh-optivem-system.yaml` — a name that exists nowhere. So the runtime error is `no gh-optivem.yaml at gh-optivem-system.yaml`, not the pre-mangle `gh-optivem-monolith-java.yaml`. Multitier has no equivalent second hop (its Sonar suffix `-multitier-backend-<lang>` is not a substring of `gh-optivem-multitier-<lang>.yaml`). **Implication:** the Step 1 content-pass rewrite (running before the Sonar pass) fixes both; but the Step 3 guardrail — which runs *last*, after the Sonar pass — must forbid the post-mangle residual `gh-optivem-system.yaml`, not just `gh-optivem-monolith-<lang>.yaml`.
**End result:** The scaffolder rewrites every `GH_OPTIVEM_CONFIG:` reference (including commit-stage) to the canonical `gh-optivem.yaml` regardless of which language named it, across all four apply paths (monolith/multitier × monorepo/multirepo); the forbidden-refs guardrail fails the scaffold loudly if any per-flavor config name survives; polyglot test cases lock the behavior in.

## Outcomes

What we get out of this — the goals and deliverables:

- A polyglot multitier scaffold (e.g. backend=java, test=typescript) produces `backend-commit-stage.yml` and `frontend-commit-stage.yml` whose `GH_OPTIVEM_CONFIG` is `gh-optivem.yaml` (not `gh-optivem-multitier-java.yaml`), so `gh optivem component-test setup` finds its config and the commit stage passes.
- The same correctness for monolith polyglot scaffolds (lang ≠ testLang) and for any multitier scaffold whose testLang ≠ `java` (the frontend's hardcoded-`java` ref).
- The scaffolder's forbidden-refs verification fails loudly (per the fail-loud / `check-*` convention) if **any** residual `gh-optivem-<arch>-<lang>.yaml` survives a rewrite — turning this class of bug into a scaffold-time hard failure instead of a CI-time surprise.
- Coverage confirmed across all four apply paths: multitier monorepo (the reported incident), multitier multirepo, monolith monorepo, monolith multirepo.
- Regression tests in `replacements_test.go` asserting commit-stage config rewrite for polyglot multitier + monolith, plus forbidden-ref tests asserting the backendLang-named leftover is now caught.

## ▶ Next executable step (resume here)

All agent work is done (Steps 1–5 shipped: `scaffoldLangs` + `optivemConfigRewrites` helper wired into all four apply paths, broadened forbidden-refs guardrail incl. the post-Sonar-mangle `gh-optivem-system.yaml`, and regression tests — `go test ./internal/scaffolding/...` green). Only the **user-run E2E** below remains; no further edits. This is a verification step, not an `/execute-plan` resume point.

## Steps

- [ ] Step 6 (⏳ Deferred — user-run E2E, do not self-run): Re-run a polyglot scaffold (backend=java, test=typescript) and confirm `backend-commit-stage.yml` and `frontend-commit-stage.yml` both reference `gh-optivem.yaml`, and the commit stage passes. While doing so, validate the two open questions below.

## Notes / constraints

- **Do not change the shop workflow.** The frontend's hardcoded `gh-optivem-multitier-java.yaml` (`shop/.github/workflows/multitier-frontend-react-commit-stage.yml:56`) is correct in shop (all 12 configs exist there); the scaffold rewrite is the right layer to fix.
- **Commit-stage is latest-only** — there is no legacy commit-stage workflow, so only the `.yaml` → `gh-optivem.yaml` mapping is needed for the extra languages; the legacy mapping stays `testLang`-keyed (acceptance-stage-legacy is the only legacy ref in copied workflows).
- Root cause confirmed statically: source trace of the replacement rules + reading the live scaffolded repo's actual workflow bodies (`valentinajemuovic/manual-test-1771866363b6983c`). Failing run (multitier): https://github.com/valentinajemuovic/manual-test-1771866363b6983c/actions/runs/28102942986
- Monolith second-hop confirmed in the gh-acceptance-stage smoke matrix: run 28113696564, job `Smoke (monolith, monorepo, java)` → downstream scaffolded commit-stage run 28113897850 failed at *Compile Code* with `ERROR: no gh-optivem.yaml at gh-optivem-system.yaml`. Trace: shop `monolith-java-commit-stage.yml:56` = `gh-optivem-monolith-java.yaml` → survives testLang-keyed content pass → `monolithSonarKeyReplacements("java")` `-monolith-java`→`-system` mangles it to `gh-optivem-system.yaml`.

## Open questions

- **Forbidden-ref false positives:** broadening the guardrail to all three languages must not flag legitimate content in a scaffolded repo. Low risk (the existing testLang ref already scans clean; this is symmetric) — validate during the Step 6 E2E.
- **Is multitier multirepo a supported/shipped strategy?** Step 2 fixes it regardless, but confirm it is exercised (not deprecated) so the fix is worth the test coverage.
