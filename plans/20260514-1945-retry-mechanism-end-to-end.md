# Plan: end-to-end retry mechanism — consolidate, harden, audit, close gaps

> 🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-05-14T19:31:10Z`

Single source of truth for retry-related work in this workspace. Supersedes
the four standalone plans listed below by folding their goals, file lists,
and verification steps into one phased program.

## Outcome — what will be true when all 6 phases complete

**One canonical retry policy across the workspace.** Adding a new transient
pattern (HTTP 5xx, DNS hiccup, GOAWAY, etc.) becomes a *single* edit to one
canonical file plus one `sync-shared.sh` run, instead of mirroring across
five copies (`actions/shared/gh-retry.sh`, `shop/.github/workflows/scripts/gh-retry.sh`,
`gh-optivem/.github/scripts/gh-retry.sh`, `gh-optivem/internal/shell/ghretry.go`,
and a future `sonar-retry.sh`). The "lockstep edit" pain documented in the
superseded 1830 and 1850 plans is eliminated.

**Two recent incidents resolved.**
- SonarCloud 504 from acceptance run 25865827466 → sonar uploads now retry
  with bounded backoff and hard-fail pass-through on 4xx.
- GitHub GraphQL transient from acceptance run 25877369208 → `gh project ...`
  calls now retry; the misleading secondary "Commit and push" panic that
  followed it is also suppressed.

**Auditable retry coverage everywhere.** Every external-I/O call in
`gh-optivem`'s Go code and shop's 88 workflow files is either retrying via
the shared engine or explicitly documented as not needing retry. Two audit
reports plus two follow-up fix plans land alongside the infrastructure.

### Workspace files at end-state

| Location | What |
|---|---|
| `optivem/actions/shared/` | `retry-core.sh` (~80 LOC generic engine), `gh-retry.sh` / `docker-retry.sh` / `sonar-retry.sh` (~15 LOC wrappers each, declare regex + delegate), plus matching `_test-*.sh` files |
| `optivem/actions/scripts/sync-shared.sh` | Idempotent script that vendors the 4 helpers into shop and gh-optivem on demand |
| `optivem/shop/.github/workflows/scripts/` | 4 vendored helpers (`retry-core` + `gh-retry` + `docker-retry` + `sonar-retry`), each with a generated-from-SHA banner |
| `optivem/shop/.github/workflows/{monolith-dotnet,multitier-backend-dotnet,monolith-java,multitier-backend-java}-commit-stage.yml` | Source `sonar-retry.sh` and wrap the analysis-upload call in `sonar_retry ...` |
| `optivem/gh-optivem/internal/shell/retrycore.go` | New — generic `RetryWithPolicy(transient, hardFail, prefix, fn)`. Mirrors the bash factoring |
| `optivem/gh-optivem/internal/shell/ghretry.go` | Refactored to ~30 LOC against `RetryWithPolicy`; adds `RunCaptureWithRetry`; `ghRetryTransient` regex covers GitHub GraphQL wording |
| `optivem/gh-optivem/internal/steps/project.go` | Three `projectRunCapture` call sites routed through `RunCaptureWithRetry` via the existing test seam |
| `optivem/gh-optivem/internal/steps/finalize.go` | `commitAndPushRepo` skips cleanly when local dir is absent (no more misleading `chdir ... no such file or directory` panic) |
| `optivem/gh-optivem/.github/scripts/` | 4 vendored helpers, identical to shop's |
| `optivem/gh-optivem/.claude/agents/workflow-auditor.md` | New §N rubric section covering external-I/O retry coverage (N-A / N-B / N-C / R-OK categories + anti-pattern list) |
| `optivem/gh-optivem/audits/<date>-external-call-retry-coverage.md` | New audit report — every Go external-call site classified |
| `optivem/gh-optivem/audits/<date>-shop-workflow-retry-coverage.md` | New audit report — every shop workflow external-I/O step classified |
| `optivem/gh-optivem/plans/<date>-fix-gh-optivem-retry-gaps.md` | Follow-up plan listing R-MISSING Go fixes (executed in Phase 6) |
| `optivem/gh-optivem/plans/<date>-fix-shop-workflow-retry-gaps.md` | Follow-up plan listing R-MISSING shop workflow fixes (executed in Phase 6) |

### Maintenance after this plan lands

- **New transient pattern observed in CI logs** → one edit to canonical
  `retry-core.sh` (or a `<tool>-retry.sh` if tool-specific) + run
  `sync-shared.sh` + commit per affected repo via `/commit`. No mirror edits.
- **New external tool needs retry** → drop in a ~15-line
  `<tool>-retry.sh` wrapping `retry_with_policy` with the tool's regex,
  re-run `sync-shared.sh`. For Go callers, add a thin function passing the
  tool's regex to `RetryWithPolicy`.
- **Sync drift detection** → vendored files carry a generated-from-SHA
  banner that flags mismatch on visual inspection; a CI lint enforcing
  this is logged as out-of-scope follow-up but easy to add later.

## Supersedes

- `plans/20260514-1830-harden-init-graphql-transients.md` — the GraphQL-transient
  smoke-test failure plus the always-run "Commit and push" panic. Concrete Go
  fixes here become Phase 3 below.
- `plans/20260514-1845-audit-gh-optivem-retry-coverage.md` — read-only audit of
  external-call retry coverage in the Go binary. Becomes Phase 4 below.
- `plans/20260514-1850-audit-shop-workflows-retry-coverage.md` — read-only audit
  of external-I/O retry coverage in shop's 88 workflow files plus composites.
  Becomes Phase 5 below.
- `plans/20260514-1930-consolidate-retry-mechanism.md` — the shared-engine
  architecture itself. Becomes Phases 1-2 below.
- `shop/plans/20260514-1900-sonar-retry-on-504.md` — the inline-loop sonar fix
  proposal (also superseded by Phase 2 below, which uses the new shared engine
  instead of inline loops). Will be deleted as part of Phase 2 step 3.

## Intentionally NOT touched

- `audits/20260514-silent-external-call-failures.md` — orthogonal, complete
  (H1–H5 fixed). That work was about *what* errors contain (lossy stdio
  teeing); this plan is about *whether* the call is retried.

## Why this exists

Transient external-call failures surface in the workspace "every few days":
SonarCloud 504s on `analysis/analyses`, GitHub GraphQL internal errors
(`Something went wrong while executing your query`), docker registry GOAWAYs,
DNS hiccups, TLS handshake timeouts, gradle daemon timeouts, ETIMEDOUT on
npm/dotnet feed fetches, etc.

Today the retry policy is near-identically copy-pasted across five places:

- `optivem/actions/shared/gh-retry.sh`
- `optivem/actions/shared/docker-retry.sh`
- `optivem/shop/.github/workflows/scripts/gh-retry.sh`
- `optivem/gh-optivem/.github/scripts/gh-retry.sh`
- `optivem/gh-optivem/internal/shell/ghretry.go` (Go port)

The 20260514-1830 plan explicitly documented the in-lockstep pain
(*"must be updated in lockstep"*); the 20260514-1850 plan explicitly noted
*"the two audits share a regex... must list the same alternatives."*
Adding a new transient pattern today requires editing five places. The
maintenance burden is the real cost.

Recent triggers in the same day:

- gh-acceptance-stage run [25865827466][1] — 4 of 4 dotnet matrix combos
  failed on SonarCloud 504 (drove the original consolidation discussion).
- gh-acceptance-stage run [25877369208][2] — all 4 smoke jobs failed on a
  GitHub GraphQL transient at `Ensure project board`, plus a confusing
  secondary panic in always-run `Commit and push`.

This plan consolidates the retry infrastructure once, then applies it to
both immediate failures and the systematic coverage audits.

[1]: https://github.com/optivem/gh-optivem/actions/runs/25865827466
[2]: https://github.com/optivem/gh-optivem/actions/runs/25877369208

## Goals (priority order)

1. **Eliminate per-repo retry duplication.** One canonical engine, one
   editing point per regex change, sync script to vendor into each repo
   that needs runtime self-containment.
2. **Fix the immediate sonar-504 failure** in shop's dotnet/java
   commit-stage workflows, using the new shared engine (not the inline-loop
   approach proposed in superseded plan 1900).
3. **Fix the immediate GraphQL-transient failure** in gh-optivem's
   `EnsureProjectBoard`, plus the misleading secondary "Commit and push"
   panic, using the new shared engine.
4. **Audit gh-optivem Go for retry-coverage gaps.** Read-only sweep, report,
   follow-up fix list.
5. **Audit shop workflows for retry-coverage gaps.** Read-only sweep, report,
   follow-up fix list, plus a `workflow-auditor` rubric addition so future
   audits inherit the rule.
6. **Execute the fix lists produced by Phases 4 and 5.** Each R-MISSING site
   becomes a thin wrapper swap against the shared engine.

## End-state architecture

### `optivem/actions/shared/` (canonical bash source)

| File | Purpose | Approx LOC |
|---|---|---|
| `retry-core.sh` | Generic engine. Exposes `retry_with_policy <transient_re> <hard_fail_re> <prefix> -- <cmd...>`. Owns: mktemp dance, attempt loop, 5s → 15s → 45s backoff, hard-fail pass-through, `::notice::`/`::warning::` annotations | ~80 |
| `gh-retry.sh` | Declares gh-specific transient + hard-fail regex, defines `gh_retry "$@"` delegating to `retry_with_policy` | ~15 |
| `docker-retry.sh` | Same shape, docker-specific regex | ~15 |
| `sonar-retry.sh` | **NEW** — same shape, sonarscanner-specific regex (`Error 5[0-9][0-9] on https://`, `Endpoint request timed out`, plus generic network) | ~15 |
| `_test-retry-core.sh` | **NEW** — direct unit smoke test of the engine | ~30 |
| `_test-gh-retry.sh`, `_test-docker-retry.sh` | Existing tests; must stay green through refactor | unchanged |
| `_test-sonar-retry.sh` | **NEW** — smoke test for sonar wrapper | ~30 |

