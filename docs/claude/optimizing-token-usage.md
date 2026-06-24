# Optimizing Token Usage in Claude Code

Practical strategies for keeping sessions cheap, with concrete side-by-side
examples of what burns tokens vs. what does the same job for a fraction of the
cost.

For background on *why* the loop works this way, see
[how-claude-works.md](how-claude-works.md).

---

## 1. Search with `Grep` / `Glob`, not `Read` or `Bash`

Reading a whole file to find one symbol is the single most common waste.

**Expensive — reads ~2000 lines into context permanently:**

```
Read("src/server/handlers/orders.ts")
```

**Efficient — returns ~5 matched lines:**

```
Grep(pattern="createOrder", glob="**/*.ts", output_mode="content", -n=true)
```

Same applies to file discovery:

**Expensive:**

```
Bash("find . -name '*.yml'")        # dumps every path, no filtering
```

**Efficient:**

```
Glob(pattern="**/*.yml")             # sorted by mtime, structured
```

---

## 2. Read with `offset` / `limit`, not whole files

Once you know the line range, ask only for that range.

**Expensive:**

```
Read("CHANGELOG.md")                 # 8000 lines
```

**Efficient:**

```
Read("CHANGELOG.md", offset=1, limit=80)
```

A `Grep` first to find the relevant line number, then a targeted `Read`, costs
maybe 1/50th of reading the whole file.

---

## 3. Cap output with `head_limit`

`Grep` defaults to 250 lines, but for noisy patterns (e.g. `import`) even that
is too much.

**Expensive:**

```
Grep(pattern="import", type="ts")    # could be thousands of lines
```

**Efficient:**

```
Grep(pattern="import .* from '@/auth'", type="ts", head_limit=20)
```

Tighten the regex *and* cap the output. Both save tokens.

---

## 4. Use subagents for broad exploration

Subagents run their own tool loop and return only a summary. The intermediate
tool results never enter your main transcript.

**Expensive — pollutes main context with every file the search touched:**

```
Grep + Read + Grep + Read + Grep + Read ...   (you, in the main loop)
```

**Efficient — main context only sees the final summary:**

```
Agent(subagent_type="Explore", prompt="find every place we construct a JWT...")
```

Rule of thumb: if a search will take more than ~3 tool calls, delegate it.

---

## 5. Surgical `Edit` over `Agent` for small changes

Agents are expensive — they re-read context, plan, and verify. For a one-line
change in a known file, that overhead is pure waste.

**Expensive (~60–100k tokens):**

```
Agent(subagent_type="actions-readme-updater", ...)   # re-reads every action.yml
```

**Efficient (~2–5k tokens):**

```
Edit("path/to/action.yml", old="description: foo", new="description: bar")
```

Reserve agents for structural work: add/remove/rename, cross-file refactors,
broad investigations. See the "Token Usage" section in the repo
[`CLAUDE.md`](../CLAUDE.md).

---

## 6. Run independent tool calls in parallel

Each tool call is a round trip. Batching them in one message keeps the
transcript shorter and the wall-clock faster.

**Expensive — three separate turns, each replays the full transcript:**

```
turn 1: Bash("git status")
turn 2: Bash("git diff")
turn 3: Bash("git log -5")
```

**Efficient — one turn, three parallel calls:**

```
turn 1: Bash("git status") + Bash("git diff") + Bash("git log -5")
```

---

## 7. Trust your edits — don't re-read to "verify"

After an `Edit` succeeds, the new content is known. Re-reading the file just to
look at your own change wastes tokens.

**Expensive:**

```
Edit(file, old, new)
Read(file)                           # "let me check it landed"
```

**Efficient:**

```
Edit(file, old, new)                 # the tool result already confirms success
```

Only re-read if you need to see *surrounding* code that the edit changed
indirectly.

---

## 8. Keep `Bash` output tight

Long-running commands (test suites, builds, `git log` without limits) can dump
megabytes of output that stay in context for the rest of the session.

**Expensive:**

```
Bash("npm test")                     # full output, every test name
Bash("git log")                      # full history
```

**Efficient:**

```
Bash("npm test 2>&1 | tail -50")
Bash("git log -10 --oneline")
```

When you only need pass/fail, redirect output and check the exit code.

---

## 9. `/clear` between unrelated tasks

Conversation history is replayed every turn. If you finish one task and start
something completely different, the old context is dead weight.

**Expensive:**

```
[debug auth issue for 30 turns]
[then in same session: "now let's update the README"]
```

