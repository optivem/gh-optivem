# Plan: close the empty-test-selection guard for runners that exit 0 ({20260608-1502})

🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-06-08T18:02:01Z`

Follow-up to `plans/20260608-1240-empty-test-selection-guard.md` (committed as
`02b09bc` + `b392149`). That plan closed the empty-selection hole for runners
that **exit non-zero** on a zero-match filter (Gradle, Maven `failIfNoTests`,
Playwright) by classifying their "no tests" stderr as `infra` →
`TESTS_INFRA_HALT`. This plan closes the **complement**: runners that exit `0`
on a zero-match filter, which the 1240 guard cannot see.

Related (historical): plan `20260606-1458` (infra-vs-red classifier), plan
`20260608-1240` (the non-zero half of this guard).

---

## TL;DR

**Why:** The 1240 empty-selection guard is exit-code-gated — `runCommand` only
classifies on a non-zero exit (`bindings.go:964`). dotnet (and any runner that
treats "no matching tests" as success) exits `0` with zero tests run, so
`test-outcome` is stamped `pass`. A *pass*-expecting verify greens on zero tests;
a *fail*-expecting verify routes to the passing-tests fixer on a non-signal. Same
silent hole as 1240, opposite exit code.
**End result:** An empty selection is detected regardless of the tool's exit
code — surfaced as the same `infra` / `TESTS_INFRA_HALT` outcome the 1240 guard
uses — so no verify can pass (or spin a fixer) without exercising a real test on
any supported runner.

---

## The latent gap

The 1240 guard lives in `classifyShellErr`, which `runCommand` only calls on
`!succeeded` (`internal/atdd/runtime/actions/bindings.go:964`). By contract the
classifier returns `classOK` whenever `err == nil` (`verify_classify.go:138`) —
a deliberate choice so a *passing* run with noisy stderr is never misread as
infra.

dotnet `test --filter` exits `0` and prints `No test matches the given testcase
filter …` as a **warning**, not an error (confirmed during 1240 execution; it is
why the 1240 dotnet classifier row can never fire in practice). The same is true
of any runner whose default is "no tests = success." For those, a zero-match
selection becomes `test-outcome = pass`:

- a `success`-expecting verify (`verify-tests-pass` / `VERIFY_TESTS_PASS_*`)
  greens having exercised nothing — the exact regression-masking 1240 set out to
  kill, just on the other polarity;
- a `failure`-expecting verify (`verify-tests-fail` / `VERIFY_TESTS_FAIL_*`)
  reads `pass` and routes to `FIX_UNEXPECTED_PASSING_TESTS`, spinning a fixer on
  a non-signal.

The 1240 per-tool stderr patterns do not help here: on a zero-exit run cobra
prints no error, so the marker (if any) lands only in the live stream
(`result.Stdout`), and the classifier is never invoked.

---

## Resolved approach (decided 2026-06-08)

**Strategy A chosen** — runner owns the "zero executed is not a success"
invariant; it's the cleanest long-term home (single point of truth for every
caller of `gh optivem test run`, count-based not string-based, no per-tool
wording drift). Decision confirmed with operator.

**Ground-truth correction to the original premise.** The original Item 1 assumed
each suite's `TestReportPath` was a machine-readable report (JUnit XML / TRX /
JSON) it could parse for a count. It is **not**: across all three real configs
(`shop/system-test/{java,dotnet,typescript}/tests.yaml`), `testReportPath` is the
**HTML** report (`index.html` / `testResults.html`). So Strategy A needs a
*separate* machine-readable path, not `TestReportPath`.

**Scope correction.** Only **dotnet** actually exits 0 on a zero-match filter.
Gradle/Maven and Playwright already exit **non-zero** on empty → the 1240 guard
catches them today. So the real gap is dotnet alone, and dotnet already emits a
machine-readable **TRX** (`--logger 'trx;LogFileName=testResults.trx'` →
`TestResults\testResults.trx`). The minimum clean fix is: add an explicit
machine-readable count-path field, parse the executed count, and fail the run
when the aggregate is 0. JUnit-XML and Playwright-JSON parsers are
belt-and-suspenders for any future runner configured to pass-on-empty
(`failIfNoTests=false`, `--passWithNoTests`).

**Rollout shape.** The guard is **opt-in per suite** via the new count-path
field: a suite that doesn't declare it keeps today's behaviour (no count guard),
so merging the gh-optivem change alone changes no observable behaviour until the
`shop` configs add the field. This makes the exit-contract change safe to land
and reversible by config.

(Strategy B — scan `runCommand`'s stdout on success for the empty marker — was
rejected: it only protects the verify path, not direct/CI callers, and keeps the
brittle per-tool string dependency.)

---

## Items

### 3. [audit + shop config] Enumerate exit-0-on-empty runners and opt them in

**Where:** the per-language suite definitions in the **`shop` repo**
(`shop/system-test/{java,dotnet,typescript}/tests.yaml`) and Item 1's count
path.

**Change:**
- Confirm which supported runners exit 0 on a zero-match filter. Established so
  far: **dotnet** is the only one (Gradle/Maven and Playwright exit non-zero →
  already covered by the 1240 guard). Re-confirm before adding the field
  anywhere else.
- Add `testCountPath: SystemTests\TestResults\testResults.trx` to the **dotnet**
  suites in `shop/system-test/dotnet/tests.yaml` (cross-repo commit). This is
  what actually arms the guard for the real gap.
- (Optional / future) if a Gradle/Playwright suite is ever reconfigured to
  pass-on-empty, add its JUnit-XML / JSON count path too; Playwright needs a
  JSON reporter wired into its command first (none today).

**Note:** this item commits to a **separate repo** (`shop`), so it gets its own
commit via `--repo shop`. **Gate for review** — it's the step that actually
changes observable `test run` behaviour in the academy.

---

## Verification

(Operator-driven; not agent `## Items` work.)

- Run a verify with a deliberately empty `--test=` filter on a **.NET** project
  and confirm it now **halts** (infra) instead of greening a `success` verify or
  spinning the passing-tests fixer on a `failure` verify.
- Repeat for each runner identified in Item 3 that defaults to exit-0-on-empty.
- Per the statemachine-test-loop hazard memory: audit gate fixtures before
  running any statemachine test exercising the new path; kill on memory climb.
