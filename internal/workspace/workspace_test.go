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
