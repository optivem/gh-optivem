package steps

import (
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// monolithFlatPaths returns the path values the scaffolder injects into a
// monolith Config (matching resolveScaffoldPaths in internal/config). Tests
// that exercise buildOptivemYAML for the post-scaffold layout reuse these
// instead of repeating the literal strings.
func monolithFlatPaths() (system, systemTest, stubs, simulators string) {
	return "system", "system-test", "external-systems/stubs", "external-systems/simulators"
}

// multitierFlatPaths returns the path values the scaffolder injects into a
// multitier Config.
func multitierFlatPaths() (backend, frontend, systemTest, stubs, simulators string) {
	return "backend", "frontend", "system-test", "external-systems/stubs", "external-systems/simulators"
}

func TestBuildOptivemYAML_MonolithMonorepo(t *testing.T) {
	t.Parallel()
	sys, sysTest, stubs, sims := monolithFlatPaths()
	cfg := &config.Config{
		Arch:           "monolith",
		RepoStrategy:   "monorepo",
		Owner:          "x",
		Repo:           "shop",
		FullRepo:       "x/shop",
		Lang:           "java",
		TestLang:       "java",
		ProjectURL:     "https://github.com/orgs/x/projects/1",
		SystemPath:     sys,
		SystemTestPath: sysTest,
		StubsPath:      stubs,
		SimulatorsPath: sims,
	}
	got := buildOptivemYAML(cfg)
	if got.Project.URL != cfg.ProjectURL {
		t.Errorf("URL: got %q, want %q", got.Project.URL, cfg.ProjectURL)
	}
	if got.RepoStrategy != projectconfig.RepoStrategyMonoRepo {
		t.Errorf("RepoStrategy: got %q, want %q", got.RepoStrategy, projectconfig.RepoStrategyMonoRepo)
	}
	if got.System.Architecture != "monolith" {
		t.Errorf("System.Architecture: got %q", got.System.Architecture)
	}
	if got.System.Path != "system" {
		t.Errorf("System.Path: got %q", got.System.Path)
	}
	if got.System.Repo != "x/shop" {
		t.Errorf("System.Repo: got %q", got.System.Repo)
	}
	if got.System.Lang != "java" {
		t.Errorf("System.Lang: got %q", got.System.Lang)
	}
	if got.SystemTest.Path != "system-test" || got.SystemTest.Repo != "x/shop" || got.SystemTest.Lang != "java" {
		t.Errorf("SystemTest mismatch: %+v", got.SystemTest)
	}
	if got.ExternalSystems.Stubs.Path != "external-systems/stubs" || got.ExternalSystems.Stubs.Repo != "x/shop" {
		t.Errorf("Stubs mismatch: %+v", got.ExternalSystems.Stubs)
	}
	if got.ExternalSystems.Simulators.Path != "external-systems/simulators" || got.ExternalSystems.Simulators.Repo != "x/shop" {
		t.Errorf("Simulators mismatch: %+v", got.ExternalSystems.Simulators)
	}
}

func TestBuildOptivemYAML_MultitierMultirepo(t *testing.T) {
	t.Parallel()
	be, fe, sysTest, stubs, sims := multitierFlatPaths()
	cfg := &config.Config{
		Arch:             "multitier",
		RepoStrategy:     "multirepo",
		BackendLang:      "dotnet",
		FrontendLang:     "typescript",
		TestLang:         "typescript",
		BackendFullRepo:  "acme/shop-backend",
		FrontendFullRepo: "acme/shop-frontend",
		BackendPath:      be,
		FrontendPath:     fe,
		SystemTestPath:   sysTest,
		StubsPath:        stubs,
		SimulatorsPath:   sims,
	}
	got := buildOptivemYAML(cfg)
	if got.RepoStrategy != projectconfig.RepoStrategyMultiRepo {
		t.Errorf("RepoStrategy: got %q, want %q", got.RepoStrategy, projectconfig.RepoStrategyMultiRepo)
	}
	if got.System.Backend.Path != "backend" {
		t.Errorf("Backend.Path: got %q", got.System.Backend.Path)
	}
	if got.System.Backend.Repo != "acme/shop-backend" || got.System.Backend.Lang != "dotnet" {
		t.Errorf("Backend mismatch: %+v", got.System.Backend)
	}
	if got.System.Frontend.Path != "frontend" {
		t.Errorf("Frontend.Path: got %q", got.System.Frontend.Path)
	}
	if got.System.Frontend.Repo != "acme/shop-frontend" || got.System.Frontend.Lang != "typescript" {
		t.Errorf("Frontend mismatch: %+v", got.System.Frontend)
	}
	if got.SystemTest.Path != "system-test" {
		t.Errorf("SystemTest.Path: got %q", got.SystemTest.Path)
	}
	// SystemTest defaults to the backend repo in multi-repo + multitier.
	if got.SystemTest.Repo != "acme/shop-backend" {
		t.Errorf("SystemTest.Repo: got %q, want backend slug", got.SystemTest.Repo)
	}
	if got.ExternalSystems.Stubs.Repo != "acme/shop-backend" {
		t.Errorf("Stubs.Repo: got %q, want backend slug (default)", got.ExternalSystems.Stubs.Repo)
	}
}

func TestBuildOptivemYAML_MonolithMultirepo(t *testing.T) {
	t.Parallel()
	sys, sysTest, stubs, sims := monolithFlatPaths()
	cfg := &config.Config{
		Arch:           "monolith",
		RepoStrategy:   "multirepo",
		Lang:           "typescript",
		TestLang:       "typescript",
		SystemFullRepo: "acme/shop-system",
		SystemPath:     sys,
		SystemTestPath: sysTest,
		StubsPath:      stubs,
		SimulatorsPath: sims,
	}
	got := buildOptivemYAML(cfg)
	if got.System.Path != "system" {
		t.Errorf("System.Path: got %q", got.System.Path)
	}
	if got.System.Repo != "acme/shop-system" {
		t.Errorf("System.Repo: got %q", got.System.Repo)
	}
	if got.SystemTest.Repo != "acme/shop-system" {
		t.Errorf("SystemTest.Repo: got %q, want system slug", got.SystemTest.Repo)
	}
}

// TestBuildOptivemYAML_NonScaffoldPaths exercises the explicit-paths
// contract: any caller can supply arbitrary paths (e.g. the rehearsal
// script writing a YAML for shop's worktree, where system code lives at
// system/monolith/{lang}/ rather than the flat system/).
func TestBuildOptivemYAML_NonScaffoldPaths(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Arch:           "monolith",
		RepoStrategy:   "monorepo",
		FullRepo:       "optivem/shop",
		Lang:           "java",
		TestLang:       "java",
		SystemPath:     "system/monolith/java",
		SystemTestPath: "system-test/java",
		StubsPath:      "external-systems/stubs",
		SimulatorsPath: "external-systems/simulators",
	}
	got := buildOptivemYAML(cfg)
	if got.System.Path != "system/monolith/java" {
		t.Errorf("System.Path: got %q, want shop-style path", got.System.Path)
	}
	if got.SystemTest.Path != "system-test/java" {
		t.Errorf("SystemTest.Path: got %q, want shop-style path", got.SystemTest.Path)
	}
}

