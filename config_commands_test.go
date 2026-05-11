package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/configinit"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// configinit.Run/runConfigValidate are covered as a pair: round-tripping
// what `config init` writes through `config validate` is the contract
// users care about (write a fresh YAML, hand-edit, re-validate).

// monolithMonorepoFlags returns a RawFlags pre-populated with valid path
// flags matching shop's worktree layout — what the rehearsal script passes
// to `gh optivem config init`. Tests reuse this so the explicit-paths
// contract isn't restated at every call site.
func monolithMonorepoFlags() *config.RawFlags {
	return &config.RawFlags{
		Owner:          "acme",
		Repo:           "page-turner",
		SystemName:     "Page Turner",
		Arch:           "monolith",
		RepoStrategy:   "monorepo",
		Lang:           "java",
		SystemPath:     "system/monolith/java",
		SystemTestPath: "system-test/java",
		StubsPath:      "external-systems/external-stub",
		SimulatorsPath: "external-systems/external-real-sim",
		ProjectURL:     "https://github.com/orgs/acme/projects/1",
	}
}

func TestRunConfigInit_MonolithRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := monolithMonorepoFlags()
	f.ProjectURL = "https://github.com/orgs/acme/projects/1"
	path, err := configinit.Run(f, filepath.Join(dir, projectconfig.Path), false)
	if err != nil {
		t.Fatalf("configinit.Run: %v", err)
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
	if cfg.RepoStrategy != projectconfig.RepoStrategyMonoRepo {
		t.Errorf("repo_strategy: got %q, want %q", cfg.RepoStrategy, projectconfig.RepoStrategyMonoRepo)
	}
	if cfg.System.Architecture != "monolith" || cfg.System.Lang != "java" {
		t.Errorf("system mismatch: got %+v", cfg.System)
	}
	if cfg.SystemTest.Lang != "java" {
		t.Errorf("system_test.lang: got %q, want java", cfg.SystemTest.Lang)
	}
	if cfg.System.Path != "system/monolith/java" {
		t.Errorf("system.path: got %q (should round-trip the --system-path flag)", cfg.System.Path)
	}
	if cfg.System.Repo != "acme/page-turner" {
		t.Errorf("system.repo: got %q, want acme/page-turner", cfg.System.Repo)
	}
}

func TestRunConfigInit_MultitierMultirepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := &config.RawFlags{
		Owner:          "acme",
		Repo:           "page-turner",
		SystemName:     "Page Turner",
		Arch:           "multitier",
		RepoStrategy:   "multirepo",
		BackendLang:    "dotnet",
		FrontendLang:   "react",
		TestLang:       "typescript",
		BackendPath:    "system/multitier/backend-dotnet",
		FrontendPath:   "system/multitier/frontend-react",
		SystemTestPath: "system-test/typescript",
		StubsPath:      "external-systems/external-stub",
		SimulatorsPath: "external-systems/external-real-sim",
		ProjectURL:     "https://github.com/orgs/acme/projects/2",
	}
	if _, err := configinit.Run(f, filepath.Join(dir, projectconfig.Path), false); err != nil {
		t.Fatalf("configinit.Run: %v", err)
	}
	cfg, err := projectconfig.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RepoStrategy != projectconfig.RepoStrategyMultiRepo {
		t.Errorf("repo_strategy: got %q", cfg.RepoStrategy)
	}
	wantRepos := []string{"acme/page-turner-backend", "acme/page-turner-frontend"}
	gotRepos := cfg.Repos()
	if len(gotRepos) != 2 || gotRepos[0] != wantRepos[0] || gotRepos[1] != wantRepos[1] {
		t.Errorf("Repos(): got %v, want %v", gotRepos, wantRepos)
	}
	if cfg.System.Backend.Lang != "dotnet" {
		t.Errorf("system.backend.lang: got %q, want dotnet", cfg.System.Backend.Lang)
	}
	if cfg.System.Backend.Repo != "acme/page-turner-backend" {
		t.Errorf("system.backend.repo: got %q", cfg.System.Backend.Repo)
	}
	if cfg.System.Frontend.Lang != "typescript" {
		t.Errorf("system.frontend.lang: got %q, want typescript", cfg.System.Frontend.Lang)
	}
	if cfg.SystemTest.Lang != "typescript" {
		t.Errorf("system_test.lang: got %q", cfg.SystemTest.Lang)
	}
}

func TestRunConfigInit_RefusesOverwrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, projectconfig.Path)
	f := monolithMonorepoFlags()
	if _, err := configinit.Run(f, yamlPath, false); err != nil {
		t.Fatalf("first init: %v", err)
	}
	// Second invocation without --force should refuse.
	_, err := configinit.Run(f, yamlPath, false)
	if err == nil {
		t.Fatal("second init without --force: want error, got nil")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should hint at --force, got: %v", err)
	}
	// With --force it should succeed.
	if _, err := configinit.Run(f, yamlPath, true); err != nil {
		t.Fatalf("init with --force: %v", err)
	}
}

