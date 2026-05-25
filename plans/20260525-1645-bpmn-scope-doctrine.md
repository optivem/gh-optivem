# BPMN scope vocabulary doctrine + open questions

> **Parent plan:** `plans/20260525-1057-bpmn-refactor-design.md` (design archive).
> **Trigger:** review of `plans/ideas/2-bpmn-refactor-mid-level.md` (MID brainstorm) surfaced that the `**Scopes:**` blocks had been authored with invented kebab-prose names rather than the canonical Family B keys already configured in `internal/atdd/phase-scopes.yaml` and `internal/projectconfig/paths_defaults.go::CanonicalPathKeys()`. This plan captured the open questions raised while fixing that, grouping them under one execution surface so they can be resolved together before Phase C YAML encoding locks them in.

## Context

Two canonical scope-vocabulary surfaces already exist in the repo:

- **`internal/projectconfig/paths_defaults.go::CanonicalPathKeys()`** — Family B keys (snake_case): `driver_port`, `driver_adapter`, `external_system_driver_port`, `external_system_driver_adapter`, `at_test`, `dsl_port`, `dsl_core`, `ct_test`.
- **`internal/atdd/phase-scopes.yaml`** — SSoT mapping BPMN phase id → list of layer names (Family B keys above + `system_path` from Family A).

Memory `feedback_substitutable_paths_in_docs` already pins the rule: *"path-shaped scope uses Family B placeholders, never prose."*

Q-numbering continues from the parent design plan's Q-series. Q40–Q46 resolved during the 2026-05-25 `/refine-plan` walk and `/execute-plan` session (see git history); Q41 + Q43 + Q45 C landed as commits, Q42 / Q44 / Q46 were no-op confirmations. The remaining items are below.

---

## Open questions

### Q40 — Rename canonical scope keys from snake_case to kebab-case  *(DOCTRINE / NAMING)*

**✅ Decided: A** — Rename all surfaces snake→kebab in one pass before Phase C YAML lock. Family A `system_path` → `system-path` included. Per Q29 "kebab-case everywhere" doctrine; per `feedback_teaching_repo_no_legacy`, no migrate-relocation pass needed (teachers regenerate gh-optivem.yaml).

**Context.** Q29 in the parent plan locked kebab-case as the universal naming scheme ("kebab-case everywhere — YAML keys, doc headings, prompt filenames, in-prose references, anchor slugs, Go struct tags"). The canonical scope keys in `CanonicalPathKeys()` and `phase-scopes.yaml` are snake_case (`at_test`, `dsl_core`, …) — they predate Q29 and were not included in its sweep.

**Surfaces affected by the rename.**

| Surface | Change |
|---|---|
| `internal/projectconfig/paths_defaults.go::CanonicalPathKeys()` | rename 8 keys: `at_test` → `at-test`, `dsl_core` → `dsl-core`, etc. |
| `internal/projectconfig/paths_defaults.go::pathStems()` | iteration order tied to key order — no string change, but key-iterating tests flip. |
| `internal/projectconfig/paths_defaults.go::ExternalDriverKeyRenames` | existing old→new map (`external_driver_port` → `external_system_driver_port`); may absorb the snake→kebab pass OR live alongside as a second map. |
| `internal/atdd/phase-scopes.yaml` | rewrite all phase entries (22 phases × 2-3 keys). |
| `internal/projectconfig/config.go` | validator Rule 22a — error messages + canonical-key check. |
| `internal/projectconfig/path-keys.md` | doctrine doc rewrite. |
| `internal/projectconfig/config_test.go`, `paths_defaults_test.go`, `internal/atdd/phase_scopes_test.go` | fixture strings update. |
| `gh-optivem.yaml` in every real project's `paths:` block | keys all rename — per memory `feedback_teaching_repo_no_legacy`, teachers regenerate configs; no migrate-relocation pass needed. |
| `Family A: system_path` | also kebab → `system-path` (decided yes per option A; Q29 applies). |
| Prompt frontmatter `scope:` keys (in `internal/assets/runtime/prompts/atdd/*.md`) | if prompts hard-code Family B keys, those rename too. |
| The five brainstorm files `plans/ideas/*.md` (LOW/MID/HIGH/CYCLE/TOP) | MID Scopes block rewrites snake → kebab. LOW has no Scopes blocks. HIGH/CYCLE/TOP have invocations but no Scopes. |
| `process-flow.yaml` (Phase C target) | not yet authored; will be written kebab from the start. |

