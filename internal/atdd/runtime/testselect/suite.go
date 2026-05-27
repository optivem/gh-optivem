package testselect

// AcceptanceSuites returns the canonical list of acceptance suites — the
// dispatch fallback used by run_targeted_tests when its call-activity is
// invoked without a specific `suite:` param (as is the case for the
// collapsed AT_GREEN node, which writes once and verifies both channels).
// A future channel-execution plan may introduce sentinel suites like
// `<acceptance>` that union these explicitly; today, the absent-key case
// resolves to this fixed list.
func AcceptanceSuites() []string {
	return []string{"acceptance-api", "acceptance-ui"}
}

// defaultSuiteGroups is the fallback registry of group-alias names used
// when a project's tests.yaml does not declare its own `suiteGroups:`
// block. Today the only default group is "acceptance"; the registry is
// shaped this way so adding contract / e2e groups later is a one-line
// edit. Projects override the defaults by listing their own groups in
// tests.yaml — see runner.TestsConfig.SuiteGroups.
var defaultSuiteGroups = map[string][]string{
	"acceptance": AcceptanceSuites(),
}

// ExpandSuiteGroups maps known group-alias names to their constituent
// suite ids and passes any non-alias name through unchanged. The
// projectGroups map (the loaded tests.yaml's `suiteGroups:` block) takes
// precedence over the Go-side defaultSuiteGroups, so a project can both
// override a default group and declare new groups of its own. Duplicates
// after expansion are de-duped while preserving first-seen order. Unknown
// names pass through so that the downstream "suite(s) not found" check
// in the runner still catches typos.
//
// Used by the `gh optivem test run` CLI to let `--suite=acceptance`
// resolve to `acceptance-api,acceptance-ui` — and by the BPMN
// runtime, which emits `--suite=acceptance` from the
// verify-tests-pass/fail call-activities in
// `write-and-verify-acceptance-test-code`.
func ExpandSuiteGroups(names []string, projectGroups map[string][]string) []string {
	if len(names) == 0 {
		return names
	}
	lookup := func(n string) ([]string, bool) {
		if group, ok := projectGroups[n]; ok {
			return group, true
		}
		if group, ok := defaultSuiteGroups[n]; ok {
			return group, true
		}
		return nil, false
	}
	out := make([]string, 0, len(names))
	seen := map[string]bool{}
	add := func(s string) {
		if seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, n := range names {
		if group, ok := lookup(n); ok {
			for _, s := range group {
				add(s)
			}
			continue
		}
		add(n)
	}
	return out
}
