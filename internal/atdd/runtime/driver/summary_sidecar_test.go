// Tests for the .gh-optivem/runs/<ts>/summary.jsonl sidecar — the
// durable twin of the in-memory agent-summary table. Covers the
// append/load round-trip, LatestRunDir's mtime ranking, the cobra-facing
// PrintSummaryFile entry point, and the integration with the dispatch
// site so a regression where the sidecar stops being written shows up
// here rather than at first manual `gh optivem run summary` failure.
package driver

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/atdd/process/clauderun"
)

// TestSummarySidecar_RoundTrip seeds three synthetic records (mixed
// usage / no-usage / failed), writes them via appendSummaryLine, reads
// them back via loadSummary, and asserts every per-row field survives
// the round trip. The error case checks the message only (errors.New
// has no identity across encode/decode), not the value.
func TestSummarySidecar_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summary.jsonl")

	in := []dispatchRecord{
		{
			agent:   "classify-ticket-subtype",
			model:   "sonnet",
			effort:  "medium",
			elapsed: 12 * time.Second,
			usage: &clauderun.TokenUsage{
				InputTokens: 2100, OutputTokens: 300, TotalCostUSD: 0.04,
			},
		},
		{
			agent:   "parse-ticket",
			model:   "sonnet",
			effort:  "low",
			elapsed: 8 * time.Second,
			// nil usage → "interactive mode" shape; must encode as
			// usage:omitted so loadSummary returns r.usage == nil.
		},
		{
			agent:   "fix-unexpected-failing-tests",
			model:   "opus",
			effort:  "max",
			elapsed: 42 * time.Second,
			usage: &clauderun.TokenUsage{
				InputTokens: 8200, OutputTokens: 1100, TotalCostUSD: 0.21,
			},
			err: errors.New("rate limit"),
		},
		{
			// Channel-unrolled dispatch: the channel must survive the
			// round trip so the replayed table can label which channel ran.
			agent:   "system-driver-adapter-implementer",
			channel: "api",
			model:   "opus",
			effort:  "medium",
			elapsed: 12 * time.Second,
		},
	}

	for _, r := range in {
		if err := appendSummaryLine(path, r); err != nil {
			t.Fatalf("appendSummaryLine: %v", err)
		}
	}

	got, err := loadSummary(path)
	if err != nil {
		t.Fatalf("loadSummary: %v", err)
	}
	if len(got) != len(in) {
		t.Fatalf("loadSummary len: got %d, want %d", len(got), len(in))
	}

	for i, want := range in {
		g := got[i]
		if g.agent != want.agent || g.channel != want.channel || g.model != want.model || g.effort != want.effort {
			t.Errorf("row %d identity mismatch: got %+v, want %+v", i, g, want)
		}
		if g.elapsed != want.elapsed {
			t.Errorf("row %d elapsed: got %v, want %v", i, g.elapsed, want.elapsed)
		}
		if (g.usage == nil) != (want.usage == nil) {
			t.Errorf("row %d usage presence mismatch: got %v, want %v", i, g.usage, want.usage)
		}
		if g.usage != nil && want.usage != nil {
			if g.usage.InputTokens != want.usage.InputTokens ||
				g.usage.OutputTokens != want.usage.OutputTokens ||
				g.usage.TotalCostUSD != want.usage.TotalCostUSD {
				t.Errorf("row %d usage: got %+v, want %+v", i, g.usage, want.usage)
			}
		}
		if (g.err == nil) != (want.err == nil) {
			t.Errorf("row %d err presence mismatch: got %v, want %v", i, g.err, want.err)
		}
		if g.err != nil && want.err != nil && g.err.Error() != want.err.Error() {
			t.Errorf("row %d err msg: got %q, want %q", i, g.err.Error(), want.err.Error())
		}
	}
}

// TestSummarySidecar_AppendCreatesParentDirs confirms the writer is
// resilient to a missing run-ts directory at first dispatch — the
// dispatcher's runs/<ts>/ tree is created lazily by writePromptLog and
// the sidecar must not race with that creation.
func TestSummarySidecar_AppendCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deeply", "nested", "run-ts", "summary.jsonl")
	if err := appendSummaryLine(path, dispatchRecord{
		agent: "a", model: "sonnet", effort: "low", elapsed: time.Second,
	}); err != nil {
		t.Fatalf("appendSummaryLine into missing dir: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected sidecar created at %s, got %v", path, err)
	}
}

