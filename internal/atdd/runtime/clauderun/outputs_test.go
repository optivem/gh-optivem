package clauderun

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseOutputs_HappyPath(t *testing.T) {
	text := strings.Join([]string{
		"I authored two tests against the API suite.",
		"",
		"```yaml",
		"outputs:",
		"  test_names:",
		"    - shouldRegisterCustomer",
		"    - shouldRejectDuplicateCustomer",
		"  suite: <acceptance-api>",
		"```",
	}, "\n")

	got, err := ParseOutputs(text)
	if err != nil {
		t.Fatalf("ParseOutputs: %v", err)
	}
	wantNames := []string{"shouldRegisterCustomer", "shouldRejectDuplicateCustomer"}
	gotNames, ok := got["test_names"].([]string)
	if !ok {
		t.Fatalf("test_names: want []string, got %T", got["test_names"])
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Errorf("test_names: got %v, want %v", gotNames, wantNames)
	}
	if got["suite"] != "<acceptance-api>" {
		t.Errorf("suite: got %v, want <acceptance-api>", got["suite"])
	}
}

func TestParseOutputs_MissingBlock(t *testing.T) {
	// Agents that have nothing to emit are allowed to skip the block —
	// returns empty map with nil error rather than failing.
	got, err := ParseOutputs("Just some prose with no YAML at all.\n")
	if err != nil {
		t.Fatalf("ParseOutputs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestParseOutputs_EmptyText(t *testing.T) {
	got, err := ParseOutputs("")
	if err != nil {
		t.Fatalf("ParseOutputs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestParseOutputs_MalformedYAML(t *testing.T) {
	text := "```yaml\n" +
		"outputs:\n" +
		"  test_names: [unclosed\n" +
		"  suite: <acceptance-api>\n" +
		"```\n"
	_, err := ParseOutputs(text)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
	if !strings.Contains(err.Error(), "yaml unmarshal") {
		t.Errorf("error wording: got %q", err.Error())
	}
}

func TestParseOutputs_ScopeException(t *testing.T) {
	text := strings.Join([]string{
		"```yaml",
		"scope_exception:",
		"  files:",
		"    - internal/foo/out_of_scope.go",
		"    - internal/bar/also.go",
		"  reason: requires editing the shared logger",
		"```",
	}, "\n")

	got, err := ParseOutputs(text)
	if err != nil {
		t.Fatalf("ParseOutputs: %v", err)
	}
	wantFiles := []string{"internal/foo/out_of_scope.go", "internal/bar/also.go"}
	gotFiles, ok := got["scope_exception_files"].([]string)
	if !ok {
		t.Fatalf("scope_exception_files: want []string, got %T", got["scope_exception_files"])
	}
	if !reflect.DeepEqual(gotFiles, wantFiles) {
		t.Errorf("scope_exception_files: got %v, want %v", gotFiles, wantFiles)
	}
	if got["scope_exception_reason"] != "requires editing the shared logger" {
		t.Errorf("scope_exception_reason: got %v", got["scope_exception_reason"])
	}
}

func TestParseOutputs_BothBlocks(t *testing.T) {
	// outputs: + scope_exception: in the same response — the dispatcher
	// merges both into ctx.State.
	text := strings.Join([]string{
		"```yaml",
		"outputs:",
		"  test_names:",
		"    - shouldFoo",
		"  suite: <acceptance-api>",
		"```",
		"",
		"And separately I couldn't avoid touching out-of-scope:",
		"",
		"```yaml",
		"scope_exception:",
		"  files:",
		"    - internal/shared/clock.go",
		"  reason: depends on a clock helper",
		"```",
	}, "\n")

	got, err := ParseOutputs(text)
	if err != nil {
		t.Fatalf("ParseOutputs: %v", err)
	}
	if got["suite"] != "<acceptance-api>" {
		t.Errorf("suite: got %v", got["suite"])
	}
	if gotNames, ok := got["test_names"].([]string); !ok || len(gotNames) != 1 || gotNames[0] != "shouldFoo" {
		t.Errorf("test_names: got %v (%T)", got["test_names"], got["test_names"])
	}
	if gotFiles, ok := got["scope_exception_files"].([]string); !ok || len(gotFiles) != 1 || gotFiles[0] != "internal/shared/clock.go" {
		t.Errorf("scope_exception_files: got %v (%T)", got["scope_exception_files"], got["scope_exception_files"])
	}
	if got["scope_exception_reason"] != "depends on a clock helper" {
		t.Errorf("scope_exception_reason: got %v", got["scope_exception_reason"])
	}
}

func TestParseOutputs_LastBlockWins(t *testing.T) {
	// Agent quotes an example block earlier in the response (e.g. echoing
	// the prompt's format spec) and emits the real block at the end —
	// loose-match rule keeps the LAST one.
	text := strings.Join([]string{
		"Earlier I'm reminded I should emit something like:",
		"",
		"```yaml",
		"outputs:",
		"  test_names: [exampleOne]",
		"  suite: <stub>",
		"```",
		"",
		"Now my real output:",
		"",
		"```yaml",
		"outputs:",
		"  test_names: [shouldActuallyRun]",
		"  suite: <acceptance-api>",
		"```",
	}, "\n")

	got, err := ParseOutputs(text)
	if err != nil {
		t.Fatalf("ParseOutputs: %v", err)
	}
	if got["suite"] != "<acceptance-api>" {
		t.Errorf("suite: got %v, want <acceptance-api>", got["suite"])
	}
	gotNames := got["test_names"].([]string)
	if len(gotNames) != 1 || gotNames[0] != "shouldActuallyRun" {
		t.Errorf("test_names: got %v, want [shouldActuallyRun]", gotNames)
	}
}

func TestParseOutputs_UnknownKeyPassesThrough(t *testing.T) {
	// Forward-compat: an agent emits a key the parser hasn't been taught
	// about. It must NOT fail — the value passes through as whatever
	// yaml.v3 decoded it to (bool here), so a future gate that reads
	// ctx.State["dsl_interface_changed"] gets the right typed value
	// without a parser-side amendment.
	text := strings.Join([]string{
		"```yaml",
		"outputs:",
		"  dsl_interface_changed: true",
		"  test_names: [x]",
		"  suite: <acceptance-api>",
		"```",
	}, "\n")

	got, err := ParseOutputs(text)
	if err != nil {
		t.Fatalf("ParseOutputs: %v", err)
	}
	if got["dsl_interface_changed"] != true {
		t.Errorf("dsl_interface_changed: got %v (%T), want bool true",
			got["dsl_interface_changed"], got["dsl_interface_changed"])
	}
}

func TestParseOutputs_TestNamesWrongType(t *testing.T) {
	// `test_names` is locked to []string. A scalar string is a common
	// agent mistake; the parser must catch it loudly rather than
	// silently coercing.
	text := strings.Join([]string{
		"```yaml",
		"outputs:",
		"  test_names: just_one_name",
		"  suite: <acceptance-api>",
		"```",
	}, "\n")
	_, err := ParseOutputs(text)
	if err == nil {
		t.Fatal("expected error for wrong test_names type, got nil")
	}
	if !strings.Contains(err.Error(), "test_names") {
		t.Errorf("error wording: got %q", err.Error())
	}
}

func TestParseOutputs_SuiteWrongType(t *testing.T) {
	text := strings.Join([]string{
		"```yaml",
		"outputs:",
		"  test_names: [x]",
		"  suite: [<acceptance-api>]",
		"```",
	}, "\n")
	_, err := ParseOutputs(text)
	if err == nil {
		t.Fatal("expected error for wrong suite type, got nil")
	}
	if !strings.Contains(err.Error(), "suite") {
		t.Errorf("error wording: got %q", err.Error())
	}
}

func TestParseOutputs_ScopeExceptionMissingFiles(t *testing.T) {
	text := strings.Join([]string{
		"```yaml",
		"scope_exception:",
		"  reason: forgot to list files",
		"```",
	}, "\n")
	_, err := ParseOutputs(text)
	if err == nil {
		t.Fatal("expected error for missing files, got nil")
	}
	if !strings.Contains(err.Error(), "files") {
		t.Errorf("error wording: got %q", err.Error())
	}
}

func TestParseOutputs_UnfencedYAMLIgnored(t *testing.T) {
	// Bare YAML-ish text without a fenced block is not picked up —
	// agents must emit a fenced block per the prompt contract.
	text := "outputs:\n  test_names: [x]\n  suite: <acceptance-api>\n"
	got, err := ParseOutputs(text)
	if err != nil {
		t.Fatalf("ParseOutputs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map for unfenced text, got %v", got)
	}
}

func TestParseOutputs_NonYAMLFencedBlockIgnored(t *testing.T) {
	// A fenced shell block or code sample that happens to live in the
	// response must not be misparsed as outputs. The top-key heuristic
	// rejects it (no `outputs:` / `scope_exception:` start line).
	text := strings.Join([]string{
		"```bash",
		"go test ./...",
		"```",
		"",
		"```",
		"some random",
		"text with no key",
		"```",
	}, "\n")
	got, err := ParseOutputs(text)
	if err != nil {
		t.Fatalf("ParseOutputs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestParseOutputs_NoReasonIsFine(t *testing.T) {
	// scope.md formats reason as `reason: <…>` but a missing reason key
	// must not be treated as malformed — only `files` is structurally
	// required (it's what the gate reads). reason is informational.
	text := strings.Join([]string{
		"```yaml",
		"scope_exception:",
		"  files:",
		"    - x.go",
		"```",
	}, "\n")
	got, err := ParseOutputs(text)
	if err != nil {
		t.Fatalf("ParseOutputs: %v", err)
	}
	if _, ok := got["scope_exception_files"]; !ok {
		t.Errorf("scope_exception_files missing")
	}
	if _, present := got["scope_exception_reason"]; present {
		t.Errorf("scope_exception_reason should be absent when reason key not emitted")
	}
}
