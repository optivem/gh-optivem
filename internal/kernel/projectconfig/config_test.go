package projectconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, Path), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// javaPaths returns a valid Family B paths map for a (system-test.path,
// sut-namespace) pair under Java. Used by tests that need to satisfy
// Rule 22a (canonical-key presence) without caring about path values.
func javaPaths(systemTestPath, sutNamespace string) map[string]string {
	return DefaultPaths(LangJava, systemTestPath, sutNamespace)
}

// ---------------------------------------------------------------------------
// Sample configs (mirror the four canonical samples in the plan)
// ---------------------------------------------------------------------------

const sampleMonoRepoMonolith = `project:
  provider: github
  url: https://github.com/orgs/optivem/projects/20

repo-strategy: mono-repo

sonar:
  organization: optivem

system:
  architecture: monolith
  path: system/monolith/java
  repo: optivem/shop
  lang: java
  sonar-project: optivem_shop-system
  db-migration-path: system/db/migrations

system-test:
  path: system-test/java
  repo: optivem/shop
  lang: java
  sonar-project: optivem_shop-system-test
  paths:
    system-driver-port: system-test/java/src/main/java/testkit/driver/port/shop
    system-driver-adapter: system-test/java/src/main/java/testkit/driver/adapter/shop
    external-system-driver-port: system-test/java/src/main/java/testkit/external/port/shop
    external-system-driver-adapter: system-test/java/src/main/java/testkit/external/adapter/shop
    at-test: system-test/java/src/test/java/shop/latest/acceptance
    dsl-port: system-test/java/src/main/java/testkit/dsl/port/shop
    dsl-core: system-test/java/src/main/java/testkit/dsl/core/shop
    ct-test: system-test/java/src/test/java/shop/latest/contract
    system-driver-adapter-shared: system-test/java/src/main/java/testkit/driver/adapter/shop/shared
    common: system-test/java/src/main/java/testkit/common/shop
    domain-value-types: system-test/java/src/main/java/testkit/domainvaluetypes/shop

external-systems:
  warehouse:
    real-kind: simulator
    stub:
      path: stubs
      repo: optivem/shop
    simulator:
      path: simulators
      repo: optivem/shop
`

const sampleMonoRepoMultitier = `project:
  provider: github
  url: https://github.com/orgs/optivem/projects/20

repo-strategy: mono-repo

sonar:
  organization: optivem

system:
  architecture: multitier
  backend:
    path: system/multitier/backend-java
    repo: optivem/shop
    lang: java
    sonar-project: optivem_shop-backend
  frontend:
    path: system/multitier/frontend-react
    repo: optivem/shop
    lang: typescript
    sonar-project: optivem_shop-frontend
  db-migration-path: system/db/migrations

system-test:
  path: system-test/java
  repo: optivem/shop
  lang: java
  sonar-project: optivem_shop-system-test
  paths:
    system-driver-port: system-test/java/src/main/java/testkit/driver/port/shop
    system-driver-adapter: system-test/java/src/main/java/testkit/driver/adapter/shop
    external-system-driver-port: system-test/java/src/main/java/testkit/external/port/shop
    external-system-driver-adapter: system-test/java/src/main/java/testkit/external/adapter/shop
    at-test: system-test/java/src/test/java/shop/latest/acceptance
    dsl-port: system-test/java/src/main/java/testkit/dsl/port/shop
    dsl-core: system-test/java/src/main/java/testkit/dsl/core/shop
    ct-test: system-test/java/src/test/java/shop/latest/contract
    system-driver-adapter-shared: system-test/java/src/main/java/testkit/driver/adapter/shop/shared
    common: system-test/java/src/main/java/testkit/common/shop
    domain-value-types: system-test/java/src/main/java/testkit/domainvaluetypes/shop

external-systems:
  warehouse:
    real-kind: simulator
    stub:
      path: stubs
      repo: optivem/shop
    simulator:
      path: simulators
      repo: optivem/shop
`

const sampleMultiRepoMonolith = `project:
  provider: github
  url: https://github.com/orgs/optivem/projects/20

repo-strategy: multi-repo

sonar:
  organization: optivem

system:
  architecture: monolith
  path: .
  repo: optivem/shop
  lang: java
  sonar-project: optivem_shop-system
  db-migration-path: system/db/migrations

system-test:
  path: system-test
  repo: optivem/shop
  lang: java
  sonar-project: optivem_shop-system-test
  paths:
    system-driver-port: system-test/src/main/java/testkit/driver/port/shop
    system-driver-adapter: system-test/src/main/java/testkit/driver/adapter/shop
    external-system-driver-port: system-test/src/main/java/testkit/external/port/shop
    external-system-driver-adapter: system-test/src/main/java/testkit/external/adapter/shop
    at-test: system-test/src/test/java/shop/latest/acceptance
    dsl-port: system-test/src/main/java/testkit/dsl/port/shop
    dsl-core: system-test/src/main/java/testkit/dsl/core/shop
    ct-test: system-test/src/test/java/shop/latest/contract
    system-driver-adapter-shared: system-test/src/main/java/testkit/driver/adapter/shop/shared
    common: system-test/src/main/java/testkit/common/shop
    domain-value-types: system-test/src/main/java/testkit/domainvaluetypes/shop

external-systems:
  warehouse:
    real-kind: simulator
    stub:
      path: stubs
      repo: optivem/shop
    simulator:
      path: simulators
      repo: optivem/shop
`

