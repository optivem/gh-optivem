# Kebab-case everywhere in `gh-optivem.yaml`

> **Cross-reference:** This plan **fully supersedes** the narrow scope locked
> in by memory rule `feedback_kebab_snake_scope_in_process_flow.md`
> ("kebab applies only to BPMN-added keys; existing snake_case runtime-binding
> keys stay snake_case"). Scope is now uniform across the repo: **all** keys
> in **both** `gh-optivem.yaml` (project config) **and** `process-flow.yaml`
> (statemachine runtime bindings) become kebab-case. The earlier memory rule
> is retired and must be deleted as part of execution (see Q2 resolution).

## Origin / intent

User request (2026-05-25 23:11):

> "can you also add a separate plan that I want kebab case everywhere
>
> ```
> system_test:
>     path: system-test/typescript
>     config: system-test/typescript/tests.yaml
>     repo: optivem/shop
>     lang: typescript
>     sonar_project: optivem_shop-tests-typescript
> ```
> "

Read as: the top-level keys (`system_test`, `external_systems`, `repo_strategy`,
`sonar_project`, …) should follow the same kebab-case convention as the Family B
`paths:` sub-map (`driver-port`, `at-test`, …) that was just realigned in the
12 shop config files.

## Why this is a doctrine flip

Today the project-config schema is **mixed**:

- Top-level keys are snake_case: `repo_strategy`, `system_test`, `external_systems`, `sonar_project`.
- The `paths:` sub-map is kebab-case (canonical Family B vocabulary — `driver-port`, `at-test`, …).
- Enum values are already kebab: `mono-repo`, `multi-repo`.

That split mirrors a real distinction in the codebase: the **kebab side**
is operator-edited substitution vocabulary (read by phase-doc substitution and
ATDD docs); the **snake side** is Go-struct-bound configuration (read by
`projectconfig.Config` via `yaml:"..."` tags). The existing memory rule
codifies that distinction.

Moving the snake side to kebab erases the distinction in the config file
itself — the operator now sees one uniform kebab namespace — but the
**Go struct stays Go-idiomatic snake_case in code**; only the `yaml:"..."`
tags flip. That's the change.

## Surface area (everything that touches the key names)

### 1. Schema — `internal/projectconfig/config.go`

The yaml struct tags. Concretely (current → proposed):

| Field                  | Current tag             | Proposed tag             |
|------------------------|-------------------------|--------------------------|
| `RepoStrategy`         | `repo_strategy`         | `repo-strategy`          |
| `SystemTest`           | `system_test`           | `system-test`            |
| `ExternalSystems`      | `external_systems`      | `external-systems`       |
| `TierSpec.SonarProject`| `sonar_project`         | `sonar-project`          |
| (any others in struct) | …                       | …                        |

Go field names stay PascalCase / snake-internal as the language demands.

### 2. Validation / migrate / init code paths

- `projectconfig.Validate` — any error strings that quote the old key names (`system_test.paths.driver-port`, etc.) flip to `system-test.paths.driver-port`.
- `projectconfig.Migrate` (`config migrate` command) — if it has a hand-rolled key-rewrite map, add the snake→kebab pairs there; otherwise the yaml round-trip already covers it.
- `BuildOptivemYAML` (`internal/steps/optivem_yaml.go`) — emission stays correct via the new struct tags, but the SSoT comment about "scaffold-authoritative" needs an updated example snippet.
- `Config.Preflight` — error/log strings that name keys.

### 3. Test fixtures

- `internal/projectconfig/config_test.go` — every inline YAML literal (~30+ blocks based on grep at session start) has `system_test:` / `sonar_project:` / `repo_strategy:` / `external_systems:`. Bulk rename.
- `internal/projectconfig/paths_defaults_test.go` — likely none of these top-level keys, but verify.
- `internal/steps/*_test.go` — any embedded YAML or expected-output strings.
- Golden files under `testdata/` — same.

### 4. Generated / templated configs

- `gh optivem init` template (lives in `internal/steps/optivem_yaml.go` / `internal/projectconfig`) — emits keys via struct tags, so the change is automatic, but **visible diff in scaffolded repos**. Anyone re-running `init` after this change will get the new shape.
- `shop` template if there's an in-tree fixture under `internal/...` (search for `system_test:` in `internal/`).

### 5. Documentation

- `CLAUDE.md` — currently references `system_test.paths:` doctrine; update to `system-test.paths:` (or pick one canonical spelling and stick to it everywhere — see Q3 below).
- `internal/projectconfig/path-keys.md` — vocabulary doc; cross-references.
- `docs/atdd/**/*.md` — any prose that names config keys.
- `README.md` — example snippets.
- Phase-doc substitution docs — any example `${...}` placeholder using the old names.

### 6. External repos in the workspace (NOT under gh-optivem)

- `C:\GitHub\optivem\academy\shop\gh-optivem-*.yaml` × 12 — the files just fixed need a **second pass** (snake top-level keys → kebab).
- Any other repo in `C:\GitHub\optivem\academy\` that has a `gh-optivem.yaml`. Survey before executing.

## Mechanical work breakdown

Order matters: schema → code → tests → fixtures → external configs → docs.
If the schema changes flip first and the tests don't follow in the same
commit, everything goes red.

4. **Flip `process-flow.yaml` keys + statemachine-runtime yaml struct tags** (per Q2 expanded scope). Audit gate fixtures for loopback edges before running tests (memory: `feedback_statemachine_test_loop_hazard`).
5. **Bulk-rename embedded YAML in statemachine tests + gate fixtures.** Run statemachine tests with memory ceiling watch.
6. **Update CLAUDE.md + `path-keys.md` + scaffolder examples + any phase-doc / ATDD docs naming the old keys.**
7. **Delete memory rule `feedback_kebab_snake_scope_in_process_flow.md`** and its `MEMORY.md` index entry (per Q2 resolution). This is the **only** memory-store edit in this plan.
8. **Survey external workspace configs.** Apply the same snake→kebab rename to every `gh-optivem*.yaml` in `C:\GitHub\optivem\academy\*\` (starting with the 12 shop configs just realigned).
9. **Re-run rehearsal** (`bash scripts/atdd-rehearsal.sh <issue> --config <one-of-the-shop-configs>`) to confirm end-to-end.

Each step is one commit. **No commits without per-step approval** (memory rule
`feedback_no_commit_without_approval`).

## Open questions — resolve before executing

Per memory rule `feedback_resolve_questions_upfront`, resolve these now,
not during execution.

> **Overarching doctrine (user, 2026-05-25):** "consistency everywhere, the
> modern way." Kebab-case is the project-wide YAML/identifier convention.
> The only carve-outs are:
>
> 1. **Language-forced** snake/Pascal — Go struct field names (`SystemTest`,
>    `SonarProject`) stay Go-idiomatic; only `yaml:"..."` tags flip.
> 2. **Externally-owned values** — content inside YAML values that belong to
>    another system (e.g. the SonarCloud project key
>    `optivem_shop-monolith-typescript`) is not ours to rename.
>
> Everything else flips. Q3/Q4/Q5 below resolve against this doctrine.

**Q1. Migrate vs reject for old-shape configs? — RESOLVED**

**Decision:** Hard-reject at parse, **no** snake→kebab rewrite in `gh optivem
config migrate`. Operators with an old snake-form config regenerate via
`gh optivem init` (or hand-edit). No dual-shape acceptance, no legacy-alias
machinery, no migrate-relocation pass.

Rationale: consistent with `feedback_teaching_repo_no_legacy` ("teaching repo —
drop legacy-alias machinery for schema moves; teachers regenerate configs")
and with the spirit of the original Q1(a) recommendation. Adding the rewrite
to `migrate` would re-introduce exactly the "legacy alias machinery" that
rule forbids.

Implementation note for executors: do not add a `migrate` codepath for this
flip. If a snake-form config is detected, the parser's existing
"unknown field" error is the user-facing signal.

**Q2. Does this also flip `process-flow.yaml`? — RESOLVED**

**Decision:** **Expand scope.** `process-flow.yaml` flips too. All YAML in
the repo becomes kebab-case. The previously narrow scope from memory rule
`feedback_kebab_snake_scope_in_process_flow` is now superseded; that memory
rule should be **deleted** as part of this plan (see item 7 below).

Affected keys in `process-flow.yaml` (non-exhaustive — executors must
re-grep before flipping):

- `phase_id` → `phase-id`
- `compile_action` → `compile-action`
- `change_type` → `change-type`
- any other snake_case runtime-binding keys

Affected code:

- Statemachine runtime parser (`internal/atdd/runtime/...`) — yaml struct tags flip.
- Statemachine **gate fixtures** and any inline YAML in tests — bulk rename.
- BPMN substitution layer if it names process-flow keys in error strings.

**Hazard:** memory rule `feedback_statemachine_test_loop_hazard` warns that
new loopback edges in `process-flow.yaml` can deadlock statemachine tests
and consume 20GB+ RAM. A pure key-rename is structurally safe (no new
edges), but executors must still audit gate fixtures before running and
kill on memory climb.

**Memory-rule deletion:** the file
`C:\Users\valen_4rjvn9e\.claude\projects\C--GitHub-optivem-academy-gh-optivem\memory\feedback_kebab_snake_scope_in_process_flow.md`
and its `MEMORY.md` index entry must be removed at the end of execution.

**Q3. Canonical spelling in prose: hyphen-only, or both? — RESOLVED**

**Decision:** Hyphen-only in prose (kebab). Strategy: **global find/replace,
then audit Go-identifier mentions.**

Executor procedure:

1. Global sweep across docs (`CLAUDE.md`, `internal/projectconfig/path-keys.md`,
   `README.md`, `docs/**/*.md`, plan files in `plans/`):
   - `system_test` → `system-test`
   - `sonar_project` → `sonar-project`
   - `repo_strategy` → `repo-strategy`
   - `external_systems` → `external-systems`
2. After the sweep, grep for any residual snake forms and classify each match:
   - Naming a Go struct field (`SystemTest`, `TierSpec.SonarProject`, etc.) — keep.
   - Inside a URL, external-system name, or SonarCloud project key value — keep.
   - Anything else — flip.

Per the overarching doctrine, no "both spellings" tolerance in prose.

**Q4. `sonar_project` value format is unaffected, right? — RESOLVED**

**Decision:** Confirmed — **key flips, value stays.**

- `sonar_project: optivem_shop-monolith-typescript`
- → `sonar-project: optivem_shop-monolith-typescript`

The value is the SonarCloud project key (owned by SonarCloud / Sonar admin
on their side), not ours to rename. Matches the externally-owned-value
carve-out in the overarching doctrine.

Renaming the SonarCloud projects themselves is **out of scope** for this
plan. If desired later, that's a separate plan file with its own admin
coordination.

**Q5. Existing snake-named placeholders in phase-doc substitution? — RESOLVED**

**Decision:** **Flip them too.** Family A placeholder identifiers move to
kebab, in line with the overarching doctrine and with Family B (which is
already kebab — `${driver-port}`, `${at-test}`).

Concretely:

- `${system_test_path}` → `${system-test-path}`
- `${sut_namespace}` → `${sut-namespace}`
- (re-grep `config.go:266` reserved-keys list for the full set before flipping)

This **overrides** the plan's earlier default recommendation to leave them
snake. Rationale: placeholder identifiers are user-visible — operators type
them in phase docs — so the "user-facing surface is kebab" doctrine applies
fully.

Blast radius (additional work beyond items 1–9 above):

- **Reserved-keys list** in `internal/projectconfig/config.go` (≈ line 266).
- **Substitution engine** wherever it pattern-matches placeholder names.
- **Every phase doc** under `docs/atdd/**/*.md` (and equivalents) that uses these placeholders.
- **Test fixtures** referencing the placeholders by name.
- **`path-keys.md`** vocabulary doc.

These additions belong in items 1 (schema/reserved keys), 5 (statemachine
tests if they cite Family A) and 6 (docs) of the work breakdown — the
executor should expand each item's scope to include the placeholder rename
rather than introducing a separate step.

## What NOT to do

- Do **not** introduce a dual-shape acceptance pass — Q1 resolved to hard-reject (memory: `feedback_teaching_repo_no_legacy`).
- Do **not** add a snake→kebab rewrite to `gh optivem config migrate` — Q1 resolved against that.
- Do **not** rename Go struct fields — they stay `SystemTest`, `SonarProject`, etc. Only `yaml:"..."` tags flip.
- Do **not** rename `${system_test_path}` / `${sut_namespace}` placeholder identifiers without Q5 explicitly authorising it.
- Do **not** commit any step without per-step approval (memory: `feedback_no_commit_without_approval`).

## Acceptance / verification

- `go test ./internal/projectconfig/... -p 2 ./internal/steps/... -p 2` passes after step 3.
- Full `scripts/test.sh` (or `go test -p 2 ./...`) passes after step 4.
- `gh optivem config validate --config gh-optivem-monolith-typescript.yaml` (and all 11 other shop configs) reports "is valid" after step 5.
- End-to-end rehearsal `bash scripts/atdd-rehearsal.sh <issue> --config <one-of-the-shop-configs>` runs at least past the parse stage on a fresh worktree.

## Items deliberately deferred

- Renaming any other repos' `gh-optivem.yaml` outside `C:\GitHub\optivem\academy\` — out of session scope.
- (`process-flow.yaml` is no longer deferred — it is now in scope per Q2 resolution.)
