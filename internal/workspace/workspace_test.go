package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeWorkspace writes a *.code-workspace file at dir/name with the given
// JSON body.
func writeWorkspace(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write workspace %s: %v", p, err)
	}
	return p
}

// makeGitRepo creates dir and a dir/.git subdirectory (mimicking a real
// repo well enough for parseWorkspaceFolders' filter and walkUpForGitRepo).
func makeGitRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir %s/.git: %v", dir, err)
	}
}

// workspaceJSON renders a *.code-workspace body with the given folder
// paths (in declaration order).
func workspaceJSON(paths ...string) string {
	var sb strings.Builder
	sb.WriteString(`{"folders":[`)
	for i, p := range paths {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"path":"`)
		sb.WriteString(p)
		sb.WriteString(`"}`)
	}
	sb.WriteString("]}")
	return sb.String()
}

// setupWorkspace builds a tempdir laid out like a real workspace tree:
//
//	<root>/
//	  myworkspace.code-workspace
//	  repoA/.git/
//	  repoB/.git/
//	  notAGit/
func setupWorkspace(t *testing.T) (root string, wsPath string) {
	t.Helper()
	root = t.TempDir()
	makeGitRepo(t, filepath.Join(root, "repoA"))
	makeGitRepo(t, filepath.Join(root, "repoB"))
	if err := os.MkdirAll(filepath.Join(root, "notAGit"), 0o755); err != nil {
		t.Fatalf("mkdir notAGit: %v", err)
	}
	wsPath = writeWorkspace(t, root, "myworkspace.code-workspace",
		workspaceJSON("repoA", "repoB", "notAGit", "missingDir"))
	return root, wsPath
}

func TestResolve_ExplicitFlag(t *testing.T) {
	root, _ := setupWorkspace(t)

	// CWD is somewhere with no workspace and no git — the flag should win.
	scope, err := resolveFrom(root, "", t.TempDir())
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeWorkspace {
		t.Errorf("mode = %v, want ModeWorkspace", scope.Mode)
	}
	wantRoot, _ := filepath.Abs(root)
	if scope.Root != wantRoot {
		t.Errorf("root = %q, want %q", scope.Root, wantRoot)
	}
	wantFolders := []string{
		filepath.Join(wantRoot, "repoA"),
		filepath.Join(wantRoot, "repoB"),
	}
	if !equalStrings(scope.Folders, wantFolders) {
		t.Errorf("folders = %v, want %v", scope.Folders, wantFolders)
	}
	if scope.SourceFile == "" {
		t.Errorf("SourceFile must be populated in workspace mode")
	}
}

func TestResolve_EnvVar(t *testing.T) {
	root, _ := setupWorkspace(t)

	// CWD points somewhere with no workspace and no git repo. The env var should win.
	cwd := t.TempDir()
	scope, err := resolveFrom("", root, cwd)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeWorkspace {
		t.Errorf("mode = %v, want ModeWorkspace", scope.Mode)
	}
	if len(scope.Folders) != 2 {
		t.Errorf("folders = %v, want 2 entries", scope.Folders)
	}
}

func TestResolve_FlagBeatsEnv(t *testing.T) {
	flagRoot, _ := setupWorkspace(t)
	envRoot, _ := setupWorkspace(t) // a different workspace; we never expect to pick from this one

	scope, err := resolveFrom(flagRoot, envRoot, t.TempDir())
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	wantRoot, _ := filepath.Abs(flagRoot)
	if scope.Root != wantRoot {
		t.Errorf("root = %q, want flag's %q (flag should beat env)", scope.Root, wantRoot)
	}
}

