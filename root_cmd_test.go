package main

import (
	"strings"
	"testing"

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
