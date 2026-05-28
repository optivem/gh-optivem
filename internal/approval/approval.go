// Package approval resolves the operator's global auto-approve policy
// (--auto + --confirm=<tier>) and gates every y/n confirmation against
// it. It sits one layer above promptio: promptio is the dumb y/n reader,
// approval decides whether to ask at all.
//
// The contract is small: Resolve once at command start (flag/env/default
// precedence baked in), stash the Resolved struct on the command context,
// and call Confirm / ConfirmVia at every confirmation site with a category
// tag. A site short-circuits to (true, nil) iff Auto is on and the site's
// category is BELOW the configured threshold floor. CategoryHuman is the
// top tier so human-STOPs never short-circuit at any reachable floor.
//
// promptio is unchanged — approval depends on promptio, not the reverse.
package approval

import (
	"fmt"
	"io"
	"strings"

	"github.com/optivem/gh-optivem/internal/promptio"
)

// Category names a tier in the approval-policy ladder. Tiers are
// ordered low-to-high by stakes:
//
//	command     — execute-command BPMN nodes (compile / build /
//	              start / test run). Cheap, no AI cost, no global
//	              state mutation.
//	prod-agent  — execute-agent for production code (implement-*,
//	              update-*, refactor-system). AI cost; produces
//	              reviewable diffs.
//	test-agent  — execute-agent for tests (write-*-tests,
//	              disable-tests, enable-tests, refactor-tests).
//	              Tests-as-contract: ranked above prod-agent
//	              because broken tests mask regressions.
//	prod-commit — commit BPMN node after a prod-agent phase.
//	              Persistent git write.
//	test-commit — commit BPMN node after a test-agent phase.
//	              Persistent git write of the test contract.
//	human       — always-prompts, operator-uncontrollable. Covers
//	              fix-* agents (signals of upstream defect),
//	              refine-acceptance-criteria (always-engage
//	              contract step), BPMN STOP nodes, release.
//
// `--confirm=<tier>` uses threshold semantics: this tier becomes
// the floor. Tiers at or above the floor still prompt; tiers
// below auto-yes under `--auto`. Default `--auto` floor is
// `human` (truly autonomous). Iota order is load-bearing — it
// encodes the threshold ranking.
type Category int

const (
	CategoryCommand    Category = iota // tier 1 — cheap commands
	CategoryProdAgent                  // tier 2 — production agents
	CategoryTestAgent                  // tier 3 — test agents
	CategoryProdCommit                 // tier 4 — production-code commits
	CategoryTestCommit                 // tier 5 — test-code commits
	CategoryHuman                      // tier 6 — always-prompt, operator-uncontrollable
)

// String returns the lowercase token used in --confirm=<tier> and env vars.
func (c Category) String() string {
	switch c {
	case CategoryCommand:
		return "command"
	case CategoryProdAgent:
		return "prod-agent"
	case CategoryTestAgent:
		return "test-agent"
	case CategoryProdCommit:
		return "prod-commit"
	case CategoryTestCommit:
		return "test-commit"
	case CategoryHuman:
		return "human"
	default:
		return fmt.Sprintf("category(%d)", int(c))
	}
}

// allCategories is the parse-acceptable set, in iota (threshold) order.
var allCategories = []Category{
	CategoryCommand,
	CategoryProdAgent,
	CategoryTestAgent,
	CategoryProdCommit,
	CategoryTestCommit,
	CategoryHuman,
}

// ParseCategory parses a category token (case-insensitive). Returns an error
// whose message lists the valid set; main.go's flag plumbing surfaces this
// to the operator verbatim.
func ParseCategory(s string) (Category, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "command":
		return CategoryCommand, nil
	case "prod-agent":
		return CategoryProdAgent, nil
	case "test-agent":
		return CategoryTestAgent, nil
	case "prod-commit":
		return CategoryProdCommit, nil
	case "test-commit":
		return CategoryTestCommit, nil
	case "human":
		return CategoryHuman, nil
	default:
		return 0, fmt.Errorf("unknown category %q; valid: %s", s, validList())
	}
}

func validList() string {
	parts := make([]string, len(allCategories))
	for i, c := range allCategories {
		parts[i] = c.String()
	}
	return strings.Join(parts, ", ")
}

