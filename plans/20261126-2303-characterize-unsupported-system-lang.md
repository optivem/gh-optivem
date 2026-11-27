# 2026-11-26 22:03:02 UTC — Let `characterize` run on a system whose language gh-optivem doesn't scaffold

> Step 0 of `20261126-2243-ep-legacy-coverage-rehearsal.md`; that plan waits on this one.

## TL;DR

**Why:** `characterize` writes tests for an existing system and must never change its code, so the system's language does not matter to it. Today it still refuses to run on such a system. The EP repo's system is PHP, and `system.lang` only accepts `java|dotnet|typescript`. A monolith requires `system.{path,repo,lang}` (`validateMonolithExclusivity`, `internal/kernel/projectconfig/config.go`). So EP leaves `system.architecture` unset, and `characterize`'s safety gate (`systemRepoPaths`, `internal/atdd/process/actions/characterize_safety.go`) errors on an unset architecture. It also errors on an empty `system.db-migration-path`, which a system without Flyway-style migrations doesn't have.
**End result:** a `gh-optivem.yaml` with `system.architecture: monolith`, `system.path` + `system.repo` set and **no** `system.lang` validates. `characterize` runs on it. The safety gate watches `system.path` (and `db-migration-path` only when set). Prompts' `${language}` resolves to `system-test.lang`. Commands that do need the system language (scaffolding, building system code) still fail loudly with a clear message.

## Outcomes

- `architecture: monolith` without `system.lang` passes config validation.
- A clear, specific error from every command that really needs `system.lang` when it is unset, naming the command and the fix. Never a silent skip or wrong default.
- The `characterize` safety gate works without `system.db-migration-path`: it watches only the system surface paths and says so in the run log.
- `${language}` in characterize prompts resolves to `system-test.lang` when `system.lang` is unset. That is already the behaviour of `PlaceholderMap`; `primaryLanguage` in `internal/atdd/runtime/driver/driver.go` must match it.
- Docs: `docs/cli-reference.md` (characterize section) and the `gh-optivem.yaml` schema docs say `system.lang` is optional for a system gh-optivem doesn't build.
- EP config can then set `architecture: monolith` and drop the "no php lang" workaround comment. That is a change in the EP repo, made in the rehearsal plan, not here.

## ▶ Next executable step (resume here)

Step 1: enumerate every reader of `cfg.System.Lang` / `primaryLanguage` / monolith `IsValidLang` (grep `System.Lang`, `primaryLanguage`, `"language"` across `internal/`, excluding tests). Classify each as "needs the system language" (must fail loudly when unset) or "doesn't" (falls back to `system-test.lang` or ignores it). Write the table into this plan before changing any code.

## Steps

- [ ] Step 1: **Enumerate readers** of `System.Lang` and classify them (see above). Known so far:
  - `validateMonolithExclusivity` (config.go:904): requires lang. Relax.
  - `PlaceholderMap` (config.go:712): already falls back to `SystemTest.Lang`.
  - `primaryLanguage` (driver.go): returns `System.Lang` on monolith. Add the same fallback.
  - `internal/config/config.go` `IsValidLang` / `--monolith-lang`: `init` flags. Out of scope, since `init` scaffolds the system and needs it.
- [ ] Step 2: **Relax validation.** Monolith requires `system.{path,repo}`. `lang` is optional but, when set, still enum-checked (Rule 3 unchanged). Update `validate_characterization_test.go` and the monolith validation tests.
- [ ] Step 3: **Fail loudly where the language is needed.** Each reader classified "needs it" in Step 1 gets an explicit error when `System.Lang` is empty, e.g. `system.lang is unset in gh-optivem.yaml; <command> builds/scaffolds the system and needs it (java|dotnet|typescript)`.
- [ ] Step 4: **`primaryLanguage` fallback** to `SystemTest.Lang` when the monolith `System.Lang` is empty. Extend the clauderun render-matrix test (`TestRenderMatrix_NoUnfilledPlaceholders`) with a monolith-without-lang seed.
- [ ] Step 5: **Optional `db-migration-path` in the characterize safety gate.** `systemRepoPaths` includes `DbMigrationPath` only when set. When it is unset, it logs one line that migrations are not watched. Unit test both cases. `scope.go`'s `system-db-migration-path` layer error stays as is (implement flow, not characterize).
- [ ] Step 6: **Docs.** Update the characterize section of `docs/cli-reference.md` and the config schema docs for optional `system.lang` and `db-migration-path`.
- [ ] Step 7: **Tests.** Run the touched packages with `-p 2` (or `scripts/test.sh`), never unbounded `go test ./...`.

## Decisions (2026-11-26)

- **Make `system.lang` optional; don't add `php`.** Adding `php` to the enum would make `${language}` resolve to `php` in test-side prompts, which are about the TypeScript/Java/.NET test code. It would also imply gh-optivem can scaffold and build PHP, which it can't. Optional lang plus a loud failure where a command really needs it keeps the black-box case honest.
- `init`'s `--monolith-lang` stays required: `init` builds the system.
