package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	systemJSONFilename = "systems.json"
	testsJSONFilename  = "tests.json"
	systemYAMLFilename = "systems.yaml"
	testsYAMLFilename  = "tests.yaml"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestLoadSystemHappyPath(t *testing.T) {
	path := writeTempFile(t, systemJSONFilename, `{
		"systems": [
			{
				"label": "real",
				"composeFile": "monolith/docker-compose.local.real.yml",
				"containerName": "my-shop-real",
				"components": [{ "name": "Monolith", "url": "http://localhost:3311", "containerName": "system" }],
				"externalSystems": [{ "name": "ERP", "url": "http://localhost:9311/erp/health", "containerName": "external-real" }]
			},
			{
				"label": "stub",
				"composeFile": "monolith/docker-compose.local.stub.yml",
				"containerName": "my-shop-stub",
				"components": [],
				"externalSystems": []
			}
		]
	}`)
	cfg, err := LoadSystem(path)
	if err != nil {
		t.Fatalf("LoadSystem: %v", err)
	}
	if len(cfg.Systems) != 2 {
		t.Errorf("want 2 systems, got %d", len(cfg.Systems))
	}
	if cfg.Systems[0].Label != "real" {
		t.Errorf("want first label 'real', got %q", cfg.Systems[0].Label)
	}
	if cfg.Systems[0].Components[0].URL != "http://localhost:3311" {
		t.Errorf("component URL not parsed: %+v", cfg.Systems[0].Components[0])
	}
}

func TestLoadSystemEmptySystemsRejected(t *testing.T) {
	path := writeTempFile(t, systemJSONFilename, `{"systems":[]}`)
	_, err := LoadSystem(path)
	if err == nil || !strings.Contains(err.Error(), "systems[] is empty") {
		t.Errorf("want 'systems[] is empty' error, got: %v", err)
	}
}

func TestLoadSystemMissingComposeFileRejected(t *testing.T) {
	path := writeTempFile(t, systemJSONFilename, `{
		"systems": [{ "label": "x", "components": [], "externalSystems": [] }]
	}`)
	_, err := LoadSystem(path)
	if err == nil || !strings.Contains(err.Error(), "missing composeFile") {
		t.Errorf("want 'missing composeFile' error, got: %v", err)
	}
}

