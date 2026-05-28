# Approval tier-ladder — log-level threshold model for `implement`

## Context

Supersedes `plans/20260527-2119-approval-category-explicit-ssot.md`.
That plan kept today's 4-token flat closed set
(`commit`, `fix`, `prompt`, `human`) with set-membership `--confirm`
semantics, and would have made `category:` explicit at every BPMN
site as the SSoT.

During the 2026-05-28 design discussion we replaced that model
entirely. The new model is a **6-tier ordered ladder with threshold
`--confirm` semantics**, scoped to `implement` (and `atdd run`). The
explicit-SSoT mechanism (every approve site declares its tier,
parse-time validation, no implicit defaults) carries over — it is
the foundation the ladder requires.

The 4-token flat enum was hard to reason about because
`commit`/`fix`/`prompt`/`human` are not naturally comparable: every
operator workflow ended up encoded as a *set* of tokens. Reframing
them as log-level tiers makes the operator's mental model
single-knob ("how far down the autonomy ladder do I want?") and
removes the awkward "is this a `commit` or a `fix`?" classification
calls.

## Resolution log

Decisions taken during chat on 2026-05-28:

- **D1 — Semantics: threshold, not set membership.**
  `--confirm=<tier>` means "this tier becomes the floor; everything
  *at or above* the floor still prompts, everything below auto-yeses
  under `--auto`." One knob, not a set.

- **D2 — Vocabulary: semantic names, ordered low-to-high stakes.**

  | Order | Token | What dispatches at this tier | Examples |
  |---|---|---|---|
  | 1 (lowest) | `command` | `execute-command` BPMN nodes — cheap, no AI cost, no global state mutation | compile, system compile, test compile, system build, system start, test run |
  | 2 | `prod-agent` | `execute-agent` for production code | implement-dsl, implement-system, update-system, implement-system-driver-adapters, update-system-driver-adapters, implement-external-system-driver-adapters, update-external-system-driver-adapters, implement-external-system-stubs, refactor-system (9 agents) |
  | 3 | `test-agent` | `execute-agent` for test code | write-acceptance-tests, write-contract-tests, disable-tests, enable-tests, refactor-tests (5 agents) |
  | 4 | `prod-commit` | `commit` BPMN node when the upstream phase was a `prod-agent` | COMMIT_SYSTEM after implement-system |
  | 5 | `test-commit` | `commit` BPMN node when the upstream phase was a `test-agent` | COMMIT_TEST_CODE after write-acceptance-tests, COMMIT_TESTS after refactor-tests, COMMIT_LAYER in implement-test-layer |
  | top (highest) | `human` | always prompts; operator-uncontrollable | fix-* agents, refine-acceptance-criteria, BPMN human-STOP nodes, release |

  Order in this table is the iota order in the Go enum and is
  load-bearing: it encodes the threshold ranking.

- **D3 — Default `--auto` floor: `human`.**
  `gh optivem implement --auto` (no `--confirm`) prompts only at
  `human`-tier sites. Everything below — every command, every
  agent dispatch, every commit — auto-yeses. This is a stronger
  semantic shift than today's `--auto` default
  (`commit,fix`-still-prompts): the new model assumes operators who
  type `--auto` mean it.

  Operators who want a more conservative autonomy threshold pass an
  explicit `--confirm=<tier>` (e.g. `--confirm=prod-commit` to still
  prompt at every commit, or `--confirm=command` to effectively
  disable autonomy).

- **D4 — `--yes` is orthogonal, kept as-is.**
  `--yes` short-circuits *per-command* confirmations (bug-report
  prompts, configinit wizards) before `approval.Confirm` is reached.
  It does not conflict with the ladder because the ladder is
  BPMN-engine-scoped (`implement` / `atdd run`). Both flags can be
  passed together; `--yes` fires first for the prompts it covers.

- **D5 — Non-implement sites: all map to `human` placeholder.**
  The 7 non-ATDD `approval.Confirm` call sites
  (cross_repo_commands.go:367 workspace-commit, config/config.go:984,
  release.go:186, main.go:758 + 791 bug-report prompts,
  configinit/prompt.go:250, steps/project.go:460) all get
  `CategoryHuman` in this plan, with a `TODO(non-implement-tiering)`
  comment marking them for the follow-up plan that designs the
  non-ATDD tiering properly.

  Day-1 consequence: `gh optivem workspace commit --auto` no longer
  auto-yeses per-repo prompts (it prompts because everything is
  `human` now). Accepted as a temporary regression — fully resolved
  by the follow-up plan, which will introduce per-domain category
  treatment for non-implement commands.

