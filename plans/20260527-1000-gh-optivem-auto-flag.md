# Plan: `gh optivem --auto` + `--confirm` exclusion overlay

> 🤖 **Picked up by agent** — `Valentina_Desk` at `2026-05-27T08:49:26Z`

> Supersedes the earlier `plans/20260527-0915-gh-optivem-mode-flag.md` draft (deleted; original committed at `74358bd`, recoverable via `git show 74358bd:plans/20260527-0915-gh-optivem-mode-flag.md`). The three-mode preset (`cautious|commits-only|autonomous`) was abandoned in `/refine-plan` because `commits-only` lied about its scope (it covered cleanups, pushes, releases too) and because operators reason in terms of "auto-approve everything except these specific things," not in 3-tier presets. This plan replaces the preset model with a flag-plus-exclusion-list design and reuses today's `--yes` per-command primitive unchanged.

## Context

`gh optivem` skip-confirmation surface today is fragmentary:

| Flag | Where | What it does |
|---|---|---|
| `--yes` | `commit` only (`cross_repo_commands.go:117`) | Skip per-repo "Commit changes to X?" |
| `--autonomous` | `implement` only (`implement_commands.go:109`) | Bundle: (a) skip ATDD human STOPs; (b) run claude subprocess headless via `claude -p` |
| (nothing) | `cleanup`, `configinit`, ATDD `Approve?`, release | No skip mechanism — `--dry-run` for cleanup, otherwise prompt-always |

10 `promptio.ConfirmYN`/`ConfirmYNVia` call sites today; no shared policy.

User wants a single global toggle with sensible safe defaults: auto-yes everywhere, *except* `commit` (visible — lands on GitHub history) and `fix` (recovery dispatch — means something already went wrong, rewriting more code unsupervised is risky). Operators override the default via `--confirm=<categories>`.

## Decisions resolved (best long-term, autonomous)

Five design decisions resolved upfront so executors aren't stalled mid-implementation. Recorded with rationale so they can be challenged in `/refine-plan`.

### 1. Two new flags, one split, `--yes` unchanged.

- **`--auto`** — boolean, root-level persistent flag. Opt into auto-approve policy. Env var `GH_OPTIVEM_AUTO=true`.
- **`--confirm=<categories>`** — comma-separated string, root-level persistent flag. Exclusion list — categories named here still prompt even when `--auto` is set. Defaults to `commit,fix` when `--auto` is set and `--confirm` is not given. Env var `GH_OPTIVEM_CONFIRM=commit,fix`.
- **`commit --yes`** — kept exactly as today. Per-command primitive, semantics unchanged.
- **`implement --autonomous`** — split into two flags:
  - `--auto` (global) handles the "skip human STOPs" half.
  - **`--headless`** (new, on `implement`) handles the "run claude subprocess headless via `claude -p`" half.
  - `--autonomous` becomes a deprecated alias for `--auto --headless` for one release; removed in a follow-up.

**Why two flags not one preset:** the 3-mode preset (`cautious|commits-only|autonomous`) buried the categorisation; operators couldn't say "auto, but ask before fix-agents specifically." The exclusion model makes the categorisation a public, composable vocabulary.

**Why keep `--yes` per-command:** scripts in `acceptance-test/action.yml`-style flows and `--help` examples document `gh optivem commit --yes "msg"`. Promoting `--yes` to global with default exclusion `commit,fix` would silently break those scripts (the commit prompt would re-appear under `--yes`). Keeping `--yes` as the per-command primitive and introducing `--auto` as the global policy is two names for two concepts; no magic, no breakage. `--auto` and `--yes` compose: `--auto commit --yes "msg"` works fine, both skip the prompt.

### 2. Category vocabulary (closed set, pinned in code).

```
commit   — git commit confirmations              (cross_repo_commands.go:319)
fix      — ATDD approve nodes wrapping fix-*      (driver.go subset — encoded in process-flow.yaml)
release  — ATDD release confirmer                 (release/release.go:163)
prompt   — interactive walk / Q-A prompts          (configinit, project, config, doctor, bug-report,
                                                    non-fix approve nodes)
human    — ATDD `agent: human` STOP node           ALWAYS implicitly in confirm set. Cannot be removed.
```

Five categories, one of which (`human`) is implicit and operator-uncontrollable.

