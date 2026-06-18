package config

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// validMonolithYAML returns a fully-populated projectconfig.Config matching
// what `gh optivem config init` writes for a monolith mono-repo run.
// Tests mutate a copy to drive FillRawFlagsFromYAML through its branches.
func validMonolithYAML() *projectconfig.Config {
	return &projectconfig.Config{
		Project:      projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
		RepoStrategy: projectconfig.RepoStrategyMonoRepo,
		SystemName:   "Page Turner",
		License:      projectconfig.LicenseMIT,
		Deploy:       projectconfig.DeployDocker,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMonolith,
			Path:         "system",
			Repo:         "acme/page-turner",
			Lang:         projectconfig.LangJava,
		},
		SystemTest: projectconfig.TierSpec{Path: "system-test", Repo: "acme/page-turner", Lang: projectconfig.LangJava},
		// external-systems is operator-owned and not scaffolded by init
		// (plan 20260606-1356), so FillRawFlagsFromYAML never reads it.
	}
}

func validMultitierMultirepoYAML() *projectconfig.Config {
	return &projectconfig.Config{
		Project:      projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/acme/projects/2"},
		RepoStrategy: projectconfig.RepoStrategyMultiRepo,
		SystemName:   "Page Turner",
		License:      projectconfig.LicenseApache2,
		Deploy:       projectconfig.DeployDocker,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMultitier,
			Backend:      projectconfig.TierSpec{Path: "backend", Repo: "acme/page-turner-backend", Lang: projectconfig.LangDotnet},
			Frontend:     projectconfig.TierSpec{Path: "frontend", Repo: "acme/page-turner-frontend", Lang: projectconfig.LangTypescript},
		},
		SystemTest: projectconfig.TierSpec{Path: "system-test", Repo: "acme/page-turner-backend", Lang: projectconfig.LangTypescript},
	}
}

// validMicroservicesMonorepoYAML returns the heterogeneous mono-repo
// microservices shape from the plan: two backend services (java + dotnet)
// sharing one repo, plus the single typescript frontend.
func validMicroservicesMonorepoYAML() *projectconfig.Config {
	return &projectconfig.Config{
		Project:      projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/optivem/projects/20"},
		RepoStrategy: projectconfig.RepoStrategyMonoRepo,
		SystemName:   "Shop",
		License:      projectconfig.LicenseMIT,
		Deploy:       projectconfig.DeployDocker,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMicroservices,
			BackendServices: map[string]projectconfig.TierSpec{
				"orders":    {Path: "system/microservices/orders-java", Repo: "optivem/shop", Lang: projectconfig.LangJava, SonarProject: "optivem_shop-orders"},
				"inventory": {Path: "system/microservices/inventory-dotnet", Repo: "optivem/shop", Lang: projectconfig.LangDotnet, SonarProject: "optivem_shop-inventory"},
			},
			Frontend: projectconfig.TierSpec{Path: "system/microservices/frontend-react", Repo: "optivem/shop", Lang: projectconfig.LangTypescript, SonarProject: "optivem_shop-frontend"},
		},
		SystemTest: projectconfig.TierSpec{Path: "system-test/java", Repo: "optivem/shop", Lang: projectconfig.LangJava},
	}
}

func TestFillRawFlagsFromYAML_MicroservicesMonorepo(t *testing.T) {
	t.Parallel()
	f := &RawFlags{}
	if err := FillRawFlagsFromYAML(f, validMicroservicesMonorepoYAML()); err != nil {
		t.Fatalf("FillRawFlagsFromYAML: %v", err)
	}
	if f.Arch != "microservices" {
		t.Errorf("arch: got %q, want microservices", f.Arch)
	}
	// Mono-repo: owner/repo derived from the single frontend slug optivem/shop.
	if f.Owner != "optivem" || f.Repo != "shop" {
		t.Errorf("owner/repo: got %q/%q, want optivem/shop", f.Owner, f.Repo)
	}
	// backend-services populated in sorted-name order (inventory < orders).
	if len(f.BackendServices) != 2 {
		t.Fatalf("BackendServices: got %d, want 2", len(f.BackendServices))
	}
	if f.BackendServices[0].Name != "inventory" || f.BackendServices[1].Name != "orders" {
		t.Errorf("service order: got %q,%q want inventory,orders", f.BackendServices[0].Name, f.BackendServices[1].Name)
	}
	if f.BackendServices[0].Lang != "dotnet" || f.BackendServices[1].Lang != "java" {
		t.Errorf("service langs: got %q,%q", f.BackendServices[0].Lang, f.BackendServices[1].Lang)
	}
	if f.BackendServices[1].Path != "system/microservices/orders-java" || f.BackendServices[1].SonarProject != "optivem_shop-orders" {
		t.Errorf("orders service mismatch: %+v", f.BackendServices[1])
	}
	if f.FrontendLang != "typescript" || f.FrontendPath != "system/microservices/frontend-react" {
		t.Errorf("frontend: got lang=%q path=%q", f.FrontendLang, f.FrontendPath)
	}
	if f.FrontendRepoSlug != "optivem/shop" {
		t.Errorf("FrontendRepoSlug: got %q", f.FrontendRepoSlug)
	}
}

