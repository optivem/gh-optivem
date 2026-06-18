# Per-channel acceptance verify must cover the isolated partition

## TL;DR

**Why:** The per-channel acceptance verify binds its suite to `acceptance-<channel>` only, which the project `tests.yaml` maps to `-DexcludeTags=isolated`. Any `@Isolated` acceptance test compiles into the *separate* `acceptance-isolated-<channel>` partition and is therefore **invisible** to that verify. When the verify is name-filtered (`--test=<name>`), the missing test trips `TESTS_INFRA_HALT` ("requested test never executed"). The canonical resolver `testselect.AcceptanceSuites` already knows a channel owns **both** partitions — the channel-unroll dropped the isolated half.
**End result:** The per-channel verify (in both the GREEN `change-system-behavior` system unroll and the RED write-and-verify-acceptance-tests driver-adapter unroll, plus the scoped-resume path) runs `acceptance-<ch>` **and** `acceptance-isolated-<ch>`. Isolated acceptance tests — including time-dependent, clock-mutating ones that *must* be isolated to avoid parallel clock clobber — are verified by name like any other, and tickets that ship one go green instead of halting.

## ▶ Next executable step (resume here)

Start at **Item 1** (`internal/engine/statemachine/channels.go:73`). No pickup marker yet; `git status` clean on `main` at authoring time. The three edit sites are mechanically identical; Items 1–3 can land in one commit.

## Motivation

Rehearsal **#76** ("Bug: Order cancellation blackout on Dec 31 ends at 22:30 instead of 23:00") halted at `VERIFY_TESTS_PASS` inside `change-system-behavior → implement-and-verify-system-api`:

```
gh optivem test run --suite=acceptance-api --test=cannotCancelOrderAt2245OnDec31
ERROR: requested test(s) never executed: cannotCancelOrderAt2245OnDec31
       — not found in any selected suite; … gated off (e.g. GH_OPTIVEM_RUN_WIP_TESTS)?
→ TESTS_INFRA_HALT
```

The error's own "gated off (`GH_OPTIVEM_RUN_WIP_TESTS`)" guess is a **red herring** — the orchestrator lifts that gate for every `test run` (`internal/atdd/process/actions/command.go:128-134`). The real cause is the third item in that triple: **wrong suite/partition.**

The acceptance-test-writer emitted the test inside an `@Isolated` class (`CancelOrderNegativeIsolatedTest`), correct for a clock-mutating `@TimeDependent` test — parallel clock mutation is flaky, so isolation is *required*, not optional. The project suite map then routes it away from the suite the verify ran:

```
# system-test/java/tests.yaml
acceptance-api          → .\gradlew.bat test … -DexcludeTags=isolated -Dchannel=API   # line 63
acceptance-isolated-api → .\gradlew.bat test … -DincludeTags=isolated -Dchannel=API   # line 77
```

So the named test lives only in `acceptance-isolated-api`, but the verify ran `--suite=acceptance-api`, which excludes it → "never executed" → halt.

**This is reproducible-by-construction, not flaky.** The identical ticket #76 *passed* the previous day (run `20260616-191628`, `result: ok`) only because that run's writer happened to emit the test **non-isolated** — i.e. the framework silently depends on acceptance tests never being isolated. The moment a test is (correctly) isolated, the per-channel verify cannot see it.

### Where the narrowing happens — and why it's a regression of a known-good resolver

Three sites bind the per-channel verify suite, all to the non-isolated partition only:

