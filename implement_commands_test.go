package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/projectconfig"
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

	cfg := &projectconfig.Config{Project: projectconfig.Project{URL: "https://github.com/orgs/x/projects/1"}}
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
		Project: projectconfig.Project{URL: "https://github.com/orgs/x/projects/1"},
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
		t.Errorf("Replace should be empty when no node_replacements set, got %+v", hooks.Replace)
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
		Project: projectconfig.Project{URL: "https://github.com/orgs/x/projects/1"},
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
		Project: projectconfig.Project{URL: "https://github.com/orgs/x/projects/1"},
		NodeReplacements: map[string]string{
			"AT_RED_TEST_WRITE": filepath.Join(t.TempDir(), "no-such.md"),
		},
	}
	_, err := overrideHooksFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing node_replacements path, got nil")
	}
	if !strings.Contains(err.Error(), "AT_RED_TEST_WRITE") {
		t.Errorf("error should name the node ID, got: %v", err)
	}
}

// TestAgentPromptOverridesFromConfig_ReadsFiles: cfg.AgentPrompts paths are
// read at startup and the returned map holds bodies, not paths.
func TestAgentPromptOverridesFromConfig_ReadsFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := "You are the customized atdd-test agent.\n"
	path := filepath.Join(dir, "atdd-test.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cfg := &projectconfig.Config{
		Project: projectconfig.Project{URL: "https://github.com/orgs/x/projects/1"},
		AgentPrompts: map[string]string{
			"atdd-test": path,
		},
	}
	out, err := agentPromptOverridesFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := out["atdd-test"]; got != body {
		t.Errorf("atdd-test: got %q, want %q (file body, not path)", got, body)
	}
}

// TestAgentPromptOverridesFromConfig_NilAndEmpty: nil cfg and empty
// AgentPrompts both yield (nil, nil), so the driver sees a clean nil map.
func TestAgentPromptOverridesFromConfig_NilAndEmpty(t *testing.T) {
	t.Parallel()
	out, err := agentPromptOverridesFromConfig(nil)
	if err != nil {
		t.Fatalf("nil cfg: unexpected error %v", err)
	}
	if out != nil {
		t.Errorf("nil cfg: expected nil map, got %+v", out)
	}

	cfg := &projectconfig.Config{Project: projectconfig.Project{URL: "https://github.com/orgs/x/projects/1"}}
	out, err = agentPromptOverridesFromConfig(cfg)
	if err != nil {
		t.Fatalf("empty cfg: unexpected error %v", err)
	}
	if out != nil {
		t.Errorf("empty cfg: expected nil map, got %+v", out)
	}
}

// TestAgentPromptOverridesFromConfig_MissingPathErrors: a missing file
// surfaces at startup naming the agent so the operator sees what's wrong
// before any pipeline node runs.
func TestAgentPromptOverridesFromConfig_MissingPathErrors(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{
		Project: projectconfig.Project{URL: "https://github.com/orgs/x/projects/1"},
		AgentPrompts: map[string]string{
			"atdd-test": filepath.Join(t.TempDir(), "no-such.md"),
		},
	}
	_, err := agentPromptOverridesFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing agent_prompts path, got nil")
	}
	if !strings.Contains(err.Error(), "atdd-test") {
		t.Errorf("error should name the agent, got: %v", err)
	}
}

// TestNewImplementCmd_HasExpectedFlagsAndUse: thin wiring check — the
// `implement` command must declare every flag callers rely on (issue,
// autonomous, manual-agents, workspace, log-file, keep-runs, show-prompt)
// and its Use line must be the bare verb so the noun-first surface keeps
// `gh optivem implement` as a top-level command.
func TestNewImplementCmd_HasExpectedFlagsAndUse(t *testing.T) {
	t.Parallel()
	cmd := newImplementCmd()
	if cmd.Use != "implement" {
		t.Errorf("Use: got %q, want %q", cmd.Use, "implement")
	}
	wantFlags := []string{"issue", "autonomous", "manual-agents", "workspace", "log-file", "keep-runs", "show-prompt"}
	for _, name := range wantFlags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s", name)
		}
	}
	// --issue must NOT have a short form (-i is reserved for future flags).
	if f := cmd.Flags().Lookup("issue"); f != nil && f.Shorthand != "" {
		t.Errorf("--issue should have no short form, got -%s", f.Shorthand)
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

func commandUses(cs []*cobra.Command) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Use)
	}
	return out
}
