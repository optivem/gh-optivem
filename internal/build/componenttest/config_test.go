package componenttest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeConfig writes body to a component-tests.yaml in a fresh temp dir and
// returns the file path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

const validConfig = `
setupCommands:
  - name: Install Dependencies
    command: npm ci
suites:
  - id: unit
    name: Unit
    command: npm test
    sampleTest: "test harness"
  - id: integration
    name: Narrow Integration
    pending: true
  - id: component
    name: Component
    command: npm run test:component
    sampleTest: "NewOrder"
  - id: contract
    name: Consumer Contract (Pact)
    command: npm run test:pact
suiteGroups:
  all: [unit, integration, component, contract]
`

func TestLoad_Valid(t *testing.T) {
	cfg, err := Load(writeConfig(t, validConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cfg.SuiteIDs(); !reflect.DeepEqual(got, []string{"unit", "integration", "component", "contract"}) {
		t.Errorf("SuiteIDs = %v", got)
	}
	if len(cfg.SetupCommands) != 1 || cfg.SetupCommands[0].Command != "npm ci" {
		t.Errorf("setupCommands = %+v", cfg.SetupCommands)
	}
	integration := cfg.FindSuite("integration")
	if integration == nil || !integration.Pending {
		t.Errorf("integration suite should be pending: %+v", integration)
	}
	if integration.Command != "" {
		t.Errorf("pending suite should have no command, got %q", integration.Command)
	}
}

func TestLoad_Errors(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantSub string
	}{
		{
			name:    "empty suites",
			body:    "setupCommands: []\n",
			wantSub: "suites[] is empty",
		},
		{
			name:    "missing id",
			body:    "suites:\n  - name: Unit\n    command: npm test\n",
			wantSub: "missing id",
		},
		{
			name:    "missing name",
			body:    "suites:\n  - id: unit\n    command: npm test\n",
			wantSub: "missing name",
		},
		{
			name:    "non-pending missing command",
			body:    "suites:\n  - id: unit\n    name: Unit\n",
			wantSub: "missing command",
		},
		{
			name:    "duplicate id",
			body:    "suites:\n  - id: unit\n    name: Unit\n    command: a\n  - id: unit\n    name: Unit2\n    command: b\n",
			wantSub: "duplicate suite id",
		},
		{
			name:    "backslash in path",
			body:    "suites:\n  - id: unit\n    name: Unit\n    command: npm test\n    path: src\\test\n",
			wantSub: "backslash separator",
		},
		{
			name:    "backslash in testReportPath",
			body:    "suites:\n  - id: unit\n    name: Unit\n    command: npm test\n    testReportPath: build\\report.html\n",
			wantSub: "backslash separator",
		},
		{
			name:    "setup missing command",
			body:    "setupCommands:\n  - name: Install\nsuites:\n  - id: unit\n    name: Unit\n    command: npm test\n",
			wantSub: "setupCommands[0] (Install) missing command",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, tc.body))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestLoad_PendingMissingCommandOK(t *testing.T) {
	body := "suites:\n  - id: integration\n    name: Narrow Integration\n    pending: true\n"
	if _, err := Load(writeConfig(t, body)); err != nil {
		t.Fatalf("pending suite without command should load, got: %v", err)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil || !strings.Contains(err.Error(), "read component-tests config") {
		t.Fatalf("expected read error, got: %v", err)
	}
}

func TestExpandGroups(t *testing.T) {
	cfg := &Config{
		SuiteGroups: map[string][]string{
			"all": {"unit", "component", "contract"},
		},
	}
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"group alias expands", []string{"all"}, []string{"unit", "component", "contract"}},
		{"non-group passes through", []string{"unit"}, []string{"unit"}},
		{"dedupe group + explicit", []string{"all", "unit"}, []string{"unit", "component", "contract"}},
		{"unknown passes through", []string{"bogus"}, []string{"bogus"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cfg.expandGroups(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("expandGroups(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestSelectSuites(t *testing.T) {
	cfg, err := Load(writeConfig(t, validConfig))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	t.Run("empty selects all in declaration order", func(t *testing.T) {
		got := suiteIDsOf(mustSelect(t, cfg, nil))
		want := []string{"unit", "integration", "component", "contract"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("group all expands", func(t *testing.T) {
		got := suiteIDsOf(mustSelect(t, cfg, []string{"all"}))
		want := []string{"unit", "integration", "component", "contract"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("explicit subset preserves declaration order", func(t *testing.T) {
		// Request out of declaration order; result must still be declaration order.
		got := suiteIDsOf(mustSelect(t, cfg, []string{"contract", "unit"}))
		want := []string{"unit", "contract"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("unknown suite fails loud", func(t *testing.T) {
		_, err := cfg.selectSuites([]string{"bogus"})
		if err == nil || !strings.Contains(err.Error(), "suite(s) not found: bogus") {
			t.Fatalf("expected not-found error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "Available:") {
			t.Errorf("error should list available suites, got: %v", err)
		}
	})
}

func mustSelect(t *testing.T, cfg *Config, req []string) []Suite {
	t.Helper()
	suites, err := cfg.selectSuites(req)
	if err != nil {
		t.Fatalf("selectSuites(%v): %v", req, err)
	}
	return suites
}

func suiteIDsOf(suites []Suite) []string {
	ids := make([]string, len(suites))
	for i, s := range suites {
		ids[i] = s.ID
	}
	return ids
}
