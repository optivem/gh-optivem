package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

func TestParseIssueArg(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		in      string
		want    int
		wantErr bool
	}{
		{"bare number", "42", 42, false},
		{"bare number with whitespace", "  42  ", 42, false},
		{"with hash prefix", "#42", 42, false},
		{"github url", "https://github.com/myorg/myrepo/issues/61", 61, false},
		{"github url trailing slash", "https://github.com/myorg/myrepo/issues/61/", 61, false},
		{"short repo path", "myorg/myrepo/issues/7", 7, false},
		{"empty", "", 0, true},
		{"whitespace only", "   ", 0, true},
		{"non-numeric tail", "https://github.com/myorg/myrepo/pulls/foo", 0, true},
		{"zero", "0", 0, true},
		{"negative", "-1", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseIssueArg(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseIssueArg(%q): want error, got %d", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseIssueArg(%q): unexpected error %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("parseIssueArg(%q): want %d, got %d", tc.in, tc.want, got)
			}
		})
	}
}

// TestOverrideHooksFromConfig_NilAndEmpty: nil cfg and a cfg with no
// override maps both yield (nil, nil), so wrapOverride sees a no-op hook
// rather than an empty-but-allocated struct.
func TestOverrideHooksFromConfig_NilAndEmpty(t *testing.T) {
	t.Parallel()
	hooks, err := overrideHooksFromConfig(nil)
	if err != nil {
		t.Fatalf("nil cfg: unexpected error %v", err)
	}
	if hooks != nil {
		t.Errorf("nil cfg: expected nil hooks, got %+v", hooks)
	}

	cfg := &projectconfig.Config{Project: projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/x/projects/1"}}
	hooks, err = overrideHooksFromConfig(cfg)
	if err != nil {
		t.Fatalf("empty maps: unexpected error %v", err)
	}
	if hooks != nil {
		t.Errorf("empty maps: expected nil hooks, got %+v", hooks)
	}
}

// TestOverrideHooksFromConfig_NodeExtrasPassedThrough: literal text from
// cfg.NodeExtras lands verbatim in Hooks.Extra.
func TestOverrideHooksFromConfig_NodeExtrasPassedThrough(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{
		Project: projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/x/projects/1"},
		NodeExtras: map[string]string{
			"AT_RED_DSL_WRITE": "prefer record types",
		},
	}
	hooks, err := overrideHooksFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hooks == nil {
		t.Fatal("expected non-nil hooks")
	}
	if got := hooks.Extra["AT_RED_DSL_WRITE"]; got != "prefer record types" {
		t.Errorf("Extra[AT_RED_DSL_WRITE]: got %q, want %q", got, "prefer record types")
	}
	if len(hooks.Replace) != 0 {
		t.Errorf("Replace should be empty when no node-replacements set, got %+v", hooks.Replace)
	}
}

// TestOverrideHooksFromConfig_NodeReplacementsReadFiles: paths in
// cfg.NodeReplacements are read at startup and Hooks.Replace carries the
// file *body*, not the path.
func TestOverrideHooksFromConfig_NodeReplacementsReadFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := "Custom prompt body for AT_RED_TEST_WRITE.\nSecond line."
	path := filepath.Join(dir, "at-red.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cfg := &projectconfig.Config{
		Project: projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/x/projects/1"},
		NodeReplacements: map[string]string{
			"AT_RED_TEST_WRITE": path,
		},
	}
	hooks, err := overrideHooksFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hooks == nil {
		t.Fatal("expected non-nil hooks")
	}
	if got := hooks.Replace["AT_RED_TEST_WRITE"]; got != body {
		t.Errorf("Replace[AT_RED_TEST_WRITE]: got %q, want %q (file body, not path)", got, body)
	}
}

// TestOverrideHooksFromConfig_MissingReplacementPathErrors: a missing file
// surfaces at startup with the node ID and path in the message, so the
// operator doesn't see "why didn't my override apply?" deep in the pipeline.
func TestOverrideHooksFromConfig_MissingReplacementPathErrors(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{
		Project: projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/x/projects/1"},
		NodeReplacements: map[string]string{
			"AT_RED_TEST_WRITE": filepath.Join(t.TempDir(), "no-such.md"),
		},
	}
	_, err := overrideHooksFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing node-replacements path, got nil")
	}
	if !strings.Contains(err.Error(), "AT_RED_TEST_WRITE") {
		t.Errorf("error should name the node ID, got: %v", err)
	}
}