const sampleMultiRepoMultitier = `project:
  provider: github
  url: https://github.com/orgs/optivem/projects/20

repo-strategy: multi-repo

sonar:
  organization: optivem

system:
  architecture: multitier
  backend:
    path: .
    repo: optivem/shop-backend
    lang: java
    sonar-project: optivem_shop-backend
  frontend:
    path: .
    repo: optivem/shop-frontend
    lang: typescript
    sonar-project: optivem_shop-frontend
  db-migration-path: system/db/migrations

system-test:
  path: system-test
  repo: optivem/shop-backend
  lang: java
  sonar-project: optivem_shop-system-test
  paths:
    system-driver-port: system-test/src/main/java/testkit/driver/port/shop-backend
    system-driver-adapter: system-test/src/main/java/testkit/driver/adapter/shop-backend
    external-system-driver-port: system-test/src/main/java/testkit/external/port/shop-backend
    external-system-driver-adapter: system-test/src/main/java/testkit/external/adapter/shop-backend
    at-test: system-test/src/test/java/shop-backend/latest/acceptance
    dsl-port: system-test/src/main/java/testkit/dsl/port/shop-backend
    dsl-core: system-test/src/main/java/testkit/dsl/core/shop-backend
    ct-test: system-test/src/test/java/shop-backend/latest/contract
    system-driver-adapter-shared: system-test/src/main/java/testkit/driver/adapter/shop-backend/shared
    common: system-test/src/main/java/testkit/common/shop-backend
    domain-value-types: system-test/src/main/java/testkit/domainvaluetypes/shop-backend

external-systems:
  warehouse:
    real-kind: simulator
    stub:
      path: stubs
      repo: optivem/shop-main
    simulator:
      path: simulators
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
  provider: github
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

// TestLoad_EmptyFileRejectsMissingProvider pins the contract that an
// empty gh-optivem.yaml fails to load because project.provider is
// mandatory. The error names the migrate command so an operator with a
// pre-provider config has a one-shot fix path. Pre-provider configs
// previously loaded as a zero-value Config; that contract was retired
// when the Tracker abstraction shipped (a config without a provider
// can't pick a Tracker adapter).
func TestLoad_EmptyFileRejectsMissingProvider(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, "")
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error on empty config (missing project.provider), got nil")
	}
	if !strings.Contains(err.Error(), "project.provider is required") {
		t.Errorf("error should mention project.provider, got: %v", err)
	}
	if !strings.Contains(err.Error(), "config migrate") {
		t.Errorf("error should hint at `config migrate`, got: %v", err)
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
				!reflect.DeepEqual(got.System.Backend, cfg.System.Backend) ||
				!reflect.DeepEqual(got.System.Frontend, cfg.System.Frontend) ||
				!reflect.DeepEqual(got.SystemTest, cfg.SystemTest) ||
				!reflect.DeepEqual(got.ExternalSystems, cfg.ExternalSystems) {
				t.Fatalf("round-trip mismatch:\n got:  %+v\n want: %+v", got, cfg)
			}
		})
	}
}

// TestRoundTrip_PreservesProcessFlowAndOverrides verifies that the optional
// process-flow: / task-prompts: / node-extras: / node-replacements: fields
// survive a Write→Load round-trip when set.
func TestRoundTrip_PreservesProcessFlowAndOverrides(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := &Config{
		Project:     Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
		ProcessFlow: "config/process-flow.yaml",
		TaskPrompts: map[string]string{
			"write-acceptance-tests": "config/prompts/write-acceptance-tests.md",
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
		t.Errorf("process-flow: got %q, want %q", got.ProcessFlow, cfg.ProcessFlow)
	}
	if got.TaskPrompts["write-acceptance-tests"] != cfg.TaskPrompts["write-acceptance-tests"] {
		t.Errorf("task-prompts[write-acceptance-tests]: got %q, want %q",
			got.TaskPrompts["write-acceptance-tests"], cfg.TaskPrompts["write-acceptance-tests"])
	}
	if got.NodeExtras["AT_RED_DSL_WRITE"] != cfg.NodeExtras["AT_RED_DSL_WRITE"] {
		t.Errorf("node-extras: got %q", got.NodeExtras["AT_RED_DSL_WRITE"])
	}
	if got.NodeReplacements["AT_RED_TEST_WRITE"] != cfg.NodeReplacements["AT_RED_TEST_WRITE"] {
		t.Errorf("node-replacements: got %q", got.NodeReplacements["AT_RED_TEST_WRITE"])
	}
}

// TestValidate_ProcessFlow_RejectsAbsolutePath verifies path-validation
// kicks in for the new process-flow: field.
func TestValidate_ProcessFlow_RejectsAbsolutePath(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project:     Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
		ProcessFlow: "/abs/process-flow.yaml",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for absolute process-flow, got nil")
	}
	if !strings.Contains(err.Error(), "process-flow") {
		t.Fatalf("error should mention process-flow, got: %v", err)
	}
}

// TestValidate_TaskPrompts_RejectsAbsolutePath verifies values pass the
// same path-validation as system/system-test paths.
func TestValidate_TaskPrompts_RejectsAbsolutePath(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project: Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
		TaskPrompts: map[string]string{
			"write-acceptance-tests": "/abs/prompts/write-acceptance-tests.md",
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for absolute task-prompts path, got nil")
	}
}

// TestValidate_NodeReplacements_RejectsAbsolutePath verifies values pass
// the same path-validation as task-prompts.
func TestValidate_NodeReplacements_RejectsAbsolutePath(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project: Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
		NodeReplacements: map[string]string{
			"AT_RED_TEST_WRITE": "/abs/prompts/x.md",
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for absolute node-replacements path, got nil")
	}
}

// TestValidate_RejectsSameKeyInExtrasAndReplacements verifies the
// "replace supersedes extras" rule: a node ID may not appear in both
// maps simultaneously.
func TestValidate_RejectsSameKeyInExtrasAndReplacements(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project:          Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
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
	cfg := &Config{Project: Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("empty override maps should validate, got: %v", err)
	}
}

// TestRoundTrip_PreservesSystemAndTestConfig verifies that the optional
// system.config: / system-test.config: fields survive a Write→Load round-trip
// when set, and stay empty (and absent from the written YAML) when unset.
func TestRoundTrip_PreservesSystemAndTestConfig(t *testing.T) {
	t.Parallel()

	t.Run("set", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cfg := &Config{
			Project:    Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
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
			t.Errorf("system-test.config: got %q, want %q", got.SystemTest.Config, cfg.SystemTest.Config)
		}
	})

	t.Run("unset omits the keys", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cfg := &Config{Project: Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"}}
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

// TestValidate_RejectsConfigOnBackendOrFrontend verifies the Config field
// is rejected on system.backend / system.frontend (it's only meaningful on
// system-test). Catches typos like accidentally placing the tests.yaml path
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
				Project: Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
				System: System{
					Backend: TierSpec{Path: "x", Repo: "r", Lang: LangJava, Config: "nope"},
				},
			},
			wantHint: "system.backend.config",
		},
		{
			name: "frontend.config",
			cfg: &Config{
				Project: Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
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

// TestValidate_SystemDriverAdapterChannels_MemberShape covers the member-level
// checks of Rule 24, which run independent of architecture: a member must name
// a declared channel (casing slip → did-you-mean), and its value must be
// fully-resolved and repo-relative.
func TestValidate_SystemDriverAdapterChannels_MemberShape(t *testing.T) {
	t.Parallel()
	mk := func(channels []string, members map[string]string) *Config {
		return &Config{
			Project:    Project{Provider: ProviderGitHub},
			Channels:   channels,
			SystemTest: TierSpec{SystemDriverAdapterChannels: members},
		}
	}
	cases := []struct {
		name     string
		cfg      *Config
		wantHint string // "" → expect success
	}{
		{"valid 1:1", mk([]string{"api", "ui"}, map[string]string{
			"api": "system-test/driver/adapter/api", "ui": "system-test/driver/adapter/ui"}), ""},
		{"undeclared channel", mk([]string{"api"}, map[string]string{
			"api": "x", "ui": "y"}), "not a declared channel"},
		{"casing slip", mk([]string{"api"}, map[string]string{
			"Api": "x"}), "did you mean"},
		{"substitution marker", mk([]string{"api"}, map[string]string{
			"api": "system-test/${sut-namespace}/api"}), "${...} marker"},
		{"absolute path", mk([]string{"api"}, map[string]string{
			"api": "/abs/api"}), "repo-relative"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := c.cfg.Validate()
			if c.wantHint == "" {
				if err != nil {
					t.Fatalf("want valid, got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantHint) {
				t.Fatalf("want error hinting %q, got: %v", c.wantHint, err)
			}
		})
	}
}

// TestValidate_SystemDriverAdapterChannels_RequiredWhenArchitectureSet — the
// 1:1 tie's "every declared channel has a member" half gates on architecture
// (matching Rule 22a): a declared channel with no member is rejected, and
// supplying the members makes it valid.
func TestValidate_SystemDriverAdapterChannels_RequiredWhenArchitectureSet(t *testing.T) {
	t.Parallel()

	missing := validMonolithBase()
	missing.Channels = []string{"api", "ui"}
	err := missing.Validate()
	if err == nil || !strings.Contains(err.Error(), "missing a member") {
		t.Fatalf("want missing-member error, got: %v", err)
	}

	ok := validMonolithBase()
	ok.Channels = []string{"api", "ui"}
	ok.SystemTest.SystemDriverAdapterChannels = map[string]string{
		"api": "system-test/driver/adapter/api",
		"ui":  "system-test/driver/adapter/ui",
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("want valid once members supplied, got: %v", err)
	}
}

// TestValidate_RejectsSystemDriverAdapterChannelsOnBackendOrFrontend — the
// field is system-test-only (Rule 22d), same as paths:.
func TestValidate_RejectsSystemDriverAdapterChannelsOnBackendOrFrontend(t *testing.T) {
	t.Parallel()
	for _, tier := range []string{"backend", "frontend"} {
		cfg := &Config{Project: Project{Provider: ProviderGitHub}}
		ts := TierSpec{SystemDriverAdapterChannels: map[string]string{"api": "x"}}
		if tier == "backend" {
			cfg.System.Backend = ts
		} else {
			cfg.System.Frontend = ts
		}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "system."+tier+".system-driver-adapter-channels") {
			t.Errorf("%s: want system-test-only rejection, got: %v", tier, err)
		}
	}
}

// TestPlaceholderMap_EmitsAdapterChannelMembers — the per-channel members
// surface as dotted Family B keys so a layer reference can name one.
func TestPlaceholderMap_EmitsAdapterChannelMembers(t *testing.T) {
	t.Parallel()
	cfg := &Config{SystemTest: TierSpec{SystemDriverAdapterChannels: map[string]string{
		"api": "system-test/driver/adapter/api",
		"ui":  "system-test/driver/adapter/ui",
	}}}
	pm := cfg.PlaceholderMap()
	if got := pm["system-driver-adapter-channels.api"]; got != "system-test/driver/adapter/api" {
		t.Errorf("api member: got %q", got)
	}
	if got := pm["system-driver-adapter-channels.ui"]; got != "system-test/driver/adapter/ui" {
		t.Errorf("ui member: got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Validation rules
// ---------------------------------------------------------------------------

// TestValidate_AcceptsEmptyProjectURL pins the contract that an empty
// project.url is valid at YAML-load time: `gh optivem init` Path A
// auto-creates the board on first run and rewrites the file with the
// URL. project.provider is still required — only the URL is allowed
// empty. The ATDD runtime (via internal/atdd/runtime/tracker/factory)
// still enforces non-empty at use time.
func TestValidate_AcceptsEmptyProjectURL(t *testing.T) {
	t.Parallel()
	cfg := &Config{Project: Project{Provider: ProviderGitHub}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("provider-only config (empty project.url) should validate now that Path A auto-creates; got: %v", err)
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
	cfg := &Config{Project: Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("project-url-only config should validate, got: %v", err)
	}
}

func TestValidate_UnknownRepoStrategyErrors(t *testing.T) {
	t.Parallel()
	cfg := &Config{RepoStrategy: "poly-repo"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for unknown repo-strategy, got nil")
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
		System:     System{Architecture: ArchMonolith, Path: "/abs/path", Repo: "x/y", Lang: LangJava},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for absolute path, got nil")
	}
}

func TestValidate_RejectsDotDotPath(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System:     System{Architecture: ArchMonolith, Path: "../foo", Repo: "x/y", Lang: LangJava},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for ..-prefixed path, got nil")
	}
}

func TestValidate_RejectsEmbeddedDotDotPath(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System:     System{Architecture: ArchMonolith, Path: "foo/../bar", Repo: "x/y", Lang: LangJava},
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
		t.Fatal("expected error for system-test missing lang, got nil")
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
		t.Fatal("expected error for system-test missing repo, got nil")
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
		t.Fatal("expected error for missing system-test, got nil")
	}
}

// Rule 22a: paths block is required (with every canonical Family B key
// populated) once system.architecture is set. The doctrine is
// "explicit only — no defaults anywhere"; missing keys must surface at
// config load, not deep inside a per-ticket agent dispatch.

func TestValidate_RejectsMissingPathsBlockWhenArchitectureSet(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project:      Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/optivem/projects/20"},
		RepoStrategy: RepoStrategyMonoRepo,
		Sonar:        Sonar{Organization: "optivem"},
		System: System{
			Architecture: ArchMonolith,
			Path:         "system", Repo: "optivem/shop", Lang: LangJava,
			SonarProject: "optivem_shop-system",
		},
		SystemTest: TierSpec{
			Path: "system-test", Repo: "optivem/shop", Lang: LangJava,
			SonarProject: "optivem_shop-system-test",
		},
		// Paths intentionally absent.
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing paths block, got nil")
	}
	for _, want := range []string{"system-test.paths.system-driver-port", "system-test.paths.system-driver-adapter", "system-test.paths.at-test", "system-test.paths.ct-test"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name %s, got: %v", want, err)
		}
	}
}

func TestValidate_RejectsMissingCanonicalKey(t *testing.T) {
	t.Parallel()
	full := DefaultPaths(LangJava, "system-test", "shop")
	delete(full, "system-driver-adapter")
	cfg := &Config{
		Project:      Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/optivem/projects/20"},
		RepoStrategy: RepoStrategyMonoRepo,
		Sonar:        Sonar{Organization: "optivem"},
		System: System{
			Architecture: ArchMonolith,
			Path:         "system", Repo: "optivem/shop", Lang: LangJava,
			SonarProject: "optivem_shop-system",
		},
		SystemTest: TierSpec{
			Path: "system-test", Repo: "optivem/shop", Lang: LangJava,
			SonarProject: "optivem_shop-system-test",
			Paths:        full,
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing system-driver-adapter, got nil")
	}
	if !strings.Contains(err.Error(), "system-test.paths.system-driver-adapter") {
		t.Errorf("error should name the missing key, got: %v", err)
	}
	if strings.Contains(err.Error(), "system-test.paths.system-driver-port") {
		t.Errorf("error should not name keys that ARE present, got: %v", err)
	}
}

func TestValidate_RejectsEmptyCanonicalValue(t *testing.T) {
	t.Parallel()
	full := DefaultPaths(LangJava, "system-test", "shop")
	full["dsl-core"] = ""
	cfg := &Config{
		Project:      Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/optivem/projects/20"},
		RepoStrategy: RepoStrategyMonoRepo,
		Sonar:        Sonar{Organization: "optivem"},
		System: System{
			Architecture: ArchMonolith,
			Path:         "system", Repo: "optivem/shop", Lang: LangJava,
			SonarProject: "optivem_shop-system",
		},
		SystemTest: TierSpec{
			Path: "system-test", Repo: "optivem/shop", Lang: LangJava,
			SonarProject: "optivem_shop-system-test",
			Paths:        full,
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty dsl-core value, got nil")
	}
	if !strings.Contains(err.Error(), "system-test.paths.dsl-core") {
		t.Errorf("error should name the empty-value key, got: %v", err)
	}
}

// assertPartialConfigValidates builds a config with only a project block (no
// architecture, no paths) and asserts Validate accepts it — the shared body
// for the architecture-gated "absent X is fine when architecture is unset"
// rules.
func assertPartialConfigValidates(t *testing.T) {
	t.Helper()
	t.Parallel()
	cfg := &Config{
		Project: Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/optivem/projects/20"},
		// No architecture, no paths — partial config shape.
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("partial config without architecture should validate, got: %v", err)
	}
}

func TestValidate_AcceptsAbsentPathsWhenArchitectureUnset(t *testing.T) {
	assertPartialConfigValidates(t)
}

// Rule 22b: system.db-migration-path is required once architecture is
// set. The Family A path-shaped key names the shared canonical Flyway
// migration set consumed by every SUT; an absent value would have the
// system-implementer / system-updater agent's write set resolve a
// scope-eligible layer to "" and fail at dispatch time.

func TestValidate_RejectsMissingDbMigrationPathWhenArchitectureSet(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	cfg.System.DbMigrationPath = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing db-migration-path, got nil")
	}
	if !strings.Contains(err.Error(), "system.db-migration-path") {
		t.Errorf("error should name system.db-migration-path, got: %v", err)
	}
	// Error must name the back-fill path so an operator with a pre-this-
	// plan config has a one-shot fix path.
	if !strings.Contains(err.Error(), "config migrate") {
		t.Errorf("error should hint at `config migrate`, got: %v", err)
	}
}

func TestValidate_AcceptsAbsentDbMigrationPathWhenArchitectureUnset(t *testing.T) {
	// Rule 22b gates db-migration-path on architecture: absent it validates
	// when no architecture is set. Same shape as the general absent-paths case.
	assertPartialConfigValidates(t)
}

func TestValidate_RejectsAbsoluteDbMigrationPath(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	cfg.System.DbMigrationPath = "/abs/migrations"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for absolute db-migration-path, got nil")
	}
	if !strings.Contains(err.Error(), "system.db-migration-path") {
		t.Errorf("error should name system.db-migration-path, got: %v", err)
	}
}

// Family A reservation: `system-db-migration-path` cannot appear in
// `system-test.paths:` — a typo'd Family B key with that name would
// otherwise quietly override the canonical Family A value.
func TestValidate_RejectsFamilyBShadowOfDbMigrationPath(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	cfg.SystemTest.Paths["system-db-migration-path"] = "elsewhere"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for Family B key shadowing Family A name, got nil")
	}
	if !strings.Contains(err.Error(), "system-db-migration-path") {
		t.Errorf("error should name the shadowing key, got: %v", err)
	}
}

// PlaceholderMap must expose `system-db-migration-path` so prompt
// rendering can substitute `${system-db-migration-path}` in agent
// bodies (system-implementer.md, system-updater.md).
func TestPlaceholderMap_IncludesDbMigrationPath(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	got := cfg.PlaceholderMap()
	if got["system-db-migration-path"] != DefaultDbMigrationPath {
		t.Errorf("system-db-migration-path: got %q, want %q", got["system-db-migration-path"], DefaultDbMigrationPath)
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
			Lang:         LangJava,
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
		Project: Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
		Sonar:   Sonar{Organization: "x"},
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/y", Lang: LangJava,
			SonarProject:    "x_y-system",
			DbMigrationPath: DefaultDbMigrationPath,
		},
		SystemTest: TierSpec{
			Path: "t", Repo: "x/y", Lang: LangJava, SonarProject: "x_y-system-test",
			Paths: javaPaths("t", "y"),
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config without external-systems should validate, got: %v", err)
	}
}

func TestValidate_AcceptsTestInstanceAndSimulatorKinds(t *testing.T) {
	t.Parallel()
	base := func() *Config {
		return &Config{
			Project: Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
			Sonar:   Sonar{Organization: "x"},
			System: System{
				Architecture: ArchMonolith,
				Path:         "p", Repo: "x/y", Lang: LangJava,
				SonarProject:    "x_y-system",
				DbMigrationPath: DefaultDbMigrationPath,
			},
			SystemTest: TierSpec{
				Path: "t", Repo: "x/y", Lang: LangJava, SonarProject: "x_y-system-test",
				Paths: javaPaths("t", "y"),
			},
		}
	}

	// test-instance: stub only, no simulator block.
	c := base()
	c.ExternalSystems = ExternalSystems{
		"warehouse": {
			RealKind: RealKindTestInstance,
			Stub:     ExternalSpec{Path: "stubs", Repo: "x/y"},
		},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("test-instance with stub only should validate, got: %v", err)
	}

	// simulator: stub + simulator block.
	c = base()
	c.ExternalSystems = ExternalSystems{
		"warehouse": {
			RealKind:  RealKindSimulator,
			Stub:      ExternalSpec{Path: "stubs", Repo: "x/y"},
			Simulator: ExternalSpec{Path: "simulators", Repo: "x/y"},
		},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("simulator with stub + simulator should validate, got: %v", err)
	}
}

// TestValidate_ExternalSystemRealKindRules covers the per-system rules:
// real-kind required + enum, stub always required, and the simulator
// present-iff-simulator invariant (plan 20260606-1356).
func TestValidate_ExternalSystemRealKindRules(t *testing.T) {
	t.Parallel()
	base := func() *Config {
		return &Config{
			Project: Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
			Sonar:   Sonar{Organization: "x"},
			System: System{
				Architecture: ArchMonolith,
				Path:         "p", Repo: "x/y", Lang: LangJava,
				SonarProject:    "x_y-system",
				DbMigrationPath: DefaultDbMigrationPath,
			},
			SystemTest: TierSpec{
				Path: "t", Repo: "x/y", Lang: LangJava, SonarProject: "x_y-system-test",
				Paths: javaPaths("t", "y"),
			},
		}
	}

	cases := []struct {
		name     string
		entry    ExternalSystem
		wantSubs string // non-empty → expect an error containing this
	}{
		{
			name:     "real-kind missing",
			entry:    ExternalSystem{Stub: ExternalSpec{Path: "stubs", Repo: "x/y"}},
			wantSubs: "real-kind is required",
		},
		{
			name:     "real-kind unknown",
			entry:    ExternalSystem{RealKind: "live", Stub: ExternalSpec{Path: "stubs", Repo: "x/y"}},
			wantSubs: "must be one of",
		},
		{
			name:     "stub missing",
			entry:    ExternalSystem{RealKind: RealKindTestInstance},
			wantSubs: "stub is required",
		},
		{
			name:     "simulator kind without simulator block",
			entry:    ExternalSystem{RealKind: RealKindSimulator, Stub: ExternalSpec{Path: "stubs", Repo: "x/y"}},
			wantSubs: "requires a simulator block",
		},
		{
			name: "test-instance kind with stray simulator block",
			entry: ExternalSystem{
				RealKind:  RealKindTestInstance,
				Stub:      ExternalSpec{Path: "stubs", Repo: "x/y"},
				Simulator: ExternalSpec{Path: "simulators", Repo: "x/y"},
			},
			wantSubs: "must not carry a simulator block",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := base()
			c.ExternalSystems = ExternalSystems{"warehouse": tc.entry}
			err := c.Validate()
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.wantSubs)
			}
			if !strings.Contains(err.Error(), tc.wantSubs) {
				t.Errorf("error should contain %q, got: %v", tc.wantSubs, err)
			}
		})
	}
}

func TestValidate_RejectsExternalWithMissingRepo(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/y", Lang: LangJava,
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
		ExternalSystems: ExternalSystems{
			"warehouse": {RealKind: RealKindTestInstance, Stub: ExternalSpec{Path: "stubs" /* repo missing */}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for external stub missing repo, got nil")
	}
}

func TestValidate_AcceptsExternalRepoNotInOtherTiers(t *testing.T) {
	t.Parallel()
	// External systems can live in their own repo (multi-repo case).
	// system-test.repo carries the canonical base ("x/main"), so the Sonar
	// keys use base="main" — independent of where each component lives.
	cfg := &Config{
		Project:      Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
		RepoStrategy: RepoStrategyMultiRepo,
		Sonar:        Sonar{Organization: "x"},
		System: System{
			Architecture:    ArchMultitier,
			Backend:         TierSpec{Path: "be", Repo: "x/backend", Lang: LangJava, SonarProject: "x_main-backend"},
			Frontend:        TierSpec{Path: "fe", Repo: "x/frontend", Lang: LangTypescript, SonarProject: "x_main-frontend"},
			DbMigrationPath: DefaultDbMigrationPath,
		},
		SystemTest: TierSpec{
			Path: "t", Repo: "x/main", Lang: LangJava, SonarProject: "x_main-system-test",
			Paths: javaPaths("t", "main"),
		},
		ExternalSystems: ExternalSystems{
			"warehouse": {
				RealKind:  RealKindSimulator,
				Stub:      ExternalSpec{Path: "stubs", Repo: "x/externals" /* unique slug */},
				Simulator: ExternalSpec{Path: "simulators", Repo: "x/externals"},
			},
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
		SystemTest: TierSpec{Path: "t", Repo: "x/backend", Lang: LangJava},
		ExternalSystems: ExternalSystems{
			"warehouse": {RealKind: RealKindTestInstance, Stub: ExternalSpec{Path: "stub", Repo: "x/main"}},
		},
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
		Project: Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
		Sonar:   Sonar{Organization: "x"},
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/y", Lang: LangJava,
			SonarProject:    "x_y-system",
			DbMigrationPath: DefaultDbMigrationPath,
		},
		SystemTest: TierSpec{
			Path: "t", Repo: "x/y", Lang: LangJava, SonarProject: "x_y-system-test",
			Paths: javaPaths("t", "y"),
		},
	}
	if err := Write(dir, cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, Path))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(data)
	if strings.Contains(body, "external-systems:") {
		t.Errorf("expected external-systems to be omitted; got:\n%s", body)
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
		Project:      Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/7"},
		RepoStrategy: RepoStrategyMonoRepo,
		Sonar:        Sonar{Organization: "acme"},
		System: System{
			Architecture:    ArchMonolith,
			Path:            "system/monolith/java",
			Repo:            "acme/page-turner",
			Lang:            LangJava,
			SonarProject:    "acme_page-turner-system",
			DbMigrationPath: DefaultDbMigrationPath,
		},
		SystemTest: TierSpec{
			Path:         "system-test/java",
			Repo:         "acme/page-turner",
			Lang:         LangJava,
			SonarProject: "acme_page-turner-system-test",
			Paths:        javaPaths("system-test/java", "page-turner"),
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
// system-name / license / deploy field validation
// ---------------------------------------------------------------------------

// validMonolithBase is the smallest Config that Validate accepts with
// architecture set. Each system-name/license/deploy test mutates a copy.
func validMonolithBase() *Config {
	return &Config{
		Project:      Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
		RepoStrategy: RepoStrategyMonoRepo,
		Sonar:        Sonar{Organization: "acme"},
		System: System{
			Architecture:    ArchMonolith,
			Path:            "system",
			Repo:            "acme/page-turner",
			Lang:            LangJava,
			SonarProject:    "acme_page-turner-system",
			DbMigrationPath: DefaultDbMigrationPath,
		},
		SystemTest: TierSpec{
			Path: "system-test", Repo: "acme/page-turner", Lang: LangJava,
			SonarProject: "acme_page-turner-system-test",
			Paths:        javaPaths("system-test", "page-turner"),
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
			t.Errorf("Validate(system-name=%q): %v", name, err)
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
			t.Errorf("Validate(system-name=%q): want error, got nil", name)
			continue
		}
		if !strings.Contains(err.Error(), "system-name") {
			t.Errorf("Validate(system-name=%q): want error mentioning system-name, got: %v", name, err)
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
	if !strings.Contains(err.Error(), "system-name") {
		t.Errorf("error should mention system-name, got: %v", err)
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
	// system-name / license / deploy are all optional at the schema layer
	// (init re-checks presence). A config with architecture set and these
	// fields absent must still validate.
	cfg := validMonolithBase()
	cfg.SystemName = ""
	cfg.License = ""
	cfg.Deploy = ""
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v (empty system-name/license/deploy must be OK)", err)
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
		t.Errorf("system-name: got %q, want %q", out.SystemName, in.SystemName)
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
// code tier that exists for the architecture must carry its sonar-project.
func TestValidate_RejectsMissingPerTierSonarProject(t *testing.T) {
	t.Parallel()

	t.Run("monolith missing system.sonar-project", func(t *testing.T) {
		cfg := validMonolithBase()
		cfg.System.SonarProject = ""
		if err := cfg.Validate(); err == nil ||
			!strings.Contains(err.Error(), "system.sonar-project") {
			t.Fatalf("want system.sonar-project error, got: %v", err)
		}
	})

	t.Run("monolith missing system-test.sonar-project", func(t *testing.T) {
		cfg := validMonolithBase()
		cfg.SystemTest.SonarProject = ""
		if err := cfg.Validate(); err == nil ||
			!strings.Contains(err.Error(), "system-test.sonar-project") {
			t.Fatalf("want system-test.sonar-project error, got: %v", err)
		}
	})

	t.Run("multitier missing backend.sonar-project", func(t *testing.T) {
		cfg := validMultitierBase()
		cfg.System.Backend.SonarProject = ""
		if err := cfg.Validate(); err == nil ||
			!strings.Contains(err.Error(), "system.backend.sonar-project") {
			t.Fatalf("want system.backend.sonar-project error, got: %v", err)
		}
	})

	t.Run("multitier missing frontend.sonar-project", func(t *testing.T) {
		cfg := validMultitierBase()
		cfg.System.Frontend.SonarProject = ""
		if err := cfg.Validate(); err == nil ||
			!strings.Contains(err.Error(), "system.frontend.sonar-project") {
			t.Fatalf("want system.frontend.sonar-project error, got: %v", err)
		}
	})
}

// TestValidate_RejectsSonarKeyOnWrongArchitecture pins the cross-tier
// exclusivity in Rule 18: system.sonar-project belongs only on monolith,
// backend/frontend.sonar-project belong only on multitier.
func TestValidate_RejectsSonarKeyOnWrongArchitecture(t *testing.T) {
	t.Parallel()

	t.Run("monolith with stray backend.sonar-project", func(t *testing.T) {
		cfg := validMonolithBase()
		cfg.System.Backend.SonarProject = "acme_page-turner-backend"
		if err := cfg.Validate(); err == nil ||
			!strings.Contains(err.Error(), "system.backend.sonar-project") {
			t.Fatalf("want exclusivity error, got: %v", err)
		}
	})

	t.Run("multitier with stray system.sonar-project", func(t *testing.T) {
		cfg := validMultitierBase()
		cfg.System.SonarProject = "acme_page-turner-system"
		if err := cfg.Validate(); err == nil ||
			!strings.Contains(err.Error(), "system.sonar-project") {
			t.Fatalf("want exclusivity error, got: %v", err)
		}
	})
}

// TestValidate_AcceptsEmptySonarBlockWithoutArchitecture confirms the
// schema accepts a partial Config: when system.architecture is unset,
// the sonar block has nothing to express and Rules 17/18 stay
// dormant. Matches the pattern already used for repo-strategy /
// system-test (architecture is the gate).
func TestValidate_AcceptsEmptySonarBlockWithoutArchitecture(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project:      Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
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
		Project:      Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
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
			DbMigrationPath: DefaultDbMigrationPath,
		},
		SystemTest: TierSpec{
			Path: "system-test", Repo: "acme/page-turner", Lang: LangJava,
			SonarProject: "acme_page-turner-system-test",
			Paths:        javaPaths("system-test", "page-turner"),
		},
	}
}

// ---------------------------------------------------------------------------
// LocalRepos (repos: field) — schema acceptance and rejection
// ---------------------------------------------------------------------------

// TestValidate_AcceptsAbsentReposField pins the backwards-compat
// contract: a config produced before the repos: field was introduced
// must still load and validate cleanly. The four canonical samples
// already cover this transitively (none has repos:), but pinning it
// directly makes the contract findable in one place.
func TestValidate_AcceptsAbsentReposField(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	if cfg.LocalRepos != nil {
		t.Fatalf("validMonolithBase should not set LocalRepos; got %+v", cfg.LocalRepos)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("absent repos: should validate, got: %v", err)
	}
}

func TestValidate_AcceptsValidRepoPaths(t *testing.T) {
	t.Parallel()
	cfg := validMultitierBase()
	cfg.LocalRepos = []RepoEntry{
		{Path: "../page-turner-backend"},
		{Path: "../page-turner-frontend"},
		{Path: "system-tests"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("well-formed repos: should validate, got: %v", err)
	}
}

func TestValidate_RejectsRepoEntryWithEmptyPath(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	cfg.LocalRepos = []RepoEntry{{Path: ""}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty repos[0].path, got nil")
	}
	if !strings.Contains(err.Error(), "repos[0].path") {
		t.Errorf("error should name repos[0].path, got: %v", err)
	}
}

func TestValidate_RejectsAbsoluteRepoPath(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	cfg.LocalRepos = []RepoEntry{{Path: "/abs/page-turner-backend"}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for absolute repos path, got nil")
	}
	if !strings.Contains(err.Error(), "repo-relative") {
		t.Errorf("error should explain repo-relative requirement, got: %v", err)
	}
}

// TestValidate_AcceptsRepoPathWithParentSegmentInside pins that
// embedded `..` (e.g. `system/../escape`) is accepted in repos[].
// validateRepoPath is intentionally more permissive than validatePath
// — repos[] declares clone locations, and any path expression that
// resolves to a sensible directory is the operator's call. Duplicate
// detection runs filepath.Clean so this form collapses to `escape`
// and would be rejected if another entry already pointed there.
func TestValidate_AcceptsRepoPathWithParentSegmentInside(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	cfg.LocalRepos = []RepoEntry{{Path: "system/../escape"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("embedded `..` should validate for repos[], got: %v", err)
	}
}

// TestValidate_AcceptsRepoPathStartingWithDotDot pins the sibling-folder
// pattern used by every multi-repo project (`../page-turner-backend`
// reaches a sibling clone of the gh-optivem.yaml directory).
func TestValidate_AcceptsRepoPathStartingWithDotDot(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	cfg.LocalRepos = []RepoEntry{{Path: "../sibling"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("`../sibling` should validate for repos[], got: %v", err)
	}
}

func TestValidate_RejectsDuplicateRepoPaths(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	cfg.LocalRepos = []RepoEntry{
		{Path: "system-tests"},
		{Path: "system-tests"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate paths, got nil")
	}
	if !strings.Contains(err.Error(), "more than once") {
		t.Errorf("error should mention duplication, got: %v", err)
	}
}

// TestValidate_RepoPathDuplicationAfterNormalization confirms that
// `./foo` and `foo` (which filepath.Clean reduces to the same value)
// are detected as duplicates. Operators sometimes hand-edit one form
// and re-run init which writes the other; the validator should reject
// the conflict rather than silently iterate the same folder twice.
func TestValidate_RepoPathDuplicationAfterNormalization(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	cfg.LocalRepos = []RepoEntry{
		{Path: "./system-tests"},
		{Path: "system-tests"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected duplication error after normalization, got nil")
	}
}

func TestLoad_AcceptsReposFieldInYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, sampleMonoRepoMonolith+`
repos:
  - path: system-frontend
  - path: system-backend
  - path: system-tests
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.LocalRepos) != 3 {
		t.Fatalf("got %d entries, want 3: %+v", len(cfg.LocalRepos), cfg.LocalRepos)
	}
	want := []string{"system-frontend", "system-backend", "system-tests"}
	for i, p := range want {
		if cfg.LocalRepos[i].Path != p {
			t.Errorf("repos[%d].path = %q, want %q", i, cfg.LocalRepos[i].Path, p)
		}
	}
}

func TestWrite_RoundTripsReposField(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	cfg.LocalRepos = []RepoEntry{
		{Path: "system-frontend"},
		{Path: "system-backend"},
	}
	dir := t.TempDir()
	if err := Write(dir, cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.LocalRepos) != 2 ||
		got.LocalRepos[0].Path != "system-frontend" ||
		got.LocalRepos[1].Path != "system-backend" {
		t.Errorf("round-trip mismatch: got %+v", got.LocalRepos)
	}
}
