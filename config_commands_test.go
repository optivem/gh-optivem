package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/preflight"
	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/configinit"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// init replaces the GitHub-side existence checks with pass-everything
// stubs for the duration of the test binary. ValidateAndDeriveForYAML
// — the path every configinit.Run test below reaches — calls
// CheckOwnerExists + CheckProjectExists in production; without these
// stubs every test would shell out to `gh` and fail offline. Tests that
// want to exercise the failure path can re-override the Fn vars
// per-test.
func init() {
	config.CheckOwnerExistsFn = func(string) error { return nil }
	config.CheckProjectExistsFn = func(string) error { return nil }
}

// offlinePreflightOpts returns a preflight.Options factory that wires
// only workspace + cwd — every remote-check field is left nil so the
// test surface stays local-FS-only. Used by every runConfigPreflight
// test below; the cobra layer's real defaultPreflightOptions adds the
// GitHub / SonarCloud / board-URL wiring.
func offlinePreflightOpts(workspace, cwd string) func(*projectconfig.Config) (preflight.Options, error) {
	return func(*projectconfig.Config) (preflight.Options, error) {
		return preflight.Options{Workspace: workspace, Cwd: cwd}, nil
	}
}

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
		TestLang:       "java",
		SystemPath:     "system/monolith/java",
		SystemTestPath: "system-test/java",
		StubsPath:      "external-systems/stubs",
		SimulatorsPath: "external-systems/simulators",
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