**Why this grain:** matches the user's reasoning ("auto-approve everything except commit and fix"). Five categories is small enough to memorise from `--help`, large enough to compose interesting policies. `prompt` is a deliberate catch-all for low-stakes interactive prompts — splitting it further (e.g. separate `doctor`, `bug-report`, `walk`) adds vocabulary cost without composability benefit; can be done later if a real use case appears.

**Why `human` is implicit:** `agent: human` BPMN STOP nodes are the explicit BPMN-author hard-halt. They exist precisely because the workflow author decided "no machine decides this." Allowing `--confirm=...` to opt out would make the STOP a soft suggestion; that's the wrong default and the wrong opt-out gesture. If the operator truly wants to auto-approve human STOPs for a one-off run, they can edit the BPMN — that's the explicit gesture the design expects.

### 3. Default exclusion is `commit,fix` (not empty).

When `--auto` is set and `--confirm` is not given, the resolved exclusion set is `{commit, fix, human}`. `human` is the always-implicit member; `commit,fix` is the configurable default.

- `--auto` alone → prompts for `commit`, `fix`, `human`; auto-yes for `release`, `prompt`.
- `--auto --confirm=fix` → prompts for `fix`, `human`; auto-yes for `commit`, `release`, `prompt`.
- `--auto --confirm=` (empty string) → prompts for `human` only; auto-yes for everything else. This is true "autonomous" mode.
- `--auto --confirm=commit,fix,release` → stricter than default.

**Why safe-by-default:** the cost asymmetry is one-sided. Auto-approving a `commit` wrongly publishes content to GitHub that the operator wanted to inspect — recoverable but visible. Auto-approving a `fix` wrongly burns model budget and may rewrite files the operator wanted to look at first — minutes of wasted time per dispatch. Auto-approving a `prompt` wrongly costs a label or an extra column — trivial. The default should protect the expensive failure mode.

### 4. Sources tracked separately for `--auto` and `--confirm`.

`Resolved` carries two source strings (`"flag"`, `"env"`, `"default"`). A banner is emitted to stderr at command start when `--auto` is on:

```
Auto: true (auto-source: flag, confirm-source: default → commit,fix)
```

Cautious mode (no `--auto`) is silent. This matches today's no-banner default.

**Why separate sources:** an operator can have `GH_OPTIVEM_CONFIRM=fix` in their shell profile and pass `--auto` per-invocation. The banner needs to show the env-derived narrower confirm set so the operator can tell why a commit didn't prompt.

### 5. Mechanism: `internal/approval/` package wraps `promptio`.

- New package `internal/approval/` exports:
  - `Category` enum + `ParseCategory`/`String`
  - `Resolved` struct (Auto bool, ConfirmSet map[Category]bool, AutoSource string, ConfirmSource string)
  - `Resolve(auto bool, autoChanged bool, confirm string, confirmChanged bool, env func(string) string) (Resolved, error)` — pure function, testable without env mutation. Handles flag-vs-env precedence and the default-exclusion-when-auto-set logic.
  - `Confirm(r, category, in, out, prompt) (bool, error)` — short-circuits to `true` when `r.Auto && !r.ConfirmSet[category]`; delegates to `promptio.ConfirmYN` otherwise. `human` always delegates.
  - `ConfirmVia(r, category, asker, out, prompt) (bool, error)` — same short-circuit, delegates to `promptio.ConfirmYNVia`. Needed for the ATDD Asker abstraction.
- `promptio.ConfirmYN` itself is **not modified**. `internal/approval` depends on `promptio`, not the reverse.

**Why a new package not a `promptio.ConfirmYNAuto`:** approval resolution depends on flag/env state that `promptio` (a pure y/n helper) has no business knowing about. Keeping `promptio` pure lets `internal/approval` be tested in isolation without prompt I/O.

**Why the `*Changed` bools on `Resolve`:** Cobra distinguishes "flag explicitly set" from "flag has its default value." Without the `Changed` bit, `--confirm=` (explicit empty) and "no --confirm given" both look like `""`, and the default-exclusion fallback can't fire correctly.

## Call-site audit (10 sites)

Today's `promptio.ConfirmYN`/`ConfirmYNVia` call sites tagged with the new category vocabulary:

