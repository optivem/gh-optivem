package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// outputFile creates a fresh temp file path inside t.TempDir() that
// runOutputWrite is expected to create on first append.
func outputFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "agent.outputs.jsonl")
}

// readLines reads the JSONL file at path and returns each line decoded
// as a generic map. Empty files return zero lines.
func readLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out []map[string]any
	for raw := range strings.SplitSeq(strings.TrimRight(string(data), "\n"), "\n") {
		if raw == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatalf("decode line %q: %v", raw, err)
		}
		out = append(out, m)
	}
	return out
}

// TestRunOutputWriteMissingFileEnv asserts that an empty/unset
// GH_OPTIVEM_OUTPUT_FILE fails with the dispatch-required message — the
// CLI is unusable outside an agent subprocess.
func TestRunOutputWriteMissingFileEnv(t *testing.T) {
	cases := []struct {
		name    string
		hasFile bool
		path    string
	}{
		{"unset", false, ""},
		{"empty string", true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := runOutputWrite([]string{"foo=bar"}, c.path, c.hasFile, "foo:string", true)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), envOutputFile) {
				t.Errorf("error %q should mention %s", err, envOutputFile)
			}
		})
	}
}

// TestRunOutputWriteMissingKeysEnv asserts that an unset
// GH_OPTIVEM_OUTPUT_KEYS fails with the no-outputs-declared message. An
// empty allow-list (env set, value "") is a different state and is
// covered by TestRunOutputWriteEmptyAllowList.
func TestRunOutputWriteMissingKeysEnv(t *testing.T) {
	err := runOutputWrite([]string{"foo=bar"}, outputFile(t), true, "", false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no outputs declared") {
		t.Errorf("error %q should say 'no outputs declared'", err)
	}
}

// TestRunOutputWriteEmptyAllowList asserts that GH_OPTIVEM_OUTPUT_KEYS
// set to the empty string (allow-list configured but empty) rejects
// every key as unknown. Distinct from the unset case in
// TestRunOutputWriteMissingKeysEnv.
func TestRunOutputWriteEmptyAllowList(t *testing.T) {
	err := runOutputWrite([]string{"foo=bar"}, outputFile(t), true, "", true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown output key") {
		t.Errorf("error %q should say 'unknown output key'", err)
	}
}

// TestRunOutputWriteUnknownKey asserts that a key not present in the
// allow-list fails with a list of declared keys in the message, so the
// agent can self-correct without re-reading the prompt.
func TestRunOutputWriteUnknownKey(t *testing.T) {
	err := runOutputWrite(
		[]string{"typo-key=true"},
		outputFile(t),
		true,
		"dsl-port-changed:bool,test-names:string-list",
		true,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "typo-key") {
		t.Errorf("error %q should mention the typo'd key", err)
	}
	if !strings.Contains(err.Error(), "dsl-port-changed") || !strings.Contains(err.Error(), "test-names") {
		t.Errorf("error %q should list declared keys", err)
	}
}

// TestRunOutputWriteCoercionFailures covers the per-type coercion
// errors: a value that doesn't parse as the declared type must reject
// non-zero so the agent sees the failure mid-run.
func TestRunOutputWriteCoercionFailures(t *testing.T) {
	cases := []struct {
		name string
		arg  string
		keys string
	}{
		{"bool got string", "dsl-port-changed=notabool", "dsl-port-changed:bool"},
		{"bool got integer", "dsl-port-changed=1", "dsl-port-changed:bool"},
		{"bool got capitalized", "dsl-port-changed=True", "dsl-port-changed:bool"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := runOutputWrite([]string{c.arg}, outputFile(t), true, c.keys, true)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "expects bool") {
				t.Errorf("error %q should say 'expects bool'", err)
			}
		})
	}
}

