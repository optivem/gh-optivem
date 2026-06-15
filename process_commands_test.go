package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// TestProcessScope_AllPhases_NoProject asserts the no-arg listing
// surfaces every writing-agent MID with inline scope from
// process-flow.yaml, printing layer NAMES (no resolved paths) when no
// gh-optivem.yaml is reachable. Uses a temp dir + explicit configPath
// so the test never depends on the real cwd's project state.
func TestProcessScope_AllPhases_NoProject(t *testing.T) {
	eng, err := statemachine.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}

	tmp := t.TempDir()
	var buf bytes.Buffer
	if err := runProcessScope(&buf, "", filepath.Join(tmp, "no-such-config.yaml")); err != nil {
		t.Fatalf("runProcessScope: %v", err)
	}
	out := buf.String()

	for _, id := range writingAgentMIDs(eng) {
		if _, _, ok := eng.Scope(id); !ok {
			continue
		}
		if !strings.Contains(out, "Phase:  "+id) {
			t.Errorf("expected phase %q in output, got:\n%s", id, out)
		}
	}
}

// TestProcessScope_OnePhase_NoProject narrows to a single known
// writing-agent MID and asserts layers print bare (no paths).
func TestProcessScope_OnePhase_NoProject(t *testing.T) {
	tmp := t.TempDir()
	var buf bytes.Buffer
	if err := runProcessScope(&buf, "write-acceptance-tests", filepath.Join(tmp, "no-such-config.yaml")); err != nil {
		t.Fatalf("runProcessScope: %v", err)
	}
	out := buf.String()

	wantSubs := []string{"Phase:  write-acceptance-tests", "Agent:  write-acceptance-tests", "at-test", "dsl-port", "dsl-core"}
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
// Family B layers resolve via paths:.
func TestProcessScope_ResolvesAgainstProjectPaths(t *testing.T) {
	cfg := minimalMonolithConfig(t)
	path := writeConfigToTempDir(t, cfg)

	var buf bytes.Buffer
	if err := runProcessScope(&buf, "write-acceptance-tests", path); err != nil {
		t.Fatalf("runProcessScope: %v", err)
	}
	out := buf.String()

	wantSubs := []string{
		"at-test",
		"dsl-port",
		"dsl-core",
		cfg.SystemTest.Paths["at-test"],
		cfg.SystemTest.Paths["dsl-port"],
		cfg.SystemTest.Paths["dsl-core"],
	}
	for _, sub := range wantSubs {
		if !strings.Contains(out, sub) {
			t.Errorf("expected %q in output, got:\n%s", sub, out)
		}
	}
}

// TestProcessScope_SystemPathReadsFamilyA asserts the Family A
// `system-path` layer resolves via system.path, not paths:.
func TestProcessScope_SystemPathReadsFamilyA(t *testing.T) {
	cfg := minimalMonolithConfig(t)
	path := writeConfigToTempDir(t, cfg)

	var buf bytes.Buffer
	if err := runProcessScope(&buf, "implement-system", path); err != nil {
		t.Fatalf("runProcessScope: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, cfg.System.Path) {
		t.Errorf("expected system.path %q in implement-system output, got:\n%s", cfg.System.Path, out)
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
			Paths: map[string]string{
				"at-test":                        "system-test/src/atdd/shop",
				"dsl-port":                       "system-test/src/testkit/dsl/port/shop",
				"dsl-core":                       "system-test/src/testkit/dsl/core/shop",
				"system-driver-port":                    "system-test/src/testkit/driver/port/shop",
				"system-driver-adapter":                 "system-test/src/testkit/driver/adapter/shop",
				"external-system-driver-port":    "system-test/src/testkit/external/driver/port/shop",
				"external-system-driver-adapter": "system-test/src/testkit/external/driver/adapter/shop",
			},
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
