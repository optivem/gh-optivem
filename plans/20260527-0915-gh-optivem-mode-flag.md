# Plan: `gh optivem --mode=` switchable confirmation modes

> A first draft targeted the Claude harness (slash commands + memory rewrites) and was abandoned. This plan reframes the feature as a `gh optivem` CLI flag.

## Context

`gh optivem` surfaces every human y/n prompt through one chokepoint: `internal/promptio.ConfirmYN` (10 call sites today — see audit below). Today the only way to bypass these prompts is per-command (`gh optivem commit --yes`), and that flag only exists on `commit`. Other state-mutating subcommands (`cleanup repos`, `cleanup releases`, `cleanup packages`, `cleanup sonar-projects`, configinit, ATDD driver `Approve?` gates, release) have no `--yes` equivalent.

The user wants a global toggle: `gh optivem --mode=autonomous <subcommand>` (or `GH_OPTIVEM_MODE=autonomous`) — a single switch that governs every confirmation across the CLI for one invocation.

## Decisions resolved this draft (best long-term, autonomous)

Five design questions were resolved upfront so executors aren't stalled mid-implementation. Recorded with rationale so they can be challenged in `/refine-plan`.

1. **Three modes, defined by blast-radius gate** (not by command list).
   - `cautious` (default) — every `ConfirmYN` call prompts. Identical to today's interactive behavior.
   - `commits-only` — prompts tagged `BlastHigh` still ask; prompts tagged `BlastLow` auto-yes. `BlastHigh` = commits, pushes, cleanups, pr merges, ATDD release. `BlastLow` = "Add missing statuses?", "Do you have an existing GitHub Project?", ATDD agent `Approve?` checkpoints, configinit walk prompts.
   - `autonomous` — all confirmations auto-yes (equivalent to `--yes` everywhere today).
   - **Why three not two:** the user's original framing was three. `commits-only` covers the common case "do scaffold/init/atdd work without me babysitting, but stop before anything I'd want to review on GitHub." If `commits-only` proves unused in practice it can be removed later; adding it now is cheap because the blast tag is required at each call site regardless.

2. **Source of mode: flag + env var, no persistent file.**
   - Global flag on the root command: `--mode=cautious|commits-only|autonomous`.
   - Env var: `GH_OPTIVEM_MODE=...` (matches existing `GH_OPTIVEM_CONFIG`, `GH_OPTIVEM_WORKSPACE`).
   - Resolution: flag > env > default (`cautious`).
   - **Why no persistent file:** gh-optivem doesn't persist user preferences anywhere else (config is per-project YAML, not per-user). A `~/.gh-optivem/mode` file would be a new pattern with no precedent in the codebase and would silently affect operators who don't realize they set it. Per-invocation is explicit and grep-able in shell history.