// Resolved is the post-precedence policy snapshot. main.go's
// PersistentPreRunE builds one and stashes it on the command context; every
// confirmation call site reads it back.
//
// ConfirmFloor is the threshold tier: under Auto, calls with category
// strictly below ConfirmFloor short-circuit to (true, nil); calls at or
// above the floor prompt. CategoryHuman is the top tier, so any reachable
// floor (≤ CategoryHuman) always prompts human-tier sites — the load-bearing
// invariant for BPMN human-STOP nodes.
type Resolved struct {
	Auto          bool
	ConfirmFloor  Category
	AutoSource    string // "flag" | "env" | "default"
	ConfirmSource string // "flag" | "env" | "default"
}

// Env var names. Kept here, not in main.go, so the resolution logic and the
// docs stay in one place.
const (
	EnvAuto    = "GH_OPTIVEM_AUTO"
	EnvConfirm = "GH_OPTIVEM_CONFIRM"
)

// defaultFloorWhenAuto is the floor applied when --auto is on and no
// explicit --confirm is given. CategoryHuman means "truly autonomous: only
// human-tier sites still prompt."
const defaultFloorWhenAuto = CategoryHuman

// Resolve applies flag/env/default precedence to produce a Resolved policy.
//
// The *Changed bools are how Cobra distinguishes "flag explicitly set" from
// "flag has its default value." Without them --confirm= (explicit empty) and
// "no --confirm given" both look like "" and the default cannot fire
// correctly.
//
// env is injected for testability — pass os.Getenv in production, a stub
// in tests. An empty env value is treated as unset (matches shell
// ergonomics).
func Resolve(auto bool, autoChanged bool, confirm string, confirmChanged bool, env func(string) string) (Resolved, error) {
	r := Resolved{}

	// Auto: flag > env > default(false).
	switch {
	case autoChanged:
		r.Auto = auto
		r.AutoSource = "flag"
	default:
		if envVal := env(EnvAuto); envVal != "" {
			if v, ok := promptio.ParseYN(envVal); ok && v {
				r.Auto = true
				r.AutoSource = "env"
			}
		}
		if r.AutoSource == "" {
			r.AutoSource = "default"
		}
	}

	// Confirm: flag > env > default.
	// Default-when-Auto floor is `human` (truly autonomous). When Auto is
	// off the floor is unused at confirmation time (every call prompts), so
	// the zero value is fine.
	var (
		confirmRaw    string
		confirmSource string
	)
	switch {
	case confirmChanged:
		confirmRaw = confirm
		confirmSource = "flag"
	default:
		if envVal := env(EnvConfirm); envVal != "" {
			confirmRaw = envVal
			confirmSource = "env"
		} else {
			confirmSource = "default"
			if r.Auto {
				confirmRaw = defaultFloorWhenAuto.String()
			}
		}
	}
	r.ConfirmSource = confirmSource

	if confirmRaw != "" {
		if strings.Contains(confirmRaw, ",") {
			return Resolved{}, fmt.Errorf("--confirm takes a single tier (threshold), not a comma list: %q; valid: %s", confirmRaw, validList())
		}
		c, err := ParseCategory(confirmRaw)
		if err != nil {
			return Resolved{}, err
		}
		r.ConfirmFloor = c
	}

	return r, nil
}

// ConfirmFloorString returns the token name of r's confirm floor. Used by
// the startup banner and by clauderun's child-env propagation so the child
// sees the same operator-controlled floor the parent resolved.
func (r Resolved) ConfirmFloorString() string {
	return r.ConfirmFloor.String()
}

// Confirm asks for y/n confirmation unless the policy permits auto-yes for
// this category. Short-circuits to (true, nil) iff r.Auto && c < r.ConfirmFloor.
// CategoryHuman is the top tier, so any reachable floor (≤ CategoryHuman)
// always prompts human-tier sites.
func Confirm(r Resolved, c Category, in io.Reader, out io.Writer, prompt string) (bool, error) {
	if r.Auto && c < r.ConfirmFloor {
		return true, nil
	}
	return promptio.ConfirmYN(in, out, prompt)
}

// ConfirmVia is the Asker-routed variant for BPMN bindings and other call
// sites that hand prompts to a Prompter abstraction instead of raw stdio.
// Same short-circuit semantics as Confirm.
func ConfirmVia(r Resolved, c Category, asker promptio.Asker, out io.Writer, prompt string) (bool, error) {
	if r.Auto && c < r.ConfirmFloor {
		return true, nil
	}
	return promptio.ConfirmYNVia(asker, out, prompt)
}
