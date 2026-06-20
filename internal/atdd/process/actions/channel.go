package actions

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/testselect"
	"github.com/optivem/gh-optivem/internal/build/runner"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// resolveChannel runs at the START of each unrolled per-channel clone (plan
// 20260619-1139 Steps 3 + 4) — the channel analog of resolveExternalSystem. The
// channel unrolls (UnrollSystemChannels / UnrollSystemDriverAdapterChannels)
// bake each clone's `channel` into ctx.Params at call-activity entry; this
// action reads it and stamps ctx.State["channel-touched"] = (any of the ticket's
// acceptance tests registered for this channel). The downstream
// GATE_CHANNEL_TOUCHED routes on the bool: an untouched channel no-ops past the
// whole clone to its skip end-event, so an API-only ticket does no UI implement
// and no UI verify (no wasted ~12 min implementer, no TESTS_INFRA_HALT like
// rehearsal #76).
//
// Membership source (decision #6): the RED acceptance verify — filtered to the
// ticket's new tests — runs in write-and-verify-acceptance-tests BEFORE these
// per-channel clones and writes one report per acceptance-<ch> partition. A
// failing test still appears as a <testcase> in that report, so membership is
// RED-tolerant: resolve-channel reads the report straight off disk
// (runner.NamesInReport — no run, no cache) and intersects it with the ticket's
// test names. An untouched channel's filtered RED run wrote nothing, so its
// report is absent/empty → not touched.
//
// Ordering invariant (the one soundness obligation): this gate reads the report
// the RED *filtered* acceptance verify wrote, which exists before this node; the
// only thing that overwrites a channel's acceptance-<ch> report is that
// channel's own — later — system implementer, which runs AFTER this gate inside
// the (touched) clone. So the report this gate reads is always the RED one,
// never a later green overwrite for the same channel.
//
// Non-channel callers (the structural redesign/refactor cycles reuse
// implement-and-verify-system without a channel unroll) bake no `channel` param
// and bind `test-names: ""`; an empty channel means "not a per-channel clone" →
// touched=true (run normally), so the guard is inert for them. Deterministic —
// no agent.
func (a actions) resolveChannel(ctx *statemachine.Context) statemachine.Outcome {
	channel := strings.TrimSpace(ctx.Params["channel"])
	if channel == "" {
		// Not a per-channel clone (structural cycle, or a project with no
		// channels: configured that kept the single static node) — the guard
		// is inert: run the cycle as before.
		ctx.Set("channel-touched", true)
		return statemachine.Outcome{}
	}
	ticketTests := splitTestNames(ctx.Params["test-names"])
	if len(ticketTests) == 0 {
		// No ticket tests to locate. On the behavioral cycle the RED writer
		// always emits names, so this is not expected; fail OPEN (run the
		// channel) rather than silently skip — the dangerous direction (a test
		// for an unconfigured channel vanishing) is owned by the Step-5
		// validate-channels-registered guard, which runs before any clone.
		ctx.Set("channel-touched", true)
		return statemachine.Outcome{}
	}
	if a.deps.TestsConfig == nil {
		return statemachine.Outcome{Err: fmt.Errorf("resolve-channel: tests.yaml not loaded — driver must inject actions.Deps.TestsConfig")}
	}

	names, err := a.channelReportNames(channel)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("resolve-channel: %w", err)}
	}
	touched := false
	for _, t := range ticketTests {
		if names[t] {
			touched = true
			break
		}
	}
	ctx.Set("channel-touched", touched)
	return statemachine.Outcome{}
}