- **D6 — Scope: `implement` only.**
  The non-implement design is **out of scope** for this plan.
  Workspace-commit's "bulk-yes" semantics, release's always-prompt
  invariant, configinit's wizard model — all addressed in a
  follow-up plan once this one lands.

- **D7 — Mechanism: explicit at every BPMN approve site +
  parse-time validation.**
  The parent plan's mechanism survives: every `process: approve`
  call-activity in `process-flow.yaml` must resolve a `category:`
  (either via the caller's `params:` or pinned on the inner
  ASK_HUMAN node), and the statemachine loader errors at boot if
  any approve site is unresolved. The dispatch-time fallback in
  `classifyApproveCategory` (driver.go:1151) returns an error
  instead of defaulting to `CategoryPrompt`. Defense-in-depth
  exactly as the parent plan defined.

## What changes vs the parent plan

| Aspect | Parent plan (20260527-2119) | This plan |
|---|---|---|
| Enum size | 4 (after CategoryRelease removed: commit, fix, prompt, human) | 6 (command, prod-agent, test-agent, prod-commit, test-commit, human) |
| `--confirm` semantics | Set membership ("tokens that prompt") | Threshold ("tier and above prompt") |
| `--auto` default floor | `commit,fix` still prompts | `human` only (truly autonomous) |
| BPMN approve site pinning | Same mechanism (explicit, no fallback) | Same mechanism, new vocabulary |
| Non-implement reclassification | Per-site rubric (commit/prompt/human) | All `human` placeholder (deferred) |
| Commit tier | Single `commit` | Split `prod-commit` / `test-commit` |
| Fix tier | Separate `fix` | Folded into `human` (always-prompt) |
| `refine-acceptance-criteria` tier | (default) `prompt` | `human` |
| `release.go:186` | `CategoryHuman` (Q6) | `CategoryHuman` (placeholder) |

## Out of scope (deliberately)

- Non-implement tiering — the 7 non-ATDD call sites are placeholder-
  mapped to `human` until the follow-up plan designs them properly.
- Restructuring `approval.go` beyond the enum / parser swap.
- Adding tiers beyond the 6 above. The closed set is final.
- BPMN-orchestrating any non-ATDD CLI command.
- Regenerating diagrams or process-flow artifacts (per
  `feedback_plans_no_diagram_regen`).
- Renaming the `--auto` / `--confirm` flags themselves; only their
  semantics and the `--confirm` vocabulary change.

## Action node inventory

Reference data, identical in shape to the parent plan but re-tiered
under the new ladder. Two tables.

### Agent action nodes (18 total)

**Test-tier (5 agents → `test-agent`):**

| Line | task-name | agent | tier |
|---|---|---|---|
| 1338 | `write-acceptance-tests` | `acceptance-test-writer` | test-agent |
| 1372 | `write-contract-tests` | `contract-test-writer` | test-agent |
| 1607 | `disable-tests` | `test-disabler` | test-agent |
| 1629 | `enable-tests` | `test-enabler` | test-agent |
| 1698 | `refactor-tests` | `test-refactorer` | test-agent |

**Production-tier (9 agents → `prod-agent`):**

| Line | task-name | agent | tier |
|---|---|---|---|
| 1404 | `implement-dsl` | `dsl-implementer` | prod-agent |
| 1435 | `implement-system` | `system-implementer` | prod-agent |
| 1463 | `update-system` | `system-updater` | prod-agent |
| 1488 | `implement-system-driver-adapters` | `system-driver-adapter-implementer` | prod-agent |
| 1513 | `update-system-driver-adapters` | `system-driver-adapter-updater` | prod-agent |
| 1535 | `implement-external-system-driver-adapters` | `external-system-driver-adapter-implementer` | prod-agent |
| 1561 | `update-external-system-driver-adapters` | `external-system-driver-adapter-updater` | prod-agent |
| 1582 | `implement-external-system-stubs` | `external-system-stub-implementer` | prod-agent |
| 1719 | `refactor-system` | `system-refactorer` | prod-agent |

**Human-tier (4 dispatches → `human`):**

