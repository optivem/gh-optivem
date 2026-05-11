package projectconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, Path), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Sample configs (mirror the four canonical samples in the plan)
// ---------------------------------------------------------------------------

const sampleMonoRepoMonolith = `project:
  url: https://github.com/orgs/optivem/projects/20

repo_strategy: mono-repo

system:
  architecture: monolith
  path: system/monolith/java
  repo: optivem/shop
  lang: java

system_test:
  path: system-test/java
  repo: optivem/shop
  lang: java

external_systems:
  stubs:
    path: external-stub
    repo: optivem/shop
  simulators:
    path: external-real-sim
    repo: optivem/shop
`

const sampleMonoRepoMultitier = `project:
  url: https://github.com/orgs/optivem/projects/20

repo_strategy: mono-repo

system:
  architecture: multitier
  backend:
    path: system/multitier/backend-java
    repo: optivem/shop
    lang: java
  frontend:
    path: system/multitier/frontend-react
    repo: optivem/shop
    lang: typescript

system_test:
  path: system-test/java
  repo: optivem/shop
  lang: java

external_systems:
  stubs:
    path: external-stub
    repo: optivem/shop
  simulators:
    path: external-real-sim
    repo: optivem/shop
`

const sampleMultiRepoMonolith = `project:
  url: https://github.com/orgs/optivem/projects/20

repo_strategy: multi-repo

system:
  architecture: monolith
  path: .
  repo: optivem/shop
  lang: java

system_test:
  path: system-test
  repo: optivem/shop
  lang: java

external_systems:
  stubs:
    path: external-stub
    repo: optivem/shop
  simulators:
    path: external-real-sim
    repo: optivem/shop
`

const sampleMultiRepoMultitier = `project:
  url: https://github.com/orgs/optivem/projects/20

repo_strategy: multi-repo

system:
  architecture: multitier
  backend:
    path: .
    repo: optivem/shop-backend
    lang: java
  frontend:
    path: .
    repo: optivem/shop-frontend
    lang: typescript

system_test:
  path: system-test
  repo: optivem/shop-main
  lang: java

external_systems:
  stubs:
    path: external-stub
    repo: optivem/shop-main
  simulators:
    path: external-real-sim
    repo: optivem/shop-main
`

// ---------------------------------------------------------------------------
// Load — basic shape and missing-file behavior
// ---------------------------------------------------------------------------

func TestLoad_MissingFileReturnsNil(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config for missing file, got %+v", cfg)
	}
}

