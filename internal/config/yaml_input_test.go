package config

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// validMonolithYAML returns a fully-populated projectconfig.Config matching
// what `gh optivem config init` writes for a monolith mono-repo run.
// Tests mutate a copy to drive FillRawFlagsFromYAML through its branches.
func validMonolithYAML() *projectconfig.Config {
	return &projectconfig.Config{
		Project:      projectconfig.Project{URL: "https://github.com/orgs/acme/projects/1"},
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
		ExternalSystems: projectconfig.ExternalSystems{
			Stubs:      projectconfig.ExternalSpec{Path: "external-stub", Repo: "acme/page-turner"},
			Simulators: projectconfig.ExternalSpec{Path: "external-real-sim", Repo: "acme/page-turner"},
		},
	}
}

func validMultitierMultirepoYAML() *projectconfig.Config {
	return &projectconfig.Config{
		Project:      projectconfig.Project{URL: "https://github.com/orgs/acme/projects/2"},
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
		ExternalSystems: projectconfig.ExternalSystems{
			Stubs:      projectconfig.ExternalSpec{Path: "external-stub", Repo: "acme/page-turner-backend"},
			Simulators: projectconfig.ExternalSpec{Path: "external-real-sim", Repo: "acme/page-turner-backend"},
		},
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
		t.Errorf("system_name: got %q", f.SystemName)
	}
	if f.Arch != "monolith" {
		t.Errorf("arch: got %q (must use init's spelling, not yaml's)", f.Arch)
	}
	if f.RepoStrategy != "monorepo" {
		t.Errorf("repo_strategy: got %q (must use init's spelling, not yaml's)", f.RepoStrategy)
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
		{"system_name", func(pc *projectconfig.Config) { pc.SystemName = "" }, "system_name"},
		{"architecture", func(pc *projectconfig.Config) { pc.System.Architecture = "" }, "system.architecture"},
		{"project.url", func(pc *projectconfig.Config) { pc.Project.URL = "" }, "project.url"},
		{"repo_strategy", func(pc *projectconfig.Config) { pc.RepoStrategy = "" }, "repo_strategy"},
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
