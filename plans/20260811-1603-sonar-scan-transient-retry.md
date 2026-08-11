# 2026-08-11 14:03:00 UTC — Retry the local SonarCloud scan on transient failures

> ⏸ **PENDING DISCUSSION — do not execute.**
>
> Investigation is complete and the naive fix is **proven wrong**: wrapping the
> call in `RetryWithPolicy` with the shared regexes would be a **no-op** against
> the exact failure it targets (Q1, verified against real log output). Do not
> ship the one-liner.
>
> **Discussion agenda — settle these before touching code:**
> 1. **Q1 remedy** — classify on the extracted failing line *(recommended)*, a
>    scanner-specific hard-fail regex, or both. The *finding* is resolved; the
>    *fix* is not chosen.
> 2. **Q2** — attempt count, given each attempt costs 1–2 min, not milliseconds.
> 3. **Scope — Go-only, or the shared pattern?** *(investigated; likely the
>    biggest decision)* The scaffolded CI wires retry correctly but hits the
>    **same** false positive, so a Go-only fix leaves CI exposed. The defect is
>    in the shared hard-fail pattern mirrored in `optivem/actions/shared/retry.sh`
>    and `internal/kernel/shell/retry.go`. See `## Note — pending discussion`.
> 4. **Go has no force-retry mechanism** — the SonarCloud JRE/CDN 403 fix
>    protects CI but has never protected `gh optivem init`. Separate latent bug;
>    fold in or split out?
> 5. **Latent risk elsewhere** — whether other `RetryWithPolicy` call sites with
>    large outputs are already silently no-op for the same reason.

## TL;DR

