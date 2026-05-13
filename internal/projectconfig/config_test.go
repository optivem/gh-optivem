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

sonar:
  organization: optivem

system:
  architecture: multitier
  backend:
    path: system/multitier/backend-java
    repo: optivem/shop
    lang: java
    sonar_project: optivem_shop-backend
  frontend:
    path: system/multitier/frontend-react
    repo: optivem/shop
    lang: typescript
    sonar_project: optivem_shop-frontend

system_test:
  path: system-test/java
  repo: optivem/shop
  lang: java
  sonar_project: optivem_shop-system-test

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

sonar:
  organization: optivem

system:
  architecture: monolith
  path: .
  repo: optivem/shop
  lang: java
  sonar_project: optivem_shop-system

system_test:
  path: system-test
  repo: optivem/shop
  lang: java
  sonar_project: optivem_shop-system-test

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
  repo: optivem/shop-backend
  lang: java
  sonar_project: optivem_shop-system-test

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

// TestLoad_EmptyFileLoads pins the contract that an empty gh-optivem.yaml
// loads to a zero-value config without error: Validate accepts empty
// values across the board, and project.url is no longer a hard
// requirement at validate-time (auto-create happens in `gh optivem init`
// Path A). Specific consumers (FillRawFlagsFromYAML, ATDD runtime) still
// enforce the fields they actually need.
func TestLoad_EmptyFileLoads(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, "")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("empty file should load to a zero-value config, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config for empty file")
	}
	if cfg.Project.URL != "" {
		t.Errorf("project.url on empty config: want empty, got %q", cfg.Project.URL)
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

// TestRoundTrip_PreservesProcessFlowAndOverrides verifies that the optional
// process_flow: / agent_prompts: / node_extras: / node_replacements: fields
// survive a Write→Load round-trip when set.
func TestRoundTrip_PreservesProcessFlowAndOverrides(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := &Config{
		Project:     Project{URL: "https://github.com/orgs/acme/projects/1"},
		ProcessFlow: "config/process-flow.yaml",
		AgentPrompts: map[string]string{
			"atdd-test": "config/prompts/atdd-test.md",
		},
		NodeExtras: map[string]string{
			"AT_RED_DSL_WRITE": "prefer record types",
		},
		NodeReplacements: map[string]string{
			"AT_RED_TEST_WRITE": "config/prompts/at-red-test-write.md",
		},
	}
	if err := Write(dir, cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ProcessFlow != cfg.ProcessFlow {
		t.Errorf("process_flow: got %q, want %q", got.ProcessFlow, cfg.ProcessFlow)
	}
	if got.AgentPrompts["atdd-test"] != cfg.AgentPrompts["atdd-test"] {
		t.Errorf("agent_prompts[atdd-test]: got %q, want %q",
			got.AgentPrompts["atdd-test"], cfg.AgentPrompts["atdd-test"])
	}
	if got.NodeExtras["AT_RED_DSL_WRITE"] != cfg.NodeExtras["AT_RED_DSL_WRITE"] {
		t.Errorf("node_extras: got %q", got.NodeExtras["AT_RED_DSL_WRITE"])
	}
	if got.NodeReplacements["AT_RED_TEST_WRITE"] != cfg.NodeReplacements["AT_RED_TEST_WRITE"] {
		t.Errorf("node_replacements: got %q", got.NodeReplacements["AT_RED_TEST_WRITE"])
	}
}

// TestValidate_ProcessFlow_RejectsAbsolutePath verifies path-validation
// kicks in for the new process_flow: field.
func TestValidate_ProcessFlow_RejectsAbsolutePath(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project:     Project{URL: "https://github.com/orgs/acme/projects/1"},
		ProcessFlow: "/abs/process-flow.yaml",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for absolute process_flow, got nil")
	}
	if !strings.Contains(err.Error(), "process_flow") {
		t.Fatalf("error should mention process_flow, got: %v", err)
	}
}

// TestValidate_AgentPrompts_RejectsUnknownAgent verifies typos in agent
// names surface at config-load, not deep inside the pipeline.
func TestValidate_AgentPrompts_RejectsUnknownAgent(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project: Project{URL: "https://github.com/orgs/acme/projects/1"},
		AgentPrompts: map[string]string{
			"atdd-not-a-real-agent": "config/prompts/x.md",
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unknown agent name, got nil")
	}
	if !strings.Contains(err.Error(), "atdd-not-a-real-agent") {
		t.Fatalf("error should name the bad agent, got: %v", err)
	}
}

// TestValidate_AgentPrompts_RejectsAbsolutePath verifies values pass the
// same path-validation as system/system_test paths.
func TestValidate_AgentPrompts_RejectsAbsolutePath(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project: Project{URL: "https://github.com/orgs/acme/projects/1"},
		AgentPrompts: map[string]string{
			"atdd-test": "/abs/prompts/atdd-test.md",
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for absolute agent_prompts path, got nil")
	}
}

// TestValidate_NodeReplacements_RejectsAbsolutePath verifies values pass
// the same path-validation as agent_prompts.
func TestValidate_NodeReplacements_RejectsAbsolutePath(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project: Project{URL: "https://github.com/orgs/acme/projects/1"},
		NodeReplacements: map[string]string{
			"AT_RED_TEST_WRITE": "/abs/prompts/x.md",
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for absolute node_replacements path, got nil")
	}
}

// TestValidate_RejectsSameKeyInExtrasAndReplacements verifies the
// "replace supersedes extras" rule: a node ID may not appear in both
// maps simultaneously.
func TestValidate_RejectsSameKeyInExtrasAndReplacements(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project:          Project{URL: "https://github.com/orgs/acme/projects/1"},
		NodeExtras:       map[string]string{"AT_RED_DSL_WRITE": "prefer records"},
		NodeReplacements: map[string]string{"AT_RED_DSL_WRITE": "config/prompts/x.md"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate node key, got nil")
	}
	if !strings.Contains(err.Error(), "AT_RED_DSL_WRITE") {
		t.Fatalf("error should name the duplicate node, got: %v", err)
	}
}

// TestValidate_AcceptsEmptyOverrideMaps confirms a config with all four
// override fields nil/empty validates cleanly (the common case).
func TestValidate_AcceptsEmptyOverrideMaps(t *testing.T) {
	t.Parallel()
	cfg := &Config{Project: Project{URL: "https://github.com/orgs/acme/projects/1"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("empty override maps should validate, got: %v", err)
	}
}

// TestRoundTrip_PreservesSystemAndTestConfig verifies that the optional
// system.config: / system_test.config: fields survive a Write→Load round-trip
// when set, and stay empty (and absent from the written YAML) when unset.
func TestRoundTrip_PreservesSystemAndTestConfig(t *testing.T) {
	t.Parallel()

	t.Run("set", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cfg := &Config{
			Project:    Project{URL: "https://github.com/orgs/acme/projects/1"},
			System:     System{Config: "docker/systems.json"},
			SystemTest: TierSpec{Config: "system-test/tests.json"},
		}
		if err := Write(dir, cfg); err != nil {
			t.Fatalf("Write: %v", err)
		}
		got, err := Load(dir)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.System.Config != cfg.System.Config {
			t.Errorf("system.config: got %q, want %q", got.System.Config, cfg.System.Config)
		}
		if got.SystemTest.Config != cfg.SystemTest.Config {
			t.Errorf("system_test.config: got %q, want %q", got.SystemTest.Config, cfg.SystemTest.Config)
		}
	})

	t.Run("unset omits the keys", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cfg := &Config{Project: Project{URL: "https://github.com/orgs/acme/projects/1"}}
		if err := Write(dir, cfg); err != nil {
			t.Fatalf("Write: %v", err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, Path))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		body := string(raw)
		if strings.Contains(body, "config:") {
			t.Errorf("unset config: should not appear in YAML, got:\n%s", body)
		}
		got, err := Load(dir)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.System.Config != "" || got.SystemTest.Config != "" {
			t.Errorf("zero-value round-trip got non-empty config fields: %+v", got)
		}
	})
}