// TestTaskPromptOverridesFromConfig_ReadsFiles: cfg.TaskPrompts paths are
// read at startup and the returned map holds bodies, not paths.
func TestTaskPromptOverridesFromConfig_ReadsFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := "You are the customized write-acceptance-tests task.\n"
	path := filepath.Join(dir, "write-acceptance-tests.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cfg := &projectconfig.Config{
		Project: projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/x/projects/1"},
		TaskPrompts: map[string]string{
			"write-acceptance-tests": path,
		},
	}
	out, err := taskPromptOverridesFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := out["write-acceptance-tests"]; got != body {
		t.Errorf("write-acceptance-tests: got %q, want %q (file body, not path)", got, body)
	}
}

// TestTaskPromptOverridesFromConfig_NilAndEmpty: nil cfg and empty
// TaskPrompts both yield (nil, nil), so the driver sees a clean nil map.
func TestTaskPromptOverridesFromConfig_NilAndEmpty(t *testing.T) {
	t.Parallel()
	out, err := taskPromptOverridesFromConfig(nil)
	if err != nil {
		t.Fatalf("nil cfg: unexpected error %v", err)
	}
	if out != nil {
		t.Errorf("nil cfg: expected nil map, got %+v", out)
	}

	cfg := &projectconfig.Config{Project: projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/x/projects/1"}}
	out, err = taskPromptOverridesFromConfig(cfg)
	if err != nil {
		t.Fatalf("empty cfg: unexpected error %v", err)
	}
	if out != nil {
		t.Errorf("empty cfg: expected nil map, got %+v", out)
	}
}

// TestTaskPromptOverridesFromConfig_MissingPathErrors: a missing file
// surfaces at startup naming the task so the operator sees what's wrong
// before any pipeline node runs.
func TestTaskPromptOverridesFromConfig_MissingPathErrors(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{
		Project: projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/x/projects/1"},
		TaskPrompts: map[string]string{
			"write-acceptance-tests": filepath.Join(t.TempDir(), "no-such.md"),
		},
	}
	_, err := taskPromptOverridesFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing task-prompts path, got nil")
	}
	if !strings.Contains(err.Error(), "write-acceptance-tests") {
		t.Errorf("error should name the task, got: %v", err)
	}
}

// TestNewImplementCmd_HasExpectedFlagsAndUse: thin wiring check — the
// `implement` command must declare every flag callers rely on (issue, target,
// channel, headless, autonomous [deprecated alias], manual-agents, workspace,
// log-file, keep-runs, show-prompt). Its Use line leads with the bare verb
// (now `implement [issue]` since the issue is also positional per D-positional)
// so the noun-first surface keeps `gh optivem implement` as a top-level
// command, and it accepts at most one positional arg (the issue).
func TestNewImplementCmd_HasExpectedFlagsAndUse(t *testing.T) {
	t.Parallel()
	cmd := newImplementCmd()
	if cmd.Use != "implement [issue]" {
		t.Errorf("Use: got %q, want %q", cmd.Use, "implement [issue]")
	}
	wantFlags := []string{"issue", "target", "channel", "headless", "autonomous", "manual-agents", "workspace", "log-file", "keep-runs", "show-prompt"}
	for _, name := range wantFlags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s", name)
		}
	}
	// --issue must NOT have a short form (-i is reserved for future flags).
	if f := cmd.Flags().Lookup("issue"); f != nil && f.Shorthand != "" {
		t.Errorf("--issue should have no short form, got -%s", f.Shorthand)
	}
	// --autonomous is documented as deprecated; the Usage string must say so
	// so `--help` callers see the alias signal without reading the plan.
	if f := cmd.Flags().Lookup("autonomous"); f != nil && !strings.Contains(strings.ToLower(f.Usage), "deprecated") {
		t.Errorf("--autonomous Usage should mark it deprecated, got %q", f.Usage)
	}
	// The positional issue is additive (D-positional): at most one arg, and
	// the Args validator must accept zero (the --issue-flag form) and one.
	if cmd.Args == nil {
		t.Fatal("expected an Args validator (MaximumNArgs(1)), got nil")
	}
	if err := cmd.Args(cmd, []string{"42"}); err != nil {
		t.Errorf("Args should accept one positional issue, got %v", err)
	}
	if err := cmd.Args(cmd, []string{"1", "2"}); err == nil {
		t.Error("Args should reject two positional arguments")
	}
}

