# 2026-07-10 20:21:00 UTC — Fix generated Java backend DSL leaving an unused `import java.util.Map;` that fails the ATDD quality gate

## TL;DR

**Why:** The multitier/multirepo/java smoke rehearsal (Actions run 29067245795) crashes at the terminal quality gate: an ATDD agent authors `BackendDsl.java` with an unused `import java.util.Map;`, Checkstyle treats `UnusedImports` as a hard error, `run-sonar.sh` exits 1, and the whole run fails after every test is already green. No author agent gets feedback because `javac` (the only in-loop compile gate) tolerates unused imports — Checkstyle runs only at the end.
**End result:** Authoring agents are instructed to leave no unused imports/usings, so generated code passes the scaffold's Checkstyle/analyzer quality gate on the first try. The rule lives once in the shared preamble (covering every author and all three languages) and is reinforced in the confirmed offender, `dsl-implementer.md`.

## Outcomes

What we get out of this:

- Every authoring agent knows unused imports/usings are a **hard failure** at the quality gate, not a cosmetic nit — one language-agnostic rule in `shared/preamble.md` covers Java (Checkstyle `UnusedImports`), TypeScript (`noUnusedLocals`/ESLint), and .NET (unused-using analyzers).
- `dsl-implementer.md` carries a targeted reminder to strip imports left dangling when a DSL method that would have used them isn't emitted for the current scenario (the exact `java.util.Map` case).
- The multitier/multirepo/java rehearsal (run 29067245795) reaches a clean sonar/Checkstyle pass instead of exit 1.
- The scaffold's Checkstyle policy is **left untouched** — treating unused imports as errors is intentional (teaching clean code); the fix is in the authoring layer, not by relaxing the gate.

## ▶ Next executable step (resume here)

Edit `internal/atdd/assets/runtime/shared/preamble.md`: in the `## Compile your slice; don't invent API surface` section, append an import-hygiene rule — final output must contain no unused imports/usings, because the scaffold's quality gate (Checkstyle `UnusedImports` for Java; `noUnusedLocals`/ESLint for TypeScript; unused-`using` analyzers for .NET) treats them as **hard errors that fail the final scan**, and the in-loop compile gate (`javac`/`tsc`/`dotnet build`) does not catch them; after the final edit, remove any import the code no longer references. Then make the Step 2 one-line reinforcement in `dsl-implementer.md`. Both are prompt-body edits under `internal/atdd/assets/runtime/` — no code, no diagram regen. Unblocks the operator rehearsal re-run in `## Verification`.

## Steps

- [ ] Step 1: Add the import-hygiene rule to `internal/atdd/assets/runtime/shared/preamble.md`, inside the `## Compile your slice; don't invent API surface` section. State it language-agnostically: emit no unused imports/usings; the scaffold's quality gate treats them as hard errors (name Checkstyle `UnusedImports` for Java, `noUnusedLocals`/ESLint for TypeScript, unused-`using` analyzers for .NET) that fail the terminal scan even when every test is green; the in-loop compile gate does not catch them, so after your final edit remove any import your code no longer references.
- [ ] Step 2: Add a one-line reinforcement to `internal/atdd/assets/runtime/agents/atdd/dsl-implementer.md` — when a DSL method that would have used an import (e.g. a `Map`-based builder) isn't emitted for this scenario, delete the now-unused import it left behind.

## Alternative (follow-up option, not this plan's primary fix)

If the prompt-rule fix proves insufficient (unused imports recur — LLM hygiene rules are probabilistic), escalate to a **deterministic lint gate**: run the linter (Checkstyle for Java, and the TS/.NET equivalents) inside the `gh optivem test compile` / compile step so unused imports fail loudly **during** the ATDD loop and route to a fixer agent, instead of surfacing only at the terminal sonar gate. This is more robust but a larger change to the compile command and process flow — deferred unless the prompt rule doesn't hold.

## Verification

Operator-driven (not agent work):

- Re-run the multitier/multirepo/java ATDD rehearsal — the smoke matrix cell that failed in run 29067245795 (`TestValidMultitierConfigurations/multitier_multirepo_java_ts_typescript`) — and confirm the sonar/Checkstyle gate passes and the run exits 0.
- No static unit test is added: the offending file is generated at runtime, so the rehearsal re-run is the proof.
