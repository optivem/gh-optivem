# 2026-06-24 14:14:00 UTC — Fix scaffolder config-name rewrite for commit-stage workflows (polyglot)

## TL;DR

**Why:** In polyglot scaffolds (backend/system language ≠ test language), the scaffolder leaves `GH_OPTIVEM_CONFIG:` in the commit-stage workflows pointing at a per-flavor config name (`gh-optivem-multitier-<backendLang>.yaml`) that does not exist in the scaffolded repo — because the config-name rewrite is keyed only on `testLang`. The scaffolded `backend-commit-stage` / `frontend-commit-stage` workflows then fail at CI runtime, and the scaffolder's own guardrail never catches it.
**End result:** The scaffolder rewrites every `GH_OPTIVEM_CONFIG:` reference (including commit-stage) to the canonical `gh-optivem.yaml` regardless of which language named it, across all four apply paths (monolith/multitier × monorepo/multirepo); the forbidden-refs guardrail fails the scaffold loudly if any per-flavor config name survives; polyglot test cases lock the behavior in.

## Outcomes

What we get out of this — the goals and deliverables:

- A polyglot multitier scaffold (e.g. backend=java, test=typescript) produces `backend-commit-stage.yml` and `frontend-commit-stage.yml` whose `GH_OPTIVEM_CONFIG` is `gh-optivem.yaml` (not `gh-optivem-multitier-java.yaml`), so `gh optivem component-test setup` finds its config and the commit stage passes.
- The same correctness for monolith polyglot scaffolds (lang ≠ testLang) and for any multitier scaffold whose testLang ≠ `java` (the frontend's hardcoded-`java` ref).
- The scaffolder's forbidden-refs verification fails loudly (per the fail-loud / `check-*` convention) if **any** residual `gh-optivem-<arch>-<lang>.yaml` survives a rewrite — turning this class of bug into a scaffold-time hard failure instead of a CI-time surprise.
- Coverage confirmed across all four apply paths: multitier monorepo (the reported incident), multitier multirepo, monolith monorepo, monolith multirepo.
- Regression tests in `replacements_test.go` asserting commit-stage config rewrite for polyglot multitier + monolith, plus forbidden-ref tests asserting the backendLang-named leftover is now caught.

## ▶ Next executable step (resume here)

Step 1: In `internal/scaffolding/steps/apply_template.go`, make the `GH_OPTIVEM_CONFIG` rewrite language-agnostic in `multitierContentReplacements` (~820-821) and `monolithContentReplacements` (~675-676). Keep the existing `testLang` `-legacy.yaml` rule **first** (so `-legacy.yaml` is not partially matched by the bare `.yaml` rule), then append latest-only rules for all three languages — `gh-optivem-<arch>-{dotnet,java,typescript}.yaml` → `gh-optivem.yaml` (`<arch>` = `multitier` / `monolith`). No extra legacy rules are needed (there is no legacy commit-stage workflow). Then run `go test ./internal/scaffolding/...` to confirm nothing regresses before adding the new tests in Step 4.

## Steps

- [ ] Step 1: **Language-agnostic latest rewrite.** In `multitierContentReplacements` (`apply_template.go` ~820-821) and `monolithContentReplacements` (~675-676), after the existing `testLang` `-legacy.yaml` rule, append latest-only rules mapping `gh-optivem-<arch>-{dotnet,java,typescript}.yaml` → `gh-optivem.yaml`. Preserve ordering: testLang `-legacy.yaml` rule stays first; the testLang `.yaml` rule and the new per-language `.yaml` rules follow. (The testLang `.yaml` rule becomes redundant once all three langs are covered — fold it into the loop or leave it; note the choice.)
- [ ] Step 2: **Verify & fix the multirepo split path.** `applyMultitierMultirepo` (`apply_template.go` ~462-613) builds separate backend/frontend repos via `backendReplacements`/`frontendReplacements` (~565-572, ~602-609) which include **no** `gh-optivem` config rewrite at all. Determine what each split repo's `gh-optivem.yaml` is actually named (read `optivem_yaml.go` / `BuildOptivemYAML`) and what `GH_OPTIVEM_CONFIG` ends up as in their commit-stage workflows. If broken, add the same language-agnostic rewrite to `backendReplacements`/`frontendReplacements`. Also check `applyMonolithMultirepo` (~278-368), which copies a commit-stage workflow to the system repo (~350-368), and `applyMonolithMonorepo` (~218-273).
- [ ] Step 3: **Close the guardrail gap.** Extend `multitierForbiddenRefs` (`apply_template.go` ~1007-1024) and `monolithForbiddenRefs` (~987-1005) to forbid **any** residual `gh-optivem-<arch>-<lang>.yaml` (all three languages, not just `testLang` at lines ~1020 / ~999), so a future regression fails the scaffold loudly rather than shipping a broken repo.
- [ ] Step 4: **Regression tests.** In `internal/scaffolding/steps/replacements_test.go`: (a) assert that for backendLang=java / testLang=typescript multitier, the commit-stage `GH_OPTIVEM_CONFIG` rewrites `gh-optivem-multitier-java.yaml` → `gh-optivem.yaml` (covers both the backendLang backend ref and the hardcoded-java frontend ref); (b) an equivalent monolith lang≠testLang case; (c) forbidden-ref cases asserting the residual `gh-optivem-multitier-java.yaml` / `gh-optivem-monolith-<lang>.yaml` is now caught.
- [ ] Step 5: **Run unit tests.** `go test ./internal/scaffolding/...` in `C:/GitHub/optivem/academy/gh-optivem`; all green.
- [ ] Step 6: **E2E confirmation (heavy, user-run).** Re-run a polyglot scaffold (backend=java, test=typescript) and confirm `backend-commit-stage.yml` and `frontend-commit-stage.yml` both reference `gh-optivem.yaml`, and the commit stage passes. (User initiates — do not self-run.)

## Notes / constraints

- **Do not change the shop workflow.** The frontend's hardcoded `gh-optivem-multitier-java.yaml` (`shop/.github/workflows/multitier-frontend-react-commit-stage.yml:56`) is correct in shop (all 12 configs exist there); the scaffold rewrite is the right layer to fix.
- **Commit-stage is latest-only** — there is no legacy commit-stage workflow, so only the `.yaml` → `gh-optivem.yaml` mapping is needed for the extra languages; the legacy mapping stays `testLang`-keyed (acceptance-stage-legacy is the only legacy ref in copied workflows).
- Root cause confirmed statically: source trace of the replacement rules + reading the live scaffolded repo's actual workflow bodies (`valentinajemuovic/manual-test-1771866363b6983c`). Failing run: https://github.com/valentinajemuovic/manual-test-1771866363b6983c/actions/runs/28102942986

## Open questions

- **Scope of the per-language list:** hardcode the three known languages (`dotnet`/`java`/`typescript`) in the rewrite, or derive from a shared languages constant if one exists in the codebase? (Recommended: reuse an existing constant if present; otherwise the literal three — there are no other flavors.)
- **Multirepo (Step 2):** is the split-repo path actually broken, or do those repos retain the full per-flavor config name (making them fine)? Needs the `optivem_yaml.go` check before deciding whether Step 2 edits anything or is verify-only.
