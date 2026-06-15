package configinit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// TestEnsureExists_ExistingFileNil — when the file is present, EnsureExists
// returns nil without writing or prompting. Establishes the no-op case
// the three entry-point call sites depend on.
func TestEnsureExists_ExistingFileNil(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	if err := os.WriteFile(path, []byte("placeholder\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := ensureExists(path, false, nil, nil); err != nil {
		t.Errorf("EnsureExists on existing file: got %v, want nil", err)
	}
}

// TestEnsureExists_MissingNonTTY — non-TTY stdin reverts to the existing
// terse error verbatim. CI logs and unattended runs keep their stable
// "run config init first" wording.
func TestEnsureExists_MissingNonTTY(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	err := ensureExists(path, false, nil, nil)
	if err == nil {
		t.Fatal("want error for missing file on non-TTY, got nil")
	}
	want := projectconfig.MissingFileError(path).Error()
	if err.Error() != want {
		t.Errorf("error message mismatch:\n got:  %q\n want: %q", err.Error(), want)
	}
}

// TestEnsureExistsOrBuild_ExistingFileLoads — when the file is present,
// EnsureExistsOrBuild loads it from disk and returns the absolute path
// as sourcePath. Mirrors the pre-existing-file branch of
// loadProjectConfigForInit's default-path arm (operator-authored input
// at CWD/gh-optivem.yaml is never silently relocated).
func TestEnsureExistsOrBuild_ExistingFileLoads(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	seed := &projectconfig.Config{
		Project: projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
	}
	if err := projectconfig.WriteToPath(path, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	pc, src, err := ensureExistsOrBuild(path, false, nil, nil)
	if err != nil {
		t.Fatalf("EnsureExistsOrBuild on existing file: %v", err)
	}
	if pc == nil || pc.Project.URL != seed.Project.URL {
		t.Errorf("loaded config mismatch: %+v", pc)
	}
	if src != path {
		t.Errorf("sourcePath: got %q, want %q", src, path)
	}
}

// TestEnsureExistsOrBuild_MissingNonTTY — non-TTY stdin reverts to the
// existing terse error verbatim, and no file is written.
func TestEnsureExistsOrBuild_MissingNonTTY(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	pc, src, err := ensureExistsOrBuild(path, false, nil, nil)
	if err == nil {
		t.Fatal("want error for missing file on non-TTY, got nil")
	}
	if pc != nil {
		t.Errorf("want nil pc, got %+v", pc)
	}
	if src != "" {
		t.Errorf("sourcePath: want empty, got %q", src)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("file should not have been written, stat err: %v", statErr)
	}
	want := projectconfig.MissingFileError(path).Error()
	if err.Error() != want {
		t.Errorf("error message mismatch:\n got:  %q\n want: %q", err.Error(), want)
	}
}

// TestEnsureExistsOrBuild_MissingTTYBuildsInMemory — when stdin is a
// TTY and the prompt completes, EnsureExistsOrBuild returns an in-memory
// *projectconfig.Config with sourcePath == "" and writes nothing to disk.
// This is the seam that lets the scaffold dir become the sole on-disk
// materialization for default-path init runs.
func TestEnsureExistsOrBuild_MissingTTYBuildsInMemory(t *testing.T) {
	stubExistenceChecks(t, nil, nil)
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	in := script(monolithAnswers())
	var out bytes.Buffer
	pc, src, err := ensureExistsOrBuild(path, true, in, &out)
	if err != nil {
		t.Fatalf("EnsureExistsOrBuild: %v", err)
	}
	if pc == nil {
		t.Fatal("want non-nil pc")
	}
	if src != "" {
		t.Errorf("sourcePath: want empty (in-memory case), got %q", src)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("YAML file should not have been written at %s; stat err: %v", path, statErr)
	}
	// No .gitignore side-effect either — that belongs to operator-chosen
	// on-disk paths (Run / RunWithBanner), not the in-memory default-path flow.
	if _, statErr := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(statErr) {
		t.Errorf(".gitignore should not have been written in the in-memory path")
	}
	if pc.SystemName != "Page Turner" || pc.System.Architecture != "monolith" {
		t.Errorf("built pc mismatch: %+v", pc)
	}
	if !strings.Contains(out.String(), "creating one in memory") {
		t.Errorf("output should mention in-memory creation, got:\n%s", out.String())
	}
}

// TestEnsureExists_MissingTTYValidPrompt — when stdin is a TTY and the
// prompt completes successfully, EnsureExists writes the YAML (with the
// review-banner prepended) and runs the .gitignore side-effect that
// comes with Run.
func TestEnsureExists_MissingTTYValidPrompt(t *testing.T) {
	stubExistenceChecks(t, nil, nil)
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	in := script(monolithAnswers())
	var out bytes.Buffer
	if err := ensureExists(path, true, in, &out); err != nil {
		t.Fatalf("EnsureExists: %v", err)
	}
	yamlBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("YAML not written at %s: %v", path, err)
	}
	if !strings.HasPrefix(string(yamlBytes), "# gh-optivem.yaml") {
		t.Errorf("generated YAML should start with the review banner, got first line: %.80s", string(yamlBytes))
	}
	if !strings.Contains(string(yamlBytes), "gh optivem config validate") {
		t.Errorf("banner should mention the validate command, got:\n%s", yamlBytes)
	}
	// Run's gitignore side-effect — verifies the prompt path went all the
	// way through Run, not just Prompt + WriteOptivemYAMLToFilePathWithBanner.
	giData, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(giData), ".gh-optivem/") {
		t.Errorf(".gitignore should contain .gh-optivem/, got:\n%s", giData)
	}
	if !strings.Contains(out.String(), "creating one interactively") {
		t.Errorf("output should include the recovery banner, got:\n%s", out.String())
	}
}