| Line | task-name | agent | tier | reason |
|---|---|---|---|---|
| 1653 | `fix-unexpected-passing-tests` | `unexpected-passing-tests-fixer` | human | fix-flow dispatch |
| 1676 | `fix-unexpected-failing-tests` | `unexpected-failing-tests-fixer` | human | fix-flow dispatch |
| 1745 | `refine-acceptance-criteria` | `acceptance-criteria-refiner` | human | always-engage contract step |
| 2220 | `${failure-kind}-fixer` | templated (via `fix` process) | human | fix-flow dispatch |

### Command action nodes (7 total)

| Line | command | tier |
|---|---|---|
| 1765 | `gh optivem compile` | command |
| 1783 | `gh optivem system compile` | command |
| 1801 | `gh optivem test compile` | command |
| 1819 | `gh optivem system build` | command |
| 1859 | `gh optivem system start ${start-flags}` | command |
| 1889 | `gh optivem commit --yes --include-untracked` | (special — see Commit caller table) |
| 1923 | `gh optivem test run` | command |

Note that the BPMN `commit` node is dispatched as an
`execute-command`, but its tier is determined by the caller, not by
the node itself — see next table.

### Commit call-activity callers (4 total)

These pin `category:` on the call-activity that invokes
`process: commit`, not on the commit primitive itself. The commit
node has no intrinsic prod/test classification — only the upstream
phase does.

| Line | call-activity ID | parent process | tier |
|---|---|---|---|
| 801 | `COMMIT_TEST_CODE` | `write-and-verify-acceptance-tests` | test-commit |
| 1079 | `COMMIT_SYSTEM` | `implement-and-verify-system` | prod-commit |
| 1129 | `COMMIT_TESTS` | `refactor-and-verify-tests` | test-commit |
| 1208 | `COMMIT_LAYER` | `implement-test-layer` | test-commit |

3× `test-commit`, 1× `prod-commit`. The asymmetry — production code
only commits once per ticket but tests commit at multiple gates —
reflects the ATDD flow: tests are written, verified, and committed
incrementally as the contract evolves; production code is committed
in one shot once the system implementation passes.

## Items

### 1. Replace the `Category` enum vocabulary

**Files:** `internal/approval/approval.go`,
`internal/approval/approval_test.go`

Rewrite the `Category` enum, its `String()` and `ParseCategory`
arms, and the `allCategories` slice to match the 6-tier ladder.
**Order matters** — iota encodes the threshold ranking, so the
declaration order must be `command` < `prod-agent` < `test-agent` <
`prod-commit` < `test-commit` < `human`.

```go
const (
    CategoryCommand    Category = iota // tier 1 — cheap commands
    CategoryProdAgent                  // tier 2 — production agents
    CategoryTestAgent                  // tier 3 — test agents
    CategoryProdCommit                 // tier 4 — production-code commits
    CategoryTestCommit                 // tier 5 — test-code commits
    CategoryHuman                      // tier 6 — always-prompt, operator-uncontrollable
)
```

CLI tokens: `command`, `prod-agent`, `test-agent`, `prod-commit`,
`test-commit`, `human`.

Delete: `CategoryCommit`, `CategoryFix`, `CategoryRelease`,
`CategoryPrompt`. Their existing call sites are reclassified in
Items 3 + 4.

`approval_test.go` — rewrite the tests:
- `TestCategory_String` covers the 6 new tokens.
- `TestParseCategory_RoundTrip` for the 6 new tokens.
- `TestParseCategory_Invalid_ErrorListsValidSet` updated valid set.
- Delete `CategoryRelease`-specific cases.
- Add `--confirm=fix`, `--confirm=commit`, `--confirm=prompt`,
  `--confirm=release` all error with "unknown category" listing the
  new closed set (regression guards against operators carrying
  muscle memory from the old vocabulary).

**Acceptance:** `go build ./internal/approval/...` succeeds.
`go test ./internal/approval/...` passes. `grep` for the old
`Category*` constants in `internal/` returns matches only in the
call sites scheduled for reclassification in Items 3 + 4.

**Gate before commit:** yes — enum redesign.

### 2. Replace `ConfirmSet` with `ConfirmFloor` (threshold semantics)

**Files:** `internal/approval/approval.go`,
`internal/approval/approval_test.go`,
`internal/cmdctx/cmdctx.go` (if it touches ConfirmSet),
`main.go` (banner that displays the resolved policy)