The README work pays full cost for all 30 prior auth turns, every turn.

**Efficient:**

```
[debug auth issue for 30 turns]
/clear
[fresh session: "let's update the README"]
```

---

## 10. Mind the prompt cache (5-minute TTL)

The static prefix of your conversation (system prompt, `CLAUDE.md`, early turns)
is cached server-side. Cache hits cost ~10% of normal input tokens. The cache
expires after 5 minutes of inactivity.

**Expensive:** sleep / step away for 6–10 minutes, then resume — full prefix
re-billed at 100%.

**Efficient:** if you must pause briefly, keep it under 5 minutes. If you must
pause longer, commit to a real break (20+ minutes) — one cache miss buys a much
longer wait than spamming 5-minute pauses.

This is why background polling loops should sleep either **<270s** (cache stays
warm) or **>1200s** (amortize the miss). 300s is the worst-of-both choice.

---

## 11. Reference files by path; don't paste content

Pasting a long file into the chat puts it in context twice (once when you
paste, again because it stays in history).

**Expensive:**

```
"Here's the config file: <2000 lines pasted>"
```

**Efficient:**

```
"Look at config/app.yml — focus on the auth section"
```

Claude can `Read` it on demand, optionally with `offset`/`limit`.

---

## 12. Keep `CLAUDE.md` and `MEMORY.md` lean

Both files are loaded into *every* turn of *every* session. A 500-line
`CLAUDE.md` costs you those 500 lines on every single round trip.

**Expensive:** verbose preferences, long examples, FAQ-style explanations.

**Efficient:** terse rules with a one-line "why". Move long context into
`docs/*.md` files that get read on demand instead of loaded by default.

Keep `MEMORY.md` under ~200 lines — lines beyond the cap are truncated but
still count toward the load — and delete stale memories (completed projects,
outdated decisions) rather than letting them ride along every turn.

---

The remaining tips are for the **human driver** rather than the agent — they
shape how you steer a session, where the biggest savings actually come from.

## 13. `/compact` proactively, don't wait for the limit

Context accumulates with every message. `/compact` between focus shifts within
a session (e.g. every 2–3 commits) keeps the replayed transcript small instead
of dragging the full history into every later turn. Complements `/clear` (§9):
`/clear` for an unrelated task, `/compact` to trim within related work.

---

## 14. Drive in short, direct turns

Each round trip replays the whole transcript, so ambiguity is expensive:
"maybe we could rename this?" → "yes" → "should I commit?" → "yes" is four
turns for one change.

**Expensive:** vague, one-thing-at-a-time prompts that trigger clarifying
questions.

**Efficient:** state the outcome up front and batch related work — "Rename A,
B, C and commit" in one turn. Give known file paths, and prefer slash commands
(`/commit`) over re-explaining intent.

---

## 15. Close unused IDE tabs and deselect unrelated code

Open editor files and highlighted selections are sent as context on every
turn. Ten tabs open "just in case" is ten paths (and any selections) re-sent
each round trip.

**Efficient:** keep only the files you're working on open, and clear a
selection once it's no longer relevant to what you're asking.

---

## Quick reference

| Burns tokens                         | Cheaper alternative                                |
| ------------------------------------ | -------------------------------------------------- |
| `Read` whole file                    | `Grep` first, then `Read` with `offset`/`limit`    |
| `Bash("find ...")`, `Bash("ls -R")`  | `Glob`                                             |
| `Bash("cat file")`                   | `Read`                                             |
| `Bash("grep ...")`                   | `Grep` (with `head_limit`)                         |
| Agent for a 1-file edit              | `Edit` directly                                    |
| Sequential `Bash` calls              | Parallel calls in one message                      |
| `npm test` (full output)             | `npm test 2>&1 \| tail -50`                        |
| Re-reading after `Edit`              | Trust the edit result                              |
| Long session across unrelated tasks  | `/clear` between tasks                             |
| 6-minute pause                       | <5 minutes (cache warm) or >20 minutes (real break)|
| Pasting file content into chat       | Reference path, let Claude `Read` on demand        |
| 500-line `CLAUDE.md`                 | Terse rules + on-demand `docs/*.md`                |
| Vague, one-at-a-time prompts         | Direct, batched turns (state outcome up front)     |
| Many IDE tabs / stray selections     | Close unused tabs, deselect unrelated code         |
| Waiting for the context limit        | `/compact` proactively between focus shifts        |
