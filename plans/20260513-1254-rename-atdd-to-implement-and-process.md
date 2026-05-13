# Rename `atdd` subcommands → `implement` (verb-first) and `process show` (noun-first)

> 🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-05-13T11:19:11Z`

> ⚠️ **Needs explicit human approval before implementation. Discuss first.**
> This plan is a draft. Do not execute any step until the author signs off
> on the overall shape (and the open questions in the final section).

> **Relationship to noun-first CLI rename:** Two existing plans reference a
> future `20260511-2000-noun-first-cli-rename.md` (e.g. `build system` →
> `system build`). That plan file does not yet exist. The rename in *this*
> plan partially overlaps with that effort (`process show` is noun-first),
> but keeps `optivem implement` top-level because the methodology landscape
> is the variable part, not the verb. See Open Questions for how to
> reconcile.

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
- Migrating the *other* verb-first commands (`build system`, `test
  system`, …) to noun-first. That's the separate-and-not-yet-written
  `20260511-2000-noun-first-cli-rename.md` plan.
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
    newAtddCmd(),       // see Step 4 — keep, hide, or delete
    newImplementCmd(),  // new
    newProcessCmd(),    // new
    newEnvironmentCmd(),
    newUpgradeCmd(),
)
```

### Step 4 — Decide fate of `atdd …` parent (see Open Questions #1)

Three options, listed in increasing breakage:

- **A. Keep as hidden aliases.** Add `Hidden: true` to `newAtddCmd()`
  and leave the three subcommands as thin shims that call into the new
  command paths. Both old and new forms work; help text only advertises
  the new forms. Lowest friction.
- **B. Delete outright.** Remove `newAtddCmd` and its three children;
  rip the `atdd_commands.go` file. Any caller (scripts, CI, docs) using
  the old form breaks loudly. Cleanest, most disruptive.
- **C. Keep with a deprecation warning.** Old commands still work but
  print `WARN: 'gh optivem atdd implement-ticket' is deprecated; use
  'gh optivem implement --issue N'` to stderr before running.

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

### Step 7 — Update the forward-referencing plans

The two existing plans that reference `20260511-2000-noun-first-cli-rename.md`
should be updated to also point at *this* plan (or vice versa, depending
on how Open Question #4 is resolved):

- `plans/20260505-220100-verify-runs-from-wrong-cwd.md:3–5`
- `plans/20260511-1418-gh-optivem-test-disable-enable-subcommand.md:7–9`

## Open questions for discussion

1. **Old-command fate.** Step 4 options A / B / C — keep hidden, delete
   outright, or deprecate with warning? Author preference?
2. **Workspace preflight on `manage-project` path.** Today
   `implement-ticket` runs `runImplementTicketPreflight` (workspace +
   on-disk layout) but `manage-project` does not (see
   `atdd_commands.go:217+` vs `:117+`). After the merge into one
   `implement` command, should preflight always run, or only when
   `--issue` is provided? Recommendation: always run — picking the top
   Ready item still needs the workspace to exist.
3. **Relationship to `20260511-2000-noun-first-cli-rename.md`.** Two
   existing plans reference that file but it doesn't exist. Options:
   - **Merge:** Fold this plan into the broader noun-first rename when
     it's written, so all renames ship together.
   - **Sequence:** Land this plan first (it's the only one with a clear
     trigger — TDD landing soon), and let the broader rename pick it up
     by reference later.
   - **Independent:** Treat this as a one-off (verb-first for the
     headline action, noun-first for introspection) and never write the
     broader rename. The noun-first hint may be stale.
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