| File:line | Prompt | Category |
|---|---|---|
| `cross_repo_commands.go:319` | `"Commit these changes to %s?"` | `commit` |
| `main.go:695` | `"  Proceed?"` (doctor flow) | `prompt` |
| `main.go:728` | `"  File a bug report?"` | `prompt` |
| `internal/atdd/runtime/agents/registry.go:54` | `"Approve?"` (humanStop) | `human` |
| `internal/configinit/prompt.go:250` | `"Do you have an existing GitHub Project?"` | `prompt` |
| `internal/atdd/runtime/driver/driver.go:700, 735, 1083` | `"Approve?"` (approve nodes — three sites) | `fix` or `prompt` per BPMN context (see item 9a) |
| `internal/atdd/runtime/release/release.go:163` | release confirmer | `release` |
| `internal/steps/project.go:460` | `"Add missing statuses?"` | `prompt` |
| `internal/config/config.go:971` | `"Proceed?"` | `prompt` (default; revisit during item 7 if it gates state-mutation) |

Cleanup subcommands today have **no** y/n prompt — they go from `--dry-run` to live delete. That's a separate bug; flagged in "Follow-ups" below.

## Items

### 11. Split `implement --autonomous`

- Remove the bundled behavior at `implement_commands.go:109`. Replace with:
  - `--headless` (bool, default false) — controls the claude-subprocess headless-vs-interactive bit only.
  - Keep `--autonomous` as a deprecated alias for `--auto --headless`. Emit a deprecation warning to stderr when used: `gh optivem: --autonomous is deprecated; use --auto --headless`.
- The driver's `Options.Autonomous` field splits into `Options.SkipHumanSTOP` (driven by `Resolved` lookup of `human`) and `Options.Headless` (from `--headless`). Today's call sites that read `opts.Autonomous` map to the appropriate replacement at migration time.
- Drop the deprecated alias in a follow-up release once docs/scripts have caught up.

### 12. Update `--help` text

- Root `--auto` description: `Auto-approve confirmations except for categories listed in --confirm. Defaults to --confirm=commit,fix. Env: GH_OPTIVEM_AUTO.`
- Root `--confirm` description: `Comma-separated category list that still prompts under --auto. Categories: commit, fix, release, prompt. (human is always confirmed.) Default when --auto is set: commit,fix. Env: GH_OPTIVEM_CONFIRM.`
- Update `commit --yes` description: unchanged. Optionally add: `Equivalent to passing --auto --confirm= for this command.`
- Update `implement --headless` description: `Run the claude subprocess in headless `claude -p` mode (no interactive window).`
- Update `implement --autonomous` description: `[Deprecated] Equivalent to --auto --headless. Will be removed in a future release.`
- Update top-level `--help` Long string to list `--auto` and `--confirm` in the global-flags section alongside `--config` and `--workspace`.
- Run the help-text-updater agent (or `gh optivem doctor` if it covers this) after wiring to catch stale references.

### 13. Acceptance tests