Replace the `Resolved.ConfirmSet map[Category]bool` with
`Resolved.ConfirmFloor Category`. Replace `Confirm`/`ConfirmVia`
short-circuit logic from
`if r.Auto && !r.ConfirmSet[c] { return true, nil }`
to
`if r.Auto && c < r.ConfirmFloor { return true, nil }`.

The default-when-Auto floor is `CategoryHuman` (D3). Explicit
`--confirm=<tier>` parses the token and sets the floor.

Update `defaultConfirmWhenAuto` (a slice today) to a single
constant: `defaultFloorWhenAuto = CategoryHuman`.

`CategoryHuman` always-prompts is preserved structurally:
`c < CategoryHuman` is false for `c == CategoryHuman`, so human-tier
calls never short-circuit even at floor=human.
`TestConfirm_HumanNeverShortCircuits` adapts to the new shape.

Update `Resolved.ConfirmListString()` — replace the
"comma-joined set" rendering with the floor name. Used by the
startup banner and clauderun's child-env propagation. Rename to
`Resolved.ConfirmFloorString()`.

Update `Resolve()` so `confirmRaw` parses as a single tier token,
not a comma-list. Operators who pass `--confirm=fix,commit`
(multi-token, old syntax) error with a clear message naming the new
single-token vocabulary.

Update `approval_test.go` — replace every test referencing
`ConfirmSet` with the floor equivalent:
- `TestResolve_DefaultAllOff` — no Auto, no floor needed; default
  floor is the zero-value `CategoryCommand` but it's unused (no
  short-circuit when Auto is off).
- `TestResolve_AutoFlag_DefaultsToHuman` — replaces
  `TestResolve_AutoFlag_DefaultsToCommitFix`; asserts
  `r.ConfirmFloor == CategoryHuman` under `--auto` with no
  `--confirm`.
- `TestResolve_ConfirmFlag_ExplicitTier` — replaces
  `TestResolve_ConfirmFlag_Custom`; asserts `--confirm=prod-commit`
  resolves to `r.ConfirmFloor == CategoryProdCommit`.
