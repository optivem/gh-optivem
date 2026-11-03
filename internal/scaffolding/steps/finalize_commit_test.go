package steps

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitIn runs git in dir and fails the test on error.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newClonedRepo returns a working clone (with one commit on main) of a local
// bare remote, plus the bare remote path.
func newClonedRepo(t *testing.T) (work, bare string) {
	t.Helper()
	// Code under test shells out to git commit; CI runners have no identity.
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	root := t.TempDir()
	bare = filepath.Join(root, "remote.git")
	work = filepath.Join(root, "work")
	gitIn(t, root, "init", "--bare", "-b", "main", bare)
	gitIn(t, root, "clone", bare, work)
	gitIn(t, work, "checkout", "-B", "main")
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, work, "add", "-A")
	gitIn(t, work, "commit", "-m", "seed")
	gitIn(t, work, "push", "-u", "origin", "main")
	return work, bare
}

func TestStageCommitAndPush_CleanTreePreExisting_NoOp(t *testing.T) {
	work, bare := newClonedRepo(t)
	before := gitIn(t, bare, "rev-parse", "main")

	pushed, err := stageCommitAndPush(work, "acme/shop", "msg", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pushed {
		t.Fatal("expected pushed=false for a clean pre-existing repo")
	}
	if after := gitIn(t, bare, "rev-parse", "main"); after != before {
		t.Fatalf("remote main moved from %s to %s; expected no commit", before, after)
	}
}

func TestStageCommitAndPush_CleanTreeFresh_Errors(t *testing.T) {
	work, _ := newClonedRepo(t)

	pushed, err := stageCommitAndPush(work, "acme/shop", "msg", false)
	if err == nil {
		t.Fatal("expected an error for a clean tree in a fresh repo")
	}
	if pushed {
		t.Fatal("expected pushed=false on error")
	}
	if !strings.Contains(err.Error(), "acme/shop") || !strings.Contains(err.Error(), "clean") {
		t.Fatalf("error should name the repo and the clean tree, got: %v", err)
	}
}

func TestStageCommitAndPush_DirtyPreExisting_CommitsAndPushes(t *testing.T) {
	work, bare := newClonedRepo(t)
	before := gitIn(t, bare, "rev-parse", "main")
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pushed, err := stageCommitAndPush(work, "acme/shop", "Apply pipeline template", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pushed {
		t.Fatal("expected pushed=true for a dirty tree")
	}
	if after := gitIn(t, bare, "rev-parse", "main"); after == before {
		t.Fatal("remote main did not advance")
	}
	if got := gitIn(t, bare, "log", "-1", "--format=%s", "main"); got != "Apply pipeline template" {
		t.Fatalf("unexpected commit message %q", got)
	}
	if files := gitIn(t, bare, "ls-tree", "--name-only", "main"); !strings.Contains(files, "new.txt") {
		t.Fatalf("new.txt not pushed: %s", files)
	}
}