Coverage that matters (executor's discretion on exact split):
- `gh optivem --auto implement` → prompts on `commit`, `fix`, `human`; auto-yes on `release`, `prompt`.
- `gh optivem --auto --confirm= implement` → prompts on `human` only.
- `gh optivem --auto --confirm=fix commit "msg"` → commit auto-yes (commit not in confirm set).
- `gh optivem commit --yes "msg"` → unchanged (preserves backwards compat).
- `gh optivem --auto commit --yes "msg"` → both routes skip; commits without prompt.
- `gh optivem --confirm=garbage --auto commit "msg"` → parse-time error listing valid categories.
- Banner emitted to stderr correctly under each source combination.
- `implement --autonomous` → emits deprecation warning; behaves as `--auto --headless`.

### 14. README / CONTRIBUTING

- README: add `### Auto-approve` subsection under "Usage" covering `--auto`, `--confirm=<categories>`, the default exclusion (`commit,fix`), env vars, and the `--yes` per-command primitive's continued role.
- CONTRIBUTING (if present): new y/n prompts must go through `approval.Confirm` with a category tag; no fresh `promptio.ConfirmYN` call sites without going through the approval wrapper.

## Special case: ATDD fix-agent dispatch (reframed)

The original plan had a Special case section arguing fix-agent dispatch needs `BlastHigh` treatment under the 3-mode design. In the new design this section largely collapses: `fix` is its own category, and it's in the default `--confirm` set. An operator gets fix-prompting by default under `--auto`; opting out requires an explicit `--confirm=` (or `--confirm=commit` to drop fix from the exclusion set).

What still warrants special treatment:

- **`category: fix` audit in `process-flow.yaml`** (item 9a). Every approve node wrapping a `fix-*` dispatch must be tagged `fix`. Statemachine tests should enforce this — an `approve` node directly preceding a `Dispatch` of a `fix-*` agent that lacks `category: fix` is a config error.
- **Env propagation into claude subprocess** (item 9b). Without this, nested `gh optivem` calls inside the agent fall back to `cautious` and block on prompts the operator can't see.
- **No mid-run mode change.** Approval is resolved once at startup. Mid-run override is out of scope.
- **Fix-agent retry caps unchanged.** Mode does not change retry behavior; if FIX_COMPILE loops, that's a separate plan.

## Default state

`gh optivem` with no `--auto`, no `--confirm`, no env vars: behaves identically to today. The new package is dead code until call sites are migrated, so item 1 + item 2 alone is a safe first slice.

## Verification

1. **Default (no change)** — full test suite passes before any migration.
2. **`gh optivem --auto commit "msg"`** on dirty repo → prompts (commit in default exclusion).
3. **`gh optivem --auto --confirm= commit "msg"`** → commits without prompt.
4. **`gh optivem --auto --confirm=fix commit "msg"`** → commits without prompt (commit not in narrower confirm set).
5. **`gh optivem commit --yes "msg"`** → commits without prompt (per-command --yes unchanged).
6. **`GH_OPTIVEM_AUTO=true gh optivem implement`** → prompts on commit, fix, human; auto-yes on release, prompt.
7. **`gh optivem --auto --confirm=garbage commit`** → parse-time error listing valid categories.
8. **`gh optivem implement --autonomous`** → emits deprecation warning; same behavior as `--auto --headless`.
9. **ATDD `Approve?` under `--auto`** on a `fix-*` approve node → still prompts. On a non-fix approve → auto-yes.
10. **Spawned claude subprocess** under `--auto`: child env contains `GH_OPTIVEM_AUTO=true` and `GH_OPTIVEM_CONFIRM=<resolved>`; child banner reads `auto-source: env`.

## Follow-ups (out of scope, raise as separate plans)

- **Cleanup subcommands have no y/n today.** Wrap them with `approval.Confirm(..., CategoryPrompt, ...)` so `--auto` cautious operators still get prompted. Separate plan.
- **Drop the `implement --autonomous` deprecated alias** in a follow-up release once docs/scripts have migrated.
- **Bypass for non-interactive CI without `--auto`.** Today `promptio.ConfirmYN` returns `false` on EOF in non-TTY. If a CI script forgets `--auto`, the operation declines. Consider auto-detecting non-TTY and promoting to `--auto` only when the operator also passed `--confirm=` explicitly — usability tweak, separate plan.

## Critical files

- `internal/approval/approval.go` (new)
- `internal/approval/approval_test.go` (new)
- `main.go` (root command `--auto` + `--confirm` flags, PersistentPreRunE, banner, doctor/bug-report call sites)
- `cross_repo_commands.go` (commit confirmation; `--yes` flag unchanged)
- `implement_commands.go` (split `--autonomous` into `--auto` + `--headless`)
- `internal/configinit/prompt.go` (init walk)
- `internal/config/config.go` (Proceed?)
- `internal/steps/project.go` (Add missing statuses?)
- `internal/atdd/runtime/agents/registry.go` (humanStop)
- `internal/atdd/runtime/driver/driver.go` (three approve sites; split `Options.Autonomous` → `SkipHumanSTOP` + `Headless`)
- `internal/atdd/runtime/release/release.go` (release confirmer)
- `internal/atdd/runtime/clauderun/` (child env propagation — set `GH_OPTIVEM_AUTO`/`GH_OPTIVEM_CONFIRM`)
- `internal/atdd/runtime/statemachine/process-flow.yaml` (per-node `category:` on approve nodes that wrap `fix-*` dispatch)
- `internal/atdd/runtime/statemachine/` (parser support for `category:` field on RawNode)
- `README.md` (Auto-approve subsection)