// TestSummarySidecar_AppendEmptyPathIsNoOp mirrors the dispatchPaths
// nil-rs contract: an empty path means "no driver-managed runState"
// (test fixtures), and the writer skips silently rather than erroring.
func TestSummarySidecar_AppendEmptyPathIsNoOp(t *testing.T) {
	if err := appendSummaryLine("", dispatchRecord{agent: "x"}); err != nil {
		t.Errorf("empty path must be no-op, got err: %v", err)
	}
}

// TestSummarySidecar_LoadSkipsMalformedLines covers the forgiving stance
// — a single corrupt line (e.g. truncated due to filesystem flush race)
// must not invalidate the surrounding good lines.
func TestSummarySidecar_LoadSkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summary.jsonl")
	body := `{"agent":"good-1","model":"sonnet","effort":"low","elapsed_ns":1000000000}
{not valid json
{"agent":"good-2","model":"opus","effort":"high","elapsed_ns":2000000000}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := loadSummary(path)
	if err != nil {
		t.Fatalf("loadSummary: %v", err)
	}
	if len(got) != 2 || got[0].agent != "good-1" || got[1].agent != "good-2" {
		t.Errorf("loadSummary skipped wrong lines: %+v", got)
	}
}

// TestLatestRunDir_PicksMostRecentMtime seeds three dir entries with
// staggered mtimes and asserts the newest wins. Mirrors how operators
// will invoke `gh optivem run summary` immediately after a failed run.
func TestLatestRunDir_PicksMostRecentMtime(t *testing.T) {
	repo := t.TempDir()
	runs := filepath.Join(repo, ".gh-optivem", "runs")
	if err := os.MkdirAll(runs, 0o755); err != nil {
		t.Fatal(err)
	}
	names := []string{"20260101-000000", "20260601-000000", "20260301-000000"}
	for i, n := range names {
		p := filepath.Join(runs, n)
		if err := os.Mkdir(p, 0o755); err != nil {
			t.Fatal(err)
		}
		// Set mtime so name-sort order is NOT the same as mtime-sort
		// order — confirms LatestRunDir uses mtime, not lexical sort.
		mtime := time.Now().Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LatestRunDir(repo)
	if err != nil {
		t.Fatalf("LatestRunDir: %v", err)
	}
	want := filepath.Join(runs, "20260301-000000") // last in loop = newest mtime
	if got != want {
		t.Errorf("LatestRunDir: got %q, want %q", got, want)
	}
}

// TestLatestRunDir_NoRunsIsError confirms the empty-state user message
// path. Without this, callers see a confusing "" path instead of a
// clear "no runs found" message.
func TestLatestRunDir_NoRunsIsError(t *testing.T) {
	repo := t.TempDir()
	if _, err := LatestRunDir(repo); err == nil {
		t.Error("LatestRunDir on missing runs/ must return err, got nil")
	}
}

// TestLatestRunWithSummary_SkipsEmptyRuns is the UX-driving case: an
// operator who bombed a run at preflight (no agents dispatched → no
// sidecar) and then started a real run wants `gh optivem run summary`
// to land on the real run, not error on the empty one. Without this
// skip, the strict mtime-latest pick errors noisily and points at the
// wrong run.
func TestLatestRunWithSummary_SkipsEmptyRuns(t *testing.T) {
	repo := t.TempDir()
	runs := filepath.Join(repo, ".gh-optivem", "runs")
	if err := os.MkdirAll(runs, 0o755); err != nil {
		t.Fatal(err)
	}
	// Two runs: older has a sidecar, newer does not. The newer's
	// mtime is later so strict LatestRunDir picks it; the with-summary
	// variant must skip to the older one.
	older := filepath.Join(runs, "20260101-000000")
	newer := filepath.Join(runs, "20260601-000000")
	for _, d := range []string{older, newer} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(older, "summary.jsonl"),
		[]byte(`{"agent":"a","model":"sonnet","effort":"low","elapsed_ns":1000000000}`+"\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(older, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, now, now); err != nil {
		t.Fatal(err)
	}

	got, err := LatestRunWithSummary(repo)
	if err != nil {
		t.Fatalf("LatestRunWithSummary: %v", err)
	}
	if got != older {
		t.Errorf("LatestRunWithSummary picked %q, want %q (newer has no sidecar)", got, older)
	}
}

// TestLatestRunWithSummary_NoSidecarAnywhere confirms the error message
// surfaces every checked timestamp so the operator can see what was
// scanned. Without that context the error is opaque ("no runs?") when
// the operator can plainly see directories under runs/.
func TestLatestRunWithSummary_NoSidecarAnywhere(t *testing.T) {
	repo := t.TempDir()
	runs := filepath.Join(repo, ".gh-optivem", "runs")
	if err := os.MkdirAll(filepath.Join(runs, "20260101-000000"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(runs, "20260102-000000"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := LatestRunWithSummary(repo)
	if err == nil {
		t.Fatal("want err when no run has a sidecar, got nil")
	}
	for _, ts := range []string{"20260101-000000", "20260102-000000"} {
		if !strings.Contains(err.Error(), ts) {
			t.Errorf("err must list checked ts %q; got %v", ts, err)
		}
	}
}

// TestPrintSummaryFile_MatchesRenderAgentSummary is the single-source-of-
// truth check: PrintSummaryFile loading from disk must produce the same
// table as renderAgentSummary on the same records in memory. If the two
// drift, the live banner and the historical replay would show different
// tables — defeating the whole point of stage 2.
func TestPrintSummaryFile_MatchesRenderAgentSummary(t *testing.T) {
	records := []dispatchRecord{
		{
			agent: "classify-ticket-subtype", model: "sonnet", effort: "medium",
			elapsed: 12 * time.Second,
			usage:   &clauderun.TokenUsage{InputTokens: 2100, OutputTokens: 300, TotalCostUSD: 0.04},
		},
		{
			agent: "write-acceptance-tests", model: "opus", effort: "high",
			elapsed: 2*time.Minute + 31*time.Second,
			usage:   &clauderun.TokenUsage{InputTokens: 28500, OutputTokens: 4100, TotalCostUSD: 0.71},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "summary.jsonl")
	for _, r := range records {
		if err := appendSummaryLine(path, r); err != nil {
			t.Fatalf("appendSummaryLine: %v", err)
		}
	}

	var inMem, fromDisk bytes.Buffer
	renderAgentSummary(&inMem, records)
	if err := PrintSummaryFile(&fromDisk, path); err != nil {
		t.Fatalf("PrintSummaryFile: %v", err)
	}
	if inMem.String() != fromDisk.String() {
		t.Errorf("live banner and historical replay diverged.\nLIVE:\n%s\nDISK:\n%s",
			inMem.String(), fromDisk.String())
	}
}

// TestRenderAgentSummary_ChannelColumn asserts the channel-unrolled
// dispatches surface their channel in the table while channel-agnostic
// agents leave the cell blank — the whole point of the column is to tell
// the two otherwise-identical per-channel rows apart.
func TestRenderAgentSummary_ChannelColumn(t *testing.T) {
	records := []dispatchRecord{
		{agent: "acceptance-test-writer", model: "sonnet", effort: "medium", elapsed: time.Minute},
		{agent: "system-driver-adapter-implementer", channel: "api", model: "opus", effort: "medium", elapsed: 12 * time.Second},
		{agent: "system-driver-adapter-implementer", channel: "ui", model: "opus", effort: "medium", elapsed: 23 * time.Second},
	}

	var buf bytes.Buffer
	renderAgentSummary(&buf, records)
	got := buf.String()

	for _, want := range []string{"channel", "api", "ui"} {
		if !strings.Contains(got, want) {
			t.Errorf("table missing %q; got:\n%s", want, got)
		}
	}

	// The channel-agnostic row must not borrow a neighbour's channel: its
	// line carries the agent name but neither "api" nor "ui".
	for line := range strings.SplitSeq(got, "\n") {
		if strings.Contains(line, "acceptance-test-writer") {
			if strings.Contains(line, "api") || strings.Contains(line, "ui") {
				t.Errorf("channel-agnostic row leaked a channel cell: %q", line)
			}
		}
	}
}

// TestClaudeRunDispatch_WritesSummarySidecar is the wiring integration
// test: drive a real dispatch through the wrapped engine and assert
// a summary.jsonl was created and contains one row matching the
// in-memory record. Catches regressions where the dispatch site stops
// calling appendSummaryLine.
func TestClaudeRunDispatch_WritesSummarySidecar(t *testing.T) {
	oldNow := nowFn
	defer func() { nowFn = oldNow }()
	calls := 0
	nowFn = func() time.Time {
		calls++
		if calls == 1 {
			return time.Unix(1_700_000_000, 0)
		}
		return time.Unix(1_700_000_009, 0) // +9s
	}

	usage := &clauderun.TokenUsage{InputTokens: 5000, OutputTokens: 800, TotalCostUSD: 0.12}
	claudeFake := &fakeClaude{result: clauderun.RunResult{Usage: usage}}
	gitFake := &fakeGit{out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")}}
	opts := newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake})
	opts.Stdout = io.Discard

	repo := t.TempDir()
	rs := &runState{runTimestamp: "20260528-150000", repoPath: repo}
	fn := buildEngineWithRunState(t, opts, defaultTestConfig(), rs)

	if out := fn(newCtxWithIssue()); out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}

	sidecarPath := filepath.Join(repo, ".gh-optivem", "runs", "20260528-150000", "summary.jsonl")
	body, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatalf("expected summary sidecar at %s: %v", sidecarPath, err)
	}
	if !strings.Contains(string(body), `"agent":"acceptance-test-writer"`) {
		t.Errorf("sidecar missing recorded agent identity; body:\n%s", string(body))
	}
	if !strings.Contains(string(body), `"input_tokens":5000`) {
		t.Errorf("sidecar missing recorded usage; body:\n%s", string(body))
	}

	// Round-trip end to end: cobra subcommand would read this file via
	// PrintSummaryFile, so confirm that path works.
	var buf bytes.Buffer
	if err := PrintSummaryFile(&buf, sidecarPath); err != nil {
		t.Fatalf("PrintSummaryFile: %v", err)
	}
	if !strings.Contains(buf.String(), "acceptance-test-writer") ||
		!strings.Contains(buf.String(), "5.0k") {
		t.Errorf("replay table missing expected cells:\n%s", buf.String())
	}
}

// ---------------------------------------------------------------------------
// Run digest (summary.md) — renderer + writer
// ---------------------------------------------------------------------------

// TestRenderRunDigest_PassingRunWithUsage covers the happy path: a
// succeeded run with a ticket header, description + acceptance-criteria
// blockquotes, and the agent table fenced for Markdown. Asserts the
// verdict line, the blockquote prefixing, the fenced table, and that the
// reused renderAgentSummary cells appear inside the fence.
func TestRenderRunDigest_PassingRunWithUsage(t *testing.T) {
	d := runDigest{
		issueNum:           "42",
		title:              "Add PUT /carts/{id}/items endpoint",
		url:                "https://github.com/optivem/shop/issues/42",
		description:        "Shoppers can update item quantity.\nIdempotent on repeat calls.",
		acceptanceCriteria: "Scenario: update quantity\n  Given a cart\n  Then the quantity changes",
		records: []dispatchRecord{
			{
				agent: "write-acceptance-tests", model: "opus", effort: "high",
				elapsed: 2*time.Minute + 31*time.Second,
				usage:   &clauderun.TokenUsage{InputTokens: 28000, OutputTokens: 4100, TotalCostUSD: 0.71},
			},
		},
	}

	var buf bytes.Buffer
	renderRunDigest(&buf, d)
	got := buf.String()

	for _, want := range []string{
		"# Run digest — #42 Add PUT /carts/{id}/items endpoint",
		"**Result:** ✅ succeeded",
		"**Ticket:** https://github.com/optivem/shop/issues/42",
		"## Description",
		"> Shoppers can update item quantity.",
		"> Idempotent on repeat calls.",
		"## Acceptance criteria",
		"> Scenario: update quantity",
		"## Agents dispatched",
		"=== Agent summary ===", // reused renderAgentSummary body
		"write-acceptance-tests",
		"2m31s",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("digest missing %q; got:\n%s", want, got)
		}
	}
	// The table must be fenced so columns survive Markdown rendering: at
	// least two ``` fence lines around the summary block.
	if strings.Count(got, "```") < 2 {
		t.Errorf("agent table not fenced; got:\n%s", got)
	}
}

