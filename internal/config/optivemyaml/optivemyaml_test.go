package optivemyaml

import (
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// monolithFlatPaths returns the path values the scaffolder injects into a
// monolith Config (matching the Default*Path constants in internal/config).
// Tests that exercise BuildOptivemYAML for the post-scaffold layout reuse
// these instead of repeating the literal strings.
func monolithFlatPaths() (system, systemTest string) {
	return "system", "system-test"
}

// multitierFlatPaths returns the path values the scaffolder injects into a
// multitier Config.
func multitierFlatPaths() (backend, frontend, systemTest string) {
	return "backend", "frontend", "system-test"
}

func TestBuildOptivemYAML_MonolithMonorepo(t *testing.T) {
	t.Parallel()
	sys, sysTest := monolithFlatPaths()
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
	// SSoT (plan 20260518-1530 item 3): System.Path is fully resolved
	// at scaffold time — `cfg.SystemPath` joined with the derived
	// sutNamespace (last segment of `cfg.FullRepo` = "x/shop" → "shop").
	if got.System.Path != "system/shop" {
		t.Errorf("System.Path: got %q, want system/shop (SSoT)", got.System.Path)
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
	// external-systems is operator-owned and not scaffolded by init
	// (plan 20260606-1356, option 1B): the block is omitted.
	if len(got.ExternalSystems) != 0 {
		t.Errorf("ExternalSystems should be omitted by init, got: %+v", got.ExternalSystems)
	}
}

func TestBuildOptivemYAML_MultitierMultirepo(t *testing.T) {
	t.Parallel()
	be, fe, sysTest := multitierFlatPaths()
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
	if len(got.ExternalSystems) != 0 {
		t.Errorf("ExternalSystems should be omitted by init, got: %+v", got.ExternalSystems)
	}
}

func TestBuildOptivemYAML_MicroservicesMonorepo(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Arch:             "microservices",
		RepoStrategy:     "monorepo",
		Owner:            "optivem",
		Repo:             "shop",
		FullRepo:         "optivem/shop",
		FrontendLang:     "typescript",
		FrontendPath:     "system/microservices/frontend-react",
		FrontendRepoSlug: "optivem/shop",
		TestLang:         "java",
		SystemTestPath:   "system-test/java",
		ProjectURL:       "https://github.com/orgs/optivem/projects/20",
		BackendServices: []config.BackendService{
			{Name: "inventory", Path: "system/microservices/inventory-dotnet", Repo: "optivem/shop", Lang: "dotnet", SonarProject: "optivem_shop-inventory"},
			{Name: "orders", Path: "system/microservices/orders-java", Repo: "optivem/shop", Lang: "java", SonarProject: "optivem_shop-orders"},
		},
	}
	got := BuildOptivemYAML(cfg)
	if got.System.Architecture != "microservices" {
		t.Fatalf("Architecture: got %q", got.System.Architecture)
	}
	if len(got.System.BackendServices) != 2 {
		t.Fatalf("BackendServices: got %d, want 2", len(got.System.BackendServices))
	}
	orders, ok := got.System.BackendServices["orders"]
	if !ok {
		t.Fatalf("BackendServices missing 'orders': %+v", got.System.BackendServices)
	}
	if orders.Path != "system/microservices/orders-java" || orders.Lang != "java" || orders.SonarProject != "optivem_shop-orders" {
		t.Errorf("orders service mismatch: %+v", orders)
	}
	inv := got.System.BackendServices["inventory"]
	if inv.Lang != "dotnet" || inv.SonarProject != "optivem_shop-inventory" {
		t.Errorf("inventory service mismatch: %+v", inv)
	}
	// Single frontend (D5), sonar-project derived as <owner>_<repo>-frontend.
	if got.System.Frontend.Repo != "optivem/shop" || got.System.Frontend.Lang != "typescript" {
		t.Errorf("Frontend mismatch: %+v", got.System.Frontend)
	}
	if got.System.Frontend.SonarProject != "optivem_shop-frontend" {
		t.Errorf("Frontend.SonarProject: got %q, want optivem_shop-frontend", got.System.Frontend.SonarProject)
	}
	// monolith/multitier singular fields stay empty.
	if got.System.Path != "" || !got.System.Backend.IsEmpty() {
		t.Errorf("singular backend fields should be empty: path=%q backend=%+v", got.System.Path, got.System.Backend)
	}
	// The whole thing must validate as a real microservices config.
	if err := got.Validate(); err != nil {
		t.Errorf("BuildOptivemYAML output should validate: %v", err)
	}
}

func TestBuildOptivemYAML_MonolithMultirepo(t *testing.T) {
	t.Parallel()
	sys, sysTest := monolithFlatPaths()
	cfg := &config.Config{
		Arch:           "monolith",
		RepoStrategy:   "multirepo",
		Lang:           "typescript",
		TestLang:       "typescript",
		SystemFullRepo: "acme/shop-system",
		SystemPath:     sys,
		SystemTestPath: sysTest,
	}
	got := BuildOptivemYAML(cfg)
	// SSoT (plan 20260518-1530 item 3): System.Path baked with
	// sutNamespace derived from cfg.SystemFullRepo's last segment
	// (multirepo monolith uses SystemFullRepo, not FullRepo).
	if got.System.Path != "system/shop-system" {
		t.Errorf("System.Path: got %q, want system/shop-system (SSoT)", got.System.Path)
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
	}
	got := BuildOptivemYAML(cfg)
	// SSoT: even non-scaffold (shop-worktree-style) callers get
	// sutNamespace baked into System.Path. FullRepo=optivem/shop →
	// sutNamespace=shop → cfg.SystemPath joined to it.
	if got.System.Path != "system/monolith/java/shop" {
		t.Errorf("System.Path: got %q, want shop-style path with sut-namespace", got.System.Path)
	}
	if got.SystemTest.Path != "system-test/java" {
		t.Errorf("SystemTest.Path: got %q, want shop-style path", got.SystemTest.Path)
	}
}

func TestBuildOptivemYAML_OutputValidates(t *testing.T) {
	t.Parallel()
	monoSys, monoTest := monolithFlatPaths()
	multiBE, multiFE, multiTest := multitierFlatPaths()
	const url = "https://github.com/orgs/x/projects/1"
	cases := []*config.Config{
		{Owner: "x", Repo: "y", Arch: "monolith", RepoStrategy: "monorepo", FullRepo: "x/y", Lang: "java", TestLang: "java", ProjectURL: url,
			SystemPath: monoSys, SystemTestPath: monoTest},
		{Owner: "x", Repo: "y", Arch: "multitier", RepoStrategy: "multirepo", BackendLang: "java", FrontendLang: "typescript", TestLang: "java", ProjectURL: url,
			BackendFullRepo: "x/y-backend", FrontendFullRepo: "x/y-frontend",
			BackendPath: multiBE, FrontendPath: multiFE, SystemTestPath: multiTest},
		{Owner: "x", Repo: "y", Arch: "monolith", RepoStrategy: "multirepo", Lang: "dotnet", TestLang: "dotnet", SystemFullRepo: "x/y-system", ProjectURL: url,
			SystemPath: monoSys, SystemTestPath: monoTest},
		{Owner: "x", Repo: "y", Arch: "monolith", RepoStrategy: "monorepo", FullRepo: "x/y", Lang: "typescript", TestLang: "typescript", ProjectURL: url,
			SystemPath: monoSys, SystemTestPath: monoTest},
		{Owner: "x", Repo: "y", Arch: "multitier", RepoStrategy: "monorepo", FullRepo: "x/y", BackendLang: "java", FrontendLang: "typescript", TestLang: "java", ProjectURL: url,
			BackendPath: multiBE, FrontendPath: multiFE, SystemTestPath: multiTest},
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
// on first dispatch (any phase doc referencing ${system-driver-port} /
// ${system-driver-adapter} / ${external-system-driver-*} would surface as an
// unfilled placeholder).
func TestBuildOptivemYAML_PathsBlockSeededPerLanguage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		testLang      string
		wantKey       string
		wantPath      string
		wantAPIMember string // per-language system-driver-adapter-channels.api
	}{
		// The emitted paths reproduce the shop template's checked-in `paths:`
		// block (plan 20260526-1430). TS/.NET carry no namespace; Java carries
		// the resolved source package `com/<owner>/<system>` as a middle segment
		// — here Owner="acme", SystemName="My Shop" → `com/acme/myshop`. The api
		// member is the adapter root + channel, cased per language (TS/Java
		// lowercase, .NET PascalCase).
		{"typescript", projectconfig.LangTypescript, "system-driver-port", "system-test/src/testkit/driver/port", "system-test/src/testkit/driver/adapter/api"},
		{"java", projectconfig.LangJava, "system-driver-port", "system-test/src/main/java/com/acme/myshop/testkit/driver/port", "system-test/src/main/java/com/acme/myshop/testkit/driver/adapter/api"},
		{"dotnet", projectconfig.LangDotnet, "system-driver-port", "system-test/Driver.Port", "system-test/Driver.Adapter/Api"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				Arch:           "monolith",
				RepoStrategy:   "monorepo",
				Owner:          "acme",
				SystemName:     "My Shop",
				FullRepo:       "acme/my-shop",
				Lang:           tc.testLang,
				TestLang:       tc.testLang,
				SystemPath:     "system",
				SystemTestPath: "system-test",
			}
			got := BuildOptivemYAML(cfg)
			if len(got.SystemTest.Paths) == 0 {
				t.Fatal("system-test.paths: block should be seeded by the scaffolder")
			}
			for _, k := range []string{"system-driver-port", "system-driver-adapter", "external-system-driver-port", "external-system-driver-adapter"} {
				if _, ok := got.SystemTest.Paths[k]; !ok {
					t.Errorf("system-test.paths.%s missing", k)
				}
			}
			if got.SystemTest.Paths[tc.wantKey] != tc.wantPath {
				t.Errorf("system-test.paths.%s: got %q, want %q", tc.wantKey, got.SystemTest.Paths[tc.wantKey], tc.wantPath)
			}
			// Per-channel adapter members seeded 1:1 with the default channel set.
			members := got.SystemTest.SystemDriverAdapterChannels
			for _, ch := range projectconfig.DefaultChannels() {
				if _, ok := members[ch]; !ok {
					t.Errorf("system-driver-adapter-channels.%s missing", ch)
				}
			}
			if members["api"] != tc.wantAPIMember {
				t.Errorf("system-driver-adapter-channels.api: got %q, want %q", members["api"], tc.wantAPIMember)
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
	}
	pc := BuildOptivemYAML(cfg)
	if err := pc.Validate(); err != nil {
		t.Fatalf("scaffolded config fails Validate: %v", err)
	}
	pm := pc.PlaceholderMap()
	for _, key := range []string{"system-driver-port", "system-driver-adapter", "external-system-driver-port", "external-system-driver-adapter"} {
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
