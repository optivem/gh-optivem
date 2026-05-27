package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestWorkingTreeClean(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "clean-check")
	initTestRepo(t, repo)

	clean, err := workingTreeClean(repo)
	if err != nil {
		t.Fatalf("workingTreeClean (initial): %v", err)
	}
	if !clean {
		t.Errorf("expected clean on freshly-initialised repo")
	}

	// Untracked file → dirty.
	if err := os.WriteFile(filepath.Join(repo, "stray.log"), []byte("oops\n"), 0o644); err != nil {
		t.Fatalf("write stray: %v", err)
	}
	clean, err = workingTreeClean(repo)
	if err != nil {
		t.Fatalf("workingTreeClean (untracked): %v", err)
	}
	if clean {
		t.Errorf("expected dirty when an untracked file is present")
	}

	// Untracked removed, modified tracked file → dirty.
	if err := os.Remove(filepath.Join(repo, "stray.log")); err != nil {
		t.Fatalf("remove stray: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "seed.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("modify seed: %v", err)
	}
	clean, err = workingTreeClean(repo)
	if err != nil {
		t.Fatalf("workingTreeClean (modified): %v", err)
	}
	if clean {
		t.Errorf("expected dirty when a tracked file is modified")
	}
}

func TestIsNonFastForwardRejection(t *testing.T) {
	cases := map[string]bool{
		"":                                  false,
		"fatal: Authentication failed":      false,
		" ! [rejected]        main -> main (non-fast-forward)\nerror: failed to push some refs":       true,
		" ! [rejected]        main -> main (fetch first)\nhint: Updates were rejected because the tip": true,
		"Updates were rejected because the remote contains work that you do":                            true,
	}
	for in, want := range cases {
		if got := isNonFastForwardRejection(in); got != want {
			t.Errorf("isNonFastForwardRejection(%q) = %v, want %v", in, got, want)
		}
	}
}