3. **`--yes` stays as a per-command primitive; `--mode=autonomous` implies it.**
   - `commit --yes` keeps its existing semantics (the `--include-untracked` interplay is preserved — see commit's --help).
   - `--mode=autonomous` is equivalent to `--yes` on every subcommand that has it, and additionally auto-yes for subcommands that have no `--yes` today (cleanup, configinit, atdd driver).
   - Explicit `--yes` + `--mode=cautious` is a contradiction; the CLI rejects it at parse time with a clear error rather than silently picking one.
   - **Why keep `--yes`:** removing it would break existing scripted callers (`gh optivem commit --yes "msg"` is documented in `--help` examples and used in `acceptance-test/action.yml`-style flows). `--yes` is the low-level primitive; `--mode` is the global composer over it.

4. **ATDD fix-agent dispatch is a special case (see "Special case: fix-agent dispatch" below).** Short version: introduce a third blast tier `BlastCritical` for `human` STOP nodes (always prompt, ignore mode entirely); reclassify `approve` nodes that wrap `fix-*` recovery dispatches as `BlastHigh` (not `BlastLow`) because a fix-agent run is unbounded code rewriting; propagate `GH_OPTIVEM_MODE` into the spawned Claude subprocess env so nested `gh optivem` calls inherit. **Why upgrade these to High/Critical:** the cost asymmetry — auto-approving "Add missing statuses?" wrongly is recoverable (one extra label); auto-approving a fix-agent dispatch wrongly burns minutes of model time and may rewrite files the operator wanted to inspect first. Critical/High default protects against that with one explicit operator gesture (`--mode=autonomous`) as the opt-in.

5. **Mechanism: mode-aware `Confirm` helper layered over `promptio.ConfirmYN`.**
   - New package `internal/mode/` exports `Mode` (enum), `Resolved` (the resolved-this-invocation mode + source), and `Confirm(ctx, blast, in, out, prompt) (bool, error)`.
   - `Confirm` short-circuits to `true` when `Resolved.Mode == Autonomous`, or when `Resolved.Mode == CommitsOnly && blast == BlastLow`. Otherwise it delegates to `promptio.ConfirmYN`.
   - Each call site is updated to pass a `BlastLow`/`BlastHigh` tag — this is the only behavioural change at the call site; the prompt string is unchanged.
   - `promptio.ConfirmYN` itself is **not modified** — it stays the low-level helper. `internal/mode` depends on `promptio`, not the other way around, so packages that already import `promptio` directly (e.g. `release`) can keep working unchanged and migrate at their own pace.
   - **Why a new package not a `promptio.ConfirmYNMode`:** mode resolution depends on flag/env state that `promptio` (a pure y/n helper) has no business knowing about. Keeping `promptio` pure also lets `internal/mode` be tested in isolation without touching prompt I/O.

## Call-site audit (10 sites)

Today's `promptio.ConfirmYN` / `ConfirmYNVia` call sites and the blast tag each one gets:

| File:line | Prompt | Blast |
|---|---|---|
| `cross_repo_commands.go:319` | `"Commit these changes to %s?"` | High |
| `main.go:695` | `"  Proceed?"` (doctor flow) | High |
| `main.go:728` | `"  File a bug report?"` | Low |
| `internal/atdd/runtime/agents/registry.go:54` | `"Approve?"` (`humanStop` — `agent: human` STOP node) | **Critical** |
| `internal/configinit/prompt.go:250` | `"  Do you have an existing GitHub Project?"` | Low |
| `internal/atdd/runtime/driver/driver.go:700` | `"  Approve?"` (driver `approve` primitive — classify per BPMN context) | Low or High (see Special case) |
| `internal/atdd/runtime/driver/driver.go:735` | `"  Approve?"` (driver `approve` primitive — classify per BPMN context) | Low or High (see Special case) |
| `internal/atdd/runtime/driver/driver.go:1083` | `"  Approve?"` (driver `approve` primitive — classify per BPMN context) | Low or High (see Special case) |
| `internal/atdd/runtime/release/release.go:163` | release confirmer (prompt varies) | High |
| `internal/steps/project.go:460` | `"  Add missing statuses?"` | Low |
| `internal/config/config.go:971` | `"Proceed?"` | Low/High (decide during item 7 review of the call site) |

Cleanup subcommands today have **no** y/n prompt — they go straight from `--dry-run` to live delete. That's a separate bug (a `cleanup repos myorg --prefix foo` with no `--dry-run` is destructive without confirmation). Out of scope for this plan; flagged in "Follow-ups" below.

## Items

### 1. New package `internal/mode/mode.go`

- Define `type Mode int` with constants `Cautious`, `CommitsOnly`, `Autonomous` and `String()` / `ParseMode(string) (Mode, error)`.
- Define `type Blast int` with constants `BlastLow`, `BlastHigh`, `BlastCritical`. `BlastCritical` is the "always prompt regardless of mode" tier — reserved for `human` STOP nodes and any other gate that must never be auto-approved.
- Define `type Resolved struct { Mode Mode; Source string }` where `Source` is `"flag"`, `"env"`, or `"default"` (used in the banner — see item 4).
- Export `Resolve(flagValue string, env func(string) string) (Resolved, error)` — pure function, testable without env mutation. Returns `("", "default", Cautious)` when flag and env are both empty.
- Export `Confirm(r Resolved, blast Blast, in io.Reader, out io.Writer, prompt string) (bool, error)` — short-circuit logic from decision 5, delegates to `promptio.ConfirmYN` otherwise. `BlastCritical` never short-circuits — always delegates, even under `Autonomous`.
- Export `ConfirmVia(r Resolved, blast Blast, asker promptio.Asker, out io.Writer, prompt string) (bool, error)` — same short-circuit, delegates to `promptio.ConfirmYNVia`. Needed because the ATDD bindings use the Asker abstraction, not direct stdin.

### 2. Unit tests `internal/mode/mode_test.go`

- `TestResolve_FlagBeatsEnv`, `TestResolve_EnvBeatsDefault`, `TestResolve_DefaultIsCautious`, `TestResolve_InvalidFlagError`, `TestResolve_InvalidEnvError`.
- `TestConfirm_CautiousAlwaysPrompts` (both blasts).
- `TestConfirm_CommitsOnlySkipsLowAutoYes` (BlastLow), `TestConfirm_CommitsOnlyHighPrompts` (BlastHigh).
- `TestConfirm_AutonomousAlwaysYes` (Low and High blasts).
- `TestConfirm_CriticalAlwaysPrompts` (cautious, commits-only, autonomous — all three modes must still prompt when `blast == BlastCritical`).
- Use `bytes.Buffer` for `out` and `strings.NewReader` for `in`; assert no prompt written when short-circuit fires.

### 3. Wire the global flag in `main.go`

- Add `--mode` as a persistent flag on the root command (alongside `--config` and `--workspace`).
- Read `GH_OPTIVEM_MODE` env at flag-resolution time.
- Call `mode.Resolve` in the `PersistentPreRunE` (or equivalent), stash `Resolved` on a context the subcommands can pull from.
- Add a one-line banner emitted to stderr at command start when `Resolved.Mode != Cautious`: `Mode: <mode> (source: <source>)`. Cautious is silent (matches today's no-banner behavior).
- `--mode=autonomous` + explicit `--yes=false` (where applicable) → error: `--mode=autonomous and --yes=false are contradictory; pick one`. `--mode=autonomous` + `--yes=true` → no-op (consistent).

### 4. Pipe `Resolved` to subcommand call sites

- Where Cobra commands receive `*cobra.Command`, add a `cmdctx.Mode(cmd) Resolved` accessor (or a small helper) that reads from the persistent flag.
- For non-Cobra entry points (e.g. ATDD driver invoked from `gh optivem implement`), thread `Resolved` through the same options struct that already carries `Stdin`/`Stdout`.

### 5. Migrate `cross_repo_commands.go:319` (commit per-repo confirmation)

- Replace `promptio.ConfirmYN(os.Stdin, os.Stdout, "Commit these changes to %s?", ...)` with `mode.Confirm(resolved, mode.BlastHigh, os.Stdin, os.Stdout, ...)`.
- The existing `--yes` flag short-circuits before reaching this call site (current behaviour). Keep that order — `--yes` checked first, `mode.Confirm` second — so a contradictory `--yes=false --mode=autonomous` invocation is caught by the parse-time check in item 3 rather than by silent fall-through here.

### 6. Migrate `internal/configinit/prompt.go:250` (project init)

- Replace with `mode.Confirm(resolved, mode.BlastLow, ...)`.
- The configinit caller needs the `Resolved` value threaded in — it's currently called from `init` command flow.

### 7. Migrate `internal/config/config.go:971` ("Proceed?")

- Locate the call site, read context to classify blast. The prompt is generic "Proceed?" so this needs human judgement at execution time. Default classification: BlastHigh if it gates a state-mutating action, BlastLow if it gates a continue-walking-the-config flow. Note the classification in the commit message.

### 8. Migrate `internal/steps/project.go:460` ("Add missing statuses?")

- Replace with `mode.Confirm(resolved, mode.BlastLow, ...)`. This is a "add fields to GitHub Project that the YAML declared but the live project lacks" prompt — low blast.

### 9. Migrate ATDD agent / driver / release call sites

- `internal/atdd/runtime/agents/registry.go:54` (`humanStop`) — `mode.Confirm(resolved, mode.BlastCritical, ...)`. The `agent: human` STOP node is the BPMN author's explicit hard-halt; it must never be auto-approved (see Special case section).
- `internal/atdd/runtime/driver/driver.go:700, 735, 1083` — `newApproveDispatcher` call sites. Blast level depends on what the `approve` node wraps in the BPMN:
  - When immediately preceding a `fix-*` agent dispatch (FIX_COMPILE, FIX_TEST, etc.) → `mode.BlastHigh`.
  - When gating any other routable Q3=A choice (non-fix, non-recovery) → `mode.BlastLow`.
  - **Decision needed at implementation time:** read `process-flow.yaml` to map each `approve` BPMN node to its blast level. The dispatcher itself (driver.go) can't know — the BPMN context determines it. Cleanest encoding: add an optional `blast: low|high|critical` field on `approve` nodes in `process-flow.yaml`, default `low`. Statemachine parser surfaces it on `RawNode.Blast`; the dispatcher reads it and passes to `mode.Confirm`.
- `internal/atdd/runtime/release/release.go:163` — `mode.Confirm(resolved, mode.BlastHigh, ...)`. The release confirmer is called with operator-supplied prompts; the wrapper signature accepts the prompt string as-is.
- `Resolved` is threaded through the ATDD `opts` struct already used for `Stdin`/`Stdout`.

### 10. Migrate `main.go:695, 728`

- `main.go:695` (doctor "Proceed?") → `mode.BlastHigh` (gates a `git config --global` write).
- `main.go:728` ("File a bug report?") → `mode.BlastLow`.

### 11. Update `--help` text

- Root `--mode` flag description: `Approval mode: cautious (default — ask before every confirmation), commits-only (ask only before commits, pushes, cleanups, releases), or autonomous (auto-yes everywhere). Env: GH_OPTIVEM_MODE.`
- Update `commit --yes` description to mention the relationship: `Skip per-repo confirmation (required without a TTY). Equivalent to --mode=autonomous for this command.`
- Update top-level `--help` Long string to list `--mode` alongside `--config` and `--workspace` in the global-flags section.
- Run `gh optivem doctor` (or whatever the help-text-updater agent uses) after wiring to catch any other stale `--help` references.

### 12. Acceptance tests

- Add `mode_test.go` at the repo root (or inside `cross_repo_commands_test.go`) covering:
  - `gh optivem commit --mode=autonomous "msg"` on a dirty repo: commits without prompting.
  - `GH_OPTIVEM_MODE=autonomous gh optivem commit "msg"`: same.
  - `gh optivem commit --mode=commits-only "msg"`: still prompts (commit is BlastHigh).
  - `gh optivem commit --mode=cautious --yes "msg"`: parse-time error (or `--yes` wins — pick during item 3 implementation and note the choice here).
  - `gh optivem --mode=garbage commit "msg"`: parse-time error with help text listing valid modes.
- Test the banner: `--mode=autonomous` emits `Mode: autonomous (source: flag)` to stderr; `--mode=cautious` is silent.

### 13. README / CONTRIBUTING

- README: add a `### Approval modes` subsection under "Usage" describing the three modes, the flag, the env var, and `--yes` equivalence.
- CONTRIBUTING (if it exists): note that new y/n prompts must use `mode.Confirm` and pick a blast tag — no fresh `promptio.ConfirmYN` call sites without going through the mode wrapper.

## Special case: ATDD fix-agent dispatch

The `fix-*` recovery dispatches (FIX_COMPILE, FIX_TEST, …) routed through `newClaudeRunDispatcher` (`driver.go:737`) sit at a different blast radius than ordinary CLI confirmations. A fix-agent run is an unbounded autonomous Claude session that can rewrite source files, run tests, and stage commits — minutes of model time and material code mutation per dispatch. The mode mechanism must not silently auto-approve them under `commits-only`.

### Why fix-agent dispatch needs special handling

- **Cost asymmetry**: auto-yes on "Add missing statuses?" wrongly costs one extra label. Auto-yes on "Approve FIX_COMPILE?" wrongly burns the model budget and may rewrite files the operator wanted to inspect before delegating.
- **No mid-run abort**: once `clauderun.Dispatch` is called, the subprocess runs to completion or hard timeout. The operator can't `Ctrl-C` cleanly mid-fix without leaving the working tree in a half-rewritten state.
- **Recursive `gh optivem` invocations**: a fix agent may itself run `gh optivem commit`, `gh optivem test`, etc. The mode the parent invocation chose must propagate, or the child invocations fall back to `cautious` and block waiting for input the parent operator can't see.

### Behaviour matrix

| Mode | `agent: human` STOP nodes (`BlastCritical`) | `approve` wrapping `fix-*` dispatch (`BlastHigh`) | Other `approve` (`BlastLow`) | Inside the spawned fix-agent subprocess |
|---|---|---|---|---|
| `cautious` | Prompt | Prompt | Prompt | child inherits `cautious` → child's nested `gh optivem` prompts as usual |
| `commits-only` | **Prompt** (never auto-yes) | **Prompt** (high blast) | Auto-yes | child inherits `commits-only` |
| `autonomous` | **Prompt** (never auto-yes) | Auto-yes | Auto-yes | child inherits `autonomous` |

Note the `commits-only` row: `BlastHigh` still prompts, `BlastCritical` always prompts. The two upgrades together mean an operator who chose `commits-only` to skip low-stakes prompts will still get a stop-and-confirm before a fix-agent burns time.

### Item 9a (new): encode blast on `approve` BPMN nodes

- Add an optional `blast: low|high|critical` field to the `approve` (and `human`) node types in `internal/atdd/runtime/statemachine/process-flow.yaml`.
- Update the statemachine parser to surface `blast:` on `RawNode` (default `low` for `approve`, `critical` for `human`).
- Audit `process-flow.yaml`: every `approve` node that immediately precedes a `fix-*` dispatch gets `blast: high`. Leave the rest at the default.
- The driver dispatcher reads `raw.Blast` and passes it to `mode.Confirm`.

### Item 9b (new): propagate `GH_OPTIVEM_MODE` into spawned Claude subprocess

- In `clauderun.Dispatch` (or wherever `exec.Command(...)` builds the child env for the Claude subprocess), set `GH_OPTIVEM_MODE=<resolved.Mode.String()>` on the child env.
- The child `gh optivem` invocations inside the agent then resolve the same mode via env (per decision 2 of this plan: env beats default).
- The flag-source banner emitted by the child will read `Mode: autonomous (source: env)`, which is the desired audit trail — the operator can tell that the child inherited rather than being given an explicit flag.

### Test additions

- `TestFixAgentApprove_CommitsOnlyStillPrompts` — set `--mode=commits-only`, run a BPMN that hits an `approve` node with `blast: high` before a `fix-*` dispatch; assert the prompt is written and the test feeds `y`.
- `TestHumanStop_AutonomousStillPrompts` — set `--mode=autonomous`, hit `agent: human` STOP; assert prompt is still written.
- `TestFixAgentSubprocess_InheritsMode` — capture the child env built by `clauderun.Dispatch` (or run an end-to-end smoke that checks the child's `--help` banner output); assert `GH_OPTIVEM_MODE` is set to the parent's resolved mode.

### Out of scope (raise separately if needed)

- **Fix-agent retry caps.** If FIX_COMPILE fails and the engine routes back to FIX_COMPILE, that loop can be expensive under `--mode=autonomous`. There is likely already a per-Run cap in the engine; mode does not change it. If the cap doesn't exist, that's a separate plan, not this one.
- **Mid-run mode change.** Operators can't switch mode after `gh optivem implement` has started; the flag is resolved once at startup. If mid-run is desired, that's a separate plan.

## Default state

`gh optivem` with no `--mode` flag and no `GH_OPTIVEM_MODE` env var behaves identically to today (cautious). Verification: the existing test suite passes unchanged before any call-site migration — the new package is dead code until call sites are migrated, so item 1 + item 2 alone are a safe first slice.

## Verification

1. **Default (no change)** — run the full test suite before any migration; nothing breaks.
2. **`gh optivem --mode=autonomous commit "msg"`** on a workspace with one dirty repo → commits without prompting, banner `Mode: autonomous (source: flag)` on stderr.
3. **`GH_OPTIVEM_MODE=autonomous gh optivem commit "msg"`** → same, banner says `source: env`.
4. **`gh optivem --mode=commits-only init <project>`** on a fresh project → scaffolds without asking "Do you have an existing GitHub Project?" (BlastLow auto-yes), still prompts before the final commit (BlastHigh).
5. **`gh optivem --mode=cautious commit "msg"`** → identical to today's `gh optivem commit "msg"` (no banner).
6. **`gh optivem --mode=garbage commit "msg"`** → exits non-zero with `invalid --mode value "garbage"; want one of: cautious, commits-only, autonomous`.
7. **ATDD driver Approve? under `--mode=commits-only`** → auto-yes (BlastLow). Under `--mode=autonomous` → also auto-yes. Under `--mode=cautious` → prompts.

## Follow-ups (out of scope, raise as separate plans)

- **Cleanup subcommands have no y/n today.** `cleanup repos --prefix foo` deletes immediately after the dry-run check. Should pass through `mode.Confirm` with BlastHigh so cautious operators get prompted. Separate plan.
- **Bypass for non-interactive CI.** If a CI script runs `gh optivem` without a TTY and without `--mode=autonomous`, `promptio.ConfirmYN` returns `false` on EOF and the operation declines. That's correct behaviour but may surprise CI authors. Consider auto-promoting to `autonomous` when stdin is not a TTY *and* the operator passed `--mode=` explicitly — but that's a usability tweak, not a correctness bug.

## Critical files

- `internal/mode/mode.go` (new)
- `internal/mode/mode_test.go` (new)
- `main.go` (root command flag + PersistentPreRunE + doctor/bug-report call sites)
- `cross_repo_commands.go` (commit confirmation)
- `internal/configinit/prompt.go` (init walk)
- `internal/config/config.go` (Proceed?)
- `internal/steps/project.go` (Add missing statuses?)
- `internal/atdd/runtime/agents/registry.go` (Approve?)
- `internal/atdd/runtime/driver/driver.go` (three Approve? sites)
- `internal/atdd/runtime/release/release.go` (release confirmer)
- `internal/atdd/runtime/clauderun/` (child env propagation — find the `exec.Command(...)` site and set `GH_OPTIVEM_MODE`)
- `internal/atdd/runtime/statemachine/process-flow.yaml` (per-node `blast:` annotation on `approve` nodes that wrap `fix-*` dispatch)
- `internal/atdd/runtime/statemachine/` (parser support for `blast:` field on RawNode)
- `README.md` (Approval modes subsection)
