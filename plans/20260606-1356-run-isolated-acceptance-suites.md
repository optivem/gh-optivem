# Plan: Run the isolated acceptance suites (and guide agents which to write)

## TL;DR

**Why:** The scaffolded `tests.yaml` defines four acceptance suites — `acceptance-api`,
`acceptance-ui` (both `-DexcludeTags=isolated`) and `acceptance-isolated-api`,
`acceptance-isolated-ui` (`-DincludeTags=isolated`). But the `acceptance` group the BPMN emits
expands to only the two non-isolated ones, so the `@Isolated` acceptance tests
(`PlaceOrderPositiveIsolatedTest`, …) are **written but never executed** by the pipeline or CI.
And the `acceptance-test-writer` agent has **no guidance** on when to emit `@Isolated` vs a plain
test — the choice is accidental (it copies whatever sibling it lands on).

**End result:** The `acceptance` alias expands to all four suites, so the existing
`--suite=acceptance` verify call-activities run isolated ATs too — no BPMN change.

**Scope:** run-side only. The *authoring* question — where the isolated/non-isolated decision is
made and whether the agent should make it — is a separate human-decision spun out to
`plans/20260606-1410-decide-where-acceptance-isolation-is-set.md`. This plan does **not** touch any
agent prompt.

## Why (full)

Two coupled gaps were found end-to-end. **This plan fixes gap 1 only**; gap 2 (agent guidance) is
documented here for context but deferred to the authoring-decision plan (see Scope above), because
*where* the isolation decision belongs is a human design call, not a mechanical fix.

1. **The isolated acceptance suites never run.** The `acceptance` group is declared identically in
   two places, both listing only the non-isolated pair:
   - Go default: `internal/atdd/runtime/testselect/suite.go` → `AcceptanceSuites()` returns
     `{"acceptance-api", "acceptance-ui"}`, and `defaultSuiteGroups["acceptance"]` reuses it.
   - Project override (wins over the Go default): the shop template's
     `system-test/<lang>/tests.yaml` → `suiteGroups: { acceptance: [acceptance-api, acceptance-ui] }`.

   The BPMN's `verify-tests-pass/fail` call-activities in `write-and-verify-acceptance-test-code`
   emit `suite: acceptance` (`process-flow.yaml:711,712,797,798,851,859`). It expands to
   `acceptance-api,acceptance-ui` only — both carry `-DexcludeTags=isolated`. CI
   (`.github/actions/acceptance-test/action.yml`) drives the orchestrator and names no suites, so
   it inherits the same gap. The isolated suites only run if a human types
   `gh optivem test run --suite acceptance-isolated-api`.

2. **No agent guidance on isolated vs non-isolated.** `grep -ri isolat` over `.claude/agents/`,
   `internal/assets/runtime/`, and `docs/atdd/` returns nothing. `acceptance-test-writer.md` does a
   "mechanical 1:1 translation … model each new test on the existing sibling test" — so whether a
   new AT is `@Isolated` depends entirely on which sibling the agent happens to read, not on a rule.

## What `@Isolated` means (the rule to encode)

`@Isolated` (`com.optivem.testing.Isolated`) tags acceptance tests that control a **process-global
external whose state is shared across concurrently-running tests** — today the **clock**
(`.given().clock().withTime(...)`) and **promotion** (`.given().promotion().withActive(...)`).
build.gradle runs `-DincludeTags=isolated` with `maxParallelForks=1` and parallel disabled, because
these tests would flake under the parallel forks the normal acceptance run uses. Plain
per-scenario data (product / country / coupon) is isolated *per scenario* already and needs no tag.

So the split is an **execution** concern (serial vs parallel), not a semantic category — both prove
system behaviour through the same channels. That is why folding them under one `acceptance` group is
correct: the runner runs suites sequentially, so the serial isolated suites simply run as two more
entries after the parallel ones.

## Design

### Run side — expand `acceptance` to all four (no BPMN change)

Make the `acceptance` alias resolve to
`[acceptance-api, acceptance-ui, acceptance-isolated-api, acceptance-isolated-ui]` in **both** places
that declare it. The list is deliberately kept in two places (decided, not an oversight — see the
SSoT note below):

- Shop template (**authoritative / operative**) — `academy/shop/system-test/{java,dotnet,typescript}/`
  `tests.yaml` (`suiteGroups.acceptance`). **Cross-repo**: the `shop` repo, not gh-optivem. The
  project `tests.yaml` override *wins* over the Go default, so this is what actually governs every
  scaffolded project; existing/new scaffolds are only fixed by editing here. Update the doc-comment
  line in each tests.yaml that quotes the default group to match.
- Go default (**mirror / fallback**) — `internal/atdd/runtime/testselect/suite.go`. Only applies to
  a config that omits the `suiteGroups` block; kept in agreement with the template so the fallback
  is correct.

**SSoT note (decided):** `[[feedback_question_second_file_ssots]]` flags two-file declarations as a
drift source, and deleting the redundant copy was considered. We keep both, on purpose, because (1)
this is a teaching repo and `tests.yaml` is a primary read artifact — the `acceptance → suites`
mapping must be *visible* there, not hidden in compiled Go; (2) `tests.yaml` is the operative source
for real projects anyway, so the declaration belongs there; (3) the drift is one-directional and
harmless — a stale Go default is dead weight for scaffolded projects (their `tests.yaml` wins), not
a correctness bug. Cross-repo means a gh-optivem unit test can't guard the shop copy, so the
mitigation is convention (edit both), not automation.

