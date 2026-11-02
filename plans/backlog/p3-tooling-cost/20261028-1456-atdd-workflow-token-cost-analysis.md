# 2026-10-28 14:56:09 UTC — Whole-workflow token cost analysis of the ATDD flow

## TL;DR

**Why:** The ATDD flow has many substeps, each a fresh headless `claude -p` dispatch, so it is unclear whether the structure is token-efficient or expensive. Optimizing one phase (e.g. plan-before-execute in green) without knowing where the spend actually is risks tuning the wrong thing.
**End result:** A measured, ranked picture of token/cost spend across the entire workflow (every phase, agent type, fixer loop and retry), plus a short list of data-backed optimizations — each adopted or explicitly rejected.

## Outcomes

- Cost per agent type and per phase across full ATDD runs (tokens in/out, cache read/creation, `total_cost_usd`, dispatch count, retry count).
- A ranked list of top spenders and a clear answer to "is the many-substeps structure cheap or expensive?".
- Quantified effect of cold starts (repeated repo re-reading per dispatch), fixer/retry loops, and shared-preamble repetition.
- Data-backed decisions on candidate optimizations (below), recorded either as prompt/flow changes or as rejected findings.
- A repeatable way to re-run the measurement later.

## ▶ Next executable step (resume here)

Step 1: locate where headless dispatches write their stream-json events logs (`EventsLogPath` / `EventsTextLogPath` in `internal/atdd/process/clauderun/clauderun.go`, and where the driver in `internal/atdd/runtime/driver/driver.go` points them), and confirm the terminal `result` event carries `usage` + `total_cost_usd` with the agent name. Read-only. Stop with a short note in this plan on log location, format, and how many recent full runs are available.

## Steps

- [ ] Step 1: Locate the events logs and confirm per-dispatch fields (agent, model, usage incl. cache tokens, `total_cost_usd`, retries). If runs don't log enough to attribute cost per agent/phase, note the gap (a small logging change would become Step 1b).
- [ ] Step 2: Write a throwaway aggregation script (scratch, not committed unless it proves worth keeping) that totals cost/tokens/dispatch count per agent type, per phase, and per run.
- [ ] Step 3: Run it over several recent full runs. Produce a ranked table of spend, split into: first-pass writer/implementer dispatches vs. fixer dispatches vs. retries (transient-error retries and dispatch retry budget).
- [ ] Step 4: Quantify structural overhead: input tokens spent re-reading repo files per dispatch (cold starts), shared preamble/prompt size × dispatch count, and cache hit rate.
- [ ] Step 5: Evaluate candidates against the data, only for those that target a top spender:
  - Plan-before-execute (green, and other phases where rework is high) — same-turn bounded plan vs. none.
  - Trim prompt bodies and shared chunks (feeds `runtime-prompts-audit` / `token-usage-audit`).
  - Merge or batch tiny substeps that each pay a cold start.
  - Pass pre-resolved context (relevant file contents/paths) into the dispatch instead of letting the agent explore.
  - Cap exploration (read only files the failing test points at); trim test output returned to agents.
  - Downgrade opus fixers to sonnet where fix success rate allows; tune `--effort`.
  - Reduce fix-loop frequency by catching causes earlier (static gates before dispatch).
- [ ] Step 6: Decide per candidate (adopt / reject / measure further). Implement the adopted ones as separate follow-up plans in this repo; record rejected findings in the repo docs so they aren't re-litigated.

## Open questions

- Are there enough recent full-run events logs on disk to measure from, or do runs need to be executed first (cost and time)? Recommendation: use existing logs first; run new ones only if coverage of fixer paths is thin.
- Is cost measured in dollars (`total_cost_usd`), tokens, or both? Recommendation: both — dollars for prioritizing (opus is pricier), tokens for context/latency effects.
- Include wall-clock time and first-pass success rate as secondary metrics? Recommendation: yes, since an optimization that saves tokens but lowers success rate is a net loss.
- Inferred scope: all ATDD phases and all 20 agents in `internal/atdd/assets/runtime/agents/atdd/`; confirm nothing outside the ATDD flow should be included.
