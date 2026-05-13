# Rename `atdd` subcommands → `implement` (verb-first) and `process show` (noun-first)

> 🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-05-13T11:25:43Z`

> ⚠️ **Needs explicit human approval before implementation. Discuss first.**
> This plan is a draft. Do not execute any step until the author signs off
> on the overall shape (and the open questions in the final section).

> **Relationship to the existing noun-first CLI convention:** The broader
> noun-first rename already shipped (`0360402 rename: gh optivem CLI to
> noun-first` → `19e8fa5 drop verb-first deprecation aliases (v1.6.0)` →
> `5d89fa6 plan: delete noun-first CLI rename plan (work done)`). Today
> the noun groups `system` / `test` / `config` / `environment` are
> established; the only verb-first / methodology-first holdout is
> `atdd …`. This plan finishes that migration: `process show` continues
> the existing noun-first convention, and `implement` is the deliberate
> top-level verb exception (the methodology is config, not a noun).
> Two older plans still contain dangling references to the deleted
> `20260511-2000-noun-first-cli-rename.md`; Step 7 cleans those up.

## Context

Today the ATDD pipeline lives under a methodology-named namespace:

```
gh optivem atdd implement-ticket --issue 42        # specific issue
gh optivem atdd manage-project                     # pick top Ready
gh optivem atdd show diagram                       # print process-flow Mermaid
```

Source:
- `atdd_commands.go:40` `newAtddCmd` — parent
- `atdd_commands.go:97` `newAtddImplementTicketCmd`
- `atdd_commands.go:217` `newAtddManageProjectCmd`
- `atdd_commands.go:66` `newAtddShowCmd` / `atdd_commands.go:78` `newAtddShowDiagramCmd`
- `main.go:100` registers `newAtddCmd()` on the root

### Why rename now

1. **TDD (and possibly DDD) are coming.** Once a second methodology lands,
   the `atdd` namespace stops being the obvious home for "run the
   pipeline" — every methodology would need its own clone of
   `implement-ticket` / `manage-project`.
2. **Implementing is the stable concept.** "Implement an issue" is the
   noun-verb pair that doesn't change between methodologies; *which*
   methodology runs is configuration, not a top-level command. (Discussed
   with author 2026-05-13.)
3. **`manage-project` is a poor name.** It hides what it actually does
   (pick the top Ready issue and run the same pipeline as
   `implement-ticket`) and doesn't pair cleanly with `implement-ticket`.
   Today the only difference between the two commands is whether
   `--issue` was passed.
4. **`atdd show` is methodology-coupled too.** The artifact it prints
   (process-flow diagram) is conceptually a *process*, not specifically
   *ATDD's process*. Grouping it under the stable noun `process` ages
   better and gives a home for future introspection verbs (`process
   validate`, `process list`, …).

## Design (per author's confirmations 2026-05-13)

### Top-level shape

```
gh optivem implement                                     # pick top Ready issue
gh optivem implement --issue 42                          # specific issue
gh optivem implement --issue https://github.com/optivem/shop/issues/42
gh optivem process show                                  # print Mermaid diagram
gh optivem process show > docs/process-diagram.md
```

- **`implement`** absorbs both `implement-ticket` and `manage-project`.
  No `--issue` flag → pipeline starts at `PICK_TOP_READY` (current
  `manage-project` behavior). With `--issue N` (or a URL) → pipeline
  skips `PICK_TOP_READY` and starts at `MOVE_TICKET_IN_PROGRESS` (current
  `implement-ticket` behavior).
- **`--issue`** is the universal noun across GitHub, Jira, GitLab,
  Linear. (Confirmed: existing flag already uses `--issue`; muscle memory
  preserved.)
- **`process show`** (3 levels, pure noun-verb) replaces
  `atdd show diagram`. Noun-first because the artifact belongs to *the
  process*, not to *the methodology*. Today the diagram is the only
  artifact, so no fourth level is needed — matches the mainstream CLI
  convention (`terraform show`, `git show`, `kubectl get <resource>`) of
  capping routine commands at three levels and pushing variants to flags
  or noun arguments only when there is a real second artifact.
- **No `--methodology` flag** for now. The methodology is read from
  config (current behavior — `process_flow:` in `gh-optivem.yaml`).
  When TDD lands, we revisit; if `atdd+tdd` composition is needed, a
  flag is the natural escape hatch.

### Out of scope (deferred)

- A `--methodology` flag on `implement` (composition: atdd, atdd+tdd,
  atdd+tdd+ddd). Author explicitly deferred 2026-05-13.
- Migrating the *other* verb-first commands. Already done — the
  broader noun-first rename shipped in v1.6.0 (commits 0360402 and
  19e8fa5). `atdd` is the only remaining holdout, which is what this
  plan finishes.
- Deprecating the old `atdd …` names. See Open Questions #1 — author
  hasn't decided whether to keep them as hidden aliases.

## Steps

### Step 1 — Add new top-level `implement` command

Create `implement_commands.go` (next to `atdd_commands.go`):

```go
func newImplementCmd() *cobra.Command {
    var (
        issueArg     string
        autonomous   bool
        manualAgents bool
        logFile      string
        workspace    string
        keepRuns     int
        showPrompt   bool
    )
    cmd := &cobra.Command{
        Use:   "implement",
        Short: "Run the configured implementation pipeline on an issue",
        Long: `Run the implementation pipeline (currently ATDD; future:
TDD, DDD, or compositions) against a GitHub issue. With --issue, the
pipeline targets that specific issue. Without --issue, it picks the top
Ready item from the project board.`,
        Example: `  gh optivem implement                                  # top Ready
  gh optivem implement --issue 42
  gh optivem implement --issue https://github.com/optivem/shop/issues/42
  gh optivem implement --issue 42 --workspace /abs/path
  gh optivem implement --issue 42 --log-file run.log
  gh optivem implement --issue 42 --show-prompt
  gh optivem implement --issue 42 --keep-runs 0`,
        Run: func(cmd *cobra.Command, args []string) {
            // Branch on whether --issue was provided.
            // Empty issueArg → manage-project path
            // Non-empty       → implement-ticket path
            ...
        },
    }
    // Same flag set as today's atdd implement-ticket (issue is optional now).
    cmd.Flags().StringVar(&issueArg, "issue", "", "GitHub issue number or URL (optional; omit to pick the top Ready item)")
    cmd.Flags().BoolVar(&autonomous, "autonomous", false, "...")
    cmd.Flags().BoolVar(&manualAgents, "manual-agents", false, "...")
    cmd.Flags().StringVar(&workspace, "workspace", "", "...")
    cmd.Flags().StringVar(&logFile, "log-file", "", "...")
    cmd.Flags().IntVar(&keepRuns, "keep-runs", 10, "...")
    cmd.Flags().BoolVar(&showPrompt, "show-prompt", false, "...")
    return cmd
}
```

Implementation notes:
- The `Run` body is the union of today's two `Run` bodies. When
  `issueArg == ""`, skip `parseIssueArg` and `runImplementTicketPreflight`'s
  workspace check (today, `manage-project` does *not* run the workspace
  preflight; verify whether the new unified command should always run
  it — see Open Questions #2).
- Both `driver.Run` paths already accept `driver.Options{IssueNum: 0, …}`
  for the picker path, so the dispatch logic is one shared call.

### Step 2 — Add `process show` command

Create `process_commands.go`:

```go
func newProcessCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "process",
        Short: "Inspect the configured implementation process",
    }
    cmd.AddCommand(newProcessShowCmd())
    return cmd
}

func newProcessShowCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "show",
        Short: "Render the process-flow Mermaid diagram to stdout",
        Example: `  gh optivem process show
  gh optivem process show > docs/process-diagram.md`,
        Run: func(cmd *cobra.Command, args []string) {
            eng, err := statemachine.LoadDefault()
            exitOnError(err)
            fmt.Print(diagram.Render(eng))
        },
    }
}
```

`process show` is a leaf — three levels total (`optivem process show`),
no `diagram` child. The diagram render path is identical to today's
`newAtddShowDiagramCmd`; just reparented and one level shallower.

### Step 3 — Register new commands on root

`main.go:94`:

```go
cmd.AddCommand(
    newInitCmd(),
    newConfigCmd(),
    newSystemCmd(),
    newTestCmd(),
    newCompileCmd(),
    // newAtddCmd() removed — see Step 4
    newImplementCmd(),  // new
    newProcessCmd(),    // new
    newEnvironmentCmd(),
    newUpgradeCmd(),
)
```

### Step 4 — Delete the `atdd …` parent (Option B, resolved 2026-05-13)

Delete in this order:

1. Remove the `newAtddCmd()` line from `main.go`'s `cmd.AddCommand(...)`
   block (around `main.go:100`).
2. Delete `atdd_commands.go` entirely.
3. Delete `atdd_commands_test.go` entirely.
4. Verify `go build ./...` and `go vet ./...` are clean — any remaining
   in-package reference to `newAtddCmd` / `newAtddImplementTicketCmd` /
   `newAtddManageProjectCmd` / `newAtddShowCmd` / `newAtddShowDiagramCmd`
   will surface here.

No hidden aliases, no deprecation warning. Any caller (this repo's
scripts/docs/tests in Step 5/6, plus any sibling-repo CI calling
`gh optivem atdd …`) breaks loudly until updated. This is the
established repo convention for CLI renames — same as the prior
`19e8fa5 drop verb-first deprecation aliases` flip.

### Step 5 — Update callers

Grep for the old names and rewrite:

```
gh optivem atdd implement-ticket  →  gh optivem implement --issue ...
gh optivem atdd manage-project    →  gh optivem implement
gh optivem atdd show diagram      →  gh optivem process show
```

Known call sites (from a search 2026-05-13):
- `README.md:218–220, 233, 274, 311` — usage docs and ATDD overrides section
- `CONTRIBUTING.md:55, 123, 144, 173, 200–204, 239` — contributor workflow
- `scripts/atdd-rehearsal.sh` — rehearsal harness
- `internal/atdd/runtime/preflight/preflight.go` — error messages mention
  the command
- `internal/atdd/runtime/agents/prompts/atdd-chore.md` — agent prompt text
- `internal/atdd/runtime/driver/driver.go` — banners / error messages
- `internal/atdd/runtime/board/board.go` — error messages
- Possibly CI workflows in the academy that call `gh optivem atdd …`
  (grep across sibling repos — out of this plan's scope but flag for the
  author).

### Step 6 — Update tests

- `atdd_commands_test.go` — current tests assert on `Use: "atdd"` /
  `"implement-ticket"` / `"manage-project"`. Either:
  - Add parallel `implement_commands_test.go` for the new commands and
    leave the old test file in place (if Option A or C in Step 4), or
  - Replace it entirely (if Option B).
- Bindings tests in `internal/atdd/runtime/actions/bindings_test.go`
  don't reference the parent command name; should be untouched.
- End-to-end rehearsal: run `scripts/atdd-rehearsal.sh` after rewrite,
  confirm the new commands drive a full pipeline pass.

### Step 7 — Clean up dangling references to the deleted noun-first plan

Two existing plans still contain callouts referencing
`20260511-2000-noun-first-cli-rename.md`, which was deleted in commit
`5d89fa6` once its work shipped. The callouts are stale — they tell a
reader "see this file for the noun-first equivalents" when the file no
longer exists. The repo convention is to delete the plan when the work
is done, so the right fix is to remove the callouts entirely:

- `plans/20260505-220100-verify-runs-from-wrong-cwd.md:3–5`
- `plans/20260511-1418-gh-optivem-test-disable-enable-subcommand.md:7–9`

If those plans still contain pre-rename verb-first command examples in
the body, rewrite those examples to noun-first while we're in there
(the examples themselves are the only reason the callout existed).

## Open questions for discussion

1. **Old-command fate.** ✅ **Resolved 2026-05-13: Option B — delete
   outright.** Rip `atdd_commands.go` and `atdd_commands_test.go`. No
   hidden aliases, no deprecation warnings. Step 5's call-site sweep
   becomes mandatory (anything still referencing `gh optivem atdd …`
   after the rip will fail loudly).
2. **Workspace preflight on `manage-project` path.** Today
   `implement-ticket` runs `runImplementTicketPreflight` (workspace +
   on-disk layout) but `manage-project` does not (see
   `atdd_commands.go:217+` vs `:117+`). After the merge into one
   `implement` command, should preflight always run, or only when
   `--issue` is provided? Recommendation: always run — picking the top
   Ready item still needs the workspace to exist.
3. **~~Relationship to `20260511-2000-noun-first-cli-rename.md`.~~**
   ✅ **Resolved 2026-05-13: moot.** The broader noun-first rename
   already shipped (see intro). This plan finishes the last holdout;
   no coordination needed. Step 7 cleans up stale references in two
   older plans.
4. **`process` as a top-level noun.** Does `process` read naturally
   alongside the other top-level groups (`system`, `test`, `compile`,
   `config`, `environment`, `init`, `upgrade`)? Or is something like
   `pipeline` clearer? The state-machine YAML is literally
   `process-flow.yaml`, so `process` has source-of-truth alignment going
   for it.
5. **Short flag for `--issue`.** Worth adding `-i` for muscle memory,
   or keep the long form only to avoid clashing with future flags? (No
   current `-i` shorthand exists at the root level — `init -v` is the
   only documented short-flag collision concern, per `main.go:88–91`.)
