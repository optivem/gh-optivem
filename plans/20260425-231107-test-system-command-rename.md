# Idea: rename `run system tests` → `test system`

## Status

Idea / proposal. No implementation work scheduled.

## Problem

Today's command structure mixes verb-first and verb+noun+noun shapes:

| Current command           | Shape                |
|---------------------------|----------------------|
| `gh optivem build system` | verb + noun          |
| `gh optivem run system`   | verb + noun          |
| `gh optivem stop system`  | verb + noun          |
| `gh optivem run system tests` | verb + noun + noun |

`run system tests` is the odd one out. It's longer, breaks the parallel pattern, and reads as "run (the) system tests" which is ambiguous — is "tests" the object of "run", or is "system tests" the object?

## Proposal

Rename to `gh optivem test system`:

| Future command            | Shape       |
|---------------------------|-------------|
| `gh optivem build system` | verb + noun |
| `gh optivem run system`   | verb + noun |
| `gh optivem test system`  | verb + noun |
| `gh optivem stop system`  | verb + noun |

All four lifecycle commands become a clean parallel set: build, run, test, stop — each acting on `system`.

## Pros

- Shorter (3 tokens → 3 tokens, but the third token is a noun matching siblings instead of a redundant `tests`).
- Parallel structure with `build/run/stop system` — easier to remember and document.
- "test system" reads unambiguously: verb `test` operating on `system`.
- Reads better in ATDD prompts that students will run manually:
  `gh optivem test system --suite acceptance-ui`
  vs.
  `gh optivem run system tests --suite acceptance-ui`.

## Cons

- Breaking change. Anything pinning the old form has to update.
- Loses some symmetry with `run system` (test runner conceptually starts with bringing the system up). But `gh optivem test system` already implies the system needs to be running — same as `gh optivem stop system` implies it's running.

## Migration

If we do this:

- [ ] Add `test system` as the new canonical form.
- [ ] Keep `run system tests` as a deprecated alias for one or two minor versions, printing a deprecation warning to stderr.
- [ ] Update docs:
  - `gh-optivem/main.go:96` (printUsage).
  - `gh-optivem/runner_commands.go` comments and `flag.NewFlagSet` name.
  - `gh-optivem/docs/gh-monitoring-process.md:52-53`.
  - `gh-optivem/scripts/manual-test-runner-shop.sh:10-11,55,60`.
  - Any other plan files / READMEs that reference the old form.
- [ ] Update the ATDD templates (per `shop/plans/atdd-claude-distribution.md`) to use the new form from day one — no need to template both forms.
- [ ] Remove the alias in a later major version.

## Alternatives considered

- **Keep status quo (`run system tests`).** Rejected — the asymmetry will only get more painful as more lifecycle commands are added.
- **`gh optivem run tests` (drop `system`).** Rejected — loses the `<verb> system` parallel and creates ambiguity if non-system tests are ever added (unit, contract, etc.).
- **`gh optivem system test` (noun-first).** Rejected — would require renaming all four (`gh optivem system build`, `gh optivem system run`, etc.) which is a much larger break for marginal gain.

## Decision

Pending.
