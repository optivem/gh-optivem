package steps

import (
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// monolithFlatPaths returns the path values the scaffolder injects into a
// monolith Config (matching the Default*Path constants in internal/config).
// Tests that exercise BuildOptivemYAML for the post-scaffold layout reuse
// these instead of repeating the literal strings.
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
	got := BuildOptivemYAML(cfg)
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
	got := BuildOptivemYAML(cfg)
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
	got := BuildOptivemYAML(cfg)
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
	got := BuildOptivemYAML(cfg)
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
		pc := BuildOptivemYAML(cfg)
		if err := pc.Validate(); err != nil {
			t.Errorf("case %d: generated config fails Validate: %v\n%+v", i, err, pc)
		}
	}
}

// TestBuildOptivemYAML_PathsBlockSeededPerLanguage — the scaffolder emits
// a non-empty `paths:` Family B block whose keys match the placeholder
// doctrine. Without this block a freshly-scaffolded project would fail
// MaterializeProject on first dispatch (any phase doc referencing
// ${driver_port} / ${driver_adapter} / ${external_driver_*} would
// surface as an unfilled placeholder).
func TestBuildOptivemYAML_PathsBlockSeededPerLanguage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		testLang string
		wantKey  string
		wantPath string
	}{
		{"typescript", projectconfig.LangTypescript, "driver_port", "system-test/src/testkit/driver/port"},
		{"java", projectconfig.LangJava, "driver_port", "system-test/src/main/java/testkit/driver/port"},
		{"dotnet", projectconfig.LangDotnet, "driver_port", "system-test/Testkit.Driver.Port"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				Arch:           "monolith",
				RepoStrategy:   "monorepo",
				FullRepo:       "x/y",
				Lang:           tc.testLang,
				TestLang:       tc.testLang,
				SystemPath:     "system",
				SystemTestPath: "system-test",
				StubsPath:      "external-systems/stubs",
				SimulatorsPath: "external-systems/simulators",
			}
			got := BuildOptivemYAML(cfg)
			if len(got.Paths) == 0 {
				t.Fatal("paths: block should be seeded by the scaffolder")
			}
			for _, k := range []string{"driver_port", "driver_adapter", "external_driver_port", "external_driver_adapter"} {
				if _, ok := got.Paths[k]; !ok {
					t.Errorf("paths.%s missing", k)
				}
			}
			if got.Paths[tc.wantKey] != tc.wantPath {
				t.Errorf("paths.%s: got %q, want %q", tc.wantKey, got.Paths[tc.wantKey], tc.wantPath)
			}
		})
	}
}

// TestBuildOptivemYAML_PathsBlockMaterializeOK — the scaffolded
// `paths:` block plus the schema's Family A values must yield a
// placeholder map that satisfies every ${name} reference in the
// embedded phase docs. Smoke-test by validating the emitted config.
func TestBuildOptivemYAML_PathsBlockMaterializeOK(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Arch:           "monolith",
		RepoStrategy:   "monorepo",
		Owner:          "x",
		FullRepo:       "x/y",
		Lang:           projectconfig.LangTypescript,
		TestLang:       projectconfig.LangTypescript,
		ProjectURL:     "https://github.com/orgs/x/projects/1",
		SystemPath:     "system",
		SystemTestPath: "system-test",
		StubsPath:      "external-systems/stubs",
		SimulatorsPath: "external-systems/simulators",
	}
	pc := BuildOptivemYAML(cfg)
	if err := pc.Validate(); err != nil {
		t.Fatalf("scaffolded config fails Validate: %v", err)
	}
	pm := pc.PlaceholderMap()
	for _, key := range []string{"driver_port", "driver_adapter", "external_driver_port", "external_driver_adapter"} {
		if pm[key] == "" {
			t.Errorf("placeholder map missing %q after scaffold", key)
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
	got := BuildOptivemYAML(cfg)
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
