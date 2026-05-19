package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/optivem/gh-optivem/internal/atdd"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// TestProcessScope_AllPhases_NoProject asserts the no-arg listing
// surfaces every phase in phase-scopes.yaml plus the deferred allowlist,
// printing layer NAMES (no resolved paths) when no gh-optivem.yaml is
// reachable. Uses a temp dir + explicit configPath so the test never
// depends on the real cwd's project state.
func TestProcessScope_AllPhases_NoProject(t *testing.T) {
	scopes, err := atdd.LoadPhaseScopes()
	if err != nil {
		t.Fatalf("LoadPhaseScopes: %v", err)
	}

	tmp := t.TempDir()
	var buf bytes.Buffer
	if err := runProcessScope(&buf, "", filepath.Join(tmp, "no-such-config.yaml")); err != nil {
		t.Fatalf("runProcessScope: %v", err)
	}
	out := buf.String()

	for phaseID := range scopes.Phases {
		if !strings.Contains(out, "Phase:  "+phaseID) {
			t.Errorf("expected phase %q in output, got:\n%s", phaseID, out)
		}
	}
	for phaseID := range atdd.PhasesDeferredByPlan {
		if !strings.Contains(out, phaseID) {
			t.Errorf("expected deferred entry %q in output, got:\n%s", phaseID, out)
		}
	}
	if !strings.Contains(out, "Deferred — scope not yet declared") {
		t.Errorf("expected deferred section header in output, got:\n%s", out)
	}
}

// TestProcessScope_OnePhase_NoProject narrows to a single known phase
// and asserts layers print bare (no paths).
func TestProcessScope_OnePhase_NoProject(t *testing.T) {
	tmp := t.TempDir()
	var buf bytes.Buffer
	if err := runProcessScope(&buf, "AT_RED_TEST", filepath.Join(tmp, "no-such-config.yaml")); err != nil {
		t.Fatalf("runProcessScope: %v", err)
	}
	out := buf.String()

	wantSubs := []string{"Phase:  AT_RED_TEST", "Agent:  at-red-test", "at_test", "dsl_port", "dsl_core"}
	for _, sub := range wantSubs {
		if !strings.Contains(out, sub) {
			t.Errorf("expected %q in output, got:\n%s", sub, out)
		}
	}
	// Without a project, layers shouldn't carry trailing path values.
	if strings.Contains(out, "system-test/") {
		t.Errorf("did not expect resolved path in no-project output, got:\n%s", out)
	}
}

// TestProcessScope_DeferredPhase asserts an allowlist phase prints its
// deferred-plan citation instead of layer rows.
func TestProcessScope_DeferredPhase(t *testing.T) {
	tmp := t.TempDir()
	var buf bytes.Buffer
	if err := runProcessScope(&buf, "AT_GREEN_BACKEND", filepath.Join(tmp, "no-such-config.yaml")); err != nil {
		t.Fatalf("runProcessScope: %v", err)
	}
	out := buf.String()

	wantSubs := []string{"Phase:  AT_GREEN_BACKEND", "deferred", "plans/deferred/20260518-1530-multitier-green-scope.md"}
	for _, sub := range wantSubs {
		if !strings.Contains(out, sub) {
			t.Errorf("expected %q in output, got:\n%s", sub, out)
		}
	}
}

// TestProcessScope_UnknownPhase asserts a typo'd phase id fails loudly
// rather than emitting nothing.
func TestProcessScope_UnknownPhase(t *testing.T) {
	tmp := t.TempDir()
	var buf bytes.Buffer
	err := runProcessScope(&buf, "NOT_A_PHASE", filepath.Join(tmp, "no-such-config.yaml"))
	if err == nil {
		t.Fatalf("expected error for unknown phase, got nil and output:\n%s", buf.String())
	}
	if !strings.Contains(err.Error(), "NOT_A_PHASE") {
		t.Errorf("expected error to name the phase, got: %v", err)
	}
}

// TestProcessScope_ResolvesAgainstProjectPaths writes a minimal
// gh-optivem.yaml to a temp dir, points the command at it, and asserts
// Family A `system_path` resolves via system.path and Family B layers
// resolve via paths:.
func TestProcessScope_ResolvesAgainstProjectPaths(t *testing.T) {
	cfg := minimalMonolithConfig(t)
	path := writeConfigToTempDir(t, cfg)

	var buf bytes.Buffer
	if err := runProcessScope(&buf, "AT_RED_TEST", path); err != nil {
		t.Fatalf("runProcessScope: %v", err)
	}
	out := buf.String()

	wantSubs := []string{
		"at_test",
		"dsl_port",
		"dsl_core",
		cfg.Paths["at_test"],
		cfg.Paths["dsl_port"],
		cfg.Paths["dsl_core"],
	}
	for _, sub := range wantSubs {
		if !strings.Contains(out, sub) {
			t.Errorf("expected %q in output, got:\n%s", sub, out)
		}
	}
}

// TestProcessScope_SystemPathReadsFamilyA asserts the Family A
// `system_path` layer resolves via system.path, not paths:.
func TestProcessScope_SystemPathReadsFamilyA(t *testing.T) {
	cfg := minimalMonolithConfig(t)
	path := writeConfigToTempDir(t, cfg)

	var buf bytes.Buffer
	if err := runProcessScope(&buf, "AT_GREEN_SYSTEM", path); err != nil {
		t.Fatalf("runProcessScope: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, cfg.System.Path) {
		t.Errorf("expected system.path %q in AT_GREEN_SYSTEM output, got:\n%s", cfg.System.Path, out)
	}
}

// minimalMonolithConfig returns a Config that satisfies projectconfig
// Validate for a monolith TypeScript project. The fixed values are
// chosen to be distinct enough that the test can grep them out of the
// command's output without false positives.
func minimalMonolithConfig(t *testing.T) *projectconfig.Config {
	t.Helper()
	return &projectconfig.Config{
		Project: projectconfig.Project{
			Provider: "github",
		},
		System: projectconfig.System{
			Path: "src/shop",
			Repo: "myorg/shop",
			Lang: "typescript",
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test",
			Repo: "myorg/shop",
			Lang: "typescript",
		},
		Paths: map[string]string{
			"at_test":                        "system-test/src/atdd/shop",
			"dsl_port":                       "system-test/src/testkit/dsl/port/shop",
			"dsl_core":                       "system-test/src/testkit/dsl/core/shop",
			"driver_port":                    "system-test/src/testkit/driver/port/shop",
			"driver_adapter":                 "system-test/src/testkit/driver/adapter/shop",
			"external_system_driver_port":    "system-test/src/testkit/external/driver/port/shop",
			"external_system_driver_adapter": "system-test/src/testkit/external/driver/adapter/shop",
		},
	}
}

// writeConfigToTempDir marshals cfg to <tempdir>/gh-optivem.yaml and
// returns the file path. Uses yaml.Marshal directly (not projectconfig.
// WriteToPath) so the test stays close to the LoadFromPath contract
// without coupling to Write's validation pre-flight.
func writeConfigToTempDir(t *testing.T, cfg *projectconfig.Config) string {
	t.Helper()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "gh-optivem.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
