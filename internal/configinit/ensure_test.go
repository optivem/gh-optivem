package configinit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/projectconfig"
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
	want := "no gh-optivem.yaml at " + path + "; run `gh optivem config init` first"
	if err.Error() != want {
		t.Errorf("error message mismatch:\n got:  %q\n want: %q", err.Error(), want)
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