- `TestResolve_ConfirmFlag_MultiTokenErrors` — new — `--confirm=`
  with a comma in it errors clearly ("threshold is a single tier,
  not a list").
- `TestConfirm_ShortCircuit_BelowFloor` — covers each tier below
  human, asserts auto-yes when floor=human.
- `TestConfirm_Prompts_AtOrAboveFloor` — covers human-tier under
  any floor.

**Acceptance:** `go test ./internal/approval/...` passes. The
`--confirm` vocabulary listed in error messages is the single-token
set.

**Gate before commit:** yes — semantic change to the policy layer.

### 3. Update non-implement `approval.Confirm` call sites to `CategoryHuman`

**Files:**
- `cross_repo_commands.go:367` (workspace commit)
- `internal/config/config.go:984` ("Proceed?")
- `internal/atdd/runtime/release/release.go:186`
- `main.go:758` ("Proceed?" pre-bug-report)
- `main.go:791` ("File a bug report?")
- `internal/configinit/prompt.go:250` ("Do you have an existing
  GitHub Project?")
- `internal/steps/project.go:460` ("Add missing statuses?")

Each call site is a one-line argument swap to
`approval.CategoryHuman`. Add a one-line comment above each:

```go
// TODO(non-implement-tiering): placeholder; proper tier assignment
// deferred to the follow-up plan. See plan
// 20260528-0930-approval-tier-ladder.md §D5.
```

Day-1 behavior change: `gh optivem workspace commit --auto` now
prompts per repo (human-tier never auto-yeses). Accept as a
temporary regression until the follow-up plan re-tiers
non-implement.

Update `cross_repo_commands.go:79` and `:357` doc-comments to drop
the obsolete reference to `CategoryCommit` and replace with
`CategoryHuman + TODO`.

Update `internal/atdd/runtime/agents/registry.go:63` — already uses
`CategoryHuman`, no change.

Update `internal/atdd/runtime/driver/driver.go:686` — already uses
`CategoryHuman` (this is the BPMN human-STOP dispatcher), no change.

Update `internal/atdd/runtime/driver/driver.go:1119-1123` — the
manual-agent dispatcher's `if strings.HasPrefix(agent, "fix-")`
branch. Under the new model the manual-agent dispatch should
always be `CategoryHuman` (D2 + D6: fix → human; everything else
manual-launched is operator-driven anyway). Drop the prefix check;
hardcode `CategoryHuman`.

**Acceptance:** `grep -rn "CategoryCommit\|CategoryFix\|CategoryRelease\|CategoryPrompt" .` returns no matches in non-test Go files.

**Gate before commit:** yes — list the diff before committing.

### 4. Pin `category:` on every BPMN approve call-activity

**Files:** `internal/atdd/runtime/statemachine/process-flow.yaml`

The pinning model:

- **`execute-agent` primitive's APPROVE_PRE + APPROVE_POST**: do
  **not** pin on the primitive itself. Each writing-agent MID
  process (write-acceptance-tests, implement-system, …) declares
  `category: prod-agent` or `category: test-agent` in its
  EXECUTE_AGENT call-activity `params:` block. The execute-agent
  primitive's approve nodes read `${category}` from params via the
  call-activity propagation chain — same mechanism the parent plan
  used for `category: fix` on the `fix` primitive's call-activity
  invocation of approve.

  Specifically, on each writing-agent MID's EXECUTE_AGENT
  call-activity (Action node inventory above), add `category:` in
  `params:`:

  - 5× test-agent MIDs: write-acceptance-tests,
    write-contract-tests, disable-tests, enable-tests,
    refactor-tests → `category: test-agent`.
  - 9× prod-agent MIDs: implement-dsl, implement-system,
    update-system, implement-system-driver-adapters,
    update-system-driver-adapters,
    implement-external-system-driver-adapters,
    update-external-system-driver-adapters,
    implement-external-system-stubs, refactor-system →
    `category: prod-agent`.
  - 1× human MID: refine-acceptance-criteria → `category: human`.

- **`execute-agent` primitive's APPROVE_PRE / APPROVE_POST
  nodes**: thread `${category}` through. The primitive's
  call-activity invocations of `process: approve` must include
  `category: ${category}` in their `params:` blocks so the inner
  approve's ASK_HUMAN resolves the caller's tier. This is the same
  passthrough pattern the parent plan used for fix.

- **`execute-command` primitive's APPROVE_PRE**: do not pin on the
  primitive. Each caller of `process: execute-command` declares
  `category:` in its `params:`. The primitive's invocation of
  `process: approve` threads `${category}` through.

  Per-caller mapping (Command action nodes inventory):
  - 6× `category: command` (compile, system compile, test compile,
    system build, system start, test run).
  - 1× the `commit` process (line 1885) → see commit-callers below.
    The `commit` process itself does NOT pin a `category:` because
    its callers do (4 callers, see next bullet).

- **`commit` process callers** (4 sites, Commit caller table
  above): each `process: commit` call-activity declares `category:`
  in its `params:` block. 3× `test-commit`, 1× `prod-commit`. The
  `commit` process's inner `EXECUTE_COMMAND` call-activity (the one
  that actually invokes `gh optivem commit --yes`) threads
  `${category}` through so the inner execute-command approve gate
  resolves at the caller's commit tier, not as `command`.

- **`fix` process's APPROVE_PRE** (line 2204): replace the existing
  `category: fix` with `category: human`. The fix primitive's
  APPROVE_PRE is the operator-engages point for "something broke,
  here's the proposed remediation" — under D2 + D6 that's a
  human-tier interaction, never bypassable.

- **`fix` process's EXECUTE_AGENT call-activity** (line 2220):
  add `category: human` in its `params:` block. The dispatched
  fix-* agent is itself human-tier (D2), distinct from the
  remediation-approval prompt above.

For each `category:` line added, include a one-line comment
explaining the tier choice, mirroring the style of the parent
plan's edits.

Update the load-bearing-flag comment at process-flow.yaml:1894
(the `gh optivem commit --yes --include-untracked` inner command)
to mark `--yes` as load-bearing for double-prompt avoidance —
preserved from the parent plan, still relevant.

**Acceptance:**
- `grep "process: approve" -A 6` shows a `category:` line under
  every match.
- `grep "process: execute-command" -A 6` shows a `category:` line
  under every match.
- `grep "process: commit" -A 6` shows a `category:` line under
  every match.
- `grep "process: execute-agent" -A 6` shows a `category:` line
  under every match.

**Gate before commit:** yes — content change adding fields to YAML.

### 5. Parse-time validation in the statemachine loader

**Files:** `internal/atdd/runtime/statemachine/load.go`,
`internal/atdd/runtime/statemachine/load_test.go`

After Item 4 lands, every approve site has an explicit category.
Add a validation pass to `LoadBytes` (or `buildProcess`, whichever
is the natural seam) that walks every `process: approve`,
`process: execute-agent`, `process: execute-command`, and
`process: commit` call-activity and verifies one of the following
resolves to a known category token:
- the call-activity's own `params.category`, OR
- the called primitive's transitively-resolvable `category:` chain.

For each unresolved site, error at load with a message naming the
call-activity's process+ID, its file:line if reachable, and the
closed set of valid category tokens.

Implementation note: the statemachine loader doesn't currently
inspect cross-process param propagation. The simplest sufficient
check is shallow: for every approve-or-dispatch call-activity, the
call-activity's own `params.category` must be non-empty OR its
parent process's call-activity (the wrapper) propagated one. For
the primitives (execute-agent / execute-command / approve / fix),
the test is "does the `${category}` placeholder in their `params:`
have a default or is it threaded from a known caller?" — write the
validator to walk the static call graph once at load and verify
every approve-node reaches a literal `category:` token via at most
one level of `${category}` indirection.

Add `load_test.go` cases:
- A process-flow fixture missing `category:` on a writing-agent
  MID's EXECUTE_AGENT errors at load with the offending node named.
- A process-flow fixture with an invalid `category:` token
  (`category: foo`) errors at load with the valid-set listed.
- The shipped `process-flow.yaml` loads cleanly (smoke test
  against the embedded asset).

**Acceptance:** `go test ./internal/atdd/runtime/statemachine/...`
passes. Deliberately stripping `category:` from one
writing-agent MID in `process-flow.yaml` produces a clear load
error naming the node; restore.

**Gate before commit:** yes — behavior change to the loader.

### 6. Replace the dispatch-time fallback in `classifyApproveCategory`

**Files:** `internal/atdd/runtime/driver/driver.go`,
`internal/atdd/runtime/driver/driver_test.go` (if it covers this
path)

`classifyApproveCategory` (driver.go:1151) currently falls through
to `CategoryPrompt` when neither `ctx.Params["category"]` nor
`raw.Category` resolves. Replace that fallback with an error:

```go
func classifyApproveCategory(raw statemachine.RawNode, ctx *statemachine.Context) (approval.Category, error) {
    if v, ok := ctx.Params["category"]; ok && v != "" {
        if c, err := approval.ParseCategory(v); err == nil {
            return c, nil
        }
        return 0, fmt.Errorf("approve node %q: invalid category param %q (valid: %s)", raw.ID, v, validCategoryList())
    }
    if raw.Category != "" {
        if c, err := approval.ParseCategory(raw.Category); err == nil {
            return c, nil
        }
        return 0, fmt.Errorf("approve node %q: invalid category attribute %q (valid: %s)", raw.ID, raw.Category, validCategoryList())
    }
    return 0, fmt.Errorf("approve node %q: no category resolved (should not reach — parse-time validator missed this; valid: %s)", raw.ID, validCategoryList())
}
```

Update the two call sites — driver.go:733 (the approve dispatcher)
and driver.go:1123 (the manual-agent dispatcher) — to surface the
error via the normal `Outcome{Err: …}` path. After Item 3, the
manual-agent dispatcher hardcodes `CategoryHuman` so its call to
`classifyApproveCategory` goes away entirely.

Update the doc-block above `classifyApproveCategory`
(driver.go:1133-1150):
- Replace the "Default: CategoryPrompt" bullet with a "no default —
  parse-time validator is the SSoT" note.
- Drop the "A typo'd `category: foo` falls through to default"
  sentence — typos now error explicitly.

**Acceptance:** `go test
./internal/atdd/runtime/driver/...` passes. A deliberate
process-flow with a missing `category:` would error at the
parse-time gate before this dispatch-time path ever fires (the
defense-in-depth nature).

**Gate before commit:** yes — behavior change.

### 7. Update `--confirm` flag help text + add doc-block rubric

**Files:** `internal/approval/approval.go`, `main.go`

Add a doc-comment block above the `Category` enum (approval.go:25)
describing the tier ladder and threshold semantics:

```go
// Category names a tier in the approval-policy ladder. Tiers are
// ordered low-to-high by stakes:
//
//   command     — execute-command BPMN nodes (compile / build /
//                 start / test run). Cheap, no AI cost, no global
//                 state mutation.
//   prod-agent  — execute-agent for production code (implement-*,
//                 update-*, refactor-system). AI cost; produces
//                 reviewable diffs.
//   test-agent  — execute-agent for tests (write-*-tests,
//                 disable-tests, enable-tests, refactor-tests).
//                 Tests-as-contract: ranked above prod-agent
//                 because broken tests mask regressions.
//   prod-commit — commit BPMN node after a prod-agent phase.
//                 Persistent git write.
//   test-commit — commit BPMN node after a test-agent phase.
//                 Persistent git write of the test contract.
//   human       — always-prompts, operator-uncontrollable. Covers
//                 fix-* agents (signals of upstream defect),
//                 refine-acceptance-criteria (always-engage
//                 contract step), BPMN STOP nodes, release.
//
// `--confirm=<tier>` uses threshold semantics: this tier becomes
// the floor. Tiers at or above the floor still prompt; tiers
// below auto-yes under `--auto`. Default `--auto` floor is
// `human` (truly autonomous).
```

Update the `--confirm` flag's `Usage:` string in `main.go` (around
line 147):

