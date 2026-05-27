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

**Pre-decided assignment** (preserves current behavior, makes it
explicit; revisit only if the audit in Item 2 surfaces a mismatch):

- `execute-agent` `APPROVE_PRE` → `category: prompt`
- `execute-agent` `APPROVE_POST` → `category: prompt`
- `execute-command` `APPROVE_PRE` → `category: prompt`
- `fix` `APPROVE_PRE` → already `category: fix`, no change

For each of the three implicit sites, add `category: prompt` as a
sibling of `question:` under `params:`, with a one-line comment
explaining the choice (mirroring the style at line 2209-2212).

**Acceptance:** every `process: approve` call-activity in
`process-flow.yaml` has an explicit `category:` param. `grep "process:
approve" -A 6` shows a `category:` line under each.

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

**Known suspicion to verify, not pre-decide:**
`steps/project.go:460` mutates the GitHub project (adds statuses) —
that is closer to a commit-tier action than a low-stakes prompt.
Either reclassify to `CategoryCommit` or document why it stays
`CategoryPrompt`.

**Acceptance:** audit table appended to this plan. Any reclassification
proposals listed with file:line and one-line justification.

**Gate before commit:** yes — pause after the audit table is written;
operator confirms each reclassification before the Go source is edited.

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

### 5. Drop the soft fallback in `classifyApproveCategory`

**Files:** `internal/atdd/runtime/driver/driver.go`

After Item 1 has pinned every YAML call site, change
`classifyApproveCategory` (driver.go:1151) so the default branch
returns an error instead of `CategoryPrompt`. The function signature
changes from `(…) approval.Category` to `(…) (approval.Category,
error)`; the two call sites (driver.go:733 and driver.go:1123)
surface the error via the normal `Outcome{Err: …}` path.

Update the doc-block (driver.go:1133-1150) to remove the
"typo'd `category: foo` falls through to default" sentence — typos
already fail via `ParseCategory`; missing fields now also fail rather
than silently fall through.

Drop or update the third-bullet "Default: CategoryPrompt" in the
doc-block.

**Acceptance:** a `process: approve` call-activity that omits
`category:` AND whose caller omits `category:` in params produces a
clear dispatch-time error naming the node. The existing four call
sites still resolve correctly.

**Gate before commit:** yes — behavior change.

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