// TestValidate_RejectsLegacyTopLevelConfigKeys verifies the pre-2026-05
// top-level system_config: / test_config: spellings produce a clear
// migration error rather than silently falling through to default paths.
func TestValidate_RejectsLegacyTopLevelConfigKeys(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		body     string
		wantHint string
	}{
		{
			name: "legacy system_config",
			body: `project:
  url: https://github.com/orgs/acme/projects/1
system_config: docker/systems.json
`,
			wantHint: "system.config",
		},
		{
			name: "legacy test_config",
			body: `project:
  url: https://github.com/orgs/acme/projects/1
test_config: system-test/tests.json
`,
			wantHint: "system_test.config",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeConfig(t, dir, c.body)
			_, err := Load(dir)
			if err == nil {
				t.Fatalf("expected migration error, got nil")
			}
			if !strings.Contains(err.Error(), c.wantHint) {
				t.Errorf("error should hint at %q, got: %v", c.wantHint, err)
			}
		})
	}
}

// TestValidate_RejectsConfigOnBackendOrFrontend verifies the Config field
// is rejected on system.backend / system.frontend (it's only meaningful on
// system_test). Catches typos like accidentally placing the tests.yaml path
// under a code tier.
func TestValidate_RejectsConfigOnBackendOrFrontend(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		cfg      *Config
		wantHint string
	}{
		{
			name: "backend.config",
			cfg: &Config{
				Project: Project{URL: "https://github.com/orgs/acme/projects/1"},
				System: System{
					Backend: TierSpec{Path: "x", Repo: "r", Lang: LangJava, Config: "nope"},
				},
			},
			wantHint: "system.backend.config",
		},
		{
			name: "frontend.config",
			cfg: &Config{
				Project: Project{URL: "https://github.com/orgs/acme/projects/1"},
				System: System{
					Frontend: TierSpec{Path: "x", Repo: "r", Lang: LangTypescript, Config: "nope"},
				},
			},
			wantHint: "system.frontend.config",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := c.cfg.Validate()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), c.wantHint) {
				t.Errorf("error should hint at %q, got: %v", c.wantHint, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Validation rules
// ---------------------------------------------------------------------------

// TestValidate_AcceptsEmptyProjectURL pins the contract that an empty
// project.url is valid at YAML-load time: `gh optivem init` Path A
// auto-creates the board on first run and rewrites the file with the
// URL. The ATDD runtime (internal/atdd/runtime/board) still enforces
// non-empty at use time.
func TestValidate_AcceptsEmptyProjectURL(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("zero-value config (empty project.url) should validate now that Path A auto-creates; got: %v", err)
	}
}

func TestValidate_NilReceiverIsOK(t *testing.T) {
	t.Parallel()
	var cfg *Config
	if err := cfg.Validate(); err != nil {
		t.Fatalf("nil receiver should validate, got: %v", err)
	}
}

// TestValidate_OnlyProjectURLIsOK verifies that a config carrying just a
// project.url (everything else empty) passes Validate. Matches the
// "partial config written before architecture is chosen" flow.
func TestValidate_OnlyProjectURLIsOK(t *testing.T) {
	t.Parallel()
	cfg := &Config{Project: Project{URL: "https://github.com/orgs/acme/projects/1"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("project-url-only config should validate, got: %v", err)
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
		Project: Project{URL: "https://github.com/orgs/acme/projects/1"},
		Sonar:   Sonar{Organization: "x"},
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/y", Lang: LangJava,
			SonarProject: "x_y-system",
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava, SonarProject: "x_y-system-test"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config without external_systems should validate, got: %v", err)
	}
}

func TestValidate_AcceptsOnlyStubsOrOnlySimulators(t *testing.T) {
	t.Parallel()
	base := func() *Config {
		return &Config{
			Project: Project{URL: "https://github.com/orgs/acme/projects/1"},
			Sonar:   Sonar{Organization: "x"},
			System: System{
				Architecture: ArchMonolith,
				Path:         "p", Repo: "x/y", Lang: LangJava,
				SonarProject: "x_y-system",
			},
			SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava, SonarProject: "x_y-system-test"},
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
	// system_test.repo carries the canonical base ("x/main"), so the Sonar
	// keys use base="main" — independent of where each component lives.
	cfg := &Config{
		Project:      Project{URL: "https://github.com/orgs/acme/projects/1"},
		RepoStrategy: RepoStrategyMultiRepo,
		Sonar:        Sonar{Organization: "x"},
		System: System{
			Architecture: ArchMultitier,
			Backend:      TierSpec{Path: "be", Repo: "x/backend", Lang: LangJava, SonarProject: "x_main-backend"},
			Frontend:     TierSpec{Path: "fe", Repo: "x/frontend", Lang: LangTypescript, SonarProject: "x_main-frontend"},
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/main", Lang: LangJava, SonarProject: "x_main-system-test"},
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
		Project: Project{URL: "https://github.com/orgs/acme/projects/1"},
		Sonar:   Sonar{Organization: "x"},
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/y", Lang: LangJava,
			SonarProject: "x_y-system",
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava, SonarProject: "x_y-system-test"},
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
		Sonar:        Sonar{Organization: "acme"},
		System: System{
			Architecture: ArchMonolith,
			Path:         "system/monolith/java",
			Repo:         "acme/page-turner",
			Lang:         LangJava,
			SonarProject: "acme_page-turner-system",
		},
		SystemTest: TierSpec{
			Path:         "system-test/java",
			Repo:         "acme/page-turner",
			Lang:         LangJava,
			SonarProject: "acme_page-turner-system-test",
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

// ---------------------------------------------------------------------------
// system_name / license / deploy field validation
// ---------------------------------------------------------------------------

// validMonolithBase is the smallest Config that Validate accepts with
// architecture set. Each system_name/license/deploy test mutates a copy.
func validMonolithBase() *Config {
	return &Config{
		Project:      Project{URL: "https://github.com/orgs/acme/projects/1"},
		RepoStrategy: RepoStrategyMonoRepo,
		Sonar:        Sonar{Organization: "acme"},
		System: System{
			Architecture: ArchMonolith,
			Path:         "system",
			Repo:         "acme/page-turner",
			Lang:         LangJava,
			SonarProject: "acme_page-turner-system",
		},
		SystemTest: TierSpec{
			Path: "system-test", Repo: "acme/page-turner", Lang: LangJava,
			SonarProject: "acme_page-turner-system-test",
		},
	}
}

func TestValidate_AcceptsValidSystemName(t *testing.T) {
	t.Parallel()
	// "Shop" is the template placeholder per NAMING.md — naming a system
	// "Shop" produces no-op replacements, which is by definition safe.
	for _, name := range []string{"Page Turner", "Shop"} {
		cfg := validMonolithBase()
		cfg.SystemName = name
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate(system_name=%q): %v", name, err)
		}
	}
}

func TestValidate_RejectsReservedSystemName(t *testing.T) {
	t.Parallel()
	cases := []string{"class", "Switch Class"}
	for _, name := range cases {
		cfg := validMonolithBase()
		cfg.SystemName = name
		err := cfg.Validate()
		if err == nil {
			t.Errorf("Validate(system_name=%q): want error, got nil", name)
			continue
		}
		if !strings.Contains(err.Error(), "system_name") {
			t.Errorf("Validate(system_name=%q): want error mentioning system_name, got: %v", name, err)
		}
	}
}

func TestValidate_RejectsBadCharsInSystemName(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	cfg.SystemName = "page-turner" // hyphens not allowed; letters + spaces only
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate: want error for hyphenated name, got nil")
	}
	if !strings.Contains(err.Error(), "system_name") {
		t.Errorf("error should mention system_name, got: %v", err)
	}
}

func TestValidate_AcceptsKnownLicenses(t *testing.T) {
	t.Parallel()
	for _, key := range []string{LicenseMIT, LicenseApache2, LicenseGPL3, LicenseBSD2, LicenseBSD3, LicenseUnlicense} {
		cfg := validMonolithBase()
		cfg.License = key
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate(license=%q): %v", key, err)
		}
	}
}

func TestValidate_RejectsUnknownLicense(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	cfg.License = "bogus-license"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate: want error for unknown license, got nil")
	}
	if !strings.Contains(err.Error(), "license") {
		t.Errorf("error should mention license, got: %v", err)
	}
}

func TestValidate_AcceptsKnownDeploys(t *testing.T) {
	t.Parallel()
	for _, key := range []string{DeployDocker, DeployCloudRun} {
		cfg := validMonolithBase()
		cfg.Deploy = key
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate(deploy=%q): %v", key, err)
		}
	}
}

func TestValidate_RejectsUnknownDeploy(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	cfg.Deploy = "bare-metal"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate: want error for unknown deploy, got nil")
	}
	if !strings.Contains(err.Error(), "deploy") {
		t.Errorf("error should mention deploy, got: %v", err)
	}
}