```go
"Threshold tier under --auto: this tier and above still prompt; " +
    "lower tiers auto-yes. Valid: command, prod-agent, test-agent, " +
    "prod-commit, test-commit, human. Default when --auto is set: " +
    "human. Env: " + approval.EnvConfirm + "."
```

Update the `--auto` flag's `Usage:` string similarly to reflect the
new default (line 145-146):

```go
"Auto-approve approvals below the --confirm threshold tier. " +
    "Defaults to --confirm=human (truly autonomous). Env: " +
    approval.EnvAuto + "."
```

**Acceptance:** `go doc github.com/optivem/gh-optivem/internal/approval`
prints the rubric. `gh optivem implement --help` mentions the tier
set and threshold semantics on the `--confirm` line.

**Gate before commit:** yes — wording change.

## Verification

- After Item 4 commit: `gh optivem atdd run --issue <id>` against a
  known scenario still pauses at the same approval points; the
  banner shows the resolved tier (`command` / `prod-agent` /
  `test-agent` / `prod-commit` / `test-commit` / `human`) for each
  approve site.
- After Item 5 commit: deliberately remove a `category:` line from
  one approve site and confirm the run halts with a clear error
  naming the node; restore.
- After Item 6 commit: `gh optivem implement --help` shows the
  tier rubric on `--confirm`.
