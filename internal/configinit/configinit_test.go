package configinit

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// monolithRawFlags returns the minimum RawFlags ValidateAndDeriveForYAML
// accepts for a mono-repo Java monolith. Mirrors the values monolithAnswers
// produces via Prompt — keeps the BuildConfig test independent of the
// interactive surface while exercising the same validation path.
func monolithRawFlags() *config.RawFlags {
	return &config.RawFlags{
		Owner:        "acme",
		Repo:         "page-turner",
		SystemName:   "Page Turner",
		Arch:         "monolith",
		RepoStrategy: "monorepo",
		Lang:         "java",
		TestLang:     "java",
		ProjectURL:   "https://github.com/orgs/acme/projects/1",
		License:      projectconfig.LicenseMIT,
		Deploy:       projectconfig.DeployDocker,
	}
}

// TestBuildConfig_HappyPath — BuildConfig returns a fully-populated
// *projectconfig.Config equivalent to what runWithBanner would have
// written to disk. Single shared validation chain (config.ValidateAndDeriveForYAML
// → steps.BuildOptivemYAML) so the in-memory and on-disk surfaces can
// not drift.
func TestBuildConfig_HappyPath(t *testing.T) {
	t.Parallel()
	stubExistenceChecks(t, nil, nil)
	pc, err := BuildConfig(monolithRawFlags())
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if pc == nil {
		t.Fatal("want non-nil *projectconfig.Config")
	}
	if pc.SystemName != "Page Turner" {
		t.Errorf("SystemName: got %q", pc.SystemName)
	}
	if pc.System.Architecture != "monolith" {
		t.Errorf("Architecture: got %q", pc.System.Architecture)
	}
	if pc.RepoStrategy != projectconfig.RepoStrategyMonoRepo {
		t.Errorf("RepoStrategy: got %q", pc.RepoStrategy)
	}
	if pc.Project.URL != "https://github.com/orgs/acme/projects/1" {
		t.Errorf("Project.URL: got %q", pc.Project.URL)
	}
	// Round-trip: the in-memory config must satisfy Validate, i.e. it is
	// the same shape WriteToPath / LoadFromPath would accept on disk.
	if err := pc.Validate(); err != nil {
		t.Errorf("built config fails Validate: %v", err)
	}
}

// TestBuildConfig_ValidationError — a RawFlags value that
// ValidateAndDeriveForYAML rejects propagates the error unchanged. Confirms
// BuildConfig does not swallow validation failures or fall back to a
// partial config.
func TestBuildConfig_ValidationError(t *testing.T) {
	t.Parallel()
	stubExistenceChecks(t, nil, nil)
	f := monolithRawFlags()
	f.Arch = "" // required
	pc, err := BuildConfig(f)
	if err == nil {
		t.Fatal("want validation error, got nil")
	}
	if pc != nil {
		t.Errorf("want nil pc on validation error, got %+v", pc)
	}
	if !strings.Contains(err.Error(), "required flags") {
		t.Errorf("error should mention required flags, got: %v", err)
	}
}
