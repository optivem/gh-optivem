package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBranchRefresh_HappyPath_RebasesAndForceWithLeasePushes builds a tiny
// origin/clone topology: a bare remote, an "author" clone that advances main
// with a new commit and pushes it, then a "developer" clone where we create a
// feature branch with one local commit and an upstream. We then call
// runBranchRefresh() and verify that the feature branch ends up containing
// the new main commit AND that the remote tip of the feature branch matches
// the local tip (i.e. the force-with-lease push landed).
func TestBranchRefresh_HappyPath_RebasesAndForceWithLeasePushes(t *testing.T) {
	root := t.TempDir()

	// Bare remote.
	bare := filepath.Join(root, "remote.git")
	if err := os.MkdirAll(bare, 0o755); err != nil {
		t.Fatalf("mkdir bare: %v", err)
	}
	mustGit(t, bare, "init", "-q", "--bare", "-b", "main")

	// Seed clone — push initial main so the bare has a starting commit.
	seed := filepath.Join(root, "seed")
	initTestRepo(t, seed)
	mustGit(t, seed, "remote", "add", "origin", bare)
	mustGit(t, seed, "push", "-q", "-u", "origin", "main")

	// "Author" clone advances origin/main with a new commit so the dev's
	// feature branch is genuinely behind.
	author := filepath.Join(root, "author")
	mustGit(t, root, "clone", "-q", bare, author)
	mustGit(t, author, "config", "user.email", "author@example.com")
	mustGit(t, author, "config", "user.name", "Author")
	if err := os.WriteFile(filepath.Join(author, "main_advance.txt"), []byte("from author\n"), 0o644); err != nil {
		t.Fatalf("write main_advance: %v", err)
	}
	mustGit(t, author, "add", ".")
	mustGit(t, author, "commit", "-q", "-m", "advance main")
	mustGit(t, author, "push", "-q", "origin", "main")

	// "Developer" clone — branches from the *old* main (before the author's
	// commit was pushed). We have to clone before fetching to capture that
	// stale view, but a fresh clone here already sees the new tip, so
	// instead we clone, then reset main back one commit locally, then
	// branch — the rebase will still pick up the new origin/main on fetch.
	dev := filepath.Join(root, "dev")
	mustGit(t, root, "clone", "-q", bare, dev)
	mustGit(t, dev, "config", "user.email", "dev@example.com")
	mustGit(t, dev, "config", "user.name", "Dev")
	// Reset main back to the seed commit so the feature branch starts
	// before the author's advance.
	mustGit(t, dev, "reset", "-q", "--hard", "HEAD~1")
	mustGit(t, dev, "checkout", "-q", "-b", "feature/x")
	if err := os.WriteFile(filepath.Join(dev, "feature.txt"), []byte("feature work\n"), 0o644); err != nil {
		t.Fatalf("write feature: %v", err)
	}
	mustGit(t, dev, "add", ".")
	mustGit(t, dev, "commit", "-q", "-m", "feature commit")
	mustGit(t, dev, "push", "-q", "-u", "origin", "feature/x")

	withWorkdir(t, dev)

	if err := runBranchRefresh(); err != nil {
		t.Fatalf("runBranchRefresh: %v", err)
	}

	// After refresh, the feature branch must contain both the author's
	// "advance main" commit and the dev's "feature commit".
	log := captureGitOut(t, dev, "log", "--oneline")
	if !strings.Contains(log, "advance main") {
		t.Errorf("feature branch missing rebased main commit; log:\n%s", log)
	}
	if !strings.Contains(log, "feature commit") {
		t.Errorf("feature branch missing local feature commit; log:\n%s", log)
	}

	// Local tip must match the remote tip — i.e. the force-with-lease push
	// actually landed.
	localTip := strings.TrimSpace(captureGitOut(t, dev, "rev-parse", "HEAD"))
	remoteTip := strings.TrimSpace(captureGitOut(t, bare, "rev-parse", "refs/heads/feature/x"))
	if localTip != remoteTip {
		t.Errorf("remote feature/x tip %q != local HEAD %q after refresh", remoteTip, localTip)
	}
}

// TestBranchRefresh_OnMain_Refuses verifies that running refresh on `main` is
// rejected with a clear error message — the ritual is for feature branches,
// not for trunk.
func TestBranchRefresh_OnMain_Refuses(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "on-main")
	initTestRepo(t, repo)
	withWorkdir(t, repo)

	err := runBranchRefresh()
	if err == nil {
		t.Fatalf("expected refusal when current branch is main")
	}
	msg := err.Error()
	if !strings.Contains(msg, "main") {
		t.Errorf("error message does not mention main: %v", err)
	}
	if !strings.Contains(msg, "refusing") {
		t.Errorf("error message does not say 'refusing': %v", err)
	}
}
