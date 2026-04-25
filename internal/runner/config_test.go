package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	systemJSONFilename = "system.json"
	testsJSONFilename  = "tests.json"
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
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Errorf("want parse error, got: %v", err)
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
