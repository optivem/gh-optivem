# Complete the snake → kebab placeholder-naming migration

## Why

The codebase is mid-migration from snake_case to kebab-case for the
`${name}` placeholder vocabulary that flows through ATDD dispatch:

- **YAML placeholders** (`process-flow.yaml`) — fully kebab.
- **State stashes from runCommand / validateOutputsAndScopes**
  (`bindings.go:732-958`) — fully kebab (`failure-kind`, `command-line`,
  `failing-task-name`, `missing-outputs`, …).
- **PARSE_TICKET state stashes** (`bindings.go:357-360`) — migrated to
  kebab (no prefix) in commit landing alongside this plan; see "Path A"
  context below.
- **Renderer placeholder registry** (`clauderun.go:674-747`) — **mixed**.
  Older entries are snake (`acceptance_criteria`, `language`,
  `parsed_concepts`, `command`, `command_exit_code`,
  `command_stderr_tail`, `ticket_id`); newer entries are kebab
  (`failing-task-name`, `missing-outputs`, `violating-paths`).
  Line 737-739 already documents kebab as the project convention:
  > Kebab-cased placeholder names mirror the kebab state keys the
  > action stashes (failing-task-name, missing-outputs,
  > scope-violating-paths).
- **Agent prompt bodies** (`internal/assets/runtime/agents/atdd/*.md`)
  — **mixed**. Same split: older content placeholders snake
  (`${scope_block}`, `${changed_files}`, `${verify_results}`,
  `${acceptance_criteria}`, `${parsed_concepts}`, `${expected_outputs}`,
  `${references_root}`, `${disable_marker_example}`,
  `${disable_marker_removal_example}`, `${command_exit_code}`,
  `${command_stderr_tail}`); newer ones kebab (`${dsl-port}`,
  `${at-test}`, `${test-names}`, `${failing-task-name}`,
  `${missing-outputs}`, `${violating-paths}`, every `${*-path}` and
  `${*-port}` / `${*-adapter}`).

The half-migrated state has produced two concrete bugs already:

1. The `${acceptance-criteria}` unresolved-placeholder failure observed
   in the 2026-05-27 09:27 ATDD rehearsal — PARSE_TICKET stashed
   `ticket_acceptance_criteria` (snake + `ticket_` prefix) while YAML
   referenced `${acceptance-criteria}` (kebab, no prefix). Closed under
   Path A (state-layer kebab without prefix).
2. The `${suite}` literal leak surfaced 2026-05-27 ~01:55 CEDT,
   triggering the audit plan
   `plans/20260527-0205-expandparams-unresolved-placeholder-audit.md`
   and the strict-mode flip in commit `8b1b83b`.

Strict-mode `ExpandParams` (now live) will keep surfacing new variants
of this bug at every fresh dispatch site until the convention split is
gone. Path A closed the immediate rehearsal failure by aligning the
state layer; Path B finishes the job by aligning the renderer +
prompt-body layers, so the project converges on one canonical
placeholder spelling.

## Scope

In scope:

- `internal/atdd/runtime/clauderun/clauderun.go` — renderer placeholder
  registrations (the snake entries listed above) and their surrounding
  doc-comments.
- `internal/atdd/runtime/clauderun/clauderun_test.go` — every assertion
  referencing a renamed `${snake_case}` placeholder.
- `internal/assets/runtime/agents/atdd/*.md` — every snake-cased
  `${name}` placeholder in every agent prompt body. (Inventory in
  "Item 2" below.)
- `internal/atdd/runtime/clauderun/clauderun.go::findUnfilledPlaceholders`
  — confirm the regex covers kebab names equivalently (it should, but
  pin it with a test).
- A new regression test that asserts no `${[a-z]+_[a-z]+}` survives in
  the renderer's registered params or in any agent .md file under
  `internal/assets/runtime/agents/atdd/`.

Out of scope:

