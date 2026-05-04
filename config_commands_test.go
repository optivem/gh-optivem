package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// runConfigInit/runConfigValidate are covered as a pair: round-tripping
// what `config init` writes through `config validate` is the contract
// users care about (write a fresh YAML, hand-edit, re-validate).

func TestRunConfigInit_MonolithRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := &config.RawFlags{
		Owner:        "acme",
		Repo:         "page-turner",
		Arch:         "monolith",
		RepoStrategy: "monorepo",
		Lang:         "java",
		ProjectURL:   "https://github.com/orgs/acme/projects/1",
	}
	path, err := runConfigInit(f, dir, false)
	if err != nil {
		t.Fatalf("runConfigInit: %v", err)
	}
	want := filepath.Join(dir, projectconfig.Path)
	if path != want {
		t.Errorf("path: got %q, want %q", path, want)
	}
	cfg, err := projectconfig.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load returned nil — file should exist")
	}
	if cfg.Project.URL != f.ProjectURL {
		t.Errorf("project.url: got %q, want %q", cfg.Project.URL, f.ProjectURL)
	}
	if cfg.Project.RepoStrategy != projectconfig.RepoStrategyMonoRepo {
		t.Errorf("project.repo_strategy: got %q, want %q", cfg.Project.RepoStrategy, projectconfig.RepoStrategyMonoRepo)
	}
	if cfg.Scope.Architecture != "monolith" || cfg.Scope.SystemLang != "java" || cfg.Scope.TestLang != "java" {
		t.Errorf("scope mismatch: got %+v", cfg.Scope)
	}
}

func TestRunConfigInit_MultitierMultirepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := &config.RawFlags{
		Owner:        "acme",
		Repo:         "page-turner",
		Arch:         "multitier",
		RepoStrategy: "multirepo",
		BackendLang:  "dotnet",
		FrontendLang: "react",
		TestLang:     "typescript",
	}
	if _, err := runConfigInit(f, dir, false); err != nil {
		t.Fatalf("runConfigInit: %v", err)
	}
	cfg, err := projectconfig.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Project.RepoStrategy != projectconfig.RepoStrategyMultiRepo {
		t.Errorf("project.repo_strategy: got %q", cfg.Project.RepoStrategy)
	}
	wantRepos := []string{"acme/page-turner-backend", "acme/page-turner-frontend"}
	if len(cfg.Project.Repos) != 2 || cfg.Project.Repos[0] != wantRepos[0] || cfg.Project.Repos[1] != wantRepos[1] {
		t.Errorf("project.repos: got %v, want %v", cfg.Project.Repos, wantRepos)
	}
	if cfg.Scope.SystemLang != "dotnet" {
		t.Errorf("scope.system_lang: got %q, want backend lang %q", cfg.Scope.SystemLang, "dotnet")
	}
	if cfg.Scope.TestLang != "typescript" {
		t.Errorf("scope.test_lang: got %q", cfg.Scope.TestLang)
	}
}

func TestRunConfigInit_RefusesOverwrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := &config.RawFlags{
		Owner:        "acme",
		Repo:         "page-turner",
		Arch:         "monolith",
		RepoStrategy: "monorepo",
		Lang:         "java",
	}
	if _, err := runConfigInit(f, dir, false); err != nil {
		t.Fatalf("first init: %v", err)
	}
	// Second invocation without --force should refuse.
	_, err := runConfigInit(f, dir, false)
	if err == nil {
		t.Fatal("second init without --force: want error, got nil")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should hint at --force, got: %v", err)
	}
	// With --force it should succeed.
	if _, err := runConfigInit(f, dir, true); err != nil {
		t.Fatalf("init with --force: %v", err)
	}
}

func TestRunConfigInit_RejectsBadFlags(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		f    *config.RawFlags
		want string
	}{
		{
			"missing required flag",
			&config.RawFlags{Owner: "acme"},
			"required flags",
		},
		{
			"bad arch",
			&config.RawFlags{Owner: "acme", Repo: "sky-travel", Arch: "bogus", RepoStrategy: "monorepo", Lang: "java"},
			"--arch",
		},
		{
			"bad repo-strategy",
			&config.RawFlags{Owner: "acme", Repo: "sky-travel", Arch: "monolith", RepoStrategy: "bogus", Lang: "java"},
			"--repo-strategy",
		},
		{
			"monolith missing lang",
			&config.RawFlags{Owner: "acme", Repo: "sky-travel", Arch: "monolith", RepoStrategy: "monorepo"},
			"--monolith-lang",
		},
		{
			"multitier missing backend lang",
			&config.RawFlags{Owner: "acme", Repo: "sky-travel", Arch: "multitier", RepoStrategy: "multirepo", FrontendLang: "react"},
			"--backend-lang",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			_, err := runConfigInit(tc.f, dir, false)
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error should mention %q, got: %v", tc.want, err)
			}
		})
	}
}

func TestRunConfigValidate_Missing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := runConfigValidate(dir)
	if err == nil {
		t.Fatal("want error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "no gh-optivem.yaml") {
		t.Errorf("error should mention missing file, got: %v", err)
	}
	if !strings.Contains(err.Error(), "config init") {
		t.Errorf("error should hint at `config init`, got: %v", err)
	}
}

func TestRunConfigValidate_Valid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := &config.RawFlags{
		Owner: "acme", Repo: "sky-travel",
		Arch: "monolith", RepoStrategy: "monorepo", Lang: "java",
	}
	if _, err := runConfigInit(f, dir, false); err != nil {
		t.Fatalf("seed init: %v", err)
	}
	path, err := runConfigValidate(dir)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if path != filepath.Join(dir, projectconfig.Path) {
		t.Errorf("path: got %q", path)
	}
}

func TestRunConfigValidate_InvalidContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, projectconfig.Path)
	bad := []byte("project:\n  repo_strategy: bogus\n")
	if err := os.WriteFile(yamlPath, bad, 0o644); err != nil {
		t.Fatalf("seed bad yaml: %v", err)
	}
	_, err := runConfigValidate(dir)
	if err == nil {
		t.Fatal("want error for invalid file, got nil")
	}
	if !strings.Contains(err.Error(), "repo_strategy") {
		t.Errorf("error should mention the invalid field, got: %v", err)
	}
}