// TestResolveIssueSource exercises the D-positional reconciliation: the issue
// may come from a positional arg or --issue, exactly one of them. Both-set is a
// conflict; neither is the missing-issue error.
func TestResolveIssueSource(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		flag    string
		args    []string
		want    string
		wantErr bool
	}{
		{"positional only", "", []string{"42"}, "42", false},
		{"flag only", "42", nil, "42", false},
		{"flag with url", "https://github.com/myorg/myrepo/issues/7", nil, "https://github.com/myorg/myrepo/issues/7", false},
		{"positional trims whitespace", "", []string{"  42  "}, "42", false},
		{"flag trims whitespace", "  42  ", nil, "42", false},
		{"both set conflicts", "42", []string{"43"}, "", true},
		{"both set conflicts even if equal", "42", []string{"42"}, "", true},
		{"neither set errors", "", nil, "", true},
		{"empty positional treated as absent", "42", []string{"   "}, "42", false},
		{"blank flag + blank positional errors", "  ", []string{"  "}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveIssueSource(tc.flag, tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveIssueSource(%q, %v): want error, got %q", tc.flag, tc.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveIssueSource(%q, %v): unexpected error %v", tc.flag, tc.args, err)
			}
			if got != tc.want {
				t.Fatalf("resolveIssueSource(%q, %v): got %q, want %q", tc.flag, tc.args, got, tc.want)
			}
		})
	}
}

// TestNewProcessCmd_HasShowChild: `process` is a noun parent; its only
// child today is `show`. Verifies the wiring stays at three levels
// (`optivem process show`) and that `show` is the leaf with no diagram
// child of its own.
func TestNewProcessCmd_HasShowChild(t *testing.T) {
	t.Parallel()
	cmd := newProcessCmd()
	if cmd.Use != "process" {
		t.Errorf("parent Use: got %q, want %q", cmd.Use, "process")
	}
	var show *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Use == "show" {
			show = c
		}
	}
	if show == nil {
		t.Fatalf("process: missing `show` child; have %v", commandUses(cmd.Commands()))
	}
	if got := len(show.Commands()); got != 0 {
		t.Errorf("process show: expected leaf (0 children), got %d", got)
	}
}

// TestPrintRunEndBanner_writesBannerWithConfig: with a valid systems.yaml
// pointed to by cfg.System.Config, the per-system header (label fallback,
// since the fixture sets no description) plus the per-component status line
// both land on the writer. Empty ticketURL exercises the no-ticket branch.
func TestPrintRunEndBanner_writesBannerWithConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	systemsPath := filepath.Join(dir, "systems.yaml")
	body := `systems:
  - label: real
    composeFile: ./docker-compose.yaml
    components:
      - name: api
        url: http://127.0.0.1:1/
`
	if err := os.WriteFile(systemsPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write systems.yaml fixture: %v", err)
	}
	cfg := &projectconfig.Config{
		Project: projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/x/projects/1"},
		System:  projectconfig.System{Config: systemsPath},
	}
	var buf bytes.Buffer
	printRunEndBanner(&buf, cfg, "", "")
	got := buf.String()
	if !strings.Contains(got, "=== System connected to real ===") {
		t.Errorf("missing per-system header (label fallback) in output:\n%s", got)
	}
	// The probe targets 127.0.0.1:1 which is unreachable; expect DOWN.
	if !strings.Contains(got, "DOWN api:") {
		t.Errorf("expected per-component DOWN line, got:\n%s", got)
	}
	if strings.Contains(got, "Ticket:") {
		t.Errorf("empty ticketURL should produce no Ticket line, got:\n%s", got)
	}
}

// TestPrintRunEndBanner_silentOnNilConfig: nil cfg + empty ticket writes
// nothing. Banner is best-effort and must never panic on missing inputs.
func TestPrintRunEndBanner_silentOnNilConfig(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	printRunEndBanner(&buf, nil, "", "")
	if buf.Len() != 0 {
		t.Errorf("expected empty output on nil cfg + empty ticket, got: %q", buf.String())
	}
}

// TestPrintRunEndBanner_silentOnEmptyConfigPath: cfg with no system.config:
// set + empty ticket writes nothing.
func TestPrintRunEndBanner_silentOnEmptyConfigPath(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{
		Project: projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/x/projects/1"},
	}
	var buf bytes.Buffer
	printRunEndBanner(&buf, cfg, "", "")
	if buf.Len() != 0 {
		t.Errorf("expected empty output when system.config is empty + empty ticket, got: %q", buf.String())
	}
}

// TestPrintRunEndBanner_silentOnUnreadableFile: cfg pointing at a
// non-existent systems.yaml + empty ticket writes nothing rather than
// failing the implement run with a load error.
func TestPrintRunEndBanner_silentOnUnreadableFile(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{
		Project: projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/x/projects/1"},
		System:  projectconfig.System{Config: filepath.Join(t.TempDir(), "no-such.yaml")},
	}
	var buf bytes.Buffer
	printRunEndBanner(&buf, cfg, "", "")
	if buf.Len() != 0 {
		t.Errorf("expected empty output on unreadable systems.yaml + empty ticket, got: %q", buf.String())
	}
}

