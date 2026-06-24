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

Step 1: In `internal/scaffolding/steps/apply_template.go`, add a package-local `scaffoldLangs = []string{"dotnet", "java", "typescript"}` and a helper `optivemConfigRewrites(arch, testLang string) [][2]string` (see **End logic** below). Use it to replace the two testLang-keyed rewrite lines in `multitierContentReplacements` (~820-821) and `monolithContentReplacements` (~675-676) — splice in `optivemConfigRewrites("multitier", testLang)...` / `optivemConfigRewrites("monolith", testLang)...`. Then run `go test ./internal/scaffolding/...` to confirm nothing regresses before adding the new tests in Step 4.

## End logic (resolved design)

One shared helper drives all four apply paths:

```go
var scaffoldLangs = []string{"dotnet", "java", "typescript"}

// optivemConfigRewrites flattens every per-flavor GH_OPTIVEM_CONFIG name to the
// canonical scaffolded pair. Legacy (testLang-keyed) first, then every per-language
// latest name -> gh-optivem.yaml. Commit-stage workflows name the config by
// backend/system language; pipeline stages name it by testLang — covering all
// languages catches both. arch is "monolith" or "multitier".
func optivemConfigRewrites(arch, testLang string) [][2]string {
	r := [][2]string{
		{"gh-optivem-" + arch + "-" + testLang + "-legacy.yaml", "gh-optivem.legacy.yaml"},
	}
	for _, lang := range scaffoldLangs {
		r = append(r, [2]string{"gh-optivem-" + arch + "-" + lang + ".yaml", "gh-optivem.yaml"})
	}
	return r
}
```

The all-langs latest rules subsume the old testLang `.yaml` rule; only one legacy file exists (testLang), so one legacy rule kept first preserves the existing anti-partial-match ordering. A symmetric helper (or inline loop over `scaffoldLangs`) replaces the single testLang entry in the forbidden-refs.

**Confirmed breakage matrix** (all four paths need the rewrite):

| Apply path | Current commit-stage config rewrite | Broken when |
|---|---|---|
| multitier monorepo (`applyMultitierMonorepo`) | testLang-keyed only (820-821) | polyglot, or testLang≠java (frontend hardcoded-java) |
| multitier multirepo (`applyMultitierMultirepo`) | **none** (565-572, 602-609) | **always** (every combo) |
| monolith monorepo (`applyMonolithMonorepo`) | testLang-keyed only (675-676) | lang≠testLang |
| monolith multirepo (`applyMonolithMultirepo`) | **none** (sysContentReplacements 359-367) | **always** (every combo) |

`WriteOptivemYAML` (`optivem_yaml.go:62-70`) writes canonical `gh-optivem.yaml`/`gh-optivem.legacy.yaml` into every split repo, so the multirepo paths genuinely need the rewrite — they are not coincidentally correct.

## Steps

