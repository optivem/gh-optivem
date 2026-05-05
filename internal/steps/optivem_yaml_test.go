package steps

import (
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

func TestBuildOptivemYAML_MonolithMonorepo(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Arch:         "monolith",
		RepoStrategy: "monorepo",
		Owner:        "x",
		Repo:         "shop",
		FullRepo:     "x/shop",
		Lang:         "java",
		TestLang:     "java",
		ProjectURL:   "https://github.com/orgs/x/projects/1",
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
	if got.System.Path != "system/monolith/java" {
		t.Errorf("System.Path: got %q", got.System.Path)
	}
	if got.System.Repo != "x/shop" {
		t.Errorf("System.Repo: got %q", got.System.Repo)
	}
	if got.System.Lang != "java" {
		t.Errorf("System.Lang: got %q", got.System.Lang)
	}
	if got.SystemTest.Path != "system-test/java" || got.SystemTest.Repo != "x/shop" || got.SystemTest.Lang != "java" {
		t.Errorf("SystemTest mismatch: %+v", got.SystemTest)
	}
	if got.ExternalSystems.Stubs.Path != "external-stub" || got.ExternalSystems.Stubs.Repo != "x/shop" {
		t.Errorf("Stubs mismatch: %+v", got.ExternalSystems.Stubs)
	}
	if got.ExternalSystems.Simulators.Path != "external-real-sim" || got.ExternalSystems.Simulators.Repo != "x/shop" {
		t.Errorf("Simulators mismatch: %+v", got.ExternalSystems.Simulators)
	}
}

func TestBuildOptivemYAML_MultitierMultirepo(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Arch:             "multitier",
		RepoStrategy:     "multirepo",
		BackendLang:      "dotnet",
		FrontendLang:     "react",
		TestLang:         "typescript",
		BackendFullRepo:  "acme/shop-backend",
		FrontendFullRepo: "acme/shop-frontend",
	}
	got := buildOptivemYAML(cfg)
	if got.RepoStrategy != projectconfig.RepoStrategyMultiRepo {
		t.Errorf("RepoStrategy: got %q, want %q", got.RepoStrategy, projectconfig.RepoStrategyMultiRepo)
	}
	if got.System.Backend.Path != "system/multitier/backend-dotnet" {
		t.Errorf("Backend.Path: got %q", got.System.Backend.Path)
	}
	if got.System.Backend.Repo != "acme/shop-backend" || got.System.Backend.Lang != "dotnet" {
		t.Errorf("Backend mismatch: %+v", got.System.Backend)
	}
	if got.System.Frontend.Path != "system/multitier/frontend-react" {
		t.Errorf("Frontend.Path: got %q", got.System.Frontend.Path)
	}
	if got.System.Frontend.Repo != "acme/shop-frontend" || got.System.Frontend.Lang != "typescript" {
		t.Errorf("Frontend mismatch: %+v", got.System.Frontend)
	}
	if got.SystemTest.Path != "system-test/typescript" {
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
	cfg := &config.Config{
		Arch:           "monolith",
		RepoStrategy:   "multirepo",
		Lang:           "typescript",
		TestLang:       "typescript",
		SystemFullRepo: "acme/shop-system",
	}
	got := buildOptivemYAML(cfg)
	if got.System.Path != "system/monolith/typescript" {
		t.Errorf("System.Path: got %q", got.System.Path)
	}
	if got.System.Repo != "acme/shop-system" {
		t.Errorf("System.Repo: got %q", got.System.Repo)
	}
	if got.SystemTest.Repo != "acme/shop-system" {
		t.Errorf("SystemTest.Repo: got %q, want system slug", got.SystemTest.Repo)
	}
}

func TestBuildOptivemYAML_OutputValidates(t *testing.T) {
	t.Parallel()
	cases := []*config.Config{
		{Arch: "monolith", RepoStrategy: "monorepo", FullRepo: "x/y", Lang: "java", TestLang: "java"},
		{Arch: "multitier", RepoStrategy: "multirepo", BackendLang: "java", FrontendLang: "react", TestLang: "java",
			BackendFullRepo: "x/y-backend", FrontendFullRepo: "x/y-frontend"},
		{Arch: "monolith", RepoStrategy: "multirepo", Lang: "dotnet", TestLang: "dotnet", SystemFullRepo: "x/y-system"},
		{Arch: "monolith", RepoStrategy: "monorepo", FullRepo: "x/y", Lang: "typescript", TestLang: "typescript"},
		{Arch: "multitier", RepoStrategy: "monorepo", FullRepo: "x/y", BackendLang: "java", FrontendLang: "react", TestLang: "java"},
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
