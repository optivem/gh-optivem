package steps

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/optivem/gh-optivem/internal/kernel/log"
)

func TestMatchGlobPath(t *testing.T) {
	cases := []struct {
		pattern string
		file    string
		want    bool
	}{
		// No ** — delegates to path.Match
		{"*.go", "main.go", true},
		{"*.go", "sub/main.go", false},
		{"system/main.go", "system/main.go", true},
		// Trailing **
		{"system/**", "system/main.go", true},
		{"system/**", "system/sub/main.go", true},
		{"system/**", "other/main.go", false},
		{"backend/**", "frontend/src/app.ts", false},
		// Leading **
		{"**/*.go", "main.go", true},
		{"**/*.go", "sub/main.go", true},
		{"**/*.go", "sub/deep/main.go", true},
		{"**/*.go", "main.ts", false},
		// Empty file path — must never match
		{"system/**", "", false},
		{"*.go", "", false},
	}
	for _, tc := range cases {
		got := matchGlobPath(tc.pattern, tc.file)
		if got != tc.want {
			t.Errorf("matchGlobPath(%q, %q) = %v, want %v", tc.pattern, tc.file, got, tc.want)
		}
	}
}

func TestCheckPushPathsFilter_MatchingFilter(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}
	repoDir := t.TempDir()
	mustInitGitRepo(t, repoDir)

	sysDir := filepath.Join(repoDir, "system")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	mustGitAdd(t, repoDir, ".")
	mustGitCommit(t, repoDir, "init")

	wfDir := filepath.Join(repoDir, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0755); err != nil {
		t.Fatal(err)
	}
	wfPath := filepath.Join(wfDir, "commit-stage.yml")
	if err := os.WriteFile(wfPath, []byte("on:\n  push:\n    paths:\n      - 'system/**'\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should not panic — filter matches system/main.go
	checkPushPathsFilter(wfPath, repoDir)
}

func TestCheckPushPathsFilter_NonMatchingFilter(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}
	repoDir := t.TempDir()
	mustInitGitRepo(t, repoDir)

	sysDir := filepath.Join(repoDir, "system")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	mustGitAdd(t, repoDir, ".")
	mustGitCommit(t, repoDir, "init")

	wfDir := filepath.Join(repoDir, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0755); err != nil {
		t.Fatal(err)
	}
	wfPath := filepath.Join(wfDir, "commit-stage.yml")
	// Filter points at backend/ — no tracked files there
	if err := os.WriteFile(wfPath, []byte("on:\n  push:\n    paths:\n      - 'backend/**'\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var caught *log.StepError
	func() {
		defer func() {
			if r := recover(); r != nil {
				var ok bool
				caught, ok = r.(*log.StepError)
				if !ok {
					t.Fatalf("panic value is %T, want *log.StepError", r)
				}
			}
		}()
		checkPushPathsFilter(wfPath, repoDir)
	}()
	if caught == nil {
		t.Fatal("expected panic (StepError) for filter with no matching tracked file, got none")
	}
}

// TestCheckPushPathsFilter_UnstagedFiles mirrors the real scaffold pipeline
// ordering: the check runs in phaseApplyTemplate, before CommitAndPush stages
// anything, so matching files exist on disk but are neither tracked nor staged.
// The check must still pass — it evaluates against the set the upcoming commit
// will include (tracked + untracked-not-ignored), not the git index.
func TestCheckPushPathsFilter_UnstagedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}
	repoDir := t.TempDir()
	mustInitGitRepo(t, repoDir)

	sysDir := filepath.Join(repoDir, "system")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	wfDir := filepath.Join(repoDir, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0755); err != nil {
		t.Fatal(err)
	}
	wfPath := filepath.Join(wfDir, "commit-stage.yml")
	if err := os.WriteFile(wfPath, []byte("on:\n  push:\n    paths:\n      - 'system/**'\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Deliberately do NOT `git add` — files are unstaged, as they are when the
	// check actually runs. The pre-fix `git ls-files` code would have fataled here.
	checkPushPathsFilter(wfPath, repoDir)
}

func mustInitGitRepo(t *testing.T, dir string) {
	t.Helper()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.email", "test@test.com")
	mustGit(t, dir, "config", "user.name", "Test")
}

func mustGitAdd(t *testing.T, dir, path string) {
	t.Helper()
	mustGit(t, dir, "add", path)
}

func mustGitCommit(t *testing.T, dir, msg string) {
	t.Helper()
	mustGit(t, dir, "commit", "-m", msg)
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