Verify aggregation is unaffected: the group already holds 2 suites, and RED/GREEN already tolerate
"some suites pass, the new test's suite fails" — adding 2 more suites is the same mechanism with a
longer list. A new **isolated** AT written in RED runs in its isolated suite and fails there →
RED verified; in GREEN all four pass → GREEN verified. Both authoring directions work.

**Channel-correctness (carry-over, not introduced here):** `AcceptanceSuites()` hard-codes both
channels; the in-flight preflight plan (see Coordination) expands `acceptance → acceptance-<ch>`
from `cfg.Channels` for api-only projects. The isolated ids are per-channel too
(`acceptance-isolated-<ch>`), so wherever expansion is channel-aware it must key the isolated ids
the same way. This plan does not change channel handling — it only lengthens the suite list, leaving
api-only correctness exactly where it is today (the flat template group already lists both channels).

### Author side — deferred to a separate plan

Not addressed here. See `plans/20260606-1410-decide-where-acceptance-isolation-is-set.md`. The open
question: whether the `@Isolated` choice is made by the `acceptance-test-writer` (with a
domain-agnostic rule), by the acceptance-criteria refiner / ticket, or somewhere else — and the
shop's `clock`/`promotion` builders must **not** be hardcoded into any generic agent prompt.

## Coordination (read before executing)

- **BLOCKED ON 1345 — wait for it to land before executing this plan (decided).**
  `plans/20260606-1345-bpmn-suite-existence-preflight.md` was picked up by agent `Valentina_Desk`
  (working tree shows it + `internal/atdd/runtime/preflight/preflight.go` modified mid-session, and
  HEAD advanced two commits since this session began). It adds a preflight that the BPMN-required
  suites **exist** in `tests.yaml`, and its expander maps `acceptance → acceptance-<ch>`.
  **Coupling:** once this plan folds the isolated ids into the `acceptance` group, that expander
  must track the longer list. Required follow-through when picking this up: ensure the 1345 expander
  resolves the group via `testselect.ExpandSuiteGroups` (so it tracks the canonical list
  automatically) rather than a hardcoded channel pair — if it hardcodes, this plan must update it
  too. Per `[[feedback_check_concurrent_agents]]` / `[[feedback_concurrent_agent_collision]]`,
  re-check 1345's state, `git log`, and `git status` before starting, and never stage another
  agent's dirty files (`preflight.go`, the 1345 plan).
- **New plan, not an edit** of 1345 or the (already-executed) decouple/slim-red plans, per
  `[[feedback_new_plan_not_extend]]`.

## Sibling finding (OUT OF SCOPE — recommend a separate follow-up)

The contract side has the identical orphan: `tests.yaml` defines `contract-stub-isolated`
(`-DincludeTags=isolated`), but the BPMN runs only `contract-real` and `contract-stub`
(`-DexcludeTags=isolated`) — so `contract-stub-isolated` is **also never run**. Same root cause
(isolated tag excluded from the suite the pipeline runs, no group that re-includes it). Not folded
into this plan because the user scoped to acceptance and the contract verify path is shaped
differently (no group alias — the BPMN names `contract-real`/`contract-stub` literally, so the fix
is a BPMN/process-flow change, not just a group edit). Recommend a fresh plan.

## Items

1. **Go default group.** In `internal/atdd/runtime/testselect/suite.go`, extend the `acceptance`
   group to include `acceptance-isolated-api` and `acceptance-isolated-ui`. Decide placement:
   either add the isolated ids to `AcceptanceSuites()` (if that list is meant to be "all acceptance
   suites") or keep `AcceptanceSuites()` as the non-isolated channel pair and compose the four-id
   list in `defaultSuiteGroups["acceptance"]`. Update the doc comments on both symbols to match.
   Files: `internal/atdd/runtime/testselect/suite.go`.
2. **Shop template groups (cross-repo).** In `academy/shop/system-test/{java,dotnet,typescript}/`
   `tests.yaml`, change `suiteGroups.acceptance` to the four-id list, and update the doc-comment
   reference to the default group in each. Confirm the suite ids match exactly per language (Item
   verified: all three declare `acceptance-isolated-api` / `acceptance-isolated-ui`).

No agent-prompt item — authoring guidance is out of scope (see Scope / the authoring-decision plan).

## Tests

- `internal/atdd/runtime/testselect/suite_test.go`: assert `ExpandSuiteGroups(["acceptance"], nil)`
  now yields the four ids in order, de-duped. Update any existing assertion pinned to the two-id
  result.
- `internal/runner/config_test.go`: if it asserts the resolved `acceptance` group membership, update
  to four ids.
- Run scoped, never unbounded on Windows per `[[feedback_go_test_windows]]`:
  `go test -p 2 ./internal/atdd/runtime/testselect/... ./internal/runner/...` (or `scripts/test.sh`).

## Verification

- Scoped `go test` (above) green.
- Manual smoke in a scaffolded project: `gh optivem test run --suite acceptance` now runs the
  isolated tests (e.g. `shouldRecordPlacementTimestamp`, `shouldApplyDiscountWhenPromotionIsActive`)
  alongside the non-isolated ones — confirm they appear in the report.
- Manual: in a RED→GREEN cycle, confirm the `--suite=acceptance` verify steps now include the
  isolated suites in their run (the isolated ATs show up in the verify output, not just the parallel
  ones).
- Do **not** add a diagram-regeneration step — handled by the push-to-main workflow per
  `[[feedback_plans_no_diagram_regen]]`.
