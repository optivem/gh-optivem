# Convert snake_case ctx.State and YAML params keys to kebab-case

## Why

The naming-sweep audit (one-shot plan `20260527-1210-agent-naming-sweep-singular-vs-plural`, executed and deleted same day with zero shape-rule hits — see git history) found that the gh-optivem codebase has two layered naming conventions for runtime-context keys:

- **Phase D (newer, documented in `gates/bindings.go:252–256`)** —
  all `ctx.State` / `ctx.Params` / `r.Register(...)` keys are
  kebab-case (`command-succeeded`, `test-outcome`, `dsl-port-changed`).
- **Pre-Phase-D (older driver + gate code)** — a handful of keys
  predate the kebab convention and are still snake_case
  (`issue_url`, `phase_scope_clean`, `cycle_phase`, etc.).

The mix is purely a case-style drift; the singular-vs-plural shape
audit found zero shape hits. Aligning the legacy keys to the documented
Phase D convention removes the only remaining case-style outlier from
the runtime so future readers don't have to know which generation a key
belongs to.

## Convention to enforce

- All `ctx.State` keys → kebab-case.
- All `ctx.Params` keys → kebab-case.
- All `${name}` placeholders inside `params:` blocks in
  `process-flow.yaml` → kebab-case.
- All keys inside `params:` blocks in `process-flow.yaml` → kebab-case.
- Comments, doc strings, and error messages referencing these keys
  must be updated to the new spelling so grep stays honest.

Go identifier names (struct field, local var, const name) stay in
Go-idiomatic camelCase / PascalCase — only the **string value** of the
key changes. Example:

```go
// before
CtxKeyPhaseScopeClean = "phase_scope_clean"
// after
CtxKeyPhaseScopeClean = "phase-scope-clean"
```

## Scope

In scope (read AND rename the string values):

- `internal/atdd/runtime/gates/bindings.go` and `bindings_test.go`
- `internal/atdd/runtime/actions/bindings.go` and `bindings_test.go`
- `internal/atdd/runtime/statemachine/process-flow.yaml` —
  `params:` keys and `${name}` placeholders that match the renamed
  keys.
- Any other file under `internal/` that reads or writes these keys
  (driver code that calls `ctx.Set(...)` to seed `issue_*` / `phase_*`
  is the main candidate; `Grep` confirms scope before each rename).
- Test fixtures and harness code that seed these keys.

Out of scope (do NOT edit):

- `docs/process-diagram.md`, `docs/architecture-diagram.md`,
  `docs/images/process-diagram-*.svg` — auto-generated.
- `plans/archived/**` — historical record.
- Memory files under `~/.claude/projects/.../memory/`.
- Agent prompts under `internal/assets/runtime/agents/atdd/*.md` —
  the audit confirmed no `${snake_case}` placeholders appear in any
  agent body; only the parameter-doc lines in `test-disabler.md` and
  `test-enabler.md` describe the snake_case names. Those doc lines
  get updated when their kebab counterparts land (see Item 2).

## Items

The audit found the following keys to convert. Each item is a single
focused rename (operator preference: small, focused diffs; one commit
per rename for easier revert).

### 1. Ticket-id + issue-handle quintet (driver pre-resolve keys)

Rename in lockstep — these five are seeded together by the driver
pre-resolve step (`driver.preResolveIssue` + the adjacent
`sCtx.Set("ticket_id", issue.ID)` at `driver.go:580`) and consumed
together by `issueFromContext` and the YAML `commit:` subprocess:

- `ticket_id` → `ticket-id`
- `issue_num` → `issue-num`
- `issue_url` → `issue-url`
- `issue_title` → `issue-title`
- `issue_handle` → `issue-handle`

Sweep sites confirmed by audit + post-`d0e0b00` re-grep:

- `internal/atdd/runtime/gates/bindings.go` (lines 171–184) — string
  reads, comment, error message.
- `internal/atdd/runtime/actions/bindings.go` (lines 269–317, 363–376)
  — `ctx.GetString(...)` calls + error messages.
- `internal/atdd/runtime/gates/bindings_test.go` (lines 645, 685,
  707, 719) — test setup + assertion strings.
- `internal/atdd/runtime/driver/driver.go` (lines 570–580, 931) —
  `sCtx.Set("ticket_id", issue.ID)` seed, `ctx.GetString("ticket_id")`
  read, and the `// ticket_id is a backend-agnostic alias` doc
  paragraph (needs rewording to `ticket-id`).
- `internal/atdd/runtime/clauderun/clauderun.go` (lines 64–68, 702)
  — the existing `params["ticket-id"]` (already kebab) and the
  comment explaining the snake-vs-kebab alias. After rename the
  alias collapses to a single canonical kebab key.
- `internal/atdd/runtime/statemachine/process-flow.yaml` — the
  `commit:` subprocess uses `"#${ticket_id} ${issue_title}"` (line
  ~1841 post-`d0e0b00`); rename both placeholders.
- `internal/assets/runtime/agents/atdd/test-disabler.md` (line 17)
  + `test-enabler.md` (line 17) — parameter-doc lines referencing
  `${ticket_id}` in agent prompts. Update both the prose and the
  literal placeholder.
- Anywhere else `Grep` finds the snake string under `internal/`.

