# 2026-09-17 19:04:23 UTC — Stop the retry classifier reading Java stack frames as failure messages

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

**Cross-repo note:** this plan lives in `gh-optivem` (where the failure surfaced) but Steps 1–4 edit the **`optivem/actions`** repo at `academy/actions/`. Steps 5–6 are back in `gh-optivem`. Commits are per-repo; do not stage across repo roots.

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

**Step 1** — in `academy/actions/shared/retry-core.sh`, change how `match_content` is built (currently line 84) so Java stack-frame lines are excluded from classification:

```bash
match_content=$(cat "$stdout_file" "$stderr_file" \
    | grep -Ev '^[[:space:]]*(at [A-Za-z_$][A-Za-z0-9_.$/]*\(|\.\.\. [0-9]+ (common frames omitted|more)$)')
```

Two things to get right:
- `grep -Ev` returns exit 1 when it filters *everything*; under the callers' inherited `set -e` that must not abort the function. Guard it (`|| true`, or a pipeline whose status is discarded) and confirm the empty-result case still classifies as "matches neither → pass through".
- The filter applies to the **classification input only**. The pass-through and exhaustion branches (`retry-core.sh:98-99`, `:106-107`, `:128-129`) must keep `cat`-ing the *unfiltered* `$stdout_file` / `$stderr_file` so the operator still sees the complete stack trace.

Record the rationale in the comment: stack frames are code identifiers, not failure messages; classifying on them lets any identifier containing a policy keyword silently hijack the verdict.

Unblocks Steps 2–3 (regex companion + tests) in the same repo.

## Steps

- [ ] **Step 1 — `retry-core.sh`: exclude stack frames from classification.** As detailed in the resume block above. File: `academy/actions/shared/retry-core.sh` (repo `optivem/actions`). Also extend the file's header comment block (the behaviour contract at lines 11–33) to state that classification reads messages, not frames.

- [ ] **Step 2 — `retry.sh`: add the missing scanner-CLI 4xx hard-fail clause.** Add `Error 4[0-9][0-9] on https://` to `_RETRY_HARD_FAIL` (line 72), symmetric with the existing transient `Error 5[0-9][0-9] on https://` in `_RETRY_RETRYABLE` (line 65). **This is a required companion to Step 1, not a nice-to-have:** verified that `HttpException: Error 401 on https://sonarcloud.io/batch/project.protobuf?key=x : {"errors":[{"msg":"Insufficient privileges"}]}` matches neither list today — `HTTP 4[0-9][0-9]` does not cover the `Error 4NN on` phrasing. Without this clause, Step 1 would leave real auth failures unclassified. Cite run `35261501113` in the comment block, matching the file's existing convention of naming the triggering run.

- [ ] **Step 3 — Regression tests.** `academy/actions/shared/_test-retry-core.sh` and `_test-retry.sh`, using the existing harness shape `run_case "desc" 'rc|output;rc|output' expected_rc expected_attempts`. Add:
  - sonar-scanner-cli 500 whose output carries the `at ...failIfUnauthorized(DefaultScannerWsClient.java:88)` frame → **retries** (transient), not 1 attempt. This is the regression test for run 35261501113.
  - genuine scanner-CLI `Error 401 on https://sonarcloud.io/...` as a *message*, no frames → hard-fail, 1 attempt.
  - a stack trace whose only policy-keyword hit is in a frame and whose message matches nothing → pass-through, 1 attempt (the "unknown failure mode — don't retry blindly" contract must survive).
  - a frame-only trace that filters down to empty classification input → pass-through, 1 attempt, no `set -e` abort (guards the `grep -Ev` edge from Step 1).
  - confirm the existing message-side hard-fail cases stay green, in particular `"sonar hard-fail: Not authorized"` at `_test-retry.sh:190`.

- [ ] **Step 4 — Release `optivem/actions` to `v1`.** `.github/workflows/gh-commit-stage.yml:70` pins `uses: optivem/actions/retry@v1`, so nothing reaches CI until the `v1` tag advances past the fix commit. Follow the repo's existing release convention for moving `v1`.

- [ ] **Step 5 — Re-sync the vendored copies in `gh-optivem`.** Run `bash academy/actions/scripts/sync-shared.sh` (resolve the path dynamically) *after* Steps 1–2 land. This regenerates `.github/scripts/retry.sh` and `.github/scripts/retry-core.sh`, whose headers cite source commits `e1915a91` and `b746f07b`. They are **already stale by two clauses** independent of this fix — the vendored `_RETRY_RETRYABLE` lacks `unexpected end of JSON input` and the vendored `_RETRY_FORCE_RETRY` lacks the `\[remote rejected\].*\((Internal Server Error|Bad Gateway|…)\)` clause — so the sync picks up both the fix and the drift. These files are marked `GENERATED — DO NOT EDIT`: never hand-edit them.

- [ ] **Step 6 — Go mirror parity.** `internal/kernel/shell/retry.go:35-43`. `retryHardFail` carries the same `unauthorized` clause, but the Go side is **not affected by this bug**: every call site (`internal/devworkflow/sonar/sonar.go:128`, `internal/config/token_auth.go:69/116/176/335`, `internal/scaffolding/steps/project.go:340`) classifies short `HTTP <code>\n<body>` summaries or `gh` CLI output, never Java stack traces. The file header nonetheless states it "Mirrors the union of patterns in optivem/actions/shared/retry.sh", so add the `Error 4\d\d on https://` hard-fail clause to keep that invariant honest. Add a unit test beside the existing retry tests. Do **not** add frame-stripping to Go — no Go caller feeds it stack traces, and a field that nothing branches on does not earn its slot.

## Out of scope (flag, do not fix here)

- **Go transient regex drift from bash.** `internal/kernel/shell/retry.go` lacks `Request failed with status code 5\d\d`, `Bootstrapper: An error occurred`, `Connection reset by peer`, `Error response from daemon: Get "…": unknown`, and `unexpected end of JSON input`; Go has no force-retry list at all, so none of the SonarCloud JRE-provisioning / GHCR secondary-rate-limit reclaims exist on that side. Pre-existing, orthogonal to this bug — needs its own plan.
- **Dead vendored legacy wrappers.** `gh-optivem/.github/scripts/` still carries generated `sonar-retry.sh`, `docker-retry.sh`, and `git-retry.sh` whose canonical sources no longer exist under `actions/shared/` (the unified `retry.sh` replaced all four). Only `gh-retry.sh` is still sourced live — `gh-post-release-stage.yml:32/98/163/214` and `gh-release-stage.yml:42`. Dead-file cleanup is a separate plan.

## Verification

- `bash academy/actions/shared/_test-retry-core.sh && bash academy/actions/shared/_test-retry.sh` — all green, including the new cases.
- Replay the real captured failure log through `retry_run` and assert it now makes 4 attempts and ends with `::warning::[retry] exhausted 4 attempts`.
- `bash academy/actions/shared/_lint/check-shell-scripts.sh` over the edited bash.
- `go test ./internal/kernel/shell/...` — scoped. Never unbounded `go test ./...` on Windows.
- After Step 5, `diff` each vendored copy against canonical and confirm only the generated header block differs.
- Final proof: a green `gh-commit-stage` run on `gh-optivem` `main` once the `v1` tag has advanced.
