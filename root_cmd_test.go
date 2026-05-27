package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/approval"
	"github.com/optivem/gh-optivem/internal/workspace"
)

// TestRootCmd_CrossRepoVerbsRegistered confirms the new root surface from
// plan 20260515-0736: the cross-repo verbs that were previously under the
// `workspace` noun now sit at the root, and `check-actions` is reshaped as
// the noun-verb `actions status`.
func TestRootCmd_CrossRepoVerbsRegistered(t *testing.T) {
	root := newRootCmd()

	wantVerbs := []string{
		"commit",
		"sync",
		"actions",
		"rate-limit",
		// Hidden but still registered — see plan "Deferred follow-ups".
		"lint-history",
		"stale-branches",
	}
	for _, name := range wantVerbs {
		found, _, err := root.Find([]string{name})
		if err != nil || found == nil || found.Name() != name {
			t.Errorf("root cmd has no subcommand %q (found=%v, err=%v)", name, found, err)
		}
	}
}

func TestRootCmd_ActionsStatusSubcommandRegistered(t *testing.T) {
	root := newRootCmd()

	found, _, err := root.Find([]string{"actions", "status"})
	if err != nil {
		t.Fatalf("actions status Find: %v", err)
	}
	if found.Name() != "status" {
		t.Errorf("expected to resolve `actions status` → status subcommand, got %q", found.Name())
	}
	if parent := found.Parent(); parent == nil || parent.Name() != "actions" {
		t.Errorf("status parent = %v, want actions", parent)
	}
}

// TestRootCmd_WorkspaceNounRemoved is the inverse of CrossRepoVerbsRegistered.
// The plan calls for a hard remove with no alias — `gh optivem workspace
// commit` must produce cobra's standard unknown-command error, not silently
// fall through to anything else.
func TestRootCmd_WorkspaceNounRemoved(t *testing.T) {
	root := newRootCmd()
	for _, c := range root.Commands() {
		if c.Name() == "workspace" {
			t.Fatalf("workspace noun must be removed entirely (no aliases). Found: %s", c.Use)
		}
	}
}

// TestRootCmd_HiddenTBDVerbs pins lint-history and stale-branches to the
// hidden state until their final placement is decided (plan "Deferred
// follow-ups"). They must remain findable / invokable — only --help output
// is suppressed.
func TestRootCmd_HiddenTBDVerbs(t *testing.T) {
	root := newRootCmd()
	for _, name := range []string{"lint-history", "stale-branches"} {
		found, _, err := root.Find([]string{name})
		if err != nil || found == nil || found.Name() != name {
			t.Fatalf("%s not registered: %v", name, err)
		}
		if !found.Hidden {
			t.Errorf("%s should be Hidden=true while placement is deferred", name)
		}
	}
}

// TestRootCmd_WorkspaceFlagIsPersistent confirms --workspace moved from the
// (now-deleted) workspace noun to a root-level persistent flag, so every
// cross-repo verb consumes it identically regardless of where it sits.
func TestRootCmd_WorkspaceFlagIsPersistent(t *testing.T) {
	root := newRootCmd()
	flag := root.PersistentFlags().Lookup("workspace")
	if flag == nil {
		t.Fatal("--workspace must be a root-level persistent flag")
	}
	// Also confirm the flag description references the cascade so operators
	// see the discoverability hints.
	if !strings.Contains(flag.Usage, workspace.EnvVar) {
		t.Errorf("--workspace flag usage should mention $%s for discoverability; got: %q",
			workspace.EnvVar, flag.Usage)
	}
}

// TestRootCmd_GroupsRegistered pins the three help-text groups the plan
// calls for. Without grouping, the new root verbs would interleave
// alphabetically with the project verbs, defeating the visual distinction
// the rename creates.
func TestRootCmd_GroupsRegistered(t *testing.T) {
	root := newRootCmd()
	wantIDs := map[string]bool{
		"project":    false,
		"cross-repo": false,
		"other":      false,
	}
	for _, g := range root.Groups() {
		if _, ok := wantIDs[g.ID]; ok {
			wantIDs[g.ID] = true
		}
	}
	for id, present := range wantIDs {
		if !present {
			t.Errorf("expected group %q to be registered on root", id)
		}
	}
}

