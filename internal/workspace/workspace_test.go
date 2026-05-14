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
// repo well enough for parseWorkspaceFolders' filter).
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

// setupAcademy builds a tempdir laid out like the real academy workspace:
//
//	<root>/
//	  academy.code-workspace
//	  repoA/.git/
//	  repoB/.git/
//	  notAGit/
func setupAcademy(t *testing.T) (root string, wsPath string) {
	t.Helper()
	root = t.TempDir()
	makeGitRepo(t, filepath.Join(root, "repoA"))
	makeGitRepo(t, filepath.Join(root, "repoB"))
	if err := os.MkdirAll(filepath.Join(root, "notAGit"), 0o755); err != nil {
		t.Fatalf("mkdir notAGit: %v", err)
	}
	wsPath = writeWorkspace(t, root, "academy.code-workspace",
		workspaceJSON("repoA", "repoB", "notAGit", "missingDir"))
	return root, wsPath
}

func TestResolve_ExplicitFlag(t *testing.T) {
	root, _ := setupAcademy(t)

	gotRoot, folders, err := resolveFrom(root, "", filepath.Join(root, "unrelated"))
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	wantRoot, _ := filepath.Abs(root)
	if gotRoot != wantRoot {
		t.Errorf("root = %q, want %q", gotRoot, wantRoot)
	}
	wantFolders := []string{
		filepath.Join(wantRoot, "repoA"),
		filepath.Join(wantRoot, "repoB"),
	}
	if !equalStrings(folders, wantFolders) {
		t.Errorf("folders = %v, want %v", folders, wantFolders)
	}
}

func TestResolve_EnvVar(t *testing.T) {
	root, _ := setupAcademy(t)

	// CWD points somewhere that has no workspace file. The env var should win.
	cwd := t.TempDir()
	_, folders, err := resolveFrom("", root, cwd)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if len(folders) != 2 {
		t.Errorf("folders = %v, want 2 entries", folders)
	}
}

func TestResolve_FlagBeatsEnv(t *testing.T) {
	flagRoot, _ := setupAcademy(t)
	envRoot, _ := setupAcademy(t) // a different academy; we never expect to pick from this one

	gotRoot, _, err := resolveFrom(flagRoot, envRoot, t.TempDir())
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	wantRoot, _ := filepath.Abs(flagRoot)
	if gotRoot != wantRoot {
		t.Errorf("root = %q, want flag's %q (flag should beat env)", gotRoot, wantRoot)
	}
}

func TestResolve_WalkUpHitsParent(t *testing.T) {
	root, _ := setupAcademy(t)
	makeGitRepo(t, filepath.Join(root, "repoA")) // already exists; reuse path

	// CWD is one directory below root — the workspace file lives in the parent.
	gotRoot, folders, err := resolveFrom("", "", filepath.Join(root, "repoA"))
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	wantRoot, _ := filepath.Abs(root)
	if gotRoot != wantRoot {
		t.Errorf("root = %q, want %q", gotRoot, wantRoot)
	}
	if len(folders) != 2 {
		t.Errorf("folders = %v, want 2 entries", folders)
	}
}

func TestResolve_WalkUpHitsGrandparent(t *testing.T) {
	root, _ := setupAcademy(t)
	deep := filepath.Join(root, "repoA", "internal", "subpkg")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}

	gotRoot, _, err := resolveFrom("", "", deep)
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	wantRoot, _ := filepath.Abs(root)
	if gotRoot != wantRoot {
		t.Errorf("root = %q, want %q", gotRoot, wantRoot)
	}
}

func TestResolve_WalkUpMiss(t *testing.T) {
	// A directory with no workspace file anywhere on the way up to the
	// filesystem root. On most systems that means walking up to "/" (or
	// "C:\") finds nothing.
	cwd := t.TempDir()
	_, _, err := resolveFrom("", "", cwd)
	if err == nil {
		t.Fatal("resolveFrom: expected error for walk-up miss, got nil")
	}
	if !strings.Contains(err.Error(), "no *.code-workspace") {
		t.Errorf("error = %v, want mention of missing workspace file", err)
	}
	if !strings.Contains(err.Error(), EnvVar) {
		t.Errorf("error = %v, want mention of $%s for discoverability", err, EnvVar)
	}
}

func TestResolve_MalformedJSON(t *testing.T) {
	root := t.TempDir()
	writeWorkspace(t, root, "academy.code-workspace", "not-json-at-all")

	_, _, err := resolveFrom(root, "", "")
	if err == nil {
		t.Fatal("resolveFrom: expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "could not parse") {
		t.Errorf("error = %v, want 'could not parse'", err)
	}
}

func TestResolve_MissingFoldersKey(t *testing.T) {
	root := t.TempDir()
	writeWorkspace(t, root, "academy.code-workspace", `{}`)

	_, _, err := resolveFrom(root, "", "")
	if err == nil {
		t.Fatal("resolveFrom: expected error for missing folders[], got nil")
	}
	if !strings.Contains(err.Error(), "folders[]") {
		t.Errorf("error = %v, want mention of folders[]", err)
	}
}

func TestResolve_EmptyFoldersArray(t *testing.T) {
	root := t.TempDir()
	writeWorkspace(t, root, "academy.code-workspace", `{"folders":[]}`)

	_, _, err := resolveFrom(root, "", "")
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
	writeWorkspace(t, root, "academy.code-workspace", workspaceJSON("real", "fake"))

	_, folders, err := resolveFrom(root, "", "")
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	wantRoot, _ := filepath.Abs(root)
	want := []string{filepath.Join(wantRoot, "real")}
	if !equalStrings(folders, want) {
		t.Errorf("folders = %v, want %v (fake should be filtered)", folders, want)
	}
}

func TestResolve_MissingPathFiltered(t *testing.T) {
	root := t.TempDir()
	makeGitRepo(t, filepath.Join(root, "real"))
	writeWorkspace(t, root, "academy.code-workspace", workspaceJSON("real", "doesNotExist"))

	_, folders, err := resolveFrom(root, "", "")
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if len(folders) != 1 {
		t.Errorf("folders = %v, want 1 entry (missing path should be filtered)", folders)
	}
}

func TestResolve_FlagDirHasNoWorkspaceFile(t *testing.T) {
	dir := t.TempDir()
	_, _, err := resolveFrom(dir, "", "")
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

	_, _, err := resolveFrom(dir, "", "")
	if err == nil {
		t.Fatal("resolveFrom: expected error for multiple workspace files")
	}
	if !strings.Contains(err.Error(), "multiple") {
		t.Errorf("error = %v, want 'multiple'", err)
	}
}

func TestResolve_FlagDirDoesNotExist(t *testing.T) {
	_, _, err := resolveFrom(filepath.Join(t.TempDir(), "nope"), "", "")
	if err == nil {
		t.Fatal("resolveFrom: expected error for non-existent flag dir")
	}
}

func TestResolve_EmptyPathEntrySkipped(t *testing.T) {
	root := t.TempDir()
	makeGitRepo(t, filepath.Join(root, "real"))
	writeWorkspace(t, root, "academy.code-workspace",
		`{"folders":[{"path":""},{"path":"real"}]}`)

	_, folders, err := resolveFrom(root, "", "")
	if err != nil {
		t.Fatalf("resolveFrom: %v", err)
	}
	if len(folders) != 1 {
		t.Errorf("folders = %v, want 1 entry (empty path should be skipped)", folders)
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
