package projectconfig

// UseCasesPathKey is the `system-test.paths:` key and placeholder naming the
// use cases folder (the use case narratives, business rules and UC-NNN.M
// acceptance criteria). The characterize flow's characterization test fixer
// writes it: a failing characterization test may mean the criterion or the
// narrative is wrong, not the test (plan 20261110-1130).
const UseCasesPathKey = "use-cases-path"

// DefaultUseCasesPath is the repo-relative folder ${use-cases-path} resolves
// to when gh-optivem.yaml does not set system-test.paths.use-cases-path. It
// equals spec.DefaultSpecDir, the default `specDir:` of spec.yaml; a project
// that moves its use cases sets both.
const DefaultUseCasesPath = "docs/use-cases"

// UseCasesPath returns the configured use cases folder, or
// DefaultUseCasesPath when the key is unset.
//
// Optional like GuidelinesPath (not in CanonicalPathKeys, so Rule 22a does not
// require it and preflight does not stat it): adding a required key would
// break every existing config on the next release (see path-keys.md,
// "Release ordering").
func (c *Config) UseCasesPath() string {
	if c != nil {
		if v := c.SystemTest.Paths[UseCasesPathKey]; v != "" {
			return v
		}
	}
	return DefaultUseCasesPath
}
