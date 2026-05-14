# Decouple `academy` and `shop` from `gh-optivem` tool surfaces

> ⚠️ **STATUS: NEEDS HUMAN DECISIONS.** This plan does not execute anything. It catalogs every `academy` and `shop` reference in the repo, classifies which ones leak organisation- or template-specific names into surfaces the tool exposes to *any* operator, and proposes a per-bucket cleanup. The "Decisions needed" section lists the questions a human must answer before any execution begins. Do not start work without explicit go-ahead per item.

## Context

Triggered by a leak found while writing `plans/20260514-2043-tbd-discipline-in-workspace-tool.md`: the workspace command descriptions and one comment in `workspace_commands.go` said "academy `*.code-workspace`" / "Operate on every repo in the academy workspace", even though the tool itself accepts any `*.code-workspace` file via its cascade (`--workspace > $GH_OPTIVEM_WORKSPACE > walk up from CWD`). Those four leaks have been fixed in place; this plan is the systematic follow-up.

The rule comes from memory `feedback_no_scaffold_repo_coupling.md`:

> gh-optivem workspace/cleanup code, `--help` Examples, error hints, and test fixtures must use `myorg/myrepo` placeholders, never `shop` / `greeter-*` / `eshop-*`.

The two terms have *very* different scopes:

- **`academy`** is the user's *organisation choice*. It is not a property of the tool. Every operator's gh-optivem could be pointed at a different org's workspace. Every `academy` reference in tool-facing surfaces is a coupling.
- **`shop`** is the *actual repo name* of the upstream template (`optivem/shop`). The `--shop-ref` flag, `ShopRef` config field, `CloneShopTemplate` step, scaffold-replacement tests that exercise the rewrite logic, and the ATDD/teaching docs that document the template all legitimately say `shop`. The rule applies only to surfaces where the *operator's* repo is what's being shown — `--help` Examples, README usage, error/test fixtures.

## Cross-reference