**Options.**
- **(A) Rename all surfaces snake → kebab in one pass before Phase C YAML lock.** Single doctrine moment. Mechanical find/replace once the rename map is fixed. Q29 fully realized. Family A `system_path` → `system-path` for consistency.
- **(B) Rename everything except `system_path`.** Family B kebab, Family A stays snake. Justification: Family A is structurally distinct (single key, not a key set), so Q29's "everywhere" might not extend to it. Asymmetric but defensible.
- **(C) Defer the rename indefinitely.** MID/HIGH/CYCLE/TOP brainstorm + Phase C YAML use snake_case canonical. Q29 stays unfulfilled for Family B. Justification: rename is mechanical at any point, no urgency, churn cost not justified by clarity gain.

**Recommendation: A.** Q29's "everywhere" is explicit. Family A is the same scope vocabulary at a different level, so it goes too. Doing it once before Phase C YAML lock prevents writing the YAML twice. Find/replace is bounded. Per memory `feedback_teaching_repo_no_legacy`, the gh-optivem.yaml-in-real-projects ripple is "regenerate, don't migrate" — fits the teaching repo's no-legacy doctrine.

---

## Resolution order

Only Q40 remains actionable.

1. **Q40** (snake → kebab rename) — bounded find/replace once the rename map is locked. Best done in a fresh `/clear`-ed session due to surface count (~6 file edits + test fixtures + prompt frontmatter + brainstorm Scopes blocks). Sequence after `plans/20260525-1659-bpmn-acceptance-test-rename.md` (Q-new-6) and `plans/20260525-1710-bpmn-name-consistency-audit.md` (Q45 C) — both leave Q40's surfaces unchanged but may rewrite adjacent text; doing Q40 last avoids re-baselining.

## Items

**Rename map** (snake → kebab):

| Old (snake) | New (kebab) | Family |
|---|---|---|
| `driver_port` | `driver-port` | B |
| `driver_adapter` | `driver-adapter` | B |
| `external_system_driver_port` | `external-system-driver-port` | B |
| `external_system_driver_adapter` | `external-system-driver-adapter` | B |
| `at_test` | `at-test` | B |
| `dsl_port` | `dsl-port` | B |
| `dsl_core` | `dsl-core` | B |
| `ct_test` | `ct-test` | B |
| `system_path` | `system-path` | A |

**Per-surface edit items** (execute in order):

1. **`internal/projectconfig/paths_defaults.go`** — rename the 8 Family B keys in `CanonicalPathKeys()` and the `system_path` constant in Family A. `pathStems()` has no string change but its iteration tests need fixture updates (handled in item 6). For `ExternalDriverKeyRenames`: leave the existing `external_driver_port` → `external_system_driver_port` map intact — it's a separate concern from Q40, and per `feedback_teaching_repo_no_legacy` no parallel snake→kebab migrate map is needed (teachers regenerate gh-optivem.yaml).

2. **`internal/atdd/phase-scopes.yaml`** — rewrite all phase entries (22 phases × 2-3 keys). Mechanical find/replace using the rename map.

3. **`internal/projectconfig/config.go`** — Rule 22a validator strings: error messages and canonical-key list reference. Mechanical find/replace.

4. **`internal/projectconfig/path-keys.md`** — doctrine doc text rewrite. In-prose key references all snake → kebab.

5. **Prompt frontmatter** — grep `internal/assets/runtime/prompts/atdd/*.md` for `scope:` blocks and any in-prose Family B references; rewrite. (May be zero hits if prompts use phase ids instead of layer keys directly — confirm during /execute-plan.)

6. **Test fixtures** — update string fixtures in:
   - `internal/projectconfig/config_test.go`
   - `internal/projectconfig/paths_defaults_test.go`
   - `internal/atdd/phase_scopes_test.go`

7. **MID brainstorm** — `plans/ideas/2-bpmn-refactor-mid-level.md` Scopes blocks rewrite snake → kebab. LOW/HIGH/CYCLE/TOP brainstorms have no Scopes blocks; skip.

8. **Full test sweep** — `scripts/test.sh` (or `go test -p 2 ./...`; per memory `feedback_go_test_windows` never run unbounded `go test ./...`). Verify nothing flips on the iteration-order shift in `pathStems()`.

## Cross-references

- Parent design plan: `plans/20260525-1057-bpmn-refactor-design.md`
- Phase C execution plan: `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md`
- Sibling rename plan: `plans/20260525-1659-bpmn-acceptance-test-rename.md` (Q-new-6 HIGH renames)
- Spawned by Q45 C: `plans/20260525-1710-bpmn-name-consistency-audit.md`
- Memory `feedback_substitutable_paths_in_docs` — path scopes use Family B placeholders, never prose.
- Memory `feedback_teaching_repo_no_legacy` — Q40 doesn't need a migrate-relocation pass; teachers regenerate.
