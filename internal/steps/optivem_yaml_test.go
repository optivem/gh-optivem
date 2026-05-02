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
		Lang:         "java",
		TestLang:     "java",
		ProjectURL:   "https://github.com/orgs/x/projects/1",
	}
	got := buildOptivemYAML(cfg)
	if got.Project.URL != cfg.ProjectURL {
		t.Errorf("URL: got %q, want %q", got.Project.URL, cfg.ProjectURL)
	}
	if got.Project.RepoStrategy != projectconfig.RepoStrategyMonoRepo {
		t.Errorf("RepoStrategy: got %q, want %q", got.Project.RepoStrategy, projectconfig.RepoStrategyMonoRepo)
	}
	if len(got.Project.Repos) != 0 {
		t.Errorf("Repos: got %v, want empty for mono-repo", got.Project.Repos)
	}
	if got.Scope.Architecture != "monolith" || got.Scope.SystemLang != "java" || got.Scope.TestLang != "java" {
		t.Errorf("Scope mismatch: got %+v", got.Scope)
	}
}

func TestBuildOptivemYAML_MultitierMultirepo(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Arch:             "multitier",
		RepoStrategy:     "multirepo",
		Lang:             "", // unset for multitier
		BackendLang:      "dotnet",
		FrontendLang:     "react",
		TestLang:         "typescript",
		BackendFullRepo:  "acme/shop-backend",
		FrontendFullRepo: "acme/shop-frontend",
	}
	got := buildOptivemYAML(cfg)
	if got.Project.RepoStrategy != projectconfig.RepoStrategyMultiRepo {
		t.Errorf("RepoStrategy: got %q, want %q", got.Project.RepoStrategy, projectconfig.RepoStrategyMultiRepo)
	}
	want := []string{"acme/shop-backend", "acme/shop-frontend"}
	if len(got.Project.Repos) != 2 || got.Project.Repos[0] != want[0] || got.Project.Repos[1] != want[1] {
		t.Errorf("Repos: got %v, want %v", got.Project.Repos, want)
	}
	if got.Scope.SystemLang != "dotnet" {
		t.Errorf("SystemLang: got %q, want backend %q", got.Scope.SystemLang, "dotnet")
	}
	if got.Scope.TestLang != "typescript" {
		t.Errorf("TestLang: got %q", got.Scope.TestLang)
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
	want := []string{"acme/shop-system"}
	if len(got.Project.Repos) != 1 || got.Project.Repos[0] != want[0] {
		t.Errorf("Repos: got %v, want %v", got.Project.Repos, want)
	}
	if got.Scope.SystemLang != "typescript" {
		t.Errorf("SystemLang: got %q, want monolith %q", got.Scope.SystemLang, "typescript")
	}
}

func TestBuildOptivemYAML_OutputValidates(t *testing.T) {
	t.Parallel()
	cases := []*config.Config{
		{Arch: "monolith", RepoStrategy: "monorepo", Lang: "java", TestLang: "java"},
		{Arch: "multitier", RepoStrategy: "multirepo", BackendLang: "java", FrontendLang: "react", TestLang: "java",
			BackendFullRepo: "x/y-backend", FrontendFullRepo: "x/y-frontend"},
		{Arch: "monolith", RepoStrategy: "multirepo", Lang: "dotnet", TestLang: "dotnet", SystemFullRepo: "x/y-system"},
	}
	for i, cfg := range cases {
		pc := buildOptivemYAML(cfg)
		if err := pc.Validate(); err != nil {
			t.Errorf("case %d: generated config fails Validate: %v\n%+v", i, err, pc)
		}
	}
}
