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

**Out of scope (deliberately):**

- Removing `CategoryHuman` from `ConfirmSet` (locked invariant; the
  whole point is humans always prompt for STOP nodes).
- Restructuring `approval.go` (already centralized; no design
  change).
- Adding new categories. The closed set `{commit, fix, release,
  prompt, human}` stays as is.
- BPMN-orchestrating the non-ATDD CLI commands (workspace commit,
  release, configinit) — they keep calling `approval.Confirm`
  directly; categories live next to the call.
- Regenerating any diagram or process-flow artifact — see
  `feedback_plans_no_diagram_regen.md`.

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

**Sub-task — enumerate `process: execute-command` callers** and add
`category:` to each. Grep `process-flow.yaml` for `process:
execute-command` call-activities; for each, add `category:` in its
`params:` block alongside the existing `command:` / `question:`. The
category reflects the stakes of the command being run (e.g. a `git
commit` step is `commit`; a `go vet` step is `prompt`).

Add a one-line comment at each new `category:` line explaining the
choice, mirroring the style at process-flow.yaml:2209-2212.

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

For the remaining 6 sites, the audit confirms category choice
against the Item 4 rubric. Expected outcome: most stay as-is
(workspace commit and release are already correctly tiered;
configinit wizards and `main.go` bug-report prompts are genuinely
low-stakes `prompt`). If the audit surfaces an unexpected misfit,
pause and surface it.

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
//   release — release-flow steps (version bumps, tag pushes, artifact
//             uploads); always high-stakes.
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
under --auto (commit, fix, release, prompt, human); see
'approval' package doc for the rubric"`.

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
# prompt + release auto-yes; STOP nodes always prompt).
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

# Release, attended (release.go:186 prompts).
gh optivem release

# Release, autonomous.
gh optivem release --auto --confirm=

# Equivalent to --auto via environment (CI / scheduled runs).
GH_OPTIVEM_AUTO=true GH_OPTIVEM_CONFIRM=human gh optivem atdd run --issue 42
```
