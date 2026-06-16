// Real-git integration tests for the run-digest commit helpers
// (fullHeadSHA / commitsSince / currentBranch / remoteWebURL /
// compareURL). These shell out to a real `git` against a throwaway repo
// rather than a fake, because the helpers' whole job is to parse real git
// output; a fake would only re-encode the assumptions under test. The
// suite skips when `git` isn't on PATH.
package driver

import (
	"os/exec"
	"testing"
)

// gitRepo initializes a throwaway git repo at t.TempDir() with a
// deterministic identity and an initial commit, returning the repo path.
func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// gitCommit makes an empty commit with the given subject.
func gitCommit(t *testing.T, dir, subject string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-q", "--allow-empty", "-m", subject)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit %q: %v\n%s", subject, err, out)
	}
}

func TestCommitsSince_ListsCommitsAfterBase(t *testing.T) {
	dir := gitRepo(t)
	gitCommit(t, dir, "base commit")
	base := fullHeadSHA(dir)
	if base == "" {
		t.Fatal("fullHeadSHA returned empty for a repo with a commit")
	}
	gitCommit(t, dir, "first change")
	gitCommit(t, dir, "second change")

	commits := commitsSince(dir, base)
	if len(commits) != 2 {
		t.Fatalf("want 2 commits since base, got %d: %+v", len(commits), commits)
	}
	// Newest first, same order GitHub lists a PR's commits.
	if commits[0].subject != "second change" || commits[1].subject != "first change" {
		t.Errorf("unexpected order/subjects: %+v", commits)
	}
	for _, c := range commits {
		if c.shortSHA == "" || c.author != "Test User" || c.relative == "" {
			t.Errorf("incomplete commit row: %+v", c)
		}
	}
}

func TestCommitsSince_EmptyWhenNoNewCommits(t *testing.T) {
	dir := gitRepo(t)
	gitCommit(t, dir, "only commit")
	base := fullHeadSHA(dir)
	if got := commitsSince(dir, base); got != nil {
		t.Errorf("want nil when HEAD == base, got %+v", got)
	}
}

func TestCommitsSince_EmptyBaseOrNonRepo(t *testing.T) {
	if got := commitsSince(t.TempDir(), ""); got != nil {
		t.Errorf("empty base must yield nil, got %+v", got)
	}
	if got := commitsSince(t.TempDir(), "deadbeef"); got != nil {
		t.Errorf("non-repo dir must yield nil, got %+v", got)
	}
}

func TestRemoteWebURL_NormalizesTransports(t *testing.T) {
	cases := map[string]string{
		"https://github.com/optivem/shop.git": "https://github.com/optivem/shop",
		"https://github.com/optivem/shop":     "https://github.com/optivem/shop",
		"git@github.com:optivem/shop.git":     "https://github.com/optivem/shop",
		"ssh://git@github.com/optivem/shop":   "https://github.com/optivem/shop",
		"https://gitlab.com/optivem/shop.git": "", // non-GitHub host: no link
	}
	for remote, want := range cases {
		dir := gitRepo(t)
		cmd := exec.Command("git", "remote", "add", "origin", remote)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git remote add %q: %v\n%s", remote, err, out)
		}
		if got := remoteWebURL(dir); got != want {
			t.Errorf("remoteWebURL(%q) = %q, want %q", remote, got, want)
		}
	}
}

func TestRemoteWebURL_EmptyWithoutOrigin(t *testing.T) {
	if got := remoteWebURL(gitRepo(t)); got != "" {
		t.Errorf("no origin must yield empty, got %q", got)
	}
}

func TestCompareURL_BranchRange(t *testing.T) {
	dir := gitRepo(t)
	gitCommit(t, dir, "base")
	base := fullHeadSHA(dir)
	cmd := exec.Command("git", "remote", "add", "origin", "git@github.com:optivem/shop.git")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}

	got := compareURL(dir, base, 3)
	want := "https://github.com/optivem/shop/compare/" + base + "...main"
	if got != want {
		t.Errorf("compareURL = %q, want %q", got, want)
	}
}

func TestCompareURL_EmptyWhenNothingToLink(t *testing.T) {
	dir := gitRepo(t)
	gitCommit(t, dir, "base")
	base := fullHeadSHA(dir)
	// No commits this run → no link even with a remote present.
	cmd := exec.Command("git", "remote", "add", "origin", "git@github.com:optivem/shop.git")
	cmd.Dir = dir
	cmd.CombinedOutput()
	if got := compareURL(dir, base, 0); got != "" {
		t.Errorf("zero commits must yield empty compare URL, got %q", got)
	}
	// No remote → no link even with commits.
	if got := compareURL(gitRepo(t), "deadbeef", 2); got != "" {
		t.Errorf("no remote must yield empty compare URL, got %q", got)
	}
}
