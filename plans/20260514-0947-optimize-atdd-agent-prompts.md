# Plan: optimize ATDD agent prompts and centralize ATDD ownership in gh-optivem

## Context

`gh optivem implement` dispatches one of nine embedded agent prompts under
`internal/atdd/runtime/agents/prompts/*.md` per phase via `claude -p` (see
`internal/atdd/runtime/clauderun/clauderun.go:984`). The user reports
the Claude steps feel slow even on simple work and suspects they burn
more tokens than necessary. Static analysis of the embedded prompts
confirms the suspicion: the prompt bodies are large and heavily duplicated.

A second motivation surfaced during planning: the user wants
**gh-optivem to be the canonical owner of the ATDD process**, so that any
repo — not just ones scaffolded from `shop` — can run `gh optivem implement`.
That reshapes the fix from "shrink the embedded prompts" into "make the
prompts thin pointers into a single canonical source-of-truth that
gh-optivem itself owns."

### Size baseline (today)

```
atdd-backend.md          71 lines    4.7 KB
atdd-chore.md           223 lines   15.4 KB
atdd-driver.md          302 lines   21.5 KB
atdd-dsl.md             324 lines   23.2 KB
atdd-fix-verify.md       64 lines    5.4 KB
atdd-frontend.md         73 lines    4.6 KB
atdd-stubs.md            91 lines    5.1 KB
atdd-task.md            178 lines   13.2 KB
atdd-test.md            313 lines   21.0 KB
shared/session-end.md    25 lines    1.8 KB
─────────────────────────────────────────────
total                 1664 lines  115.9 KB
```

Each `claude -p` invocation gets the **full prompt as system input every
time** (`runAutonomous` at `clauderun.go:1009-1043` passes the rendered
string via argv with no caching layer in between). Three of the nine
prompts are over 20 KB.

### Problem inventory

**P0. Three-way duplication of the ATDD source-of-truth.**
The ATDD process docs and Claude assets exist in three places today:

| Where | What | Role |
|---|---|---|
| `shop/docs/atdd/` and `shop/.claude/agents/atdd/` | Source of truth | Hand-maintained in a course repo |
| `gh-optivem/internal/atdd/runtime/agents/prompts/*.md` | Embedded prompts | Copy-pasted with shop's docs inlined, used by `claude -p` |
| Every scaffolded repo (page-turner, etc.) | Installed copy | Output of `gh optivem init` |

`internal/atdd/install.go:5-9` explicitly states the design intent:
"Source-of-truth: the shop checkout… This package never embeds
templates." But the prompts package broke that rule by embedding
shop's docs into the prompt bodies (see P1 below). Worse, anyone who
wants to run `gh optivem implement` on a repo that wasn't scaffolded
from shop has no path to install the docs — shop is a hard
prerequisite for the install step.

The audit of `academy/` confirms the blast radius is small:
`hub/checklists/atdd/` is unrelated (student checklists, different
content); `rehearsal-*/docs/atdd/` is scaffold output not source;
nothing in `courses/`, `optivem-testing/`, `github-utils/`, `actions/`,
or `claude/` references `docs/atdd`. Within shop, references come
from `.claude/agents/atdd/meta/*.md` (which move together with the
docs under the fix) and intra-doc cross-links.

**P1. Inlined reference docs are the bulk of the payload (highest impact).**
Every prompt body ends with a `## References` section that pastes the
contents of multiple `docs/atdd/process/*.md` and `docs/atdd/architecture/*.md`
files verbatim. Examples:

- `atdd-dsl.md` inlines six full reference files
  (`shared-phase-progression.md`, `at-cycle-conventions.md`,
  `ct-cycle-conventions.md`, `at-red-dsl.md`, `ct-red-dsl.md`,
  `dsl-core.md`, `driver-port.md`, `language-equivalents.md`).
  About **85 %** of the file is references.
- `atdd-driver.md` inlines `at-cycle-conventions.md`,
  `ct-cycle-conventions.md`, `at-red-system-driver.md`,
  `ct-red-external-driver.md`, `driver-port.md`,
  `language-equivalents.md`. About **85 %** is references.
- `atdd-test.md` (313 lines) follows the same pattern.

But the consumer-repo working tree already contains those files
(`docs/atdd/process/*.md`, `docs/atdd/architecture/*.md`,
`docs/atdd/code/language-equivalents.md`) — `atdd-fix-verify.md:178`
and `atdd-task.md:178` even acknowledge this with
"omitted at WRITE time — both files exist in the consumer repo… Read them
with the Read tool if a step asks you to consult them." The agent has
`Read`. There is no need to ship the docs in the prompt.

