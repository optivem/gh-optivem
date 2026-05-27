# 20260527-2214 — ATDD Runtime Prompts Audit Plan

Per-agent prompts analysed: 20
Shared chunks analysed: 2 (`preamble.md` 47 lines, `scope.md` 25 lines)
Estimated total savings: ~180–220 lines per representative ticket dispatch set (rough, line-count proxy; weighted toward the five `fix-*` agents where the same boilerplate is paid five times per ticket if all kinds trigger)

Source: `internal/atdd/runtime/agents/embed.go` confirms `sharedPreamble + "\n\n" + sharedScope + "\n\n" + body` is the canonical concatenation for **every** dispatched agent — so any line in a per-agent body that restates a preamble or scope rule is paid as pure duplication on every dispatch of that agent.

Dispatcher substitution surface (from `internal/atdd/runtime/clauderun/clauderun.go`): kebab-case `${name}` keys only — confirmed by `TestNoSnakeCasePlaceholdersInPromptBodies`. The substituted keys are: `issue-num`, `issue-title`, `phase`, `architecture`, `subtype`, `changed-files`, `references-root`, `verify-results`, `scope-block`, `language`, `ticket-id`, `disable-marker-example`, `disable-marker-removal-example`, `acceptance-criteria`, `checklist`, `parsed-concepts`, `command`, `command-exit-code`, `command-stderr-tail`, `failing-task-name`, `missing-outputs`, `violating-paths`, `expected-outputs` (always), plus every kebab key from `cfg.PlaceholderMap()` (Family B path keys: `driver-port`, `driver-adapter`, `dsl-core`, `dsl-port`, `external-system-driver-port`, `external-system-driver-adapter`, `at-test`, `ct-test`, `system-path`, …) and every `params:` entry from the dispatching MID.

## Top wins (read this first)

