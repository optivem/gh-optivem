package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// TestNewTestRunCmdRepeatableTestFlag verifies cobra's StringSliceVar
// wiring on --test: the flag is repeatable AND comma-separated, and an
// absent flag yields an empty slice (no filter).
func TestNewTestRunCmdRepeatableTestFlag(t *testing.T) {
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
			cmd := newTestRunCmd()
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

// TestNewTestRunCmdRepeatableSuiteFlag verifies cobra's StringSliceVar
// wiring on --suite: the flag is repeatable AND comma-separated, and an
// absent flag yields an empty slice (run-all-suites behavior).
func TestNewTestRunCmdRepeatableSuiteFlag(t *testing.T) {
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
			cmd := newTestRunCmd()
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
// to exercise the bare system.config: / system_test.config: fields without
// setting up a full architecture-shaped config).
func writeYAMLConfig(t *testing.T, tmpDir string, systemConfig, testConfig string) {
	t.Helper()
	// project.url is mandatory in any loadable gh-optivem.yaml; the value
	// is irrelevant to these path-resolution tests but must be present so
	// projectconfig.Load returns a non-nil cfg rather than a Validate error.
	body := "project:\n  provider: github\n  url: https://github.com/orgs/acme/projects/1\n"
	if systemConfig != "" {
		body += "system:\n  config: " + systemConfig + "\n"
	}
	if testConfig != "" {
		body += "system_test:\n  config: " + testConfig + "\n"
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

// TestResolveSystemPath_EmptyYAMLErrors: gh-optivem.yaml exists but
// system.config: is empty → hard error pointing at the field plus --config.
// Runner commands no longer fall back to a default ./systems.yaml.
func TestResolveSystemPath_EmptyYAMLErrors(t *testing.T) {
	tmp := t.TempDir()
	runnerResolveSetup(t, tmp)
	writeYAMLConfig(t, tmp, "", "")

	_, err := resolveSystemPath()
	if err == nil {
		t.Fatalf("resolveSystemPath: want error, got nil")
	}
	for _, want := range []string{"system.config", "--config"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q hint: %v", want, err)
		}
	}
}

// TestResolveSystemPath_MissingYAMLErrors: no gh-optivem.yaml at the
// resolved path → hard error pointing the operator at `gh optivem config
// init` plus --config. Mirrors `gh optivem init`'s hard-error contract.
func TestResolveSystemPath_MissingYAMLErrors(t *testing.T) {
	tmp := t.TempDir()
	// Point $GH_OPTIVEM_CONFIG at a path inside tmp that we *don't* seed,
	// so the resolver attempts to load a file that doesn't exist.
	t.Setenv(projectconfig.EnvVar, filepath.Join(tmp, projectconfig.Path))
	saved := projectConfigPath
	projectConfigPath = ""
	t.Cleanup(func() { projectConfigPath = saved })

	_, err := resolveSystemPath()
	if err == nil {
		t.Fatalf("resolveSystemPath: want error, got nil")
	}
	for _, want := range []string{"no gh-optivem.yaml", "gh optivem config init", "--config"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q hint: %v", want, err)
		}
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

// TestResolveTestsPath_EmptyYAMLErrors mirrors the system case for
// system_test.config:.
func TestResolveTestsPath_EmptyYAMLErrors(t *testing.T) {
	tmp := t.TempDir()
	runnerResolveSetup(t, tmp)
	writeYAMLConfig(t, tmp, "", "")

	_, err := resolveTestsPath()
	if err == nil {
		t.Fatalf("resolveTestsPath: want error, got nil")
	}
	for _, want := range []string{"system_test.config", "--config"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q hint: %v", want, err)
		}
	}
}

// TestResolveTestsPath_MissingYAMLErrors mirrors the system case for a
// missing gh-optivem.yaml.
func TestResolveTestsPath_MissingYAMLErrors(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(projectconfig.EnvVar, filepath.Join(tmp, projectconfig.Path))
	saved := projectConfigPath
	projectConfigPath = ""
	t.Cleanup(func() { projectConfigPath = saved })

	_, err := resolveTestsPath()
	if err == nil {
		t.Fatalf("resolveTestsPath: want error, got nil")
	}
	for _, want := range []string{"no gh-optivem.yaml", "gh optivem config init", "--config"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q hint: %v", want, err)
		}
	}
}