### `optivem/actions/scripts/sync-shared.sh` (NEW)

Idempotent script. Reads `optivem/actions/shared/{retry-core,gh-retry,docker-retry,sonar-retry}.sh`
and writes them into:

- `../shop/.github/workflows/scripts/`
- `../gh-optivem/.github/scripts/`

Each vendored file gets a generated-from-X-at-SHA banner. Run manually after
editing the engine or any wrapper, then commit per repo via the usual
`/commit` skill.

### `optivem/gh-optivem/internal/shell/` (Go side)

| File | Change |
|---|---|
| `retrycore.go` | **NEW** — generic `RetryWithPolicy(transient, hardFail *regexp.Regexp, prefix string, fn func() error) error`. Mirrors the bash factoring |
| `ghretry.go` | Refactor: keep the gh-specific regex constants, body becomes ~30 lines calling `RetryWithPolicy`. Also adds `RunCaptureWithRetry` (per 1830 (A)) and extends `ghRetryTransient` with the GraphQL alternative (per 1830 (B)) |
| `ghretry_test.go` | Must stay green; no exported-API changes — `GhRetry`/`RunWithRetry` callers untouched. Adds case for the new GraphQL wording (per 1830 (B) verification) |
| `retrycore_test.go` | **NEW** — unit tests for the engine (synthetic transient/hard-fail/timeout behaviour using injectable sleep + fake fn) |

