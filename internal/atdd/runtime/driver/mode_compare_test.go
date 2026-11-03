package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRunSidecar(t *testing.T, repo, ts string, lines ...string) {
	t.Helper()
	dir := filepath.Join(repo, ".gh-optivem", "runs", ts)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSidecarRoundTripsInnerLoopTags(t *testing.T) {
	r := dispatchRecord{agent: "a", innerLoopMode: "test-after", ticketID: "42"}
	got := recordFromJSON(recordToJSON(r))
	if got.innerLoopMode != "test-after" || got.ticketID != "42" {
		t.Fatalf("round trip lost tags: %+v", got)
	}
}

func TestCompareModes_GroupsRunsByMode(t *testing.T) {
	repo := t.TempDir()
	writeRunSidecar(t, repo, "20260101-000001",
		`{"agent":"system-implementer","elapsed_ns":10000000000,"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":1000,"total_cost_usd":1.0}}`)
	writeRunSidecar(t, repo, "20260101-000002",
		`{"agent":"system-implementer","inner_loop_mode":"test-after","ticket":"7","elapsed_ns":20000000000,"usage":{"input_tokens":200,"output_tokens":100,"cache_read_input_tokens":2000,"total_cost_usd":2.0}}`,
		`{"agent":"command-failed-fixer","inner_loop_mode":"test-after","attempt_number":1,"attempt_max":2,"elapsed_ns":10000000000}`)
	var out strings.Builder
	if err := CompareModes(&out, repo); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"none", "test-after", "fresh-tokens", "cached-tokens"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
	// test-after: 1 run, 2 dispatches, 300 fresh, 2000 cached, 2.00 cost, 1 fix loop, 30s.
	var row string
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(l, "test-after") {
			row = l
		}
	}
	for _, want := range []string{"2.0", "300", "2000", "2.00", "1.0", "30s"} {
		if !strings.Contains(row, want) {
			t.Errorf("test-after row missing %q: %s", want, row)
		}
	}
}

func TestCompareModes_NoRuns(t *testing.T) {
	if err := CompareModes(&strings.Builder{}, t.TempDir()); err == nil {
		t.Fatal("no runs dir must error")
	}
}
