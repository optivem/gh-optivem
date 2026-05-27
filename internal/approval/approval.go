// Package approval resolves the operator's global auto-approve policy
// (--auto + --confirm=<categories>) and gates every y/n confirmation against
// it. It sits one layer above promptio: promptio is the dumb y/n reader,
// approval decides whether to ask at all.
//
// The contract is small: Resolve once at command start (flag/env/default
// precedence baked in), stash the Resolved struct on the command context,
// and call Confirm / ConfirmVia at every confirmation site with a category
// tag. A site short-circuits to (true, nil) iff Auto is on and the site's
// category is NOT in the confirm-set. CategoryHuman is always in the
// confirm-set, so BPMN human-STOP nodes never auto-yes.
//
// promptio is unchanged — approval depends on promptio, not the reverse.
package approval

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/optivem/gh-optivem/internal/promptio"
)

// Category names a class of confirmation prompt. The closed set is pinned
// here so the --confirm=<list> vocabulary is a public, composable contract.
type Category int

const (
	CategoryCommit Category = iota
	CategoryFix
	CategoryRelease
	CategoryPrompt
	CategoryHuman
)

// String returns the lowercase token used in --confirm=<list> and env vars.
func (c Category) String() string {
	switch c {
	case CategoryCommit:
		return "commit"
	case CategoryFix:
		return "fix"
	case CategoryRelease:
		return "release"
	case CategoryPrompt:
		return "prompt"
	case CategoryHuman:
		return "human"
	default:
		return fmt.Sprintf("category(%d)", int(c))
	}
}

// allCategories is the parse-acceptable set. CategoryHuman is included so
// --confirm=human is a no-op rather than a parse error (operators may type
// it for symmetry with the docs even though it is always implicit).
var allCategories = []Category{
	CategoryCommit,
	CategoryFix,
	CategoryRelease,
	CategoryPrompt,
	CategoryHuman,
}

// ParseCategory parses a category token (case-insensitive). Returns an error
// whose message lists the valid set; main.go's flag plumbing surfaces this
// to the operator verbatim.
func ParseCategory(s string) (Category, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "commit":
		return CategoryCommit, nil
	case "fix":
		return CategoryFix, nil
	case "release":
		return CategoryRelease, nil
	case "prompt":
		return CategoryPrompt, nil
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
// CategoryHuman is always in ConfirmSet regardless of input — the invariant
// the Confirm helper relies on so human-STOP nodes never auto-yes.
type Resolved struct {
	Auto          bool
	ConfirmSet    map[Category]bool
	AutoSource    string // "flag" | "env" | "default"
	ConfirmSource string // "flag" | "env" | "default"
}

// Env var names. Kept here, not in main.go, so the resolution logic and the
// docs stay in one place.
const (
	EnvAuto    = "GH_OPTIVEM_AUTO"
	EnvConfirm = "GH_OPTIVEM_CONFIRM"
)

// defaultConfirmWhenAuto is the configurable default. CategoryHuman is added
// to the resolved set unconditionally, separately from this list.
var defaultConfirmWhenAuto = []Category{CategoryCommit, CategoryFix}

// Resolve applies flag/env/default precedence to produce a Resolved policy.
//
// The *Changed bools are how Cobra distinguishes "flag explicitly set" from
// "flag has its default value." Without them --confirm= (explicit empty,
// meaning "no exclusions") and "no --confirm given" (meaning "fall back to
// the default exclusion list") both look like "" and the default cannot
// fire correctly.
//
// env is injected for testability — pass os.Getenv in production, a stub
// in tests. An empty env value is treated as unset (matches shell
// ergonomics).
func Resolve(auto bool, autoChanged bool, confirm string, confirmChanged bool, env func(string) string) (Resolved, error) {
	r := Resolved{ConfirmSet: map[Category]bool{}}

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
	// Default-when-Auto is commit,fix. Default-when-not-Auto is empty (the
	// set is unused at confirmation time, but we still populate the implicit
	// human entry below to preserve the invariant).
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
				confirmRaw = joinCategories(defaultConfirmWhenAuto)
			}
		}
	}
	r.ConfirmSource = confirmSource

	for _, tok := range splitConfirm(confirmRaw) {
		c, err := ParseCategory(tok)
		if err != nil {
			return Resolved{}, err
		}
		r.ConfirmSet[c] = true
	}

	// CategoryHuman is always set — implicit, operator-uncontrollable.
	r.ConfirmSet[CategoryHuman] = true

	return r, nil
}

func splitConfirm(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func joinCategories(cs []Category) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = c.String()
	}
	return strings.Join(parts, ",")
}

// ConfirmListString returns the comma-joined list of categories in r's
// confirm set, minus the implicit CategoryHuman, in canonical order. Used
// by the startup banner and by clauderun's child-env propagation so the
// child sees the same operator-controlled exclusion list the parent
// resolved (the implicit human is re-added by the child's own Resolve).
func (r Resolved) ConfirmListString() string {
	keys := make([]Category, 0, len(r.ConfirmSet))
	for c := range r.ConfirmSet {
		if c == CategoryHuman {
			continue
		}
		keys = append(keys, c)
	}
	slices.Sort(keys)
	return joinCategories(keys)
}

// Confirm asks for y/n confirmation unless the policy permits auto-yes for
// this category. Short-circuits to (true, nil) iff r.Auto && !r.ConfirmSet[c].
// CategoryHuman never short-circuits (it is always in ConfirmSet).
func Confirm(r Resolved, c Category, in io.Reader, out io.Writer, prompt string) (bool, error) {
	if r.Auto && !r.ConfirmSet[c] {
		return true, nil
	}
	return promptio.ConfirmYN(in, out, prompt)
}

// ConfirmVia is the Asker-routed variant for BPMN bindings and other call
// sites that hand prompts to a Prompter abstraction instead of raw stdio.
// Same short-circuit semantics as Confirm.
func ConfirmVia(r Resolved, c Category, asker promptio.Asker, out io.Writer, prompt string) (bool, error) {
	if r.Auto && !r.ConfirmSet[c] {
		return true, nil
	}
	return promptio.ConfirmYNVia(asker, out, prompt)
}