1. [Shared-chunk edits #1] Introduce a new `internal/assets/runtime/shared/fix-recovery.md` chunk and concatenate it on every `fix-*` agent dispatch — kills ~30 lines × 5 fix agents = ~150 lines of duplication per ticket-with-all-fixers (needs-decision: dispatcher contract change).
2. [Per-agent body edits #1] Drop the `### Exception to the anti-rediscovery rule` block from each `fix-*` body (≈21 lines × 5 = ~105 lines). Once the new shared chunk lands, or compress to a single bullet listing the allowed commands.
3. [Per-agent body edits #2] Drop the `### Why you were dispatched` framing from each `fix-*` body (≈5–8 lines × 5 = ~30 lines). The orchestrator already routed here; framing the dispatch as a story is paid every recovery dispatch.
4. [Per-agent body edits #3] Compress the `Key semantics` Outputs prose in `acceptance-test-writer.md` / `contract-test-writer.md` / `dsl-implementer.md` — the dispatcher renders `${expected-outputs}` from the MID's `outputs:` block, and the bool flag semantics duplicate Step 3 of the bodies. ~30 lines × 3 writers = ~90 lines.
5. [Per-agent body edits #4] Drop the wide TBD framing paragraph in `external-system-stub-implementer.md` (~6 lines per dispatch) **or** flesh out the body — needs-decision.

## Per-agent body edits — `internal/assets/runtime/agents/atdd/<file>.md`

### 1. [fix-* × 5] Drop the `### Exception to the anti-rediscovery rule` block

**Files:**
- `internal/assets/runtime/agents/atdd/command-failed-fixer.md:64-85`
- `internal/assets/runtime/agents/atdd/missing-output-fixer.md:57-78`
- `internal/assets/runtime/agents/atdd/scope-diff-fixer.md:62-86`
- `internal/assets/runtime/agents/atdd/unexpected-failing-tests-fixer.md:50-70`
- `internal/assets/runtime/agents/atdd/unexpected-passing-tests-fixer.md:50-70`

**Current (representative — command-failed-fixer.md:64-85):**
> ### Exception to the anti-rediscovery rule
>
> The preamble forbids exploratory `git`/`gh` calls because every other
> ATDD phase has its context fully substituted. Fixing is different:
> `${changed-files}` lists *which files* are dirty, but not the *content*
> of those changes. To diagnose what tripped the command before you fix
> it, you need to see the actual diff.
>
> You may run:
>
> - `git diff` (or `git diff HEAD`) — to see the line-level changes in
>   the working tree that may have caused the command to fail.
> - `git show HEAD:<path>` — to see the pre-edit state of a file you've
>   already read in its current form.
>
> You may NOT run `gh issue view`, `git log`, `git status`, `git branch`,
> or `git rev-parse` — the ticket body and history are irrelevant to "why
> this command failed," and the working tree state is already in
> `${changed-files}`.
>
> This exception applies only to this fix-* task. The CYCLE will not
> re-dispatch you with the exception in force.

**Proposed (preferred):** DELETE each block; replace with a single line near the top of `## Steps`:

> Per the preamble carve-out for `fix-*` tasks, you MAY run `git diff`, `git diff HEAD`, and `git show HEAD:<path>` to read the content of files in `${changed-files}`. No other `git`/`gh` calls.

**Proposed (alt — if Needs-decision #1 is chosen):** DELETE entirely; the new `fix-recovery.md` shared chunk carries the rule once.

**Estimated savings:** ~21 lines per fix-* body × 5 bodies = ~105 lines per ticket (per fix-* dispatch — only paid for dispatches that actually trigger; common fix paths are command-failed and unexpected-failing-tests).

**Evidence:**
- `preamble.md:11-14` already names the forbidden commands; `preamble.md:25-26` already names this carve-out (`The fix-* tasks' git diff / git show HEAD:<path> carve-out applies only to those tasks.`).
- All five fix-* bodies carry near-verbatim copies. `command-failed-fixer.md` only diversifies the example ("what tripped the command"), `unexpected-*-fixer.md` only diversify ("what broke" / "what's wrong"), `missing-output-fixer.md` only diversifies ("did the work or skip the work"), `scope-diff-fixer.md` adds one extra allowed command (`git checkout HEAD -- <path>`) that survives in the proposed one-liner as a Steps detail.

**Rationale:** The shared `preamble.md` already declares the carve-out exists for `fix-*` tasks. Each fix-* body re-justifies *why* the carve-out exists (because `${changed-files}` is filename-only), then re-enumerates the same allowed/forbidden commands. The justification is paid every recovery dispatch; the agent can act on a one-line cross-reference.

### 2. [fix-* × 5] Compress `### Why you were dispatched` framing

**Files:**
- `internal/assets/runtime/agents/atdd/command-failed-fixer.md:54-62`
- `internal/assets/runtime/agents/atdd/missing-output-fixer.md:47-55`
- `internal/assets/runtime/agents/atdd/scope-diff-fixer.md:52-60`
- `internal/assets/runtime/agents/atdd/unexpected-failing-tests-fixer.md:38-48`
- `internal/assets/runtime/agents/atdd/unexpected-passing-tests-fixer.md:40-48`

**Current (representative — command-failed-fixer.md:54-62):**
> ### Why you were dispatched
>
> The calling CYCLE ran `${command}`, expected it to succeed, and `GATE_COMMAND_SUCCEEDED` routed false because the process exited with `${command-exit-code}`. The captured stderr tail and the working-tree state at the moment of failure are the entire signal. The CYCLE assumed the command would pass; it did not, so the CYCLE handed control to you.
>
> This is one of the closed `fix-*` failure-kinds:
>
> - You get **one** attempt. You do not retry. You do not re-run the command — the caller re-validates after you exit.
> - Approval gates upstream of you (the PRE step) already decided this dispatch should happen; you do not gate again.
> - Stay inside scope (see the `### Scope` block above). If the diagnosis points outside that scope (e.g. tooling owned by an external system, a CI-only environment variable, the calling CYCLE's wiring), emit the scope-exception envelope and stop.

**Proposed:** DELETE the first paragraph (recapitulates which gate routed the dispatch; the agent cannot act on it). KEEP the bullet list, but lift it into the new shared `fix-recovery.md` chunk per Needs-decision #1 — or, if no shared chunk lands, compress the bullets to a single one-line statement:

> One attempt only — do not retry, do not re-dispatch, do not re-run the verify step the caller will replay after you exit. Approval upstream of you already gated this dispatch. Stay inside `${scope-block}` — emit the scope-exception envelope if you need to widen.

**Estimated savings:** ~5–8 lines per fix-* body × 5 bodies = ~30 lines per ticket (only paid for dispatches that trigger).

**Evidence:**
- The "one attempt / no retry / upstream gated / scope envelope" bullets are near-verbatim across all five fix-* bodies.
- The opening paragraph names a specific gate (e.g. `GATE_COMMAND_SUCCEEDED`, `validate-outputs-and-scopes`) — useful narrative, but the agent's actionable contract is `${command}`/`${verify-results}`/`${violating-paths}` placeholders already in the Inputs section.

**Rationale:** The framing paragraph repeats which orchestrator decision routed the dispatch — informational only. The shared `## Steps` plus the substituted placeholders already tell the agent what to do.

### 3. [writers × 3] Compress `## Outputs` `Key semantics` prose

**Files:**
- `internal/assets/runtime/agents/atdd/acceptance-test-writer.md:24-59`
- `internal/assets/runtime/agents/atdd/contract-test-writer.md:22-57`
- `internal/assets/runtime/agents/atdd/dsl-implementer.md:28-58`

**Current (representative — acceptance-test-writer.md:24-59):** the `## Outputs` section opens with a four-line preamble about the JSONL mechanism, then `${expected-outputs}` (dispatcher-filled), then a 27-line `Key semantics:` block that re-explains:
- what `test-names` means (mostly informational; one load-bearing carve-out — "re-runs include both tests; pre-existing tests excluded");
- what `dsl-port-changed` means (this duplicates Step 3 of `dsl-implementer.md` verbatim — *"the flag is a question about the directory, not about methods"* appears in both the Step and the Outputs explanation);
- what `scope-exception-files` / `scope-exception-reason` are (this is the scope-exception envelope, fully documented in the concatenated `scope.md` — pointing at `${references-root}/scope.md` is also a bug, see Needs-decision #2).
- a hardcoded `Example call:` block that duplicates the shape `${expected-outputs}` already renders.

**Proposed:** Reduce the `## Outputs` body to:

> Emit each declared output by calling `gh optivem output write KEY=VAL` from the `Bash` tool (multiple `KEY=VAL` allowed per call; last-write-wins on re-call). The dispatcher reads the per-invocation JSONL file after you exit.
>
> ${expected-outputs}
>
> Notes:
> - `test-names` is every unqualified test method added or modified by this ticket across re-runs — not pre-existing tests the ticket did not touch.
> - For `*-port-changed` flags, list every file you wrote and set the flag `true` if any file sits under the flag's port directory (interface, DTO, enum — anything). The dispatcher's `validate-outputs-and-scopes` re-derives directory keying from `${changed-files}`, so an incorrect value mis-routes the cycle.
> - `scope-exception-files` / `scope-exception-reason` are the envelope from the prepended scope rule above. Emit only when you read or wrote outside scope.

DELETE the four-line JSONL preamble (the `gh optivem output write` reference plus the JSONL/last-write-wins detail can be moved into the dispatcher's substituted `${expected-outputs}` block if not already there — see Needs-decision #3) and DELETE the `Example call:` block (the agent has the shape from `${expected-outputs}`'s "Emit:" footer).

**Estimated savings:** ~25–30 lines per body × 3 bodies = ~75–90 lines per ticket (writers fire on most cycles, so amplification is high).

**Evidence:**
- `${expected-outputs}` is dispatcher-rendered (clauderun.go:849-879 `renderExpectedOutputs`) with `key: type` pairs, Required/Optional grouping, and a footer line `Emit: gh optivem output write KEY=VAL [KEY=VAL...]` — every duplicate in the per-agent bodies is paid per dispatch.
- The dsl-port-changed semantics paragraph in `acceptance-test-writer.md:40-48` is paraphrased into `dsl-implementer.md`'s `## Steps` 3 (`internal/assets/runtime/agents/atdd/dsl-implementer.md:24`) and again into the Flag-semantics table at lines 41-46. The same rule is restated three times in one prompt body.
- The `Example call:` block hardcodes one example shape per writer body (`dsl-implementer.md:54-58`, `acceptance-test-writer.md:55-58`, `contract-test-writer.md:54-57`) — three different shapes for three different prompts, all derivable from the dispatcher's `${expected-outputs}` Emit footer.

**Rationale:** `${expected-outputs}` is the dispatcher's drift-killer (plan 20260526-2118). Per-agent bodies that re-prose the keys, the JSONL mechanism, and an example call invert that win.

### 4. [external-system-stub-implementer.md] Drop the TBD framing paragraph (or flesh out)

**File:** `internal/assets/runtime/agents/atdd/external-system-stub-implementer.md:6`

**Current:**
> **Ownership of this task is TBD** — this placeholder body exists so the dispatcher can route the `implement-external-system-stubs` task without a missing-prompt error. The operator who claims this task should fill in the specifics (any anti-patterns specific to the dockerized stub layer (`${external-system-driver-adapter}`) beyond what is captured below). Until then, follow the task description below — it is fully specified — and treat this prompt as the canonical guide.

**Proposed:** Surface as needs-decision (#4 below) — drop the framing if the operator confirms the body is the canonical guide, otherwise leave for the task owner.

**Estimated savings:** ~6 lines per dispatch of this agent (rare in current ticket flow; this is a small win).

**Evidence:** The paragraph's last sentence already says "follow the task description below — it is fully specified" — that contradicts the "TBD" framing.

**Rationale:** Conflicting signal in one paragraph: "TBD" suggests "do not trust this," "fully specified" says "trust this." Pick one.

### 5. [refactor agents × 2] Drop redundant "Treat any path outside the Scope above as out-of-scope" line in `test-refactorer.md`

**File:** `internal/assets/runtime/agents/atdd/test-refactorer.md:24`

**Current:**
> Treat any path outside the Scope above as out-of-scope and do not modify it. `system/` is deliberately excluded — refactoring test code does not change production code.

**Proposed:** Replace with:
> `system/` is deliberately excluded from the scope above — refactoring test code does not change production code.

**Estimated savings:** 1 line per dispatch.

**Evidence:**
- The concatenated `scope.md:19-24` already declares "anything not in `write:` you cannot write to" — the first sentence is pure restatement.
- The remaining `system/` exclusion rationale is the actual content worth keeping.

**Rationale:** Small but exemplifies the pattern — every agent body re-stating the scope contract is paid per dispatch.

### 6. [acceptance-test-writer / contract-test-writer / dsl-implementer] Drop "Cohesion" / single-Edit-call restatements

**Files:**
- `internal/assets/runtime/agents/atdd/test-disabler.md:35` (`**Cohesion:** make all edits to a single file in one Edit (or Write) call. Multiple sequential edits to the same file cost extra tool round-trips for no gain.`)
- `internal/assets/runtime/agents/atdd/test-enabler.md:38` (`**Cohesion:** make all edits to a single file in one Edit (or Write) call.`)

**Current (test-disabler.md:35):**
> **Cohesion:** make all edits to a single file in one `Edit` (or `Write`) call. Multiple sequential edits to the same file cost extra tool round-trips for no gain.

**Proposed:** DELETE both lines.

**Estimated savings:** 1–2 lines per dispatch × 2 bodies; these two agents (mechanical edits) fire frequently across a ticket.

**Evidence:** `preamble.md:43-45` already states:
> ## Edit cohesion
> Batch all edits to the same file into one `Write` or `Edit` call.

The per-agent restatements add no carve-out — the only extension is a tool-cost rationale that does not change the agent's behaviour.

**Rationale:** Pure restatement of preamble rule, paid every disable/enable dispatch.

### 7. [command-failed-fixer.md] Drop classify "Misuse of the command" restatement of envelope mechanism

**File:** `internal/assets/runtime/agents/atdd/command-failed-fixer.md:46`

**Current:**
> - **Misuse of the command** — wrong arguments, wrong working directory, command run before a prerequisite step. The fix is in the calling CYCLE's wiring, not the SUT. You cannot fix this from inside the dispatch — emit the scope-exception envelope via `gh optivem output write` (see `scope.md`) and exit so the operator can repair the wiring.

**Proposed (compression):** Keep the classification but drop the envelope-mechanism reminder ("emit … via `gh optivem output write` (see `scope.md`)") — the prepended `scope.md` already specifies the envelope shape. End the bullet at:
> ... You cannot fix this from inside the dispatch — emit the scope-exception envelope and exit.

**Estimated savings:** ~1 line per dispatch; pattern repeats in every fix-* body's step-4 envelope reminder.

**Evidence:** Concatenated `scope.md:8-14` shows the exact `gh optivem output write scope-exception-* …` shape. Every per-agent reference to the CLI call form is restatement.

**Rationale:** The shared chunk owns the mechanism; per-agent reminders should name the action ("emit the envelope") without re-spelling the CLI command.

### 8. [fix-* × 5] Prune duplicate `### Anti-patterns` bullets that restate Steps or preamble

**Files (representative bullets to drop):**
- `command-failed-fixer.md:89-91, 94-95` — "Re-running the command yourself" (Step 4 already says "the caller's verify re-runs"); "Bundling a while-I'm-here cleanup" (preamble + scope-block already governs); "Fixing outside `${scope-block}`" (Step 4 already says it); "Fixing more than one or two files of change" (Step 4 already says emit envelope vs widen).
- `missing-output-fixer.md:83-86` — Same shape: "Bundling … cleanup" + "Fixing outside `${scope-block}`" duplicates.
- `scope-diff-fixer.md:91-93` — Same.
- `unexpected-failing-tests-fixer.md:75-79` — Same shape: "Bundling cleanup", "Fixing outside `${scope-block}`", "Re-running verify yourself" (Step 4 already says).
- `unexpected-passing-tests-fixer.md:75-78` — Same.

**Proposed:** Keep one bullet per fix-* body, the one that adds a *new* constraint not stated in the Steps:
- `command-failed-fixer.md`: keep "Blaming the working tree when the stderr points at the environment" (90) and "Blaming the environment when the stderr points at the working tree" (91) — these are diagnostic anti-patterns not stated elsewhere. Drop the other 5 bullets.
- `missing-output-fixer.md`: keep "Branching the fix on diff inspection" (the uniform redo+emit doctrine — load-bearing, hard for an agent to derive) and "Emitting `gh optivem output write` calls without actually doing the work". Drop the other 3.
- `scope-diff-fixer.md`: keep "Reverting violating edits that are actually legitimate" (Mode A vs Mode B is the agent's primary decision) and "Fixing more than one or two violating paths in depth". Drop the other 3.
- `unexpected-failing-tests-fixer.md`: keep "Treating the red as feedback and ignoring it" (inverts the change-cycle WRITE policy — load-bearing) and "Editing a test to silence a real SUT regression" (genuine diagnostic guard). Drop the other 4.
- `unexpected-passing-tests-fixer.md`: keep "Defaulting to an SUT edit because red→green pattern-matches" (the unique inversion this agent exists for — load-bearing). Drop "Retrying" + "Bundling cleanup" + "Fixing outside `${scope-block}`" + "Re-running verify yourself" + "Refusing to pick a side because the assertion is ambiguous" (the last is also already covered by Step 3 — "Pick the more likely side and surface the reasoning").

**Estimated savings:** ~3–5 lines per fix-* body × 5 bodies = ~20 lines per ticket (per fix-* dispatch).

**Evidence:** The duplicate bullets restate either Step 4 ("apply the smallest fix within `${scope-block}`", "the caller's verify re-runs") or `preamble.md`'s "Don't commit, don't summarise, don't ask" section or `scope.md`'s scope-envelope rule.

**Rationale:** Anti-patterns are useful when they capture a *non-obvious* failure mode. Bullets that paraphrase the same Step in negative form are paid every dispatch for no decision-shifting value.

### 9. [external-system-driver-adapter-implementer.md] Drop redundant Real-driver references

**File:** `internal/assets/runtime/agents/atdd/external-system-driver-adapter-implementer.md:6`

**Current:**
> The implement-external-system-driver-adapters task fills in real adapter logic for the External System Driver port (`${external-system-driver-port}`) — the Real driver (`${external-system-driver-adapter}`) that talks to the live external service plus the Stub driver (`${external-system-driver-adapter}`) used in test runs. Replace each `TODO: External System Driver` prototype with actual logic.

**Proposed:**
> The implement-external-system-driver-adapters task fills in real adapter logic for the External System Driver port (`${external-system-driver-port}`) — the Real and Stub drivers under `${external-system-driver-adapter}`. Replace each `TODO: External System Driver` prototype with actual logic.

**Estimated savings:** 0–1 lines per dispatch; this is a cosmetic improvement to remove repeating the `${external-system-driver-adapter}` placeholder three times in one sentence (which expands to the same path string).

**Evidence:** Same placeholder repeated three times — when substituted, the agent sees the same path string three times in one sentence (e.g. `system-test/.../external-system-driver-adapter` three times).

**Rationale:** Cosmetic but reflects a broader pattern in `external-system-driver-adapter-updater.md` (lines 6 and 22-25 — `${external-system-driver-adapter}` is referenced 7+ times in one short body). Audit the pattern; the prose can name "Ext DTOs / Real / Stub" once after the path expansion.

## Shared-chunk edits — `internal/assets/runtime/shared/<file>.md`

### 1. [scope.md] Tighten "Scope is the complete contract" to one sentence

**File:** `internal/assets/runtime/shared/scope.md:19-24`

**Current lines (19-24):**
> ## Scope is the complete contract
>
> The `## Scope` block is the only read/write contract — no prose
> "forbidden layers" or "frozen" lists exist in any agent body. Anything
> not in `read:` you cannot read; anything not in `write:` you cannot
> write to. Both escape via the envelope above.

**Proposed wording:**
> The `## Scope` block is the complete read/write contract — anything not in `read:` you cannot read, anything not in `write:` you cannot write to. Both escape via the envelope above.

**Estimated savings:** ~3 lines × every dispatch = ~3 lines × (20 agents) = ~60 line-equivalents per ticket (paid on every concatenation).

**Evidence:**
- The "no prose 'forbidden layers' or 'frozen' lists exist in any agent body" sentence is meta-commentary aimed at maintainers; agents do not need to be told that the contract is the contract.
- The second clause and the envelope reference carry the actionable content.

**Rationale:** Highest-amplification surface — every dispatch pays. Trim meta-commentary that does not change agent behaviour.

### 2. [preamble.md] Compress "Trust the orchestrator's context" intro

**File:** `internal/assets/runtime/shared/preamble.md:6-14`

**Current lines (6-14):**
> ## Trust the orchestrator's context — do not rediscover it
>
> Every ticket and repo-state value you need is already substituted into
> this prompt; re-fetching wastes tokens and risks racing the orchestrator.
>
> **Do not run** `gh issue view`, `git status`, `git log`, `git branch`,
> `git rev-parse`, or `git show <sha>`. The ticket body is in the AC /
> Checklist blocks below; the working-tree state is in `${changed-files}`
> (when populated); per-cycle history is not load-bearing.

**Proposed wording:**
> ## Trust the orchestrator's context — do not rediscover it
>
> Every ticket and repo-state value you need is already substituted into this prompt. **Do not run** `gh issue view`, `git status`, `git log`, `git branch`, `git rev-parse`, or `git show <sha>` — the ticket body is in the AC / Checklist blocks below and the working-tree state is in `${changed-files}` (when populated).

**Estimated savings:** ~3–4 lines × every dispatch = ~3 × 20 = ~60–80 line-equivalents per ticket.

**Evidence:**
- The "re-fetching wastes tokens and risks racing the orchestrator" rationale is paid every dispatch but does not change agent behaviour — the action (don't run these commands) is the load-bearing bit.
- "per-cycle history is not load-bearing" is similarly meta — the forbidden-commands list carries the contract.

**Rationale:** Highest-amplification surface; rationale prose can compress to one sentence with the forbidden-commands list as the load-bearing payload.

### 3. [preamble.md] Compress "Scope-bound reads" wording

**File:** `internal/assets/runtime/shared/preamble.md:16-30`

**Current lines (16-30):** ~15 lines explaining what's in scope, what carve-outs apply (`${changed-files}`, the fix-* git-diff carve-out), and what to do for an out-of-scope read.

**Proposed wording (~9 lines):**
> ## Scope-bound reads
>
> Read only files in the prompt's `scope:` frontmatter, plus files an explicit Step makes load-bearing. Targeted greps for prompt-named symbols are fine; open-ended exploration is a scope violation.
>
> `${changed-files}` is already-substituted context, not a read. The `fix-*` tasks' `git diff` / `git show HEAD:<path>` carve-out applies only to those tasks.
>
> If the work needs a path outside scope, emit the scope-exception envelope (see `scope.md` below) and exit.

**Estimated savings:** ~5 lines × every dispatch = ~5 × 20 = ~100 line-equivalents per ticket (highest single shared-chunk win).

**Evidence:**
- The bracketed (a)/(b) enumeration on lines 18-20 can collapse into one phrase ("files an explicit Step makes load-bearing") without losing meaning.
- The bracketed example clauses (`"look for related tests"`, `"find similar code"`) are illustrative; the rule itself ("open-ended exploration is a scope violation") is the contract.

**Rationale:** This is the third-highest amplification edit available — paid on every dispatch and currently the longest sub-section in `preamble.md`.

## Needs-decision — tradeoffs (NOT auto-applied)

### 1. Introduce a `shared/fix-recovery.md` chunk concatenated only for `fix-*` dispatches?

**Observation:** Five `fix-*` agents share three sub-sections nearly verbatim:
- `### Exception to the anti-rediscovery rule` (~21 lines, near-identical).
- `This is one of the closed fix-* failure-kinds` bullets (~5 lines, near-identical).
- Trailing anti-patterns "Bundling a while-I'm-here cleanup", "Fixing outside `${scope-block}`", "Re-running … yourself" (~6 lines, near-identical).

Aggregate: ~30 lines × 5 = 150 lines of duplication baked into the embedded prompts, paid on every fix-* dispatch.

**Tradeoff:**
- **Option A (extract).** Add `internal/assets/runtime/shared/fix-recovery.md`. Teach `internal/atdd/runtime/agents/embed.go::Prompt` to concatenate it when the agent name starts with `fix-` (or, more cleanly, key it off a frontmatter tag like `concatenate: [fix-recovery]`). Drop the duplicated sub-sections from each fix-* body. Saves ~120 lines on disk + ~30 lines per fix-* dispatch (one chunk instead of five embedded copies).
- **Option B (keep in-body but trim).** Apply Per-agent edits #1, #2, #8 above without a new shared chunk. Each fix-* body shrinks ~25 lines; the runtime concatenation contract is unchanged.

**Suggested owner:** Operator decision (the executor needs approval before adding a new shared chunk because it's a dispatcher-contract change in `embed.go`).

**Question for the user:** Add `fix-recovery.md` as a fourth shared chunk (Option A), or apply the in-body trims only (Option B)?

### 2. `${references-root}/scope.md` reference in writer agents — drift bug?

**Observation:** Three writer agents reference `${references-root}/scope.md`:
- `acceptance-test-writer.md:50`
- `contract-test-writer.md:48`
- `dsl-implementer.md:49`

But `${references-root}` is the materialized references-root (e.g. `~/.gh-optivem/references/` or `${project-root}/.gh-optivem/references/`), not the path where `scope.md` is rendered into the prompt. The canonical `scope.md` content is already concatenated above the body by `embed.go::Prompt`, so the reference is misdirecting: an agent that follows it goes to a non-existent file in the references-root tree.

The same prompts that reference `${references-root}/scope.md` are missing in `fix-*` bodies, which correctly reference just `scope.md` (because that's how the agent encounters it — as the concatenated `sharedScope` block).

**Tradeoff:**
- **Option A.** Treat as a small prompt bug. Change the writer-agent references to plain `scope.md` (matching the `fix-*` convention). Pure rename; no semantic change.
- **Option B.** Leave alone — the materialized references-root may carry a copy of `scope.md` for operators who browse the references tree.

**Suggested owner:** `architecture-sync` — the references-root materialization contract is dispatcher-owned (`assetsync.MaterializeProject`), so confirming whether `scope.md` is materialized there is an architecture check.

**Question for the user:** Route this to `architecture-sync` to confirm the materialized references-root contract, then apply Option A if `scope.md` is not materialized there?

### 3. Move the `## Outputs` boilerplate preamble into `${expected-outputs}`?

**Observation:** Per-agent body edit #3 above proposes a compressed `## Outputs` section. The four-line preamble ("Emit each declared output by calling `gh optivem output write KEY=VAL`…") is identical in `acceptance-test-writer.md`, `contract-test-writer.md`, and `dsl-implementer.md` — it could be rendered once by the dispatcher and prepended to the `${expected-outputs}` substitution.

**Tradeoff:**
- **Option A.** Move the four-line preamble into `renderExpectedOutputs` (clauderun.go:849). Per-agent bodies drop it entirely. Saves 4 lines × 3 writer dispatches per ticket = ~12 lines. Drift-killer: no prompt author writes the preamble by hand.
- **Option B.** Keep it in each prompt body; per-agent author owns the tone.

**Suggested owner:** Operator decision (changes dispatcher-side rendering, which is a contract change small enough to be self-applied but worth confirming).

**Question for the user:** Move the preamble dispatcher-side (Option A), or leave it per-prompt (Option B)?

### 4. `external-system-stub-implementer.md` TBD framing — own or drop?

**Observation:** The body says "Ownership of this task is TBD" and "treat this prompt as the canonical guide" in the same paragraph. The task is wired in `process-flow.yaml:1578-1597` and dispatches like any other writing-agent.

**Tradeoff:**
- **Option A.** Drop the TBD framing entirely. The body becomes the canonical guide. Saves ~6 lines per dispatch.
- **Option B.** Flesh out the body (add the stub-layer specifics the TBD framing teases). Keep TBD framing until done.
- **Option C.** Leave the framing — it's a stop-gap until an operator claims the task.

**Suggested owner:** Operator decision — only the human knows whether the task is genuinely unowned.

**Question for the user:** Drop the framing (Option A), flesh out (Option B), or leave (Option C)?

### 5. `unexpected-failing-tests-fixer.md` and `unexpected-passing-tests-fixer.md` carry `${verify-results}` / `${changed-files}` inputs *outside* the `## Inputs / ### Scope` block

**Observation:** Both prompts list `verify_results` and `changed_files` as bullets under `## Inputs` *before* the `### Verify results to address` / `### Changed files from the WRITE phase` sub-headers, which carry the actual substitution. The first list paraphrases the second. Compare to `command-failed-fixer.md` and `missing-output-fixer.md`, which put the parameter description and the substituted block in the same `### Parameters` entry (no duplication).

**Tradeoff:**
- **Option A.** Restructure both unexpected-* prompts to match the command-failed-fixer convention (one entry per parameter, with the substituted block immediately following). Saves ~5 lines × 2 = ~10 lines per dispatch (only paid on verify-red dispatches).
- **Option B.** Leave; the prose intro adds context the bare substituted block doesn't.

**Suggested owner:** Operator decision (consistency vs. minor verbosity).

**Question for the user:** Restructure for consistency (Option A) or leave (Option B)?

### 6. `acceptance-criteria-refiner.md` `## Inputs` section duplicates the parameter substituted into the prompt

**Observation:** `acceptance-criteria-refiner.md:19-25` (the `## Inputs` section) lists `parsed_concepts` as the input artifact (in snake_case for the prose name, while the placeholder is kebab `${parsed-concepts}`), but the prompt body Step 1 ("Read `${parsed-concepts}`") already references the placeholder directly. The prose block does not declare the substitution; it just narrates the input.

**Tradeoff:**
- **Option A.** Drop the `## Inputs` section. Step 1 names the file. The dispatcher's substituted value is the contract.
- **Option B.** Keep the section — it explains the upstream provenance ("the parsed-concepts artifact produced upstream during ticket intake"), which the agent might benefit from.

**Suggested owner:** Operator decision (prose context vs. token cost).

**Question for the user:** Drop the section (Option A), or keep (Option B)?

## Out-of-scope findings (route elsewhere)

### 1. `external-system-driver-adapter-updater.md` parameter list omits `checklist`, but Step 1 references it

**Where:** `internal/assets/runtime/agents/atdd/external-system-driver-adapter-updater.md:10-22` (the `## Inputs` section has `### Checklist` with `${checklist}` substitution, but no corresponding `### Parameters` entry like `architecture` has on line 8).

**Issue:** Inconsistent input documentation across the four "updater" agents (`system-updater.md`, `external-system-driver-adapter-updater.md`, `system-driver-adapter-updater.md`, `system-refactorer.md`, `test-refactorer.md`) — some declare `architecture` and `checklist` as parameters in a `### Parameters` block, others jump straight to the `### Checklist` substitution. Not a logical bug; just inconsistent shape. The Checklist parameter is declared on the MID in `process-flow.yaml`.

**Suggested owner:** `architecture-sync` — confirm the MID parameter declarations are the SSoT, then either align prompts to that or `process-audit` for the documentation contract.

### 2. `system-implementer.md:22` says "trace through the DSL, the driver port, and the driver adapter" but the scope (`process-flow.yaml:1442`) reads `at-test, ct-test, dsl-port, dsl-core, driver-port, driver-adapter, external-system-driver-port, external-system-driver-adapter, system-path`

**Where:** `internal/assets/runtime/agents/atdd/system-implementer.md:22`

**Issue:** The prose names a subset of the read-scope (drops dsl-core, external-system-driver-port, external-system-driver-adapter, at-test, ct-test). For some tickets the agent will need to read the external-system layer too; the prose under-sells what the agent is allowed to do. This is not a token-density bug; it's a content drift between the prose and the scope block.

**Suggested owner:** `architecture-sync` or `process-audit` — confirm whether the prose is intentionally narrowing the contract (operator-design choice) or drifting from the MID-declared scope.

### 3. `acceptance-test-writer.md:21` and `contract-test-writer.md:19` carry the same load-bearing dsl-core asymmetry rule that is paid twice (and is missing from `dsl-implementer.md`)

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
