package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// withWorkdir chdirs into dir for the duration of t. Restores the prior CWD
// on cleanup. Used because runHooksInstall locates the hooks dir via
// `git rev-parse` against the process CWD.
func withWorkdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func TestHooksInstall_FreshRepo_WritesHookWithMarker(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "fresh")
	initTestRepo(t, repo)
	withWorkdir(t, repo)

	if err := runHooksInstall(); err != nil {
		t.Fatalf("runHooksInstall: %v", err)
	}

	hookPath := filepath.Join(repo, ".git", "hooks", "pre-push")
	body, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read installed hook: %v", err)
	}
	if !strings.Contains(string(body), prePushMarker) {
		t.Errorf("installed hook missing marker %q", prePushMarker)
	}
	if !strings.Contains(string(body), "refs/heads/main") {
		t.Errorf("installed hook does not reference refs/heads/main")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(hookPath)
		if err != nil {
			t.Fatalf("stat installed hook: %v", err)
		}
		if info.Mode().Perm()&0o100 == 0 {
			t.Errorf("installed hook not executable; mode = %v", info.Mode())
		}
	}
}

func TestHooksInstall_ReRun_OverwritesOwnHook(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "rerun")
	initTestRepo(t, repo)
	withWorkdir(t, repo)

	if err := runHooksInstall(); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if err := runHooksInstall(); err != nil {
		t.Fatalf("second install should be idempotent: %v", err)
	}
}

func TestHooksInstall_ForeignHookPresent_Refuses(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "foreign")
	initTestRepo(t, repo)
	withWorkdir(t, repo)

	hookPath := filepath.Join(repo, ".git", "hooks", "pre-push")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\necho hand-written\n"), 0o755); err != nil {
		t.Fatalf("seed foreign hook: %v", err)
	}

	err := runHooksInstall()
	if err == nil {
		t.Fatalf("expected refusal when foreign pre-push hook present")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Errorf("error did not mention refusal: %v", err)
	}

	// Foreign content must be left untouched.
	body, _ := os.ReadFile(hookPath)
	if !strings.Contains(string(body), "hand-written") {
		t.Errorf("foreign hook content was clobbered: %q", body)
	}
}
