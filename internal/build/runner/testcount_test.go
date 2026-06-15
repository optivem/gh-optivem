package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCountExecutedTests covers the per-format parsers and the missing-file
// rule. Each fixture is written to a temp dir so the extension dispatch and
// directory-glob paths are exercised against real files.
func TestCountExecutedTests(t *testing.T) {
	trxOne := `<?xml version="1.0" encoding="UTF-8"?>
<TestRun id="x" xmlns="http://microsoft.com/schemas/VisualStudio/TeamTest/2010">
  <ResultSummary outcome="Completed">
    <Counters total="1" executed="1" passed="1" failed="0" />
  </ResultSummary>
</TestRun>`
	trxZero := `<?xml version="1.0" encoding="UTF-8"?>
<TestRun id="x" xmlns="http://microsoft.com/schemas/VisualStudio/TeamTest/2010">
  <ResultSummary outcome="Completed">
    <Counters total="0" executed="0" passed="0" failed="0" />
  </ResultSummary>
</TestRun>`
	junitBare := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.FooTest" tests="3" skipped="1" failures="0" errors="0" />`
	junitWrapped := `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="A" tests="2" skipped="0" />
  <testsuite name="B" tests="2" skipped="1" />
</testsuites>`
	pwJSON := `{"stats":{"expected":4,"unexpected":1,"flaky":0,"skipped":2}}`
	pwJSONZero := `{"stats":{"expected":0,"unexpected":0,"flaky":0,"skipped":0}}`

	cases := []struct {
		name    string
		file    string // file name (ext drives dispatch); "" => path that doesn't exist
		content string
		want    int
		wantErr bool
	}{
		{name: "trx one executed", file: "r.trx", content: trxOne, want: 1},
		{name: "trx zero executed", file: "r.trx", content: trxZero, want: 0},
		{name: "junit bare suite tests minus skipped", file: "r.xml", content: junitBare, want: 2},
		{name: "junit wrapped suites summed", file: "r.xml", content: junitWrapped, want: 3},
		{name: "playwright json executed", file: "r.json", content: pwJSON, want: 5},
		{name: "playwright json zero", file: "r.json", content: pwJSONZero, want: 0},
		{name: "missing file counts as zero", file: "", content: "", want: 0},
		{name: "malformed trx errors", file: "bad.trx", content: "<TestRun><not-closed>", wantErr: true},
		{name: "unsupported extension errors", file: "r.txt", content: "whatever", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "does-not-exist.trx")
			if tc.file != "" {
				path = filepath.Join(dir, tc.file)
				if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := countExecutedTests(path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got count %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("count: got %d, want %d", got, tc.want)
			}
		})
	}
}

// TestCountExecutedTests_JUnitDir exercises the directory path: gradle writes
// one TEST-*.xml per class, so a directory of bare-root files must sum.
func TestCountExecutedTests_JUnitDir(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"TEST-A.xml": `<testsuite name="A" tests="2" skipped="0" />`,
		"TEST-B.xml": `<testsuite name="B" tests="3" skipped="1" />`,
		"ignore.txt": "not xml",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := countExecutedTests(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := 4; got != want { // (2-0) + (3-1)
		t.Fatalf("count: got %d, want %d", got, want)
	}
}
