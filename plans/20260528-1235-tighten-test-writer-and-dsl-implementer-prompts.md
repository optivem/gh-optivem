# Plan: Tighten `acceptance-test-writer`, `contract-test-writer`, and `dsl-implementer` agent prompts

## Context

A walkthrough of the three writer/implementer agent prompts surfaced small editorial issues:

- `acceptance-test-writer.md` and `contract-test-writer.md` share two near-identical prose blocks (Step 2 and the `*-port-changed` Notes bullet). Each block has a fixable wording problem.
- `dsl-implementer.md` has a longer-than-necessary intro, a jargon-y "no substituted input" note, and the same explanation written twice (Step 3 + the `*-port-changed` Notes bullet).

None of these edits change agent behaviour. They are pure prose tightening of static prompt assets under `internal/assets/runtime/agents/atdd/`. The same files are embedded via `internal/assets/runtime/agents/embed.go` and concatenated with `shared/preamble.md` + `shared/scope.md` at dispatch time, so the only runtime surface is what the LLM reads.

Out of scope: any edit to other writer/implementer agents that don't carry the duplicated text, and any change to the `${expected-outputs}` template or the gate bindings these prompts target.

## Items

1. **`acceptance-test-writer.md` + `contract-test-writer.md`: label the DSL Core path.**
   - File: `internal/assets/runtime/agents/atdd/acceptance-test-writer.md` (Step 2, currently line 21) and `internal/assets/runtime/agents/atdd/contract-test-writer.md` (Step 2, currently line 19).
   - Change `to the impl class in (`${dsl-core}`)` to `to the impl class in DSL Core (`${dsl-core}`)`.
   - Rationale: the same sentence already uses `DSL Port (`${dsl-port}`)` two clauses earlier. The labelled form is the established pattern; the unlabelled `(${dsl-core})` reads as a bare path with no antecedent noun.

2. **`acceptance-test-writer.md` + `contract-test-writer.md`: split the overloaded `*-port-changed` Notes bullet.**
   - File: `internal/assets/runtime/agents/atdd/acceptance-test-writer.md` (Notes, currently line 30) and `internal/assets/runtime/agents/atdd/contract-test-writer.md` (Notes, currently line 28).
   - Replace the single bullet:

     > For `*-port-changed` flags, list every file you wrote and set the flag `true` if any file sits under the flag's port directory (interface, DTO, enum — anything). The dispatcher's `validate-outputs-and-scopes` re-derives directory keying from `${changed-files}`, so an incorrect value mis-routes the cycle. For new methods you add to `${dsl-port}` you must also write a `"TODO: DSL"` stub in the DSL Core per Step 2; DTO/enum changes don't require a stub.

     with two bullets:

     - "For `*-port-changed` flags, list every file you wrote and set the flag `true` if any file sits under the flag's port directory (interface, DTO, enum — anything). The dispatcher's `validate-outputs-and-scopes` re-derives directory keying from `${changed-files}`, so an incorrect value mis-routes the cycle."
     - "Step 2's `TODO: DSL` stub applies to new *methods* on `${dsl-port}` only — DTO/enum changes don't require a stub."

   - Rationale: the original bullet bundled two rules with opposite triggers (the flag is directory-keyed and fires on any file under the port path; the stub is method-only). A reader scanning the first sentence could conclude "DTO change → flag=true → must add a stub," which is wrong. Splitting decouples the rules.

3. **`dsl-implementer.md`: trim the intro paragraph.**
   - File: `internal/assets/runtime/agents/atdd/dsl-implementer.md` (currently line 8).
   - Replace:

     > The implement-dsl task replaces every `TODO: DSL` prototype in the DSL Core (`${dsl-core}`) with real logic and, when the DSL surface needs new behaviour, extends the driver port (`${driver-port}`) or external-system driver port (`${external-system-driver-port}`) in scope — either by adding matching prototype methods or by adding/changing fields on the DTOs those methods carry.

     with:

     > Replace every `TODO: DSL` prototype in the DSL Core (`${dsl-core}`) with real logic. If new port-side behaviour is needed, you may add methods to the driver ports or add/change fields on the DTOs they carry.

   - Rationale: the current intro restates Steps 1–2 in prose. The only load-bearing detail not also in the Steps is "method-or-DTO-field" (which the `*-port-changed` flag rule later depends on). The trimmed version keeps that hint plus a goal sentence, matching the one-line lead-in pattern in `acceptance-test-writer.md` and `contract-test-writer.md`.

