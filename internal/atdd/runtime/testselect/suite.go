package testselect

// AcceptanceSuites returns the concrete acceptance suite ids for the given
// channels — the single channel-aware source for the `acceptance` group alias.
// For each channel <ch> it emits BOTH partitions, named symmetrically: the
// parallel non-isolated suite (acceptance-parallel-<ch>) and the serial
// isolated suite (acceptance-isolated-<ch>). The split is an execution concern
// (parallel vs maxParallelForks=1), not a semantic one, so `--suite=acceptance`
// runs all of them — the runner runs suites sequentially, so the serial
// isolated suites simply run after the parallel ones.
//
// Symmetric naming is deliberate: neither partition id may pretend to be the
// whole channel. The bare `acceptance-<ch>` is NOT a concrete suite — it is a
// per-channel group alias (see defaultSuiteGroups) that fans out to exactly
// these two partitions, so a single binding `acceptance-<ch>` is always
// complete by construction.
//
// When channels is empty it falls back to the canonical {api, ui} pair. That
// fallback exists for the one caller that cannot see the project's channel set:
// the `gh optivem system-test run` CLI loads only tests.yaml, not gh-optivem.yaml, so
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
		out = append(out, channelAcceptanceSuites(ch)...)
	}
	return out
}

// channelAcceptanceSuites returns the two concrete partition suites that the
// per-channel `acceptance-<ch>` group alias fans out to. The single source for
// both AcceptanceSuites (which composes the top-level `acceptance` group from
// every channel) and defaultSuiteGroups (which registers the per-channel
// alias), so the partition ids are named in exactly one place.
func channelAcceptanceSuites(ch string) []string {
	return []string{"acceptance-parallel-" + ch, "acceptance-isolated-" + ch}
}

// defaultSuiteGroups is the fallback registry of group-alias names used when a
// project's tests.yaml does not declare its own `suiteGroups:` block. Built
// per-call because every default group is channel-derived. It registers:
//
//   - "acceptance": all channels' both-partition suites (the top-level group);
//   - "acceptance-<ch>" per channel: that channel's two partitions only.
//
// The per-channel aliases are what make the bare `acceptance-<ch>` resolve to
// both partitions by construction — the per-channel verify unrolls
// (statemachine/channels.go) bind the literal `acceptance-<ch>`, and the CLI
// expands it here, so the isolated partition can never be silently dropped from
// a per-channel verify. Constituents are always concrete partition suites (not
// nested group names), since ExpandSuiteGroups does a single, non-recursive
// pass.
//
// Projects override the defaults by listing their own groups in tests.yaml —
// see runner.TestsConfig.SuiteGroups.
func defaultSuiteGroups(channels []string) map[string][]string {
	if len(channels) == 0 {
		channels = []string{"api", "ui"}
	}
	groups := map[string][]string{
		"acceptance": AcceptanceSuites(channels),
	}
	for _, ch := range channels {
		groups["acceptance-"+ch] = channelAcceptanceSuites(ch)
	}
	return groups
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
// Used by the `gh optivem system-test run` CLI to let `--suite=acceptance` resolve to
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
