# 2026-06-19 21:32:00 UTC — Fix GetString/coerceStateValue []string parity (rehearsal #72 false halt)

## TL;DR

**Why:** Rehearsal #72 false-halted at `VALIDATE_CHANNELS_REGISTERED` claiming three correctly-authored acceptance tests "ran in none of the configured channels". Root cause: `Context.GetString` renders a `[]string` State value as `"[a b c]"` (bracketed, space-separated), while its sibling `coerceStateValue` joins on `,`. The gate read `at-test-names` via `GetString`, comma-split the bracketed blob into one junk token, matched nothing, and flagged every test as an orphan.
**End result:** `GetString` and `coerceStateValue` share one coercion helper, so a `[]string` renders identically (comma-joined) on both the State-read and substitution paths; `splitTestNames` is hardened to fail loud rather than silently emit a junk token; and a correctly-authored multi-channel acceptance test never false-halts at this gate again.

## Outcomes

What we get out of this — the goals and deliverables:

- `Context.GetString("at-test-names")` returns `"a,b,c"` for a `[]string{"a","b","c"}` State value — identical to what `coerceStateValue`/`ExpandParams` produces — so `validateChannelsRegistered` parses the real method names and passes when they are in the RED report.
- A single shared coercion helper backs **both** `GetString` (engine/statemachine/context.go) and `coerceStateValue` (engine/statemachine/run.go), so the two stringify paths can never silently diverge on slice rendering again.
- `splitTestNames` (atdd/process/actions/channel.go) is tolerant/loud: a bracketed or whitespace-separated input is parsed correctly (or rejected loudly), never collapsed into one unmatchable token.
- Regression tests lock the invariant: (a) `GetString` comma-joins a `[]string` identically to `coerceStateValue`; (b) `validateChannelsRegistered` passes when `at-test-names` is landed as a `[]string` (the production shape) whose names appear in the report.
- `scripts/test.sh` (or `-p 2`-scoped) green on the touched packages.

## ▶ Next executable step (resume here)

Step 1: in `internal/engine/statemachine/run.go`, extract the `coerceStateValue` body into a single exported-within-package helper (e.g. `coerceValueToString`) and have `coerceStateValue` delegate to it; then in `internal/engine/statemachine/context.go`, change `GetString`'s `default` branch to call that same helper so a `[]string` is comma-joined. Both files are in package `statemachine`, so no new import cycle. Stop after the engine change compiles; it unblocks the channel.go hardening and the regression tests.

## Steps

- [ ] Step 1 (engine — the fix): Extract one shared value→string coercion helper in `internal/engine/statemachine/` used by both `coerceStateValue` (run.go) and `GetString` (context.go). The helper keeps the existing `string` / `bool` / `[]string`→`strings.Join(",")` arms and a `fmt.Sprint` fallback. After this, `GetString` on a `[]string` yields the comma-joined form. Preserve `GetString`'s existing contract for `string`/`bool` (predicate `==`/`in` callers depend on `"true"`/`"false"`).
- [ ] Step 2 (engine — decide `[]any` handling): Confirm whether string-list outputs land in State as `[]string` or `[]any` (JSON-decoded array → likely `[]any`). If `[]any`-of-strings can reach `GetString`, the shared helper must handle it too (join element-wise). Pin this against how `validate-outputs-and-scopes` lands a `string-list` output before finalizing the helper's type switch. (Resolve in Open questions first.)
- [ ] Step 3 (action — harden `splitTestNames`, channel.go:159): Make it tolerant and/or loud — strip a surrounding `[...]`, split on commas-or-whitespace, drop tokens that aren't valid method identifiers (`[A-Za-z_$][A-Za-z0-9_$]*`). Belt-and-suspenders so a future upstream stringify slip fails loud (or parses correctly) instead of producing one unmatchable token. Keep it a pure helper (no behavior change for the already-correct comma-joined input).
- [ ] Step 4 (test — engine parity): Add a `GetString` unit test asserting it comma-joins a `[]string` (and `[]any`-of-strings if Step 2 says so) identically to `coerceStateValue` for the same value — locks the two paths together.
- [ ] Step 5 (test — gate regression): Adjust/add a `channel_test.go` case that lands `at-test-names` as a `[]string` (the production shape — today `channel_test.go:121` uses a plain single-name `string`, which masked the bug) and asserts `validateChannelsRegistered` passes when those names are in the report, and still errors when a name is genuinely absent.
- [ ] Step 6 (test — splitTestNames): Unit-test the hardened `splitTestNames` against comma-joined, bracketed-space-separated, and mixed inputs.
- [ ] Step 7 (verify): Run the touched packages green via `scripts/test.sh` or `go test -p 2 ./internal/engine/statemachine/... ./internal/atdd/process/actions/...` (never unbounded `go test ./...` on Windows).

## Open questions

- **`[]string` vs `[]any` (blocks Step 1's type switch):** Does a `string-list` output (e.g. `test-names`) land in `ctx.State` as `[]string` or `[]any`? The bindings tests reference both (`[]any{"foo","bar"}` in output_commands_test.go; `[]string` assertions in bindings_test.go:1916). The shared helper must cover whatever actually reaches `GetString` at runtime. Resolve by reading the landing path in `validate-outputs-and-scopes` / `landingStateKey` before writing the helper.
- **Helper scope:** Should the shared helper also replace the `fmt.Sprint` fallbacks elsewhere, or stay minimal (string/bool/[]string/[]any + fallback)? Default: minimal — only what's needed for parity; no opportunistic broadening (per "materialize ≠ expand").
- **`splitTestNames` loud-vs-tolerant:** Reject a bracketed token as a hard error, or silently strip and parse it? Given the engine fix already eliminates the bracketed input in production, lean tolerant-parse (strip + split-on-comma-or-space) so it's a pure safety net, not a new failure mode — but confirm.