- After Item 7 commit: `gh optivem implement --auto` (no
  `--confirm`) prompts only at human-tier sites (fix dispatches,
  refine-AC, STOP nodes). Run with `--confirm=prod-commit` and
  verify every commit prompts but prod-agent / test-agent
  dispatches auto-yes.
- `go test ./internal/approval/...
  ./internal/atdd/runtime/driver/...
  ./internal/atdd/runtime/statemachine/...` passes.

## CLI examples (for reference)

```bash
# Fully attended (default): every tier prompts.
gh optivem atdd run --issue 42

# Default autonomous: only human-tier prompts (fix, refine, STOP).
gh optivem atdd run --issue 42 --auto

# Autonomous, but still prompt at every commit (conservative).
gh optivem atdd run --issue 42 --auto --confirm=prod-commit

# Autonomous, prompt at every agent dispatch too.
gh optivem atdd run --issue 42 --auto --confirm=prod-agent

# "Audit mode" — everything prompts even with --auto on.
gh optivem atdd run --issue 42 --auto --confirm=command

# Equivalent to --auto via environment.
GH_OPTIVEM_AUTO=true GH_OPTIVEM_CONFIRM=prod-commit \
    gh optivem atdd run --issue 42

# Non-implement (placeholder behavior — all human until follow-up):
# every prompt shows regardless of --auto.
gh optivem workspace commit --auto       # still prompts per repo
gh optivem release                       # still prompts at release gate
```

## Follow-up plan (out of scope, but flagged)

After this plan lands, a follow-up plan addresses the non-implement
tiering. It will need to decide:

- Does `--auto` apply to non-implement commands at all, or scope
  it to ATDD?
- If yes, does it use the same ladder (with `workspace commit` at
  `prod-commit`, etc.) or a separate vocabulary?
- Does `--yes` get deprecated in favor of `--auto`, or stay as
  the per-command short-circuit?
- The 7 non-implement sites placeholder-mapped to `human` in Item 3
  get their proper tier assignments.

Cross-reference this plan when the follow-up is written.