func TestBuildOptivemYAML_OutputValidates(t *testing.T) {
	t.Parallel()
	monoSys, monoTest, monoStubs, monoSims := monolithFlatPaths()
	multiBE, multiFE, multiTest, multiStubs, multiSims := multitierFlatPaths()
	const url = "https://github.com/orgs/x/projects/1"
	cases := []*config.Config{
		{Owner: "x", Repo: "y", Arch: "monolith", RepoStrategy: "monorepo", FullRepo: "x/y", Lang: "java", TestLang: "java", ProjectURL: url,
			SystemPath: monoSys, SystemTestPath: monoTest, StubsPath: monoStubs, SimulatorsPath: monoSims},
		{Owner: "x", Repo: "y", Arch: "multitier", RepoStrategy: "multirepo", BackendLang: "java", FrontendLang: "typescript", TestLang: "java", ProjectURL: url,
			BackendFullRepo: "x/y-backend", FrontendFullRepo: "x/y-frontend",
			BackendPath: multiBE, FrontendPath: multiFE, SystemTestPath: multiTest, StubsPath: multiStubs, SimulatorsPath: multiSims},
		{Owner: "x", Repo: "y", Arch: "monolith", RepoStrategy: "multirepo", Lang: "dotnet", TestLang: "dotnet", SystemFullRepo: "x/y-system", ProjectURL: url,
			SystemPath: monoSys, SystemTestPath: monoTest, StubsPath: monoStubs, SimulatorsPath: monoSims},
		{Owner: "x", Repo: "y", Arch: "monolith", RepoStrategy: "monorepo", FullRepo: "x/y", Lang: "typescript", TestLang: "typescript", ProjectURL: url,
			SystemPath: monoSys, SystemTestPath: monoTest, StubsPath: monoStubs, SimulatorsPath: monoSims},
		{Owner: "x", Repo: "y", Arch: "multitier", RepoStrategy: "monorepo", FullRepo: "x/y", BackendLang: "java", FrontendLang: "typescript", TestLang: "java", ProjectURL: url,
			BackendPath: multiBE, FrontendPath: multiFE, SystemTestPath: multiTest, StubsPath: multiStubs, SimulatorsPath: multiSims},
	}
	for i, cfg := range cases {
		pc := buildOptivemYAML(cfg)
		if err := pc.Validate(); err != nil {
			t.Errorf("case %d: generated config fails Validate: %v\n%+v", i, err, pc)
		}
	}
}

func TestBuildOptivemYAML_EmptyArchProducesPartialConfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ProjectURL:   "https://github.com/orgs/x/projects/1",
		RepoStrategy: "monorepo",
		// Arch empty.
	}
	got := buildOptivemYAML(cfg)
	if got.Project.URL == "" {
		t.Error("URL should be carried through even when Arch is empty")
	}
	if got.RepoStrategy != projectconfig.RepoStrategyMonoRepo {
		t.Error("RepoStrategy should be mapped even when Arch is empty")
	}
	if got.System.Architecture != "" {
		t.Errorf("expected empty System.Architecture, got %q", got.System.Architecture)
	}
	if !got.SystemTest.IsEmpty() {
		t.Errorf("expected empty SystemTest, got %+v", got.SystemTest)
	}
	if err := got.Validate(); err != nil {
		t.Errorf("partial config should still validate, got: %v", err)
	}
}
