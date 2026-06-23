// Package componenttest loads and runs component-level test suites declared in
// a per-component component-tests.yaml. These are the commit-stage, in-process
// suites — unit, narrow integration, component, consumer-contract (Pact) — that
// run without a deployed system: the inside-the-SUT counterpart to the
// docker-compose-backed system tests in package runner.
//
// One config lives per component, co-located with the code it tests (e.g.
// system/multitier/backend-java/component-tests.yaml). The CLI discovers them
// from the component paths in gh-optivem.yaml (monolith system.path; multitier
// system.backend/frontend.path) — there is no central registry. The schema
// mirrors runner.TestsConfig (setupCommands / testFilter / suiteGroups / suites)
// so the vocabulary is uniform across both test tiers, and adds the two fields
// component tests need that system tests don't: per-suite `pending` and
// `requiresDocker`.
package componenttest

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigFileName is the fixed, convention-based filename the runner looks for in
// each component's directory. Discovery is by convention rather than an explicit
// gh-optivem.yaml field because TierSpec.Config is validate-restricted to the
// system-test tier.
const ConfigFileName = "component-tests.yaml"

// Config is the parsed component-tests.yaml.
type Config struct {
	SetupCommands  []SetupCommand      `yaml:"setupCommands"`
	TestFilter     string              `yaml:"testFilter"`
	TestFilterJoin string              `yaml:"testFilterJoin,omitempty"`
	SuiteGroups    map[string][]string `yaml:"suiteGroups,omitempty"`
	Suites         []Suite             `yaml:"suites"`
}

// SetupCommand is one component-side preparation step run by `setup` — npm ci,
// gradle warm, dependency restore. Not a SUT image build.
type SetupCommand struct {
	Name    string            `yaml:"name"`
	Command string            `yaml:"command"`
	Env     map[string]string `yaml:"env,omitempty"`
}

// Suite is one runnable component-level suite. Env vars are set on the suite
// process, not interpolated into Command. Path is the suite cwd relative to the
// component directory (defaults to the component root).
type Suite struct {
	ID             string            `yaml:"id"`
	Name           string            `yaml:"name"`
	Command        string            `yaml:"command"`
	Env            map[string]string `yaml:"env,omitempty"`
	Path           string            `yaml:"path,omitempty"`
	SampleTest     string            `yaml:"sampleTest,omitempty"`
	TestReportPath string            `yaml:"testReportPath,omitempty"`

	// Pending marks a level that exists in the taxonomy but has no tests yet.
	// A pending suite is never executed: `--suite <pending>` prints a "not
	// implemented yet" notice and passes, and an `all` run skips it without
	// failing. It replaces the `if: false` "not yet implemented" stub steps —
	// the slot is held in the config, not in workflow YAML.
	Pending bool `yaml:"pending,omitempty"`

	// RequiresDocker marks a suite whose command needs a running Docker daemon
	// (e.g. Java Testcontainers-Postgres). The runner preflights `docker info`
	// and fails fast with a clear message when Docker is unavailable, instead of
	// surfacing a cryptic Testcontainers stack trace mid-run.
	RequiresDocker bool `yaml:"requiresDocker,omitempty"`
}

// Load reads and validates a component-tests.yaml from path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read component-tests config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("component-tests config %s: expected a valid YAML file, but content is not valid: %w", path, err)
	}
	for i, sc := range cfg.SetupCommands {
		if sc.Name == "" {
			return nil, fmt.Errorf("component-tests config %s: setupCommands[%d] missing name", path, i)
		}
		if sc.Command == "" {
			return nil, fmt.Errorf("component-tests config %s: setupCommands[%d] (%s) missing command", path, i, sc.Name)
		}
	}
	if len(cfg.Suites) == 0 {
		return nil, fmt.Errorf("component-tests config %s: suites[] is empty", path)
	}
	seen := make(map[string]bool, len(cfg.Suites))
	for i, s := range cfg.Suites {
		if s.ID == "" {
			return nil, fmt.Errorf("component-tests config %s: suites[%d] missing id", path, i)
		}
		if s.Name == "" {
			return nil, fmt.Errorf("component-tests config %s: suites[%d] (%s) missing name", path, i, s.ID)
		}
		if seen[s.ID] {
			return nil, fmt.Errorf("component-tests config %s: duplicate suite id %q", path, s.ID)
		}
		seen[s.ID] = true
		// A non-pending suite must carry the command that runs it. A pending
		// suite legitimately has no command — it is a held slot, never executed.
		if !s.Pending && s.Command == "" {
			return nil, fmt.Errorf("component-tests config %s: suites[%d] (%s) missing command (set `pending: true` if the level has no tests yet)", path, i, s.ID)
		}
		// Reject non-portable path separators in the fields the runner resolves
		// with filepath.Join: a Windows-authored `build\reports` silently fails
		// to resolve on a Linux runner. Command is not checked — it is a shell
		// invocation that legitimately carries `.\` (e.g. `.\gradlew.bat`).
		for _, pf := range []struct{ key, val string }{
			{"path", s.Path},
			{"testReportPath", s.TestReportPath},
		} {
			if strings.Contains(pf.val, "\\") {
				return nil, fmt.Errorf("component-tests config %s: suites[%d] (%s) %s %q uses a backslash separator — use forward slashes so the path resolves on every OS", path, i, s.ID, pf.key, pf.val)
			}
		}
	}
	return &cfg, nil
}

// FindSuite returns the suite with the given id, or nil if not found.
func (c *Config) FindSuite(id string) *Suite {
	for i := range c.Suites {
		if c.Suites[i].ID == id {
			return &c.Suites[i]
		}
	}
	return nil
}

// SuiteIDs returns all suite ids in declaration order — used in error messages
// when a requested suite doesn't exist.
func (c *Config) SuiteIDs() []string {
	ids := make([]string, len(c.Suites))
	for i, s := range c.Suites {
		ids[i] = s.ID
	}
	return ids
}

// expandGroups maps any name that matches a key in this config's SuiteGroups to
// its constituent suite ids, passing non-group names through unchanged. The pass
// is single-level (group constituents must be concrete suite ids, not nested
// group names) and de-dupes while preserving first-seen order. Unlike the
// system-test expander it consults only the config's own suiteGroups — there are
// no channel-derived defaults at the component tier.
func (c *Config) expandGroups(names []string) []string {
	out := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, n := range names {
		if grp, ok := c.SuiteGroups[n]; ok {
			for _, s := range grp {
				add(s)
			}
			continue
		}
		add(n)
	}
	return out
}

// selectSuites resolves the requested suite ids/groups against this config and
// returns the matching suites in declaration order. Empty requested means every
// declared suite (the full, gate-equivalent set). Unknown ids fail loud with the
// available set, so a typo can't silently run nothing.
func (c *Config) selectSuites(requested []string) ([]Suite, error) {
	if len(requested) == 0 {
		return c.Suites, nil
	}
	expanded := c.expandGroups(requested)
	want := make(map[string]bool, len(expanded))
	var missing []string
	for _, id := range expanded {
		if c.FindSuite(id) == nil {
			missing = append(missing, id)
		}
		want[id] = true
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("suite(s) not found: %s. Available: %s",
			strings.Join(missing, ", "), strings.Join(c.SuiteIDs(), ", "))
	}
	var picked []Suite
	for _, s := range c.Suites { // preserve declaration order
		if want[s.ID] {
			picked = append(picked, s)
		}
	}
	return picked, nil
}