// initBareAndClone builds a bare origin and a clone, returning their paths.
// The clone has its committer identity configured locally (so commits in
// the test do not require a global git identity) and a fresh main branch
// pointing at one seed commit pushed to origin.
func initBareAndClone(t *testing.T) (origin, clone string) {
	t.Helper()
	root := t.TempDir()
	origin = filepath.Join(root, "origin.git")
	clone = filepath.Join(root, "clone")

	mustGit(t, root, "init", "-q", "--bare", "-b", "main", origin)

	mustGit(t, root, "clone", "-q", origin, clone)
	mustGit(t, clone, "config", "user.email", "test@example.com")
	mustGit(t, clone, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(clone, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	mustGit(t, clone, "add", ".")
	mustGit(t, clone, "commit", "-q", "-m", "seed")
	mustGit(t, clone, "push", "-q", "origin", "main")
	return origin, clone
}

func TestPullWithAutoStash_DirtyTrackedFile_PreservesEditAndPullsRemote(t *testing.T) {
	origin, a := initBareAndClone(t)
	// Second clone that races ahead and pushes a new file.
	rootB := t.TempDir()
	b := filepath.Join(rootB, "b")
	mustGit(t, rootB, "clone", "-q", origin, b)
	mustGit(t, b, "config", "user.email", "test@example.com")
	mustGit(t, b, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(b, "remote-added.txt"), []byte("from b\n"), 0o644); err != nil {
		t.Fatalf("write remote-added: %v", err)
	}
	mustGit(t, b, "add", ".")
	mustGit(t, b, "commit", "-q", "-m", "from b")
	mustGit(t, b, "push", "-q", "origin", "main")

	// Now `a` has dirty tracked changes; pull --rebase via the helper should
	// stash them, pull, and pop — leaving the dirty edit in the working tree
	// AND the remote-added file present.
	if err := os.WriteFile(filepath.Join(a, "seed.txt"), []byte("local edit\n"), 0o644); err != nil {
		t.Fatalf("dirty seed: %v", err)
	}

	if err := pullWithAutoStash(a); err != nil {
		t.Fatalf("pullWithAutoStash: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(a, "seed.txt"))
	if err != nil {
		t.Fatalf("read seed back: %v", err)
	}
	if strings.TrimSpace(string(got)) != "local edit" {
		t.Errorf("local dirty edit lost; got %q", got)
	}
	if _, err := os.Stat(filepath.Join(a, "remote-added.txt")); err != nil {
		t.Errorf("remote-added.txt not pulled in: %v", err)
	}
}

func TestPullWithAutoStash_CleanWorkingTree_NoStashNoError(t *testing.T) {
	_, clone := initBareAndClone(t)
	if err := pullWithAutoStash(clone); err != nil {
		t.Fatalf("pullWithAutoStash on clean tree: %v", err)
	}
	// No stash entries left behind.
	out := captureGitOut(t, clone, "stash", "list")
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected empty stash list, got %q", out)
	}
}

func TestPushWithRebaseRetry_LosesRace_RecoversAndPushes(t *testing.T) {
	origin, a := initBareAndClone(t)

	// Second clone simulates a teammate / bot pushing first.
	rootB := t.TempDir()
	b := filepath.Join(rootB, "b")
	mustGit(t, rootB, "clone", "-q", origin, b)
	mustGit(t, b, "config", "user.email", "test@example.com")
	mustGit(t, b, "config", "user.name", "Test")

	// `a` commits locally but doesn't push yet.
	if err := os.WriteFile(filepath.Join(a, "from-a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write from-a: %v", err)
	}
	mustGit(t, a, "add", ".")
	mustGit(t, a, "commit", "-q", "-m", "from a")

	// `b` commits and pushes first — origin/main is now ahead of `a`.
	if err := os.WriteFile(filepath.Join(b, "from-b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatalf("write from-b: %v", err)
	}
	mustGit(t, b, "add", ".")
	mustGit(t, b, "commit", "-q", "-m", "from b")
	mustGit(t, b, "push", "-q", "origin", "main")

	// `a`'s push should now fail non-fast-forward, the retry loop should
	// rebase onto origin/main and push again.
	if err := pushWithRebaseRetry(a); err != nil {
		t.Fatalf("pushWithRebaseRetry: %v", err)
	}

	// Both commits should be reachable from origin/main now.
	log := captureGitOut(t, b, "fetch", "-q", "origin", "main")
	_ = log
	out := captureGitOut(t, a, "log", "--pretty=%s", "origin/main")
	if !strings.Contains(out, "from a") || !strings.Contains(out, "from b") {
		t.Errorf("expected both commits on origin/main, got:\n%s", out)
	}
}

func TestTbdModeBanner_OnMain_ReportsPlain(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "main-branch")
	initTestRepo(t, repo)

	got := tbdModeBanner(repo)
	want := "plain TBD (on `main`)"
	if got != want {
		t.Errorf("banner on main = %q, want %q", got, want)
	}
}

func TestTbdModeBanner_OnFeatureBranch_ReportsScaledWithUpstream(t *testing.T) {
	origin, clone := initBareAndClone(t)
	_ = origin

	mustGit(t, clone, "checkout", "-q", "-b", "feature/x")
	if err := os.WriteFile(filepath.Join(clone, "x.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write x: %v", err)
	}
	mustGit(t, clone, "add", ".")
	mustGit(t, clone, "commit", "-q", "-m", "x")
	mustGit(t, clone, "push", "-q", "-u", "origin", "feature/x")

	got := tbdModeBanner(clone)
	want := "Scaled TBD (on `feature/x`, upstream `origin/feature/x`)"
	if got != want {
		t.Errorf("banner on feature branch = %q, want %q", got, want)
	}
}

func TestTbdModeBanner_OnFeatureBranchNoUpstream_ReportsScaledNoUpstream(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "feature-no-up")
	initTestRepo(t, repo)
	mustGit(t, repo, "checkout", "-q", "-b", "feature/y")

	got := tbdModeBanner(repo)
	want := "Scaled TBD (on `feature/y`)"
	if got != want {
		t.Errorf("banner on local-only branch = %q, want %q", got, want)
	}
}

func TestMainForcePushGuard_FastForwardOnMain_Allows(t *testing.T) {
	_, clone := initBareAndClone(t)
	// New local commit beyond origin/main — fast-forward push is fine.
	if err := os.WriteFile(filepath.Join(clone, "ahead.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write ahead: %v", err)
	}
	mustGit(t, clone, "add", ".")
	mustGit(t, clone, "commit", "-q", "-m", "ahead")

	if err := mainForcePushGuard(clone); err != nil {
		t.Errorf("guard fired on fast-forward main: %v", err)
	}
}

func TestMainForcePushGuard_DivergedMain_Refuses(t *testing.T) {
	origin, a := initBareAndClone(t)

	// Land a commit on origin via a second clone.
	rootB := t.TempDir()
	b := filepath.Join(rootB, "b")
	mustGit(t, rootB, "clone", "-q", origin, b)
	mustGit(t, b, "config", "user.email", "test@example.com")
	mustGit(t, b, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(b, "from-b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatalf("write from-b: %v", err)
	}
	mustGit(t, b, "add", ".")
	mustGit(t, b, "commit", "-q", "-m", "from b")
	mustGit(t, b, "push", "-q", "origin", "main")

	// `a` makes its own commit (now ahead 1) and then fetches so @{u}
	// advances to b's commit — local and remote diverge.
	if err := os.WriteFile(filepath.Join(a, "from-a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write from-a: %v", err)
	}
	mustGit(t, a, "add", ".")
	mustGit(t, a, "commit", "-q", "-m", "from a")
	mustGit(t, a, "fetch", "-q", "origin")

	err := mainForcePushGuard(a)
	if err == nil {
		t.Fatalf("expected guard to refuse divergent main, got nil")
	}
	if !strings.Contains(err.Error(), "diverged") {
		t.Errorf("guard error missing 'diverged': %v", err)
	}
}

func TestMainForcePushGuard_OnFeatureBranch_NoOp(t *testing.T) {
	origin, clone := initBareAndClone(t)
	_ = origin

	mustGit(t, clone, "checkout", "-q", "-b", "feature/x")
	// Even with a rewrite, a non-main branch is the guard's no-business case.
	if err := os.WriteFile(filepath.Join(clone, "x.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write x: %v", err)
	}
	mustGit(t, clone, "add", ".")
	mustGit(t, clone, "commit", "-q", "-m", "x")

	if err := mainForcePushGuard(clone); err != nil {
		t.Errorf("guard fired on non-main branch: %v", err)
	}
}

func TestMainLintRef_PrefersOriginMain(t *testing.T) {
	_, clone := initBareAndClone(t)
	if got := mainLintRef(clone); got != "origin/main" {
		t.Errorf("ref = %q, want origin/main", got)
	}
}

func TestMainLintRef_FallsBackToLocalMain(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "local-only")
	initTestRepo(t, repo)
	if got := mainLintRef(repo); got != "main" {
		t.Errorf("ref = %q, want main", got)
	}
}

func TestMainLintRef_ReturnsEmptyWhenNeither(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "no-main")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustGit(t, repo, "init", "-q", "-b", "trunk")
	mustGit(t, repo, "config", "user.email", "test@example.com")
	mustGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	mustGit(t, repo, "add", ".")
	mustGit(t, repo, "commit", "-q", "-m", "seed")
	if got := mainLintRef(repo); got != "" {
		t.Errorf("ref = %q, want empty (neither origin/main nor main present)", got)
	}
}

func TestLintHistoryOneRepo_LinearHistory_ReturnsNoMerges(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "linear")
	initTestRepo(t, repo)
	for i := 0; i < 3; i++ {
		path := filepath.Join(repo, fmt.Sprintf("f%d.txt", i))
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write f%d: %v", i, err)
		}
		mustGit(t, repo, "add", ".")
		mustGit(t, repo, "commit", "-q", "-m", fmt.Sprintf("c%d", i))
	}

	merges, err := lintHistoryOneRepo(repo, "main", 100)
	if err != nil {
		t.Fatalf("lintHistoryOneRepo: %v", err)
	}
	if len(merges) != 0 {
		t.Errorf("expected no merges on linear history, got %v", merges)
	}
}