func TestValidate_AcceptsEmptyIdentityFields(t *testing.T) {
	t.Parallel()
	// system_name / license / deploy are all optional at the schema layer
	// (init re-checks presence). A config with architecture set and these
	// fields absent must still validate.
	cfg := validMonolithBase()
	cfg.SystemName = ""
	cfg.License = ""
	cfg.Deploy = ""
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v (empty system_name/license/deploy must be OK)", err)
	}
}

func TestRoundTrip_PreservesIdentityFields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	in := validMonolithBase()
	in.SystemName = "Page Turner"
	in.License = LicenseApache2
	in.Deploy = DeployDocker
	if err := Write(dir, in); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.SystemName != in.SystemName {
		t.Errorf("system_name: got %q, want %q", out.SystemName, in.SystemName)
	}
	if out.License != in.License {
		t.Errorf("license: got %q, want %q", out.License, in.License)
	}
	if out.Deploy != in.Deploy {
		t.Errorf("deploy: got %q, want %q", out.Deploy, in.Deploy)
	}
}

// ---------------------------------------------------------------------------
// Sonar block (Rules 17/18)
// ---------------------------------------------------------------------------

// TestValidate_RejectsMissingSonarOrganization pins Rule 17: once
// architecture is set, sonar.organization is required.
func TestValidate_RejectsMissingSonarOrganization(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	cfg.Sonar.Organization = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate: want error for missing sonar.organization, got nil")
	}
	if !strings.Contains(err.Error(), "sonar.organization") {
		t.Errorf("error should mention sonar.organization, got: %v", err)
	}
}

