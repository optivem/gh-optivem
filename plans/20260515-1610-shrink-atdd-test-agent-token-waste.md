# Plan: Shrink token waste in the AT - RED - TEST dispatch

## Background

Diagnosed live from a `rehearsal-20260515-132502` run on issue #65 ("View
product list") dispatched into `atdd-test-at`. Three observed symptoms
that all trace to the same gap — the prompt hands the agent a ticket
*reference* but not the *content* — plus a stale-link bug that
compounds the waste.

Observed in the transcript:

1. The agent reported "The TypeScript equivalents file wasn't found".
2. The agent reasoned "look at the ticket for #65" despite already
   being launched against that ticket.
3. The agent ran `gh issue view 65 --repo optivem/academy-shop` and
   got `Could not resolve to a Repository`.
4. The agent then grepped the system-test tree to reconstruct the DSL
   port/adapter layout.

### Where each comes from

**(1) — broken doc links.** The agent template
`internal/assets/runtime/prompts/atdd/atdd-test-at.md:20` correctly
points to the per-language file
(`${docs_root}/atdd/code/language-equivalents/${language}.md`). But the
phase doc the agent also reads — `at-red-test.md` — has four stale
links to a single flat file that no longer exists:

```
- `@Disabled` / skip syntax per language: see [language-equivalents.md](../code/language-equivalents.md).
- "TODO: DSL" prototype syntax per language: see [language-equivalents.md](../code/language-equivalents.md).
```

(`at-red-test.md:16`, `:17`, `:36`, `:59`.) `dsl-core.md:5` carries the
same stale reference. The split into `language-equivalents/<lang>.md`
happened earlier; these links were missed.

**(2) and (3) — only the ticket number and title reach the prompt.**
The shared preamble at `internal/assets/runtime/shared/preamble.md:3`
injects:

```
Ticket: #${issue_num} "${issue_title}"
```

`renderPrompt` in `clauderun.go:437-451` substitutes those plus
`${checklist}`. But:

- `atdd-test-at.md` does not reference `${checklist}`. Only the
  structural-task agents (`atdd-task-system-interface-redesign.md:15`,
  `atdd-task-external-system-interface-redesign.md:15`,
  `atdd-chore.md:15`) consume it.
- `${checklist}` is only the parsed `## Checklist` section anyway. The
  Gherkin scenarios / acceptance criteria block is never piped through.

`intake/parse.go` already parses the ticket body during intake — the
scenarios are extracted, just not surfaced to the agent. The agent
falls back to `gh issue view 65` to read what intake already saw.
Because the rehearsal worktree's `git` remote doesn't match the
issue's actual repo, the fallback resolves to the wrong slug
(`optivem/academy-shop`) and 404s. Even if it had worked, the round
trip is wasted tokens.

**(4) — same root cause.** Nothing in the prompt describes this repo's
DSL port/adapter layout, existing scenarios, or what's already
implemented, so the agent spelunks the codebase to reconstruct it.

## Goals

- Fix the broken `language-equivalents.md` links so the agent can
  follow them without searching.
- Inject the parsed scenarios into the AT - RED - TEST prompt so the
  agent stops shelling out to `gh issue view`.
- Decide whether `atdd-test-at` should also receive `${checklist}`
  (separate question — see Item 3).

## Non-goals

- Re-architecting how intake stores parsed ticket sections. Whatever
  field is already populated by `intake.Parse` is what we surface; if
  it isn't there yet, the work is in scope but the storage shape stays.
- Touching agents other than `atdd-test-at` for the scenarios
  substitution. The same gap may apply to `atdd-dsl`, `atdd-driver`,
  and `atdd-system` but those are out of scope here — flag for a
  follow-up.
- Adding a `${dsl_index}` / repo-layout substitution. The codebase
  spelunking is a real waste but the right shape isn't obvious yet;
  defer to a separate plan.

## Items

### Item 1 — Fix stale `language-equivalents.md` links in process/architecture docs

The split into per-language files left **15 stale references** across
11 docs, pointing at a flat file that no longer exists. The agent
template is already correct, so the agent *has* the per-language path
— the doc links just compound the confusion when the agent reads
those docs. All references now point at
`../code/language-equivalents/README.md` (the directory index).

**Resolution:** link to the directory README (chosen 2026-05-15). The
file already exists and serves as the language index; GitHub
auto-renders README.md when navigating the directory; humans benefit
from a valid pointer.