// TestRunConfigInit_DefaultsFlatLayoutPaths pins the contract that
// `gh optivem config init` materializes the flat-scaffold path defaults
// into gh-optivem.yaml when the operator passes no path flags — the
// same layout `gh optivem init` itself produces. Covers both arches in
// one parametric pass.
func TestRunConfigInit_DefaultsFlatLayoutPaths(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name              string
		flags             *config.RawFlags
		wantSystemPath    string // empty on multitier
		wantBackendPath   string // empty on monolith
		wantFrontendPath  string // empty on monolith
		wantSystemTest    string
		wantStubsPath     string
		wantSimsPath      string
	}{
		{
			name: "monolith flat defaults",
			flags: &config.RawFlags{
				Owner: "acme", Repo: "page-turner", SystemName: "Page Turner",
				Arch: "monolith", RepoStrategy: "monorepo", Lang: "java",
				TestLang:   "java",
				ProjectURL: "https://github.com/orgs/acme/projects/1",
			},
			wantSystemPath: config.DefaultSystemPath,
			wantSystemTest: config.DefaultSystemTestPath,
			wantStubsPath:  config.DefaultStubsPath,
			wantSimsPath:   config.DefaultSimulatorsPath,
		},
		{
			name: "multitier flat defaults",
			flags: &config.RawFlags{
				Owner: "acme", Repo: "page-turner", SystemName: "Page Turner",
				Arch: "multitier", RepoStrategy: "multirepo",
				BackendLang: "java", FrontendLang: "typescript",
				TestLang:   "java",
				ProjectURL: "https://github.com/orgs/acme/projects/1",
			},
			wantBackendPath:  config.DefaultBackendPath,
			wantFrontendPath: config.DefaultFrontendPath,
			wantSystemTest:   config.DefaultSystemTestPath,
			wantStubsPath:    config.DefaultStubsPath,
			wantSimsPath:     config.DefaultSimulatorsPath,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if _, err := configinit.Run(tc.flags, filepath.Join(dir, projectconfig.Path), false); err != nil {
				t.Fatalf("configinit.Run: %v", err)
			}
			cfg, err := projectconfig.Load(dir)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if tc.wantSystemPath != "" && cfg.System.Path != tc.wantSystemPath {
				t.Errorf("system.path: got %q, want %q", cfg.System.Path, tc.wantSystemPath)
			}
			if tc.wantBackendPath != "" && cfg.System.Backend.Path != tc.wantBackendPath {
				t.Errorf("system.backend.path: got %q, want %q", cfg.System.Backend.Path, tc.wantBackendPath)
			}
			if tc.wantFrontendPath != "" && cfg.System.Frontend.Path != tc.wantFrontendPath {
				t.Errorf("system.frontend.path: got %q, want %q", cfg.System.Frontend.Path, tc.wantFrontendPath)
			}
			if cfg.SystemTest.Path != tc.wantSystemTest {
				t.Errorf("system_test.path: got %q, want %q", cfg.SystemTest.Path, tc.wantSystemTest)
			}
			if cfg.ExternalSystems.Stubs.Path != tc.wantStubsPath {
				t.Errorf("external_systems.stubs.path: got %q, want %q", cfg.ExternalSystems.Stubs.Path, tc.wantStubsPath)
			}
			if cfg.ExternalSystems.Simulators.Path != tc.wantSimsPath {
				t.Errorf("external_systems.simulators.path: got %q, want %q", cfg.ExternalSystems.Simulators.Path, tc.wantSimsPath)
			}
		})
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
		FrontendLang:   "typescript",
		TestLang:       "typescript",
		BackendPath:    "system/multitier/backend-dotnet",
		FrontendPath:   "system/multitier/frontend-react",
		SystemTestPath: "system-test/typescript",
		StubsPath:      "external-systems/stubs",
		SimulatorsPath: "external-systems/simulators",
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
				Arch: "multitier", RepoStrategy: "multirepo", FrontendLang: "typescript",
				FrontendPath:   "frontend",
				SystemTestPath: "system-test",
				StubsPath:      "external-systems/stubs",
				SimulatorsPath: "external-systems/simulators",
			},
			"--backend-lang",
		},
		{
			"system-path on multitier",
			&config.RawFlags{
				Owner: "acme", Repo: "sky-travel", SystemName: "Sky Travel",
				Arch: "multitier", RepoStrategy: "multirepo",
				BackendLang: "java", FrontendLang: "typescript", TestLang: "java",
				SystemPath:     "system",
				BackendPath:    "backend",
				FrontendPath:   "frontend",
				SystemTestPath: "system-test",
				StubsPath:      "external-systems/stubs",
				SimulatorsPath: "external-systems/simulators",
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

// TestRunConfigPreflight_Missing mirrors TestRunConfigValidate_Missing:
// preflight's first gate is the same EnsureExists chain validate uses, so a
// missing file surfaces the same hint pointing at `config init`.
func TestRunConfigPreflight_Missing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := runConfigPreflight(filepath.Join(dir, projectconfig.Path), offlinePreflightOpts("", dir))
	if err == nil {
		t.Fatal("want error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "no gh-optivem.yaml") {
		t.Errorf("error should mention missing file, got: %v", err)
	}
}

// TestRunConfigPreflight_AllPathsExist seeds a workspace whose layout matches
// every directory declared in gh-optivem.yaml — preflight must pass.
func TestRunConfigPreflight_AllPathsExist(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	repoDir := filepath.Join(workspace, "page-turner")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatalf("seed .git: %v", err)
	}
	for _, p := range []string{
		"system/monolith/java",
		"system-test/java",
		"external-systems/stubs",
		"external-systems/simulators",
	} {
		if err := os.MkdirAll(filepath.Join(repoDir, p), 0o755); err != nil {
			t.Fatalf("seed dir %s: %v", p, err)
		}
	}
	yamlPath := filepath.Join(repoDir, projectconfig.Path)
	if _, err := configinit.Run(monolithMonorepoFlags(), yamlPath, false); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	path, err := runConfigPreflight(yamlPath, offlinePreflightOpts(workspace, repoDir))
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if path != yamlPath {
		t.Errorf("path: got %q, want %q", path, yamlPath)
	}
}

// TestRunConfigPreflight_MissingTierPath drops the simulators directory
// from the seeded workspace — the exact scenario behind the late "preflight
// failed: external_systems.simulators.path: ... does not exist" error
// `config preflight` is meant to catch up-front.
func TestRunConfigPreflight_MissingTierPath(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	repoDir := filepath.Join(workspace, "page-turner")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatalf("seed .git: %v", err)
	}
	for _, p := range []string{
		"system/monolith/java",
		"system-test/java",
		"external-systems/stubs",
		// external-systems/simulators intentionally absent.
	} {
		if err := os.MkdirAll(filepath.Join(repoDir, p), 0o755); err != nil {
			t.Fatalf("seed dir %s: %v", p, err)
		}
	}
	yamlPath := filepath.Join(repoDir, projectconfig.Path)
	if _, err := configinit.Run(monolithMonorepoFlags(), yamlPath, false); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	_, err := runConfigPreflight(yamlPath, offlinePreflightOpts(workspace, repoDir))
	if err == nil {
		t.Fatal("want error for missing simulators path, got nil")
	}
	if !strings.Contains(err.Error(), "external_systems.simulators.path") {
		t.Errorf("error should name the missing field, got: %v", err)
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should report missing dir, got: %v", err)
	}
}

