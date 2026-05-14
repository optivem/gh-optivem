package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo creates a fresh git repo in dir with one initial commit so
// HEAD exists. user.name/user.email are set locally so subsequent commits
// in the test do not fail on machines without a global git identity.
func initTestRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	mustGit(t, dir, "init", "-q", "-b", "main")
	mustGit(t, dir, "config", "user.email", "test@example.com")
	mustGit(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-q", "-m", "seed")
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}

func captureGitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

func TestCommitOneRepo_DirtyWithYes_LandsCommitAndCoAuthorTrailer(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "dirty")
	initTestRepo(t, repo)

	// Modify a tracked file (not untracked — that path is covered separately).
	if err := os.WriteFile(filepath.Join(repo, "seed.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	committed, err := commitOneRepo(repo, "test commit message", commitOptions{Yes: true})
	if err != nil {
		t.Fatalf("commitOneRepo: %v", err)
	}
	if !committed {
		t.Fatalf("expected committed=true")
	}

	log := captureGitOut(t, repo, "log", "-1", "--pretty=%B")
	if !strings.Contains(log, "test commit message") {
		t.Errorf("commit message missing from log; got:\n%s", log)
	}
	if !strings.Contains(log, commitCoAuthor) {
		t.Errorf("co-author trailer missing from log; got:\n%s", log)
	}
}

func TestCommitOneRepo_Clean_NoCommit(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "clean")
	initTestRepo(t, repo)

	before := strings.TrimSpace(captureGitOut(t, repo, "rev-parse", "HEAD"))

	committed, err := commitOneRepo(repo, "should-not-be-used", commitOptions{Yes: true})
	if err != nil {
		t.Fatalf("commitOneRepo: %v", err)
	}
	if committed {
		t.Fatalf("expected committed=false on clean repo")
	}
	after := strings.TrimSpace(captureGitOut(t, repo, "rev-parse", "HEAD"))
	if before != after {
		t.Errorf("HEAD moved on clean repo: %s → %s", before, after)
	}
}

func TestCommitOneRepo_YesUntrackedWithoutOptIn_Refuses(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "with-untracked")
	initTestRepo(t, repo)

	// Drop an untracked file in the working tree.
	if err := os.WriteFile(filepath.Join(repo, "stray.log"), []byte("oops\n"), 0o644); err != nil {
		t.Fatalf("write stray: %v", err)
	}

	committed, err := commitOneRepo(repo, "ignored", commitOptions{Yes: true})
	if err == nil {
		t.Fatalf("expected error refusing untracked stage; got committed=%v", committed)
	}
	if !strings.Contains(err.Error(), "--yes refuses to stage untracked files") {
		t.Errorf("error did not mention untracked-refusal: %v", err)
	}
}

func TestCommitOneRepo_YesUntrackedWithOptIn_Commits(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "include-untracked")
	initTestRepo(t, repo)
	if err := os.WriteFile(filepath.Join(repo, "stray.log"), []byte("oops\n"), 0o644); err != nil {
		t.Fatalf("write stray: %v", err)
	}

	committed, err := commitOneRepo(repo, "stage stray", commitOptions{Yes: true, IncludeUntracked: true})
	if err != nil {
		t.Fatalf("commitOneRepo: %v", err)
	}
	if !committed {
		t.Fatalf("expected committed=true with --include-untracked")
	}
	tracked := captureGitOut(t, repo, "ls-files", "stray.log")
	if !strings.Contains(tracked, "stray.log") {
		t.Errorf("stray.log not tracked after commit: %q", tracked)
	}
}

func TestCommitOneRepo_DirtyWithoutMessage_Errors(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "no-msg")
	initTestRepo(t, repo)
	if err := os.WriteFile(filepath.Join(repo, "seed.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	_, err := commitOneRepo(repo, "", commitOptions{Yes: true})
	if err == nil {
		t.Fatalf("expected error when message is empty and repo is dirty")
	}
	if !strings.Contains(err.Error(), "commit message is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRepoBaseName(t *testing.T) {
	cases := map[string]string{
		`/a/b/myrepo`:    "myrepo",
		`C:\foo\bar\baz`: "baz",
		`myrepo`:         "myrepo",
		`/a/b/myrepo/`:   "myrepo",
		`C:\foo\baz\`:    "baz",
	}
	for in, want := range cases {
		if got := repoBaseName(in); got != want {
			t.Errorf("repoBaseName(%q) = %q, want %q", in, got, want)
		}
	}
}