// TestRootCmd_CommandGroupAssignments confirms each command joined the
// group the plan assigns to it. The plan's exact grouping (decisions log
// 2026-05-15) is "Project ops" (compile, test, system, doctor, config,
// init), "Cross-repo ops" (commit, sync, actions, rate-limit + hidden
// lint-history / stale-branches), and "Other" for the rest.
func TestRootCmd_CommandGroupAssignments(t *testing.T) {
	root := newRootCmd()
	wantGroup := map[string]string{
		// Project ops
		"compile": "project",
		"test":    "project",
		"system":  "project",
		"doctor":  "project",
		"config":  "project",
		"init":    "project",
		// Cross-repo ops
		"commit":         "cross-repo",
		"sync":           "cross-repo",
		"actions":        "cross-repo",
		"rate-limit":     "cross-repo",
		"lint-history":   "cross-repo",
		"stale-branches": "cross-repo",
	}
	for _, c := range root.Commands() {
		want, tracked := wantGroup[c.Name()]
		if !tracked {
			continue // "other"-group commands aren't pinned by this test
		}
		if c.GroupID != want {
			t.Errorf("%s.GroupID = %q, want %q", c.Name(), c.GroupID, want)
		}
	}
}

// withCleanApprovalFlags resets the package-global autoFlag / confirmFlag
// before and after the test so Cobra's BoolVar/StringVar parse-side-effects
// don't leak between cases. newRootCmd rebinds the globals every call, but
// rebinding doesn't clear the pre-existing value.
func withCleanApprovalFlags(t *testing.T) {
	t.Helper()
	prevAuto, prevConfirm := autoFlag, confirmFlag
	autoFlag, confirmFlag = false, ""
	t.Cleanup(func() { autoFlag, confirmFlag = prevAuto, prevConfirm })
}

// rootCmdWithNoopLeaf wires a no-side-effect leaf onto root so Execute can
// drive PersistentPreRunE end-to-end without firing any production verb.
func rootCmdWithNoopLeaf(t *testing.T) *cobra.Command {
	t.Helper()
	root := newRootCmd()
	root.AddCommand(&cobra.Command{
		Use:    "noop-test",
		Hidden: true,
		Run:    func(cmd *cobra.Command, args []string) {},
	})
	return root
}

// TestRootCmd_AutoFlagIsPersistent pins --auto as a root-level persistent
// flag and confirms its Usage references the env var so operators reading
// `--help` see the env-vs-flag discoverability hint.
func TestRootCmd_AutoFlagIsPersistent(t *testing.T) {
	root := newRootCmd()
	flag := root.PersistentFlags().Lookup("auto")
	if flag == nil {
		t.Fatal("--auto must be a root-level persistent flag")
	}
	if !strings.Contains(flag.Usage, approval.EnvAuto) {
		t.Errorf("--auto Usage should mention $%s; got: %q", approval.EnvAuto, flag.Usage)
	}
}

// TestRootCmd_ConfirmFlagIsPersistent pins --confirm at root and confirms
// the Usage advertises the closed category set + env var. Without that
// `--help` text the exclusion vocabulary is invisible to operators.
func TestRootCmd_ConfirmFlagIsPersistent(t *testing.T) {
	root := newRootCmd()
	flag := root.PersistentFlags().Lookup("confirm")
	if flag == nil {
		t.Fatal("--confirm must be a root-level persistent flag")
	}
	if !strings.Contains(flag.Usage, approval.EnvConfirm) {
		t.Errorf("--confirm Usage should mention $%s; got: %q", approval.EnvConfirm, flag.Usage)
	}
	for _, cat := range []string{"commit", "fix", "release", "prompt"} {
		if !strings.Contains(flag.Usage, cat) {
			t.Errorf("--confirm Usage should list category %q; got: %q", cat, flag.Usage)
		}
	}
}