func TestLintHistoryOneRepo_MergeCommitOnMain_FlagsIt(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "with-merge")
	initTestRepo(t, repo)

	mustGit(t, repo, "checkout", "-q", "-b", "feature")
	if err := os.WriteFile(filepath.Join(repo, "feat.txt"), []byte("f"), 0o644); err != nil {
		t.Fatalf("write feat: %v", err)
	}
	mustGit(t, repo, "add", ".")
	mustGit(t, repo, "commit", "-q", "-m", "feat commit")

	mustGit(t, repo, "checkout", "-q", "main")
	if err := os.WriteFile(filepath.Join(repo, "mainfile.txt"), []byte("m"), 0o644); err != nil {
		t.Fatalf("write mainfile: %v", err)
	}
	mustGit(t, repo, "add", ".")
	mustGit(t, repo, "commit", "-q", "-m", "main commit")
	mustGit(t, repo, "merge", "--no-ff", "-m", "merge feature", "feature")

	merges, err := lintHistoryOneRepo(repo, "main", 100)
	if err != nil {
		t.Fatalf("lintHistoryOneRepo: %v", err)
	}
	if len(merges) != 1 {
		t.Fatalf("expected 1 merge commit, got %d: %v", len(merges), merges)
	}
	if !strings.Contains(merges[0], "merge feature") {
		t.Errorf("merge line missing expected subject: %q", merges[0])
	}
}