// TestPrintRunEndBanner_printsTicketLine: non-empty ticketURL prints the
// Ticket line even when cfg is nil (no system block to load). With an empty
// title, the line is the URL alone (no trailing quoted suffix).
func TestPrintRunEndBanner_printsTicketLine(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	printRunEndBanner(&buf, nil, "https://github.com/myorg/myrepo/issues/42", "")
	got := buf.String()
	if !strings.Contains(got, "Ticket: https://github.com/myorg/myrepo/issues/42") {
		t.Errorf("expected Ticket line, got: %q", got)
	}
	if strings.Contains(got, "\"") {
		t.Errorf("empty title should not emit a quoted suffix, got: %q", got)
	}
	if strings.Contains(got, "===") {
		t.Errorf("nil cfg should not emit any system block, got: %q", got)
	}
}

// TestPrintRunEndBanner_printsTicketLineWithTitle: when both URL and title
// are set, the title is appended in quotes after the URL on the same line.
func TestPrintRunEndBanner_printsTicketLineWithTitle(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	printRunEndBanner(&buf, nil, "https://github.com/myorg/myrepo/issues/42", "Reject order with line quantity of 100")
	got := buf.String()
	want := "Ticket: https://github.com/myorg/myrepo/issues/42 \"Reject order with line quantity of 100\""
	if !strings.Contains(got, want) {
		t.Errorf("expected %q in output, got: %q", want, got)
	}
}

// TestPrintRunEndBanner_descriptionInHeader: when systems.yaml entries
// declare a description, the per-system header uses it instead of the label.
func TestPrintRunEndBanner_descriptionInHeader(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	systemsPath := filepath.Join(dir, "systems.yaml")
	body := `systems:
  - label: stub
    description: External System Stubs
    composeFile: ./docker-compose.yaml
    components:
      - name: api
        url: http://127.0.0.1:1/
`
	if err := os.WriteFile(systemsPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write systems.yaml fixture: %v", err)
	}
	cfg := &projectconfig.Config{
		Project: projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/x/projects/1"},
		System:  projectconfig.System{Config: systemsPath},
	}
	var buf bytes.Buffer
	printRunEndBanner(&buf, cfg, "", "")
	got := buf.String()
	if !strings.Contains(got, "=== System connected to External System Stubs ===") {
		t.Errorf("expected description-driven header, got:\n%s", got)
	}
	if strings.Contains(got, "=== System connected to stub ===") {
		t.Errorf("description should win over label, got:\n%s", got)
	}
}

// TestPrintRunEndBanner_ticketAndSystemsTogether: ticket line appears before
// the system blocks, and declaration order is preserved (stub before real).
func TestPrintRunEndBanner_ticketAndSystemsTogether(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	systemsPath := filepath.Join(dir, "systems.yaml")
	body := `systems:
  - label: stub
    description: External System Stubs
    composeFile: ./docker-compose.stub.yaml
    components:
      - name: api
        url: http://127.0.0.1:1/
  - label: real
    description: External System Test Instances
    composeFile: ./docker-compose.real.yaml
    components:
      - name: api
        url: http://127.0.0.1:1/
`
	if err := os.WriteFile(systemsPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write systems.yaml fixture: %v", err)
	}
	cfg := &projectconfig.Config{
		Project: projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/x/projects/1"},
		System:  projectconfig.System{Config: systemsPath},
	}
	var buf bytes.Buffer
	printRunEndBanner(&buf, cfg, "https://github.com/myorg/myrepo/issues/42", "")
	got := buf.String()
	ticketIdx := strings.Index(got, "Ticket: https://github.com/myorg/myrepo/issues/42")
	stubIdx := strings.Index(got, "=== System connected to External System Stubs ===")
	realIdx := strings.Index(got, "=== System connected to External System Test Instances ===")
	if ticketIdx < 0 || stubIdx < 0 || realIdx < 0 {
		t.Fatalf("missing expected lines in output:\n%s", got)
	}
	if !(ticketIdx < stubIdx && stubIdx < realIdx) {
		t.Errorf("expected order: Ticket < stub header < real header, got indices %d / %d / %d in:\n%s", ticketIdx, stubIdx, realIdx, got)
	}
}

func commandUses(cs []*cobra.Command) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Use)
	}
	return out
}
