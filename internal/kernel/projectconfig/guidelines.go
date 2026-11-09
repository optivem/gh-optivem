package projectconfig

// GuidelinesPathKey is the `system-test.paths:` key and placeholder naming the
// folder of per-area review guideline files (architecture.md, naming.md,
// testing.md, ...) that the Code & Architecture Review reads and grows (plan
// 20261109-1259). The folder is addressed as one ${guidelines-path}; the
// reviewer reads only that folder, never CLAUDE.md.
const GuidelinesPathKey = "guidelines-path"

// DefaultGuidelinesPath is the repo-relative folder ${guidelines-path}
// resolves to when gh-optivem.yaml does not set system-test.paths.guidelines-path.
const DefaultGuidelinesPath = "docs/guidelines"

// GuidelinesPath returns the configured guidelines folder, or
// DefaultGuidelinesPath when the key is unset.
//
// Unlike the canonical Family B keys this one is optional (not in
// CanonicalPathKeys, so Rule 22a does not require it and preflight does not
// stat it): the folder starts empty and is created by the first guideline the
// review loop records, and adding a required key would break every existing
// config on the next release (see path-keys.md, "Release ordering").
func (c *Config) GuidelinesPath() string {
	if c != nil {
		if v := c.SystemTest.Paths[GuidelinesPathKey]; v != "" {
			return v
		}
	}
	return DefaultGuidelinesPath
}