### `optivem/gh-optivem/.claude/agents/workflow-auditor.md` (rubric addition)

Per the 1850 plan deliverable: add a new §N section to the rubric covering
external-I/O retry coverage, with N-A / N-B / N-C / R-OK categories. Future
audits inherit the rule.

## Phases

Total: ~10-12 commits across 3 repos plus 2 audit reports plus follow-up
fix plans. Push order across phases: **actions → gh-optivem → shop**,
preserving cross-repo reference validity.

### Phase 1 — Build shared retry infrastructure (`optivem/actions`)

Foundation. Everything downstream depends on it.

1. **`1a`** Add `actions/shared/retry-core.sh`. Refactor `gh-retry.sh` and
   `docker-retry.sh` to delegate to `retry_with_policy`. Run
   `_test-gh-retry.sh` and `_test-docker-retry.sh` locally — they must pass
   unchanged. Add `_test-retry-core.sh`.
2. **`1b`** Add `actions/shared/sonar-retry.sh` + `_test-sonar-retry.sh`.
3. **`1c`** Add `actions/scripts/sync-shared.sh`. Run it locally to vendor
   helpers into `../shop/.github/workflows/scripts/` and
   `../gh-optivem/.github/scripts/`. Working trees in shop and gh-optivem
   are now dirty (synced files appear) but not yet committed.
