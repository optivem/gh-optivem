package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/build/componenttest"
	"github.com/optivem/gh-optivem/internal/config/configinit"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// componentYAML is the on-disk kind: component config the guard tests point
// projectConfigPath at.
const componentYAML = `kind: component

project:
  provider: github

component:
  name: backend-clean-java
  path: system/multitier/backend-clean-java
  repo: optivem/shop
  lang: java
`

// systemYAML is a minimal kind: system config — enough to load and validate,
// with no architecture set (the partial-config shape Validate accepts). It
// stands in for "any project that is not a component".
const systemYAML = `project:
  provider: github
`

// withProjectConfig writes body to a temp file and points the package-level
// projectConfigPath (the persistent --config flag's backing variable) at it
// for the duration of the test, restoring the previous value afterwards.
func withProjectConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), projectconfig.Path)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	prev := projectConfigPath
	projectConfigPath = path
	t.Cleanup(func() { projectConfigPath = prev })
	return path
}

// ---------------------------------------------------------------------------
// The guard itself.
// ---------------------------------------------------------------------------

// TestRequireSystemKind_RefusesComponent pins the whole contract of the
// refusal message: it names the verb, the config, the kind, the reason, and
// the verbs that DO apply. Each of those is load-bearing — an operator who
// hits this needs to know what to run instead, not just that they were
// refused.
func TestRequireSystemKind_RefusesComponent(t *testing.T) {
	path := withProjectConfig(t, componentYAML)
	err := requireSystemKind("gh optivem system start", "there is no compose stack to bring up")
	if err == nil {
		t.Fatal("a kind: component project should refuse a SUT-only verb")
	}
	msg := err.Error()
	for _, want := range []string{
		"gh optivem system start",
		path,
		"kind: component",
		"there is no compose stack to bring up",
		"gh optivem component-test run",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal should mention %q, got:\n%s", want, msg)
		}
	}
}

func TestRequireSystemKind_AllowsSystem(t *testing.T) {
	withProjectConfig(t, systemYAML)
	if err := requireSystemKind("gh optivem system start", "no stack"); err != nil {
		t.Fatalf("a kind: system project should not be refused, got: %v", err)
	}
}