// validateChannelsRegistered is the upfront no-silent-skip guard for the
// channel unroll (plan 20260619-1139 Step 5) — the channel analog of
// validateExternalSystemsRegistered. It runs ONCE in shared-contract, after the
// RED acceptance verify wrote its per-channel reports and before any unrolled
// clone, and hard-errors if a ticket's acceptance test ran in NONE of the
// configured channels: such a test registers for a channel absent from
// gh-optivem.yaml channels: (or does not exist) and would be silently skipped by
// every per-channel touched-gate, leaving the acceptance criterion unverified.
//
// This is the dangerous direction the per-channel touched-gate cannot catch: the
// gate's benign direction is "a configured channel with no matching test → skip";
// this guard's direction is "a test with no configured channel → error". Both
// read the same on-disk reports (runner.NamesInReport), but the gate asks
// per-channel and this asks across the whole configured set. Deterministic — no
// agent.
func (a actions) validateChannelsRegistered(ctx *statemachine.Context) statemachine.Outcome {
	ticketTests := splitTestNames(ctx.GetString("at-test-names"))
	if len(ticketTests) == 0 {
		// Nothing to validate (no acceptance tests emitted — e.g. a cascade that
		// did not write any). The membership question is moot.
		return statemachine.Outcome{}
	}
	if a.deps.TestsConfig == nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-channels-registered: tests.yaml not loaded — driver must inject actions.Deps.TestsConfig")}
	}
	var channels []string
	if a.deps.Config != nil {
		channels = a.deps.Config.Channels
	}
	suiteIDs := a.acceptanceSuiteIDs("acceptance", channels)
	names, err := runner.NamesInReport(a.deps.TestsConfig, a.deps.TestsCwd, suiteIDs)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-channels-registered: %w", err)}
	}
	var orphans []string
	for _, t := range ticketTests {
		if !names[t] {
			orphans = append(orphans, t)
		}
	}
	if len(orphans) > 0 {
		return statemachine.Outcome{Err: fmt.Errorf("validate-channels-registered: acceptance test(s) %s ran in none of the configured channels (%s) — they register for a channel not in gh-optivem.yaml channels:, so every per-channel gate would skip them and the criterion would go unverified; add the channel to channels: or fix the test's channel registration", strings.Join(orphans, ", "), strings.Join(channels, ", "))}
	}
	return statemachine.Outcome{}
}

// channelReportNames reads the union of method names in the on-disk
// acceptance-<channel> report partition(s). It resolves the per-channel group
// alias through ExpandSuiteGroups (honouring a project's own tests.yaml
// suiteGroups: override and falling back to the channel-aware default), then
// reads each partition's report via runner.NamesInReport without running
// anything.
func (a actions) channelReportNames(channel string) (map[string]bool, error) {
	var channels []string
	if a.deps.Config != nil {
		channels = a.deps.Config.Channels
	}
	suiteIDs := a.acceptanceSuiteIDs("acceptance-"+channel, channels)
	return runner.NamesInReport(a.deps.TestsConfig, a.deps.TestsCwd, suiteIDs)
}

// acceptanceSuiteIDs expands an `acceptance` / `acceptance-<ch>` group alias to
// its concrete partition suite ids, preferring the project's tests.yaml
// suiteGroups: block and falling back to the channel-aware default registry
// (testselect.ExpandSuiteGroups). Shared by the per-channel guard and the
// across-all-channels registration check so the two resolve identically.
func (a actions) acceptanceSuiteIDs(alias string, channels []string) []string {
	var projectGroups map[string][]string
	if a.deps.TestsConfig != nil {
		projectGroups = a.deps.TestsConfig.SuiteGroups
	}
	return testselect.ExpandSuiteGroups([]string{alias}, projectGroups, channels)
}

// methodIdentRE matches a bare test-method identifier (the shape acceptance /
// contract test method names take across Java, .NET, and TypeScript). A token
// that fails this is dropped rather than reported as a phantom orphan.
var methodIdentRE = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// splitTestNames parses the `test-names` list the writing agent emits (and the
// unroll carries onto each clone's params) into trimmed bare method names.
//
// The production input is the comma-joined form coerceValueToString produces
// for a `[]string` (e.g. "Foo,Bar,Baz"). This helper is also a belt-and-
// suspenders safety net for an upstream stringify slip: it strips a surrounding
// `[...]` (the bracketed `[Foo Bar Baz]` shape a fmt.Sprint on a slice produced
// in the rehearsal #72 false-halt), splits on commas OR whitespace, and drops
// any token that isn't a valid method identifier. So a bracketed or
// space-separated blob parses correctly instead of collapsing into one
// unmatchable token, and genuine noise is dropped rather than surfaced as a
// phantom orphan. The already-correct comma-joined input is unaffected.
func splitTestNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	var out []string
	for _, t := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	}) {
		if methodIdentRE.MatchString(t) {
			out = append(out, t)
		}
	}
	return out
}
