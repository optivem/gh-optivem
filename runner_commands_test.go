package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// TestNewTestSystemCmdRepeatableTestFlag verifies cobra's StringSliceVar
// wiring on --test: the flag is repeatable AND comma-separated, and an
// absent flag yields an empty slice (no filter).
func TestNewTestSystemCmdRepeatableTestFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"repeated --test", []string{"--test", "T1", "--test", "T2"}, []string{"T1", "T2"}},
		{"comma-separated value", []string{"--test", "T1,T2"}, []string{"T1", "T2"}},
		{"mixed repeat + comma", []string{"--test", "T1,T2", "--test", "T3"}, []string{"T1", "T2", "T3"}},
		{"single value", []string{"--test", "Only"}, []string{"Only"}},
		{"flag absent", []string{}, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := newTestSystemCmd()
			if err := cmd.ParseFlags(c.args); err != nil {
				t.Fatalf("ParseFlags(%v): %v", c.args, err)
			}
			got, err := cmd.Flags().GetStringSlice("test")
			if err != nil {
				t.Fatalf("GetStringSlice: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

// TestNewTestSystemCmdRepeatableSuiteFlag verifies cobra's StringSliceVar
// wiring on --suite: the flag is repeatable AND comma-separated, and an
// absent flag yields an empty slice (run-all-suites behavior).
func TestNewTestSystemCmdRepeatableSuiteFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"repeated --suite", []string{"--suite", "A", "--suite", "B"}, []string{"A", "B"}},
		{"comma-separated value", []string{"--suite", "A,B"}, []string{"A", "B"}},
		{"mixed repeat + comma", []string{"--suite", "A,B", "--suite", "C"}, []string{"A", "B", "C"}},
		{"single value", []string{"--suite", "Only"}, []string{"Only"}},
		{"flag absent", []string{}, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := newTestSystemCmd()
			if err := cmd.ParseFlags(c.args); err != nil {
				t.Fatalf("ParseFlags(%v): %v", c.args, err)
			}
			got, err := cmd.Flags().GetStringSlice("suite")
			if err != nil {
				t.Fatalf("GetStringSlice: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

// TestValidateSuiteTestCombo guards the rejection path for the
// --test + multi-`--suite` combination. The helper exists precisely so
// this can be exercised without mocking os.Exit.
func TestValidateSuiteTestCombo(t *testing.T) {
	cases := []struct {
		name    string
		suites  []string
		tests   []string
		wantErr bool
	}{
		{"no suites, no tests", nil, nil, false},
		{"no suites, one test", nil, []string{"T1"}, false},
		{"single suite, no tests", []string{"A"}, nil, false},
		{"single suite, one test", []string{"A"}, []string{"T1"}, false},
		{"single suite, multiple tests", []string{"A"}, []string{"T1", "T2"}, false},
		{"multiple suites, no tests", []string{"A", "B"}, nil, false},
		{"multiple suites, one test (rejected)", []string{"A", "B"}, []string{"T1"}, true},
		{"multiple suites, multiple tests (rejected)", []string{"A", "B"}, []string{"T1", "T2"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateSuiteTestCombo(c.suites, c.tests)
			if (err != nil) != c.wantErr {
				t.Fatalf("got err=%v, wantErr=%v", err, c.wantErr)
			}
			if err == nil {
				return
			}
			msg := err.Error()
			for _, want := range []string{"--test", "--suite", "narrow to a single --suite"} {
				if !strings.Contains(msg, want) {
					t.Errorf("error message missing %q hint, got: %v", want, err)
				}
			}
		})
	}
}

// TestLoadSystemMissingFileHintsAtYAML verifies that `gh optivem build/run/...`
// commands surface the two-knob hint (system_config: YAML field, default
// path) when the resolved systems.json is absent — the case a new user runs
// into first.
func TestLoadSystemMissingFileHintsAtYAML(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "systems.json")
	_, err := loadSystem(missing)
	if err == nil {
		t.Fatalf("loadSystem(%q): want error, got nil", missing)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("loadSystem: want errors.Is(err, fs.ErrNotExist), got %v", err)
	}
	for _, want := range []string{"system_config", defaultSystemConfig} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("loadSystem error missing %q hint: %v", want, err)
		}
	}
}

// TestLoadTestsMissingFileHintsAtYAML mirrors the two-knob hint check for
// test_config: / defaultTestsConfig.
func TestLoadTestsMissingFileHintsAtYAML(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "tests.json")
	_, err := loadTests(missing)
	if err == nil {
		t.Fatalf("loadTests(%q): want error, got nil", missing)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("loadTests: want errors.Is(err, fs.ErrNotExist), got %v", err)
	}
	for _, want := range []string{"test_config", defaultTestsConfig} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("loadTests error missing %q hint: %v", want, err)
		}
	}
}

// TestHintIfMissingPassesThrough ensures non-ENOENT errors come through
// unchanged (e.g. JSON parse errors keep their original message and don't
// gain a misleading hint).
func TestHintIfMissingPassesThrough(t *testing.T) {
	want := errors.New("parse system config: bad json")
	got := hintIfMissing(want, "system_config", defaultSystemConfig)
	if got != want {
		t.Errorf("hintIfMissing rewrote a non-ENOENT error: got %v, want %v", got, want)
	}
	if hintIfMissing(nil, "system_config", defaultSystemConfig) != nil {
		t.Errorf("hintIfMissing(nil) returned non-nil")
	}
}