- Builds on the fix done in `plans/20260514-2043-tbd-discipline-in-workspace-tool.md` (which covers the four `academy` leaks in `workspace_commands.go` and that plan's own command descriptions).
- Memory note: `feedback_no_scaffold_repo_coupling.md`.

## Survey — `academy`

Outside the already-fixed `workspace_commands.go` entries, `academy` appears in:

### Tool surfaces — fix

| Location | Form | Why it's a leak |
|---|---|---|
| `main.go:111` | `Short: "Scaffold and operate Optivem academy pipeline projects"` | **Highest priority.** Top-level command Short string, shown by `gh optivem --help`. Couples the tool to one org. |
| `README.md:248` | "iterates every repo declared in the academy `*.code-workspace` file" | User-facing doc; same shape as the workspace_commands.go leak that was just fixed. |
| `internal/workspace/workspace.go:122` | error message: "set `--workspace`, `$%s`, or cd into the academy tree" | Operator-facing error hint; assumes "the academy tree" is where they live. |
| `internal/workspace/workspace.go:1, 4` | package-doc comment: "resolves the academy workspace root", "academy/github-utils/scripts/common.sh" | Mostly stylistic, but the package docstring shows up in `go doc`. |
| `scripts/cleanup-orphans.sh:38` | help text: `--tmp-dir <path>      Local orphan dir (default: <academy>/.tmp)` | Operator-facing `--help`. |
| `scripts/manual-test-runner-shop.sh:17` | comment: "Requires: …the optivem academy workspace cloned alongside" | Operator-facing setup instruction. |
| `internal/sonar/sonar.go:3`, `internal/ghbulk/ghbulk.go:2` | comments referencing `academy/github-utils/scripts/...` | Historical "ported from" comments; not strictly user-facing but reinforce the coupling story. |
| `internal/atdd/runtime/actions/bindings.go:74`, `internal/atdd/runtime/release/release.go:331, 350`, `internal/atdd/runtime/testselect/tracer.go:344` | comments referencing "the academy's compile-all.sh", "the academy convention", etc. | Same as above — internal comments only, but they bake the assumption in. |

### Test fixtures — bucketed separately (decision 2)

| Location | Form | Notes |
|---|---|---|
| `internal/workspace/workspace_test.go` (~14 hits) | `setupAcademy()` helper, `academy.code-workspace` filename in test data | Tests for the workspace resolver. The literal filename is fine if the resolver doesn't care about names; only the helper *name* and the in-source label matter. |

### Legitimate — keep

| Location | Form | Why keep |
|---|---|---|
| `BACKLOG.md:5` | URL pointing at `optivem/academy/blob/main/courses/...` | This is a literal link to a real repo for context; renaming would break it. |
| `.claude/agents/workflow-auditor.md` | agent's own description of where it operates | This file describes my agent setup; not part of the tool's surface. |
| `plans/deferred/...` | historical plan files | Plans are point-in-time records; don't rewrite them. |

## Survey — `shop`

`shop` appears in ~105 files. Almost all of them are legitimate. Only the following categories should change.

### `--help` Examples and operator-facing docs — fix

| Location | Form | Notes |
|---|---|---|
| `README.md:252` | `gh optivem workspace commit --repo shop "Fix bug"` | Workspace command has no relationship to the template; this should be `--repo myrepo`. |
| `README.md:268` | `gh optivem cleanup packages optivem/shop --before-date 2026-01-01` | Cleanup has no template coupling; should be `myorg/myrepo`. |
| `README.md:282` | `gh optivem implement --issue https://github.com/optivem/shop/issues/42` | Implement works against the *operator's* scaffolded repo, not the template. |
| `config_commands.go:94` | `--help` example: "Write to a non-default filename (shop's multi-combination matrix)" | The reason it's non-canonical isn't shop-specific; the example should stand on its own. |
| `config_commands.go:163, 224` | `--help` examples: `gh-optivem.shop-monolith.yaml` | Filename in examples — should be `gh-optivem.myrepo-monolith.yaml` or similar. |
| `implement_commands.go:74` | `--help` example: `https://github.com/optivem/shop/issues/42` | Should be `https://github.com/myorg/myrepo/issues/42`. |

### Test fixtures — fix (per memory rule)

| Location | Form | Notes |
|---|---|---|
| `cleanup_commands_test.go:15-20` | Repo-arg-format validation cases use `optivem/shop`, `optivem/eshop-tests`, etc. | These tests check the *format* of a repo argument; the literal repo name is irrelevant. Replace with `myorg/myrepo` placeholders per memory rule. |
| `implement_commands_test.go:25-30` | URL-parser test cases use `optivem/shop/issues/61` etc. | Same shape as above — parsing tests. |
| `workspace_commands_test.go:149-152` | `repoBaseName` test cases use `shop` as the name | Same. |

### Legitimate — keep

- **`--shop-ref` flag, `ShopRef` config field, `CloneShopTemplate` step, `main.go:151, 311, 312, 342, 817`** — these refer to the actual template repo by its actual name. Renaming the flag would be a much bigger plan, and the flag's job is literally "pick a ref of `optivem/shop`."
- **`internal/steps/replacements*.go`** and the runtime ATDD test data — the rewrite logic exists *because* the template ships with `shop` in source. Tests must use `shop` as input to exercise the rewrite.
- **`internal/projectconfig/config_test.go`, `internal/runner/config_test.go`, `internal/atdd/runtime/{driver,board,classify,preflight,repolocator,gates,clauderun}_test.go`** — uses `optivem/shop` as the canonical "real-world example" for ATDD tests of the live pipeline. This is template-rewrite-adjacent and is checking real-world parsing. **Decision 3** asks whether to bring these inside the placeholder rule too.
- **`CONTRIBUTING.md`** — contributor walkthrough that literally describes "go to `../shop`, run rehearsal there." The shop checkout *is* the rehearsal target; the walkthrough is unintelligible without naming it.
- **`docs/gh-monitoring-process.md`** — describes how to investigate failures across "clone + shop + gh-optivem"; shop is one of three named real repos.
- **`MAPPING.md`, `NAMING.md`** — explicitly document the shop→scaffolded-repo rewrite mapping. The whole point of the docs is to name shop.
- **`CLAUDE.md`** — single reference to "the `shop` template" in the no-Pages rule; correct.
- **`BACKLOG.md`** — single passing reference; fine.
- **`scripts/manual-test-runner-shop.sh`** — the filename itself indicates it's the test rig that exercises the shop template. Renaming would be misleading.

## Proposed work

Three layers, smallest first. None of these is large; the whole plan should be one or two PRs total.

### Layer 1 — `academy` cleanup (small, mechanical)

1. Replace the `academy` leaks listed in the "Tool surfaces — fix" table above with neutral phrasing:
   - `main.go:111` — `Short: "Scaffold and operate Optivem pipeline projects"` *or* `"Scaffold and operate gh-optivem pipeline projects"` (decision 1).
   - `README.md:248`, `internal/workspace/workspace.go:1, 4, 122` — "the resolved `*.code-workspace` file" / "the workspace tree".
   - `internal/sonar/sonar.go:3`, `internal/ghbulk/ghbulk.go:2`, `internal/atdd/runtime/{actions/bindings.go, release/release.go, testselect/tracer.go}` — drop `academy/` from the path references in comments, or restate as "the upstream bash scripts" / "the pipeline convention".
   - `scripts/cleanup-orphans.sh:38`, `scripts/manual-test-runner-shop.sh:17` — neutral phrasing.

### Layer 2 — `shop` cleanup in tool surfaces (small, targeted)

2. Rewrite `--help` Examples and README usage snippets listed in the "fix" table to use `myorg/myrepo` placeholders. Specifically:
   - `README.md:252, 268, 282`
   - `config_commands.go:94, 163, 224`
   - `implement_commands.go:74`
3. Rewrite the unit-test fixtures that test repo-format / URL-parsing / base-name extraction to use placeholders:
   - `cleanup_commands_test.go:15-20`
   - `implement_commands_test.go:25-30`
   - `workspace_commands_test.go:149-152`

### Layer 3 — drift prevention (optional)

4. A simple grep-based lint that runs in CI and flags any new occurrence of `academy` or `shop` in a configurable set of files (the "tool surfaces" set). Allowlist explicitly enumerates the legitimate places (the `--shop-ref` flag, `MAPPING.md`, `NAMING.md`, ATDD test fixtures, etc.). Cheap to write; expensive to forget.

## Decisions needed (human)

Each one is genuinely open — do not pick a default.

1. **`main.go:111` `Short` wording.** Two natural replacements: (a) `"Scaffold and operate Optivem pipeline projects"` — keeps "Optivem" as the publisher; (b) `"Scaffold and operate gh-optivem pipeline projects"` — purely tool-named. Which one?
2. **Test-helper rename.** `internal/workspace/workspace_test.go` defines `setupAcademy()` and uses `academy.code-workspace` as the filename. Rename the helper to `setupWorkspace()` and use a neutral filename like `myworkspace.code-workspace`, or leave the test internals alone since they're never user-visible?
3. **ATDD test fixtures using `optivem/shop`.** The `internal/atdd/runtime/**_test.go` files use `optivem/shop` as the canonical real-world example. Per a strict reading of the memory rule, "test fixtures" should be placeholders. Per a strict reading of *what these tests actually exercise*, `optivem/shop` is the real input the system runs against. Bring them inside the placeholder rule (Layer 2 extended), or leave them as live-system fixtures (Layer 2 narrow as written)?
4. **`internal/workspace/workspace.go:122` error wording.** The current "or cd into the academy tree" is friendly but leaky. Replace with something terse like "or cd into a directory below a `*.code-workspace` file," or keep a friendly-but-generic phrasing like "or cd into your workspace tree"?
5. **Layer 3 lint adoption.** Build the grep-based lint now, or defer until the next time a leak gets noticed?
6. **Scope of `academy/github-utils/scripts/` comments.** These reference a real path on disk in another repo. Strip the prefix and just say `github-utils/scripts/`, or strip the whole reference since the Go ports superseded them anyway?

## Out of scope

- Renaming the `--shop-ref` flag, the `ShopRef` config field, or any of the `CloneShopTemplate` / `ValidateNoLeftoverShopRefs` step names. Those would be a *much* bigger plan: contract-breaking flag changes, downstream rehearsal/scripts updates, possible release-binary backwards-compat shims.
- Renaming the upstream `optivem/shop` template repo itself.
- Rewriting `CONTRIBUTING.md`, `MAPPING.md`, `NAMING.md`, `docs/gh-monitoring-process.md`, `CLAUDE.md`, or `BACKLOG.md` — they all name `shop` for good reason (teaching content, mapping docs, monitoring process docs that reference real repos).
- Plans under `plans/deferred/` — they are point-in-time records.
- The four `academy` leaks in `workspace_commands.go` and `plans/20260514-2043-...md` — already fixed.

## References

- `feedback_no_scaffold_repo_coupling.md` — the canonical rule.
- `plans/20260514-2043-tbd-discipline-in-workspace-tool.md` — the plan whose drafting surfaced the original `academy` leak.
- Surveyed files: `git grep -ni academy`, `git grep -nw shop`.