// TestRenderRunDigest_FailedRun asserts the verdict line carries the
// engine error string on a failed run.
func TestRenderRunDigest_FailedRun(t *testing.T) {
	d := runDigest{
		issueNum: "7",
		title:    "Broken thing",
		url:      "https://example.com/7",
		records: []dispatchRecord{
			{agent: "system-implementer", model: "opus", effort: "max", elapsed: 30 * time.Second,
				err: errors.New("dispatch failed")},
		},
		result: errors.New("node IMPLEMENT busted: exit status 1"),
	}

	var buf bytes.Buffer
	renderRunDigest(&buf, d)
	got := buf.String()

	if !strings.Contains(got, "**Result:** ❌ failed: node IMPLEMENT busted: exit status 1") {
		t.Errorf("failed digest missing verdict line; got:\n%s", got)
	}
	// The failed dispatch row keeps its ✗ marker (renderAgentSummary
	// contract) inside the fenced table.
	if !strings.Contains(got, "✗ system-implementer") {
		t.Errorf("failed dispatch row missing ✗ marker; got:\n%s", got)
	}
}

// TestRenderRunDigest_NoDispatches covers a run that produced no agent
// dispatches (bombed in setup, or a pure-gate walk): the digest still
// renders the header + verdict and an explicit "no agents" note instead
// of an empty fenced block.
func TestRenderRunDigest_NoDispatches(t *testing.T) {
	var buf bytes.Buffer
	renderRunDigest(&buf, runDigest{issueNum: "1", title: "Nothing ran"})
	got := buf.String()

	if !strings.Contains(got, "## Agents dispatched") {
		t.Errorf("digest missing agents section; got:\n%s", got)
	}
	if !strings.Contains(got, "_No agents were dispatched._") {
		t.Errorf("no-dispatch digest must note the absence; got:\n%s", got)
	}
	if strings.Contains(got, "```") {
		t.Errorf("no-dispatch digest must not fence an empty table; got:\n%s", got)
	}
}

