package projectconfig

import (
	"fmt"
	"slices"
	"strings"
)

// Channel token enum values — the canonical lowercase slugs the scaffold
// writes into channels: and the runtime selectors are derived from. Each
// channel <token> owns a per-channel acceptance GROUP, `acceptance-<token>`
// (e.g. acceptance-api / acceptance-ui), which fans out to that channel's two
// concrete partition suites — acceptance-parallel-<token> and
// acceptance-isolated-<token> (see testselect.AcceptanceSuites). The bare
// `acceptance-<token>` is the group, never a single suite, so binding it always
// covers both partitions. Each token names a channel the testkit physically
// has: a driver (MyShopApiDriver / MyShopUiDriver), a ChannelType constant, and
// that acceptance partition pair.
// channels: therefore *selects a subset* of these — narrow to [api] for an
// API-only project — it does not declare arbitrary new channels, and its
// order does not change the implementation order (see ExecutionChannels).
// Adding a genuinely new channel (cli, grpc) means building its driver +
// testkit and extending this enum, not editing config.
const (
	ChannelAPI = "api"
	ChannelUI  = "ui"
)

// canonicalChannels is the supported channel set — the closed enum
// validateChannels checks membership against, and the order DefaultChannels
// emits. Extending the system with a new channel adds a member here
// (alongside its driver + testkit), the same way the lang enum grows.
var canonicalChannels = []string{ChannelAPI, ChannelUI}

// DefaultChannels is the scaffold-authoritative initial channel set written
// by `gh optivem init` — matching the api+ui testkit copySystemTests
// produces. Mirrors DefaultPaths: the only place the binary writes
// channels:, and it owns the just-created testkit tree, so the value is
// authoritative by construction. Operator-owned afterwards. Deliberately
// not wired as a validate-time or migrate-time fallback — see
// internal/projectconfig/path-keys.md for the scaffold-authoritative
// doctrine the channels: field follows.
func DefaultChannels() []string {
	out := make([]string, len(canonicalChannels))
	copy(out, canonicalChannels)
	return out
}

// ExecutionChannels returns the project's channels in implementation order:
// canonical order (API before UI), whatever order channels: lists them in.
// The API channel goes first so the first (common:true) channel builds the
// shared layer behind the API, and the UI channel then only adds its adapter
// delta on top of an already-working backend (plan 20261109-1257). The
// per-channel unrolls and the scoped --target resume both read this, so a
// full run and a scoped resume agree on which channel is first.
func (c *Config) ExecutionChannels() []string {
	out := make([]string, 0, len(c.Channels))
	for _, ch := range canonicalChannels {
		if slices.Contains(c.Channels, ch) {
			out = append(out, ch)
		}
	}
	return out
}

// isCanonicalChannel reports whether ch is one of the supported channel
// tokens (exact, case-sensitive — the single-canon contract).
func isCanonicalChannel(ch string) bool {
	return slices.Contains(canonicalChannels, ch)
}

// validateChannels enforces the closed-enum + single-canon rule for
// channels: tokens. A casing slip on a real channel (the ChannelType
// constant *name* is idiomatic uppercase, so reaching for `API` here is a
// natural mistake) gets a precise did-you-mean; any other unknown token is
// named against the supported set; duplicates are rejected. Empty/absent
// channels: is accepted — the field is optional, written
// scaffold-authoritatively at init.
//
// Mirrors Rule 22a's hard-error-no-backfill philosophy: rather than a
// case-insensitive fold repeated in every consumer (codegen, AT param,
// verify filter), where a missed fold would be a silent drift bug, a single
// rejection point keeps one canon.
func validateChannels(channels []string) error {
	seen := map[string]struct{}{}
	for i, ch := range channels {
		if ch == "" {
			return fmt.Errorf("config: channels[%d] must not be empty", i)
		}
		if !isCanonicalChannel(ch) {
			if lower := strings.ToLower(ch); isCanonicalChannel(lower) {
				return fmt.Errorf("config: channels: tokens must be lowercase; got %q, did you mean %q?", ch, lower)
			}
			return fmt.Errorf("config: channels: %q must be one of %q, %q", ch, ChannelAPI, ChannelUI)
		}
		if _, dup := seen[ch]; dup {
			return fmt.Errorf("config: channels: %q appears more than once", ch)
		}
		seen[ch] = struct{}{}
	}
	return nil
}