4. Commit/push `optivem/actions` via `/commit`.

### Phase 2 — Immediate sonar-504 fix (`optivem/shop`)

Closes the original incident from acceptance run 25865827466.

5. **`2a`** Revert the 4 inline retry loops added earlier this session in:
   - `monolith-dotnet-commit-stage.yml`
   - `multitier-backend-dotnet-commit-stage.yml`
   - `monolith-java-commit-stage.yml`
   - `multitier-backend-java-commit-stage.yml`
6. **`2b`** Replace each with `source "$GITHUB_WORKSPACE/.github/workflows/scripts/sonar-retry.sh"`
   + `sonar_retry <scanner command>`. For java workflows, also split
   `./gradlew build sonar --info` into separate `build` then retry-wrapped
   `sonar` calls.
7. **`2c`** Delete `shop/plans/20260514-1900-sonar-retry-on-504.md` (the
   inline-loop plan, superseded).
8. Commit/push `optivem/shop` via `/commit`. Synced bash helpers from
   Phase 1c land in the same commit.

### Phase 3 — Immediate GraphQL fix + Go-side consolidation (`optivem/gh-optivem`)

Folds the three changes from superseded plan 1830 into the consolidated Go
infrastructure. Closes the incident from acceptance run 25877369208.

9. **`3a` — Engine extraction.** Add `internal/shell/retrycore.go` with
   `RetryWithPolicy`. Refactor `ghretry.go` against it — body becomes ~30
   lines. Existing exports `GhRetry`, `RunWithRetry`, `MustRunWithRetry`,
   `MustRunStdinWithRetry`, `MustRunPostCreate` keep byte-identical
   signatures. Run `go test ./internal/shell/...` — `ghretry_test.go` must
   pass unchanged. Add `retrycore_test.go`.

10. **`3b` — From 1830 (A): `RunCaptureWithRetry` + project.go seam.** In
    `internal/shell/ghretry.go`, add:
    ```go
    func RunCaptureWithRetry(cmdStr, cwd string) (string, error) {
        return runWithRetryLoop(
            func() (string, error) { return RunCapture(cmdStr, cwd) },
            classifyGHError,
            ghRetryAttempts,
            ghRetryDelays,
        )
    }
    ```
    In `internal/steps/project.go:48`, repoint the test seam:
    ```go
    projectRunCapture = shell.RunCaptureWithRetry
    ```
    The three call sites (lines 255, 272, 293) keep their syntax. The test
    stub in `project_test.go:54-65` is unaffected (replaces
    `projectRunCapture` wholesale).

11. **`3c` — From 1830 (B): GraphQL transient wording.** In
    `internal/shell/ghretry.go:24-32`, extend `ghRetryTransient` with the
    alternative `Something went wrong while executing your query`. Add a
    matching `TestClassifyGHError` case in `ghretry_test.go`.

