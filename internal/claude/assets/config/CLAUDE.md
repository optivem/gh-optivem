# Claude Code Guidelines

## General Rules

- Never use hardcoded local paths (e.g. `C:\Users\...`). Always resolve paths dynamically (e.g. `git rev-parse --show-toplevel`, `$HOME`, environment variables). The user works from different computers.
- Never use `isolation: "worktree"` when spawning agents. Worktrees leave behind orphaned directories that block git operations. Always run agents in the default (non-isolated) mode.
- When presenting options or asking for decisions, always mark one as **recommended** and explain why in one sentence. Minimize the number of choices the user needs to make — default to the recommended option and proceed unless the user objects.

## Token Usage

**Minimize token usage is a top-level guiding principle.** Every task has the question: is the cheaper tool good enough?

- Prefer surgical `Edit` / `Write` over spawning an agent when the change is small and localized (1-2 files, no cross-references to update, no broad scan needed). Agents cost 10–50x more tokens than direct edits.
- Reserve agents for work that genuinely needs their scope: structural changes (add/remove/rename), cross-file cross-references, broad codebase exploration, multi-step investigations.
- **Example (README regeneration):** the `actions-readme-updater` agent costs ~60–100k tokens per run because it reads all ~40 `action.yml` files. For an input-only change to a single action, a surgical `Edit` (~2–5k tokens) is ~20x cheaper and just as safe. Invoke the agent only when an action is added, removed, or renamed.
- If an agent run reports high token usage for a small change, reconsider whether the agent was the right tool. Prefer to document the cheaper path going forward.

## Code Comments

- Never automatically convert TODO comments to NOTE or remove them. TODOs represent real work to be done — either implement the TODO or ask the user how to proceed.

## GitHub Issues

- When implementing a GitHub issue, after the work is done, tell the user it's complete and propose closing the ticket. Only close after the user approves.

## CLI Preferences

- Always use `gh` CLI instead of `git` for GitHub/remote operations (pushing, pulling, cloning, creating repos, etc.).
- For local-only operations with no `gh` equivalent (e.g. finding the repo root with `git rev-parse --show-toplevel`), `git` is acceptable.
- Always use plain `git pull` (merge), never `git pull --rebase`. Rebase can silently drop commits on conflict — merge is safer.
- Never commit, push, or sync repos with ad-hoc commands. Always use the `/commit` skill exclusively for these operations.
- Be conservative with `gh` API calls to avoid rate limiting. When monitoring CI runs, sleep at least 2 minutes between status checks. Prefer single batch queries over multiple parallel `gh` calls.
- Never use `gh api` to read file contents from external repositories (e.g. fetching individual files via the GitHub API). Instead, ask the user for permission to clone the repo locally into a temp directory, read files locally, then delete the clone when done. This is faster, avoids rate limiting, and allows using normal file tools.

## Plans

- When processing plan files (any file under a `plans/` directory), follow the rules in `courses/docs/rules/00-shared.md` → **Plan Processing** section. Key rule: remove each item from the plan file as it is executed, then delete the plan file when empty, and delete the `plans/` directory when empty.

## Consistency Checks

- When checking consistency across files (e.g. latest vs legacy configs), always enumerate concretely before judging. List every item/stage/type in each file, then compare side-by-side and flag anything present in one but missing from the other.
- Never conclude "no changes needed" based on a quick read. Produce a structured comparison (table or list) that makes gaps self-evident before reaching any conclusion.
- "Consistent" means structural parity: every feature/type/stage in one file must have an equivalent in the other, unless explicitly documented otherwise.

## GitHub Actions — `check-*` actions must NOT swallow errors

A probe's boolean output (`exist`, `exists`, `results[i].exists`, etc.) must reflect a definitive answer to the question the action name asks:

- `true` when the resource demonstrably exists (e.g. HTTP 200, tag matches).
- `false` when the resource demonstrably does not exist (e.g. HTTP 404, clean response with no match).

Anything else — auth failure (401/403), unexpected HTTP code, registry 5xx after retries, network failure, malformed response, token-exchange failure — is **not** a definitive answer. The probe MUST fail hard (`echo "::error::..." ; exit 1`) with a specific, actionable message naming the resource and the likely fix. Returning `false` on an indeterminate result is a lie: it hides the real cause and turns workflow misconfigurations into silent green skips.

Why: a caller asking "does X exist?" and getting `false` should be able to trust that answer means "X is absent" — not "we couldn't tell." Conflating the two masks misconfiguration as legitimate skip and propagates failure downstream (e.g. a 403 from GHCR coerced to `exists=false` silently skips the acceptance stage; QA then fails looking for an RC tag that was never published).

`fail-on-error` inputs on `check-*` actions are vestigial under this rule (the always-loud behavior is mandatory) and should be removed when the action is next touched.
