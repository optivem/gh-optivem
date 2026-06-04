# Plan conventions

Conventions for the `plans/` tree — how plans are written, where they live, and what
happens to them once their execution is done.

## Lifecycle of a finished plan

When a plan's execution items are all done, retired, or split into separate (already
landed) follow-up plans, the plan no longer drives work. Decide its fate by reference
value:

- **Purely procedural** — a checklist whose value evaporated once executed → **delete it.**
  Git history preserves the full text; `git log` / `git show` recover it if ever needed.
- **Reference-worthy** — design Q&A, decision rationale, or a cross-check inventory that
  explains *why* something is the way it is → **distill the durable doctrine into
  `docs/`**, then delete the plan. The result of every decision already lives in the code
  and config; only the reasoning needs a discoverable home, and `docs/` is that home.

There is no in-tree archive of frozen plans. Reference material lives in `docs/`, not in
the plans tree — a plan that has stopped driving work is either distilled or deleted, never
parked.

## Cross-referencing

- Plan files **may** cite `docs/...` paths (and other plan paths) when load-bearing
  rationale lives there — e.g. an upcoming plan citing a settled design decision.
- Code and runtime YAML files **must not** cite plan paths. Code stands on its own; the
  plan record is for humans reading the plan tree, not for the binary to consume. When a
  plan's reasoning matters to the code, distil it into `docs/` and cite that.

## Authoring

- New work goes in a fresh `plans/YYYYMMDD-HHMM-<slug>.md` (local time in the filename
  timestamp). Broadening scope means a new plan that cross-references the original — never
  an in-place rewrite of a plan already in flight.
- Before executing a plan, check for an existing pickup marker and inspect `git status`
  in case another agent is already editing overlapping files.