- The PARSE_TICKET state-stash renames (Path A — already landed).
- YAML placeholder renames (already kebab everywhere).
- Single-word placeholders (`${language}`, `${architecture}`,
  `${command}`, `${phase}`, `${checklist}`) — no separator means no
  convention question.
- The `${suite}` / `${test-names}` audit conclusions (already
  closed by commit `bd1c958` + the audit plan).

## Items

### 1. Renderer registration sweep (`clauderun.go:674-747`)

Rename, one per entry:

| Before | After |
|--------|-------|
| `params["acceptance_criteria"]` | `params["acceptance-criteria"]` |
| `params["parsed_concepts"]` | `params["parsed-concepts"]` |
| `params["command_exit_code"]` | `params["command-exit-code"]` |
| `params["command_stderr_tail"]` | `params["command-stderr-tail"]` |
| `params["ticket_id"]` | `params["ticket-id"]` |
| `params["disable_marker_example"]` | `params["disable-marker-example"]` |
| `params["disable_marker_removal_example"]` | `params["disable-marker-removal-example"]` |

`params["scope_block"]`, `params["changed_files"]`, `params["verify_results"]`,
`params["expected_outputs"]`, `params["references_root"]` — check the
registration sites I didn't enumerate (grep for `params\["[a-z_]+_`)
and rename in the same pass.

Doc-comments in the registration block update inline (e.g. the
explicit kebab-convention note at line 737-739 becomes the universal
rule, not a partial-migration flag).

### 2. Agent `.md` prompt-body sweep

Files to edit (full inventory from `grep '${[a-z_]+_[a-z_]+}'`):

- `acceptance-test-writer.md` — `${acceptance_criteria}`, `${scope_block}`, `${expected_outputs}`, `${references_root}`
- `contract-test-writer.md` — `${scope_block}`, `${expected_outputs}`, `${references_root}`
- `dsl-implementer.md` — `${scope_block}`, `${expected_outputs}`, `${references_root}`
- `external-system-stub-implementer.md` — `${scope_block}`
- `external-system-driver-adapter-implementer.md` — `${scope_block}`
- `external-system-driver-adapter-updater.md` — `${scope_block}`
- `system-driver-adapter-implementer.md` — `${scope_block}`
- `system-driver-adapter-updater.md` — `${scope_block}`
- `system-implementer.md` — `${scope_block}`
- `system-updater.md` — `${scope_block}`
- `system-refactorer.md` — `${scope_block}`
- `test-refactorer.md` — `${scope_block}`
- `acceptance-criteria-refiner.md` — `${parsed_concepts}` (×3)
- `command-failed-fixer.md` — `${scope_block}`, `${command_exit_code}`, `${command_stderr_tail}`, `${changed_files}`
- `missing-output-fixer.md` — `${scope_block}`, `${changed_files}`
- `scope-diff-fixer.md` — `${scope_block}`, `${changed_files}`
- `unexpected-passing-tests-fixer.md` — `${scope_block}`, `${verify_results}`, `${changed_files}`
- `unexpected-failing-tests-fixer.md` — `${scope_block}`, `${verify_results}`, `${changed_files}`
- `test-disabler.md` — `${scope_block}`, `${disable_marker_example}`
- `test-enabler.md` — `${scope_block}`, `${disable_marker_removal_example}`

Each rename is mechanical: `${snake_name}` → `${snake-name}` (replace
the underscore that separates words; single-word entries unaffected).

### 3. Test updates

`internal/atdd/runtime/clauderun/clauderun_test.go`:

- Lines around 306-307: `${checklist}` assertion — already
  single-word, no change.
- Lines around 532-545, 553-590: `${acceptance_criteria}` assertions
  rename to `${acceptance-criteria}`.
- Any `mustContain(t, got, "${...}")` calls — rename the matched
  literal.

`internal/atdd/runtime/driver/driver_test.go`:

- Doc-comments that say `${acceptance_criteria}` update to
  `${acceptance-criteria}` (the actual `c.Set("acceptance-criteria", …)`
  already landed under Path A).

