package testselect

// AcceptanceSuites returns the acceptance suite ids for the given channels —
// the single channel-aware source for the `acceptance` group alias. For each
// channel <ch> it emits BOTH the parallel non-isolated suite (acceptance-<ch>)
// and the serial isolated suite (acceptance-isolated-<ch>): the split is an
// execution concern (parallel vs maxParallelForks=1), not a semantic one, so
// `--suite=acceptance` runs all of them — the runner runs suites sequentially,
// so the serial isolated suites simply run after the parallel ones.
//
// When channels is empty it falls back to the canonical {api, ui} pair. That
// fallback exists for the one caller that cannot see the project's channel set:
// the `gh optivem test run` CLI loads only tests.yaml, not gh-optivem.yaml, so
// it passes nil and relies on the project's own `suiteGroups: acceptance:` (which
// takes precedence over this default) for a non-{api,ui} channel layout. Preflight,
// which CAN see cfg.Channels, passes them through — so it expands `acceptance`
// against the real channel set rather than the fallback. Both reach this one
// function (the CLI/runtime via ExpandSuiteGroups's default registry, preflight
// via cfg.Channels), so the two cannot drift.
func AcceptanceSuites(channels []string) []string {
	if len(channels) == 0 {
		channels = []string{"api", "ui"}
	}
	out := make([]string, 0, len(channels)*2)
	for _, ch := range channels {
		out = append(out, "acceptance-"+ch, "acceptance-isolated-"+ch)
	}
	return out
}

// defaultSuiteGroups is the fallback registry of group-alias names used when a
// project's tests.yaml does not declare its own `suiteGroups:` block. Built
// per-call because the only default group, "acceptance", is channel-derived;
// the registry is shaped this way so adding contract / e2e groups later is a
// one-line edit. Projects override the defaults by listing their own groups in
// tests.yaml — see runner.TestsConfig.SuiteGroups.
func defaultSuiteGroups(channels []string) map[string][]string {
	return map[string][]string{
		"acceptance": AcceptanceSuites(channels),
	}
}

// ExpandSuiteGroups maps known group-alias names to their constituent suite ids
// and passes any non-alias name through unchanged. The projectGroups map (the
// loaded tests.yaml's `suiteGroups:` block) takes precedence over the Go-side
// defaultSuiteGroups, so a project can both override a default group and declare
// new groups of its own. The channels slice feeds the channel-aware default
// `acceptance` group (see AcceptanceSuites); pass nil to use the {api,ui}
// fallback. Duplicates after expansion are de-duped while preserving first-seen
// order. Unknown names pass through so that the downstream "suite(s) not found"
// check in the runner still catches typos.
//
// Used by the `gh optivem test run` CLI to let `--suite=acceptance` resolve to
// the acceptance suites (parallel + isolated, per channel) — and by preflight's
// suite-existence sweep, which passes cfg.Channels so it validates exactly the
// ids the runtime's `--suite=acceptance` emission will request.
func ExpandSuiteGroups(names []string, projectGroups map[string][]string, channels []string) []string {
	if len(names) == 0 {
		return names
	}
	defaults := defaultSuiteGroups(channels)
	lookup := func(n string) ([]string, bool) {
		if group, ok := projectGroups[n]; ok {
			return group, true
		}
		if group, ok := defaults[n]; ok {
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
