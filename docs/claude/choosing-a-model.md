# Choosing a Model: Opus vs. Sonnet

Practical guidance for picking between Opus and Sonnet in Claude Code, and what
the "thinking effort" toggle (low / medium / high / max) actually buys you.

For background on what each turn costs, see
[how-claude-works.md](how-claude-works.md). For ways to keep any model cheaper,
see [optimizing-token-usage.md](optimizing-token-usage.md).

---

## TL;DR

- **Sonnet** is the default workhorse: fast, cheap, very capable on routine
  coding work.
- **Opus** is the heavy lifter: better at planning, multi-step reasoning, and
  gnarly debugging. Slower and more expensive per token, but often *cheaper per
  task* on hard work because it gets there in one shot.
- **Opus + medium thinking** is the sweet spot for most non-trivial work — full
  Opus capability without paying for max-depth reasoning you usually don't need.

---

## What "thinking effort" means

Independent of model choice, Claude Code lets you dial *how much hidden
reasoning* the model does before answering: `low`, `medium`, `high`, `max`.

- More thinking = better answers on hard problems, more tokens spent on
  internal reasoning you never see.
- Less thinking = faster, cheaper, fine for shallow tasks.

Thinking effort and model are orthogonal. "Opus medium" and "Sonnet high" are
both valid points on the grid.

---

## When to use Sonnet

Sonnet is the right default. Reach for it when the task is well-scoped and
mostly mechanical:

- Single-file edits, renames, small refactors.
- Writing tests against an existing pattern.
- Codebase Q&A ("where is X defined?", "how does Y flow?").
- Adding a flag, wiring up a known integration, fixing a clear bug.
- Anything where you already know roughly what the diff should look like.

Sonnet at low or medium thinking handles the bulk of day-to-day coding faster
and cheaper than Opus would.

## When to use Opus

Switch to Opus when the task requires actual *judgment* across the codebase:

- Multi-file refactors where the right boundaries aren't obvious yet.
- Architectural decisions, plan authoring, design reviews.
- Debugging that has already resisted one or two obvious attempts.
- Reading an unfamiliar codebase and synthesizing how it works.
- Anything where "looks plausible" is not good enough and you need it
  *correct*.

Opus at medium thinking is the default Opus setting worth trying first. Bump to
high or max only if you see Opus medium hand-waving past something important.

---

## A simple decision rule

Ask yourself: *if I had to do this myself, would I need to stop and think, or
would my fingers just type it?*

- **Fingers just type it** → Sonnet.
- **Stop and think** → Opus medium.
- **Stop, think, sketch on paper, then think again** → Opus high or max.

---

## Cost intuition

Per token, Opus costs more than Sonnet. But "per token" is the wrong unit. What
matters is **cost per finished task**:

- Sonnet on the wrong task = many loops, many re-reads, many wrong-turn edits.
  Cheap tokens, expensive outcome.
- Opus on the right task = one careful pass, fewer tool calls, no rework.
  Pricier tokens, cheaper outcome.

The expensive failure mode isn't "I used Opus when Sonnet would do" — it's "I
used Sonnet on something that needed Opus, and burned an hour of looping before
escalating."

---

## Switching mid-session

You can change model and thinking effort mid-conversation. Reasonable patterns:

- Start a planning session on Opus medium, then drop to Sonnet to execute the
  plan item by item.
- Hit a wall on Sonnet, escalate to Opus medium without clearing — Opus
  inherits the full transcript.
- Long sessions get expensive on any model. When the work shifts topic, prefer
  `/clear` and start fresh over staying in the same context.
