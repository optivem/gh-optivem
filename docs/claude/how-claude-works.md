# How Claude Code Works Behind the Scenes (and How It Affects Token Usage)

A short explainer of the request loop Claude Code runs on every turn, and what
that loop implies for token cost.

## The core loop

1. **You send a message.** Claude Code packages it with: the system prompt, your
   `CLAUDE.md`, tool schemas, conversation history, and any auto-loaded context
   (e.g. `MEMORY.md`, git status). All of that goes to the model on **every
   turn**.
2. **The model responds** with text and/or tool calls.
3. **Tool results come back** and get appended to the transcript.
4. **Repeat** until the model stops calling tools.

Each step re-sends the entire growing transcript. Token cost per turn is roughly
proportional to the *total conversation size so far*, not just your latest
message. That is why long sessions get expensive.

## What inflates tokens

- **Tool results stay in context.** `Read` on a 2000-line file dumps all of it
  into the transcript permanently. `Bash` commands like `find` or `grep` over a
  large tree can return huge output that lingers for the rest of the session.
- **Reading instead of grepping.** `Grep` returns matched lines; `Read` returns
  whole files.
- **Re-reading the same file** after edits, instead of trusting the edit
  result.
- **Long-lived sessions.** Every prior turn is replayed on every new turn.
- **Verbose subagent output.** Subagents help by isolating their own searches,
  but if they return long summaries those summaries land in the main transcript.

## What saves tokens

- **Prompt caching (5-minute TTL).** The static prefix of your conversation
  (system prompt, `CLAUDE.md`, early turns) is cached on Anthropic's side. Cache
  hits cost roughly 10% of normal input tokens. This is why pausing for more
  than 5 minutes between turns is expensive: the cache expires and the whole
  prefix gets re-billed at full rate.
- **Targeted tools.** `Grep` with `head_limit`, `Read` with `offset` / `limit`,
  `Glob` instead of recursive `ls`.
- **Subagents for broad work.** A subagent runs its own loop and returns only a
  summary, so giant search results never pollute the parent context.
- **Auto-compaction.** When you near the context limit, older turns get
  summarized into a shorter form.
- **Surgical edits over agents for small changes.** See the "Token Usage"
  section in the repo `CLAUDE.md` — agents cost 10–50x more tokens than direct
  edits.

## Practical implication

The expensive turn is not the one with the long answer. It is the one that
loaded a 5000-line file ten turns ago and is still being re-sent on every
round trip.