func TestLoad_ParsesProjectURL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, `project:
  url: https://github.com/orgs/optivem/projects/20
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Project.URL != "https://github.com/orgs/optivem/projects/20" {
		t.Fatalf("project URL: got %q", cfg.Project.URL)
	}
}

func TestLoad_EmptyFileIsValid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, "")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("empty file should yield non-nil zero-value config")
	}
	if cfg.Project.URL != "" {
		t.Fatalf("expected empty project URL, got %q", cfg.Project.URL)
	}
}

func TestLoad_MalformedYAMLErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, "project: [not, a, map\n")
	if _, err := Load(dir); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestLoad_EmptyRepoPathErrors(t *testing.T) {
	t.Parallel()
	if _, err := Load(""); err == nil {
		t.Fatal("expected error for empty repoPath, got nil")
	}
}

// ---------------------------------------------------------------------------
// Sample configs round-trip and validate cleanly
// ---------------------------------------------------------------------------

func TestLoad_AllFourSamplesValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
	}{
		{"mono-repo+monolith", sampleMonoRepoMonolith},
		{"mono-repo+multitier", sampleMonoRepoMultitier},
		{"multi-repo+monolith", sampleMultiRepoMonolith},
		{"multi-repo+multitier", sampleMultiRepoMultitier},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, tc.body)
			if _, err := Load(dir); err != nil {
				t.Fatalf("sample %s should validate, got: %v", tc.name, err)
			}
		})
	}
}

func TestWrite_RoundTripPreservesAllFourSamples(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
	}{
		{"mono-repo+monolith", sampleMonoRepoMonolith},
		{"mono-repo+multitier", sampleMonoRepoMultitier},
		{"multi-repo+monolith", sampleMultiRepoMonolith},
		{"multi-repo+multitier", sampleMultiRepoMultitier},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, tc.body)
			cfg, err := Load(dir)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			// Write to a fresh dir and re-Load — the value should
			// survive a round trip.
			out := t.TempDir()
			if err := Write(out, cfg); err != nil {
				t.Fatalf("Write: %v", err)
			}
			got, err := Load(out)
			if err != nil {
				t.Fatalf("Load after Write: %v", err)
			}
			if got.Project.URL != cfg.Project.URL ||
				got.RepoStrategy != cfg.RepoStrategy ||
				got.System.Architecture != cfg.System.Architecture ||
				got.System.Path != cfg.System.Path ||
				got.System.Repo != cfg.System.Repo ||
				got.System.Lang != cfg.System.Lang ||
				got.System.Backend != cfg.System.Backend ||
				got.System.Frontend != cfg.System.Frontend ||
				got.SystemTest != cfg.SystemTest ||
				got.ExternalSystems != cfg.ExternalSystems {
				t.Fatalf("round-trip mismatch:\n got:  %+v\n want: %+v", got, cfg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Validation rules
// ---------------------------------------------------------------------------

func TestValidate_AbsenceIsOK(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("zero-value config should validate, got: %v", err)
	}
}

func TestValidate_NilReceiverIsOK(t *testing.T) {
	t.Parallel()
	var cfg *Config
	if err := cfg.Validate(); err != nil {
		t.Fatalf("nil receiver should validate, got: %v", err)
	}
}

func TestValidate_UnknownRepoStrategyErrors(t *testing.T) {
	t.Parallel()
	cfg := &Config{RepoStrategy: "poly-repo"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for unknown repo_strategy, got nil")
	}
}

func TestValidate_UnknownArchitectureErrors(t *testing.T) {
	t.Parallel()
	cfg := &Config{System: System{Architecture: "hexagonal"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for unknown architecture, got nil")
	}
}

func TestValidate_UnknownLangErrors(t *testing.T) {
	t.Parallel()
	cfg := &Config{System: System{Lang: "rust"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for unknown lang, got nil")
	}
}

func TestValidate_RejectsReactAsLang(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{
			Architecture: ArchMultitier,
			Backend:      TierSpec{Path: "be", Repo: "x/y", Lang: LangJava},
			Frontend:     TierSpec{Path: "fe", Repo: "x/y", Lang: "react"},
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for lang=react, got nil")
	}
	if !strings.Contains(err.Error(), "react") {
		t.Fatalf("error should mention 'react', got: %v", err)
	}
}

func TestValidate_RejectsAbsolutePath(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{Architecture: ArchMonolith, Path: "/abs/path", Repo: "x/y", Lang: LangJava},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for absolute path, got nil")
	}
}

func TestValidate_RejectsDotDotPath(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{Architecture: ArchMonolith, Path: "../foo", Repo: "x/y", Lang: LangJava},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for ..-prefixed path, got nil")
	}
}

func TestValidate_RejectsEmbeddedDotDotPath(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{Architecture: ArchMonolith, Path: "foo/../bar", Repo: "x/y", Lang: LangJava},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for embedded .. segment, got nil")
	}
}

// Architecture exclusivity.

func TestValidate_MonolithRejectsBackend(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/y", Lang: LangJava,
			Backend: TierSpec{Path: "be", Repo: "x/y", Lang: LangJava},
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for monolith with backend present, got nil")
	}
}

func TestValidate_MonolithRejectsFrontend(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/y", Lang: LangJava,
			Frontend: TierSpec{Path: "fe", Repo: "x/y", Lang: LangTypescript},
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for monolith with frontend present, got nil")
	}
}

func TestValidate_MultitierRejectsSystemPath(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{
			Architecture: ArchMultitier,
			Path:         "should-not-be-here",
			Backend:      TierSpec{Path: "be", Repo: "x/y", Lang: LangJava},
			Frontend:     TierSpec{Path: "fe", Repo: "x/y", Lang: LangTypescript},
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for multitier with system.path set, got nil")
	}
}

func TestValidate_MultitierRequiresBackendAndFrontend(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{
			Architecture: ArchMultitier,
			Backend:      TierSpec{Path: "be", Repo: "x/y", Lang: LangJava},
			// Frontend missing.
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for multitier without frontend, got nil")
	}
}

// Tier completeness.

func TestValidate_RejectsTierWithMissingLang(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/y", Lang: LangJava,
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y" /* lang missing */},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for system_test missing lang, got nil")
	}
}

func TestValidate_RejectsTierWithMissingRepo(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/y", Lang: LangJava,
		},
		SystemTest: TierSpec{Path: "t", Lang: LangJava /* repo missing */},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for system_test missing repo, got nil")
	}
}

func TestValidate_RequiresSystemTestWhenArchitectureSet(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/y", Lang: LangJava,
		},
		// SystemTest empty.
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing system_test, got nil")
	}
}

// Repo-strategy consistency.

func TestValidate_MonoRepoRejectsMultipleRepos(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		RepoStrategy: RepoStrategyMonoRepo,
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/a", Lang: LangJava,
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/b" /* different! */, Lang: LangJava},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for mono-repo with two distinct repos, got nil")
	}
}

func TestValidate_MultiRepoRejectsAllEmptyRepos(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		RepoStrategy: RepoStrategyMultiRepo,
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", /* repo missing — caught by tier rule first */
			Lang: LangJava,
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error somewhere in the chain, got nil")
	}
}

// External systems.

func TestValidate_AcceptsExternalSystemsOmitted(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/y", Lang: LangJava,
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config without external_systems should validate, got: %v", err)
	}
}

func TestValidate_AcceptsOnlyStubsOrOnlySimulators(t *testing.T) {
	t.Parallel()
	base := func() *Config {
		return &Config{
			System: System{
				Architecture: ArchMonolith,
				Path:         "p", Repo: "x/y", Lang: LangJava,
			},
			SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
		}
	}

	// Only stubs.
	c := base()
	c.ExternalSystems.Stubs = ExternalSpec{Path: "external-stub", Repo: "x/y"}
	if err := c.Validate(); err != nil {
		t.Errorf("only stubs should validate, got: %v", err)
	}

	// Only simulators.
	c = base()
	c.ExternalSystems.Simulators = ExternalSpec{Path: "external-real-sim", Repo: "x/y"}
	if err := c.Validate(); err != nil {
		t.Errorf("only simulators should validate, got: %v", err)
	}
}

func TestValidate_RejectsExternalWithMissingRepo(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/y", Lang: LangJava,
		},
		SystemTest:      TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
		ExternalSystems: ExternalSystems{Stubs: ExternalSpec{Path: "external-stub" /* repo missing */}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for external stubs missing repo, got nil")
	}
}

func TestValidate_AcceptsExternalRepoNotInOtherTiers(t *testing.T) {
	t.Parallel()
	// External systems can live in their own repo (multi-repo case).
	cfg := &Config{
		RepoStrategy: RepoStrategyMultiRepo,
		System: System{
			Architecture: ArchMultitier,
			Backend:      TierSpec{Path: "be", Repo: "x/backend", Lang: LangJava},
			Frontend:     TierSpec{Path: "fe", Repo: "x/frontend", Lang: LangTypescript},
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/main", Lang: LangJava},
		ExternalSystems: ExternalSystems{
			Stubs:      ExternalSpec{Path: "external-stub", Repo: "x/externals" /* unique slug */},
			Simulators: ExternalSpec{Path: "external-real-sim", Repo: "x/externals"},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected validate-ok, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Repos() helper
// ---------------------------------------------------------------------------

func TestRepos_UnionAcrossTiers(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{
			Architecture: ArchMultitier,
			Backend:      TierSpec{Path: "be", Repo: "x/backend", Lang: LangJava},
			Frontend:     TierSpec{Path: "fe", Repo: "x/frontend", Lang: LangTypescript},
		},
		SystemTest:      TierSpec{Path: "t", Repo: "x/backend", Lang: LangJava},
		ExternalSystems: ExternalSystems{Stubs: ExternalSpec{Path: "stub", Repo: "x/main"}},
	}
	got := cfg.Repos()
	want := []string{"x/backend", "x/frontend", "x/main"}
	if len(got) != len(want) {
		t.Fatalf("repos: got %v, want %v", got, want)
	}
	for i, r := range want {
		if got[i] != r {
			t.Errorf("repos[%d]: got %q, want %q", i, got[i], r)
		}
	}
}

func TestRepos_NilReceiver(t *testing.T) {
	t.Parallel()
	var cfg *Config
	if got := cfg.Repos(); got != nil {
		t.Errorf("nil cfg.Repos() should be nil, got %v", got)
	}
}

func TestRepos_DeduplicatesSameRepo(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/y", Lang: LangJava,
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	got := cfg.Repos()
	if len(got) != 1 || got[0] != "x/y" {
		t.Errorf("expected dedup'd [x/y], got %v", got)
	}
}

// ---------------------------------------------------------------------------
// LoadFromPath / Write surface
// ---------------------------------------------------------------------------

func TestLoadFromPath_MissingFileErrors(t *testing.T) {
	t.Parallel()
	if _, err := LoadFromPath(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error for missing file via LoadFromPath, got nil")
	}
}

func TestLoadFromPath_EmptyPathErrors(t *testing.T) {
	t.Parallel()
	if _, err := LoadFromPath(""); err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}

func TestWrite_OmitsEmptyOptionalFields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := &Config{
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/y", Lang: LangJava,
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	if err := Write(dir, cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, Path))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(data)
	if strings.Contains(body, "external_systems:") {
		t.Errorf("expected external_systems to be omitted; got:\n%s", body)
	}
	if strings.Contains(body, "url:") {
		t.Errorf("expected empty project.url to be omitted; got:\n%s", body)
	}
	if !strings.Contains(body, "architecture: monolith") {
		t.Errorf("expected architecture line; got:\n%s", body)
	}
}

func TestWrite_RejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := &Config{System: System{Architecture: ArchMultitier /* missing backend/frontend */}}
	if err := Write(dir, cfg); err == nil {
		t.Fatal("expected validation error for incomplete multitier config, got nil")
	}
}

func TestWrite_NilCfgErrors(t *testing.T) {
	t.Parallel()
	if err := Write(t.TempDir(), nil); err == nil {
		t.Fatal("expected error for nil cfg, got nil")
	}
}

func TestWrite_EmptyRepoPathErrors(t *testing.T) {
	t.Parallel()
	if err := Write("", &Config{}); err == nil {
		t.Fatal("expected error for empty repoPath, got nil")
	}
}

// ---------------------------------------------------------------------------
// ResolvePath
// ---------------------------------------------------------------------------

// TestResolvePath_FlagBeatsEnvAndDefault verifies the flag value wins over
// $GH_OPTIVEM_CONFIG and the cwd default.
func TestResolvePath_FlagBeatsEnvAndDefault(t *testing.T) {
	// Not parallel — mutates env.
	t.Setenv(EnvVar, "/env/from-env.yaml")
	path, explicit := ResolvePath("/flag/explicit.yaml")
	if path != "/flag/explicit.yaml" {
		t.Errorf("path: got %q, want /flag/explicit.yaml", path)
	}
	if !explicit {
		t.Error("explicit: got false, want true")
	}
}

// TestResolvePath_EnvUsedWhenFlagEmpty verifies $GH_OPTIVEM_CONFIG is the
// second-tier source when --config / -c wasn't passed.
func TestResolvePath_EnvUsedWhenFlagEmpty(t *testing.T) {
	t.Setenv(EnvVar, "/env/from-env.yaml")
	path, explicit := ResolvePath("")
	if path != "/env/from-env.yaml" {
		t.Errorf("path: got %q, want /env/from-env.yaml", path)
	}
	if !explicit {
		t.Error("explicit: got false, want true (env counts as explicit)")
	}
}

// TestResolvePath_DefaultFallsBackToCwd verifies the default branch joins
// CWD with the canonical Path constant and reports explicit=false.
func TestResolvePath_DefaultFallsBackToCwd(t *testing.T) {
	t.Setenv(EnvVar, "")
	path, explicit := ResolvePath("")
	cwd, _ := os.Getwd()
	want := filepath.Join(cwd, Path)
	if path != want {
		t.Errorf("path: got %q, want %q", path, want)
	}
	if explicit {
		t.Error("explicit: got true, want false (cwd default isn't explicit)")
	}
}

// ---------------------------------------------------------------------------
// WriteToPath
// ---------------------------------------------------------------------------

// TestWriteToPath_NonCanonicalFilename round-trips through a non-canonical
// filename (used when --config points at a non-default file).
func TestWriteToPath_NonCanonicalFilename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "gh-optivem.alt.yaml")
	in := &Config{
		Project:      Project{URL: "https://github.com/orgs/acme/projects/7"},
		RepoStrategy: RepoStrategyMonoRepo,
		System: System{
			Architecture: ArchMonolith,
			Path:         "system/monolith/java",
			Repo:         "acme/page-turner",
			Lang:         LangJava,
		},
		SystemTest: TierSpec{
			Path: "system-test/java",
			Repo: "acme/page-turner",
			Lang: LangJava,
		},
	}
	if err := WriteToPath(yamlPath, in); err != nil {
		t.Fatalf("WriteToPath: %v", err)
	}
	out, err := LoadFromPath(yamlPath)
	if err != nil {
		t.Fatalf("LoadFromPath: %v", err)
	}
	if out.Project.URL != in.Project.URL {
		t.Errorf("project.url mismatch after round-trip")
	}
	if out.System.Lang != in.System.Lang {
		t.Errorf("system.lang mismatch after round-trip")
	}
}

func TestWriteToPath_EmptyPathErrors(t *testing.T) {
	t.Parallel()
	if err := WriteToPath("", &Config{}); err == nil {
		t.Fatal("expected error for empty yamlPath, got nil")
	}
}