4. **`dsl-implementer.md`: reword the "no substituted input" note.**
   - File: `internal/assets/runtime/agents/atdd/dsl-implementer.md` (currently line 16).
   - Replace:

     > This task does not receive a substituted artifact input; the `TODO: DSL` prototypes the agent operates on are discovered from the files under its read-scope.

     with:

     > No substituted input — discover `TODO: DSL` prototypes by reading the files under `${dsl-core}`.

   - Rationale: the negative-existence framing ("does not receive a substituted artifact input") is jargon-y. The reworded form matches the directness of `contract-test-writer.md`'s parallel ("No per-invocation parameters; the contract-test target is the existing DSL surface visible in scope") and makes the discovery mechanism (grep for the marker under `${dsl-core}`) explicit.

5. **`dsl-implementer.md`: de-duplicate Step 3 and the `*-port-changed` Notes bullet.**
   - File: `internal/assets/runtime/agents/atdd/dsl-implementer.md` (Step 3 currently at line 24; Notes bullet currently at line 34).
   - Replace Step 3:

     > Before emitting outputs, list every file you wrote this invocation. For each port-changed flag, set it to `true` if **any** file in that list sits under the flag's port directory (see the flag-semantics table below). The flag is a question about the directory, not about methods — DTOs, enums, interfaces, and any other file under the port path all count. Setting `false` is a claim that you wrote zero files under that port path.

     with:

     > Before emitting outputs, set each `*-port-changed` flag per the rules in the Notes section below.

   - Leave the existing `*-port-changed` Notes bullet as the single source of explanation. Step 3 also incorrectly refers to "the flag-semantics table below" — there is no table; the bullet is the source. The replacement points readers at the right place.
   - Rationale: Step 3 currently explains the procedure + rationale; the Notes bullet repeats it. One explanation is enough — Steps say *what to do*, Notes explain *why and how*.

6. **Sweep the other agent prompts for the same classes of issue.**
   - Files: every other prompt under `internal/assets/runtime/agents/atdd/*.md` — i.e. all of:
     - `acceptance-criteria-refiner.md`
     - `command-failed-fixer.md`
     - `external-system-driver-adapter-implementer.md`
     - `external-system-driver-adapter-updater.md`
     - `external-system-stub-implementer.md`
     - `missing-output-fixer.md`
     - `scope-diff-fixer.md`
     - `system-driver-adapter-implementer.md`
     - `system-driver-adapter-updater.md`
     - `system-implementer.md`
     - `system-refactorer.md`
     - `system-updater.md`
     - `test-disabler.md`
     - `test-enabler.md`
     - `test-refactorer.md`
     - `unexpected-failing-tests-fixer.md`
     - `unexpected-passing-tests-fixer.md`
   - For each file, check for the same four issue classes Items 1–5 fixed in the three known prompts:
     - **(a) Unlabelled path placeholders.** Any `(`${name}`)` that sits in prose with no antecedent noun — should be `Labelled Name (`${name}`)` to match the `DSL Port (`${dsl-port}`)` / `DSL Core (`${dsl-core}`)` pattern.
     - **(b) Overloaded bullets bundling rules with different triggers.** Any Notes bullet that joins two procedures or two flag rules in one sentence — split if a reader could be misled by the first half.
     - **(c) Bloated intro paragraphs that restate the Steps.** Keep the goal sentence; drop the rest if Steps 1–N already say it.
     - **(d) Step / Notes duplication.** Where a Step explains procedure + rationale and a Notes bullet repeats the same content, shorten the Step to a pointer and let the Notes bullet hold the explanation.
   - For each file, emit a one-line verdict: `clean` or a numbered list of edits proposed (using the (a)–(d) classification). Aggregate into a comment block at the end of this plan before executing the per-file edits.
   - Rationale: the patterns fixed in Items 1–5 are not unique to writer/implementer prompts; the shared `Notes:` block and step/notes structure recurs across all atdd agents. Catching them once-per-issue across the whole agent set is cheaper than discovering them one prompt at a time as each is read in dispatch.

## Verification

- After edits, the prompt files should still embed cleanly into the runtime (`go build ./...` on `internal/atdd/...`).
- If there is a prompt-snapshot or golden-output test that pins any of these files, update the golden alongside the prose change.
- No BPMN flow change, no `${expected-outputs}` change, no gate binding change — the `process-diagram.md` regen workflow is unaffected.
