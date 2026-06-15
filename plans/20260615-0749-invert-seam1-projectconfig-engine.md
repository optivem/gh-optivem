# 2026-06-15 07:49:00 UTC — Invert seam #1: untangle `projectconfig → statemachine`

> **Child plan of** `20260615-0548-gh-optivem-modular-monolith-parent.md` (seam #1 / child #6).
> Design work, not a mechanical move. **Prerequisite for Child 1** (the engine ↔ process carve-out).

## TL;DR

**Why:** `internal/projectconfig` — a near-kernel domain type imported by almost everything — reaches *up* into the engine (`internal/atdd/runtime/statemachine`). It's the only backwards edge in the codebase and the biggest surprise in the inventory. The engine core (`statemachine`) is otherwise a clean leaf; this single import is what blocks both a clean engine carve-out (Child 1) and demoting `projectconfig` into the shared kernel (seam #5).

**End result:** `projectconfig` imports nothing from `internal/atdd/**` — it becomes a pure leaf, kernel-eligible. The one engine-dependent validation rule (task-prompts keys must be known MID task names) is relocated to the process/runtime layer, which legitimately already depends on *both* `statemachine` and `projectconfig`. CLI behavior is unchanged: a typo'd `task-prompts:` key still fails config load with the same error.

## Outcomes

What we get out of this:

- **`projectconfig` has zero upward dependencies** — `grep -r "internal/atdd" internal/projectconfig/` returns nothing (the only current hit, `config.go:37`, is gone). Its sole internal import today *is* `statemachine`; after this, it's a leaf.
- **The engine core stays a clean leaf** — `statemachine` still does not import `projectconfig` (no cycle is introduced in the other direction).
- **`projectconfig` becomes kernel-eligible** — seam #5 ("`projectconfig` can't be demoted to kernel until #1 is resolved") is unblocked as a side effect.
- **No CLI behavior change** — a typo'd `task-prompts:` key is still rejected at config load with the same `"%q is not a known embedded MID task"` error; the value-path check is unaffected.
- **Child 1 is unblocked** — the engine ↔ process carve-out no longer has a config→engine edge to drag along.
- **Regression-safe, verified** — `go build ./...` + `go test ./...` green, plus a guard proving the import is gone.

## The seam, precisely (grounded in code)

- The **only** real coupling: `internal/projectconfig/config.go` imports `statemachine` and uses it in exactly one place — `knownTaskNames()` (`config.go:1015`), which calls `statemachine.LoadDefault()` and walks `eng.Processes[].Nodes[]` for `EXECUTE_AGENT` call-activities to collect declared `task-name` verbs.
- `knownTaskNames()` has **one caller**: `Config.Validate()` Rule 11 (`config.go:672`), the `task-prompts:` validation — it rejects keys that aren't known embedded MID task names, then path-validates the values.
- `Validate()` runs inside `parse()` (used by `Load`/`LoadFromPath`) and inside `Marshal()`/`WriteToPath()`/`Write()`. So the engine load fires on every config read/write — but only when `len(c.TaskPrompts) > 0`.
- The three `statemachine` references in `paths_defaults.go` are **comments only** (doc strings citing the YAML path) — no import; nothing to change there functionally.
- **Direction check:** `statemachine` does *not* import `projectconfig`. The wider process layer (`driver`, `actions`, `preflight`, `repolocator`, `tracker/factory`, …) *does* import `projectconfig` — i.e. the legitimate direction is `process → {statemachine, projectconfig}`. This is why the relocated rule belongs in the process layer, **not** registered back into the engine core.
- **Affected tests:** `TestValidate_TaskPrompts_RejectsUnknownTask` (config_test.go:423) exercises the engine-backed check and moves with the rule. `TestValidate_TaskPrompts_RejectsAbsolutePath` (config_test.go:442) is pure path validation and stays. The round-trip test (config_test.go:364) uses valid task names and is unaffected.

## Step-2 audit — RESOLVED (config-load call sites)

Traced every external caller of `projectconfig.Load` / `LoadFromPath` / `Marshal` / `Write*`. **Key finding: there is no single config-load chokepoint** — loads are scattered across ~10 sites; the two shared root helpers (`loadProjectConfigForRunner` in `runner_helpers.go`, `loadProjectConfigForInit` in `main.go`) cover only some, while `process`/`implement`/`compile`/`config`/`driver` each call `projectconfig.LoadFromPath` directly. This is why the relocated check is wired via **load-wrappers** (below), not by editing one funnel.

**ENFORCE** (operator config that's validated or consumed by the runtime — a typo'd `task-prompts:` key must still fail):

| Site | Why |
|---|---|
| `config_commands.go:198` (`config validate`) | its entire purpose is to validate the config — the primary enforcer |
| `config_commands.go:570` (`config preflight`) | runs the same schema validation before preflight |
| `compile_commands.go:88` (`compile`) | runtime consumer |
| `process_commands.go:118` (`process`) | runtime consumer |
| `implement_commands.go:289` (`implement`) | runtime consumer |
| `driver.go:556` & `:562` (runtime `driver`) | the runtime orchestrator |
| `runner_helpers.go:92` (`loadProjectConfigForRunner`, used by `run`/`test`) | runtime consumer |
| `main.go:270` & `:282` (`loadProjectConfigForInit`) | produces the cfg that flows into runtime commands |

**DO NOT ENFORCE** (no task-prompts consumption / config is being generated, not consumed):

| Site | Why |
|---|---|
| `devworkflow/workspace.go:406` | cross-repo workspace ops; never touches task-prompts |
| `configinit/ensure.go:45` (`ensureExistsOrBuild`) | shared ensure+load helper — kept pure; its enforcing callers (config validate/preflight, main init) run the check themselves |
| `scaffolding/steps/optivem_yaml.go` (`Write`/`WriteToPath`/`Marshal`) and `steps/project.go:61–62` | scaffolding *generates* config; generated files never author `task-prompts:` |

**Accepted behavior delta (verified narrow):** after Option B, `projectconfig.Marshal`/`Write*` no longer name-check task-prompts (the check left `Validate`). Safe because no write path authors `task-prompts:` — scaffolding generates from flags (owner/repo/arch/…), and `config init` builds from `RawFlags`. The *value* path-validation on task-prompts stays in `Validate`, so Write still rejects a bad path.

## Design decision — how to invert (DECIDED: Option B)

**Chosen: Option B — relocate the rule.** The known-name check enforces *engine* knowledge ("what are the valid MID task names?"), so the real defect isn't the import direction — it's that a leaf domain type owns a rule it has no business owning. B removes that defect rather than abstracting over it: the rule moves to the process layer (which legitimately imports both `statemachine` and `projectconfig`), and `projectconfig` goes back to validating only its own shape.

Mechanically: remove `knownTaskNames()` + the Rule 11 known-name check from `Config.Validate()`; add `ValidateTaskPrompts` in a new `internal/atdd/runtime/configcheck` package (`LoadDefault` + walk + check). Because there's no single load chokepoint (see Step-2 audit), `configcheck` also exposes **load-wrappers** — `LoadFromPath(path)` and `Load(repoPath)` = the `projectconfig` load + `ValidateTaskPrompts` — and each ENFORCE call site swaps `projectconfig.LoadFromPath` → `configcheck.LoadFromPath`. DO-NOT-ENFORCE sites keep calling `projectconfig` directly, so the enforce/skip decision is explicit in code (who imports `configcheck` vs `projectconfig`). `projectconfig` keeps the *value* path-validation of `task-prompts`.

Layering holds: `configcheck` imports `statemachine` + `projectconfig` (both downward); the CLI-surface callers (`main.go`, `*_commands.go`, `runner_helpers.go`) and the runtime `driver` may all import `configcheck` (downward).

**Home package decided: `internal/atdd/runtime/configcheck`.** A dedicated package is importable by all three enforcers (`config` cmd, `compile` cmd, runtime `driver`) without any depending on each other. Folding the check into the `config`/`compile` command files was rejected: the `driver` is a library package, not a command, so it would have to either duplicate the rule or import the CLI-surface package — a fresh backwards dependency. `configcheck` sits where the rule's two deps already live and matches the existing small-focused-package shape of `internal/atdd/runtime/*`.

- **Why not C** (inject a `TaskCatalog` interface into `Validate`): it makes `projectconfig` a leaf too, but only by adding an interface + injection plumbing on the widely-called `Load`/`Validate` signatures to *preserve* config's role as the orchestrator of an engine-derived rule. Pays machinery to keep a questionable responsibility in place. Its one genuine edge — the check stays automatic on every `Load`, impossible to forget — is small here because the entry points are few (closed by the Step 2 audit).
- **Why not D** (package-level `SetTaskCatalog` hook): same as C but with global mutable state / service-locator — nil in unit tests or partial linkings silently skips the check. Rejected.

**Accepted cost of B:** the check is no longer automatic inside plain `Load`; it must be wired into the entry points that matter. Step 2's audit makes that wiring contract explicit, and Step 6's guard prevents the import from silently returning.

## Steps (coarse — sharpen during discussion)

- [ ] Step 1: **Create the home package** `internal/atdd/runtime/configcheck` exposing `ValidateTaskPrompts(cfg *projectconfig.Config) error` plus load-wrappers `LoadFromPath(path)` and `Load(repoPath)` (= the `projectconfig` load + `ValidateTaskPrompts`). It imports `statemachine` + `projectconfig` (both downward) and is the single home for the relocated rule.
- [x] Step 2: **Audit config-load entry points — DONE.** See "Step-2 audit — RESOLVED" above for the full call-site table, ENFORCE/skip classification, and the verified Marshal/Write behavior delta.
- [ ] Step 3: **Relocate the rule.** Move `knownTaskNames()` (config.go:1015) + the Rule 11 known-name branch (config.go:671–679) into `configcheck.ValidateTaskPrompts` (does `LoadDefault` + walk + known-name check). Keep the `task-prompts` *value* path-validation (the `validatePath("task-prompts."+name, …)` branch) in `projectconfig.Validate()`. Delete the `statemachine` import from `config.go:37`. Preserve the exact error string `"config: task-prompts: %q is not a known embedded MID task"`.
- [ ] Step 4: **Swap the ENFORCE call sites** (the 8 rows in the audit table) from `projectconfig.LoadFromPath`/`Load` → `configcheck.LoadFromPath`/`Load`, so behavior at those entry points is unchanged. Leave the DO-NOT-ENFORCE sites on `projectconfig` directly.
- [ ] Step 5: **Migrate tests.** Move `TestValidate_TaskPrompts_RejectsUnknownTask` (config_test.go:423) into `configcheck` against `ValidateTaskPrompts`; leave `TestValidate_TaskPrompts_RejectsAbsolutePath` (config_test.go:442) and the round-trip test (config_test.go:364) in `projectconfig`; add an end-to-end test proving an enforcing entry point (e.g. `config validate`) still rejects a typo'd key.
- [ ] Step 6: **Verify & guard.** `go build ./...` + `go test ./...` green; assert `grep -r "internal/atdd" internal/projectconfig/` is empty; add a depguard/lint rule (or a guard test) forbidding `internal/projectconfig` → `internal/atdd/**` so the edge can't silently return.
- [ ] Step 7: **Update the parent plan** — mark seam #1 / child #6 resolved and note `projectconfig` is now kernel-eligible (seam #5 unblocked).

## Open questions

**None — all resolved before execution.**

- Mechanism = **Option B** (relocate the rule).
- Home package = **`internal/atdd/runtime/configcheck`** (with `ValidateTaskPrompts` + load-wrappers).
- Enforcement set = the **8 ENFORCE call sites** in the Step-2 audit table (`config validate`/`preflight`, `compile`, `process`, `implement`, `driver`, `run`/`test` helper, `init` resolver); the 3 DO-NOT-ENFORCE sites stay on `projectconfig` directly.
- Marshal/Write name-check delta = **accepted** (verified narrow — no write path authors `task-prompts:`).

Nothing is left to discover at execution time. Step 6's import guard backstops the one residual risk (a future load site silently skipping the check).
