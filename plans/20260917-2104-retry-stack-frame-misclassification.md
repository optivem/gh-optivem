# 2026-09-17 19:04:23 UTC — Stop the retry classifier reading Java stack frames as failure messages

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-09-17T19:22:10Z`

## TL;DR

**Why:** `optivem/actions/retry@v1` classifies a failure by grepping the command's whole stdout+stderr. A Java stack frame — `at org.sonar.scanner.http.DefaultScannerWsClient.failIfUnauthorized(DefaultScannerWsClient.java:88)` — contains the word `Unauthorized`, which matches `_RETRY_HARD_FAIL` and wins over the genuine transient `Error 500 on https://`. That frame sits on `DefaultScannerWsClient.call`, the path every scanner web-service call takes, so the retry envelope around `sonar-scanner-cli` is dead for **every** HTTP failure it raises. gh-optivem run [35261501113](https://github.com/optivem/gh-optivem/actions/runs/35261501113) died on a transient SonarCloud 500 after one 40s attempt with zero retries.

**End result:** The classifier ignores stack-frame lines and reads only failure *messages*. A SonarCloud 5xx retries 4 times as the action always advertised; a genuine scanner-CLI 4xx still hard-fails on its first attempt — now by an explicit `Error 4xx on https://` rule rather than by accident.

## Outcomes

- A transient SonarCloud 5xx during `Run Code Analysis` retries 4 times (5s/15s/45s) instead of failing the commit stage on the first attempt.
- No method or class name can ever hijack a retry verdict again — the whole collision class (`Unauthorized`, `PermissionDenied`, `NotFound`, …appearing as identifiers) is closed, not just this one instance.
- A genuine scanner-CLI auth failure (`Error 401 on https://sonarcloud.io/...`) is classified hard-fail by an explicit rule. Today it matches *neither* list and only fails fast because of the same accidental frame match this plan removes.
- Regression coverage pinned to run 35261501113, so the exact captured failure can never be re-misclassified silently.
- The `gh-optivem` vendored copies are back in sync with canonical — they are currently stale by two clauses beyond this fix.
- The Go mirror keeps the parity its own header claims (`internal/kernel/shell/retry.go`).

**Cross-repo note:** this plan lives in `gh-optivem` (where the failure surfaced) but the fix itself is in the **`optivem/actions`** repo at `academy/actions/`. Both repos are committed separately; do not stage across repo roots.

**Status (2026-09-17):** all six code steps are done, committed locally in both repos, and green — 95 bash assertions plus the scoped Go suite. Only the release push remains (see below).

## Evidence (already established — do not re-derive)

Confirmed by replaying the captured CI log through the real `retry_run` with shortened delays: `rc=3`, elapsed 0s, 0 retry notices — matching CI exactly.

Regex classification against the real 221-line log:

| | matches |
|---|---|
| `_RETRY_HARD_FAIL` | `Unauthorized` (from the frame at log line 201 only) |
| `_RETRY_RETRYABLE` | `Error 500 on https://` |
| `_RETRY_FORCE_RETRY` | *(none)* |

After filtering the 112 stack-frame lines: hard-fail matches drop to **none**, and `Error 500 on https://` wins → retries.

Hard-fail is checked at `retry-core.sh:97`, *before* the transient check at `:105`, which is why hard-fail wins.

## ▶ Next executable step (resume here)

**Everything in this plan is executed and released.** `optivem/actions` `cd0432c` is on `main`, the floating `v1` tag has been force-moved to it by `update-v1.yml`, and `gh-optivem` `5ce16121` (vendored re-sync + Go parity) is on `main`. Every consumer (`shop`, `gh-optivem`, `optivem-testing`) now resolves `retry@v1` to the fixed classifier.

The only thing left is to observe the final proof run: a green `gh-commit-stage` on `gh-optivem` `main` at `5ce16121`. That is a CI outcome to watch, not an edit to make — once it is green, delete this plan file.

## Out of scope (flag, do not fix here)

- **Go transient regex drift from bash.** `internal/kernel/shell/retry.go` lacks `Request failed with status code 5\d\d`, `Bootstrapper: An error occurred`, `Connection reset by peer`, `Error response from daemon: Get "…": unknown`, and `unexpected end of JSON input`; Go has no force-retry list at all, so none of the SonarCloud JRE-provisioning / GHCR secondary-rate-limit reclaims exist on that side. Pre-existing, orthogonal to this bug — needs its own plan.
- **Dead vendored legacy wrappers.** `gh-optivem/.github/scripts/` still carries generated `sonar-retry.sh`, `docker-retry.sh`, and `git-retry.sh` whose canonical sources no longer exist under `actions/shared/` (the unified `retry.sh` replaced all four). Only `gh-retry.sh` is still sourced live — `gh-post-release-stage.yml:32/98/163/214` and `gh-release-stage.yml:42`. Dead-file cleanup is a separate plan.

## Verification

Done:

- ✅ `_test-retry-core.sh` 32/32 and `_test-retry.sh` 63/63 green, including all new cases.
- ✅ Real captured CI log replayed through `retry_run`: now 4 attempts, 3 retry notices, `::warning::[retry] exhausted 4 attempts (exit 3)`, rc still 3, and the full unfiltered stack trace still reaches the operator. Before the fix: 1 attempt, 0 notices.
- ✅ `go build ./...` clean; `go test ./internal/kernel/shell/...` green (scoped — never unbounded `go test ./...` on Windows).
- ✅ Vendored copies diff clean against canonical apart from the generated banner.
- ⚠️ `shared/_lint/check-shell-scripts.sh` could **not** run locally — shellcheck is not installed on this machine, and the script correctly fails loud rather than skipping. Substituted `bash -n` syntax checks on all four edited files (all OK). CI runs the real shellcheck.

Outstanding:

- ⬜ Final proof: a green `gh-commit-stage` run on `gh-optivem` `main` once `v1` has advanced.
