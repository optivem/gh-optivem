# 2026-06-20 07:58:10 UTC — Preamble guard: don't invent API surface on a primitive

## TL;DR

**Why:** Rehearsal #65 (view-product-list) burned ~18 min in a fix-loop stall and halted at `FIX_LOOP_NO_PROGRESS_EXHAUSTED` because the `external-system-driver-adapter-implementer` agent called `Result.flatMap(...)` (`ErpStubDriver.java:46`) — a combinator the testkit `Result<T,E>` primitive (`Result.java:48–67`) does not have (only `map`/`mapError`/`mapVoid`). It invented a conventional-looking monadic method that doesn't exist → `cannot find symbol method flatMap` → contract-real compile failure.
**End result:** Two coordinated changes. (1) **All** non-fixer writer agents become **self-correcting**: before exiting, a writer compiles its own slice and fixes its own compile errors in-pass. This is a pure compile check — compilation is **not** running the test suite, so it never touches the intended runtime RED (in this model every writer's output is expected to compile — there is a `compile-tests` gate after each writer; the red is a runtime assertion failure at `run-tests`, never a compile error). A compile error is always a defect for any writer, so this is safe in the shared `preamble.md` for all writers. (2) A new `testkit-common` Family B scope key is minted so any writing agent that needs to evolve the shared `testkit/common` primitives (`Result`, `Converter`, `Closer`, `ResultAssert`) **can** — read broadly, write where extension is plausible — with no human-approval gate. Together: for #65 the agent either sequences with the existing `Result` API or, if it genuinely wants `flatMap`, adds it to `Result` (now in scope) and proves it by compiling — instead of leaving the broken call for the fix loop to choke on.

**No hardcoded paths.** The key *name* `testkit-common` is gh-optivem-owned (`CanonicalPathKeys()`); its per-language path tail lives only in `pathStems()` (one place, rule-constructed — `path.Join(main, "common")` / `src/testkit/common`, reproducing the shop template); the resolved value per repo is config in `gh-optivem.yaml`. `process-flow.yaml` and the prompt reference the **key**, never a literal path.

**Current scope facts (verified):** the driver-adapter implementer already has `write:` to `system-driver-adapter-shared` (`testkit/driver/adapter/shared/**`; `process-flow.yaml:2127–2128`) but **not** to `testkit/common/` — that path is not yet a scope key (`path-keys.md:40` marks the `testkit` parent non-scope-eligible; no `common` key exists). So `Result` is unwritable by any agent today; the agent invented `flatMap` inline precisely because it could not edit the primitive. This plan mints the missing key.

## Outcomes

What we get out of this:

- **Self-compile-and-fix in the writer contract** (shared `preamble.md`, all non-fixer writers): a writer compiles its own slice and fixes its own compile errors before exiting, so an invented/uncompiling call can never reach the downstream fix loop. A pure compile check — never runs the test suite, so the intended runtime RED is untouched. Would have caught #65 in-pass.
- A carve-out in `preamble.md`: compile yes, run-tests no — so self-compile doesn't contradict the existing "never run test commands yourself" rule (compilation ≠ running tests; the runtime red stays downstream-owned).
- A terse guard in `preamble.md`: prefer the methods that exist on a type; never fake a call to a method that isn't there; confirm by compiling. Reaches **all** non-fixer agents in one edit (`internal/atdd/runtime/agents/embed.go`).
- **A new `testkit-common` Family B scope key** so any writing agent that needs to evolve `testkit/common` primitives can — added to `CanonicalPathKeys()` + `pathStems()` (rule-constructed, not hardcoded) and to the `read:`/`write:` lists of the writing nodes that need it.
- Fixer prompts (`fixer-preamble.md`) intentionally untouched — the recovery layer was deselected; this is a source-side-only change.

## ▶ Next executable step (resume here)

Direction is settled (option **b**); self-compile placement is resolved (shared `preamble.md`, all non-fixer writers — compile yes, run-tests no). Two sub-questions remain for `/refine-plan`: (ii) the exact per-node `read:`/`write:` assignment of `testkit-common` across the writing-agent MID nodes, and (iii) which compile command a writer may invoke. Once settled, the first edit is minting the key: add `"testkit-common"` to `CanonicalPathKeys()` (`paths_defaults.go:160`) and its rule-constructed stem to `pathStems()` (`paths_defaults.go:185`) in the matching position for all three languages (.NET stem pinned against the .NET shop template, not guessed). Keep prompt/process edits keyed on `testkit-common`, never literal paths. Do **not** touch `fixer-preamble.md`. Then run the projectconfig + prompt-render/embed tests.

## Steps

**Mint the `testkit-common` scope key:**
- [ ] Step 1: Add `"testkit-common"` to `CanonicalPathKeys()` (`internal/kernel/projectconfig/paths_defaults.go:160`) in fixed order.
- [ ] Step 2: Add the matching per-language stem to `pathStems()` (`paths_defaults.go:185`) in the same position — Java `path.Join(main, "common")`, TS `src/testkit/common`, .NET pinned against the .NET shop template (do not guess `Common` — verify on disk per the deterministic-paths rule). Rule-constructed, single source; no literal path elsewhere.
- [ ] Step 3: Update the vocabulary doc `internal/kernel/projectconfig/path-keys.md` (the Family B table) to list `testkit-common` and note it names the shared common primitives.
- [ ] Step 4: Add `testkit-common` to the `read:`/`write:` lists of the writing-agent MID nodes in `process-flow.yaml` — read broadly (every writer consumes the primitives), write on every node where extending a primitive is plausible. Enumerate the nodes during refinement (Open question ii); declare both lists explicitly per node.

**Self-compile-and-fix + anti-invention guard:**
- [ ] Step 5: In `preamble.md` (all non-fixer writers), reword the "never run test commands" rule (preamble.md:18–22) into "never **run** the test suite, but **do compile** your slice" — compile yes, run-tests no. Confirm which compile command an agent may actually invoke (Open question iii).
- [ ] Step 6: Add the self-compile-and-fix instruction to `preamble.md`: before exiting, compile the in-scope slice and fix own compile errors in-pass (a compile error is always a defect; this never runs tests, so the intended runtime red is untouched).
- [ ] Step 7: Add the terse anti-invention line: prefer methods that exist on a type; never call a method that isn't there; if a shared primitive genuinely lacks it, add it (now in scope) and compile to confirm. Keep terse — no restating existing rules.

**Verify:**
- [ ] Step 8: Run the projectconfig tests (`internal/kernel/projectconfig` — `paths_defaults_test.go`, `config_test.go`) to confirm the new key resolves per language, and the prompt-render/embed + scope-block tests (`internal/atdd/runtime/agents` + `internal/atdd/process/clauderun`) to confirm scope blocks resolve and the preamble renders into every agent with no unfilled placeholders. Use `-p 2` / scoped package per the Windows go-test rule.

## Verification

- Fixers are intentionally out of scope: `fixer-preamble.md` and the `unexpected-failing-tests-fixer` recovery layer are NOT edited (the recovery layer was the deselected option in the postmortem).
- The guard reaches the source agent that triggered #65 (`external-system-driver-adapter-implementer`) transitively via the prepended preamble — no per-agent edit needed.
- (Operator) Re-run the #65 rehearsal slice to confirm the driver-adapter implementer now sequences the two `returnsProduct` stub calls with the real `Result` API instead of inventing `flatMap`.

## Open questions

- **RESOLVED — primary fork:** option **(b)** — self-compile + mint `testkit-common`.
- **RESOLVED — red-green is not a problem:** self-compile is a *compile* check, not a test run. In this model every writer's output is expected to compile (a `compile-tests` gate follows each writer; the intended RED is a runtime assertion failure at `run-tests`, never a compile error). A compile error is always a defect for any writer, so self-compile applies to **all** non-fixer writers and lives in the shared `preamble.md`. The only carve-out: compile yes, run-tests no.
- **(ii) Per-node `testkit-common` scope:** enumerate the writing-agent MID nodes in `process-flow.yaml`; add `testkit-common` to `read:` for every writer (all consume the primitives) and to `write:` for every node where extending a primitive is plausible. Resolve the exact per-node lists during refinement (declare both lists explicitly per node).
- **(iii) Compile affordance:** confirm which compile command a `prod-agent` may invoke in its sandbox (the orchestration uses `gh optivem test compile` / Gradle as separate steps) before wording Step 6.
