package clauderun

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseOutputs scans the agent's final result text for fenced YAML blocks
// whose top-level key is `outputs:` or `scope_exception:` and returns the
// decoded values flattened into a single map keyed by the eventual ctx.State
// key. The dispatcher writes each (key, value) pair into ctx.State so that
// downstream actions and gates can read them via ctx.Get / ctx.GetString.
//
// Two recognized top-level keys:
//
//   - `outputs:` — the general structured-output channel. Inner keys are
//     copied straight through to the returned map; values are coerced
//     against the type table below when the key is registered, and pass
//     through as `any` (typically `string`, `bool`, or `[]interface{}`)
//     when unknown.
//
//   - `scope_exception:` — the scope-exception envelope documented in
//     `internal/assets/runtime/shared/scope.md`. Its `files:` field is
//     flattened to `scope_exception_files` ([]string) and `reason:` is
//     flattened to `scope_exception_reason` (string), matching what the
//     `scope_exception_requested` gate already reads from ctx.State.
//
// **Loose match.** Agents may emit prose before and after the block; the
// parser scans for every fenced YAML block in the text and keeps the LAST
// one whose top-level key matches each name. This lets agents narrate
// their work (or quote example blocks earlier in the response) without
// confusing the dispatcher.
//
// **Missing-is-fine.** Agents that have nothing to emit are allowed to
// skip the block entirely — a missing block returns an empty map with a
// nil error. Downstream readers then see "not set in Context" via their
// own existing error paths, which is identical to today's behavior before
// structured output was wired up.
//
// **Malformed-is-loud.** A fenced block that DOES start with one of the
// recognized keys but fails to decode (broken YAML, wrong inner type)
// returns a non-nil error. The dispatcher routes this as Outcome.Err so
// the cycle stops with a clear "agent emitted malformed outputs block"
// signal rather than silently zeroing state.
func ParseOutputs(resultText string) (map[string]any, error) {
	out := map[string]any{}

	blocks := extractFencedYAMLBlocks(resultText)

	// Walk blocks in order and keep the LAST occurrence of each known
	// top-level key. This is the "loose match" rule: an example block
	// emitted earlier in the response is overridden by the final
	// authoritative block.
	var lastOutputs, lastScopeException *yamlBlock
	for i := range blocks {
		b := &blocks[i]
		switch b.topKey {
		case "outputs":
			lastOutputs = b
		case "scope_exception":
			lastScopeException = b
		}
	}

	if lastOutputs != nil {
		var decoded map[string]any
		if err := yaml.Unmarshal([]byte(lastOutputs.body), &decoded); err != nil {
			return nil, fmt.Errorf("clauderun: parse outputs: yaml unmarshal: %w", err)
		}
		inner, ok := decoded["outputs"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("clauderun: parse outputs: expected mapping under `outputs:`, got %T", decoded["outputs"])
		}
		for k, v := range inner {
			coerced, err := coerceKnownKey(k, v)
			if err != nil {
				return nil, fmt.Errorf("clauderun: parse outputs: %w", err)
			}
			out[k] = coerced
		}
	}

	if lastScopeException != nil {
		var decoded map[string]any
		if err := yaml.Unmarshal([]byte(lastScopeException.body), &decoded); err != nil {
			return nil, fmt.Errorf("clauderun: parse outputs: scope_exception yaml unmarshal: %w", err)
		}
		inner, ok := decoded["scope_exception"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("clauderun: parse outputs: expected mapping under `scope_exception:`, got %T", decoded["scope_exception"])
		}
		files, err := coerceStringSlice("files", inner["files"])
		if err != nil {
			return nil, fmt.Errorf("clauderun: parse outputs: scope_exception.%w", err)
		}
		out["scope_exception_files"] = files
		if reasonRaw, present := inner["reason"]; present {
			reason, ok := reasonRaw.(string)
			if !ok {
				return nil, fmt.Errorf("clauderun: parse outputs: scope_exception.reason: expected string, got %T", reasonRaw)
			}
			out["scope_exception_reason"] = reason
		}
	}

	return out, nil
}

// knownOutputKeys is the type-coercion table for keys emitted under the
// `outputs:` block. The dispatcher's downstream readers (actions, gates)
// require concrete Go types — e.g. `runTargetedTests` casts
// ctx.State["test_names"] to `[]string` and errors if the type doesn't
// match. Locking the coercion at parse time means every reader gets the
// right shape without each one repeating the cast.
//
// The parser is the single seam where the agent-side string/list values
// become typed Go values.
//
// Unregistered keys pass through as `any` (typically string, bool, or
// []interface{}). This is the forward-compat seam: a future prompt
// amendment can emit a new key without an immediate parser change, as
// long as the downstream reader is OK with the raw YAML decoding
// (boolean flags decode as `bool` straight away, for example, which is
// what the `boolGate` family already expects).
var knownOutputKeys = map[string]outputKeyKind{
	"test_names": kindStringSlice,
	"suite":      kindString,
}

