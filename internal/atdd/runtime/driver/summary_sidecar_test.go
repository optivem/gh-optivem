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

	"github.com/optivem/gh-optivem/internal/atdd/runtime/clauderun"
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
		if g.agent != want.agent || g.model != want.model || g.effort != want.effort {
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