### 4. Drift-alarm regression test

In `internal/atdd/runtime/clauderun/clauderun_test.go`, add:

```go
func TestNoSnakeCasePlaceholdersInPromptBodies(t *testing.T) {
    // Once Path B lands, no rendered prompt body or registered
    // placeholder name should contain a `${foo_bar}`-shape token.
    // Single-word names (${language}, ${command}, ${phase}) are
    // unaffected — the assertion fires only on underscore-as-separator.
    re := regexp.MustCompile(`\$\{[a-z]+_[a-z]+[a-z_]*\}`)
    // …walk every .md under internal/assets/runtime/agents/atdd/,
    // fail with the file + offending placeholder.
}
```

And a sibling test against `renderPrompt`'s output for a representative
agent of each shape (writer, fixer, refactorer) to catch the case where
the registration uses kebab but the prompt body still says snake.

### 5. Doc-comment audit

Sweep these doc-comments for stale `${snake_case}` references:

- `bindings.go:332-343` (parseTicket comment) — already updated under Path A.
- `clauderun.go:105-132` (Checklist + AcceptanceCriteria doc-comments).
- `clauderun.go:212-222` (Language doc-comment — already kebab-friendly).
- `clauderun.go:135-152` (ParsedConcepts + VerifyResults).
- `intake/parse.go:24, 80-83, 172` (Section / Parse / ExtractChecklist
  comments mentioning the prompt's `${checklist}` /
  `${acceptance_criteria}` substitutions).
- Any prompt-doc index under `docs/atdd/code/` that lists placeholders
  by name.

## Sequencing

1. **Item 1 + Item 5 (renderer + comments)** in one commit. Tests will
   fail because `renderPrompt` emits kebab params but agent .md files
   still reference snake placeholders — strict-mode `ExpandParams` +
   `findUnfilledPlaceholders` will catch it.
2. **Item 2 (agent .md sweep)** in a second commit immediately after.
   Tests should go green again.
3. **Item 3 + Item 4 (test renames + drift-alarm test)** in a third
   commit. The drift-alarm test makes future regressions impossible.

Doing items 1 and 2 as separate commits is intentional: the broken
intermediate state surfaces every snake placeholder that the audit
checklist missed (if any). Squashing into one commit hides that
verification.

## Risks

- **Prompt-body churn affects agent dispatch deterministically only if
  the renderer registration and the prompt body match.** Splitting
  Items 1 and 2 across commits means the dispatcher is broken between
  them. Land them back-to-back; do not pause for review between.
- **Parallel sessions editing agent prompts** would conflict
  catastrophically with Item 2. Check `git status` and grep
  `plans/*.md` for pickup markers on the agent files before starting.
- **Memory `feedback_check_concurrent_agents` + `feedback_never_create_patches`**
  apply — when committing Item 2, `git add` whole files only.
- **The drift-alarm regex assumes ASCII lowercase + underscore.**
  Confirm no agent files use uppercase or hyphens in the snake
  pattern (none do today, but pin it).

## References

- Path A commit: PARSE_TICKET state-key rename, 2026-05-27 09:49 CEDT.
- `plans/20260527-0205-expandparams-unresolved-placeholder-audit.md` —
  the audit plan that drove strict-mode `ExpandParams`. Its audit
  table is now superseded for the PARSE_TICKET state keys (rows are
  resolved under Path A) and would be re-runnable post-Path-B as a
  one-line invariant ("no `${[a-z]+_[a-z]+}` in YAML or .md").
- `clauderun.go:737-739` — the kebab-convention comment that this plan
  promotes from "partial-migration flag" to "universal rule".
- Memory `feedback_schema_fields_earn_slot` — snake placeholders that
  exist only because the renderer hasn't been migrated yet don't
  earn their slot; rename them.
- Memory `feedback_drop_dont_relocate` — the snake → kebab rename is
  not a relocation; the snake names go away entirely.
