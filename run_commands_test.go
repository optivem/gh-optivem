package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// chdir switches into dir for the duration of the test, restoring the
// original working directory on cleanup. `gh optivem run summary` reads
// the run tree relative to cwd, so the cobra-level tests pin cwd here.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

// seedRun writes summary.jsonl and summary.md into
// <repo>/.gh-optivem/runs/<ts>/ so the run summary subcommand can locate
// and replay them.
func seedRun(t *testing.T, repo, ts, jsonl, markdown string) {
	t.Helper()
	dir := filepath.Join(repo, ".gh-optivem", "runs", ts)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write summary.jsonl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.md"), []byte(markdown), 0o644); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}
}

// TestRunSummary_MarkdownFlag_PrintsDigest asserts `gh optivem run
// summary <ts> --markdown` prints the emitted summary.md verbatim rather
// than the table.
func TestRunSummary_MarkdownFlag_PrintsDigest(t *testing.T) {
	repo := t.TempDir()
	const ts = "20260530-160000"
	digest := "# Run digest — #42 Add endpoint\n\n**Result:** ✅ succeeded\n"
	seedRun(t, repo, ts,
		`{"agent":"write-acceptance-tests","model":"opus","effort":"high","elapsed_ns":1000000000}`+"\n",
		digest)
	chdir(t, repo)

	cmd := newRunSummaryCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{ts, "--markdown"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if buf.String() != digest {
		t.Errorf("markdown output mismatch:\ngot:\n%s\nwant:\n%s", buf.String(), digest)
	}
}

// TestRunSummary_NoFlag_PrintsTable asserts the default (no --markdown)
// path still renders the agent-summary table from the JSONL sidecar,
// unchanged by the new flag.
func TestRunSummary_NoFlag_PrintsTable(t *testing.T) {
	repo := t.TempDir()
	const ts = "20260530-160000"
	seedRun(t, repo, ts,
		`{"agent":"write-acceptance-tests","model":"opus","effort":"high","elapsed_ns":1000000000}`+"\n",
		"# Run digest\n")
	chdir(t, repo)

	cmd := newRunSummaryCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{ts})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "=== Agent summary ===") || !strings.Contains(got, "write-acceptance-tests") {
		t.Errorf("default path must render the table; got:\n%s", got)
	}
	if strings.Contains(got, "# Run digest") {
		t.Errorf("default path must not print the markdown digest; got:\n%s", got)
	}
}