**P2. Heavy cross-prompt duplication.**

- The boilerplate header (lines 1–8: ticket vars, "one-shot dispatch", "don't
  commit / don't summarise") is identical or near-identical across **all
  nine prompts** — duplicated nine times in the embed instead of once in
  `shared/`.
- `driver-port.md`, `language-equivalents.md`, and the cycle-conventions
  files are each inlined in 2–4 prompt files. Editing the canonical
  `docs/atdd/...` copy now requires hand-syncing every embedded copy
  (and a regression test, since none enforces it).
- Subtype-gated sub-blocks via `<!-- if:subtype=... -->`
  (`clauderun.go:358`) help only marginally — they trim conditional
  paragraphs, not the bulk reference inlines.

**P3. AT phase + CT phase fused into a single prompt.**
`atdd-dsl.md`, `atdd-driver.md`, and `atdd-test.md` each describe
both their AT-cycle and CT-cycle variants in the same file, then inline
the reference docs for both cycles. On any given dispatch, half the
prompt content (the other cycle's docs and anti-patterns) is irrelevant
and just consumes tokens. The dispatcher already knows the phase
(`${phase}`) — gating could prune the irrelevant half.

**P4. Verbose, repetitive prose inside the prompt-specific sections.**
The "Anti-patterns" lists frequently restate guard rails that were
already stated in "Conventions" and "WRITE / PROTOTYPES" steps a few
lines above (e.g. "do not add @Disabled markup" is repeated 3× in
`atdd-dsl.md`, 2× in `atdd-driver.md`). Tightening prose without
removing meaning would shrink each prompt by 10–20 %.

**P5. `language-equivalents.md` is pasted in full when one row is needed.**
Most prompts only ever need one column of the language-equivalents tables
(the dispatch knows the in-scope Test Lang from `${architecture}` / scope
context, but ships TODO Stubs, Test Disabling, String Field Types, DTO
Boilerplate, Test File Naming, and Awaitable ShouldSucceed for **all three
languages** every time). The orchestrator could either substitute only the
relevant row or omit the table entirely and rely on the agent's Read tool.

### Why this matters for latency and cost

- Larger prompts = larger first-turn input = more time spent ingesting
  tokens before the model emits anything.
- `claude -p` has no `--prompt-cache` flag exposed today, so every
  dispatch pays full input-token cost for the entire body (see
  `runAutonomous` at `clauderun.go:1009` — only `--output-format json`
  is set).
- Anecdotally, three of the nine prompts (`atdd-dsl`, `atdd-driver`,
  `atdd-test`) — the ones most often in the critical path of `implement`
  — are also the largest at ~21 KB each.

A conservative estimate: cutting reference-doc inlines reduces the three
heavy prompts by ~70 % (from ~21 KB to ~6 KB) — roughly **45 KB shaved
per implement run that touches DSL → Driver → Test**, which is the
common AT-cycle path.

## Proposed solutions

The fixes form a stack. S0 is a prerequisite for S1 — once gh-optivem
owns the canonical docs, the prompts can confidently point at known
paths. The rest are independently shippable.

### S0. Move ATDD source-of-truth into gh-optivem (decoupling step)

Today `internal/atdd/install.go:88-114` walks the shop checkout
(`Options.ShopPath`) and copies `docs/atdd/{process,architecture,code}/`
plus `.claude/agents/atdd/` and `.claude/commands/atdd/` into the
consumer repo. Under S0:

1. Move the canonical copies of these trees from `shop/` into
   `gh-optivem/internal/atdd/assets/` (or similar — directory name TBD):
   - `assets/docs/atdd/process/*.md`
   - `assets/docs/atdd/architecture/*.md`
   - `assets/docs/atdd/code/*.md`
   - `assets/claude/agents/atdd/*.md` (including the `meta/` subtree)
   - `assets/claude/commands/atdd/*.md`
2. Add a `//go:embed assets/...` declaration following the
   `internal/atdd/runtime/agents/embed.go:10-16` precedent
   (`promptFS embed.FS`).
3. Rewrite `internal/atdd/install.go` so `Plan` (`install.go:90`)
   walks the embed.FS instead of `filepath.Join(opts.ShopPath, …)`.
   Drop the `ShopPath` field from `Options` (or keep it for the
   non-ATDD parts of `gh optivem init` that still copy system code
   from a shop checkout — TBD based on the rest of init's needs).
4. Delete the copies in `shop/docs/atdd/`, `shop/.claude/agents/atdd/`,
   `shop/.claude/commands/atdd/`. shop becomes a regular consumer:
   after `gh optivem init` runs on the shop repo, the trees re-appear
   in shop, identical in content to every other scaffolded repo.
5. Update the `gh optivem init` flow so it works on any repo —
   not just one that has a shop checkout adjacent.

After S0:
- One canonical source: `gh-optivem/internal/atdd/assets/...`.
- Any repo can adopt ATDD via `gh optivem init` (or a dedicated
  `gh optivem atdd install` subcommand if init's broader semantics
  shouldn't fire).
- The embedded prompts in `internal/atdd/runtime/agents/prompts/*.md`
  can safely reference doc paths because gh-optivem installed them
  in the consumer repo.

**Risk:** shop loses its self-contained "open the repo, read the
process docs" property. Mitigation: leave a `shop/docs/atdd/README.md`
stub pointing at the gh-optivem source, and rely on the fact that
running `gh optivem init` against shop restores the docs locally.

**Expected savings:** none directly — S0 is structural. But it
unblocks S1's "agent reads docs at runtime" pattern across every
consumer repo, not just shop scaffolds.

### S1. Replace inlined reference docs with file pointers (highest impact)

Each `## References` section in the prompt body becomes a one-line
pointer:

```
Apply Driver Port Rules — read `docs/atdd/architecture/driver-port.md`.
Apply DSL Core Rules — read `docs/atdd/architecture/dsl-core.md`.
Cycle conventions — read `docs/atdd/process/at-cycle-conventions.md`.
```

The agent has the `Read` tool and the docs live in the consumer-repo
working directory at known paths. This is the same trade-off
`atdd-fix-verify.md` and `atdd-task.md` already made for `glossary.md`
and `language-equivalents.md`.

**Risk:** the agent forgets to read a doc it needs. Mitigation: keep one
or two **must-read-first** docs inlined when they encode rules the
agent cannot infer (e.g. the "do not commit" sentence in
`shared-phase-progression.md`) — but inline them at the top, not as
appendices, and only when they fit in <~500 bytes.

**Expected savings:** ~70 % on the three heavy prompts; ~40 % overall.

### S2. Extract the shared header into `shared/header.md` and prepend it like `session-end.md`

The "one-shot dispatch / don't commit / don't summarise" preamble lives
once. `embed.go:32` already wraps the prompt body between content and
`shared/session-end.md`; extend the same pattern with a leading
`shared/header.md` so the duplication disappears from each individual
file.

**Risk:** very low. Mechanical refactor.

**Expected savings:** ~250 bytes × 9 prompts = ~2 KB across all
dispatches, plus a maintenance win.

### S3. Split AT-cycle vs CT-cycle prompts (or gate them with `<!-- if:cycle=... -->`)

Either:

- (a) Split `atdd-dsl.md` → `atdd-dsl-at.md` + `atdd-dsl-ct.md` (same
  for `atdd-driver` and `atdd-test`), and dispatch the right one based on
  the phase prefix.
- (b) Extend the existing conditional regex (`conditionalRE` at
  `clauderun.go:358`) with a new `${cycle}` placeholder (`at` | `ct`)
  and wrap each cycle-specific block in
  `<!-- if:cycle=at -->...<!-- end-if -->`.

(b) is less code churn and keeps all the dispatch wiring untouched —
just one new params key in `renderPrompt` (`clauderun.go:414`).

**Risk:** misclassification (CT block runs on an AT phase). Already
mitigated by the `findUnfilledPlaceholders` guard
(`clauderun.go:264`) plus a small unit test asserting the rendered
prompt only contains the in-scope cycle's words.

**Expected savings:** another ~40 % on `atdd-dsl`, `atdd-driver`,
`atdd-test`. Stacks with S1.

### S4. Tighten prose — collapse "Anti-patterns" + "Conventions" + "WRITE steps"

Two-pass copyedit on each prompt body (post-S1, post-S3) — remove
restatements, keep one canonical mention of each guard rail. No
new mechanics needed. Best done after S1/S3 because the surviving
prose is what is being copyedited, not the bulk references.

**Risk:** accidental rule deletion. Mitigation: do it as a PR with the
canonical `docs/atdd/...` references in the diff context so the
reviewer can cross-check.

**Expected savings:** ~15 % on the surviving prompt size.

### S5. Materialize prompts to disk only when they exceed a threshold the cache likes

Today `materializePrompt` (`clauderun.go:931`) spills to a tempfile
above `promptArgvLimit = 8000` to dodge Windows argv limits. After S1
most prompts will be under the limit, restoring the fast inline path.
This is a side-effect of the other work, not its own task.

## Decisions taken (2026-05-14 discussion)

- **Ownership: Option B.** gh-optivem owns the ATDD source-of-truth.
  Two reasons: (a) eliminates three-way duplication that drove the
  prompt bloat in the first place, (b) lets any repo — not just shop
  scaffolds — adopt ATDD via `gh optivem init` / a dedicated install
  command.
- **S0 is the prerequisite to S1.** Without S0 the prompt pointers
  would only work in shop-scaffolded repos.

## Decision points still open

1. **Split prompt files (S3a) or gate with conditionals (S3b)?**
   S3a is one file per phase, mirroring `docs/atdd/process/*.md`.
   S3b adds a `${cycle}` placeholder and uses the existing
   `<!-- if:... -->` machinery — less file churn.
2. **Keep any inlined "safety" snippets in the prompts after S1?**
   The "do not commit" rule and a one-paragraph "what this phase
   produces" summary could stay inline so the agent never has to
   reach for a file to learn the most-load-bearing rules. The
   reference docs themselves still move out.
3. **Subcommand for the install step.** Is `gh optivem init` the
   right home for installing ATDD assets into a non-shop repo, or
   does that warrant a dedicated `gh optivem atdd install` so the
   wider init flow (system code, workflows, externals) stays
   shop-scoped? Affects `install.go`'s signature, not the move itself.
4. **`--prompt-cache` (out of scope).** Worth a follow-up plan
   if/when the Claude CLI exposes prompt caching.

## Critical files

Edited by this plan:

**S0 — ownership move:**
- `internal/atdd/assets/docs/atdd/{process,architecture,code}/*.md` —
  new (moved from `shop/docs/atdd/`).
- `internal/atdd/assets/claude/agents/atdd/*.md` — new (moved from
  `shop/.claude/agents/atdd/`, including `meta/` subtree).
- `internal/atdd/assets/claude/commands/atdd/*.md` — new (moved from
  `shop/.claude/commands/atdd/`).
- `internal/atdd/install.go` — switch `Plan` (`:90`) from
  `Options.ShopPath` walks to an `embed.FS`. Decide whether to keep
  `ShopPath` for non-ATDD init responsibilities.
- `internal/atdd/install_test.go` — update fixtures that
  currently hand-build a fake shop tree.
- shop repo (separate PR or coordinated commit): delete
  `shop/docs/atdd/`, `shop/.claude/agents/atdd/`,
  `shop/.claude/commands/atdd/`. Leave a `shop/docs/atdd/README.md`
  stub pointing at gh-optivem.

**S1–S4 — prompt slimming:**
- `internal/atdd/runtime/agents/prompts/atdd-dsl.md` — strip inlined
  references (S1), gate cycle blocks (S3), tighten prose (S4).
- `internal/atdd/runtime/agents/prompts/atdd-driver.md` — same.
- `internal/atdd/runtime/agents/prompts/atdd-test.md` — same.
- `internal/atdd/runtime/agents/prompts/atdd-task.md` — strip
  inlined `system.md`, `driver-port.md`, `driver-adapter.md`
  references (S1).
- `internal/atdd/runtime/agents/prompts/atdd-chore.md` — strip
  inlined `task-and-chore-cycles.md` (S1).
- `internal/atdd/runtime/agents/prompts/atdd-backend.md`,
  `atdd-frontend.md`, `atdd-stubs.md`, `atdd-fix-verify.md` — strip
  any remaining inlines + apply the shared header (S2).
- `internal/atdd/runtime/agents/shared/header.md` — new file (S2).
- `internal/atdd/runtime/agents/embed.go` — prepend
  `shared/header.md` the same way `shared/session-end.md` is appended
  (S2).
- `internal/atdd/runtime/clauderun/clauderun.go` — extend
  `renderPrompt` (`:414`) to seed a `${cycle}` placeholder (S3) and
  tighten `conditionalRE` (`:358`) if S3b is chosen.

Read-only references:

- `internal/atdd/runtime/clauderun/clauderun_test.go` — `TestRenderPrompt_*`
  is where the assertion-on-rendered-text regression tests live; new
  cases land here.
- `internal/atdd/runtime/agents/embed.go:10-16` — precedent for the
  `embed.FS` pattern S0 generalizes.

## Reuse references

- `internal/atdd/runtime/agents/embed.go:14-39` —
  `sharedSessionEnd` precedent shows exactly how `shared/header.md`
  should be wired (read at init, panic on missing, append with `\n---\n`).
- `internal/atdd/runtime/clauderun/clauderun.go:358-373` —
  `conditionalRE` + `filterConditionals` is the gating engine S3b
  builds on.
- `internal/atdd/runtime/clauderun/clauderun.go:264-269` —
  `findUnfilledPlaceholders` already guards against typos; adding
  `${cycle}` doesn't break the guarantee.
- `atdd-fix-verify.md:178` / `atdd-task.md:178` —
  precedent for "reference exists in consumer repo; read with the Read
  tool" — S1 generalizes this rule.

## Steps

### Step 1 — resolve the open decision points

Surface the four open decisions (split vs gate for S3; keep safety
inlines yes/no; subcommand shape for the install; `--prompt-cache`
out-of-scope confirmation). Stop here if the user wants a different
shape.

**Validation:** user picks an option for each.

### Step 1.5 — implement S0 (move ATDD ownership into gh-optivem)

Land this as one bundle (the move is non-atomic if split — every
consumer would see a window where the docs are gone). Sub-steps:

1. Create `internal/atdd/assets/` with `//go:embed assets/...` in a
   new sibling of `agents/embed.go`.
2. Copy `shop/docs/atdd/`, `shop/.claude/agents/atdd/`,
   `shop/.claude/commands/atdd/` into the new tree.
3. Rewrite `internal/atdd/install.go` to walk the embed.FS.
4. Update `internal/atdd/install_test.go` fixtures.
5. Delete the original copies in shop and replace with a
   pointer-README.
6. Verify `gh optivem init` on a freshly cloned shop produces the
   same on-disk result as before.

**Validation:**
- `diff -r` between the pre-change shop install output and the
  post-change shop install output is empty.
- `gh optivem init` on a non-shop-scaffolded repo successfully
  installs `docs/atdd/` and the `.claude/atdd` trees.
- `internal/atdd/install_test.go` passes without a real shop
  checkout on disk.

### Step 2 — implement S2 (shared header)

Create `shared/header.md`, prepend it in `embed.go`, delete the
duplicated preamble lines 1–8 from every prompt. Land as one PR.

**Validation:**
- `TestRenderPrompt_*` still passes (rendered prompt shape unchanged
  apart from header origin).
- `wc -c internal/atdd/runtime/agents/prompts/*.md` drops by ~2 KB.

### Step 3 — implement S1 (strip inlined references)

Per-prompt copyedit: replace each `### Reference: ...` block with a
short pointer line. Keep at most one inline rule-snippet per prompt if
the user opted for "keep safety inlines" in Step 1. Land as one PR per
prompt (or one big PR — operator choice) so a regression in one prompt
doesn't taint the others.

**Validation:**
- `wc -c` on the three heavy prompts drops to ~6 KB each.
- A smoke `gh optivem implement` run on a known ticket completes
  without the agent reporting "missing context."
- `findUnfilledPlaceholders` still returns empty.

### Step 4 — implement S3 (AT/CT split or gate)

Pick (a) or (b) based on Step 1's choice. Wire `${cycle}` into
`renderPrompt`. Update `TestRenderPrompt_*` with one new case per
cycle direction.

**Validation:**
- Rendered AT prompt contains no `CT - RED - ...` strings (assertion).
- Rendered CT prompt contains no `AT - RED - ...` strings.
- A `gh optivem implement` run on a ticket that triggers both AT and
  CT cycles completes both halves end-to-end.

### Step 5 — implement S4 (prose copyedit)

Two-pass review of each surviving prompt body. No mechanics. Diff
review against the canonical `docs/atdd/...` files (which are
authoritative).

**Validation:** another ~15 % size drop; same smoke test still green.

### Step 6 — measure

Capture per-dispatch token usage before / after from the
`writeExitBanner` output (`clauderun.go:844`) on the same ticket. The
banner already prints `… token in / … token out, $…` when the
`claude -p --output-format json` envelope decodes. A single side-by-side
run on a representative AT cycle is the empirical answer to "did this
help?"

**Validation:** input tokens drop by the predicted ~40-70 % on the
three heavy prompts; output tokens unchanged or slightly lower; wall
time drops proportionally.

## Out of scope

- Changing the Claude CLI invocation flags (e.g. enabling a hypothetical
  `--prompt-cache`). Worth a follow-up plan if/when the upstream CLI
  exposes one.
- Restructuring the agent set itself (merging `atdd-backend` +
  `atdd-frontend` into `atdd-system`, etc.). That is a different
  conversation about agent topology, not prompt size.
- Re-shaping the `${allowed_roots}` / `${checklist}` substitution blocks
  produced by the driver (`clauderun.go:769-820`). They are already
  compact and per-call.
- The non-ATDD portions of `gh optivem init` (system code copy,
  workflow installation, externals). S0 only touches the ATDD asset
  subset; whether the broader init flow should also stop needing a
  shop checkout is a separate question.