// TestRequireSystemKind_SilentOnUnreadableConfig pins the delegation rule: an
// absent or invalid config is the verb's own error to report (with its own
// recovery wording), not the guard's. Returning an error here would replace a
// specific "no gh-optivem.yaml; run config init" with a misleading kind
// message.
func TestRequireSystemKind_SilentOnUnreadableConfig(t *testing.T) {
	cases := []struct {
		name string
		set  func(t *testing.T)
	}{
		{"missing file", func(t *testing.T) {
			prev := projectConfigPath
			projectConfigPath = filepath.Join(t.TempDir(), "absent.yaml")
			t.Cleanup(func() { projectConfigPath = prev })
		}},
		{"invalid config", func(t *testing.T) {
			// No project.provider — Validate Rule 19 rejects it.
			withProjectConfig(t, "kind: component\n")
		}},
		{"malformed yaml", func(t *testing.T) {
			withProjectConfig(t, "kind: [unclosed\n")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.set(t)
			if err := requireSystemKind("gh optivem system start", "no stack"); err != nil {
				t.Fatalf("guard should stay silent and let the verb report, got: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Component discovery and compile dispatch.
// ---------------------------------------------------------------------------

func TestDiscoverComponents_Component(t *testing.T) {
	cfg := &projectconfig.Config{
		Kind: projectconfig.KindComponent,
		Component: projectconfig.Component{
			Name: "backend-clean-java",
			Path: "system/multitier/backend-clean-java",
			Repo: "optivem/shop",
			Lang: "java",
		},
	}
	got := discoverComponents(cfg)
	want := []componenttest.Component{
		{Name: "backend-clean-java", Path: "system/multitier/backend-clean-java", Lang: "java"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("component discovery = %+v, want %+v", got, want)
	}
}

func TestComponentTier_ProjectsComponentOntoTierSpec(t *testing.T) {
	cfg := &projectconfig.Config{
		Kind: projectconfig.KindComponent,
		Component: projectconfig.Component{
			Name: "backend-clean-java",
			Path: "system/multitier/backend-clean-java",
			Repo: "optivem/shop",
			Lang: "java",
		},
	}
	got := componentTier(cfg)
	want := projectconfig.TierSpec{
		Path: "system/multitier/backend-clean-java",
		Repo: "optivem/shop",
		Lang: "java",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("componentTier = %+v, want %+v", got, want)
	}
}

// TestCompilePhaseLabel pins that a component project never prints a phase
// named after a tier it does not have.
func TestCompilePhaseLabel(t *testing.T) {
	component := &projectconfig.Config{Kind: projectconfig.KindComponent}
	if got := compilePhaseLabel(component); got != labelCompileComponent {
		t.Errorf("component label = %q, want %q", got, labelCompileComponent)
	}
	for _, cfg := range []*projectconfig.Config{
		{},
		{Kind: projectconfig.KindSystem},
	} {
		if got := compilePhaseLabel(cfg); got != labelCompileSystem {
			t.Errorf("system label for %+v = %q, want %q", cfg, got, labelCompileSystem)
		}
	}
}

// ---------------------------------------------------------------------------
// config init --kind, and the migrate back-fill.
// ---------------------------------------------------------------------------

func TestValidateConfigInitKind(t *testing.T) {
	for _, ok := range []string{projectconfig.KindSystem, projectconfig.KindComponent} {
		if err := validateConfigInitKind(ok); err != nil {
			t.Errorf("--kind %s should be accepted, got: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "service", "System"} {
		err := validateConfigInitKind(bad)
		if err == nil {
			t.Errorf("--kind %q should be rejected", bad)
			continue
		}
		if !strings.Contains(err.Error(), projectconfig.KindComponent) {
			t.Errorf("rejection should list the valid set, got: %v", err)
		}
	}
}

// TestConfigInitComponent_WritesLoadableConfig closes the loop: the file
// `config init --kind component` writes is one `config validate` accepts and
// the runner discovers a component from.
func TestConfigInitComponent_WritesLoadableConfig(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, projectconfig.Path)
	written, err := configinit.RunComponent(configinit.ComponentFlags{
		Name:     "backend-clean-java",
		Path:     "system/multitier/backend-clean-java",
		Repo:     "optivem/shop",
		Lang:     "java",
		Provider: projectconfig.ProviderGitHub,
	}, yamlPath, false)
	if err != nil {
		t.Fatalf("RunComponent: %v", err)
	}
	if written != yamlPath {
		t.Fatalf("wrote %q, want %q", written, yamlPath)
	}
	cfg, err := projectconfig.LoadFromPath(yamlPath)
	if err != nil {
		t.Fatalf("the written config must validate, got: %v", err)
	}
	if !cfg.IsComponent() {
		t.Fatal("the written config should be kind: component")
	}
	if got := discoverComponents(cfg); len(got) != 1 || got[0].Name != "backend-clean-java" {
		t.Fatalf("runner should discover the declared component, got %+v", got)
	}
}

func TestConfigInitComponent_RejectsIncompleteFlags(t *testing.T) {
	yamlPath := filepath.Join(t.TempDir(), projectconfig.Path)
	_, err := configinit.RunComponent(configinit.ComponentFlags{
		Name:     "backend-clean-java",
		Provider: projectconfig.ProviderGitHub,
	}, yamlPath, false)
	if err == nil {
		t.Fatal("a component config missing path/repo/lang should be refused before the write")
	}
	if _, statErr := os.Stat(yamlPath); statErr == nil {
		t.Fatal("nothing should be written when validation fails")
	}
}

func TestConfigInitComponent_RefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, projectconfig.Path)
	flags := configinit.ComponentFlags{
		Name:     "backend-clean-java",
		Path:     "system/multitier/backend-clean-java",
		Repo:     "optivem/shop",
		Lang:     "java",
		Provider: projectconfig.ProviderGitHub,
	}
	if _, err := configinit.RunComponent(flags, yamlPath, false); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := configinit.RunComponent(flags, yamlPath, false); err == nil {
		t.Fatal("a second write without --force should be refused")
	}
	if _, err := configinit.RunComponent(flags, yamlPath, true); err != nil {
		t.Fatalf("--force should overwrite, got: %v", err)
	}
}

// TestConfigMigrate_BackfillsKindSystem pins that migrate makes the implicit
// default explicit, is idempotent, and does not change what the config means.
func TestConfigMigrate_BackfillsKindSystem(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, projectconfig.Path)
	if err := os.WriteFile(yamlPath, []byte(systemYAML), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	changed, err := runConfigMigrate(yamlPath)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !changed {
		t.Fatal("a config with no kind: should be migrated")
	}
	body, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.HasPrefix(string(body), "kind: "+projectconfig.KindSystem) {
		t.Fatalf("kind: should be the first line, got:\n%s", body)
	}
	cfg, err := projectconfig.LoadFromPath(yamlPath)
	if err != nil {
		t.Fatalf("migrated config should still validate, got: %v", err)
	}
	if cfg.IsComponent() {
		t.Fatal("migrate must not change what the config means")
	}

	changed, err = runConfigMigrate(yamlPath)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if changed {
		t.Fatal("migrate should be idempotent once kind: is present")
	}
}

// TestConfigMigrate_LeavesComponentKindAlone pins that the back-fill never
// rewrites an explicit component declaration into a system one.
func TestConfigMigrate_LeavesComponentKindAlone(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, projectconfig.Path)
	if err := os.WriteFile(yamlPath, []byte(componentYAML), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := runConfigMigrate(yamlPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg, err := projectconfig.LoadFromPath(yamlPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.IsComponent() {
		t.Fatalf("migrate rewrote kind: component to %q", cfg.Kind)
	}
}
