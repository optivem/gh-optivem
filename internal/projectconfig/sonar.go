package projectconfig

// DerivedSonar holds the SonarCloud project keys for each code tier of a
// scaffolded system. The fields populated depend on architecture:
//   - monolith:  System + SystemTest.
//   - multitier: Backend + Frontend + SystemTest.
// SystemTest is always populated when architecture is set, regardless of
// repo strategy or test language.
type DerivedSonar struct {
	System     string // monolith only
	Backend    string // multitier only
	Frontend   string // multitier only
	SystemTest string // always
}

// DeriveSonarProjects returns the canonical SonarCloud project keys for
// the given system identity. Keys mirror the suffix convention enforced
// by the scaffold-time replacements in internal/steps/apply_template.go:
//
//	monolith               -> <owner>_<repo>-system
//	multitier (backend)    -> <owner>_<repo>-backend
//	multitier (frontend)   -> <owner>_<repo>-frontend
//	system_test (always)   -> <owner>_<repo>-system-test
//
// Inputs are owner + base repo name + architecture + repo strategy. The
// strategy parameter does not affect the result today (multirepo keys
// resolve to the same value as monorepo: <base>-{backend,frontend,system}
// produces the same string whether derived from prefix+suffix or from the
// already-suffixed multirepo component name). It stays on the signature
// so the validation rule that re-derives keys from (owner, repo, arch,
// repo_strategy) can share one entry point.
func DeriveSonarProjects(owner, repo, arch, repoStrategy string) DerivedSonar {
	_ = repoStrategy
	prefix := owner + "_" + repo
	d := DerivedSonar{SystemTest: prefix + "-system-test"}
	switch arch {
	case ArchMonolith:
		d.System = prefix + "-system"
	case ArchMultitier:
		d.Backend = prefix + "-backend"
		d.Frontend = prefix + "-frontend"
	}
	return d
}