func TestResolve_WalkUpHitsParent(t *testing.T) {
	root, _ := setupWorkspace(t)

	// CWD is one directory below root — the workspace file lives in the parent.
	scope, err := resolveFrom("", "", filepath.Join(root, "repoA"))
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeWorkspace {
		t.Errorf("mode = %v, want ModeWorkspace (workspace walk-up should beat single-repo fallback)", scope.Mode)
	}
	wantRoot, _ := filepath.Abs(root)
	if scope.Root != wantRoot {
		t.Errorf("root = %q, want %q", scope.Root, wantRoot)
	}
	if len(scope.Folders) != 2 {
		t.Errorf("folders = %v, want 2 entries", scope.Folders)
	}
}

func TestResolve_WalkUpHitsGrandparent(t *testing.T) {
	root, _ := setupWorkspace(t)
	deep := filepath.Join(root, "repoA", "internal", "subpkg")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}

	scope, err := resolveFrom("", "", deep)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeWorkspace {
		t.Errorf("mode = %v, want ModeWorkspace", scope.Mode)
	}
	wantRoot, _ := filepath.Abs(root)
	if scope.Root != wantRoot {
		t.Errorf("root = %q, want %q", scope.Root, wantRoot)
	}
}

// TestResolve_NoWorkspaceButGitRepo_FallsBackToSingleRepo pins the new
// cascade row: when nothing in the walk-up chain has a workspace file
// but the CWD is inside a git repo, the scope shrinks to that one repo
// instead of erroring. Lets `gh optivem commit` Just Work in a standalone
// clone.
func TestResolve_NoWorkspaceButGitRepo_FallsBackToSingleRepo(t *testing.T) {
	repo := t.TempDir()
	makeGitRepo(t, repo)

	scope, err := resolveFrom("", "", repo)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeSingleRepo {
		t.Errorf("mode = %v, want ModeSingleRepo", scope.Mode)
	}
	wantRoot, _ := filepath.Abs(repo)
	if scope.Root != wantRoot {
		t.Errorf("root = %q, want %q", scope.Root, wantRoot)
	}
	if len(scope.Folders) != 1 || scope.Folders[0] != wantRoot {
		t.Errorf("folders = %v, want [%q]", scope.Folders, wantRoot)
	}
	if scope.SourceFile != "" {
		t.Errorf("SourceFile = %q, want empty in single-repo mode", scope.SourceFile)
	}
}

// TestResolve_SingleRepoWalksUpForGitRoot confirms the single-repo
// fallback can find the repo root from a nested subdirectory — same
// walk-up semantics as the workspace lookup.
func TestResolve_SingleRepoWalksUpForGitRoot(t *testing.T) {
	repo := t.TempDir()
	makeGitRepo(t, repo)
	deep := filepath.Join(repo, "internal", "pkg")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}

	scope, err := resolveFrom("", "", deep)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeSingleRepo {
		t.Errorf("mode = %v, want ModeSingleRepo", scope.Mode)
	}
	wantRoot, _ := filepath.Abs(repo)
	if scope.Root != wantRoot {
		t.Errorf("root = %q, want %q", scope.Root, wantRoot)
	}
}

func TestResolve_WalkUpMiss(t *testing.T) {
	// A directory with no workspace file AND no git repo anywhere on the
	// way up to the filesystem root. On most CI systems that means walking
	// up to "/" (or "C:\") finds nothing — t.TempDir() lives under the OS
	// temp dir, which is not inside a git checkout.
	cwd := t.TempDir()
	_, err := resolveFrom("", "", cwd)
	if err == nil {
		t.Fatal("resolveFrom: expected error for walk-up miss, got nil")
	}
	if !strings.Contains(err.Error(), "no *.code-workspace file or git repo") {
		t.Errorf("error = %v, want mention of missing workspace/git repo", err)
	}
	if !strings.Contains(err.Error(), EnvVar) {
		t.Errorf("error = %v, want mention of $%s for discoverability", err, EnvVar)
	}
}