// TestRootCmd_InvalidConfirmCategory_ParseError exercises the
// PersistentPreRunE error path: an unknown token in --confirm must fail at
// command start (before any verb runs) with a message naming the bad token
// and the valid set.
func TestRootCmd_InvalidConfirmCategory_ParseError(t *testing.T) {
	withCleanApprovalFlags(t)
	root := rootCmdWithNoopLeaf(t)
	var stderr bytes.Buffer
	root.SetErr(&stderr)
	root.SetOut(io.Discard)
	root.SetArgs([]string{"--auto", "--confirm=garbage", "noop-test"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected Execute to fail on --confirm=garbage")
	}
	if !strings.Contains(err.Error(), "garbage") {
		t.Errorf("error should name the offending token; got: %v", err)
	}
	for _, want := range []string{"commit", "fix", "release", "prompt", "human"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should list valid category %q; got: %v", want, err)
		}
	}
}

// TestRootCmd_BannerEmittedUnderAutoFlag confirms the startup banner lands
// on stderr when --auto is on, naming both sources and the resolved
// exclusion list (default commit,fix because --confirm wasn't given).
func TestRootCmd_BannerEmittedUnderAutoFlag(t *testing.T) {
	withCleanApprovalFlags(t)
	root := rootCmdWithNoopLeaf(t)
	var stderr bytes.Buffer
	root.SetErr(&stderr)
	root.SetOut(io.Discard)
	root.SetArgs([]string{"--auto", "noop-test"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := stderr.String()
	for _, want := range []string{
		"Auto: true",
		"auto-source: flag",
		"confirm-source: default",
		"commit,fix",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("banner missing %q; got: %q", want, out)
		}
	}
}

// TestRootCmd_BannerSuppressedWithoutAuto confirms cautious mode (no --auto)
// is silent — no banner at all, matching today's no-banner default.
func TestRootCmd_BannerSuppressedWithoutAuto(t *testing.T) {
	withCleanApprovalFlags(t)
	root := rootCmdWithNoopLeaf(t)
	var stderr bytes.Buffer
	root.SetErr(&stderr)
	root.SetOut(io.Discard)
	root.SetArgs([]string{"noop-test"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(stderr.String(), "Auto:") {
		t.Errorf("expected no banner when --auto is off; got: %q", stderr.String())
	}
}

// TestRootCmd_BannerEmptyConfirmList renders as "(none)" so the operator
// sees the truly-autonomous shape on stderr rather than a dangling arrow.
func TestRootCmd_BannerEmptyConfirmList(t *testing.T) {
	withCleanApprovalFlags(t)
	root := rootCmdWithNoopLeaf(t)
	var stderr bytes.Buffer
	root.SetErr(&stderr)
	root.SetOut(io.Discard)
	root.SetArgs([]string{"--auto", "--confirm=", "noop-test"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(stderr.String(), "confirm-source: flag → (none)") {
		t.Errorf("expected `→ (none)` rendering for explicit empty --confirm; got: %q", stderr.String())
	}
}

// TestScopeBannerLine_WorkspaceMode pins the workspace-mode wording from
// the plan decisions log: `Mode: workspace (N repos from <basename>)`.
func TestScopeBannerLine_WorkspaceMode(t *testing.T) {
	scope := workspace.Scope{
		Mode:       workspace.ModeWorkspace,
		Folders:    []string{"/a/repo1", "/a/repo2", "/a/repo3"},
		SourceFile: "/a/page-turner.code-workspace",
	}
	got := scopeBannerLine(scope)
	want := "Mode: workspace (3 repos from page-turner.code-workspace)"
	if got != want {
		t.Errorf("banner = %q, want %q", got, want)
	}
}

// TestScopeBannerLine_SingleRepoMode pins the single-repo wording from the
// plan decisions log: `Mode: single repo (<basename>)`, with no trailing
// "no workspace file found" parenthetical.
func TestScopeBannerLine_SingleRepoMode(t *testing.T) {
	scope := workspace.Scope{
		Mode:    workspace.ModeSingleRepo,
		Root:    "/path/to/shop",
		Folders: []string{"/path/to/shop"},
	}
	got := scopeBannerLine(scope)
	want := "Mode: single repo (shop)"
	if got != want {
		t.Errorf("banner = %q, want %q", got, want)
	}
	if strings.Contains(got, "no workspace") {
		t.Errorf("single-repo banner should NOT mention 'no workspace': %q", got)
	}
}