**Why:** Acceptance run [31497679266](https://github.com/optivem/gh-optivem/actions/runs/31497679266) lost three of four smoke jobs (`monolith/monorepo/java`, `monolith/multirepo/dotnet`, `multitier/monorepo/typescript`) to one SonarCloud-side fault, all within 18 seconds of each other (13:53:04–13:53:22 UTC):

```
com.sonarsource.scanner.engine.webapi.client.HttpException:
Error 504 on https://api.sonarcloud.io/analysis/analyses : {"message": "Endpoint request timed out"}
```

Three different toolchains (Gradle `:sonar`, dotnet scanner, sonar-scanner CLI) failed identically, so this is SonarCloud's `/analysis/analyses` endpoint timing out, not scaffolded code and not the commits made that hour (README reorder, unconditional bash/docker environment checks — none touch the sonar path). The previous full run (`31377823725`, 2026-08-10) was green.

`internal/scaffolding/steps/verify.go:650` runs the scan exactly once with no retry, so a single transient 5xx kills an otherwise-healthy ~2-hour acceptance run and forces a manual full re-scaffold + rebuild + re-scan per job.

**End result:** `sonarComponent` routes through the existing `shell.RetryWithPolicy` engine, so a transient SonarCloud 5xx/timeout is retried on the same canonical backoff every other tool in the codebase uses — and still fails loud, with attempt count and last error, when the fault is real or persistent.

## Key finding — the fix is a call-site change, not new machinery

Everything needed already exists and is already pointed at this exact use case:

- `internal/kernel/shell/retrycore.go:94` — `RetryWithPolicy(transient, hardFail *regexp.Regexp, prefix string, fn func() (string, error))`. Its own doc comment says *"Use this for non-gh callers (sonar, docker, future tools)."*
- `internal/kernel/shell/retry.go:50,54` — `RetryTransient()` / `RetryHardFail()`, exported specifically so *"callers in sibling packages … can plug it into RetryWithPolicy without redefining the pattern."*
- Canonical schedule (`retrycore.go:16`): 4 attempts, 5s → 15s → 45s, mirroring `_RETRY_CORE_ATTEMPTS` / `_RETRY_CORE_DELAYS` in `optivem/actions/shared/retry-core.sh` so bash and Go retry identically.
- `retrycore.go:78` already logs `[prefix] attempt N/M failed, retrying in …` — the visible-retry requirement is met for free.

Critically, **`retryTransient` already matches today's failure on two independent alternatives** (`retry.go:24,29`):

- `Error 5\d\d on https://` → matches `Error 504 on https://api.sonarcloud.io/…`
- `Endpoint request timed out` → matches the response body verbatim

The no-swallowing-errors rule is preserved by construction: `RetryWithPolicy` re-asks the same question and returns the final error unchanged on exhaustion — the existing `log.Fatalf` at `verify.go:653` still fires.

> ⚠️ **This section establishes that the machinery fits, not that the change is a one-liner.** The transient pattern matching is necessary but *not sufficient* — Q1 proves the hard-fail check suppresses the retry before the transient pattern is ever consulted. Read `## Open questions` Q1 before acting on anything above.

`internal/kernel/shell/sonarcloud.go:47` already uses exactly this call shape (`RetryWithPolicy(retryTransient, retryHardFail, "sonar-retry", …)`) for our own SonarCloud API calls. This plan extends the same treatment to the scanner subprocess, which was missed.

## Outcomes

- A transient SonarCloud 5xx / timeout during `bash ./run-sonar.sh` no longer fails the acceptance run on first contact.
- A definitive failure — bad `SONAR_TOKEN` (401), missing project key, quality-gate failure — still fails on the **first** attempt with no retry delay. Note this must hold *without* the over-broad matching Q1 exposed: the classifier has to distinguish a real auth failure from the word "unauthorized" appearing in a stack frame.
- Exhaustion after all attempts fails loud with the original scanner output, as it does today.
- A run that only passed on a later attempt is visibly distinguishable in the log from one that passed clean, so SonarCloud degradation stays observable instead of being silently absorbed.
- No new retry primitive, no new backoff schedule, no duplicated regex — the change is confined to the call site.

## ▶ Next executable step (resume here)

**Blocked on discussion — see the agenda at the top of this file.** Nothing here is executable until the Q1 remedy is chosen; the obvious-looking first move (plain `RetryWithPolicy` wrap) is proven to be a no-op and must not be taken. Once the agenda is settled, start at Step 1 (Q1 remedy), *then* Step 2 (the call-site change).

## Steps

- [ ] Step 1: Implement the **Q1 remedy chosen in discussion** (failing-line extraction and/or a scanner-scoped hard-fail pattern). This is the load-bearing step — without it every step below is cosmetic, since the retry will never engage. Do not start with the plain wrap.
- [ ] Step 2: In `internal/scaffolding/steps/verify.go:650`, route the `shell.Run("bash ./run-sonar.sh", true, dir)` call through `shell.RetryWithPolicy` using the Step 1 classification, keeping the existing `log.Fatalf` on the returned error so exhaustion behaviour is unchanged. Use a distinct prefix from `sonarcloud.go`'s `"sonar-retry"` (e.g. `"sonar-scan"`) so log readers can tell the scanner subprocess apart from our own API calls. Apply the Q2 decision on attempt count here.
- [ ] Step 3: Re-run the Q1 verification against the real captured output (job `93799958610`) and confirm the classifier now returns *retry = true* for the 504 case. **This is the regression guard for the exact trap Q1 uncovered** — a green build is not evidence the retry works.
- [ ] Step 4: Update the `VerifyLocalSonar` doc comment (`verify.go:628`) to state the retry behaviour and that hard-fail classes are excluded from it.
- [ ] Step 5: Add tests in `internal/scaffolding/steps` covering: (a) a 504-shaped scanner output retries then succeeds; (b) a genuine 401/auth-failure output fails on the first attempt with no sleep; (c) sustained transient output exhausts and returns the last error; (d) **regression test for Q1** — output containing the literal frame `failIfUnauthorized` alongside a 504 must still retry. Use the real captured log as the fixture for (a) and (d) rather than a hand-written string, so the test fails if classification regresses. Use `shell.SetSleepForTest` (`retrycore.go:33`) so the tests don't sleep for real. This requires a seam to inject a fake runner into `sonarComponent` — currently it calls `shell.Run` directly.
- [ ] Step 6: Run the scoped package tests plus `go build ./...` and `go vet ./...`. Never unbounded `go test ./...` on Windows — scope to the package or use `-p 2`.

## Open questions

**Q1 — Does `retryHardFail` misfire against a multi-hundred-line scanner log? → RESOLVED EMPIRICALLY: YES. A naive wrap is a no-op.**

Verified by classifying the real captured output of the failing `monolith/monorepo/java` job (job `93799958610`, the 2,208 lines `shell.Run` would have returned) against the actual exported `RetryTransient()` / `RetryHardFail()` regexes:

```
transient matches: 4   "Error 504 on https://" ×2, "Endpoint request timed out" ×2
hardFail  matches: 2   "Unauthorized" ×2
=> RetryWithPolicy would retry: false
```

Both hard-fail hits come from a **stack-frame method name**, not an auth failure:

```
at org.sonar.scanner.http.DefaultScannerWsClient.failIfUnauthorized(DefaultScannerWsClient.java:87)
```

`retryHardFail`'s `unauthorized` alternative is case-insensitive and unanchored, so it matches the substring inside `failIfUnauthorized`. Because `RetryWithPolicy` checks hard-fail first and short-circuits (`retrycore.go:100-103`), the retry would never engage — on the exact failure this plan exists to fix. This is the predicted no-op-that-looks-implemented outcome, now confirmed rather than hypothesised.

**Option (c) — "ship the naive wrap as-is" — is therefore eliminated.** It would produce zero behaviour change while appearing to fix the problem. The remaining decision is between:

- **(a) Scope classification to the failing line, not the whole log** *(recommended)* — extract the terminal `Caused by:` / `Error 5\d\d on https://…` line and classify on that alone. This fixes the root cause: the shared regexes were written for small, single-purpose API-call outputs (`sonarcloud.go`), and matching English prose against 2,000+ lines of Java stack traces will keep producing false positives regardless of how the pattern is tuned. Keeps the shared single-source-of-truth regexes intact per `retrycore.go:90`. More code than (b).
- **(b) Use a scanner-specific hard-fail regex** — narrower, matching only definitive sonar cases (401/403 on the sonar host, `Project key … does not exist`, quality-gate failure). Cheaper, but still classifies against the whole log, so it stays exposed to the same class of accidental substring match from any future stack frame or log line. Also contradicts the one-place-to-edit design note in `retrycore.go:90`.

(a) and (b) are not exclusive — (a) plus a tightened sonar pattern is defensible. **Do not pick autonomously**; this is a real trade-off between root-cause robustness and the codebase's single-source-of-truth convention.

**Corollary worth checking during the discussion:** if `failIfUnauthorized`-style substring matching can suppress a retry here, the same false-positive risk may already exist at other `RetryWithPolicy` call sites whose output is larger than a single API response. `sonarcloud.go:47` looks safe (small HTTP bodies), but this should be confirmed rather than assumed — a latent no-op retry elsewhere would be invisible until an outage.

**Q2 — Is the canonical 4-attempt schedule right for this call site?** Every other `RetryWithPolicy` caller wraps a single cheap HTTP request. Here each attempt re-runs a full scanner pass (~1–2 min observed). Worst case, 4 attempts adds 3 extra scan runs plus ~65s of backoff sleep — per component, and `VerifyLocalSonar` calls `sonarComponent` two or three times depending on architecture. On a persistent SonarCloud outage that could add substantial time to a run that fails anyway. Options: keep the canonical 4 for cross-tool consistency, or pass a reduced schedule for this call site (which `RetryWithPolicy` does not currently expose — it hardcodes `defaultRetryAttempts`/`defaultRetryDelays`, so a reduced schedule means extending the engine's signature or calling `runWithRetryLoop` directly).

## Note — pending discussion

Written up but deliberately **not executed**. The one-line version of this change is not merely risky — Q1 proves it is a **no-op** against the exact failure it targets, because `retryHardFail`'s `unauthorized` alternative matches the substring in the stack frame `failIfUnauthorized`. Q2 remains open: the canonical 4-attempt schedule may be a poor fit for a call site costing minutes per attempt rather than milliseconds. The remedy for Q1 and the answer to Q2 both need a decision before any code is touched.

### Bash-side scope — INVESTIGATED. Wiring is fine; the same bug is present anyway.

The original hypothesis (scaffolded `run-sonar.sh` bypasses the retry wrapper) is **wrong**. The scaffolded CI wires retry correctly — 9 call sites across `shop/.github/workflows/*-{acceptance,commit}-stage.yml` all invoke it through the shared composite:

```yaml
- name: Run Sonar Analysis
  uses: optivem/actions/retry@v1
  with:
    working-directory: system-test/java
    command: bash ./run-sonar.sh
```

**But that wrapper is equally a no-op for this failure.** `_RETRY_HARD_FAIL` in `.github/scripts/retry.sh` contains `[Uu]nauthorized`, which matches the same `failIfUnauthorized` stack frame. Classifying the same real captured output with the bash regexes:

```
force-retry match: no
hard-fail  match: "Unauthorized"
transient  match: "Endpoint request timed out", "Error 504 on https://"
```

`retry-core.sh:94-108` checks force-retry, then hard-fail, then transient — so this hard-fails immediately with zero attempts, exactly as the Go path does. **CI-invoked scans are exposed too**, not through a wiring gap but through the same false positive.

**This relocates the root cause.** The defect is in the shared hard-fail pattern, which exists in two synchronized places:
- `optivem/actions/shared/retry.sh` — upstream source of truth; gh-optivem's `.github/scripts/*.sh` are `GENERATED — DO NOT EDIT` copies of it, synced via `optivem/actions/scripts/sync-shared.sh`.
- `internal/kernel/shell/retry.go` — the Go mirror.

Fixing only `verify.go` therefore fixes neither the scaffolded CI nor the vendored bash copies. **The Q1 remedy should be decided for the shared pattern first, then applied consistently to both languages** — which materially changes this plan's scope, and is the single most important thing to settle in discussion.

### Additional finding — the Go path has no force-retry mechanism at all

`retrycore.go:12` claims to mirror `_RETRY_CORE_ATTEMPTS` / `_RETRY_CORE_DELAYS` from `retry-core.sh`, but the Go side implements **no equivalent of `_RETRY_FORCE_RETRY`** — `RetryWithPolicy` takes only `transient` and `hardFail` (verified: no `ForceRetry`/`jres`/`binaries.sonarsource` reference anywhere in `internal/kernel/shell/`).

That override is what reclaims known-transient-but-hard-fail-shaped SonarCloud failures — the JRE-provisioning 403s (`/analysis/jres`, `scanner.sonarcloud.io/jres`) and the `binaries.sonarsource.com` CDN 403 that triggered the original retry work. The vendored bash copy **does** carry it (3 references in `.github/scripts/retry.sh`, so that sync is current).

Consequence: **the SonarCloud 403 fix protects CI but has never protected `gh optivem init`'s local scan.** A JRE-provisioning or CDN 403 during local/smoke-test scanning still hard-fails with zero retries. This is a separate latent bug from Q1, discovered alongside it, and worth deciding whether to fold into this plan's scope or split into its own.

## Verification

- Re-run the three failed smoke jobs from run `31497679266` to confirm they were purely transient (independent of this plan — the fix is on SonarSource's side).
- After the change ships, confirm on the next full acceptance run that a clean run logs no `[sonar-scan] attempt` lines, so the retry isn't quietly firing on every run and masking a real, persistent problem.