func TestRunConfigInit_RejectsBadFlags(t *testing.T) {
	t.Parallel()
	// withPaths returns a copy of monolithMonorepoFlags with the given
	// transform applied — used to isolate a single bad-flag scenario from
	// path-validation noise.
	withPaths := func(mutate func(*config.RawFlags)) *config.RawFlags {
		f := monolithMonorepoFlags()
		mutate(f)
		return f
	}
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
			withPaths(func(f *config.RawFlags) { f.Arch = "bogus"; f.Repo = "sky-travel" }),
			"--arch",
		},
		{
			"bad repo-strategy",
			withPaths(func(f *config.RawFlags) { f.RepoStrategy = "bogus"; f.Repo = "sky-travel" }),
			"--repo-strategy",
		},
		{
			"monolith missing lang",
			withPaths(func(f *config.RawFlags) { f.Lang = ""; f.Repo = "sky-travel" }),
			"--monolith-lang",
		},
		{
			"multitier missing backend lang",
			&config.RawFlags{
				Owner: "acme", Repo: "sky-travel", SystemName: "Sky Travel",
				Arch: "multitier", RepoStrategy: "multirepo", FrontendLang: "react",
				FrontendPath:   "frontend",
				SystemTestPath: "system-test",
				StubsPath:      "external-systems/external-stub",
				SimulatorsPath: "external-systems/external-real-sim",
			},
			"--backend-lang",
		},
		{
			"missing path flags",
			&config.RawFlags{
				Owner: "acme", Repo: "sky-travel", SystemName: "Sky Travel",
				Arch: "monolith", RepoStrategy: "monorepo", Lang: "java",
			},
			"--system-path",
		},
		{
			"system-path on multitier",
			&config.RawFlags{
				Owner: "acme", Repo: "sky-travel", SystemName: "Sky Travel",
				Arch: "multitier", RepoStrategy: "multirepo",
				BackendLang: "java", FrontendLang: "react",
				SystemPath:     "system",
				BackendPath:    "backend",
				FrontendPath:   "frontend",
				SystemTestPath: "system-test",
				StubsPath:      "external-systems/external-stub",
				SimulatorsPath: "external-systems/external-real-sim",
			},
			"--system-path is not valid for --arch multitier",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			_, err := configinit.Run(tc.f, filepath.Join(dir, projectconfig.Path), false)
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
	_, err := runConfigValidate(filepath.Join(dir, projectconfig.Path))
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
	yamlPath := filepath.Join(dir, projectconfig.Path)
	f := monolithMonorepoFlags()
	f.Repo = "sky-travel"
	if _, err := configinit.Run(f, yamlPath, false); err != nil {
		t.Fatalf("seed init: %v", err)
	}
	path, err := runConfigValidate(yamlPath)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if path != yamlPath {
		t.Errorf("path: got %q", path)
	}
}

func TestRunConfigValidate_InvalidContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, projectconfig.Path)
	// Top-level repo_strategy with a bogus value — exercises the new
	// schema's location for the field (not nested under `project:`).
	bad := []byte("repo_strategy: bogus\n")
	if err := os.WriteFile(yamlPath, bad, 0o644); err != nil {
		t.Fatalf("seed bad yaml: %v", err)
	}
	_, err := runConfigValidate(yamlPath)
	if err == nil {
		t.Fatal("want error for invalid file, got nil")
	}
	if !strings.Contains(err.Error(), "repo_strategy") {
		t.Errorf("error should mention the invalid field, got: %v", err)
	}
}

// TestRunConfigValidate_NonDefaultFilename verifies validate can target a
// file with a non-canonical name (mirroring shop's monolith × multitier
// matrix where multiple gh-optivem.*.yaml live in one repo).
func TestRunConfigValidate_NonDefaultFilename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "gh-optivem.shop-monolith.yaml")
	f := monolithMonorepoFlags()
	if _, err := configinit.Run(f, yamlPath, false); err != nil {
		t.Fatalf("seed init: %v", err)
	}
	path, err := runConfigValidate(yamlPath)
	if err != nil {
		t.Fatalf("validate non-default filename: %v", err)
	}
	if path != yamlPath {
		t.Errorf("path: got %q, want %q", path, yamlPath)
	}
}

// TestResolveConfigInitTarget_Precedence covers --config > --dir > cwd.
func TestResolveConfigInitTarget_Precedence(t *testing.T) {
	t.Parallel()
	t.Run("flag wins over dir", func(t *testing.T) {
		t.Parallel()
		got, err := configinit.ResolveTarget("./explicit.yaml", "./somedir")
		if err != nil {
			t.Fatalf("configinit.ResolveTarget: %v", err)
		}
		if got != "./explicit.yaml" {
			t.Errorf("got %q, want ./explicit.yaml (flag wins)", got)
		}
	})
	t.Run("dir falls back to canonical filename", func(t *testing.T) {
		t.Parallel()
		got, err := configinit.ResolveTarget("", "/tmp/somedir")
		if err != nil {
			t.Fatalf("configinit.ResolveTarget: %v", err)
		}
		want := filepath.Join("/tmp/somedir", projectconfig.Path)
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
