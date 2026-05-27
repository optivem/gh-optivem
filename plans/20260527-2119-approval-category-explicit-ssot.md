# Approval category — explicit SSoT, no implicit defaults

## Context

The approval policy (`internal/approval/approval.go`) is already the
centralized chokepoint: one `Resolved{Auto, ConfirmSet}` struct, one
`Confirm(policy, category, …)` call shape used at every confirmation
site, one pair of flags (`--auto` + `--confirm=<list>`) plus env vars
(`GH_OPTIVEM_AUTO` / `GH_OPTIVEM_CONFIRM`). `CategoryHuman` is
hardcoded into `ConfirmSet` (approval.go:182-183) and locked by
`TestConfirm_HumanNeverShortCircuits` — BPMN human-STOP nodes always
prompt.

The remaining gap is **explicitness**, not architecture:

1. Three of four `process: approve` call-activities in
   `process-flow.yaml` rely on the soft fallback in
   `classifyApproveCategory` (driver.go:1151) that defaults to
   `CategoryPrompt` when neither `ctx.Params["category"]` nor
   `raw.Category` resolves. Only the `fix` caller pins itself
   (process-flow.yaml:2213).
2. The 7 non-ATDD `approval.Confirm` call sites (workspace commit,
   release, configinit wizards, bug-report, project-statuses) picked
   their category in Go code without a documented rubric; one of them
   (`internal/steps/project.go:460`, "Add missing statuses?") looks
   borderline — it mutates the GitHub project, which is plausibly
   `CategoryCommit` not `CategoryPrompt`.
3. The category enum has no in-repo rubric describing when to pick
   which token; `--confirm` flag help text doesn't reference one.

Making `category:` explicit at every site and dropping the fallback
turns the YAML/Go choice into the audited SSoT rather than something
inferred at runtime.

## Resolution log

Open design questions surfaced during `/refine-plan` (2026-05-27)
and resolved autonomously per `feedback_autonomous_best_long_term`.
Decisions are folded into the items below; rationale captured here
so the plan stands on its own.

- **Q1 — `execute-command` APPROVE_PRE category.** Do **not** pin a
  category on the `execute-command` primitive's `APPROVE_PRE` node.
  Every call-activity that invokes `process: execute-command` MUST
  pass `category:` in its own `params:` block. Rationale: the stakes
  of `execute-command` are caller-determined (running `ls` vs.
  running `git push`); the primitive cannot honestly declare a
  category. Pinning a default at the primitive contradicts the
  plan's whole premise of explicit SSoT. Cost: enumerate every
  `process: execute-command` caller and add `category:` to its
  params.

- **Q2 — `execute-agent` APPROVE_POST tier vs PRE tier.** Keep both
  PRE and POST as `category: prompt`. Rationale:
  `VALIDATE_OUTPUTS_AND_SCOPES` (process-flow.yaml:2117) is the
  *structural* gate that catches broken agent output;
  `APPROVE_POST` is a human courtesy review. Under `--auto` POST
  auto-yeses, which is correct — autonomous mode is unusable if
  every agent output requires human ack. Operators who want manual
  output review pass `--confirm=prompt`.