#### Status — DONE

All 19 references across 12 files now point at
`../code/language-equivalents/README.md`. Sanity-checked: `grep -r
'language-equivalents\.md' internal/assets` returns no matches.

**Architecture docs (4 files, 4 references):**

- `architecture/dsl-core.md:5` ✓
- `architecture/test.md:5` ✓
- `architecture/driver-adapter.md:19` ✓
- `architecture/driver-port.md:14` ✓

**Process docs (8 files, 15 references):**

- `process/at-red-test.md` ×4 ✓
- `process/at-red-dsl.md` ×2 ✓
- `process/at-red-system-driver.md` ×1 ✓
- `process/at-green-system.md` ×1 ✓
- `process/ct-red-test.md` ×3 ✓
- `process/ct-red-dsl.md` ×2 ✓
- `process/ct-red-external-driver.md` ×1 ✓
- `process/ct-green-stubs.md` ×1 ✓

#### Note on the concurrent agent

Mid-execution, the sister `condense-process-docs` plan committed
(`c42af87`) and absorbed three of my in-flight edits (`at-red-test.md`,
`dsl-core.md`, `test.md`) alongside its own work. The other 11
process-doc references were untouched by that commit and got fixed
cleanly in a second pass once the working tree was no longer in
collision. Per the collision-recovery memory rule, I verified the
absorbed edits matched my intent in HEAD before continuing.

### Item 2 — Inject ticket scenarios into the AT - RED - TEST prompt

The intake stage already parses the ticket body. The acceptance
criteria / scenarios block is extracted but never surfaced. Add a
`${scenarios}` (name TBD) placeholder that:

1. Is populated by `seedScopeState` / `preResolveIssue` from whatever
   field `intake.Parse` writes for the AC block.
2. Is substituted by `renderPrompt` in `clauderun.go` alongside
   `${checklist}`.
3. Is referenced from `atdd-test-at.md` in the WRITE phase body —
   e.g. an opening "Scenarios to implement:" block above the existing
   `Read ${docs_root}/...` lines.

**Acceptance:**

- A dispatch to `atdd-test-at` on a ticket whose body has Gherkin
  scenarios renders a prompt that contains those scenarios verbatim.
- The agent no longer needs `gh issue view <n>` to read scenarios.
  (We can't enforce this in code — call it observed-behaviour
  acceptance after a follow-up rehearsal run.)
- `findUnfilledPlaceholders` still passes for agents that don't
  reference the new placeholder (it's only an error when the agent
  body mentions an unfilled `${name}`; unused values are fine).

**Open questions for Item 2:**

- a. What does `intake.Parse` currently extract for the scenarios
  block? Need to read `internal/atdd/runtime/intake/parse.go` and
  `sections.go` before naming the placeholder.
- b. What should the placeholder be called? `${scenarios}`,
  `${acceptance_criteria}`, `${ticket_body}`? Strongly leaning
  `${scenarios}` because it's the smallest faithful description, but
  open to argument.
- c. Should the empty case (ticket with no scenarios block) render
  the placeholder as `(none)` or fail the dispatch? The
  AT - RED - TEST dispatch logically can't proceed without scenarios,
  so I'd argue fail fast. But that's a real behavioural change worth
  confirming.
- d. Do we surface the full ticket body for context (description +
  scenarios + legacy coverage) or only the scenarios block? The
  agent template's "all scenarios in the ticket" wording suggests
  scenarios are sufficient; the description is rarely test-relevant.

### Item 3 — Decide whether `atdd-test-at` should also reference `${checklist}`

The structural-task agents reference `${checklist}` because their
input *is* the checklist. The AT - RED - TEST agent's input is the
scenarios block; checklist is orthogonal.

**Open question for Item 3:**

- Does the AT - RED - TEST agent ever need the Checklist section? I
  don't think so — the checklist is a structural-task / chore
  artefact, and AT - RED - TEST works from AC scenarios. My
  recommendation: leave `${checklist}` unreferenced in
  `atdd-test-at.md`. But surfacing this as an explicit decision so
  Item 2's prompt edits don't accidentally drag the checklist in.

## Order of execution

Walk one item at a time, in the order above. Item 1 is mechanical and
low-risk; do it first to clear noise. Item 2 is the real fix and
needs the open questions resolved before touching code. Item 3 is a
one-line decision.
