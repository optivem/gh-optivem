# atdd-rehearsal writes gh-optivem.yaml into the worktree

## Motivation

`scripts/atdd-rehearsal.sh` creates a worktree from the consumer repo (typically `shop/`) and runs `gh optivem atdd implement-ticket` inside it. That worktree inherits whatever the consumer repo has committed — and the shop template does **not** commit `gh-optivem.yaml`. That file is normally produced by `gh optivem init` (`internal/steps/optivem_yaml.go:WriteOptivemYAML`), and `implement-ticket` consumes it via `internal/projectconfig/config.go:Load` (`Path = "gh-optivem.yaml"`).

Result: the ATDD cycle cannot run inside the worktree today. `projectconfig.Load` returns `(nil, nil)` for the missing file, callers fall back to "no config", and anything that depends on project URL or scope axes (the cycle's prompt context) is empty. The rehearsal is supposed to exercise the real `implement-ticket` path — the path real users hit after running `init` — but without the YAML it's exercising a degenerate version of it.

The narrow goal: have the rehearsal script materialise `gh-optivem.yaml` inside the worktree before invoking `implement-ticket`, so the ATDD cycle runs with the same configuration shape a real init-scaffolded repo would have.

This is the *only* gap. We are not adding a fresh-scaffold rehearsal mode, not repurposing the script for students (the author runs it; students only see the resulting branch), not splitting it into two scripts. Worktree flow stays as-is.

## Approach

Introduce a `gh optivem config` parent subcommand following the convention of `git config`, `gh config`, `npm config`, `kubectl config`. The `gh-optivem.yaml` file is the central config of the tool ("It is the single config the gh-optivem binary reads", `internal/projectconfig/config.go:8-10`), so a `config` namespace is the right home for any operation that reads or writes it.

Land two subcommands in this PR:

- **`gh optivem config init`** — write a fresh `gh-optivem.yaml` from CLI flags. Used by the rehearsal script; also useful standalone for retrofitting the file into a non-scaffolded repo.
- **`gh optivem config validate`** — parse `<CWD>/gh-optivem.yaml` and run it through `projectconfig.Validate()`. Surfaces an existing-but-currently-unreachable capability: `Validate()` lives at `internal/projectconfig/config.go:107` with no CLI surface today, so anyone hand-editing the YAML has no way to check it before running `implement-ticket`.

Both subcommands wrap existing logic in `internal/projectconfig/` and `internal/steps/optivem_yaml.go`. No duplicated YAML construction.

(Considered: `config show` instead of/as well as `validate`. Rejected for first cut — `cat gh-optivem.yaml` already covers "look at the file"; `validate` answers the actionable green/red question that the file's structure invites. `show` can ship later if a real need surfaces.)

**Delegation: code-level, not CLI-level.** `steps.WriteOptivemYAML(cfg)` stays the single source of truth for "render config from a populated `*config.Config` and write it to disk". Both `gh optivem init` (already calls it at `main.go:222`) and the new `gh optivem config init` build a `*config.Config` from their respective flag sets and call the same function. Nothing forks/execs a child gh-optivem process — they share the Go function, not the CLI invocation.

The rehearsal script then:

1. (Existing) Build `gh-optivem.exe`, resolve id, `git worktree add`.
2. (New) Inside the worktree, run `gh optivem config init --owner ... --arch ... --monolith-lang ... --repo-strategy ... --project-url ...`.
3. (Existing) Run `gh optivem atdd implement-ticket --issue $ISSUE`.
4. (Existing) On exit, prompt to delete the worktree + branch.

## Items

### 1. Add `gh optivem config` parent subcommand

**File:** `main.go`

- Register `newConfigCmd()` in `newRootCmd`'s `cmd.AddCommand(...)` list (alongside the existing subcommands at line 83).
- `newConfigCmd()` is a stub `&cobra.Command{Use: "config", Short: "Manage gh-optivem.yaml in a consumer repo"}` with no Run — it just hosts the child commands. Cobra prints usage if invoked without a subcommand.
- Attach `newConfigInitCmd()` and `newConfigValidateCmd()` as children.

### 2. `gh optivem config init` subcommand

**Files:**
- `main.go` — `newConfigInitCmd()`.
- `internal/config/config.go` — factor the YAML-relevant flag definitions into a shared helper called by both `BindInitFlags` and the new `BindConfigInitFlags`, so adding a new YAML-affecting flag (e.g. `--frontend-lang`) flows to both surfaces automatically. The shared subset: `--owner`, `--repo`, `--arch`, `--repo-strategy`, `--monolith-lang`, `--backend-lang`, `--frontend-lang`, `--test-lang`, `--system-name` (if needed for repo slug derivation), `--project-url`. **Out:** verify-level, shop-ref, workdir, license, no-* flags, dry-run (no destructive ops to dry).
- Reuse `steps.WriteOptivemYAML` directly. The subcommand sets `cfg.RepoDir` to the CWD (or accepts an optional `--dir` for explicit targeting), then calls the same function `init` calls.

Behaviour:
- Writes `<CWD>/gh-optivem.yaml` (or `<--dir>/gh-optivem.yaml`).
- Refuses to overwrite an existing file unless `--force` is passed. Conventional fit for a "scaffold a whole file" command — matches `cargo init`, `dotnet new`, `helm create`. The file is the single source of truth for the tool, and it may be hand-edited; silent overwrite would be a foot-gun. (The rehearsal path never triggers this — fresh worktrees have no pre-existing YAML — so the refuse default doesn't add friction there.)
- Validates input the same way `init` does (lang/arch/repo-strategy enums + cross-field rules).
- No network, no GitHub, no SonarCloud — pure local file write.

### 3. `gh optivem config validate` subcommand

**File:** `main.go`

- `newConfigValidateCmd()` — no flags except an optional `--dir` (default CWD).
- Reads `<dir>/gh-optivem.yaml` via `projectconfig.Load`. `Load` already invokes `Validate` internally (`internal/projectconfig/config.go:190`), so a successful Load = valid file.
- Missing file: exit non-zero with a clear message ("no gh-optivem.yaml in <dir>; run `gh optivem config init` first").
- Present + valid file: print a short success message ("`<dir>/gh-optivem.yaml` is valid") and exit 0.
- Present + invalid: surface the validation error from `Load` (already wrapped with file path context) and exit non-zero.

### 4. Wire `gh optivem config init` into atdd-rehearsal.sh

**File:** `scripts/atdd-rehearsal.sh`

Add a clearly-marked config block near the top of the script (after the shebang/header comment, before any logic). Single source of truth for rehearsal values; greppable; no env-var indirection. Hardcoded `REHEARSAL_PROJECT_URL` because we always rehearse against the same GitHub Project.

```bash
# === REHEARSAL CONFIG === (edit these for your setup)
REHEARSAL_OWNER="..."
REHEARSAL_REPO="..."
REHEARSAL_ARCH="monolith"
REHEARSAL_REPO_STRATEGY="monorepo"
REHEARSAL_MONOLITH_LANG="java"
REHEARSAL_PROJECT_URL="..."
# === END REHEARSAL CONFIG ===
```

After the `git worktree add` block (~line 124) and before the `implement-ticket` invocation (~line 135), add the `config init` call followed by an explicit commit so the YAML lands on the rehearsal branch (visible to anyone viewing the branch on github.com):

```bash
log "Writing gh-optivem.yaml into worktree..."
( cd "$WORKTREE_PATH" && "$BIN" config init \
    --owner "$REHEARSAL_OWNER" \
    --repo "$REHEARSAL_REPO" \
    --arch "$REHEARSAL_ARCH" \
    --repo-strategy "$REHEARSAL_REPO_STRATEGY" \
    --monolith-lang "$REHEARSAL_MONOLITH_LANG" \
    --project-url "$REHEARSAL_PROJECT_URL" )

log "Committing gh-optivem.yaml to rehearsal branch..."
( cd "$WORKTREE_PATH" \
    && git add gh-optivem.yaml \
    && git commit -m "Add gh-optivem.yaml for rehearsal" )
```

### 5. Update help text and header comment

**File:** `scripts/atdd-rehearsal.sh`

The header workflow comment (currently lines 22-32) lists 5 steps. Insert a new step between worktree creation (step 3) and implement-ticket invocation (step 4):

> 3.5. Inside the new worktree, run `<gh-optivem>/gh-optivem.exe config init ...` to materialise `gh-optivem.yaml` (the shop template doesn't commit it; `implement-ticket` needs it to resolve project URL and scope axes).

Renumber subsequent steps. No `--help` flag changes needed (existing `<issue-num> [label]` usage stays).

### 6. Tests

**Files:** new tests alongside existing ones in `internal/steps/` and a `main_test.go` smoke test (or extend if one exists).

- `buildOptivemYAML` is already pure and covered (`internal/steps/optivem_yaml.go:53`). The new wrappers need a thin contract test: run `gh optivem config init` against a tempdir, assert the resulting YAML parses back through `projectconfig.Load` and matches the input flags.
- For `config validate`: three cases — (a) missing file: exit non-zero with "no gh-optivem.yaml" message; (b) valid file: exit 0; (c) invalid file (e.g. `repo_strategy: bogus`): exit non-zero with the validation error in the output.
- `--force` overwrite behaviour for `config init`: assert default refuses, `--force` succeeds.

## Out of scope

- **Refactoring `init`'s YAML-writing step to invoke the new subcommand.** The shared Go function (`WriteOptivemYAML`) already covers the "single source of truth" requirement. Re-routing through CLI invocation would just add fork/exec overhead and a less direct call graph.
- **Replacing the worktree flow with a full `gh optivem init` rehearsal.** Not what we're solving.
- **Student-facing UX changes** to the rehearsal script. The author runs it; students only see the resulting branch.
- **`config get` / `config set` / `config show` subcommands.** The namespace is open for these later; not needed now. (`show` is the most likely next addition if a real "what's in this file?" debugging need surfaces; `cat gh-optivem.yaml` covers it for now.)
- **Auto-detecting rehearsal values from the consumer repo.** The author knows the values; a config block in the script is fine.
- **`--config-only` mode for `init`.** Was an alternative; rejected once `config init` was on the table — `init --config-only` muddies init's contract while `config init` reads cleanly.

## Decisions (resolved 2026-05-04)

- **YAML gets committed to the rehearsal branch.** The script runs `git add gh-optivem.yaml && git commit -m "Add gh-optivem.yaml for rehearsal"` after `config init`, before `implement-ticket`. Reasons: (1) anyone viewing the branch on github.com sees a coherent history; a working-tree-only YAML is invisible remotely; (2) predictable regardless of what `implement-ticket` does with its own commits; (3) matches what real `gh optivem init` users get (the YAML lands in their initial scaffold commit at `main.go:251-254`).
- **Project-specific values live in a `# === REHEARSAL CONFIG ===` block at the top of the script.** Single greppable place to edit. Env vars rejected as friction without benefit for a single-author tool.
- **`--project-url` is hardcoded** in the rehearsal config block — we always rehearse against the same GitHub Project. If that ever changes, promote it to an env var or third script arg.
- **`config init` refuses to overwrite existing files; `--force` opts in.** Conventional for whole-file scaffolders (`cargo init`, `dotnet new`, `helm create`). Doesn't affect the rehearsal path (worktrees start without a YAML), only matters for manual `config init` invocations against already-configured repos.

## Order of operations

1. Land Items 1 + 2 + 3 (`config` parent + `init` + `validate`) together — they share the same wiring scaffold and ship as one coherent CLI surface.
2. Land Item 6 (tests) in the same PR as Items 1-3.
3. Land Items 4 + 5 (rehearsal script + header comment) together, after the binary is in.
4. Manual rehearsal: run the script end-to-end, verify the worktree has `gh-optivem.yaml` with correct contents committed, verify `implement-ticket` consumes it (project URL resolves), verify `gh optivem config validate` exits 0 against the generated file.