func TestLoadSystemFileNotFound(t *testing.T) {
	_, err := LoadSystem(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestLoadSystemInvalidJSON(t *testing.T) {
	path := writeTempFile(t, systemJSONFilename, `{"systems":[`)
	_, err := LoadSystem(path)
	if err == nil || !strings.Contains(err.Error(), "expected JSON or YAML file format") {
		t.Errorf("want format error, got: %v", err)
	}
}

// TestLoadSystemNonJSONFileRejected covers the case a user passes a
// docker-compose YAML to --system-config under a `.json` filename — the JSON
// codec runs (extension dispatch) and the parse error mentions both accepted
// formats so the operator knows renaming to .yaml is an option.
func TestLoadSystemNonJSONFileRejected(t *testing.T) {
	path := writeTempFile(t, systemJSONFilename, "services:\n  app:\n    image: example\n")
	_, err := LoadSystem(path)
	if err == nil || !strings.Contains(err.Error(), "expected JSON or YAML file format") {
		t.Errorf("want format error, got: %v", err)
	}
}

func TestLoadSystemMissingLabelRejected(t *testing.T) {
	path := writeTempFile(t, systemJSONFilename, `{
		"systems": [{ "composeFile": "x.yml" }]
	}`)
	_, err := LoadSystem(path)
	if err == nil || !strings.Contains(err.Error(), "missing label") {
		t.Errorf("want 'missing label' error, got: %v", err)
	}
}

func TestLoadSystemMissingComponentNameRejected(t *testing.T) {
	path := writeTempFile(t, systemJSONFilename, `{
		"systems": [{
			"label": "real",
			"composeFile": "x.yml",
			"components": [{ "url": "http://localhost:1" }]
		}]
	}`)
	_, err := LoadSystem(path)
	if err == nil || !strings.Contains(err.Error(), "components[0] missing name") {
		t.Errorf("want 'components[0] missing name' error, got: %v", err)
	}
}

func TestLoadSystemMissingExternalSystemURLRejected(t *testing.T) {
	path := writeTempFile(t, systemJSONFilename, `{
		"systems": [{
			"label": "real",
			"composeFile": "x.yml",
			"externalSystems": [{ "name": "ERP" }]
		}]
	}`)
	_, err := LoadSystem(path)
	if err == nil || !strings.Contains(err.Error(), "missing url") {
		t.Errorf("want 'missing url' error, got: %v", err)
	}
}

func TestLoadTestsHappyPath(t *testing.T) {
	path := writeTempFile(t, testsJSONFilename, `{
		"setupCommands": [{ "name": "Install", "command": "npm ci" }],
		"testFilter": "--grep '<test>'",
		"suites": [
			{
				"id": "smoke",
				"name": "Smoke",
				"command": "npx playwright test smoke",
				"env": { "MODE": "stub" },
				"path": ".",
				"testReportPath": "playwright-report/index.html",
				"sampleTest": "shouldWork"
			},
			{
				"id": "e2e",
				"name": "E2E",
				"command": "npx playwright test e2e"
			}
		]
	}`)
	cfg, err := LoadTests(path)
	if err != nil {
		t.Fatalf("LoadTests: %v", err)
	}
	if len(cfg.Suites) != 2 {
		t.Errorf("want 2 suites, got %d", len(cfg.Suites))
	}
	if cfg.Suites[0].Env["MODE"] != "stub" {
		t.Errorf("env not parsed: %+v", cfg.Suites[0].Env)
	}
	if cfg.TestFilter != "--grep '<test>'" {
		t.Errorf("testFilter not parsed: %q", cfg.TestFilter)
	}
}

// TestLoadTestsBackslashPathRejected is Layer 1 of the portable-path guard: a
// Windows-authored report path with backslash separators is rejected at config
// load — before any suite runs — so it can never silently fail to resolve on a
// Linux runner and read as "0 executed".
func TestLoadTestsBackslashPathRejected(t *testing.T) {
	for _, tc := range []struct{ name, field, line string }{
		{"testCountPath", "testCountPath", `"testCountPath": "build\\test-results\\test"`},
		{"testReportPath", "testReportPath", `"testReportPath": "build\\reports\\index.html"`},
		{"path", "path", `"path": "system-test\\java"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTempFile(t, testsJSONFilename, `{
				"suites": [{ "id": "smoke", "name": "Smoke", "command": "x", `+tc.line+` }]
			}`)
			_, err := LoadTests(path)
			if err == nil || !strings.Contains(err.Error(), "uses a backslash separator") {
				t.Errorf("want backslash-separator rejection for %s, got: %v", tc.field, err)
			}
			if err != nil && !strings.Contains(err.Error(), tc.field) {
				t.Errorf("error should name the offending field %q, got: %v", tc.field, err)
			}
		})
	}
}

// TestLoadTestsBackslashInCommandAllowed guards the carve-out: a suite command
// legitimately carries `.\gradlew.bat`, so backslashes there must NOT be
// rejected — only the filepath.Join'd path fields are checked.
func TestLoadTestsBackslashInCommandAllowed(t *testing.T) {
	path := writeTempFile(t, testsJSONFilename, `{
		"suites": [{ "id": "smoke", "name": "Smoke", "command": ".\\gradlew.bat test", "path": ".", "testReportPath": "build/reports/index.html" }]
	}`)
	if _, err := LoadTests(path); err != nil {
		t.Errorf("a backslash in command must be allowed, got: %v", err)
	}
}

func TestLoadTestsEmptySuitesRejected(t *testing.T) {
	path := writeTempFile(t, testsJSONFilename, `{"suites":[]}`)
	_, err := LoadTests(path)
	if err == nil || !strings.Contains(err.Error(), "suites[] is empty") {
		t.Errorf("want 'suites[] is empty' error, got: %v", err)
	}
}

func TestLoadTestsMissingIDRejected(t *testing.T) {
	path := writeTempFile(t, testsJSONFilename, `{
		"suites": [{ "name": "X", "command": "x" }]
	}`)
	_, err := LoadTests(path)
	if err == nil || !strings.Contains(err.Error(), "missing id") {
		t.Errorf("want 'missing id' error, got: %v", err)
	}
}

func TestLoadTestsMissingCommandRejected(t *testing.T) {
	path := writeTempFile(t, testsJSONFilename, `{
		"suites": [{ "id": "smoke", "name": "Smoke" }]
	}`)
	_, err := LoadTests(path)
	if err == nil || !strings.Contains(err.Error(), "missing command") {
		t.Errorf("want 'missing command' error, got: %v", err)
	}
}

func TestLoadTestsMissingSuiteNameRejected(t *testing.T) {
	path := writeTempFile(t, testsJSONFilename, `{
		"suites": [{ "id": "smoke", "command": "x" }]
	}`)
	_, err := LoadTests(path)
	if err == nil || !strings.Contains(err.Error(), "missing name") {
		t.Errorf("want 'missing name' error, got: %v", err)
	}
}

func TestLoadTestsMissingSetupCommandNameRejected(t *testing.T) {
	path := writeTempFile(t, testsJSONFilename, `{
		"setupCommands": [{ "command": "npm ci" }],
		"suites": [{ "id": "smoke", "name": "Smoke", "command": "x" }]
	}`)
	_, err := LoadTests(path)
	if err == nil || !strings.Contains(err.Error(), "setupCommands[0] missing name") {
		t.Errorf("want 'setupCommands[0] missing name' error, got: %v", err)
	}
}

func TestLoadTestsMissingSetupCommandCommandRejected(t *testing.T) {
	path := writeTempFile(t, testsJSONFilename, `{
		"setupCommands": [{ "name": "Install" }],
		"suites": [{ "id": "smoke", "name": "Smoke", "command": "x" }]
	}`)
	_, err := LoadTests(path)
	if err == nil || !strings.Contains(err.Error(), "missing command") {
		t.Errorf("want 'missing command' error, got: %v", err)
	}
}

// TestLoadTestsNonJSONFileRejected covers the case a user passes YAML content
// under a `.json` filename to --test-config — the JSON codec runs and the error
// mentions both accepted formats.
func TestLoadTestsNonJSONFileRejected(t *testing.T) {
	path := writeTempFile(t, testsJSONFilename, "suites:\n  - id: smoke\n")
	_, err := LoadTests(path)
	if err == nil || !strings.Contains(err.Error(), "expected JSON or YAML file format") {
		t.Errorf("want format error, got: %v", err)
	}
}

// --- YAML round-trip coverage ---------------------------------------------
//
// These tests mirror the JSON-fixture tests above with `.yaml` extensions and
// YAML syntax. The struct tags carry both `json:"..."` and `yaml:"..."` with
// identical camelCase keys, so the field-level error messages should be
// byte-identical between formats — only the parse error differs.

func TestLoadSystemHappyPathYAML(t *testing.T) {
	path := writeTempFile(t, systemYAMLFilename, `
systems:
  - label: real
    composeFile: monolith/docker-compose.local.real.yml
    containerName: my-shop-real
    components:
      - name: Monolith
        url: http://localhost:3311
        containerName: system
    externalSystems:
      - name: ERP
        url: http://localhost:9311/erp/health
        containerName: external-real
  - label: stub
    composeFile: monolith/docker-compose.local.stub.yml
    containerName: my-shop-stub
    components: []
    externalSystems: []
`)
	cfg, err := LoadSystem(path)
	if err != nil {
		t.Fatalf("LoadSystem: %v", err)
	}
	if len(cfg.Systems) != 2 {
		t.Errorf("want 2 systems, got %d", len(cfg.Systems))
	}
	if cfg.Systems[0].Label != "real" {
		t.Errorf("want first label 'real', got %q", cfg.Systems[0].Label)
	}
	if cfg.Systems[0].Components[0].URL != "http://localhost:3311" {
		t.Errorf("component URL not parsed: %+v", cfg.Systems[0].Components[0])
	}
}

func TestLoadSystemEmptySystemsRejectedYAML(t *testing.T) {
	path := writeTempFile(t, systemYAMLFilename, "systems: []\n")
	_, err := LoadSystem(path)
	if err == nil || !strings.Contains(err.Error(), "systems[] is empty") {
		t.Errorf("want 'systems[] is empty' error, got: %v", err)
	}
}

func TestLoadSystemMissingComposeFileRejectedYAML(t *testing.T) {
	path := writeTempFile(t, systemYAMLFilename, `
systems:
  - label: x
    components: []
    externalSystems: []
`)
	_, err := LoadSystem(path)
	if err == nil || !strings.Contains(err.Error(), "missing composeFile") {
		t.Errorf("want 'missing composeFile' error, got: %v", err)
	}
}

func TestLoadSystemMissingLabelRejectedYAML(t *testing.T) {
	path := writeTempFile(t, systemYAMLFilename, `
systems:
  - composeFile: x.yml
`)
	_, err := LoadSystem(path)
	if err == nil || !strings.Contains(err.Error(), "missing label") {
		t.Errorf("want 'missing label' error, got: %v", err)
	}
}

func TestLoadSystemMissingComponentNameRejectedYAML(t *testing.T) {
	path := writeTempFile(t, systemYAMLFilename, `
systems:
  - label: real
    composeFile: x.yml
    components:
      - url: http://localhost:1
`)
	_, err := LoadSystem(path)
	if err == nil || !strings.Contains(err.Error(), "components[0] missing name") {
		t.Errorf("want 'components[0] missing name' error, got: %v", err)
	}
}

func TestLoadSystemMissingExternalSystemURLRejectedYAML(t *testing.T) {
	path := writeTempFile(t, systemYAMLFilename, `
systems:
  - label: real
    composeFile: x.yml
    externalSystems:
      - name: ERP
`)
	_, err := LoadSystem(path)
	if err == nil || !strings.Contains(err.Error(), "missing url") {
		t.Errorf("want 'missing url' error, got: %v", err)
	}
}

func TestLoadSystemInvalidYAML(t *testing.T) {
	path := writeTempFile(t, systemYAMLFilename, "systems:\n  - label: real\n   composeFile: bad-indent\n")
	_, err := LoadSystem(path)
	if err == nil || !strings.Contains(err.Error(), "expected JSON or YAML file format") {
		t.Errorf("want format error, got: %v", err)
	}
}

// TestLoadSystemYAMLAcceptsJSONContent proves YAML 1.2 is a JSON superset —
// a `.yaml` file containing valid JSON syntax still parses through the YAML
// codec. Reassures shop operators that hand-renaming `.json` → `.yaml` works
// without rewriting content.
func TestLoadSystemYAMLAcceptsJSONContent(t *testing.T) {
	path := writeTempFile(t, systemYAMLFilename, `{
		"systems": [{
			"label": "real",
			"composeFile": "x.yml",
			"containerName": "c",
			"components": [],
			"externalSystems": []
		}]
	}`)
	cfg, err := LoadSystem(path)
	if err != nil {
		t.Fatalf("LoadSystem on JSON-in-.yaml: %v", err)
	}
	if cfg.Systems[0].Label != "real" {
		t.Errorf("want label 'real', got %q", cfg.Systems[0].Label)
	}
}

// TestLoadSystemYMLExtension covers the `.yml` alternative extension.
func TestLoadSystemYMLExtension(t *testing.T) {
	path := writeTempFile(t, "systems.yml", `
systems:
  - label: real
    composeFile: x.yml
    containerName: c
    components: []
    externalSystems: []
`)
	if _, err := LoadSystem(path); err != nil {
		t.Errorf("LoadSystem on .yml: %v", err)
	}
}

func TestLoadTestsHappyPathYAML(t *testing.T) {
	path := writeTempFile(t, testsYAMLFilename, `
setupCommands:
  - name: Install
    command: npm ci
testFilter: "--grep '<test>'"
suites:
  - id: smoke
    name: Smoke
    command: npx playwright test smoke
    env:
      MODE: stub
    path: .
    testReportPath: playwright-report/index.html
    sampleTest: shouldWork
  - id: e2e
    name: E2E
    command: npx playwright test e2e
`)
	cfg, err := LoadTests(path)
	if err != nil {
		t.Fatalf("LoadTests: %v", err)
	}
	if len(cfg.Suites) != 2 {
		t.Errorf("want 2 suites, got %d", len(cfg.Suites))
	}
	if cfg.Suites[0].Env["MODE"] != "stub" {
		t.Errorf("env not parsed: %+v", cfg.Suites[0].Env)
	}
	if cfg.TestFilter != "--grep '<test>'" {
		t.Errorf("testFilter not parsed: %q", cfg.TestFilter)
	}
}

func TestLoadTestsEmptySuitesRejectedYAML(t *testing.T) {
	path := writeTempFile(t, testsYAMLFilename, "suites: []\n")
	_, err := LoadTests(path)
	if err == nil || !strings.Contains(err.Error(), "suites[] is empty") {
		t.Errorf("want 'suites[] is empty' error, got: %v", err)
	}
}

func TestLoadTestsMissingIDRejectedYAML(t *testing.T) {
	path := writeTempFile(t, testsYAMLFilename, `
suites:
  - name: X
    command: x
`)
	_, err := LoadTests(path)
	if err == nil || !strings.Contains(err.Error(), "missing id") {
		t.Errorf("want 'missing id' error, got: %v", err)
	}
}

func TestLoadTestsMissingCommandRejectedYAML(t *testing.T) {
	path := writeTempFile(t, testsYAMLFilename, `
suites:
  - id: smoke
    name: Smoke
`)
	_, err := LoadTests(path)
	if err == nil || !strings.Contains(err.Error(), "missing command") {
		t.Errorf("want 'missing command' error, got: %v", err)
	}
}

func TestLoadTestsMissingSuiteNameRejectedYAML(t *testing.T) {
	path := writeTempFile(t, testsYAMLFilename, `
suites:
  - id: smoke
    command: x
`)
	_, err := LoadTests(path)
	if err == nil || !strings.Contains(err.Error(), "missing name") {
		t.Errorf("want 'missing name' error, got: %v", err)
	}
}

func TestLoadTestsMissingSetupCommandNameRejectedYAML(t *testing.T) {
	path := writeTempFile(t, testsYAMLFilename, `
setupCommands:
  - command: npm ci
suites:
  - id: smoke
    name: Smoke
    command: x
`)
	_, err := LoadTests(path)
	if err == nil || !strings.Contains(err.Error(), "setupCommands[0] missing name") {
		t.Errorf("want 'setupCommands[0] missing name' error, got: %v", err)
	}
}

func TestLoadTestsMissingSetupCommandCommandRejectedYAML(t *testing.T) {
	path := writeTempFile(t, testsYAMLFilename, `
setupCommands:
  - name: Install
suites:
  - id: smoke
    name: Smoke
    command: x
`)
	_, err := LoadTests(path)
	if err == nil || !strings.Contains(err.Error(), "missing command") {
		t.Errorf("want 'missing command' error, got: %v", err)
	}
}

func TestLoadTestsInvalidYAML(t *testing.T) {
	path := writeTempFile(t, testsYAMLFilename, "suites:\n  - id: smoke\n   name: bad-indent\n")
	_, err := LoadTests(path)
	if err == nil || !strings.Contains(err.Error(), "expected JSON or YAML file format") {
		t.Errorf("want format error, got: %v", err)
	}
}

// TestLoadTestsSuiteGroupsYAML verifies the optional `suiteGroups:` block
// round-trips out of YAML into TestsConfig.SuiteGroups. The block is what
// lets a project override the Go-side default `acceptance` alias and
// declare new groups of its own.
func TestLoadTestsSuiteGroupsYAML(t *testing.T) {
	path := writeTempFile(t, testsYAMLFilename, `
suiteGroups:
  acceptance: [acceptance-api, acceptance-ui]
  contract: [contract-stub, contract-real]
suites:
  - id: acceptance-api
    name: Acceptance API
    command: npx playwright test acceptance
  - id: acceptance-ui
    name: Acceptance UI
    command: npx playwright test acceptance
  - id: contract-stub
    name: Contract Stub
    command: npx playwright test contract
  - id: contract-real
    name: Contract Real
    command: npx playwright test contract
`)
	cfg, err := LoadTests(path)
	if err != nil {
		t.Fatalf("LoadTests: %v", err)
	}
	if len(cfg.SuiteGroups) != 2 {
		t.Fatalf("want 2 suite groups, got %d: %+v", len(cfg.SuiteGroups), cfg.SuiteGroups)
	}
	if got := cfg.SuiteGroups["acceptance"]; len(got) != 2 || got[0] != "acceptance-api" || got[1] != "acceptance-ui" {
		t.Errorf("suiteGroups[acceptance] = %v, want [acceptance-api acceptance-ui]", got)
	}
	if got := cfg.SuiteGroups["contract"]; len(got) != 2 || got[0] != "contract-stub" || got[1] != "contract-real" {
		t.Errorf("suiteGroups[contract] = %v, want [contract-stub contract-real]", got)
	}
}

func TestFindSuiteAndSuiteIDs(t *testing.T) {
	cfg := &TestsConfig{
		Suites: []Suite{
			{ID: "smoke", Name: "Smoke"},
			{ID: "e2e", Name: "E2E"},
		},
	}
	if got := cfg.FindSuite("e2e"); got == nil || got.Name != "E2E" {
		t.Errorf("FindSuite(e2e) = %+v, want E2E", got)
	}
	if got := cfg.FindSuite("missing"); got != nil {
		t.Errorf("FindSuite(missing) = %+v, want nil", got)
	}
	ids := cfg.SuiteIDs()
	if len(ids) != 2 || ids[0] != "smoke" || ids[1] != "e2e" {
		t.Errorf("SuiteIDs = %v, want [smoke e2e]", ids)
	}
}
