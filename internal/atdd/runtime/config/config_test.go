package config

import (
	"os"
	"path/filepath"
	"testing"
)

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
	if err := os.MkdirAll(filepath.Join(dir, "docs", "atdd"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `project:
  url: https://github.com/orgs/optivem/projects/20
`
	if err := os.WriteFile(filepath.Join(dir, Path), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
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
	if err := os.MkdirAll(filepath.Join(dir, "docs", "atdd"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, Path), nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
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
	if err := os.MkdirAll(filepath.Join(dir, "docs", "atdd"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, Path), []byte("project: [not, a, map\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
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
	if err := os.MkdirAll(filepath.Join(dir, "docs", "atdd"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `project:
  url: https://github.com/orgs/x/projects/1
  name: Acme Project
`
	if err := os.WriteFile(filepath.Join(dir, Path), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Project.Name != "Acme Project" {
		t.Fatalf("project name: got %q, want %q", cfg.Project.Name, "Acme Project")
	}
}