func TestStaleBranchesOneRepo_MainOnly_ReturnsEmpty(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "main-only")
	initTestRepo(t, repo)
	stale, err := staleBranchesOneRepo(repo, time.Now())
	if err != nil {
		t.Fatalf("staleBranchesOneRepo: %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("expected no stale branches, got %v", stale)
	}
}

func TestStaleBranchesOneRepo_FreshBranch_NotFlagged(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "fresh")
	initTestRepo(t, repo)
	mustGit(t, repo, "checkout", "-q", "-b", "feature/fresh")
	if err := os.WriteFile(filepath.Join(repo, "fresh.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write fresh: %v", err)
	}
	mustGit(t, repo, "add", ".")
	mustGit(t, repo, "commit", "-q", "-m", "fresh commit")

	stale, err := staleBranchesOneRepo(repo, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("staleBranchesOneRepo: %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("fresh branch flagged as stale: %v", stale)
	}
}

func TestStaleBranchesOneRepo_OldBranch_Flagged(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "stale")
	initTestRepo(t, repo)
	mustGit(t, repo, "checkout", "-q", "-b", "feature/old")
	// Force tip's committer-date 72h in the past via env-overrides on commit.
	oldUnix := fmt.Sprintf("%d -0000", time.Now().Add(-72*time.Hour).Unix())
	cmd := exec.Command("git", "-C", repo, "commit", "-q", "--allow-empty", "-m", "old work")
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+oldUnix,
		"GIT_COMMITTER_DATE="+oldUnix,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit with old date: %v\n%s", err, out)
	}

	stale, err := staleBranchesOneRepo(repo, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("staleBranchesOneRepo: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale branch, got %d: %v", len(stale), stale)
	}
	if stale[0].Name != "feature/old" {
		t.Errorf("stale branch name = %q, want feature/old", stale[0].Name)
	}
}

func TestStaleBranchesOneRepo_SortsOldestFirst(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "multi-stale")
	initTestRepo(t, repo)

	makeStaleBranch := func(name string, hoursAgo int) {
		t.Helper()
		mustGit(t, repo, "checkout", "-q", "main")
		mustGit(t, repo, "checkout", "-q", "-b", name)
		oldUnix := fmt.Sprintf("%d -0000", time.Now().Add(-time.Duration(hoursAgo)*time.Hour).Unix())
		cmd := exec.Command("git", "-C", repo, "commit", "-q", "--allow-empty", "-m", name)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_DATE="+oldUnix,
			"GIT_COMMITTER_DATE="+oldUnix,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit on %s: %v\n%s", name, err, out)
		}
	}
	makeStaleBranch("feature/younger", 48)
	makeStaleBranch("feature/older", 200)

	stale, err := staleBranchesOneRepo(repo, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("staleBranchesOneRepo: %v", err)
	}
	if len(stale) != 2 {
		t.Fatalf("expected 2 stale branches, got %v", stale)
	}
	if stale[0].Name != "feature/older" || stale[1].Name != "feature/younger" {
		t.Errorf("not sorted oldest-first: got %v, %v", stale[0].Name, stale[1].Name)
	}
}

func TestFormatBranchAge(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{30 * time.Minute, "<1h"},
		{time.Hour, "1h"},
		{2 * time.Hour, "2h"},
		{23 * time.Hour, "23h"},
		{24 * time.Hour, "1d"},
		{25 * time.Hour, "1d 1h"},
		{72 * time.Hour, "3d"},
		{77 * time.Hour, "3d 5h"},
	}
	for _, c := range cases {
		if got := formatBranchAge(c.in); got != c.want {
			t.Errorf("formatBranchAge(%v) = %q, want %q", c.in, got, c.want)
		}
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
