package projectconfig

import (
	"strings"
	"testing"
)

// sampleComponent is the canonical kind: component config — one unit of code
// plus the tracker identity every project carries, and nothing else. It is the
// shape system/multitier/backend-clean-java in optivem/shop declares.
const sampleComponent = `kind: component

project:
  provider: github

component:
  name: backend-clean-java
  path: system/multitier/backend-clean-java
  repo: optivem/shop
  lang: java
`

// componentConfig returns a minimal valid kind: component config, so each test
// can mutate exactly the one field it is exercising.
func componentConfig() *Config {
	return &Config{
		Kind:    KindComponent,
		Project: Project{Provider: ProviderGitHub},
		Component: Component{
			Name: "backend-clean-java",
			Path: "system/multitier/backend-clean-java",
			Repo: "optivem/shop",
			Lang: LangJava,
		},
	}
}

// ---------------------------------------------------------------------------
// Zero migration cost: absent kind: keeps every existing config working.
// ---------------------------------------------------------------------------

// TestValidate_AbsentKindDefaultsToSystem is the migration-cost guarantee: the
// four canonical samples carry no kind: line, and must validate exactly as
// they did before the field existed.
func TestValidate_AbsentKindDefaultsToSystem(t *testing.T) {
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
			// Anchored to the start of a line: the samples do carry
			// external-systems[*].real-kind:, which is a different field.
			if strings.HasPrefix(tc.body, "kind:") || strings.Contains(tc.body, "\nkind:") {
				t.Fatalf("sample %s should carry no top-level kind: line — it is the no-migration case", tc.name)
			}
			dir := t.TempDir()
			writeConfig(t, dir, tc.body)
			cfg, err := Load(dir)
			if err != nil {
				t.Fatalf("sample %s should validate unchanged with no kind:, got: %v", tc.name, err)
			}
			if got := cfg.KindOrDefault(); got != KindSystem {
				t.Fatalf("absent kind: should resolve to %q, got %q", KindSystem, got)
			}
			if cfg.IsComponent() {
				t.Fatal("a config with no kind: must not read as a component")
			}
		})
	}
}

// TestKindOrDefault_NilConfig pins the nil-safety contract: Validate accepts a
// nil *Config, so every accessor the command guards call must tolerate one too.
func TestKindOrDefault_NilConfig(t *testing.T) {
	t.Parallel()
	var cfg *Config
	if got := cfg.KindOrDefault(); got != KindSystem {
		t.Fatalf("nil config should read as %q, got %q", KindSystem, got)
	}
	if cfg.IsComponent() {
		t.Fatal("nil config must not read as a component")
	}
}

// ---------------------------------------------------------------------------
// kind: component — the happy path and the required fields.
// ---------------------------------------------------------------------------

func TestLoad_ComponentSampleValidates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, sampleComponent)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("component sample should validate, got: %v", err)
	}
	if !cfg.IsComponent() {
		t.Fatalf("kind: %s should read as a component", cfg.Kind)
	}
	if cfg.Component.Name != "backend-clean-java" || cfg.Component.Lang != LangJava {
		t.Fatalf("component block round-tripped wrong: %+v", cfg.Component)
	}
	// The whole point of the kind: a component config carries no SUT cascade.
	if cfg.System.Architecture != "" || !cfg.SystemTest.IsEmpty() {
		t.Fatalf("component config should carry no system/system-test, got %+v / %+v", cfg.System, cfg.SystemTest)
	}
}

func TestWrite_ComponentRoundTrips(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := Write(dir, componentConfig()); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("round-tripped component config should validate, got: %v", err)
	}
	if cfg.Kind != KindComponent || cfg.Component != componentConfig().Component {
		t.Fatalf("round-trip lost data: kind=%q component=%+v", cfg.Kind, cfg.Component)
	}
}

// TestMarshal_ComponentOmitsSystemBlocks pins that a written component config
// is honest on disk: no empty system / system-test stubs implying tiers it
// does not have.
func TestMarshal_ComponentOmitsSystemBlocks(t *testing.T) {
	t.Parallel()
	out, err := Marshal(componentConfig())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(out)
	for _, unwanted := range []string{"system:", "system-test:", "channels:", "external-systems:"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("component config should not emit %q; got:\n%s", unwanted, body)
		}
	}
	for _, wanted := range []string{"kind: component", "component:", "name: backend-clean-java"} {
		if !strings.Contains(body, wanted) {
			t.Fatalf("component config should emit %q; got:\n%s", wanted, body)
		}
	}
}

func TestValidate_ComponentRequiresAllFourFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		field string
		mut   func(*Config)
	}{
		{"component.name", func(c *Config) { c.Component.Name = "" }},
		{"component.path", func(c *Config) { c.Component.Path = "" }},
		{"component.repo", func(c *Config) { c.Component.Repo = "" }},
		{"component.lang", func(c *Config) { c.Component.Lang = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			cfg := componentConfig()
			tc.mut(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("empty %s should be rejected", tc.field)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("error should name %s, got: %v", tc.field, err)
			}
		})
	}
}

func TestValidate_ComponentEnumAndPathRules(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		mut  func(*Config)
		want string
	}{
		{"unknown lang", func(c *Config) { c.Component.Lang = "cobol" }, "component.lang"},
		{"absolute path", func(c *Config) { c.Component.Path = "/abs/backend" }, "component.path"},
		{"escaping path", func(c *Config) { c.Component.Path = "../outside" }, "component.path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := componentConfig()
			tc.mut(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("%s should be rejected", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error should name %s, got: %v", tc.want, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The two shapes are exclusive, in both directions.
// ---------------------------------------------------------------------------

func TestValidate_ComponentRejectsSystemBlocks(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		mut   func(*Config)
		field string
	}{
		{"system.architecture", func(c *Config) { c.System.Architecture = ArchMonolith }, "system:"},
		{"system.backend", func(c *Config) {
			c.System.Backend = TierSpec{Path: "backend", Repo: "o/r", Lang: LangJava}
		}, "system:"},
		{"system.db-migration-path", func(c *Config) { c.System.DbMigrationPath = "system/db/migrations" }, "system:"},
		{"system-test", func(c *Config) {
			c.SystemTest = TierSpec{Path: "system-test", Repo: "o/r", Lang: LangJava}
		}, "system-test:"},
		{"system-test.paths", func(c *Config) { c.SystemTest.Paths = map[string]string{"at-test": "x"} }, "system-test:"},
		{"channels", func(c *Config) { c.Channels = []string{"api"} }, "channels:"},
		{"external-systems", func(c *Config) {
			c.ExternalSystems = ExternalSystems{"erp": ExternalSystem{}}
		}, "external-systems:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := componentConfig()
			tc.mut(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("kind: component should reject %s", tc.name)
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.field) {
				t.Fatalf("error should name the offending block %q, got: %v", tc.field, err)
			}
			// The message must route the operator to the kind that owns the
			// block, not merely refuse it.
			if !strings.Contains(msg, "kind: "+KindSystem) {
				t.Fatalf("error should point at kind: %s, got: %v", KindSystem, err)
			}
		})
	}
}

func TestValidate_SystemRejectsComponentBlock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, sampleMonoRepoMonolith+`
component:
  name: backend
  path: system/monolith/java
  repo: optivem/shop
  lang: java
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("kind: system should reject a component: block")
	}
	if !strings.Contains(err.Error(), "component:") || !strings.Contains(err.Error(), "kind: "+KindComponent) {
		t.Fatalf("error should name component: and point at kind: %s, got: %v", KindComponent, err)
	}
}

func TestValidate_RejectsUnknownKind(t *testing.T) {
	t.Parallel()
	cfg := componentConfig()
	cfg.Kind = "service"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("unknown kind should be rejected")
	}
	for _, want := range []string{"service", KindSystem, KindComponent} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should mention %q, got: %v", want, err)
		}
	}
}

// TestValidate_ExplicitKindSystemIsUnchanged pins that writing the default out
// explicitly (which gh optivem config migrate now does) changes nothing.
func TestValidate_ExplicitKindSystemIsUnchanged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, "kind: system\n\n"+sampleMonoRepoMultitier)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("explicit kind: system should validate exactly as absent does, got: %v", err)
	}
	if cfg.IsComponent() {
		t.Fatal("kind: system must not read as a component")
	}
}

// TestRepos_IncludesComponentRepo pins that a component project's repo is
// discoverable through the same accessor every consumer uses to resolve local
// clones. Without it, preflight's tier-path check has no host clone to join
// component.path onto and silently reports a vacuous pass.
func TestRepos_IncludesComponentRepo(t *testing.T) {
	t.Parallel()
	got := componentConfig().Repos()
	if len(got) != 1 || got[0] != "optivem/shop" {
		t.Fatalf("Repos() = %v, want [optivem/shop]", got)
	}
}
