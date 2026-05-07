package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveSystemTestPaths_FlatLayout covers the production case: every
// scaffolded academy repo has docker/system.json and
// system-test/tests-latest.json at fixed paths after the template's
// path-flattening pass.
func TestResolveSystemTestPaths_FlatLayout(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docker", "system.json"), "{}")
	mustWriteFile(t, filepath.Join(root, "system-test", "tests-latest.json"), "{}")

	sys, tests, err := ResolveSystemTestPaths(root)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	wantSys := filepath.Join(root, "docker", "system.json")
	wantTests := filepath.Join(root, "system-test", "tests-latest.json")
	if sys != wantSys {
		t.Errorf("systemConfig: got %q, want %q", sys, wantSys)
	}
	if tests != wantTests {
		t.Errorf("testConfig: got %q, want %q", tests, wantTests)
	}
}

// TestResolveSystemTestPaths_TemplatedMonolith covers the shop template's
// pre-flatten layout for monolith repos. <lang> is discovered via globbing
// because the verify path has no language plumbing.
func TestResolveSystemTestPaths_TemplatedMonolith(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docker", "typescript", "monolith", "system.json"), "{}")
	mustWriteFile(t, filepath.Join(root, "system-test", "typescript", "tests-latest.json"), "{}")

	sys, tests, err := ResolveSystemTestPaths(root)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(sys), "docker/typescript/monolith/system.json") {
		t.Errorf("systemConfig: got %q, want suffix docker/typescript/monolith/system.json", sys)
	}
	if !strings.HasSuffix(filepath.ToSlash(tests), "system-test/typescript/tests-latest.json") {
		t.Errorf("testConfig: got %q, want suffix system-test/typescript/tests-latest.json", tests)
	}
}

// TestResolveSystemTestPaths_TemplatedMultitier mirrors the monolith case
// for multitier — the probe order tries monolith first then multitier.
func TestResolveSystemTestPaths_TemplatedMultitier(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docker", "java", "multitier", "system.json"), "{}")
	mustWriteFile(t, filepath.Join(root, "system-test", "java", "tests-latest.json"), "{}")

	sys, tests, err := ResolveSystemTestPaths(root)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(sys), "docker/java/multitier/system.json") {
		t.Errorf("systemConfig: got %q, want suffix docker/java/multitier/system.json", sys)
	}
	if !strings.HasSuffix(filepath.ToSlash(tests), "system-test/java/tests-latest.json") {
		t.Errorf("testConfig: got %q, want suffix system-test/java/tests-latest.json", tests)
	}
}

// TestResolveSystemTestPaths_FlatTakesPrecedence guards against a regression
// where a stray template file in a flat repo would shift resolution to
// templated probe and pick the wrong system.json. Production repos are flat,
// so flat must always win when present.
func TestResolveSystemTestPaths_FlatTakesPrecedence(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docker", "system.json"), `{"flat":true}`)
	mustWriteFile(t, filepath.Join(root, "system-test", "tests-latest.json"), "{}")
	// Stray templated files that should NOT be picked.
	mustWriteFile(t, filepath.Join(root, "docker", "typescript", "monolith", "system.json"), `{"templated":true}`)
	mustWriteFile(t, filepath.Join(root, "system-test", "typescript", "tests-latest.json"), "{}")

	sys, _, err := ResolveSystemTestPaths(root)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if filepath.ToSlash(sys) != filepath.ToSlash(filepath.Join(root, "docker", "system.json")) {
		t.Errorf("flat layout should win; got %q", sys)
	}
}

// TestResolveSystemTestPaths_NeitherLayout exercises the error path. The
// caller (verify action) is expected to surface this as an infra-class halt
// rather than letting the runner see broken defaults.
func TestResolveSystemTestPaths_NeitherLayout(t *testing.T) {
	root := t.TempDir()
	if _, _, err := ResolveSystemTestPaths(root); err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
