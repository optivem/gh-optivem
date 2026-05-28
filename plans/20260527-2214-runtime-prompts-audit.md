# 20260527-2214 — ATDD Runtime Prompts Audit Plan

All per-agent body edits, shared-chunk edits, and needs-decision items resolved in this commit. The four findings below are out-of-scope for this plan and need routing to specialized agents.

Spinoff: [20260528-0959-interactive-mode-operator-suffix.md](./20260528-0959-interactive-mode-operator-suffix.md) — new operator-facing terminal hint for interactive dispatches.

## Out-of-scope findings (route elsewhere)

### 1. `external-system-driver-adapter-updater.md` parameter list omits `checklist`, but Step 1 references it

**Where:** `internal/assets/runtime/agents/atdd/external-system-driver-adapter-updater.md:10-22` (the `## Inputs` section has `### Checklist` with `${checklist}` substitution, but no corresponding `### Parameters` entry like `architecture` has on line 8).

**Issue:** Inconsistent input documentation across the four "updater" agents (`system-updater.md`, `external-system-driver-adapter-updater.md`, `system-driver-adapter-updater.md`, `system-refactorer.md`, `test-refactorer.md`) — some declare `architecture` and `checklist` as parameters in a `### Parameters` block, others jump straight to the `### Checklist` substitution. Not a logical bug; just inconsistent shape. The Checklist parameter is declared on the MID in `process-flow.yaml`.

**Suggested owner:** `architecture-sync` — confirm the MID parameter declarations are the SSoT, then either align prompts to that or `process-audit` for the documentation contract.

### 2. `system-implementer.md:22` prose names a subset of its read-scope

**Where:** `internal/assets/runtime/agents/atdd/system-implementer.md:22` says "trace through the DSL, the driver port, and the driver adapter" but the scope (`process-flow.yaml:1442`) reads `at-test, ct-test, dsl-port, dsl-core, driver-port, driver-adapter, external-system-driver-port, external-system-driver-adapter, system-path`.

**Issue:** The prose drops dsl-core, external-system-driver-port, external-system-driver-adapter, at-test, ct-test. For some tickets the agent will need to read the external-system layer too; the prose under-sells what the agent is allowed to do. This is not a token-density bug; it's a content drift between the prose and the scope block.

**Suggested owner:** `architecture-sync` or `process-audit` — confirm whether the prose is intentionally narrowing the contract (operator-design choice) or drifting from the MID-declared scope.

### 3. `acceptance-test-writer.md:21` and `contract-test-writer.md:19` carry the same load-bearing dsl-core asymmetry rule

**Where:**
- `internal/assets/runtime/agents/atdd/acceptance-test-writer.md:21`
- `internal/assets/runtime/agents/atdd/contract-test-writer.md:19`

**Issue:** Both bodies carry a sentence: *"The asymmetric scope (dsl-core is writeable but not in `read:`) is deliberate: reading impl context would leak it into test design."* This rule is invisible in the dispatcher's `${scope-block}` rendering (the block just lists read paths and write paths — the user has to spot the difference). The rule is content; it belongs either (a) as an inline explanatory comment in `process-flow.yaml`'s `write-acceptance-tests` MID (line 1346) or (b) as a one-line annotation in `${scope-block}`. Today it's restated in two writer prompts.

**Suggested owner:** `architecture-sync` — confirm whether the asymmetry rationale can be moved to the MID (or to a `${scope-block}` annotation) so it stops being paid per dispatch.

### 4. Duplication of the "If your previous WRITE didn't compile, fix the broken/missing piece" instruction across implementers

**Where:**
- `internal/assets/runtime/agents/atdd/external-system-driver-adapter-implementer.md:22`
- `internal/assets/runtime/agents/atdd/system-driver-adapter-implementer.md:22`
- `internal/assets/runtime/agents/atdd/dsl-implementer.md:62` (under `## Additional Notes`)

**Issue:** Three implementers carry the same re-entry instruction in slightly different prose. This is *content* duplication (not just style) — if the policy changes (e.g. "re-runs always start fresh") all three need to update in lockstep.

**Suggested owner:** `process-audit` or `token-usage-audit` — the policy may belong in a shared chunk or a `${re-entry-policy}` substitution.
