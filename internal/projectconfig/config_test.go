package projectconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, Path), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestLoad_MissingFileReturnsNil(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config for missing file, got %+v", cfg)
	}
}

func TestLoad_ParsesProjectURL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, `project:
  url: https://github.com/orgs/optivem/projects/20
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Project.URL != "https://github.com/orgs/optivem/projects/20" {
		t.Fatalf("project URL: got %q", cfg.Project.URL)
	}
}

func TestLoad_EmptyFileIsValid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, "")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("empty file should yield non-nil zero-value config")
	}
	if cfg.Project.URL != "" {
		t.Fatalf("expected empty project URL, got %q", cfg.Project.URL)
	}
}

func TestLoad_MalformedYAMLErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, "project: [not, a, map\n")
	if _, err := Load(dir); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestLoad_EmptyRepoPathErrors(t *testing.T) {
	t.Parallel()
	if _, err := Load(""); err == nil {
		t.Fatal("expected error for empty repoPath, got nil")
	}
}

func TestLoad_ProjectName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, `project:
  url: https://github.com/orgs/x/projects/1
  name: Acme Project
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Project.Name != "Acme Project" {
		t.Fatalf("project name: got %q, want %q", cfg.Project.Name, "Acme Project")
	}
}

func TestLoad_RepoStrategyAndRepos(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, `project:
  repo_strategy: multi-repo
  repos:
    - frontend
    - backend
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Project.RepoStrategy != "multi-repo" {
		t.Fatalf("repo_strategy: got %q", cfg.Project.RepoStrategy)
	}
	if got, want := cfg.Project.Repos, []string{"frontend", "backend"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("repos: got %v, want %v", got, want)
	}
}

func TestLoad_Scope(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, `scope:
  architecture: monolith
  system_lang: java
  test_lang: typescript
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Scope.Architecture != "monolith" || cfg.Scope.SystemLang != "java" || cfg.Scope.TestLang != "typescript" {
		t.Fatalf("scope: got %+v", cfg.Scope)
	}
}

func TestValidate_MultiRepoEmptyReposErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, `project:
  repo_strategy: multi-repo
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected validation error for multi-repo with empty repos, got nil")
	}
	if !strings.Contains(err.Error(), "non-empty project.repos") {
		t.Fatalf("error message should mention required repos list: %v", err)
	}
}

func TestValidate_MonoRepoMultipleReposErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, `project:
  repo_strategy: mono-repo
  repos:
    - foo
    - bar
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected validation error for mono-repo with multiple repos, got nil")
	}
	if !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("error message should mention incompatibility: %v", err)
	}
}

func TestValidate_MonoRepoSingleEntryOK(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, `project:
  repo_strategy: mono-repo
  repos:
    - shop
`)
	if _, err := Load(dir); err != nil {
		t.Fatalf("mono-repo with one repo should validate, got: %v", err)
	}
}

func TestValidate_UnknownRepoStrategyErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, `project:
  repo_strategy: poly-repo
`)
	if _, err := Load(dir); err == nil {
		t.Fatal("expected error for unknown repo_strategy, got nil")
	}
}

func TestValidate_UnknownArchitectureErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, `scope:
  architecture: hexagonal
`)
	if _, err := Load(dir); err == nil {
		t.Fatal("expected error for unknown architecture, got nil")
	}
}

func TestValidate_UnknownLangErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, `scope:
  system_lang: rust
`)
	if _, err := Load(dir); err == nil {
		t.Fatal("expected error for unknown system_lang, got nil")
	}
}

func TestValidate_AbsenceIsOK(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("zero-value config should validate, got: %v", err)
	}
}

func TestValidate_NilReceiverIsOK(t *testing.T) {
	t.Parallel()
	var cfg *Config
	if err := cfg.Validate(); err != nil {
		t.Fatalf("nil receiver should validate, got: %v", err)
	}
}

func TestLoadFromPath_MissingFileErrors(t *testing.T) {
	t.Parallel()
	if _, err := LoadFromPath(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error for missing file via LoadFromPath, got nil")
	}
}

func TestLoadFromPath_EmptyPathErrors(t *testing.T) {
	t.Parallel()
	if _, err := LoadFromPath(""); err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}