- [ ] Step 1: **Add `scaffoldLangs` + `optivemConfigRewrites` helper and wire monorepo paths.** In `apply_template.go`, splice `optivemConfigRewrites("multitier", testLang)...` into `multitierContentReplacements` (replacing ~820-821) and `optivemConfigRewrites("monolith", testLang)...` into `monolithContentReplacements` (replacing ~675-676).
- [ ] Step 2: **Wire the multirepo split paths.** Append `optivemConfigRewrites("multitier", testLang)...` to `backendReplacements` (~565-572) and `frontendReplacements` (~602-609) in `applyMultitierMultirepo`, and `optivemConfigRewrites("monolith", testLang)...` to `sysContentReplacements` (~359-367) in `applyMonolithMultirepo`. (Confirmed broken in all combos — these had no config rewrite at all.)
- [ ] Step 3: **Close the guardrail gap.** In `multitierForbiddenRefs` (~1007-1024) and `monolithForbiddenRefs` (~987-1005), replace the single testLang config entry (~1020 / ~999) with a loop forbidding `"gh-optivem-"+arch+"-"+lang` for every `scaffoldLangs`, so any residual per-flavor name fails the scaffold loudly. **Also forbid the post-Sonar-mangle residual** `"gh-optivem-system.yaml"` in `monolithForbiddenRefs` — because `checkNoTemplateRefs` runs *after* the Sonar-key pass, a future regression that lets `gh-optivem-monolith-<lang>.yaml` survive the content pass would reach the guardrail already rewritten to `gh-optivem-system.yaml`, which the per-lang `gh-optivem-monolith-<lang>` needles do not match. (Monolith only; multitier has no analogous mangle.)
- [ ] Step 4: **Regression tests.** In `internal/scaffolding/steps/replacements_test.go`: (a) assert that for backendLang=java / testLang=typescript multitier, the commit-stage `GH_OPTIVEM_CONFIG` rewrites `gh-optivem-multitier-java.yaml` → `gh-optivem.yaml` (covers both the backendLang backend ref and the hardcoded-java frontend ref); (b) the monolith `lang=java` / `testLang=typescript` case (the CI incident) asserting the commit-stage config rewrites `gh-optivem-monolith-java.yaml` → `gh-optivem.yaml` **and** that no `gh-optivem-system.yaml` survives the full apply (content + Sonar passes); (c) forbidden-ref cases asserting the residual `gh-optivem-multitier-java.yaml` (multitier) and the post-mangle `gh-optivem-system.yaml` (monolith) are now caught — run the forbidden-ref scan against a repo that has been through the Sonar pass, so the monolith assertion exercises the real post-mangle residual rather than the pre-mangle `gh-optivem-monolith-java.yaml`.
- [ ] Step 5: **Run unit tests.** `go test ./internal/scaffolding/...` in `C:/GitHub/optivem/academy/gh-optivem`; all green.
- [ ] Step 6: **E2E confirmation (heavy, user-run).** Re-run a polyglot scaffold (backend=java, test=typescript) and confirm `backend-commit-stage.yml` and `frontend-commit-stage.yml` both reference `gh-optivem.yaml`, and the commit stage passes. (User initiates — do not self-run.)

## Notes / constraints

- **Do not change the shop workflow.** The frontend's hardcoded `gh-optivem-multitier-java.yaml` (`shop/.github/workflows/multitier-frontend-react-commit-stage.yml:56`) is correct in shop (all 12 configs exist there); the scaffold rewrite is the right layer to fix.
- **Commit-stage is latest-only** — there is no legacy commit-stage workflow, so only the `.yaml` → `gh-optivem.yaml` mapping is needed for the extra languages; the legacy mapping stays `testLang`-keyed (acceptance-stage-legacy is the only legacy ref in copied workflows).
- Root cause confirmed statically: source trace of the replacement rules + reading the live scaffolded repo's actual workflow bodies (`valentinajemuovic/manual-test-1771866363b6983c`). Failing run (multitier): https://github.com/valentinajemuovic/manual-test-1771866363b6983c/actions/runs/28102942986
- Monolith second-hop confirmed in the gh-acceptance-stage smoke matrix: run 28113696564, job `Smoke (monolith, monorepo, java)` → downstream scaffolded commit-stage run 28113897850 failed at *Compile Code* with `ERROR: no gh-optivem.yaml at gh-optivem-system.yaml`. Trace: shop `monolith-java-commit-stage.yml:56` = `gh-optivem-monolith-java.yaml` → survives testLang-keyed content pass → `monolithSonarKeyReplacements("java")` `-monolith-java`→`-system` mangles it to `gh-optivem-system.yaml`.

## Open questions

- **Forbidden-ref false positives:** broadening the guardrail to all three languages must not flag legitimate content in a scaffolded repo. Low risk (the existing testLang ref already scans clean; this is symmetric) — validate during the Step 6 E2E.
- **Is multitier multirepo a supported/shipped strategy?** Step 2 fixes it regardless, but confirm it is exercised (not deprecated) so the fix is worth the test coverage.