- **Q3 — Enforcement layer.** Primary check is **parse-time** in
  the statemachine YAML loader: when loading
  `process-flow.yaml`, validate that every `process: approve`
  call-activity either passes `params.category` or invokes a
  primitive whose `ASK_HUMAN` node has `category:` pinned, and
  error at load if neither resolves. Dispatch-time check in
  `classifyApproveCategory` stays as defense-in-depth ("should
  never fire" path). Rationale: fail-fast at boot beats failing
  mid-run after the operator has invested time in an interactive
  flow.

- **Q4 + Q5 — `CategoryCommit` semantic and rubric wording.**
  `CategoryCommit` covers any **externally-visible state mutation
  with similar permanence**: git writes (commit, push, tag, branch
  move) AND persistent GitHub API mutations (project status add,
  label changes, milestone create). Rubric in Item 4 stands as
  drafted with the "external state mutation" clause. Therefore
  `internal/steps/project.go:460` reclassifies to `CategoryCommit`,
  no longer borderline. Rationale: a stable closed enum beats
  inventing `CategoryMutate` / `CategoryRemote`; "commit = permanent
  write" is an intuitive stretch operators already expect.

- **Q6 — `CategoryRelease` shouldn't exist.** Release is not in
  BPMN (no `release` process or call-activity in
  `process-flow.yaml`). `CategoryRelease` has exactly one consumer
  (`release.go:186`) and, worse, under default `--auto` (ConfirmSet
  = `commit, fix, human`) `release` is NOT in the set, so
  `gh optivem release --auto` silently bypasses the release prompt
  — the opposite of "release should never be automated." Fix:
  reclassify `release.go:186` to `CategoryHuman` (always prompts,
  operator-uncontrollable per the locked invariant) and delete
  `CategoryRelease` from the enum entirely. Net: enum shrinks 5 →
  4 (`commit`, `fix`, `prompt`, `human`).

**Out of scope (deliberately):**

- Removing `CategoryHuman` from `ConfirmSet` (locked invariant; the
  whole point is humans always prompt for STOP nodes).
- Restructuring `approval.go` (already centralized; no design
  change).
- Adding new categories. The remaining closed set `{commit, fix,
  prompt, human}` stays as is. (`release` is removed per Q6 — see
  Item 6.)
- BPMN-orchestrating the non-ATDD CLI commands (workspace commit,
  configinit) — they keep calling `approval.Confirm` directly;
  categories live next to the call.
- Regenerating any diagram or process-flow artifact — see
  `feedback_plans_no_diagram_regen.md`.

## Action node inventory

Reference data — supports Item 1's audit + sub-tasks. Two tables:
every BPMN node that dispatches an agent (`process: execute-agent`)
and every node that dispatches a command (`process: execute-command`).
These two primitives are what reach `ASK_HUMAN` via their
`APPROVE_PRE` / `APPROVE_POST` gates.

### Agent action nodes (18 total)

Split by primary intent: 7 test-intent, 9 production, 2 meta.
Note that some "test agents" also write DSL code as a side-effect
(declaring a new DSL method that gets stubbed in by
`dsl-implementer` later). Annotated in the `also writes` column.

**Test-intent (7):**

| # | Line | task-name | agent | primary scope | also writes |
|---|---|---|---|---|---|
| 1 | 1338 | `write-acceptance-tests` | `acceptance-test-writer` | at-test | dsl-port, dsl-core |
| 2 | 1372 | `write-contract-tests` | `contract-test-writer` | ct-test | dsl-port, dsl-core |
| 11 | 1607 | `disable-tests` | `test-disabler` | at-test, ct-test | — |
| 12 | 1629 | `enable-tests` | `test-enabler` | at-test, ct-test | — |
| 13 | 1653 | `fix-unexpected-passing-tests` | `unexpected-passing-tests-fixer` | at-test, ct-test | dsl-*, driver-*, system-path (broad) |
| 14 | 1676 | `fix-unexpected-failing-tests` | `unexpected-failing-tests-fixer` | at-test, ct-test | dsl-*, driver-*, system-path (broad) |
| 15 | 1698 | `refactor-tests` | `test-refactorer` | at-test, ct-test | dsl-*, driver-* |

**Production (9):**

| # | Line | task-name | agent | write scope |
|---|---|---|---|---|
| 3 | 1404 | `implement-dsl` | `dsl-implementer` | dsl-port, dsl-core, driver-adapter (stubs) |
| 4 | 1435 | `implement-system` | `system-implementer` | system-path |
| 5 | 1463 | `update-system` | `system-updater` | system-path, driver-adapter |
| 6 | 1488 | `implement-system-driver-adapters` | `system-driver-adapter-implementer` | driver-adapter |
| 7 | 1513 | `update-system-driver-adapters` | `system-driver-adapter-updater` | driver-adapter |
| 8 | 1535 | `implement-external-system-driver-adapters` | `external-system-driver-adapter-implementer` | external-system-driver-adapter |
| 9 | 1561 | `update-external-system-driver-adapters` | `external-system-driver-adapter-updater` | external-system-driver-adapter |
| 10 | 1582 | `implement-external-system-stubs` | `external-system-stub-implementer` | external-system-driver-adapter |
| 16 | 1719 | `refactor-system` | `system-refactorer` | system-path |

**Meta (2):**

| # | Line | task-name | agent | scope |
|---|---|---|---|---|
| 17 | 1745 | `refine-acceptance-criteria` | `acceptance-criteria-refiner` | `scope: none` |
| 18 | 2220 | `${failure-kind}-fixer` (fix loop) | templated | inherits from caller |

Under the current categorical model, every agent dispatch stays at
`category: prompt` (Item 1's pinning model). Per-agent
differentiation by stakes is not part of this plan — it surfaced as
a follow-up worth considering if/when the policy pivots to a
severity-levels model. The table above is the basis for that
classification when the time comes.

### Command action nodes (7 total)

| # | Line | command | effect | category |
|---|---|---|---|---|
| 1 | 1765 | `gh optivem compile` | compile working-tree code | `prompt` |
| 2 | 1783 | `gh optivem system compile` | compile system module | `prompt` |
| 3 | 1801 | `gh optivem test compile` | compile test module | `prompt` |
| 4 | 1819 | `gh optivem system build` | build system (produces artifacts) | `prompt` |
| 5 | 1859 | `gh optivem system start ${start-flags}` | launches the system as a running process | `prompt` |
| 6 | 1889 | `gh optivem commit --yes --include-untracked` | writes a git commit (persistent) | `commit` |
| 7 | 1923 | `gh optivem test run` | runs tests, reports outcome | `prompt` |

**Observation — double-prompt risk at #6:** the inner CLI command
(`gh optivem commit`) uses `approval.Confirm(CategoryCommit)` at
`cross_repo_commands.go:367`. The BPMN node suppresses the inner
prompt via the `--yes` flag in the command string. If a future edit
removes `--yes` or the inner command renames the flag, the operator
will be prompted twice for the same logical action. Worth a comment
at line 1894 calling out the `--yes` as load-bearing.

The other six commands do not have inner approval prompts, so this
risk is specific to #6.

## Items

### 1. Audit the 4 `process: approve` call-activities and pin explicit `category:` on each

**Files:** `internal/atdd/runtime/statemachine/process-flow.yaml`

Current state (from `grep "process: approve"`):

| Line | Process | Node | Current category | Question |
|---|---|---|---|---|
| 2037 | `execute-agent` | `APPROVE_PRE` | (default → prompt) | "Do you approve agent ${task-name} to execute?" |
| 2079 | `execute-agent` | `APPROVE_POST` | (default → prompt) | "Approve output from agent ${task-name}?" |
| 2143 | `execute-command` | `APPROVE_PRE` | (default → prompt) | "Do you approve running command ${command} ?" |
| 2204 | `fix` | `APPROVE_PRE` | `fix` (explicit) | "Do you approve fix to attempt remediation for ${failure-kind} ?" |

**Pinning model** (resolved Q1 + Q2):

- `execute-agent` `APPROVE_PRE` → pin `category: prompt` on the
  ASK_HUMAN node inside the `execute-agent` process (NOT on the
  inner `approve` primitive — pin via the call-activity `params:`
  block, sibling of `question:`).
- `execute-agent` `APPROVE_POST` → pin `category: prompt`. Behavior-
  preserving; `VALIDATE_OUTPUTS_AND_SCOPES` is the structural gate.
- `execute-command` `APPROVE_PRE` → **do NOT pin** at the primitive.
  Every caller of `process: execute-command` must pass `category:`
  in its `params:` block. The primitive's `approve` call-activity
  must propagate the caller's `category:` through to its own
  `params:`.
- `fix` `APPROVE_PRE` → already `category: fix`, no change.

**Sub-task — declare `category:` on all 7 `process: execute-command`
callers** per the **Command action nodes** table in the Action node
inventory above. Six are `prompt` (compile / build / start / test
run — no persistent state change); only node #6 (`gh optivem commit`,
line 1889) is `commit`.

For each call-activity, add `category:` in its `params:` block
alongside the existing `command:`. Add a one-line comment at each
`category:` line explaining the choice, mirroring the style at
process-flow.yaml:2209-2212.

Also add a comment at line 1894 marking the `--yes` flag in the
commit command as load-bearing for double-prompt avoidance (see
the inventory's observation).

**Acceptance:** every `process: approve` call-activity in
`process-flow.yaml` has an explicit `category:` (either pinned on
the primitive's ASK_HUMAN node or passed through from a caller).
`grep "process: approve" -A 6` and `grep "process: execute-command"
-A 6` show a `category:` line under each.

**Gate before commit:** yes — content change adding fields to YAML.

### 2. Audit the 7 non-ATDD `approval.Confirm` call sites and report reclassifications

**Files (read-only audit, then propose edits):**

- `cross_repo_commands.go:367` — `CategoryCommit`, "Commit these changes to %s?"
- `internal/config/config.go:984` — `CategoryCommit`, "Proceed?"
- `internal/atdd/runtime/release/release.go:186` — `CategoryRelease`
- `main.go:758` — `CategoryPrompt`, "Proceed?"
- `main.go:791` — `CategoryPrompt`, "File a bug report?"
- `internal/configinit/prompt.go:250` — `CategoryPrompt`, "Do you have an existing GitHub Project?"
- `internal/steps/project.go:460` — `CategoryPrompt`, "Add missing statuses?"

For each site, read the surrounding code, determine what action
follows the prompt, and judge category fit against the rubric
established in Item 4. Produce a markdown table inline in this plan
under a `## Item 2 audit findings` subsection with: site, current
category, recommended category, rationale, change-required (y/n).

**Pre-decided from Q4+Q5:** `steps/project.go:460` ("Add missing
statuses?") reclassifies to `CategoryCommit` — adding GitHub project
statuses is a persistent externally-visible mutation, which the
extended `CategoryCommit` semantic explicitly covers. No further
investigation needed for this site.

**Pre-decided from Q6:** `release.go:186` reclassifies from
`CategoryRelease` to `CategoryHuman` — release should never be
automated, and `CategoryHuman` is the only tier that operators
cannot bypass (locked invariant at approval.go:182). After this
reclassification `CategoryRelease` has zero consumers and is
removed in Item 6.

For the remaining 5 sites, the audit confirms category choice
against the Item 4 rubric. Expected outcome: most stay as-is
(workspace commit is already correctly tiered; configinit wizards
and `main.go` bug-report prompts are genuinely low-stakes `prompt`).
If the audit surfaces an unexpected misfit, pause and surface it.

**Acceptance:** audit table appended to this plan under the
`## Item 2 audit findings` header below. Any unexpected
reclassification proposals listed with file:line + justification
and pause for confirmation.

**Gate before commit:** yes — list the diff of category changes
before committing the Go source edits.

### 3. Apply reclassifications approved in Item 2

**Files:** whichever Go files Item 2 surfaced.

Each call site is a one-line argument change
(`approval.CategoryPrompt` → `approval.CategoryCommit`, etc.). No
structural changes.

**Acceptance:** every `approval.Confirm` / `ConfirmVia` call in the
repo uses a category consistent with the Item 4 rubric.

**Gate before commit:** yes — per-file approval is implicit because
Item 2 pre-approved each reclassification, but list the diff before
committing.

### 4. Document the category rubric in `approval.go` and surface it in `--confirm` help text

**Files:** `internal/approval/approval.go`, `main.go`

Add a doc-comment block above the `Category` enum (approval.go:25) that
expands to a per-category rubric. Suggested text:

```
// Category names a class of confirmation prompt. The closed set is pinned
// here so the --confirm=<list> vocabulary is a public, composable contract.
//
// Pick the category that matches the *stakes* of the prompt, not the
// surface action:
//
//   commit  — anything that writes to git (commit, push, tag, branch
//             move) or mutates external state with similar permanence
//             (e.g. GitHub project status changes that other commands
//             will see).
//   fix     — fix-on-failure dispatch points; the operator wants tight
//             control over remediation even under --auto.
//   prompt  — low-stakes wizard / dispatch confirmations where the
//             operator's only realistic answer is yes (config wizard
//             questions, "do you have an existing GitHub Project?",
//             execute-agent pre-dispatch, generic "Proceed?"). Bypasses
//             cleanly under --auto with the default exclusion set.
//   human   — BPMN human-STOP nodes. Always in ConfirmSet; never
//             bypassed; operator-uncontrollable.
```

Update the `--confirm` flag's `Usage:` string in `main.go` (around
line 145-148) so `gh optivem --help` shows the same rubric in a
compact form, e.g. `"comma-separated categories that still prompt
under --auto (commit, fix, prompt, human); see 'approval' package
doc for the rubric"`.

**Acceptance:** `go doc github.com/optivem/gh-optivem/internal/approval`
prints the rubric. `gh optivem implement --help` mentions the
category set on the `--confirm` line.

**Gate before commit:** yes — wording change, review before commit.

### 5. Enforce `category:` at YAML parse time; drop the dispatch-time fallback

**Files:** `internal/atdd/runtime/statemachine/*.go` (the loader),
`internal/atdd/runtime/driver/driver.go`

Per Q3, the primary gate is **parse-time** validation. The
dispatch-time check stays as defense-in-depth.

**Parse-time validation (primary):**
In the statemachine loader (wherever `process-flow.yaml` is parsed
into `RawProcess` / `RawNode`), add a validation pass that walks
every `process: approve` call-activity and verifies one of the
following resolves to a known category token:
  - the call-activity's own `params.category`, OR
  - the called `approve` primitive's `ASK_HUMAN` node's
    `category:` field.

Any unresolved call-activity errors at load with a message naming
the call-activity ID, its file:line, and the closed set of valid
category tokens.

**Dispatch-time defense-in-depth:**
Update `classifyApproveCategory` (driver.go:1151) so its final
fallback returns an error rather than `CategoryPrompt`. Signature
changes from `(…) approval.Category` to `(…) (approval.Category,
error)`; the two call sites (driver.go:733 and driver.go:1123)
surface the error via the normal `Outcome{Err: …}` path. The error
message is "should-not-reach" prose pointing at the parse-time
validator as the primary gate.

Update the doc-block (driver.go:1133-1150) to:
- remove the "typo'd `category: foo` falls through to default"
  sentence — typos already fail via `ParseCategory`,
- drop the third-bullet "Default: CategoryPrompt" since there is
  no default,
- reference the parse-time validator as the SSoT.

**Acceptance:**
- A `process: approve` call-activity that omits `category:` AND
  whose caller omits `category:` in params fails at load time with
  a clear error naming the offending node.
- The dispatch-time error path is unreachable in normal operation
  (parse-time gate catches it first) but produces a sensible error
  if exercised in a test.
- All existing call sites still resolve correctly after Item 1.

**Gate before commit:** yes — behavior change touching the loader
and the driver.

### 6. Remove `CategoryRelease` from the approval enum

**Files:** `internal/approval/approval.go`,
`internal/approval/approval_test.go`,
`internal/atdd/runtime/release/release.go`, `main.go`

After Item 3 has updated `release.go:186` to use `CategoryHuman`
(per the Item 2 reclassification table), `CategoryRelease` has zero
remaining consumers and is deleted:

- `approval.go:32` — remove `CategoryRelease Category = iota` from
  the enum. Subsequent iota values shift; verify no code depends on
  the integer rank.
- `approval.go:44` — remove the `case CategoryRelease: return
  "release"` arm in `String()`.
- `approval.go:61` — remove `CategoryRelease` from the
  `allCategories` slice.
- `approval.go:75` — remove the `case "release": return
  CategoryRelease, nil` arm in `ParseCategory`.
- `approval_test.go` — remove tests that exercise `CategoryRelease`
  by name. Add one test case asserting that `--confirm=release`
  produces a `ParseCategory` error citing the new closed set.
- `main.go` — update the `--confirm` flag `Usage:` string to drop
  `release` from the documented vocabulary (already covered by
  Item 4's rubric edit; this is the verification of that change).

**Acceptance:** `grep -rn CategoryRelease internal/` returns no
matches. `go build ./...` succeeds. `gh optivem implement --auto
--confirm=release` errors with "unknown category 'release'; valid:
commit, fix, prompt, human". `gh optivem release --auto` still
prompts at the release gate (because `release.go:186` now uses
`CategoryHuman`, which is in `ConfirmSet` unconditionally).

**Gate before commit:** yes — enum removal, touches multiple files.

## Item 2 audit findings

_(To be filled in during Item 2 execution. Leave header in place so
the executor knows where to write the table.)_

## Verification

- After Item 1 commit: `gh optivem atdd run --issue <id>` against a
  known scenario still pauses at the same approval points as before.
- After Item 5 commit: deliberately remove a `category:` line from
  one `process: approve` call site and confirm the run halts with a
  clear error naming the node; restore.
- `gh optivem implement --help` shows the category rubric on the
  `--confirm` line.
- `go test ./internal/approval/... ./internal/atdd/runtime/driver/...`
  passes (executor's discretion on adding new tests).

## CLI execution examples (for reference)

How the autonomous policy reads from the operator's command line —
included here so reviewers can sanity-check what behavior we are
locking in.

```bash
# Fully attended (default): every approval point prompts.
gh optivem atdd run --issue 42

# Autonomous with default exclusions (commit + fix still prompt;
# prompt auto-yeses; STOP nodes always prompt).
gh optivem atdd run --issue 42 --auto

# Autonomous, only human-STOP nodes pause (most unattended setup).
gh optivem atdd run --issue 42 --auto --confirm=human

# Autonomous, bypass everything that can be bypassed; STOP nodes
# still prompt (CategoryHuman is operator-uncontrollable).
gh optivem atdd run --issue 42 --auto --confirm=

# Workspace commit, attended (per-repo prompt for each dirty repo).
gh optivem workspace commit

# Workspace commit, fully autonomous (no per-repo prompt).
gh optivem workspace commit --auto --confirm=

# Release always prompts — release.go:186 uses CategoryHuman, which
# is operator-uncontrollable. --auto / --confirm= have no effect on
# the release gate.
gh optivem release

# Equivalent to --auto via environment (CI / scheduled runs).
GH_OPTIVEM_AUTO=true GH_OPTIVEM_CONFIRM=human gh optivem atdd run --issue 42
```
