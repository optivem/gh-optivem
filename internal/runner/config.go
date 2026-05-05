// Package runner orchestrates docker-compose-backed system tests using two
// JSON config files: a system.json (compose + health probes) and a tests.json
// (setup commands + suites). The runner has no concept of architecture,
// language, or suite flavor — those identities live in shop's filenames and
// directory names. This package treats every input as opaque.
package runner

import (
	"encoding/json"
	"fmt"
	"os"
)

// SystemConfig describes one or more docker-compose stacks ("systems") to
// build, bring up, and probe for health. Loaded from system.json.
type SystemConfig struct {
	Systems []SystemEntry `json:"systems"`
}

// SystemEntry is one compose stack. Label is a free-form log string; the
// runner never interprets it (typical values in shop: "real", "stub").
type SystemEntry struct {
	Label           string      `json:"label"`
	ComposeFile     string      `json:"composeFile"`
	ContainerName   string      `json:"containerName"`
	Components      []Component `json:"components"`
	ExternalSystems []Component `json:"externalSystems"`
}

// Component is one service within a system (a SUT component or external sim).
// URL is optional — components without one are skipped during health probes.
type Component struct {
	Name          string `json:"name"`
	URL           string `json:"url"`
	ContainerName string `json:"containerName"`
}

// TestsConfig describes test-runner setup + suites. Loaded from tests.json.
//
// TestFilter is a template containing the literal "<test>" — the runner
// substitutes the user-supplied --test value(s) (or a suite's sampleTest
// field when --sample is set). Two forms are supported:
//
//	"--grep '<test>'"       — full flag; appended as a new argument
//	"&Category=<test>"      — expression fragment beginning with "&"; injected
//	                           into an existing --filter '...' argument
//
// TestFilterJoin controls how multiple --test values are combined:
//
//	"" / "or" (default) — join names with "|" and substitute once. Covers
//	                       dotnet (`&DisplayName~T1|T2`) and playwright/jest
//	                       (`--grep 'T1|T2'`) where the runner already treats
//	                       "|" as alternation.
//	"repeat"            — substitute the whole TestFilter once per name and
//	                       concatenate. Covers gradle (`--tests T1 --tests T2`)
//	                       where the *flag itself* must repeat.
type TestsConfig struct {
	SetupCommands  []SetupCommand `json:"setupCommands"`
	TestFilter     string         `json:"testFilter"`
	TestFilterJoin string         `json:"testFilterJoin,omitempty"`
	Suites         []Suite        `json:"suites"`
}

// SetupCommand is one test-runner-side setup step — npm ci, dotnet build,
// gradle compileTestJava. NOT a SUT image build (use `build system` for that).
type SetupCommand struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Env     map[string]string `json:"env,omitempty"`
}

// Suite is one runnable test suite. Env vars are set on the test process,
// not interpolated into Command. TestInstallCommands run once before the
// suite if non-empty (e.g. installing playwright browsers per-suite).
type Suite struct {
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	Command             string            `json:"command"`
	Env                 map[string]string `json:"env,omitempty"`
	Path                string            `json:"path"`
	TestReportPath      string            `json:"testReportPath"`
	SampleTest          string            `json:"sampleTest,omitempty"`
	TestInstallCommands []string          `json:"testInstallCommands,omitempty"`
}

// LoadSystem reads and validates system.json from path.
func LoadSystem(path string) (*SystemConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read system config %s: %w", path, err)
	}
	var cfg SystemConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("system config %s: expected JSON file format, but content is not valid JSON: %w", path, err)
	}
	if len(cfg.Systems) == 0 {
		return nil, fmt.Errorf("system config %s: systems[] is empty", path)
	}
	for i, s := range cfg.Systems {
		if s.Label == "" {
			return nil, fmt.Errorf("system config %s: systems[%d] missing label", path, i)
		}
		if s.ComposeFile == "" {
			return nil, fmt.Errorf("system config %s: systems[%d] (%s) missing composeFile", path, i, s.Label)
		}
		for j, c := range s.Components {
			if c.Name == "" {
				return nil, fmt.Errorf("system config %s: systems[%d] (%s) components[%d] missing name", path, i, s.Label, j)
			}
		}
		for j, e := range s.ExternalSystems {
			if e.Name == "" {
				return nil, fmt.Errorf("system config %s: systems[%d] (%s) externalSystems[%d] missing name", path, i, s.Label, j)
			}
			if e.URL == "" {
				return nil, fmt.Errorf("system config %s: systems[%d] (%s) externalSystems[%d] (%s) missing url", path, i, s.Label, j, e.Name)
			}
		}
	}
	return &cfg, nil
}

// LoadTests reads and validates tests.json from path.
func LoadTests(path string) (*TestsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tests config %s: %w", path, err)
	}
	var cfg TestsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("tests config %s: expected JSON file format, but content is not valid JSON: %w", path, err)
	}
	for i, sc := range cfg.SetupCommands {
		if sc.Name == "" {
			return nil, fmt.Errorf("tests config %s: setupCommands[%d] missing name", path, i)
		}
		if sc.Command == "" {
			return nil, fmt.Errorf("tests config %s: setupCommands[%d] (%s) missing command", path, i, sc.Name)
		}
	}
	if len(cfg.Suites) == 0 {
		return nil, fmt.Errorf("tests config %s: suites[] is empty", path)
	}
	for i, s := range cfg.Suites {
		if s.ID == "" {
			return nil, fmt.Errorf("tests config %s: suites[%d] missing id", path, i)
		}
		if s.Name == "" {
			return nil, fmt.Errorf("tests config %s: suites[%d] (%s) missing name", path, i, s.ID)
		}
		if s.Command == "" {
			return nil, fmt.Errorf("tests config %s: suites[%d] (%s) missing command", path, i, s.ID)
		}
	}
	return &cfg, nil
}

// FindSuite returns the suite with the given id, or nil if not found.
func (t *TestsConfig) FindSuite(id string) *Suite {
	for i := range t.Suites {
		if t.Suites[i].ID == id {
			return &t.Suites[i]
		}
	}
	return nil
}

// SuiteIDs returns all suite ids in declaration order. Used in error messages
// when the user asks for a suite that doesn't exist.
func (t *TestsConfig) SuiteIDs() []string {
	ids := make([]string, len(t.Suites))
	for i, s := range t.Suites {
		ids[i] = s.ID
	}
	return ids
}
