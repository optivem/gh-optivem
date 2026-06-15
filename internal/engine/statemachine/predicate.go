package statemachine

import (
	"fmt"
	"strings"
)

// evalPredicate evaluates one `when:` expression from the YAML against the
// Context state map. Empty expressions are treated as always-true (unguarded
// sequence flows).
//
// Two operators are supported, matching the YAML's actual usage:
//
//	<key> == <value>             string / bool equality
//	<key> in [<v1>, <v2>, ...]   membership
//
// Anything more elaborate is intentionally rejected — the YAML stays
// machine-readable, and complex routing logic moves into the gateway's bound
// Go function (which can read whatever it wants from the Context).
//
// Values may be quoted ("story") or bare (story); both forms work. Whitespace
// around operators is forgiven.
func evalPredicate(expr string, ctx *Context) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}

	// Try `in [...]` first because the `==` parser would otherwise consume
	// the `==` of bracket syntax (none present, but defensive).
	if idx := strings.Index(expr, " in "); idx > 0 {
		key := strings.TrimSpace(expr[:idx])
		listExpr := strings.TrimSpace(expr[idx+len(" in "):])
		if !strings.HasPrefix(listExpr, "[") || !strings.HasSuffix(listExpr, "]") {
			return false, fmt.Errorf("predicate %q: `in` operand must be bracketed list", expr)
		}
		items := splitList(listExpr[1 : len(listExpr)-1])
		actual := ctx.GetString(key)
		for _, it := range items {
			if it == actual {
				return true, nil
			}
		}
		return false, nil
	}

	if idx := strings.Index(expr, "=="); idx > 0 {
		key := strings.TrimSpace(expr[:idx])
		want := unquote(strings.TrimSpace(expr[idx+2:]))
		return ctx.GetString(key) == want, nil
	}

	return false, fmt.Errorf("predicate %q: unsupported syntax (only `==` and `in [...]` are recognised)", expr)
}

// splitList parses the body of an `in [...]` list into trimmed, unquoted items.
// Commas inside quoted values are not supported — the YAML does not use them
// (every item is a bare identifier or short string).
func splitList(body string) []string {
	parts := strings.Split(body, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, unquote(strings.TrimSpace(p)))
	}
	return out
}

// unquote strips surrounding double or single quotes, leaving bare identifiers
// untouched.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