12. **`3d` — From 1830 (C): skip Commit-and-push cleanly.** In
    `internal/steps/finalize.go`, add an early return at the top of
    `commitAndPushRepo` (line 129):
    ```go
    if _, err := os.Stat(repoDir); os.IsNotExist(err) {
        log.Infof("Skipping commit/push for %s: local dir %s not created (earlier step failed before clone)",
            fullRepo, repoDir)
        return
    }
    ```

13. **`3e` — Bash regex parity.** Add `Something went wrong while executing
    your query` to **canonical** `actions/shared/gh-retry.sh` (one edit, in
    the actions repo). Run `actions/scripts/sync-shared.sh`. The synced copy
    in `gh-optivem/.github/scripts/gh-retry.sh` updates automatically — no
    manual mirror edit. (This is the consolidation paying back immediately.)

14. **`3f` — Confirm synced bash helpers.** Vendored files from Phase 1c
    plus the regex sync from 3e are now in `.github/scripts/`; confirm via
    the generated-from banner SHA that they match canonical.

15. Commit/push `optivem/gh-optivem` via `/commit`. Logical commits:
    (i) engine extraction (3a),
    (ii) GraphQL transient + project.go seam + finalize skip (3b+3c+3d),
    (iii) synced bash helpers (3e+3f — could roll into i or ii).

### Phase 4 — Audit gh-optivem Go for retry-coverage gaps

Read-only sweep. From superseded plan 1845.

16. Use the `code-auditor` agent (scoped to `internal/**` and `main.go`).
    Brief it on the retry-coverage pitfall pattern (it has prior experience
    with the silent-stderr sweep that produced
    `audits/20260514-silent-external-call-failures.md`).

17. Audit method per call site:
    - **Identify** invocations of `shell.Run`, `MustRun`, `RunCapture`,
      `RunStdin`, `RunPassthrough` (and `Must*` variants); direct
      `exec.Command(...)` outside the `shell` package; `net/http` calls.
    - **Classify by command kind**: GH API / Git remote / SonarCloud HTTP
      (all "must retry on transient") vs. Local git / Local tools / Probes
      designed to fail (all "no retry").
    - **Classify retry state**: R-OK (already retrying) / R-MISSING
      (external without retry) / R-DOC-OK (local or probe, no retry needed).
    - For each R-MISSING site, recommend the smallest correct wrapper swap.

18. **Deliverable**: audit report at
    `audits/<date>-external-call-retry-coverage.md`, structured like
    `audits/20260514-silent-external-call-failures.md`:
    TL;DR / Findings table (High/Medium/Low) / Healthy patterns /
    Recommended order of fixes / Counts (≤20). Plus a follow-up plan file
    `plans/<date>-fix-gh-optivem-retry-gaps.md` listing each R-MISSING
    site as a discrete edit.

19. **Highest-priority candidate** to flag (per 1845): `internal/shell/sonarcloud.go`
    — direct `net/http` calls to `sonarcloud.io/api/*`, not currently
    wrapped in any retry. After consolidation, these can adopt
    `RetryWithPolicy` using a generic regex.

### Phase 5 — Audit shop workflows for retry-coverage gaps

Read-only sweep. From superseded plan 1850.

20. **Update the rubric first.** Add a new §N section to
    `.claude/agents/workflow-auditor.md` covering external-I/O retry
    coverage. Categories:
    - **N-A** — `gh` call without retry. Recommendation: source
      `gh-retry.sh` and switch to `gh_retry ...`.
    - **N-B** — Other network call without retry (`curl`, `wget`,
      `docker pull/push`, `npm install`, `mvn deploy`, `dotnet restore`,
      SonarCloud scan upload). Recommendation: switch to the matching
      `<tool>_retry` wrapper from the shared engine, or wrap with
      `nick-fields/retry@v3` for non-standard tools.
    - **N-C** — Retry present but misconfigured (aggressive schedule, masks
      4xx, wraps a hard-fail probe). Recommendation: align with the shared
      engine's 4×{5,15,45} schedule and hard-fail pass-through.
    - **Anti-patterns**: `continue-on-error: true` as retry substitute;
      `if: failure()` blocks that re-run non-idempotent work; long-running
      commands wrapped in retry with no timeout.

