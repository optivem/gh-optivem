// Package runner orchestrates docker-compose-backed system tests using two
// YAML config files (with legacy JSON fallback): a systems.yaml (compose +
// health probes) and a tests.yaml (setup commands + suites). The
// unmarshaller is picked from the file extension — `.yaml` / `.yml` use
// YAML, anything else uses JSON. Struct keys are identical in both formats
// (camelCase composeFile etc.), so one struct round-trips either source.
// The runner has no concept of architecture, language, or suite flavor —
// those identities live in shop's filenames and directory names. This
// package treats every input as opaque.
package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// unmarshalConfig picks the codec by file extension. `.yaml` / `.yml` use
// the YAML codec; anything else (including `.json`) uses JSON. YAML 1.2 is a
// strict JSON superset, so a `.yaml` file written in JSON syntax still parses.
func unmarshalConfig(path string, data []byte, out any) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return yaml.Unmarshal(data, out)
	default:
		return json.Unmarshal(data, out)
	}
}

// SystemConfig describes one or more docker-compose stacks ("systems") to
// build, bring up, and probe for health. Loaded from systems.{yaml,json}.
type SystemConfig struct {
	Systems []SystemEntry `json:"systems" yaml:"systems"`
}

// SystemEntry is one compose stack. Label is a free-form log string; the
// runner never interprets it (typical values in shop: "real", "stub").
type SystemEntry struct {
	Label           string      `json:"label" yaml:"label"`
	ComposeFile     string      `json:"composeFile" yaml:"composeFile"`
	ContainerName   string      `json:"containerName" yaml:"containerName"`
	Components      []Component `json:"components" yaml:"components"`
	ExternalSystems []Component `json:"externalSystems" yaml:"externalSystems"`
}

// Component is one service within a system (a SUT component or external sim).
// URL is optional — components without one are skipped during health probes.
type Component struct {
	Name          string `json:"name" yaml:"name"`
	URL           string `json:"url" yaml:"url"`
	ContainerName string `json:"containerName" yaml:"containerName"`
}

// TestsConfig describes test-runner setup + suites. Loaded from tests.{yaml,json}.
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
	SetupCommands  []SetupCommand `json:"setupCommands" yaml:"setupCommands"`
	TestFilter     string         `json:"testFilter" yaml:"testFilter"`
	TestFilterJoin string         `json:"testFilterJoin,omitempty" yaml:"testFilterJoin,omitempty"`
	Suites         []Suite        `json:"suites" yaml:"suites"`
}

// SetupCommand is one test-runner-side setup step — npm ci, dotnet build,
// gradle compileTestJava. NOT a SUT image build (use `build system` for that).
type SetupCommand struct {
	Name    string            `json:"name" yaml:"name"`
	Command string            `json:"command" yaml:"command"`
	Env     map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
}

// Suite is one runnable test suite. Env vars are set on the test process,
// not interpolated into Command. TestInstallCommands run once before the
// suite if non-empty (e.g. installing playwright browsers per-suite).
type Suite struct {
	ID                  string            `json:"id" yaml:"id"`
	Name                string            `json:"name" yaml:"name"`
	Command             string            `json:"command" yaml:"command"`
	Env                 map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Path                string            `json:"path" yaml:"path"`
	TestReportPath      string            `json:"testReportPath" yaml:"testReportPath"`
	SampleTest          string            `json:"sampleTest,omitempty" yaml:"sampleTest,omitempty"`
	TestInstallCommands []string          `json:"testInstallCommands,omitempty" yaml:"testInstallCommands,omitempty"`
}

// LoadSystem reads and validates systems.{yaml,json} from path. The format is
// chosen by extension (`.yaml` / `.yml` → YAML, anything else → JSON).
func LoadSystem(path string) (*SystemConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read system config %s: %w", path, err)
	}
	var cfg SystemConfig
	if err := unmarshalConfig(path, data, &cfg); err != nil {
		return nil, fmt.Errorf("system config %s: expected JSON or YAML file format, but content is not valid: %w", path, err)
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

// LoadTests reads and validates tests.{yaml,json} from path. The format is
// chosen by extension (`.yaml` / `.yml` → YAML, anything else → JSON).
func LoadTests(path string) (*TestsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tests config %s: %w", path, err)
	}
	var cfg TestsConfig
	if err := unmarshalConfig(path, data, &cfg); err != nil {
		return nil, fmt.Errorf("tests config %s: expected JSON or YAML file format, but content is not valid: %w", path, err)
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