// TestRunOutputWriteSingleKey asserts the simple happy path: one
// KEY=VALUE writes one line with one field, correctly coerced.
func TestRunOutputWriteSingleKey(t *testing.T) {
	path := outputFile(t)
	err := runOutputWrite(
		[]string{"dsl-port-changed=true"},
		path, true,
		"dsl-port-changed:bool",
		true,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	want := map[string]any{"dsl-port-changed": true}
	if !reflect.DeepEqual(lines[0], want) {
		t.Errorf("line[0] = %v, want %v", lines[0], want)
	}
}

// TestRunOutputWriteMultiKeyOneLine asserts that a single invocation
// with multiple KEY=VALUE arguments writes ONE combined JSONL line —
// preserving the agent's emit-intent (these values were emitted
// together).
func TestRunOutputWriteMultiKeyOneLine(t *testing.T) {
	path := outputFile(t)
	err := runOutputWrite(
		[]string{"test-names=foo,bar", "dsl-port-changed=false"},
		path, true,
		"dsl-port-changed:bool,test-names:string-list",
		true,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	// JSON arrays decode to []any with string elements, not []string.
	want := map[string]any{
		"dsl-port-changed": false,
		"test-names":       []any{"foo", "bar"},
	}
	if !reflect.DeepEqual(lines[0], want) {
		t.Errorf("line[0] = %v, want %v", lines[0], want)
	}
}

// TestRunOutputWriteStringList covers `string-list` coercion: comma
// splits, surrounding whitespace trims, and an empty value yields an
// empty list (not nil).
func TestRunOutputWriteStringList(t *testing.T) {
	cases := []struct {
		name string
		arg  string
		want []any
	}{
		{"single name", "test-names=onlyOne", []any{"onlyOne"}},
		{"two names", "test-names=foo,bar", []any{"foo", "bar"}},
		{"whitespace trimmed", "test-names=foo, bar , baz", []any{"foo", "bar", "baz"}},
		{"empty list", "test-names=", []any{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := outputFile(t)
			err := runOutputWrite([]string{c.arg}, path, true, "test-names:string-list", true)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			lines := readLines(t, path)
			if len(lines) != 1 {
				t.Fatalf("got %d lines, want 1", len(lines))
			}
			got, ok := lines[0]["test-names"].([]any)
			if !ok {
				t.Fatalf("test-names is not a JSON array: %v", lines[0]["test-names"])
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("test-names = %v, want %v", got, c.want)
			}
		})
	}
}

// TestRunOutputWriteAppendSemantics asserts that two separate
// invocations against the same file produce two JSONL lines, in order
// — i.e. the dispatcher's reader sees both emissions and applies
// last-write-wins per key across lines.
func TestRunOutputWriteAppendSemantics(t *testing.T) {
	path := outputFile(t)
	keys := "dsl-port-changed:bool,test-names:string-list"
	if err := runOutputWrite([]string{"test-names=alpha"}, path, true, keys, true); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := runOutputWrite([]string{"dsl-port-changed=true"}, path, true, keys, true); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if err := runOutputWrite([]string{"test-names=beta,gamma"}, path, true, keys, true); err != nil {
		t.Fatalf("third call: %v", err)
	}
	lines := readLines(t, path)
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	if lines[0]["test-names"].([]any)[0] != "alpha" {
		t.Errorf("line[0].test-names[0] = %v, want alpha", lines[0]["test-names"])
	}
	if lines[1]["dsl-port-changed"] != true {
		t.Errorf("line[1].dsl-port-changed = %v, want true", lines[1]["dsl-port-changed"])
	}
	got := lines[2]["test-names"].([]any)
	if !reflect.DeepEqual(got, []any{"beta", "gamma"}) {
		t.Errorf("line[2].test-names = %v, want [beta gamma]", got)
	}
}

// TestRunOutputWriteScopeException exercises the scope-exception
// envelope path: emitting `scope-exception-files` (string-list) and
// `scope-exception-reason` (string) in one combined call. The keys are
// first-class declared outputs after this plan's BPMN-SSoT change, so
// they share the same channel as any other output.
func TestRunOutputWriteScopeException(t *testing.T) {
	path := outputFile(t)
	err := runOutputWrite(
		[]string{
			"scope-exception-files=src/foo.ts,src/bar.ts",
			"scope-exception-reason=acceptance test referenced helper outside writer scope",
		},
		path, true,
		"scope-exception-files:string-list,scope-exception-reason:string",
		true,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	want := map[string]any{
		"scope-exception-files":  []any{"src/foo.ts", "src/bar.ts"},
		"scope-exception-reason": "acceptance test referenced helper outside writer scope",
	}
	if !reflect.DeepEqual(lines[0], want) {
		t.Errorf("line[0] = %v, want %v", lines[0], want)
	}
}

// TestRunOutputWriteDuplicateKeyInOneCall asserts that the same key
// repeated in a single invocation is rejected — last-write-wins is the
// across-calls semantic; within a single call the agent's intent is
// ambiguous and we'd rather fail loud.
func TestRunOutputWriteDuplicateKeyInOneCall(t *testing.T) {
	err := runOutputWrite(
		[]string{"dsl-port-changed=true", "dsl-port-changed=false"},
		outputFile(t), true,
		"dsl-port-changed:bool",
		true,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate key") {
		t.Errorf("error %q should say 'duplicate key'", err)
	}
}

// TestRunOutputWriteMalformedArgument covers the parse-side rejections
// for KEY=VALUE: empty arg, leading `=`, missing `=`.
func TestRunOutputWriteMalformedArgument(t *testing.T) {
	cases := []string{
		"no-equals-here",
		"=bar",
		"",
	}
	for _, arg := range cases {
		t.Run(arg, func(t *testing.T) {
			err := runOutputWrite(
				[]string{arg},
				outputFile(t), true,
				"foo:string",
				true,
			)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "malformed KEY=VALUE") {
				t.Errorf("error %q should say 'malformed KEY=VALUE'", err)
			}
		})
	}
}

// TestParseOutputKeysSpec covers the env-var parsing layer in
// isolation — malformed entries, unknown types, duplicate keys, and
// the empty-spec path.
func TestParseOutputKeysSpec(t *testing.T) {
	cases := []struct {
		name      string
		spec      string
		wantOK    bool
		wantOrder []string
		wantMap   map[string]string
		wantErr   string
	}{
		{
			name:      "empty spec",
			spec:      "",
			wantOK:    true,
			wantOrder: nil,
			wantMap:   map[string]string{},
		},
		{
			name:      "single entry",
			spec:      "dsl-port-changed:bool",
			wantOK:    true,
			wantOrder: []string{"dsl-port-changed"},
			wantMap:   map[string]string{"dsl-port-changed": "bool"},
		},
		{
			name:   "multiple entries preserve order",
			spec:   "dsl-port-changed:bool,test-names:string-list,scope-exception-reason:string",
			wantOK: true,
			wantOrder: []string{
				"dsl-port-changed",
				"test-names",
				"scope-exception-reason",
			},
			wantMap: map[string]string{
				"dsl-port-changed":       "bool",
				"test-names":             "string-list",
				"scope-exception-reason": "string",
			},
		},
		{
			name:    "missing colon",
			spec:    "no-colon-here",
			wantErr: "malformed",
		},
		{
			name:    "empty type",
			spec:    "key:",
			wantErr: "malformed",
		},
		{
			name:    "empty key",
			spec:    ":bool",
			wantErr: "malformed",
		},
		{
			name:    "unknown type",
			spec:    "key:integer",
			wantErr: "unknown type",
		},
		{
			name:    "duplicate key",
			spec:    "key:string,key:bool",
			wantErr: "duplicate key",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotMap, gotOrder, err := parseOutputKeysSpec(c.spec)
			if c.wantOK {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !reflect.DeepEqual(gotMap, c.wantMap) {
					t.Errorf("map = %v, want %v", gotMap, c.wantMap)
				}
				if !reflect.DeepEqual(gotOrder, c.wantOrder) {
					t.Errorf("order = %v, want %v", gotOrder, c.wantOrder)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %q, got nil", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error %q should contain %q", err, c.wantErr)
			}
		})
	}
}