21. **Run the audit** via the `workflow-auditor` agent (scoped to
    `../shop/.github/workflows/` plus the composite actions under
    `../actions/**/action.yml`).

22. **Deliverable**: audit report at
    `audits/<date>-shop-workflow-retry-coverage.md`, structured like
    `20260514-silent-external-call-failures.md`. Plus a follow-up plan file
    `plans/<date>-fix-shop-workflow-retry-gaps.md`.

### Phase 6 — Execute fix plans from Phases 4 and 5

23. Open the two follow-up plan files produced by Phases 4 and 5. Each
    R-MISSING site becomes a thin wrapper swap against the shared engine
    (`shell.RunCaptureWithRetry`, `gh_retry`, `docker_retry`, `sonar_retry`,
    or `retry_with_policy` for non-canonical commands).

24. Land fixes per-repo using `/commit`, in priority order from each audit's
    "Recommended order of fixes" section. Composite-level fixes land before
    leaf-workflow fixes (they cascade).

## Validation

### Phase 1 (actions infrastructure)

- `actions/shared/_test-gh-retry.sh`, `_test-docker-retry.sh`,
  `_test-retry-core.sh`, `_test-sonar-retry.sh` all pass locally.
- Refactored `gh-retry.sh` and `docker-retry.sh` preserve exact behaviour
  of the originals — the existing test files are the safety net.

### Phase 2 (sonar fix)

- Trigger `gh-acceptance-stage` against the shop PR branch:
  ```
  gh workflow run gh-acceptance-stage -f shop-ref=<branch> -R optivem/gh-optivem
  ```
  Expect all 8 dotnet/java matrix combos to pass (or pass first try with
  no retries logged if Sonar is healthy — no negative test possible without
  an outage).

### Phase 3 (Go consolidation + GraphQL fix)

- `go build ./...` and `go test ./...` pass clean from repo root.
- `go test ./internal/shell/...` verifies the new classifier case and that
  `RunCaptureWithRetry` compiles and threads through.
- `go test ./internal/steps/...` verifies the project.go seam swap is
  invisible to existing tests.
- Re-dispatch the failed `gh-acceptance-stage` run (or run
  `gh optivem init` against a throwaway repo). If the GraphQL blip recurs,
  expect a `[gh-retry] attempt N/4 failed, retrying in 5s` warning followed
  by success.
- To exercise (3d) deterministically: temporarily make `EnsureProjectBoard`
  fail (e.g. set `cfg.Owner` to a non-existent user via local edit). The
  summary should report exactly one error — `Ensure project board` — and no
  `git add failed: chdir ... no such file or directory` line.
- **Bash sync check**: grep `_GH_RETRY_RETRYABLE` in
  `.github/scripts/gh-retry.sh` against `ghRetryTransient` in
  `internal/shell/ghretry.go` — the alternative lists must match.

### Phase 4 (gh-optivem audit)

- `go build ./...` and `go test ./...` remain green (audit is read-only).
- The audit's R-OK and R-MISSING lists together cover every external-call
  site grep finds — no "unclassified" residue.
- Spot-check 3 R-DOC-OK sites to confirm classification.

### Phase 5 (shop audit)

- Audit report classifies every external-I/O step found — no
  "unclassified" residue.
- Cross-check against the shared engine: every R-OK site sources one of the
  `<tool>-retry.sh` helpers (or uses a Marketplace action with documented
  internal retry).
- Spot-check 3 N-A findings by reading the workflow + composite to confirm
  no upstream wrapper exists.

### Phase 6 (fix execution)

