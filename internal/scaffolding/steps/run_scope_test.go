package steps

import (
	"errors"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/kernel/shell"
)

func resetRunScope(t *testing.T) {
	t.Helper()
	prevFn := latestRunIDFn
	t.Cleanup(func() {
		latestRunIDFn = prevFn
		runScopeMu.Lock()
		runBaselines = map[string]int64{}
		unchangedRepos = map[string]bool{}
		runScopeMu.Unlock()
	})
	runScopeMu.Lock()
	runBaselines = map[string]int64{}
	unchangedRepos = map[string]bool{}
	runScopeMu.Unlock()
}

func TestScopeToThisRun_PreExistingGetsBaseline(t *testing.T) {
	resetRunScope(t)
	latestRunIDFn = func(string) (int64, error) { return 500, nil }
	if err := recordRunBaseline("acme/shop"); err != nil {
		t.Fatal(err)
	}
	got := scopeToThisRun(shell.NewGitHub("acme/shop"))
	if got.MinRunID != 500 {
		t.Fatalf("MinRunID = %d, want 500", got.MinRunID)
	}
}

func TestScopeToThisRun_FreshRepoUnchanged(t *testing.T) {
	resetRunScope(t)
	gh := shell.NewGitHub("acme/fresh")
	if got := scopeToThisRun(gh); got != gh || got.MinRunID != 0 {
		t.Fatalf("repo without a baseline must be returned as is, got %+v", got)
	}
}

func TestRecordRunBaseline_FailsLoudOnUnreadableHistory(t *testing.T) {
	resetRunScope(t)
	latestRunIDFn = func(string) (int64, error) { return 0, errors.New("HTTP 403") }
	err := recordRunBaseline("acme/shop")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("want the underlying error, got %v", err)
	}
	if scopeToThisRun(shell.NewGitHub("acme/shop")).MinRunID != 0 {
		t.Fatal("a failed probe must not leave a baseline behind")
	}
}

func TestSkipUnchangedPushStage(t *testing.T) {
	resetRunScope(t)
	if skipUnchangedPushStage("acme/shop", "Commit stage") {
		t.Fatal("must not skip for a repo that was pushed")
	}
	recordUnchanged("acme/shop")
	if !skipUnchangedPushStage("acme/shop", "Commit stage") {
		t.Fatal("must skip push-triggered verification after a no-op push")
	}
	if skipUnchangedPushStage("acme/other", "Commit stage") {
		t.Fatal("skip must be per repository")
	}
}
