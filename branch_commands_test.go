package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepoWithBareRemote sets up a bare repo at <root>/remote.git and a
// clone at <root>/work with `main` tracking `origin/main`, so that
// `git pull --rebase` inside the clone has a real upstream to talk to.
// Returns the clone's working-tree path.
func initTestRepoWithBareRemote(t *testing.T, root string) string {
	t.Helper()
	remote := filepath.Join(root, "remote.git")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatalf("mkdir remote: %v", err)
	}
	mustGit(t, remote, "init", "-q", "--bare", "-b", "main")

	seed := filepath.Join(root, "seed")
	initTestRepo(t, seed)
	mustGit(t, seed, "remote", "add", "origin", remote)
	mustGit(t, seed, "push", "-q", "-u", "origin", "main")

	work := filepath.Join(root, "work")
	mustGit(t, root, "clone", "-q", "-b", "main", remote, work)
	mustGit(t, work, "config", "user.email", "test@example.com")
	mustGit(t, work, "config", "user.name", "Test")
	return work
}

func TestBranchStart_FreshRepoOnMain_SwitchesToNewBranch(t *testing.T) {
	root := t.TempDir()
	work := initTestRepoWithBareRemote(t, root)
	withWorkdir(t, work)

	if err := runBranchStart("feature/x"); err != nil {
		t.Fatalf("runBranchStart: %v", err)
	}

	got := strings.TrimSpace(captureGitOut(t, work, "rev-parse", "--abbrev-ref", "HEAD"))
	if got != "feature/x" {
		t.Errorf("HEAD after branch start = %q; want %q", got, "feature/x")
	}
}

func TestBranchStart_EmptyName_RefusesBeforeRunningGit(t *testing.T) {
	root := t.TempDir()
	work := initTestRepoWithBareRemote(t, root)
	withWorkdir(t, work)

	err := runBranchStart("   ")
	if err == nil {
		t.Fatalf("expected error for whitespace-only name")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("error did not flag empty name: %v", err)
	}
	// HEAD must still be on main — no git commands were run.
	got := strings.TrimSpace(captureGitOut(t, work, "rev-parse", "--abbrev-ref", "HEAD"))
	if got != "main" {
		t.Errorf("HEAD after refusal = %q; want %q", got, "main")
	}
}

// TestBranchStart_DirtyTreeBlockingCheckout_PropagatesGitError documents the
// dirty-tree policy: runBranchStart does NOT auto-stash. When the working
// tree carries uncommitted edits to a tracked file whose content differs on
// `main`, `git checkout main` refuses with "would be overwritten" and we
// surface that error verbatim. The operator is expected to commit / stash /
// discard before starting a new branch.
func TestBranchStart_DirtyTreeBlockingCheckout_PropagatesGitError(t *testing.T) {
	root := t.TempDir()
	work := initTestRepoWithBareRemote(t, root)

	// Create a branch `other` with a different version of seed.txt, then
	// leave an uncommitted edit on that branch so `checkout main` refuses.
	mustGit(t, work, "checkout", "-q", "-b", "other")
	if err := os.WriteFile(filepath.Join(work, "seed.txt"), []byte("other-branch\n"), 0o644); err != nil {
		t.Fatalf("write seed on other: %v", err)
	}
	mustGit(t, work, "commit", "-q", "-am", "diverge on other")
	// Now dirty seed.txt with content that does not match main's version.
	if err := os.WriteFile(filepath.Join(work, "seed.txt"), []byte("uncommitted-edit\n"), 0o644); err != nil {
		t.Fatalf("dirty seed on other: %v", err)
	}

	withWorkdir(t, work)
	err := runBranchStart("feature/y")
	if err == nil {
		t.Fatalf("expected runBranchStart to fail on dirty tree blocking checkout main")
	}
	if !strings.Contains(err.Error(), "git checkout main") {
		t.Errorf("error did not name the failing step: %v", err)
	}

	// HEAD must remain on `other` — the failed checkout did not move us.
	got := strings.TrimSpace(captureGitOut(t, work, "rev-parse", "--abbrev-ref", "HEAD"))
	if got != "other" {
		t.Errorf("HEAD after failed start = %q; want %q", got, "other")
	}
}