One commit. Verify with `go build ./...` + `go test ./internal/atdd/...`
and an ATDD rehearsal in a sandbox repo to confirm both the issue-
quartet and the commit-message `${ticket-id}` substitution still
fire correctly.

### 2. Phase-scope keys

Rename in lockstep — both are written by `check_phase_scope` and read
by the `phase-scope-clean` gate / STOP_SCOPE_VIOLATION payload:

- `phase_scope_clean` → `phase-scope-clean`
- `phase_scope_violating_paths` → `phase-scope-violating-paths`

Sweep sites confirmed by audit:

- `internal/atdd/runtime/gates/bindings.go` (lines 232–245) —
  string reads, comment, error message.
- `internal/atdd/runtime/actions/bindings.go` (lines 167, 225, 231,
  431) — const value, comment.
- `internal/atdd/runtime/gates/bindings_test.go` (lines 204, 217, 231)
  — test setup + assertion strings.

The Go const names (`CtxKeyPhaseScopeClean`,
`CtxKeyPhaseScopeViolatingPaths`) stay in PascalCase; only their
string values flip.

One commit.

### 3. Output-file-path key

- `output_file_path` → `output-file-path`

Sweep sites confirmed by audit:

- `internal/atdd/runtime/actions/bindings.go` (lines 181, 786, 806,
  877, 881) — comments + `ctx.State[...]` reads.
- Tests that exercise the per-dispatch JSONL output channel — search
  `internal/` for `"output_file_path"`.

One commit.

### 4. Cycle-phase / prev-phase params keys

Rename in lockstep — both are bound at call-activity sites in
`process-flow.yaml` and substituted into `test-disabler` /
`test-enabler` agent prompts:

- `cycle_phase` → `cycle-phase`
- `prev_phase` → `prev-phase`

Sweep sites confirmed by audit:

- `internal/atdd/runtime/statemachine/process-flow.yaml` (lines 787,
  831, 832, 854, 876, 1144, 1186) — `params:` keys + `${...}`
  placeholders.
- `internal/assets/runtime/agents/atdd/test-disabler.md` (line 19)
  — parameter doc line ("Named `cycle_phase`…").
- `internal/assets/runtime/agents/atdd/test-enabler.md` (line 18)
  — parameter doc line.

The agent-prompt doc lines need both the snake spelling replaced AND
the prose about "Named `cycle_phase` (not `phase`) because…"
updated to refer to `cycle-phase`.

One commit.

### 5. Phase-id params key

- `phase_id` → `phase-id`

Sweep sites confirmed by audit:

- `internal/atdd/runtime/actions/bindings.go` (lines 424, 434, 436)
  — comment + `ctx.Params[...]` read + error message.
- `internal/atdd/runtime/statemachine/process-flow.yaml` — search
  for `phase_id:` to find the call-activity sites that bind it; the
  audit didn't enumerate them but every call site that invokes
  `check_phase_scope` must pass it (per the error message at line
  436).

One commit.

## Method

1. **Per-item sweep:**
   1. `Grep` for the old snake_case string across `internal/` (string
      literal only — case-sensitive). The audit lines above are a
      starting point, but treat `Grep` as authoritative.
   2. For each hit, replace the string literal with the kebab form.
      Update comments, error messages, and test-fixture strings in
      the same pass so a follow-up `Grep` for the snake form returns
      zero hits.
   3. Leave Go identifier names (struct fields, vars, consts) in
      Go-idiomatic case — only the string value changes.

2. **Per-item verify:**
   - `go build ./...`
   - `go test ./internal/atdd/...` (per memory
     `feedback_go_test_windows`: never `go test ./...` on Windows
     without `-p 2`; this scoped invocation is safe).
   - For Item 4 (the only one that touches `process-flow.yaml` and
     agent prompts together): run the YAML-schema validator if one
     exists, plus an ATDD rehearsal in a sandbox repo to confirm
     `${cycle-phase}` substitution still fires correctly.

3. **Per-item commit:** small focused diff per memory
   `project_retry_consolidation_priority`. Commit message format:
   `runtime: rename <snake_case_key> → <kebab-case-key>`.

4. **Do NOT regenerate diagrams locally** — CI handles it (memory
   `feedback_plans_no_diagram_regen`).

## Open questions

None — all five items are pure rename/delete (autonomous per memory
`feedback_renames_autonomous_content_gated`); no body rewrites, no
new files, no schema changes.

## Out-of-band

Naming consistency is a code-style concern, not a behavioural one.
The runtime works fine with the mixed naming today. Schedule when
there's a quiet window; no urgency.

## References

- Parent audit plan `20260527-1210-agent-naming-sweep-singular-vs-plural`
  — closed with zero shape-rule hits and spawned this case-style
  follow-up. Plan file deleted on landing; lineage in git history.
- `internal/atdd/runtime/gates/bindings.go:252–256` — the canonical
  comment stating Phase D keys are all kebab-case.
- Memory `feedback_renames_autonomous_content_gated` — autonomous
  batch-commit for pure renames.
- Memory `feedback_never_edit_generated_diagrams` — leave
  `docs/process-diagram.md` and SVGs alone.
- Memory `project_retry_consolidation_priority` — one commit per
  rename, not a big-bang commit.