// TestValidate_RejectsMissingPerTierSonarProject pins Rule 18: each
// code tier that exists for the architecture must carry its sonar_project.
func TestValidate_RejectsMissingPerTierSonarProject(t *testing.T) {
	t.Parallel()

	t.Run("monolith missing system.sonar_project", func(t *testing.T) {
		cfg := validMonolithBase()
		cfg.System.SonarProject = ""
		if err := cfg.Validate(); err == nil ||
			!strings.Contains(err.Error(), "system.sonar_project") {
			t.Fatalf("want system.sonar_project error, got: %v", err)
		}
	})

	t.Run("monolith missing system_test.sonar_project", func(t *testing.T) {
		cfg := validMonolithBase()
		cfg.SystemTest.SonarProject = ""
		if err := cfg.Validate(); err == nil ||
			!strings.Contains(err.Error(), "system_test.sonar_project") {
			t.Fatalf("want system_test.sonar_project error, got: %v", err)
		}
	})

	t.Run("multitier missing backend.sonar_project", func(t *testing.T) {
		cfg := validMultitierBase()
		cfg.System.Backend.SonarProject = ""
		if err := cfg.Validate(); err == nil ||
			!strings.Contains(err.Error(), "system.backend.sonar_project") {
			t.Fatalf("want system.backend.sonar_project error, got: %v", err)
		}
	})

	t.Run("multitier missing frontend.sonar_project", func(t *testing.T) {
		cfg := validMultitierBase()
		cfg.System.Frontend.SonarProject = ""
		if err := cfg.Validate(); err == nil ||
			!strings.Contains(err.Error(), "system.frontend.sonar_project") {
			t.Fatalf("want system.frontend.sonar_project error, got: %v", err)
		}
	})
}

