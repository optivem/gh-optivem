# `gh optivem init` auto-creates the GitHub Project board

> 🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-05-04T10:36:47Z`

## User-facing CLI

What the operator/student types and sees after this change:

- `gh optivem init` — same invocation; now also creates a Project (v2) board with the ATDD-ready status set (`Backlog / Ready / In Progress / In Acceptance / In QA / Done`), links the scaffolded repo(s) to it, and bakes the URL into `gh-optivem.yaml`. No new required flags.
- `--project-url <url>` — **unchanged at the CLI surface**, but now also **verifies that the required ATDD statuses exist** on the supplied project. Skips create + link (the operator-supplied board wins for identity). If any required option (`Ready`, `In Progress`, `In Acceptance`, `In QA`) is missing, the step **prompts the operator for confirmation** before adding it; existing custom columns are preserved either way (nothing is renamed or removed). Aborts with a clear error if the operator declines.
- `--yes / -y` — **reuses the existing `AssumeYes` flag** (`internal/config/config.go:559`). Now also skips the supplied-URL confirmation prompt for non-interactive / CI use. Without it, a non-TTY run with missing required statuses errors out instead of silently mutating the board.
- `--no-project` — **new opt-out**. Skips project creation **and** status-ensure entirely. Intended for CI smoke tests of the scaffolder that shouldn't litter the org with throwaway projects, or for the case where the operator wants to manage the board out-of-band and not be prompted.

Output changes:

- Banner ("Will create…") lists the project board when one is going to be created.
- Summary prints `Project: <url>` alongside `Repository:` / `Actions:`.
- Registration output (`PrintRegistration`) gains a `Project:` line so the URL lands on the registration form.

`gh-optivem.yaml` schema is unchanged — `project_url` is the same field, just populated by `init` instead of by hand.

## Impact on students

**Before:** A student following the ATDD course had to leave the terminal mid-setup, open the GitHub UI, click through to create a Project (v2), realise the default `Todo / In Progress / Done` statuses don't match what the pipeline expects, manually rename them to `Ready / In Progress / In Acceptance` (or guess and get it wrong), copy the URL back into `gh-optivem.yaml`, then re-run. The most common failure mode was a silent one: the board *looked* fine, but the first `implement-ticket` run died with `ErrStatusFieldMissing` because the status options didn't match. That error message is opaque to a beginner.

**After:** `gh optivem init` produces a board that already works. First `implement-ticket` run picks a card from `Ready`, moves it to `In Progress`, then `In Acceptance` — no manual UI step, no status-name guessing, no opaque error.

Other student-visible effects:

- **Consistency across the cohort.** Every student's board has the same five status columns and the same vocabulary. Makes screen-sharing in class and pair work easier — instructors don't have to caveat "yours might say `Todo` instead of `Backlog`."
- **The escape hatch still works *and is now safer*.** A student given a shared department board can still pass `--project-url` and reuse it. The board is checked, and any missing required statuses (most commonly `In Acceptance` and `In QA`) are surfaced — but the student is shown exactly which options are missing and asked to confirm before anything is added. Existing columns on the shared board are preserved either way. The same `ErrStatusFieldMissing` failure mode that bit auto-creation is now also closed for the supplied-URL path, without the scaffolder silently mutating someone else's project.
- **Re-running `init` is safe.** Students often re-run while learning; the step finds the existing project by title and reuses it instead of creating duplicates — same contract as `CreateRepo`.
- **One more thing in the "Will create" banner.** Students see the project board listed alongside the repo before they confirm — no surprise side-effects.

Net: removes the single most common ATDD-onboarding failure mode without changing the command they type.

## Motivation

`gh optivem init` already creates the GitHub repo(s), environments, secrets, and SonarCloud project — but it stops short of creating the **GitHub Project (v2) board** that the ATDD pipeline depends on. Today the operator has two choices:

- Pass `--project-url <url>` (defined at `internal/config/config.go:545`) pointing at a project they created by hand. The URL is baked into `gh-optivem.yaml` by `WriteOptivemYAML` (`internal/steps/optivem_yaml.go:56`).
- Leave it blank and fill in `gh-optivem.yaml` later by hand.

Both paths put manual project setup in the critical-path between scaffolding and a working ATDD cycle. The board itself is mechanical — title, status options, repo link — and there is no reason `init` cannot produce it.

A board created by hand via the GitHub UI also tends to ship with the **wrong status options** for the ATDD pipeline. The default new project has `Todo / In Progress / Done`. The pipeline reads `Ready` (`internal/atdd/runtime/board/board.go:189`), writes `In progress` (`board.go:415`), and writes `In acceptance` (`internal/atdd/runtime/actions/bindings.go:191`) — both sentence case in the source. Comparison is case-insensitive (`equalStatus` at `board.go:382`), so a Title Case board column matches fine; the broader board workflow also needs `In QA` for the QA hand-off column. Most of those are missing by default. A user who runs `init`, manually creates a project via the UI, pastes the URL into the YAML, and tries to run `implement-ticket` will hit `ErrStatusFieldMissing` (`board.go:90`).

The same problem hits the `--project-url` path: a department-supplied board may have arbitrary columns and is not guaranteed to include `In Acceptance` or `In QA`. So the status-ensure logic must run **regardless of how the project came to be** — auto-created or operator-supplied. Only the create + link side of the work is conditional on auto-creation; ensuring the required options exist is universal.

## Approach

Add a new step `EnsureProjectBoard` in `internal/steps/project.go`, slotted into the **Setup repository** phase right after `CreateRepos` (so the repo exists when we link it). The step has two sub-paths driven by whether `cfg.ProjectURL` is set:

**Path A — auto-create (URL not supplied):**

1. List existing projects in the owner: `gh project list --owner <owner> --format json` → look for one matching `cfg.SystemName` (case-sensitive title match; mirrors `CreateRepo`'s identity check).
2. If found: reuse its `{url, number, id}`, log `Reusing existing project board: <url>`. If not found: create via `gh project create --owner <owner> --title <cfg.SystemName> --format json` → parse `{url, number, id}`, log `Created project board: <url>`.
3. Apply the **canonical** Status option set: `Backlog, Ready, In Progress, In Acceptance, In QA, Done` (replace the GitHub default `Todo / In Progress / Done`). Runs on both create and reuse — protects against drift between runs and matches Path B's "always ensure" behaviour.
4. Link the scaffolded repo(s) to the project: `gh project link <number> --owner <owner> --repo <fullRepo>`. For multirepo, link each component repo. On reuse, the link may already exist — implementation should tolerate "already linked" responses gracefully.
5. Write the resulting URL into `cfg.ProjectURL` so the **existing** `WriteOptivemYAML` step (which runs later at `main.go:222` and reads `cfg.ProjectURL`) bakes it into `gh-optivem.yaml`. No separate file-update step needed.

**Path B — supplied via `--project-url`:**

1. Look up the project's Status field via `gh project field-list ... --format json` → existing options.
2. Compute the missing-required set: required = `Ready, In Progress, In Acceptance, In QA` (the ATDD-pipeline-critical minimum); compare case-insensitively (matches `equalStatus` at `board.go:382`).
3. **Additively** add any missing required option via `gh api graphql` with `updateProjectV2Field` (additive — preserves existing custom columns; never renames or removes).
4. Skip create + link — the operator-supplied board owns its own identity and repo associations.

**Both paths share:**

- The same status-ensure helper. Path A calls it with the canonical superset; Path B with just the ATDD-critical minimum.
- The same dry-run behaviour: print the planned `gh` commands, no execution.
- The same failure mode: hard error. A half-set-up board (missing statuses) is worse than a clean failure the user can re-run.
- `cfg.NoProject` short-circuits the entire step (both paths) before any shell call.

The choice to write into `cfg.ProjectURL` rather than mutate the YAML directly is deliberate: `WriteOptivemYAML` is already the single source of truth for "render config into YAML" (the same function the existing `--project-url` path uses). Routing the auto-created URL through the same field keeps one write path.

The choice to ensure statuses additively for Path B (rather than replace the option set) respects the operator's existing board: a department-shared project may legitimately have extra columns (`Blocked`, `On hold`, etc.) that we have no business touching. We only guarantee the ATDD-required minimum exists.

## Items

### 1. New step: `EnsureProjectBoard`

**File:** `internal/steps/project.go` (new).

- `func EnsureProjectBoard(cfg *config.Config, gh *shell.GitHub)` — signature matches the other `Setup*` steps.
- Project title = `cfg.SystemName` (e.g. "Page Turner").
- Early return when `cfg.NoProject` is true (info log, no shell calls).
- Branch on `cfg.ProjectURL`:
  - **Empty:** Path A —
    1. `findOrCreateProject(...)` — list owner's projects via `gh project list --owner <owner> --format json`, match title case-sensitively against `cfg.SystemName`. Reuse on match (log `Reusing existing project board`); create otherwise (log `Created project board`).
    2. `ensureStatusOptions(canonical)` — runs on both create and reuse paths.
    3. `linkRepos(...)` — tolerate "already linked" responses gracefully on reuse.
    4. Write the resulting URL into `cfg.ProjectURL`.
  - **Set:** Path B — resolve the supplied URL to a `{owner, number}` pair (helper `parseProjectURL`), then call `ensureStatusOptions(atddRequired)`. No create, no link.
- Honour `cfg.DryRun` in both paths — print the planned `gh`/`gh api graphql` commands, no execution.
- Use `shell.RunCapture` for any call whose JSON output we parse, so log noise doesn't corrupt parsing.
- Fail-fast (`log.Fatalf`) on parse error, missing Status field, or empty URL after create.
- Log clearly which path ran: `Created project board: <url>` / `Reusing existing project board: <url>` for Path A; `Verified project board: <url>` (or `Added missing statuses: <list>`) for Path B.

**Constants:**

```go
var canonicalStatusOptions = []string{"Backlog", "Ready", "In Progress", "In Acceptance", "In QA", "Done"}
var atddRequiredStatusOptions = []string{"Ready", "In Progress", "In Acceptance", "In QA"}
```

### 2. `ensureStatusOptions` helper (additive)

**File:** same as Item 1.

The shared helper used by both paths. Signature: `ensureStatusOptions(projectID string, required []string) (added []string, err error)`.

- Look up the built-in Status field via `gh project field-list <number> --owner <owner> --format json` → capture its node ID and current options list.
- Compare current options against `required` **case-insensitively** (matching `equalStatus` at `board.go:382`). For each missing entry, queue it for addition.
- Apply additions via `gh api graphql` with the `updateProjectV2Field` mutation. The CLI flag `gh project field-edit --single-select-options` overwrites the option set wholesale and would clobber the operator's existing columns on Path B — so we always go through GraphQL, which lets us pass the union (existing + missing) and is version-stable.
- Path A note: when called with `canonicalStatusOptions` immediately after project creation, the existing options are GitHub's defaults (`Todo, In Progress, Done`). Additive merge would leave `Todo` behind. For Path A we therefore pass an explicit "replace" flag to the helper that overwrites the option set entirely with the canonical list — Path B never sets that flag.
- Log added options: `Added project statuses: In Acceptance, In QA` so the operator can see what changed on a supplied board.

### 3. Repo linking (Path A only)

**File:** same as Item 1.

- Skipped on Path B — operator-supplied boards manage their own repo associations; we don't presume to add ours.
- `gh project link <number> --owner <owner> --repo <fullRepo>` for `cfg.FullRepo`.
- For `cfg.RepoStrategy == "multirepo"`:
  - `multitier`: link `cfg.BackendFullRepo` and `cfg.FrontendFullRepo`.
  - `monolith`: link `cfg.SystemFullRepo`.
- Linking allows `gh project item-list` to surface issues from the repo. Note: linking does NOT auto-add new issues to the board — it just establishes the relationship. Auto-add is **out of scope** for this plan (see "Out of scope" below).

### 4. Wire the step into the pipeline

**File:** `main.go`

- Add to `buildSteps` (around `main.go:209-212`), after `Create repositories` and before `Setup environments`:

```go
{name: "Ensure project board", phase: phaseSetupRepo, fn: func() { steps.EnsureProjectBoard(cfg, gh) }},
```

- Update `printBanner`'s "Will create / will modify" block (`main.go:734-738`):
  - When `cfg.ProjectURL == ""`: list "Project board (auto-create with canonical status set)".
  - When `cfg.ProjectURL != ""`: list "Project board (verify required statuses on supplied URL — may add missing options)".
  - When `cfg.NoProject`: omit entirely.

### 5. Update `printSummary` and `PrintRegistration`

**Files:** `main.go`, `internal/steps/registration.go`

- `printSummary` (`main.go:457`): print the project URL alongside `Repository:` / `Actions:` when `cfg.ProjectURL` is set (it always will be after this change unless creation was skipped via `--no-project`).
- `PrintRegistration` (`internal/steps/registration.go:14`): add a `Project:` line so the value lands on the registration form.

### 6. Optional opt-out flag: `--no-project`

**File:** `internal/config/config.go`

- Add `--no-project` (default `false`) following the convention of `--no-legacy` / `--no-local-tests` / `--no-local-sonar` / `--no-atdd`.
- When set, skip the entire `EnsureProjectBoard` step — **both** create-and-link and the status-ensure on supplied URLs (the step itself checks `cfg.NoProject` and early-returns with an info log).
- Combines with `--project-url`: passing both means "I have a board, don't touch it at all". This is the explicit way to opt out of the status-ensure on a supplied URL.

### 7. Tests

**Files:** new tests in `internal/steps/project_test.go` and additions to existing test files where helpful.

- Pure-logic test: title derivation (`cfg.SystemName` → project title), and the JSON-parse path against captured `gh project list --format json`, `gh project create --format json`, and `gh project field-list --format json` samples.
- **Path A — first-run (create):** stubbed shell records the five command sequence (list, create, field-list, field-update, link). Assert the `gh project list` response is consulted, no match → `gh project create` fires, and the canonical option list (`Backlog, Ready, In Progress, In Acceptance, In QA, Done`) is passed to the `updateProjectV2Field` mutation.
- **Path A — reuse (existing project):** seed `gh project list` response with an existing project whose title matches `cfg.SystemName`. Assert no `gh project create` call, but `ensureStatusOptions` and `linkRepos` still run; the link step tolerates "already linked"; log says `Reusing existing project board`.
- **Path B (supplied URL) — additive merge:** seed the field-list response with `["Todo", "In Progress", "Done"]`. Assert the mutation is called with the union including the four ATDD-required options, that existing `Todo` is preserved, and that the diff log mentions only the additions.
- **Path B — already complete:** seed the field-list response with all four required options (mixed casing). Assert no mutation call (case-insensitive match), and the log says "no changes needed".
- **Path B — confirmation gate:** when missing options are detected, assert the step prompts for confirmation and respects `--yes`/non-interactive flags (see Item 9).
- Skip-when-dry-run: prints the planned commands for the relevant path, no execution.
- Skip-when-`--no-project`: assert no shell calls regardless of `cfg.ProjectURL`.
- End-to-end-ish: stub the shell layer (the package already uses `shell.Run` / `shell.RunCapture`; tests can intercept via the existing test seams used in `github_setup` tests, if any — otherwise extract a small `Runner` interface for this step).

### 9. Confirmation gate for Path B (`--project-url`)

**File:** same as Item 1.

When the operator supplies `--project-url`, they own that board — it may be shared, department-wide, or otherwise outside the scope of the scaffold being created. We must not silently mutate it. The step needs an explicit confirmation before adding any missing status options.

Flow on Path B:

1. Resolve the project, list current Status options, compute the missing-required diff (as described in Item 2).
2. **If the diff is empty:** log `Project board verified: all required statuses present` and continue. No prompt.
3. **If the diff is non-empty:** print a clear summary and prompt the operator:

   ```
   The project at <url> is missing required ATDD statuses:
     - In Acceptance
     - In QA

   To proceed, gh-optivem needs to add these options to the project's Status field.
   Existing options will be preserved. No options will be renamed or removed.

   Add missing statuses? [y/N]:
   ```

4. **On `y` / `yes`:** apply the additions (Item 2), log what was added, continue.
5. **On `n` / anything else:** abort the step with a hard error. The message differentiates by which option(s) were declined:
   - For `Ready` / `In Progress` / `In Acceptance`: "ATDD runtime will fail without this status — the pipeline reads/writes it directly (`internal/atdd/runtime/board/board.go`)."
   - For `In QA`: "ATDD runtime does not read/write `In QA` today, but the broader board workflow expects this column for the QA hand-off lane."

   The operator can either re-run with `--no-project` to skip the check entirely, add the statuses themselves via the GitHub UI, or accept the prompt on the next run.

**Non-interactive mode:** Reuse the existing `AssumeYes` flag (`--yes / -y`, defined at `internal/config/config.go:559`) to bypass the prompt for CI/scripted runs. Update the flag's help text to mention this new prompt alongside the existing existing-repo / `--report-bug` confirmations. When stdin is not a TTY and `--yes` is not passed, fail with the same hard error as a `n` response, instructing the user to add `--yes` or `--no-project`.

**Dry-run interaction:** Under `--dry-run`, print the planned mutation and the prompt that *would* be shown, but do not block on input and do not apply changes.

**Path A is exempt.** When the project is auto-created, the canonical option set is part of the scaffold contract — no separate confirmation needed beyond the existing `init` "Will create…" banner, which already lists the project board (Item 4) and gives the operator their global cancel point before any work begins.

## Out of scope

- **Templated project views / saved filters.** Out for v1. The default board view is fine for ATDD board-mode.
- **Custom fields beyond Status.** ATDD only reads Status. Adding `Priority`, `Estimate`, etc. is a v2 nicety.
- **Cross-repo project sharing.** Each `init` produces its own project, even when multiple init runs share an owner. Multi-repo-per-project is a v2 use case.
- **Migrating existing scaffolded repos to a new auto-created project.** The `--project-url` opt-out covers the "I already have a board" case.
- **Deleting the project on init failure.** `cfg.KeepLocal = true` on error (`main.go:179`) — we keep the local scaffold for inspection; same logic should keep the remote project so the user can inspect it. Cleanup is the user's call.
- **A `gh optivem project` subcommand for board management.** The board CRUD that ATDD does internally is enough; a dedicated CLI surface is a separate plan if it ever becomes useful.
- **Auto-add workflow for new issues.** Linking a repo to a project (Item 3) does not auto-add new issues to the board. Adding `actions/add-to-project@v1` (or equivalent) to the scaffold is deferred — students/instructors handle ticket-to-board mechanics out-of-band (manually, via existing org-wide rules, or in a future plan).

## Order of operations

1. **Verify GraphQL `updateProjectV2Field` mutation** works for adding options to the built-in Status field at the `gh` version pinned in CI. Run against a throwaway project. If it fails, fall back to `gh project field-edit` with full-replace logic and revisit Item 2.
2. Land Items 1, 2, 3, 4 together — the step + its pipeline wiring is one coherent change.
3. Land Item 9 (Path B confirmation gate) in the same PR — it's part of the supplied-URL contract and shouldn't ship without it.
4. Land Item 5 (summary / registration print) in the same PR (small, related).
5. Land Item 6 (`--no-project` flag) in the same PR (one-line addition to the flag set + one-line check in the step).
6. Land Item 7 (tests) in the same PR.
7. **Manual rehearsal — Path A (first run):** run `init` end-to-end against a throwaway test repo, verify the project is created with `Backlog / Ready / In Progress / In Acceptance / In QA / Done`, verify the repo is linked, verify `implement-ticket` resolves the URL from `gh-optivem.yaml` and the ATDD cycle picks/moves cards correctly.
8. **Manual rehearsal — Path A (reuse):** re-run `init` with the same `cfg.SystemName` against the same owner. Verify the existing project is reused (log says `Reusing existing project board`), `ensureStatusOptions` re-runs cleanly (no-op when columns already match), `linkRepos` tolerates the already-linked repo, and the URL in `gh-optivem.yaml` matches the prior run.
9. **Manual rehearsal — Path B:** run `init --project-url <url>` against a project that's missing `In Acceptance` and `In QA`. Verify the prompt fires, lists exactly those two options with the differentiated `In QA` workflow-column note, that confirming adds them while declining aborts the step with the differentiated error message. Re-run with the same URL and verify the second run says "all required statuses present" and does not prompt again.