- `go test ./internal/shell/...` for each new Go wrapper added.
- `go test ./internal/steps/...` to confirm seam swaps are invisible.
- For each composite changed in shop: run the smallest workflow that uses
  it via `workflow_dispatch` against a throwaway branch and confirm green.
- For each N-A leaf-workflow fix: re-dispatch on a test repo.
- One targeted failure-injection per fix family (point a `gh api` call at a
  non-existent endpoint, confirm retry logs surface, revert).

## Optional negative test (anytime after Phase 2)

On a throwaway shop branch, point `sonar.host.url` at `https://example.invalid`
in one workflow; confirm the new annotation appears:

```
::notice::[sonar-retry] attempt 1 failed (exit 1): ... -- retrying in 5s
::notice::[sonar-retry] attempt 2 failed (exit 1): ... -- retrying in 15s
::notice::[sonar-retry] attempt 3 failed (exit 1): ... -- retrying in 45s
::warning::[sonar-retry] exhausted 4 attempts (exit 1): ...
```

before the step exits 1. Do not commit the injection.

## Risks and mitigations

- **Bash arg-passing bugs across function levels.** The
  `retry_with_policy <args> -- <cmd...>` shape relies on careful `$@`
  handling. Mitigation: existing `_test-gh-retry.sh` and `_test-docker-retry.sh`
  exercise real `gh`/`docker` invocations with `$(gh_retry ...)` capture
  and `if ! gh_retry ...; then` flow control — they catch arg-passing
  regressions.

- **Sync drift.** A vendored copy is edited directly in shop or gh-optivem,
  not via the canonical source. Mitigation: generated-from-X-at-SHA banner
  at top of each vendored file. Could be reinforced later with a CI lint
  comparing the vendored file's banner SHA against the canonical (out of
  scope here; logged in "Out of scope" below).

- **Cross-repo push ordering.** If shop's PR is reviewed before actions's
  sync-shared.sh exists, the vendored files in shop's PR look like
  unexplained new code. Mitigation: push order actions → gh-optivem → shop;
  link shop's PR to the actions PR.

- **Go refactor regresses callers.** `ghretry.go`'s exported functions
  (`GhRetry`, `RunWithRetry`, etc.) are called from several places in the
  CLI. API of exported functions must stay byte-identical. Mitigation:
  `ghretry_test.go` covers the exported surface; do not change its
  inputs/outputs.

- **Phase 3 interleaving with Phase 1.** 1830-derived edits (Phase 3b/3c)
  reference line numbers in `ghretry.go` that change after the Phase 3a
  engine extraction. Mitigation: execute Phase 3a first, re-resolve line
  numbers in 3b/3c against the refactored file before editing.

- **Audit cap overflow.** Each audit is capped at 20 findings per the
  agent contracts. Mitigation: if either audit hits the cap, the agent
  emits an overflow file-count note and the follow-up plan calls for a
  second audit pass against the remainder.

## Deferred / follow-up topics

Intentionally not in this plan, but worth tracking and likely to be picked
up later as separate plan files. Each entry includes a trigger that should
prompt revisiting it.

- **TS commit-stage workflows.** Three workflows
  (`monolith-typescript-commit-stage.yml`,
  `multitier-backend-typescript-commit-stage.yml`,
  `multitier-frontend-react-commit-stage.yml`) use
  `SonarSource/sonarqube-scan-action@v7` — a `uses:` step that inline
  `sonar_retry` can't wrap. **Trigger to revisit:** a TS combo observably
  504s in CI. **Options when picked up:** (a) replace the action with
  `npx sonar-scanner` + `sonar_retry` (preferred — consistent with the
  bash pattern), or (b) wrap with `Wandalen/wretry.action@v3` (would
  re-introduce the multi-mechanism split this plan eliminates, so avoid).
- **CI lint enforcing sync-banner SHA matches canonical.** Each vendored
  helper carries a `generated-from-X-at-SHA` banner; today verification
  is visual. A small lint workflow could compare each vendored copy's
  banner SHA against the canonical file's current SHA and fail the build
  on mismatch. **Trigger to revisit:** any observed instance of a vendored
  copy drifting from canonical, or the first time `sync-shared.sh` is
  forgotten and the regex update only lands in some repos.