// runnerResolveSetup neutralizes the persistent --config / $GH_OPTIVEM_CONFIG
// state so a resolver test sees only the YAML it explicitly seeded in tmpDir.
// Sets up: $GH_OPTIVEM_CONFIG = <tmpDir>/gh-optivem.yaml (so projectconfig's
// resolver picks the seeded file), projectConfigPath cleared, original
// projectConfigPath restored via t.Cleanup. Callers seed the YAML themselves.
func runnerResolveSetup(t *testing.T, tmpDir string) {
	t.Helper()
	t.Setenv(projectconfig.EnvVar, filepath.Join(tmpDir, projectconfig.Path))
	saved := projectConfigPath
	projectConfigPath = ""
	t.Cleanup(func() { projectConfigPath = saved })
}

// writeYAMLConfig persists pc to tmpDir/gh-optivem.yaml so the resolver under
// test can pick it up via $GH_OPTIVEM_CONFIG. Skips Validate (the tests want
// to exercise the bare system_config: / test_config: fields without setting
// up a full architecture-shaped config).
func writeYAMLConfig(t *testing.T, tmpDir string, systemConfig, testConfig string) {
	t.Helper()
	// project.url is mandatory in any loadable gh-optivem.yaml; the value
	// is irrelevant to these path-resolution tests but must be present so
	// projectconfig.Load returns a non-nil cfg rather than a Validate error.
	body := "project:\n  url: https://github.com/orgs/acme/projects/1\n"
	if systemConfig != "" {
		body += "system_config: " + systemConfig + "\n"
	}
	if testConfig != "" {
		body += "test_config: " + testConfig + "\n"
	}
	if err := os.WriteFile(filepath.Join(tmpDir, projectconfig.Path), []byte(body), 0o644); err != nil {
		t.Fatalf("seed gh-optivem.yaml: %v", err)
	}
}

// TestResolveSystemPath_YAMLUsedWhenSet: YAML field set → YAML value.
func TestResolveSystemPath_YAMLUsedWhenSet(t *testing.T) {
	tmp := t.TempDir()
	runnerResolveSetup(t, tmp)
	writeYAMLConfig(t, tmp, "yaml/systems.json", "")

	got, err := resolveSystemPath()
	if err != nil {
		t.Fatalf("resolveSystemPath: %v", err)
	}
	if got != "yaml/systems.json" {
		t.Errorf("got %q, want yaml/systems.json (YAML field should be used)", got)
	}
}

// TestResolveSystemPath_EmptyYAMLFallsThrough: YAML field empty
// → defaultSystemConfig.
func TestResolveSystemPath_EmptyYAMLFallsThrough(t *testing.T) {
	tmp := t.TempDir()
	runnerResolveSetup(t, tmp)
	writeYAMLConfig(t, tmp, "", "")

	got, err := resolveSystemPath()
	if err != nil {
		t.Fatalf("resolveSystemPath: %v", err)
	}
	if got != defaultSystemConfig {
		t.Errorf("got %q, want %q (empty YAML field must fall through)", got, defaultSystemConfig)
	}
}

// TestResolveSystemPath_MissingYAMLDefaultLocation: no YAML file at the
// default location (and no explicit --config / env) → defaultSystemConfig.
// Runner subcommands must keep working in repos that have no gh-optivem.yaml.
func TestResolveSystemPath_MissingYAMLDefaultLocation(t *testing.T) {
	tmp := t.TempDir()
	// Point cwd at tmp and leave $GH_OPTIVEM_CONFIG unset so the resolver's
	// default branch fires (cwd/gh-optivem.yaml, !explicit).
	t.Setenv(projectconfig.EnvVar, "")
	saved := projectConfigPath
	projectConfigPath = ""
	t.Cleanup(func() { projectConfigPath = saved })

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	got, err := resolveSystemPath()
	if err != nil {
		t.Fatalf("resolveSystemPath: %v", err)
	}
	if got != defaultSystemConfig {
		t.Errorf("got %q, want %q (missing default-location YAML must fall through)", got, defaultSystemConfig)
	}
}

// TestResolveTestsPath_YAMLUsedWhenSet mirrors the system case.
func TestResolveTestsPath_YAMLUsedWhenSet(t *testing.T) {
	tmp := t.TempDir()
	runnerResolveSetup(t, tmp)
	writeYAMLConfig(t, tmp, "", "yaml/tests.json")

	got, err := resolveTestsPath()
	if err != nil {
		t.Fatalf("resolveTestsPath: %v", err)
	}
	if got != "yaml/tests.json" {
		t.Errorf("got %q, want yaml/tests.json (YAML field should be used)", got)
	}
}

// TestResolveTestsPath_EmptyYAMLFallsThrough mirrors the system case.
func TestResolveTestsPath_EmptyYAMLFallsThrough(t *testing.T) {
	tmp := t.TempDir()
	runnerResolveSetup(t, tmp)
	writeYAMLConfig(t, tmp, "", "")

	got, err := resolveTestsPath()
	if err != nil {
		t.Fatalf("resolveTestsPath: %v", err)
	}
	if got != defaultTestsConfig {
		t.Errorf("got %q, want %q (empty YAML field must fall through)", got, defaultTestsConfig)
	}
}
