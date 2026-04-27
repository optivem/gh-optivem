# Plan: Absorb Run-SystemTests orchestration into gh-optivem (PR 2 — shop cutover)

> 🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-04-25T14:23:04Z`

## Status

PR 2 work is committed locally on this branch in two repos: `gh-optivem` (verify.go switched to runner package, dead-code prune/dimensions files removed, runner subcommand cwd derived from `--system` / `--tests` flag) and `shop` (12 JSON configs written, 19 legacy PowerShell test-runner scripts deleted, `_prerelease-pipeline.yml` switched to `gh optivem`, docs and per-lang READMEs rewritten, `run-all-system-tests.sh` added).

End-to-end smoke test for typescript / monolith passed: `gh optivem run system → test system --suite smoke-stub --sample → stop system` cleanly cycles up, runs, and stops.

## Items still open

### Release coordination

Shop's CI installs `gh-optivem` via `gh extension install optivem/gh-optivem`, which pulls from the latest release. **Before merging PR 2:** tag and release a new gh-optivem version that includes the runner package + the runner subcommand cwd derivation from PR 2. (Third-party visible — not auto-executed by the agent.)

### Verification still owed

1. ✅ **Local end-to-end (Windows dev)** — typescript monolith smoke-stub passed via `scripts/manual-test-runner-shop.sh`.
2. ⏳ **Linux CI dry-run** — push the branch, watch `_prerelease-pipeline.yml` go green on ubuntu-latest. Expect to surface the Java cross-platform issue (see "Known follow-ups" below).
3. ⏳ **gh-optivem self-test** — `bash scripts/manual-test.sh --no-cleanup ...` (per `CONTRIBUTING.md:40-43`) to verify the scaffolder copies JSON correctly and the verify step uses the runner package without pwsh.
4. ⏳ **Diff the suite results table** before/after on all 3 langs, to confirm no behavior drift.

## Known follow-ups (not addressed in PR 2)

### Java cross-platform suite commands ✅ resolved 2026-04-25

`tests-*.json` for Java used `.\gradlew.bat ...` literally — Windows-only. Linux CI surfaced this in `meta-prerelease-stage` run [24937323802](https://github.com/optivem/shop/actions/runs/24937323802) (multitier-java + monolith-java both failed in `Setup: Clean Build` with `exec: ".\\gradlew.bat": executable file not found in $PATH`).

Fix: `normalizeExe` in `internal/runner/tests.go` translates Windows wrapper paths to Unix on non-Windows hosts (`.\gradlew.bat` → `./gradlew`). The JSON literals stay Windows-style; the runner resolves at exec time. README note in `system-test/java/README.md` removed.

### Port-clash with other scaffolded projects

The runner's `IsAnyURLUp` probe is necessarily port-based: when another local project ("page-turner" was the case during testing) is bound to shop's ports (3311/8311/9311/3312/8312/9312 for typescript), the runner skips its own restart and may run tests against the wrong stack. Tests still passed against page-turner's stub-mode endpoints because the smoke is just "did the URL respond". This isn't a new problem — same probe pattern existed in the PS1 — but worth documenting.

## Open question — relocate `system.json` + compose files out of `system-test/`?

VJ raised: should the `system.json` config and the `docker-compose.local.*.yml` files live under `shop/docker/` (or similar) rather than `shop/system-test/<lang>/<arch>/`? The reasoning: a docker-compose stack describes the SUT, not the test runner — `system-test/` should hold tests, not infrastructure.

Trade-offs to decide later:
- **Pro relocate:** clearer separation. `system-test/` becomes purely test code + `tests-*.json`. Reduces duplication if all 3 langs share one set of compose files (today they're per-lang because of build contexts and ports differing — but that's solvable with port files / compose `extends`).
- **Pro stay:** the runner resolves compose paths relative to `system.json`'s directory. Moving means cross-tree references. Each lang currently has its own ports (3111/3211/3311), so per-lang compose files are not strictly redundant.
- **Affects scaffolder:** `templates.SelectDockerCompose` flattens `system-test/<lang>/<arch>/` into `system-test/`. If the source lives elsewhere, `apply_template.go`'s copy logic changes.

Not addressed in this PR. File a follow-up issue if pursuing.