func TestFillRawFlagsFromYAML_MicroservicesMultirepoStripsFrontendSuffix(t *testing.T) {
	t.Parallel()
	pc := validMicroservicesMonorepoYAML()
	pc.RepoStrategy = projectconfig.RepoStrategyMultiRepo
	pc.System.BackendServices = map[string]projectconfig.TierSpec{
		"orders":    {Path: ".", Repo: "optivem/shop-orders", Lang: projectconfig.LangJava, SonarProject: "optivem_shop-orders"},
		"inventory": {Path: ".", Repo: "optivem/shop-inventory", Lang: projectconfig.LangDotnet, SonarProject: "optivem_shop-inventory"},
	}
	pc.System.Frontend = projectconfig.TierSpec{Path: ".", Repo: "optivem/shop-frontend", Lang: projectconfig.LangTypescript, SonarProject: "optivem_shop-frontend"}
	pc.SystemTest.Repo = "optivem/shop-orders"

	f := &RawFlags{}
	if err := FillRawFlagsFromYAML(f, pc); err != nil {
		t.Fatalf("FillRawFlagsFromYAML: %v", err)
	}
	// Multi-repo: owner/repo derived from the frontend slug, stripping
	// the "-frontend" suffix → optivem/shop.
	if f.Owner != "optivem" || f.Repo != "shop" {
		t.Errorf("owner/repo: got %q/%q, want optivem/shop (stripped -frontend)", f.Owner, f.Repo)
	}
	if f.BackendServices[1].Repo != "optivem/shop-orders" {
		t.Errorf("orders repo: got %q", f.BackendServices[1].Repo)
	}
}

func TestFillRawFlagsFromYAML_MonolithMonorepo(t *testing.T) {
	t.Parallel()
	f := &RawFlags{}
	if err := FillRawFlagsFromYAML(f, validMonolithYAML()); err != nil {
		t.Fatalf("FillRawFlagsFromYAML: %v", err)
	}
	if f.Owner != "acme" || f.Repo != "page-turner" {
		t.Errorf("owner/repo: got %q/%q, want acme/page-turner", f.Owner, f.Repo)
	}
	if f.SystemName != "Page Turner" {
		t.Errorf("system-name: got %q", f.SystemName)
	}
	if f.Arch != "monolith" {
		t.Errorf("arch: got %q (must use init's spelling, not yaml's)", f.Arch)
	}
	if f.RepoStrategy != "monorepo" {
		t.Errorf("repo-strategy: got %q (must use init's spelling, not yaml's)", f.RepoStrategy)
	}
	if f.Lang != "java" {
		t.Errorf("lang: got %q", f.Lang)
	}
	if f.SystemPath != "system" || f.SystemTestPath != "system-test" {
		t.Errorf("paths: got system=%q system-test=%q", f.SystemPath, f.SystemTestPath)
	}
	if f.License != projectconfig.LicenseMIT || f.Deploy != projectconfig.DeployDocker {
		t.Errorf("license/deploy: got %q/%q", f.License, f.Deploy)
	}
}

func TestFillRawFlagsFromYAML_MultitierMultirepoStripsSuffix(t *testing.T) {
	t.Parallel()
	f := &RawFlags{}
	if err := FillRawFlagsFromYAML(f, validMultitierMultirepoYAML()); err != nil {
		t.Fatalf("FillRawFlagsFromYAML: %v", err)
	}
	// system.backend.repo is "acme/page-turner-backend"; the workspace
	// base name strips the "-backend" suffix.
	if f.Owner != "acme" || f.Repo != "page-turner" {
		t.Errorf("owner/repo: got %q/%q, want acme/page-turner (stripped -backend)", f.Owner, f.Repo)
	}
	if f.Arch != "multitier" || f.RepoStrategy != "multirepo" {
		t.Errorf("arch/strategy: got %q/%q", f.Arch, f.RepoStrategy)
	}
	if f.BackendLang != "dotnet" || f.FrontendLang != "typescript" {
		t.Errorf("langs: got backend=%q frontend=%q", f.BackendLang, f.FrontendLang)
	}
	if f.BackendPath != "backend" || f.FrontendPath != "frontend" {
		t.Errorf("paths: got backend=%q frontend=%q", f.BackendPath, f.FrontendPath)
	}
}