// TestValidate_RejectsSonarKeyOnWrongArchitecture pins the cross-tier
// exclusivity in Rule 18: system.sonar_project belongs only on monolith,
// backend/frontend.sonar_project belong only on multitier.
func TestValidate_RejectsSonarKeyOnWrongArchitecture(t *testing.T) {
	t.Parallel()

	t.Run("monolith with stray backend.sonar_project", func(t *testing.T) {
		cfg := validMonolithBase()
		cfg.System.Backend.SonarProject = "acme_page-turner-backend"
		if err := cfg.Validate(); err == nil ||
			!strings.Contains(err.Error(), "system.backend.sonar_project") {
			t.Fatalf("want exclusivity error, got: %v", err)
		}
	})

	t.Run("multitier with stray system.sonar_project", func(t *testing.T) {
		cfg := validMultitierBase()
		cfg.System.SonarProject = "acme_page-turner-system"
		if err := cfg.Validate(); err == nil ||
			!strings.Contains(err.Error(), "system.sonar_project") {
			t.Fatalf("want exclusivity error, got: %v", err)
		}
	})
}

// TestValidate_AcceptsEmptySonarBlockWithoutArchitecture confirms the
// schema accepts a partial Config: when system.architecture is unset,
// the sonar block has nothing to express and Rules 17/18 stay
// dormant. Matches the pattern already used for repo_strategy /
// system_test (architecture is the gate).
func TestValidate_AcceptsEmptySonarBlockWithoutArchitecture(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project:      Project{URL: "https://github.com/orgs/acme/projects/1"},
		RepoStrategy: RepoStrategyMonoRepo,
		// Arch empty; no Sonar block.
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("partial Config without architecture should validate: %v", err)
	}
}

// validMultitierBase mirrors validMonolithBase for the multitier shape:
// the smallest Config with architecture=multitier that Validate accepts.
func validMultitierBase() *Config {
	return &Config{
		Project:      Project{URL: "https://github.com/orgs/acme/projects/1"},
		RepoStrategy: RepoStrategyMonoRepo,
		Sonar:        Sonar{Organization: "acme"},
		System: System{
			Architecture: ArchMultitier,
			Backend: TierSpec{
				Path: "backend", Repo: "acme/page-turner", Lang: LangJava,
				SonarProject: "acme_page-turner-backend",
			},
			Frontend: TierSpec{
				Path: "frontend", Repo: "acme/page-turner", Lang: LangTypescript,
				SonarProject: "acme_page-turner-frontend",
			},
		},
		SystemTest: TierSpec{
			Path: "system-test", Repo: "acme/page-turner", Lang: LangJava,
			SonarProject: "acme_page-turner-system-test",
		},
	}
}