- **`shop/system/.../run-sonar.sh` local helpers.** Local-dev-only;
  not invoked by CI. **Trigger to revisit:** a developer reports a
  transient 504 hitting them locally and asks for retry parity. Low
  priority either way.
- **TypeScript port of `retry-core` for future TS/JS code in shop.**
  Today shop's TS code shells via npm/npx which has its own retry
  contract. **Trigger to revisit:** new TS code in any consumer repo
  starts shelling to `gh`, `docker`, or a registry directly and observes
  transients.
- **Retry audits for repos other than `shop`** (e.g. `optivem/optivem-testing`,
  `optivem/courses`, `optivem/hub`). **Trigger to revisit:** smoke-test
  failures attributable to retry gaps in those repos. Method would
  parallel Phase 5.
- **5xx-only retry predicate vs. current regex-based classification.**
  Today both bash and Go classifiers grep stderr for keyword patterns
  (`HTTP 5\d\d|timeout|EOF|...`). A more precise approach could parse
  exit codes or HTTP status from structured output. **Trigger to revisit:**
  a misclassification observed in production (a non-retryable error
  triggering retries, or vice versa). Premature otherwise.

## Out by policy (not expected to change)

Decisions, not deferrals — these will not be revisited unless the policy
itself is reconsidered (in which case, separate plan).

- **No retry on local-only operations** — `git add`, `git commit`,
  `docker build`, `dotnet build`, `mvn package`, `gradle assemble`,
  filesystem ops, etc. Retry on local operations is rarely correct and
  usually masks bugs (compilation errors don't get better the second
  time you try). The audit phases (4 and 5) classify these as R-DOC-OK,
  not R-MISSING.
- **Backoff schedule fixed at 4 attempts, 5s → 15s → 45s.** The current
  schedule has been stable across `gh-retry.sh`, `docker-retry.sh`, and
  the Go port. Changing it without empirical justification fragments the
  observation pattern in CI logs ("how many retries are normal?") and
  invalidates the existing test fixtures.
- **No retry mechanism other than the shared in-house engine.** Adopting
  `Wandalen/wretry.action@v3`, `nick-fields/retry@v3`, or similar
  alongside the in-house engine would re-create the multi-mechanism
  split this plan exists to eliminate. If a future requirement genuinely
  doesn't fit the engine, the right response is to extend the engine,
  not bypass it.

## Notes on shape decisions (for reviewers)

- **Why not a composite action wrapping retry?** Loses `$(gh_retry ...)`
  capture, loses interleaving with non-retried shell logic in one `run:`
  block, doesn't help the Go port.
- **Why not `Wandalen/wretry.action@v3` org-wide?** Inconsistent with the
  existing in-house pattern (different log format, different
  transient/hard-fail semantics, default-retry-on-everything). The advantage
  — wrapping `uses:` steps — isn't needed today; TS is deferred.
- **Why vendor all 4 helpers into all 3 repos, not only what each uses?**
  Sync script stays trivial (one cp loop, no per-repo allow-list). Storage
  cost is negligible. The day a new tool needs retry in a repo, the helper
  is already on disk.
- **Why a sync script, not a checkout/submodule/curl?** Preserves the
  "self-contained per repo" property the project deliberately chose for the
  existing bash helpers. No runtime plumbing fee, no extra failure mode.
- **Why one plan instead of four?** The four standalone plans this
  consolidates are mutually load-bearing — 1830's bash regex update lands
  cleaner against 1930's canonical source; 1845's R-MISSING fixes use
  1930's `RetryWithPolicy`; 1850's §N rubric references 1930's `*-retry.sh`
  wrappers. One plan with one execution order avoids "did the four PRs land
  in the right sequence?" coordination overhead.