// TestDefaultPreflightOptions_SonarTokenMissing covers the strict
// behavior: when cfg declares a SonarCloud org but $SONAR_TOKEN is unset,
// the helper refuses to wire up Sonar checks (which would silently pass)
// and instead surfaces a clean "set SONAR_TOKEN" error to the cobra layer.
func TestDefaultPreflightOptions_SonarTokenMissing(t *testing.T) {
	t.Setenv("SONAR_TOKEN", "")
	cfg := &projectconfig.Config{
		Sonar: projectconfig.Sonar{Organization: "acme"},
	}
	_, err := defaultPreflightOptions(cfg, "", "")
	if err == nil {
		t.Fatal("want error when SONAR_TOKEN missing and sonar.organization set, got nil")
	}
	if !strings.Contains(err.Error(), "SONAR_TOKEN") {
		t.Errorf("error should name SONAR_TOKEN, got: %v", err)
	}
	if !strings.Contains(err.Error(), "acme") {
		t.Errorf("error should name the declared org, got: %v", err)
	}
}

// TestDefaultPreflightOptions_NoSonarConfig covers the offline-friendly
// path: when cfg has no sonar.organization, $SONAR_TOKEN is not required
// and the helper returns an Options with SonarOrgExists / SonarProjectExists
// left nil (preflight skips that class).
func TestDefaultPreflightOptions_NoSonarConfig(t *testing.T) {
	t.Setenv("SONAR_TOKEN", "")
	cfg := &projectconfig.Config{}
	opts, err := defaultPreflightOptions(cfg, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.SonarOrgExists != nil {
		t.Error("SonarOrgExists should be nil when cfg has no sonar.organization")
	}
	if opts.SonarProjectExists != nil {
		t.Error("SonarProjectExists should be nil when cfg has no sonar.organization")
	}
	if opts.RepoExists == nil {
		t.Error("RepoExists should always be wired (GitHub check is unconditional)")
	}
	if opts.BoardURLOK == nil {
		t.Error("BoardURLOK should always be wired")
	}
}

// TestDefaultPreflightOptions_SonarTokenPresent covers the happy path:
// $SONAR_TOKEN set + sonar.organization declared → both Sonar checkers
// wired up.
func TestDefaultPreflightOptions_SonarTokenPresent(t *testing.T) {
	t.Setenv("SONAR_TOKEN", "fake-token")
	cfg := &projectconfig.Config{
		Sonar: projectconfig.Sonar{Organization: "acme"},
	}
	opts, err := defaultPreflightOptions(cfg, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.SonarOrgExists == nil {
		t.Error("SonarOrgExists should be wired when token + org are set")
	}
	if opts.SonarProjectExists == nil {
		t.Error("SonarProjectExists should be wired when token + org are set")
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

// ---------------------------------------------------------------------------
// runConfigMigrate
// ---------------------------------------------------------------------------

// TestRunConfigMigrate_AddsGitHubProvider — a pre-provider config with a
// github URL is rewritten with provider: github prepended inside the
// project block. The original URL and surrounding fields are preserved
// so an operator's hand edits don't get lost.
func TestRunConfigMigrate_AddsGitHubProvider(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	body := "project:\n  url: https://github.com/orgs/acme/projects/1\nrepo_strategy: mono-repo\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	changed, err := runConfigMigrate(path)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !changed {
		t.Fatal("migrate: want changed=true on first run, got false")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(got), "provider: github") {
		t.Errorf("migrated file missing provider: github; got:\n%s", got)
	}
	if !strings.Contains(string(got), "url: https://github.com/orgs/acme/projects/1") {
		t.Errorf("migrated file lost project.url; got:\n%s", got)
	}
	if !strings.Contains(string(got), "repo_strategy: mono-repo") {
		t.Errorf("migrated file lost repo_strategy; got:\n%s", got)
	}
}

// TestRunConfigMigrate_AddsMarkdownProviderForNonGitHubURL — a config
// whose project.url is a directory path (or any non-github URL) gets
// provider: markdown so the markdown adapter takes over.
func TestRunConfigMigrate_AddsMarkdownProviderForNonGitHubURL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	body := "project:\n  url: ./board\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := runConfigMigrate(path); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "provider: markdown") {
		t.Errorf("migrated file missing provider: markdown; got:\n%s", got)
	}
}

// TestRunConfigMigrate_IsIdempotent — running migrate a second time on
// an already-migrated file is a no-op (changed=false, file untouched).
func TestRunConfigMigrate_IsIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	body := "project:\n  provider: github\n  url: https://github.com/orgs/acme/projects/1\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before, _ := os.ReadFile(path)
	changed, err := runConfigMigrate(path)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if changed {
		t.Error("migrate: want changed=false on already-migrated file, got true")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Errorf("file mutated despite no-op migrate;\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestRunConfigMigrate_RoundTripsThroughLoad — what migrate writes must
// pass projectconfig.Load. Catches schema drift between the migrated
// shape and what Validate accepts.
func TestRunConfigMigrate_RoundTripsThroughLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	body := "project:\n  url: https://github.com/orgs/acme/projects/1\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := runConfigMigrate(path); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg, err := projectconfig.LoadFromPath(path)
	if err != nil {
		t.Fatalf("load after migrate: %v", err)
	}
	if cfg.Project.Provider != projectconfig.ProviderGitHub {
		t.Errorf("loaded provider: got %q, want %q", cfg.Project.Provider, projectconfig.ProviderGitHub)
	}
}

// ---------------------------------------------------------------------------
// runConfigMigrate — repos: back-fill (Phase 3 item 13)
// ---------------------------------------------------------------------------

// multiRepoMonolithBody is a pre-repos:-field config of the canonical
// multi-repo monolith shape: the system code and the system_test code
// live in two separate repos. Used to seed migrate tests that exercise
// the repos: back-fill on a config without that field.
const multiRepoMonolithBody = `project:
  provider: github
  url: https://github.com/orgs/optivem/projects/20

repo_strategy: multi-repo

sonar:
  organization: optivem

system:
  architecture: monolith
  path: .
  repo: optivem/shop-system
  lang: java
  sonar_project: optivem_shop-system

system_test:
  path: system-test
  repo: optivem/shop-tests
  lang: java
  sonar_project: optivem_shop-system-test
`

// multiRepoMultitierBody is a pre-repos:-field config of the canonical
// multi-repo multitier shape: three independent repos (backend,
// frontend, system_test). Mirrors the sample in projectconfig tests
// minus the repos: field.
const multiRepoMultitierBody = `project:
  provider: github
  url: https://github.com/orgs/optivem/projects/20

repo_strategy: multi-repo

sonar:
  organization: optivem

system:
  architecture: multitier
  backend:
    path: .
    repo: optivem/shop-backend
    lang: java
    sonar_project: optivem_shop-backend
  frontend:
    path: .
    repo: optivem/shop-frontend
    lang: typescript
    sonar_project: optivem_shop-frontend

system_test:
  path: system-test
  repo: optivem/shop-tests
  lang: java
  sonar_project: optivem_shop-system-test
`

// monoRepoMonolithBody is the canonical mono-repo monolith config —
// repos: should NOT be back-filled because the single-repo cascade row
// already covers it.
const monoRepoMonolithBody = `project:
  provider: github
  url: https://github.com/orgs/optivem/projects/20

repo_strategy: mono-repo

sonar:
  organization: optivem

system:
  architecture: monolith
  path: system/monolith/java
  repo: optivem/shop
  lang: java
  sonar_project: optivem_shop-system

system_test:
  path: system-test/java
  repo: optivem/shop
  lang: java
  sonar_project: optivem_shop-system-test
`

// TestRunConfigMigrate_BackfillsReposForMultiRepoMonolith pins the
// monolith side of the new back-fill: two ../<name> entries, one per
// tier slug.
func TestRunConfigMigrate_BackfillsReposForMultiRepoMonolith(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	if err := os.WriteFile(path, []byte(multiRepoMonolithBody), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	changed, err := runConfigMigrate(path)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !changed {
		t.Fatal("migrate: want changed=true (repos: should be back-filled)")
	}
	cfg, err := projectconfig.LoadFromPath(path)
	if err != nil {
		t.Fatalf("load after migrate: %v", err)
	}
	want := []string{"../shop-system", "../shop-tests"}
	got := make([]string, 0, len(cfg.LocalRepos))
	for _, r := range cfg.LocalRepos {
		got = append(got, r.Path)
	}
	if !equalSliceUnordered(got, want) {
		t.Errorf("repos paths: got %v, want %v (any order)", got, want)
	}
}

// TestRunConfigMigrate_BackfillsReposForMultiRepoMultitier pins the
// multitier side: backend + frontend + system_test → three entries.
func TestRunConfigMigrate_BackfillsReposForMultiRepoMultitier(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	if err := os.WriteFile(path, []byte(multiRepoMultitierBody), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := runConfigMigrate(path); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg, err := projectconfig.LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := []string{"../shop-backend", "../shop-frontend", "../shop-tests"}
	got := make([]string, 0, len(cfg.LocalRepos))
	for _, r := range cfg.LocalRepos {
		got = append(got, r.Path)
	}
	if !equalSliceUnordered(got, want) {
		t.Errorf("repos paths: got %v, want %v (any order)", got, want)
	}
}

// TestRunConfigMigrate_SkipsReposForMonoRepo pins that mono-repo
// configs are left untouched on the repos: front — they already work
// via the single-repo cascade row. The function still reports
// changed=false (no other field needed back-filling either).
func TestRunConfigMigrate_SkipsReposForMonoRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	if err := os.WriteFile(path, []byte(monoRepoMonolithBody), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before, _ := os.ReadFile(path)
	changed, err := runConfigMigrate(path)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if changed {
		t.Error("migrate: want changed=false on mono-repo config (repos: not needed)")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Errorf("file mutated despite no-op migrate")
	}
}

// TestRunConfigMigrate_ReposIsIdempotent pins running migrate twice on
// a multi-repo config is a no-op the second time — the back-filled
// repos: list survives unchanged.
func TestRunConfigMigrate_ReposIsIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	if err := os.WriteFile(path, []byte(multiRepoMultitierBody), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := runConfigMigrate(path); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	firstPass, _ := os.ReadFile(path)
	changed, err := runConfigMigrate(path)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if changed {
		t.Error("second migrate: want changed=false, got true")
	}
	secondPass, _ := os.ReadFile(path)
	if string(firstPass) != string(secondPass) {
		t.Errorf("file changed on second migrate:\nbefore:\n%s\nafter:\n%s", firstPass, secondPass)
	}
}

// TestRunConfigMigrate_BackfillsBothProviderAndRepos pins the combined
// migration: a config older than both schema bumps gets provider AND
// repos: in one run. Models the realistic upgrade path for a project
// that has been dormant since before the provider field was added.
func TestRunConfigMigrate_BackfillsBothProviderAndRepos(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	// Pre-provider, pre-repos body — only url is set on the project
	// block. Mirrors the shape of a really-old config.
	body := strings.Replace(multiRepoMultitierBody,
		"  provider: github\n", "", 1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := runConfigMigrate(path); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg, err := projectconfig.LoadFromPath(path)
	if err != nil {
		t.Fatalf("load after migrate: %v", err)
	}
	if cfg.Project.Provider != projectconfig.ProviderGitHub {
		t.Errorf("provider: got %q, want github", cfg.Project.Provider)
	}
	if len(cfg.LocalRepos) != 3 {
		t.Errorf("repos: got %d entries, want 3 (backend + frontend + tests)", len(cfg.LocalRepos))
	}
}

// TestRunConfigMigrate_RespectsExistingRepos pins that an
// already-populated repos: list survives migration — operators may
// have hand-edited the paths to match an outlier on-disk layout, and
// migrate must not clobber that work.
func TestRunConfigMigrate_RespectsExistingRepos(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, projectconfig.Path)
	body := multiRepoMultitierBody + `
repos:
  - path: ./custom-backend
  - path: ./custom-frontend
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	changed, err := runConfigMigrate(path)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if changed {
		t.Error("migrate: want changed=false when repos: already present")
	}
	cfg, err := projectconfig.LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.LocalRepos) != 2 ||
		cfg.LocalRepos[0].Path != "./custom-backend" ||
		cfg.LocalRepos[1].Path != "./custom-frontend" {
		t.Errorf("hand-edited repos: clobbered, got %+v", cfg.LocalRepos)
	}
}

// equalSliceUnordered compares two []string for set equality (order
// independent). Migrate's repos[] insertion order is structural — it
// follows the tier traversal — but tests pin only the membership so
// future tier reorderings don't break them.
func equalSliceUnordered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}
