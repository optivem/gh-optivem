package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// withIsolatedGlobalConfig points GIT_CONFIG_GLOBAL at a fresh empty file
// for the test, so doctor's `git config --global ...` calls touch a
// throwaway file instead of the developer's real ~/.gitconfig. Also clears
// GIT_CONFIG_SYSTEM so a stray system-level value can't satisfy a check.
func withIsolatedGlobalConfig(t *testing.T) string {
	t.Helper()
	gc := filepath.Join(t.TempDir(), "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", gc)
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(t.TempDir(), "no-system-config"))
	return gc
}

func TestRunDoctor_AllUnset_ReportsAndErrors(t *testing.T) {
	withIsolatedGlobalConfig(t)

	err := runDoctor(doctorOptions{})
	if err == nil {
		t.Fatalf("expected error when all required keys unset")
	}
	if !strings.Contains(err.Error(), "3 required git config setting(s) wrong") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunDoctor_Fix_SetsAllRequiredKeys(t *testing.T) {
	gc := withIsolatedGlobalConfig(t)

	if err := runDoctor(doctorOptions{Fix: true}); err != nil {
		t.Fatalf("runDoctor --fix returned error: %v", err)
	}

	for _, kv := range requiredGitConfig {
		got, err := exec.Command("git", "config", "--file", gc, "--get", kv.Key).Output()
		if err != nil {
			t.Errorf("key %s not set after --fix: %v", kv.Key, err)
			continue
		}
		if strings.TrimSpace(string(got)) != kv.Want {
			t.Errorf("key %s = %q, want %q", kv.Key, strings.TrimSpace(string(got)), kv.Want)
		}
	}

	// Re-run without --fix; everything should pass now.
	if err := runDoctor(doctorOptions{}); err != nil {
		t.Errorf("clean re-run returned error: %v", err)
	}
}

func TestRunDoctor_WrongValue_ReportedAsWrong(t *testing.T) {
	gc := withIsolatedGlobalConfig(t)
	if err := exec.Command("git", "config", "--file", gc, "pull.rebase", "false").Run(); err != nil {
		t.Fatalf("seed pull.rebase=false: %v", err)
	}

	err := runDoctor(doctorOptions{})
	if err == nil {
		t.Fatalf("expected error with pull.rebase=false")
	}
	if !strings.Contains(err.Error(), "wrong") {
		t.Errorf("error did not name wrong setting: %v", err)
	}
}
