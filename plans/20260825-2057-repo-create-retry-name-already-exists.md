# 2026-08-25 18:57:47 UTC — Make `gh repo create` safe to retry (stop aborting on "Name already exists")

## TL;DR

**Why:** `CreateRepo()` wraps the non-idempotent `gh repo create` in a generic at-least-once retry. When attempt 1's response is lost to a transient *after* GitHub already created the repo, attempt 2 returns `Name already exists on this account` — which matches neither the transient nor the hard-fail regex — so the whole scaffold aborts even though the repo it needed exists. This killed the `monolith / multirepo / typescript / dotnet` leg of gh-acceptance-stage run 32874584907.

**End result:** A lost `gh repo create` response no longer fails a scaffold: the create path re-checks reality between attempts and treats "already exists, and our own pre-flight said it didn't" as success. A genuine collision against a real pre-existing repo still fails loud, unchanged. The same at-least-once hazard on `gh project create` — which silently produces duplicate project boards instead of erroring — is closed too.

## Outcomes

What we get out of this — the goals and deliverables:

- A scaffold run survives a transient that swallows the `gh repo create` response: the repo created on the lost attempt is adopted, and the run continues into `waitForRepoVisible()` as if the create had returned cleanly.
- Re-scaffolding an already-scaffolded repo still aborts loudly with the existing "already exists -- re-scaffolding an existing repo is not supported" message (issue #60 behaviour preserved).
- `retryTransient` / `retryHardFail` in `internal/kernel/shell/retry.go` are **unchanged** — "Name already exists" is never globally reclassified as transient, so no other caller silently swallows it.
- `gh project create` can no longer produce a duplicate project board after a lost response.
- Both behaviours are pinned by unit tests driving the existing `runFn` / stub seams, so a future refactor that reintroduces blind create-retry fails the suite instead of CI.

## ▶ Next executable step (resume here)

Step 1: in `internal/kernel/shell/github.go`, replace the `MustRunWithRetry("gh repo create "+g.Repo+" --public", "")` call at line 435 with a create-specific helper that re-checks `RepoExists(g.Repo)` when the output matches `Name already exists` (case-insensitive), adopting the repo as ours when the pre-flight check at the top of `CreateRepo()` had reported it absent, and fatalling with the existing message otherwise. Model the helper's placement and doc-comment style on the narrow post-create helpers `MustRunPostCreate` / `MustRunPostCreatePush` in `internal/kernel/shell/retry.go`. Stop after the production change compiles (`go build ./...`); Step 2 pins it with tests. Unblocks Steps 2 and 3.

## Steps

- [ ] Step 1: `internal/kernel/shell/github.go` — rework `CreateRepo()` (line 427-436). Keep the existing pre-flight `RepoExists` guard and its fatal-on-exists behaviour, and record its verdict. Replace the blind `MustRunWithRetry` create with a create-specific retry loop that, on a response matching `(?i)name already exists`, re-runs `RepoExists(g.Repo)`:
  - repo now exists **and** pre-flight said absent → adopt it, fall through to `waitForRepoVisible()`;
  - repo now exists **and** pre-flight said present → unreachable (pre-flight already fatalled), but keep the fatal for defence;
  - `RepoExists` returns an error (couldn't tell) → fatal loud, naming the repo — never coerce an indeterminate answer into success (repo `check-*` rule).
  Transient classification for every *other* failure stays on the existing shared policy. Do **not** touch the `retryTransient` / `retryHardFail` regexes in `internal/kernel/shell/retry.go`.
- [ ] Step 2: `internal/kernel/shell/github_test.go` — add two tests using the existing `withFakeRunFn` / `withFakeSleep` helpers:
  - *lost-response adoption*: call 1 `gh repo view` → 404; call 2 create attempt 1 → `net/http: TLS handshake timeout`; call 3 create attempt 2 → `GraphQL: Name already exists on this account (createRepository)`; call 4+ `gh repo view` → `{"name":"..."}`. Assert `CreateRepo()` returns **without** aborting. (This exact script reproduced the CI failure byte-for-byte against the current code.)
  - *indeterminate re-check*: same, but the post-`already exists` `RepoExists` returns `HTTP 403: Forbidden`. Assert it still aborts, and that the message names the repo.
  Leave `TestCreateRepo_ExistingRepoFailsLoud` (line 227) untouched and passing.
- [ ] Step 3: `internal/scaffolding/steps/project.go` — close the sibling defect at line 290. `gh project create --owner … --title …` currently runs through `projectRunCapture` (= `shell.RunCaptureWithRetry`, bound at line 66); because project titles are not unique, a retried create after a lost response silently creates a **duplicate** board. Fix by re-listing projects by title before re-creating on a transient — the list-by-title code already sits directly above at lines 273-288, so the create path should fall back into it rather than blindly re-issuing. If a clean fallback isn't expressible, make the create call non-retried (plain capture) so a lost response fails loud instead of duplicating; state which of the two was chosen in the commit message.
- [ ] Step 4: `internal/scaffolding/steps/project_test.go` — pin Step 3 with the existing stub harness (`stub.captureResp["project create"]`, `stub.calledViaContaining`): after a transient on the first create, assert exactly one `project create` call is ever issued (or that the re-list path returns the existing project), never two.
- [ ] Step 5: run `go test ./internal/kernel/shell/ ./internal/scaffolding/steps/ -p 2` and `go build ./...`. Never run unbounded `go test ./...` on Windows.

## Verification

- Operator: re-run `gh-acceptance-stage` and confirm the `monolith / multirepo / typescript / dotnet` leg passes. Note this leg's original failure depended on a real GitHub transient, so a green re-run is confirmation of no regression, not proof the fix fired — Step 2's test is the proof.

## Context

- Failing run: `https://github.com/optivem/gh-optivem/actions/runs/32874584907`, job `Run (monolith, multirepo, typescript, dotnet)` (id 97898649633).
- Surfacing test: `TestValidMonolithConfigurations/monolith_multirepo_ts_dotnet`, asserted at `internal/config/config_system_test.go:249` ("expected exit code 0, got 1").
- Failure tail: `WARN [retry] attempt 1/4 failed, retrying in 5s` → `FATAL: command failed: gh repo create …-system --public: exit status 1 / GraphQL: Name already exists on this account (createRepository)`.
- Mechanism: `runWithRetryLoop` (`internal/kernel/shell/retrycore.go:64`) re-issues the identical command with no re-check of reality between attempts; `classifyError` (`internal/kernel/shell/retry.go:63`) then hard-fails on attempt 2's wording.
- Go only — no bash `gh repo create` exists in this repo or in `optivem/actions`, so there is no parallel implementation to mirror the fix into.

## Open questions

- Step 3 leaves a choice between *re-list-then-adopt* and *don't-retry-the-create*. Re-list-then-adopt is the better long-term shape (it matches Step 1's "re-check reality" pattern), but don't-retry is acceptable if the capture seam makes the fallback awkward. The executor decides and records the choice.