| Site | Context |
| --- | --- |
| `internal/engine/statemachine/channels.go:73` | `UnrollSystemChannels` — GREEN system verify per channel (the halt site for #76) |
| `internal/engine/statemachine/channels.go:126` | `UnrollSystemDriverAdapterChannels` — RED driver-adapter verify per channel |
| `internal/atdd/runtime/driver/scoped.go:153` | scoped-resume slice params (mirrors the unroll for a direct `RunProcess`) |

Each does `params["suite"] = "acceptance-" + ch`.

Meanwhile the canonical resolver already encodes the correct convention — a channel owns **both** partitions:

```go
// internal/atdd/runtime/testselect/suite.go:26
out = append(out, "acceptance-"+ch, "acceptance-isolated-"+ch)
```

The per-channel narrowing (introduced to stop each channel node from re-running *all* channels' suites — see the `channels.go:96-108` doc block) over-narrowed: dropping the cross-channel suites was correct, but dropping the **same channel's isolated partition** is a coverage hole, not a redundancy win.

### Why the two-suite fix is correct and deterministic

- `--suite` is a Cobra `StringSliceVar` — "repeatable, also accepts comma-separated values" (`test_commands.go:103`). A value `acceptance-api,acceptance-isolated-api` splits into two ids, each passed through `ExpandSuiteGroups` (`test_commands.go:90`; both are concrete ids → pass through unchanged).
- `shellEscape` (`command.go:36`) leaves a comma untouched — comma is not in its special-char set — so `--suite=acceptance-api,acceptance-isolated-api` renders as a single un-quoted token, exactly what `StringSliceVar` expects.
- `RunTests` unions executed test names **across all selected suites** and requires every requested `--test` to have run *somewhere* (`internal/build/runner/tests.go:139-150`). The named isolated test executes in `acceptance-isolated-<ch>`, lands in `executedNames`, and the presence check passes. The non-isolated partition matching nothing for that name is already expected and tolerated by design (`tests.go:94-104`).
- Red/green semantics are unchanged: the isolated AT passes (GREEN) / fails (RED) in its own partition; the non-isolated partition runs the channel's pre-existing acceptance tests, which keep their prior outcome. No new failure is introduced, and the zero-count guard is satisfied.

## Items

1. **`internal/engine/statemachine/channels.go:73`** — in `UnrollSystemChannels`, change the suite binding from `"acceptance-" + ch` to also include the channel's isolated partition:
   ```go
   params["suite"] = "acceptance-" + ch + ",acceptance-isolated-" + ch
   ```
   Update the adjacent doc comment (`channels.go:60-61`, "suite: acceptance-<channel> (D1 selector)…") to say each channel verifies **both** its parallel and isolated acceptance partitions, and cross-reference `testselect.AcceptanceSuites` as the source of the convention.
   *Layering note:* `internal/engine/statemachine` is the generic engine and must **not** import `internal/atdd/runtime/testselect`; keep the literal here (the package already hardcodes the `"acceptance-"` convention) with a comment pointing at the SSoT rather than adding a cross-package import.

2. **`internal/engine/statemachine/channels.go:126`** — same change in `UnrollSystemDriverAdapterChannels` (the RED per-channel driver-adapter verify). Without it, the RED verify of a newly-written *isolated* acceptance test would hit the identical halt one cascade earlier. Update its `channels.go:95-108` doc block: narrowing stays per-channel, but now covers both isolation partitions of that channel (the anti-redundancy rationale is unaffected — this adds same-channel coverage, not cross-channel re-runs).

3. **`internal/atdd/runtime/driver/scoped.go:153`** — same change in the scoped-resume slice params, so a direct `RunProcess` resume verifies the isolated partition identically to a full run. This package *may* import `testselect`; reuse the canonical pairing here to avoid a third hand-written literal:
   ```go
   sCtx.Params["suite"] = strings.Join(testselect.AcceptanceSuites([]string{channel}), ",")
   ```
   (Equivalent string; reuse keeps the convention single-sourced on the one site that can reach it.)

4. **Tests** — extend the existing unroll/binding tests so a regression that drops the isolated partition fails:
   - `internal/engine/statemachine/channels_*_test.go` (or the existing unroll test): assert the cloned per-channel node's `suite` param equals `acceptance-<ch>,acceptance-isolated-<ch>` for both unrolls.
   - `internal/atdd/process/actions/bindings_test.go`: assert the rendered `gh optivem test run` carries `--suite=acceptance-<ch>,acceptance-isolated-<ch>` (extends the existing `bindings_test.go:555` WIP-env assertion's sibling cases).
   - `internal/atdd/runtime/driver/scoped_test.go`: assert the scoped slice binds both partitions.
   Test coverage beyond this is the executor's discretion.

## Verification

- `scripts/test.sh` (or `-p 2`) green for `internal/engine/statemachine`, `internal/atdd/process/actions`, `internal/atdd/runtime/driver`. Do **not** run unbounded `go test ./...` on Windows.
- Re-run rehearsal **#76** end-to-end; it must reach a clean `result: ok` (the isolated `cannotCancel…` test verifies GREEN in `acceptance-isolated-api`/`-ui`) instead of `TESTS_INFRA_HALT`. *(Operator step — the rehearsal harness is user-driven.)*
- Spot-check the run trace shows `suite=acceptance-api,acceptance-isolated-api` on the per-channel verify steps.

## Out of scope — follow-up (separate plan)

The framework fix above closes the halt for *all* isolated acceptance tests. A separate, fuzzier concern surfaced in the same run and should get its **own** plan, not be folded here:

- **acceptance-test-writer non-determinism.** Across two runs of the identical ticket #76 the writer produced materially different tests — one run non-isolated + single method (`cannotCancelAnOrderAt2245OnDecember31st`), the other `@Isolated` + an **invented, unreported** second parameterized test (`cannotCancelAnOrderOn31stDecBetween2200And2230`, 5 fabricated `@DataSource` rows the single-scenario AC never asked for). The prompt says isolation is "mechanical mirroring of the tag the refiner/author already set — *not* a judgement", yet the AC carries no `@isolated` tag.
- **Corpus-authoring gap.** If a clock-mutating `@TimeDependent` scenario *must* be isolated (it must, to be parallel-safe), the ticket/refiner should carry the `@isolated` tag deterministically rather than leaving the writer to infer it. Decide whether `@TimeDependent ⇒ @isolated` becomes an explicit rule.

Neither blocks Items 1–4: the framework must support isolated acceptance tests regardless of how the writer/ticket decides to use isolation.