type outputKeyKind int

const (
	kindUnknown outputKeyKind = iota
	kindString
	kindStringSlice
)

func coerceKnownKey(key string, value any) (any, error) {
	kind, registered := knownOutputKeys[key]
	if !registered {
		return value, nil
	}
	switch kind {
	case kindString:
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("outputs.%s: expected string, got %T", key, value)
		}
		return s, nil
	case kindStringSlice:
		return coerceStringSlice("outputs."+key, value)
	default:
		return value, nil
	}
}

// coerceStringSlice converts yaml.v3's `[]interface{}` decoding of a YAML
// sequence into `[]string`. The label is the dotted path used in error
// messages so the operator can locate the offending key (e.g.
// `scope_exception.files` or `outputs.test_names`).
func coerceStringSlice(label string, value any) ([]string, error) {
	if value == nil {
		return nil, fmt.Errorf("%s: missing sequence", label)
	}
	raw, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected sequence, got %T", label, value)
	}
	out := make([]string, 0, len(raw))
	for i, item := range raw {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%d]: expected string, got %T", label, i, item)
		}
		out = append(out, s)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Fenced-block extraction
// ---------------------------------------------------------------------------

// yamlBlock is one fenced ```yaml ... ``` (or unmarked ``` ... ```)
// block found in the agent's final response. `topKey` is the first
// non-empty, non-comment top-level key inside the block — used to route
// the block to the matching parser branch ("outputs" / "scope_exception"
// / anything else).
type yamlBlock struct {
	topKey string
	body   string
}

// fenceOpenRe matches the opening ``` (or ```yaml / ```yml) of a fenced
// code block. The language tag is optional and case-insensitive; we
// accept any tag because agents in practice forget to add `yaml`
// (or add `yml`, `YAML`, etc.). The closing fence is matched as a bare
// ``` on its own line.
var fenceOpenRe = regexp.MustCompile("(?m)^```(?:[a-zA-Z]*)\\s*$")

// fenceCloseRe matches the closing ``` on its own line.
var fenceCloseRe = regexp.MustCompile("(?m)^```\\s*$")

// extractFencedYAMLBlocks walks `text` and returns every fenced code
// block, paired with its detected top-level YAML key. Blocks whose first
// substantive line is NOT a `key:` line are skipped — they're presumed
// to be code samples or shell snippets, not structured output.
//
// The blocks are returned in source order; callers that want "last
// match wins" semantics iterate and overwrite, which is exactly what
// ParseOutputs does for `outputs:` / `scope_exception:`.
func extractFencedYAMLBlocks(text string) []yamlBlock {
	var out []yamlBlock
	pos := 0
	for pos < len(text) {
		openLoc := fenceOpenRe.FindStringIndex(text[pos:])
		if openLoc == nil {
			break
		}
		openStart := pos + openLoc[0]
		openEnd := pos + openLoc[1]
		// Search for the closing fence after the opening fence.
		closeLoc := fenceCloseRe.FindStringIndex(text[openEnd:])
		if closeLoc == nil {
			// Unclosed fence — treat the rest as a single block so the
			// caller still sees a (possibly malformed) attempt. Bounded
			// to end-of-text.
			body := text[openEnd:]
			if k := detectTopKey(body); k != "" {
				out = append(out, yamlBlock{topKey: k, body: body})
			}
			break
		}
		closeStart := openEnd + closeLoc[0]
		body := text[openEnd:closeStart]
		if k := detectTopKey(body); k != "" {
			out = append(out, yamlBlock{topKey: k, body: body})
		}
		pos = openEnd + closeLoc[1]
		_ = openStart // openStart is unused but documents the intent for future readers.
	}
	return out
}

// detectTopKey returns the first non-empty, non-comment top-level key
// in a YAML body (a line of the form `key:` with no leading indentation).
// Returns the empty string when no such line exists — the block is then
// skipped by extractFencedYAMLBlocks.
func detectTopKey(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if trimmed == "" {
			continue
		}
		// Skip leading whitespace lines, comments, and YAML document
		// separators — none of these mark the top-level key.
		if strings.HasPrefix(trimmed, " ") || strings.HasPrefix(trimmed, "\t") {
			continue
		}
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "---") {
			continue
		}
		// A top-level YAML mapping key ends in `:` (possibly followed by
		// whitespace). Inline scalars (`key: value`) are also accepted —
		// the colon position is what we look for.
		idx := strings.IndexByte(trimmed, ':')
		if idx <= 0 {
			continue
		}
		return trimmed[:idx]
	}
	return ""
}
