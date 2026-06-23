# 2026-06-23 09:24:18 UTC — Strip cross-language framing from runtime agent prompts

## TL;DR

**Why:** Rehearsal #61 ("Redesigning New Order UI") halted deterministically at `STOP_SCOPE_VIOLATION`. The `system-driver-adapter-updater` prompt instructs the agent to "Apply across all parallel implementations (Java/.NET/TS × monolith/multitier)", but its `write:` scope resolves to a single language by construction. The prompt mandates writing 3 languages while the scope grants 1 → guaranteed out-of-scope write → mandated scope-exception envelope → halt. The "parallel implementations" framing is shop-testbed leakage; in a concrete scaffolded project there is exactly one language.

**End result:** No runtime agent prompt instructs an agent to write across multiple language implementations, and no prompt enumerates "Java/.NET/TS" as if the student project were multi-language. A redesign/update ticket touching a UI/driver element can no longer manufacture an out-of-scope write that halts at `STOP_SCOPE_VIOLATION`.

## Outcomes

What we get out of this — the goals and deliverables:

- The three "updater/redesign" agents no longer tell the agent to write across parallel language implementations; each works within its resolved single-language scope (`system-driver-adapter-updater`, `external-system-driver-adapter-updater`, `system-updater`).
- The dead doc-link citations (`architecture/driver-adapter.md`, `architecture/system.md` — neither shipped under `runtime/`) are removed alongside the clauses that cited them.
- All "Java/.NET/TS" language enumerations are gone from agent prompt bodies and frontmatter comments; the genuine per-project axis (monolith/multitier) is preserved where it's a real choice.
- Migration-comment shop topology ("3 languages × 2 architectures … consumed by all of them") is reworded to describe a single migration set / single SUT, without changing the one-file-authoring behaviour.
- `acceptance-test-writer.md`'s three-runner enumeration (`mvn test` / `dotnet test` / `npx playwright test`) is reduced to single-language framing.
- BPMN, scope mechanism, and `gh optivem` commands are untouched — they behaved correctly; this is an agent-prompt-only fix.

## ▶ Next executable step (resume here)

Edit `internal/atdd/assets/runtime/agents/atdd/system-driver-adapter-updater.md`: remove the "Apply across all parallel implementations (Java/.NET/TS × monolith/multitier — see architecture/driver-adapter.md)" clause from Step 1 (line 25) so the agent works within its resolved single-language scope, fix the frontmatter comment (line 2), and drop the "Java/.NET/TS" enumeration from the `architecture` param description (line 16). This is the agent that crashed #61; fixing it first removes the reproducing failure. Then apply the same pattern to the other sites in Steps 2–5.

## Steps

- [ ] **Step 1 — Remove the multi-language WRITE directives + dead doc links** (the crash class). In each, delete the "Apply across all parallel implementations (Java/.NET/TS × monolith/multitier — see `<doc>`)" clause; the agent operates within its single-language scope:
  - `system-driver-adapter-updater.md:25` (Step 1) — dead link `architecture/driver-adapter.md`
  - `external-system-driver-adapter-updater.md:27` (Step 3) — dead link `architecture/driver-adapter.md`
  - `system-updater.md:30` (Step 1) — dead link `architecture/system.md` (keep the "grep the system tree for residual references" guidance that follows it)
- [ ] **Step 2 — Fix the frontmatter comments** that frame the work as cross-implementation:
  - `system-driver-adapter-updater.md:2` ("across parallel implementations … cross-implementation reasoning")
  - `external-system-driver-adapter-updater.md:2` ("per Checklist across implementations … cross-implementation reasoning")
- [ ] **Step 3 — Reword the migration comments** to drop the shop-topology rationale entirely. The "shared across every SUT (3 languages × 2 architectures); your one file is consumed by all of them" framing was true only in the multi-SUT testbed; in a concrete project there is one SUT, so the rationale evaporates. Reduce to "author a single timestamped file in the migration set" (behaviour unchanged — still one file):
  - `system-implementer.md:30`
  - `system-updater.md:31`
- [ ] **Step 4 — Drop the language enumeration from the `architecture` param descriptions** (keep monolith/multitier — that is a per-project choice). 6 sites:
  - `system-driver-adapter-updater.md:16`, `external-system-driver-adapter-updater.md:16`, `system-updater.md:16`, `system-implementer.md:20`, `system-refactorer.md:18`, `test-refactorer.md:18`
- [ ] **Step 5 — Reduce the three-runner enumeration to single-language framing** in `acceptance-test-writer.md:24`. Replace "local `mvn test` / `dotnet test` / `npx playwright test`" with generic phrasing ("a local test run" / "your local test runner") rather than a per-language placeholder, unless a per-language test-command placeholder already exists — keep the sentence's meaning ("the WIP gate is left unset there, so the test is silently skipped").
- [ ] **Step 6 — Verify.** Run the render-matrix test (`TestRenderMatrix_NoUnfilledPlaceholders`) and the prompt-render path to confirm nothing broke. Re-grep `internal/atdd/assets/runtime/agents/atdd/` for `Java|\.NET|dotnet|C#|TypeScript|parallel implementation` and confirm only legitimate single-language content remains. (No regression-guard test is planned — the prose removal is the fix; an executor may add one at their discretion if desired.)

## Explicitly out of scope (do NOT touch)

- Per-language shared chunks: `internal/atdd/assets/runtime/shared/{wip-gate-java,wip-gate-typescript,isolated-marker-java,isolated-marker-csharp,isolated-marker-typescript}.md` — these *are* the resolved language's variant, selected at render time.
- "parallel runs would be flaky" / `@Isolated` — test-execution parallelism, unrelated meaning.
- "shared across every channel" in `system-implementer.md` — channels, not languages.
- No BPMN (`process-flow.yaml`) change. No `gh optivem` command change.

## Decisions settled

- **Param descriptions (Step 4):** keep `(monolith/multitier)` — it is a genuine per-project axis, not cross-language. Only the "Java/.NET/TS" enumeration is dropped.
- **Migration comments (Step 3):** drop the shop-topology rationale entirely; reduce to "author a single timestamped file in the migration set."
- **Step 5 wording:** use generic phrasing, not a per-language placeholder (unless one already exists).
- **Regression-guard test:** not planned — prose removal is the fix; executor may add one at discretion.
