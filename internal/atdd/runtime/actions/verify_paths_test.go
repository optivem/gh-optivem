package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveSystemTestPaths_FlatLayout covers the legacy production case:
// pre-migration academy repos have docker/systems.json and
// system-test/tests-latest.json at fixed paths after the template's
// path-flattening pass. The `.json` extension still resolves via the
// extension fallback after YAML and YML probes miss.
func TestResolveSystemTestPaths_FlatLayout(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docker", "systems.json"), "{}")
	mustWriteFile(t, filepath.Join(root, "system-test", "tests-latest.json"), "{}")

	sys, tests, err := ResolveSystemTestPaths(root)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	wantSys := filepath.Join(root, "docker", "systems.json")
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
	mustWriteFile(t, filepath.Join(root, "docker", "typescript", "monolith", "systems.json"), "{}")
	mustWriteFile(t, filepath.Join(root, "system-test", "typescript", "tests-latest.json"), "{}")

	sys, tests, err := ResolveSystemTestPaths(root)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(sys), "docker/typescript/monolith/systems.json") {
		t.Errorf("systemConfig: got %q, want suffix docker/typescript/monolith/systems.json", sys)
	}
	if !strings.HasSuffix(filepath.ToSlash(tests), "system-test/typescript/tests-latest.json") {
		t.Errorf("testConfig: got %q, want suffix system-test/typescript/tests-latest.json", tests)
	}
}

// TestResolveSystemTestPaths_TemplatedMultitier mirrors the monolith case
// for multitier — the probe order tries monolith first then multitier.
func TestResolveSystemTestPaths_TemplatedMultitier(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docker", "java", "multitier", "systems.json"), "{}")
	mustWriteFile(t, filepath.Join(root, "system-test", "java", "tests-latest.json"), "{}")

	sys, tests, err := ResolveSystemTestPaths(root)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(sys), "docker/java/multitier/systems.json") {
		t.Errorf("systemConfig: got %q, want suffix docker/java/multitier/systems.json", sys)
	}
	if !strings.HasSuffix(filepath.ToSlash(tests), "system-test/java/tests-latest.json") {
		t.Errorf("testConfig: got %q, want suffix system-test/java/tests-latest.json", tests)
	}
}

// TestResolveSystemTestPaths_FlatTakesPrecedence guards against a regression
// where a stray template file in a flat repo would shift resolution to
// templated probe and pick the wrong systems.json. Production repos are flat,
// so flat must always win when present.
func TestResolveSystemTestPaths_FlatTakesPrecedence(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docker", "systems.json"), `{"flat":true}`)
	mustWriteFile(t, filepath.Join(root, "system-test", "tests-latest.json"), "{}")
	// Stray templated files that should NOT be picked.
	mustWriteFile(t, filepath.Join(root, "docker", "typescript", "monolith", "systems.json"), `{"templated":true}`)
	mustWriteFile(t, filepath.Join(root, "system-test", "typescript", "tests-latest.json"), "{}")

	sys, _, err := ResolveSystemTestPaths(root)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if filepath.ToSlash(sys) != filepath.ToSlash(filepath.Join(root, "docker", "systems.json")) {
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

// TestResolveSystemTestPaths_FlatLayoutYAML covers the post-migration
// scaffolder output: docker/systems.yaml + system-test/tests-latest.yaml.
// `.yaml` is probed before `.json`, so a freshly scaffolded repo resolves
// to YAML.
func TestResolveSystemTestPaths_FlatLayoutYAML(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docker", "systems.yaml"), "")
	mustWriteFile(t, filepath.Join(root, "system-test", "tests-latest.yaml"), "")

	sys, tests, err := ResolveSystemTestPaths(root)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	wantSys := filepath.Join(root, "docker", "systems.yaml")
	wantTests := filepath.Join(root, "system-test", "tests-latest.yaml")
	if sys != wantSys {
		t.Errorf("systemConfig: got %q, want %q", sys, wantSys)
	}
	if tests != wantTests {
		t.Errorf("testConfig: got %q, want %q", tests, wantTests)
	}
}

// TestResolveSystemTestPaths_TemplatedYAML covers shop's pre-flatten layout
// once it migrates to YAML — docker/<lang>/<arch>/systems.yaml +
// system-test/<lang>/tests-latest.yaml.
func TestResolveSystemTestPaths_TemplatedYAML(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docker", "typescript", "monolith", "systems.yaml"), "")
	mustWriteFile(t, filepath.Join(root, "system-test", "typescript", "tests-latest.yaml"), "")

	sys, tests, err := ResolveSystemTestPaths(root)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(sys), "docker/typescript/monolith/systems.yaml") {
		t.Errorf("systemConfig: got %q, want suffix docker/typescript/monolith/systems.yaml", sys)
	}
	if !strings.HasSuffix(filepath.ToSlash(tests), "system-test/typescript/tests-latest.yaml") {
		t.Errorf("testConfig: got %q, want suffix system-test/typescript/tests-latest.yaml", tests)
	}
}

// TestResolveSystemTestPaths_YAMLBeatsJSON guards the probe order: when a
// repo has both extensions (e.g. mid-migration), `.yaml` wins so the runner
// reads the migrated file rather than the stale legacy one.
func TestResolveSystemTestPaths_YAMLBeatsJSON(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docker", "systems.yaml"), "")
	mustWriteFile(t, filepath.Join(root, "docker", "systems.json"), "{}")
	mustWriteFile(t, filepath.Join(root, "system-test", "tests-latest.yaml"), "")
	mustWriteFile(t, filepath.Join(root, "system-test", "tests-latest.json"), "{}")

	sys, tests, err := ResolveSystemTestPaths(root)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if filepath.Ext(sys) != ".yaml" {
		t.Errorf("systemConfig: got %q, want .yaml extension", sys)
	}
	if filepath.Ext(tests) != ".yaml" {
		t.Errorf("testConfig: got %q, want .yaml extension", tests)
	}
}

// TestResolveSystemTestPaths_YMLExtension exercises the `.yml` shorthand.
// `gopkg.in/yaml.v3` parses both; the probe should accept both.
func TestResolveSystemTestPaths_YMLExtension(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docker", "systems.yml"), "")
	mustWriteFile(t, filepath.Join(root, "system-test", "tests-latest.yml"), "")

	sys, tests, err := ResolveSystemTestPaths(root)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if filepath.Ext(sys) != ".yml" {
		t.Errorf("systemConfig: got %q, want .yml extension", sys)
	}
	if filepath.Ext(tests) != ".yml" {
		t.Errorf("testConfig: got %q, want .yml extension", tests)
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