// TestWriteRunDigest_RoundTrip writes a digest to disk, then reads it
// back through the cobra-facing PrintRunDigestFile to confirm the live
// emission and the replay are byte-identical (same single-renderer
// guarantee renderAgentSummary/PrintSummaryFile honour).
func TestWriteRunDigest_RoundTrip(t *testing.T) {
	rs := &runState{runTimestamp: "20260530-160000", repoPath: t.TempDir()}
	path := rs.summaryMarkdownPath()
	d := runDigest{
		issueNum: "42", title: "Round trip", url: "https://example.com/42",
		records: []dispatchRecord{
			{agent: "a", model: "sonnet", effort: "low", elapsed: 5 * time.Second},
		},
	}
	if err := writeRunDigest(path, d); err != nil {
		t.Fatalf("writeRunDigest: %v", err)
	}

	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read digest: %v", err)
	}
	var inMem bytes.Buffer
	renderRunDigest(&inMem, d)
	if string(onDisk) != inMem.String() {
		t.Errorf("on-disk digest diverged from renderer:\nDISK:\n%s\nMEM:\n%s", string(onDisk), inMem.String())
	}

	var replay bytes.Buffer
	if err := PrintRunDigestFile(&replay, path); err != nil {
		t.Fatalf("PrintRunDigestFile: %v", err)
	}
	if replay.String() != string(onDisk) {
		t.Errorf("PrintRunDigestFile diverged from file bytes")
	}
}

// TestWriteRunDigest_EmptyPathIsNoOp confirms the nil-runState contract:
// summaryMarkdownPath returns "" and writeRunDigest no-ops without error.
func TestWriteRunDigest_EmptyPathIsNoOp(t *testing.T) {
	var nilRS *runState
	if p := nilRS.summaryMarkdownPath(); p != "" {
		t.Errorf("nil runState must yield empty digest path, got %q", p)
	}
	if err := writeRunDigest("", runDigest{}); err != nil {
		t.Errorf("empty path must be a no-op, got %v", err)
	}
}