func TestResolve_MalformedJSON(t *testing.T) {
	root := t.TempDir()
	writeWorkspace(t, root, "myworkspace.code-workspace", "not-json-at-all")

	_, err := resolveFrom(root, "", "")
	if err == nil {
		t.Fatal("resolveFrom: expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "could not parse") {
		t.Errorf("error = %v, want 'could not parse'", err)
	}
}

func TestResolve_MissingFoldersKey(t *testing.T) {
	root := t.TempDir()
	writeWorkspace(t, root, "myworkspace.code-workspace", `{}`)

	_, err := resolveFrom(root, "", "")
	if err == nil {
		t.Fatal("resolveFrom: expected error for missing folders[], got nil")
	}
	if !strings.Contains(err.Error(), "folders[]") {
		t.Errorf("error = %v, want mention of folders[]", err)
	}
}

func TestResolve_EmptyFoldersArray(t *testing.T) {
	root := t.TempDir()
	writeWorkspace(t, root, "myworkspace.code-workspace", `{"folders":[]}`)

	_, err := resolveFrom(root, "", "")
	if err == nil {
		t.Fatal("resolveFrom: expected error for empty folders[], got nil")
	}
}

func TestResolve_NonGitFolderFiltered(t *testing.T) {
	root := t.TempDir()
	makeGitRepo(t, filepath.Join(root, "real"))
	if err := os.MkdirAll(filepath.Join(root, "fake"), 0o755); err != nil {
		t.Fatalf("mkdir fake: %v", err)
	}
	writeWorkspace(t, root, "myworkspace.code-workspace", workspaceJSON("real", "fake"))

	scope, err := resolveFrom(root, "", "")
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	wantRoot, _ := filepath.Abs(root)
	want := []string{filepath.Join(wantRoot, "real")}
	if !equalStrings(scope.Folders, want) {
		t.Errorf("folders = %v, want %v (fake should be filtered)", scope.Folders, want)
	}
}

func TestResolve_MissingPathFiltered(t *testing.T) {
	root := t.TempDir()
	makeGitRepo(t, filepath.Join(root, "real"))
	writeWorkspace(t, root, "myworkspace.code-workspace", workspaceJSON("real", "doesNotExist"))

	scope, err := resolveFrom(root, "", "")
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if len(scope.Folders) != 1 {
		t.Errorf("folders = %v, want 1 entry (missing path should be filtered)", scope.Folders)
	}
}

func TestResolve_FlagDirHasNoWorkspaceFile(t *testing.T) {
	dir := t.TempDir()
	_, err := resolveFrom(dir, "", "")
	if err == nil {
		t.Fatal("resolveFrom: expected error when flag dir has no workspace file")
	}
	if !strings.Contains(err.Error(), "no *.code-workspace") {
		t.Errorf("error = %v, want mention of missing workspace file", err)
	}
}

func TestResolve_FlagDirHasMultipleWorkspaceFiles(t *testing.T) {
	dir := t.TempDir()
	writeWorkspace(t, dir, "a.code-workspace", `{"folders":[]}`)
	writeWorkspace(t, dir, "b.code-workspace", `{"folders":[]}`)

	_, err := resolveFrom(dir, "", "")
	if err == nil {
		t.Fatal("resolveFrom: expected error for multiple workspace files")
	}
	if !strings.Contains(err.Error(), "multiple") {
		t.Errorf("error = %v, want 'multiple'", err)
	}
}

func TestResolve_FlagDirDoesNotExist(t *testing.T) {
	_, err := resolveFrom(filepath.Join(t.TempDir(), "nope"), "", "")
	if err == nil {
		t.Fatal("resolveFrom: expected error for non-existent flag dir")
	}
}

func TestResolve_EmptyPathEntrySkipped(t *testing.T) {
	root := t.TempDir()
	makeGitRepo(t, filepath.Join(root, "real"))
	writeWorkspace(t, root, "myworkspace.code-workspace",
		`{"folders":[{"path":""},{"path":"real"}]}`)

	scope, err := resolveFrom(root, "", "")
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if len(scope.Folders) != 1 {
		t.Errorf("folders = %v, want 1 entry (empty path should be skipped)", scope.Folders)
	}
}

// TestResolve_WorktreeInsideWorkspace_FallsThroughToSingleRepo pins the
// fix for the ATDD rehearsal worktree case: a git repo placed inside a
// workspace tree (e.g. an ATDD worktree at academy/worktrees/rehearsal-X/)
// whose own root is NOT one of the workspace's folders[] entries must
// resolve to ModeSingleRepo on the worktree itself, NOT to ModeWorkspace
// iterating the surrounding workspace's declared folders. Without this,
// running a cross-repo verb from inside the worktree would silently
// iterate the workspace's declared repos and skip the worktree entirely.
func TestResolve_WorktreeInsideWorkspace_FallsThroughToSingleRepo(t *testing.T) {
	root := t.TempDir()
	makeGitRepo(t, filepath.Join(root, "repoA"))
	makeGitRepo(t, filepath.Join(root, "repoB"))
	// A sibling repo NOT declared in folders[] — the worktree case.
	worktree := filepath.Join(root, "worktrees", "rehearsal-x")
	makeGitRepo(t, worktree)
	writeWorkspace(t, root, "myworkspace.code-workspace",
		workspaceJSON("repoA", "repoB"))

	scope, err := resolveFrom("", "", worktree)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeSingleRepo {
		t.Fatalf("mode = %v, want ModeSingleRepo (worktree not in workspace folders[] should fall through)", scope.Mode)
	}
	wantRoot, _ := filepath.Abs(worktree)
	if scope.Root != wantRoot {
		t.Errorf("root = %q, want worktree root %q", scope.Root, wantRoot)
	}
	if len(scope.Folders) != 1 || scope.Folders[0] != wantRoot {
		t.Errorf("folders = %v, want [%q] (single-repo on the worktree)", scope.Folders, wantRoot)
	}
}

// TestResolve_WorktreeInsideWorkspace_FlagStillOverrides pins that the
// CWD-membership check applies only to walk-up — an explicit
// --workspace flag is honored even when CWD is outside the workspace's
// folders[]. Reflects the doc comment: "explicit operator intent".
func TestResolve_WorktreeInsideWorkspace_FlagStillOverrides(t *testing.T) {
	root := t.TempDir()
	makeGitRepo(t, filepath.Join(root, "repoA"))
	makeGitRepo(t, filepath.Join(root, "repoB"))
	worktree := filepath.Join(root, "worktrees", "rehearsal-x")
	makeGitRepo(t, worktree)
	writeWorkspace(t, root, "myworkspace.code-workspace",
		workspaceJSON("repoA", "repoB"))

	scope, err := resolveFrom(root, "", worktree)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeWorkspace {
		t.Fatalf("mode = %v, want ModeWorkspace (explicit flag should beat membership check)", scope.Mode)
	}
}

// TestResolve_WorktreeInsideWorkspace_EnvStillOverrides — symmetric to
// the flag test, for $GH_OPTIVEM_WORKSPACE.
func TestResolve_WorktreeInsideWorkspace_EnvStillOverrides(t *testing.T) {
	root := t.TempDir()
	makeGitRepo(t, filepath.Join(root, "repoA"))
	makeGitRepo(t, filepath.Join(root, "repoB"))
	worktree := filepath.Join(root, "worktrees", "rehearsal-x")
	makeGitRepo(t, worktree)
	writeWorkspace(t, root, "myworkspace.code-workspace",
		workspaceJSON("repoA", "repoB"))

	scope, err := resolveFrom("", root, worktree)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeWorkspace {
		t.Fatalf("mode = %v, want ModeWorkspace (explicit env var should beat membership check)", scope.Mode)
	}
}

// TestResolve_WorktreeInsideWorkspace_ProjectConfigWins pins that when a
// gh-optivem.yaml exists alongside the worktree (or somewhere on the
// walk-up path between the worktree and the surrounding workspace) and
// has a usable repos: list, the cascade lands on ModeProject — the
// project config is the next fallback after the workspace-walk-up
// membership check fails.
func TestResolve_WorktreeInsideWorkspace_ProjectConfigWins(t *testing.T) {
	root := t.TempDir()
	makeGitRepo(t, filepath.Join(root, "repoA"))
	makeGitRepo(t, filepath.Join(root, "repoB"))
	writeWorkspace(t, root, "myworkspace.code-workspace",
		workspaceJSON("repoA", "repoB"))

	// The worktree is itself a git repo, AND it carries a
	// gh-optivem.yaml declaring its own repos[]. The membership check
	// rejects the surrounding workspace; the project config wins.
	worktree := filepath.Join(root, "worktrees", "rehearsal-x")
	makeGitRepo(t, worktree)
	makeGitRepo(t, filepath.Join(worktree, "subrepo"))
	writeProjectConfig(t, worktree, projectConfigYAML("subrepo"))

	scope, err := resolveFrom("", "", worktree)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeProject {
		t.Fatalf("mode = %v, want ModeProject (worktree's own gh-optivem.yaml should win)", scope.Mode)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// writeProjectConfig writes a gh-optivem.yaml at <dir>/gh-optivem.yaml
// with the given YAML body. Used by the project-iteration cascade tests
// below.
func writeProjectConfig(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "gh-optivem.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write gh-optivem.yaml %s: %v", p, err)
	}
	return p
}

// projectConfigYAML renders a minimal gh-optivem.yaml body with the
// supplied repo paths. The project.provider line satisfies Validate's
// Rule 19; no architecture is set so the rest of the schema stays
// dormant — keeps the fixture small while exercising the LocalRepos
// path the cascade reads.
func projectConfigYAML(paths ...string) string {
	var sb strings.Builder
	sb.WriteString("project:\n  provider: github\n")
	if len(paths) > 0 {
		sb.WriteString("repos:\n")
		for _, p := range paths {
			sb.WriteString("  - path: ")
			sb.WriteString(p)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// TestResolve_ProjectModeFromWalkUp pins the new project-iteration row:
// a gh-optivem.yaml above CWD with a non-empty repos: list resolves to
// ModeProject and enumerates those repos.
func TestResolve_ProjectModeFromWalkUp(t *testing.T) {
	root := t.TempDir()
	makeGitRepo(t, filepath.Join(root, "backend"))
	makeGitRepo(t, filepath.Join(root, "frontend"))
	writeProjectConfig(t, root, projectConfigYAML("backend", "frontend"))

	// CWD is somewhere unrelated so the walk-up has to find the file.
	scope, err := resolveFrom("", "", root)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeProject {
		t.Fatalf("mode = %v, want ModeProject", scope.Mode)
	}
	wantRoot, _ := filepath.Abs(root)
	if scope.Root != wantRoot {
		t.Errorf("root = %q, want %q", scope.Root, wantRoot)
	}
	want := []string{
		filepath.Join(wantRoot, "backend"),
		filepath.Join(wantRoot, "frontend"),
	}
	if !equalStrings(scope.Folders, want) {
		t.Errorf("folders = %v, want %v", scope.Folders, want)
	}
	if scope.SourceFile == "" {
		t.Errorf("SourceFile must be populated in project mode (got empty)")
	}
}

// TestResolve_ProjectModeWalksUpFromSubdirectory pins that the cascade
// can find gh-optivem.yaml from a nested CWD — symmetric with the
// workspace-file walk-up.
func TestResolve_ProjectModeWalksUpFromSubdirectory(t *testing.T) {
	root := t.TempDir()
	makeGitRepo(t, filepath.Join(root, "backend"))
	makeGitRepo(t, filepath.Join(root, "frontend"))
	writeProjectConfig(t, root, projectConfigYAML("backend", "frontend"))

	deep := filepath.Join(root, "backend", "internal", "pkg")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}

	scope, err := resolveFrom("", "", deep)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeProject {
		t.Fatalf("mode = %v, want ModeProject (project walk-up should beat single-repo)", scope.Mode)
	}
	if len(scope.Folders) != 2 {
		t.Errorf("folders = %v, want 2 entries", scope.Folders)
	}
}

// TestResolve_WorkspaceFileBeatsProjectConfig pins the cascade order:
// a *.code-workspace file at the same level wins over gh-optivem.yaml.
// Reflects the plan's intent that the workspace file is the broadest
// scope; the project row is a fallback when no workspace file is
// reachable.
func TestResolve_WorkspaceFileBeatsProjectConfig(t *testing.T) {
	root, _ := setupWorkspace(t)
	// Place a gh-optivem.yaml alongside the workspace file. With repos:
	// pointing at a real git repo so the project row would be
	// well-formed if it were ever consulted.
	makeGitRepo(t, filepath.Join(root, "projectOnly"))
	writeProjectConfig(t, root, projectConfigYAML("projectOnly"))

	scope, err := resolveFrom("", "", filepath.Join(root, "repoA"))
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeWorkspace {
		t.Errorf("mode = %v, want ModeWorkspace (workspace file should win)", scope.Mode)
	}
}

// TestResolve_ProjectConfigWithEmptyReposFallsThrough pins the
// monolith / not-yet-multitier behavior: a gh-optivem.yaml exists but
// has no repos: list, so the cascade falls through to the single-repo
// row (the cwd repo).
func TestResolve_ProjectConfigWithEmptyReposFallsThrough(t *testing.T) {
	root := t.TempDir()
	makeGitRepo(t, root)
	writeProjectConfig(t, root, projectConfigYAML())

	scope, err := resolveFrom("", "", root)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeSingleRepo {
		t.Errorf("mode = %v, want ModeSingleRepo (empty repos: should fall through)", scope.Mode)
	}
	wantRoot, _ := filepath.Abs(root)
	if scope.Root != wantRoot {
		t.Errorf("root = %q, want %q", scope.Root, wantRoot)
	}
}

// TestResolve_ProjectModeFiltersMissingPaths matches parseWorkspaceFolders'
// silent-filter behavior — a repos[] entry pointing at a non-existent
// folder (or one without .git/) is skipped, not surfaced as an error.
// Lets a half-scaffolded multitier project still iterate the tiers
// that are present.
func TestResolve_ProjectModeFiltersMissingPaths(t *testing.T) {
	root := t.TempDir()
	makeGitRepo(t, filepath.Join(root, "real"))
	if err := os.MkdirAll(filepath.Join(root, "notAGit"), 0o755); err != nil {
		t.Fatalf("mkdir notAGit: %v", err)
	}
	writeProjectConfig(t, root, projectConfigYAML("real", "notAGit", "doesNotExist"))

	scope, err := resolveFrom("", "", root)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeProject {
		t.Fatalf("mode = %v, want ModeProject", scope.Mode)
	}
	wantRoot, _ := filepath.Abs(root)
	want := []string{filepath.Join(wantRoot, "real")}
	if !equalStrings(scope.Folders, want) {
		t.Errorf("folders = %v, want %v", scope.Folders, want)
	}
}

// TestResolve_ProjectModeAllReposMissingFallsThrough pins that a
// gh-optivem.yaml with repos: where every entry filters out (no .git,
// missing on disk) falls through to the single-repo row rather than
// erroring. The cwd repo is the next-best scope.
func TestResolve_ProjectModeAllReposMissingFallsThrough(t *testing.T) {
	root := t.TempDir()
	makeGitRepo(t, root)
	writeProjectConfig(t, root, projectConfigYAML("ghost-a", "ghost-b"))

	scope, err := resolveFrom("", "", root)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeSingleRepo {
		t.Errorf("mode = %v, want ModeSingleRepo (all repos missing -> fall through)", scope.Mode)
	}
}

// TestResolve_MalformedProjectConfigErrors pins that a gh-optivem.yaml
// the operator broke by hand is surfaced — silent fall-through would
// hide the bug. Errors out instead of falling back to single-repo.
// Applies only when the config belongs to CWD's repo (sits at CWD's
// repo root); a broken outer file is handled by
// TestResolve_StrayOuterProjectConfigBroken_FallsThroughSilently below.
func TestResolve_MalformedProjectConfigErrors(t *testing.T) {
	root := t.TempDir()
	makeGitRepo(t, root)
	writeProjectConfig(t, root, "project: [this, is, not, a, map\n")

	_, err := resolveFrom("", "", root)
	if err == nil {
		t.Fatal("expected error for malformed gh-optivem.yaml, got nil")
	}
}

// TestResolve_StrayOuterProjectConfig_FallsThroughToSingleRepo pins the
// CWD-membership guard on the project-config walk-up: a git repo (e.g.
// an ATDD rehearsal worktree) placed inside a directory that has a
// gh-optivem.yaml NOT claiming the repo must fall through to
// ModeSingleRepo on the worktree, NOT inherit the outer config's
// scope. Mirrors the row-3 membership guard for *.code-workspace files
// and protects the rehearsal-worktree case where worktrees live under
// <academy>/worktrees/ alongside an unrelated academy gh-optivem.yaml.
func TestResolve_StrayOuterProjectConfig_FallsThroughToSingleRepo(t *testing.T) {
	outer := t.TempDir()
	makeGitRepo(t, filepath.Join(outer, "unrelated"))
	writeProjectConfig(t, outer, projectConfigYAML("unrelated"))

	worktree := filepath.Join(outer, "worktrees", "rehearsal-x")
	makeGitRepo(t, worktree)

	scope, err := resolveFrom("", "", worktree)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if scope.Mode != ModeSingleRepo {
		t.Fatalf("mode = %v, want ModeSingleRepo (outer project config should be skipped)", scope.Mode)
	}
	wantRoot, _ := filepath.Abs(worktree)
	if scope.Root != wantRoot {
		t.Errorf("root = %q, want worktree root %q", scope.Root, wantRoot)
	}
	if len(scope.Folders) != 1 || scope.Folders[0] != wantRoot {
		t.Errorf("folders = %v, want [%q] (single-repo on the worktree)", scope.Folders, wantRoot)
	}
}

// TestResolve_StrayOuterProjectConfigBroken_FallsThroughSilently pins
// the parse-error suppression for outer configs: if the stray outer
// gh-optivem.yaml is malformed (e.g. an old academy-level file using
// the pre-kebab snake_case schema), the parse error must NOT bubble up
// — it's not our config to complain about. The cascade falls through
// to ModeSingleRepo on the worktree.
func TestResolve_StrayOuterProjectConfigBroken_FallsThroughSilently(t *testing.T) {
	outer := t.TempDir()
	writeProjectConfig(t, outer, "repo_strategy: mono-repo\nsystem_name: snake-case-no-longer-valid\n")

	worktree := filepath.Join(outer, "worktrees", "rehearsal-x")
	makeGitRepo(t, worktree)

	scope, err := resolveFrom("", "", worktree)
	if err != nil {
		t.Fatalf("resolveFrom: %v (want silent fall-through to ModeSingleRepo)", err)
	}
	if scope.Mode != ModeSingleRepo {
		t.Fatalf("mode = %v, want ModeSingleRepo", scope.Mode)
	}
	wantRoot, _ := filepath.Abs(worktree)
	if scope.Root != wantRoot {
		t.Errorf("root = %q, want worktree root %q", scope.Root, wantRoot)
	}
}