func TestFillRawFlagsFromYAML_DefaultsAbsentLicenseAndDeploy(t *testing.T) {
	t.Parallel()
	pc := validMonolithYAML()
	pc.License = ""
	pc.Deploy = ""
	f := &RawFlags{}
	if err := FillRawFlagsFromYAML(f, pc); err != nil {
		t.Fatalf("FillRawFlagsFromYAML: %v", err)
	}
	if f.License != projectconfig.LicenseMIT {
		t.Errorf("absent license should default to mit, got %q", f.License)
	}
	if f.Deploy != projectconfig.DeployDocker {
		t.Errorf("absent deploy should default to docker, got %q", f.Deploy)
	}
}

// TestFillRawFlagsFromYAML_AcceptsEmptyProjectURL pins the contract that
// yaml-load tolerates an absent project.url. Path A in
// internal/steps/project.go (EnsureProjectBoard) auto-creates the board
// on first `init` run and rewrites gh-optivem.yaml with the resulting
// URL, so the empty value is a legitimate intermediate state.
func TestFillRawFlagsFromYAML_AcceptsEmptyProjectURL(t *testing.T) {
	t.Parallel()
	pc := validMonolithYAML()
	pc.Project.URL = ""
	f := &RawFlags{}
	if err := FillRawFlagsFromYAML(f, pc); err != nil {
		t.Fatalf("empty project.url should load (Path A auto-creates); got: %v", err)
	}
	if f.ProjectURL != "" {
		t.Errorf("ProjectURL should propagate as empty, got %q", f.ProjectURL)
	}
}

func TestFillRawFlagsFromYAML_NilErrorsWithInitHint(t *testing.T) {
	t.Parallel()
	f := &RawFlags{}
	err := FillRawFlagsFromYAML(f, nil)
	if err == nil {
		t.Fatal("want error for nil yaml, got nil")
	}
	if !strings.Contains(err.Error(), "gh optivem config init") {
		t.Errorf("error should point at config init, got: %v", err)
	}
}

func TestFillRawFlagsFromYAML_RejectsMissingRequired(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		mutate   func(*projectconfig.Config)
		wantSubs string
	}{
		{"system-name", func(pc *projectconfig.Config) { pc.SystemName = "" }, "system-name"},
		{"architecture", func(pc *projectconfig.Config) { pc.System.Architecture = "" }, "system.architecture"},
		{"repo-strategy", func(pc *projectconfig.Config) { pc.RepoStrategy = "" }, "repo-strategy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pc := validMonolithYAML()
			tc.mutate(pc)
			f := &RawFlags{}
			err := FillRawFlagsFromYAML(f, pc)
			if err == nil {
				t.Fatalf("want error for missing %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubs) {
				t.Errorf("error should mention %q, got: %v", tc.wantSubs, err)
			}
		})
	}
}

func TestFillRawFlagsFromYAML_RejectsMalformedTierSlug(t *testing.T) {
	t.Parallel()
	pc := validMonolithYAML()
	pc.System.Repo = "" // empty triggers the missing-field branch
	f := &RawFlags{}
	err := FillRawFlagsFromYAML(f, pc)
	if err == nil {
		t.Fatal("want error for empty system.repo, got nil")
	}
}

func TestFillRawFlagsFromYAML_RejectsMultirepoSlugMissingSuffix(t *testing.T) {
	t.Parallel()
	// Multitier multirepo expects backend slug to end in "-backend";
	// a hand-edited slug without the suffix is a hard error.
	pc := validMultitierMultirepoYAML()
	pc.System.Backend.Repo = "acme/oddly-named-repo"
	f := &RawFlags{}
	err := FillRawFlagsFromYAML(f, pc)
	if err == nil {
		t.Fatal("want error for missing -backend suffix, got nil")
	}
	if !strings.Contains(err.Error(), "-backend") {
		t.Errorf("error should mention the expected suffix, got: %v", err)
	}
}
