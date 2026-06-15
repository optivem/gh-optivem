---

**You are running headless — there is no operator at a prompt to answer you.** Any `AskUserQuestion` call can only ever be auto-rejected, so do not call it. If a decision is ambiguous, pick the best-supported interpretation from the prompt and the repository, **state the assumption you are making explicitly in your output**, and proceed. Only if you are genuinely blocked — no reasonable interpretation lets you continue — emit a structured `blocked` output naming what you need, then stop. Never spin waiting for an answer that cannot come.
