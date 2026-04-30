// Package classify is the fast-path ticket classifier for the ATDD pipeline
// driver. It decides whether a GitHub issue is a story, bug, task, or chore
// based on the deterministic signals — Projects v2 `Type` field and labels —
// described in `.claude/agents/atdd/atdd-dispatcher.md` (in the shop repo).
//
// The fast path replaces the LLM-based atdd-dispatcher agent for the
// deterministic majority case: when exactly one canonical type signal is
// present (Type field or a single canonical type-bearing label), the
// classifier emits that classification immediately and the driver loop
// dispatches the corresponding intake agent without an LLM round-trip.
//
// When signals are missing, ambiguous, or conflicting, Classify returns a
// Result with Route == Fallback and a populated Reasoning string. The driver
// loop is expected to dispatch the LLM atdd-dispatcher agent in that case
// (wired in a later session).
//
// All shell-outs to `gh` are routed through the GhRunner interface so tests
// can inject canned JSON without touching the real CLI.
package classify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Classification is the top-level ticket type emitted by the fast path.
// It is intentionally narrower than the agent-spec vocabulary: the agent
// recognises task subtypes (system-api-redesign, system-ui-redesign,
// external-system-api-change), but the v1 fast path only commits to the
// top-level value. Subtype resolution stays in the LLM fallback.
type Classification string

const (
	Story Classification = "story"
	Bug   Classification = "bug"
	Task  Classification = "task"
	Chore Classification = "chore"
)

// Route signals whether the caller should accept the Classification or
// dispatch the LLM atdd-dispatcher agent.
type Route string

const (
	// FastPath means the deterministic signals are unambiguous; the
	// Classification field is populated and the caller can dispatch the
	// matching intake agent directly.
	FastPath Route = "fastpath"

	// Fallback means at least one classification rule was inconclusive
	// (missing Type field and labels, multiple conflicting type-labels,
	// unknown Type-field value, etc.). Classification is empty; the caller
	// must dispatch the LLM agent and read Reasoning to seed its prompt.
	Fallback Route = "fallback"
)

// Result is the classifier's decision for a single issue.
type Result struct {
	IssueNum       int
	Classification Classification // empty when Route == Fallback
	Route          Route
	Reasoning      string   // human-readable; mirrored into LogPath
	LabelsSeen     []string // sorted, lowercased, deduped
	TypeField      string   // canonical Type project-field value, if present
}

// Options tunes one Classify call.
type Options struct {
	// Repo is the owner/repo passed to `gh issue view --repo`. When empty,
	// `gh` is invoked without --repo and falls back to the current git
	// remote (matching the agent body's `--repo optivem/shop` default).
	Repo string

	// LogPath is the file appended to with one line per Classify call.
	// Empty means "./classify.log".
	LogPath string

	// GhRunner is the injection point for tests. nil means "use the real
	// `gh` binary on PATH via os/exec".
	GhRunner GhRunner
}

// GhRunner is the surface Classify uses to invoke the GitHub CLI. The real
// implementation shells out to `gh`; tests substitute a fake that returns
// canned JSON.
type GhRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// realGhRunner is the production implementation: it invokes `gh` via
// os/exec and surfaces stderr in any returned error.
type realGhRunner struct{}

func (realGhRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("gh %s: %w (stderr: %s)",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// issueJSON mirrors the subset of `gh issue view --json …` output we need.
// The `gh` schema is well-defined; unknown fields are silently ignored.
type issueJSON struct {
	Number       int           `json:"number"`
	Title        string        `json:"title"`
	Labels       []labelJSON   `json:"labels"`
	ProjectItems []projItemRaw `json:"projectItems"`
}

type labelJSON struct {
	Name string `json:"name"`
}

// projItemRaw captures the projectItems entry shape. The Type field on
// Projects v2 surfaces under several possible JSON shapes depending on the
// `gh` version; we accept all common ones via raw decoding below.
type projItemRaw struct {
	Title string          `json:"title"`
	Type  json.RawMessage `json:"type"`
}

// Classify is the package's single entry point. It returns a Result for the
// given issue and writes one line to LogPath (or ./classify.log when
// unset). The error return is reserved for IO/exec failures — a successful
// "I cannot decide" outcome is a Result with Route == Fallback, not an
// error.
func Classify(ctx context.Context, issueNum int, opts Options) (Result, error) {
	if issueNum <= 0 {
		return Result{}, fmt.Errorf("classify: issueNum must be positive, got %d", issueNum)
	}

	runner := opts.GhRunner
	if runner == nil {
		runner = realGhRunner{}
	}

	logPath := opts.LogPath
	if logPath == "" {
		logPath = "classify.log"
	}

	args := []string{"issue", "view", strconv.Itoa(issueNum),
		"--json", "number,title,labels,projectItems"}
	if opts.Repo != "" {
		args = append(args, "--repo", opts.Repo)
	}

	raw, err := runner.Run(ctx, args...)
	if err != nil {
		return Result{}, fmt.Errorf("classify: fetch issue #%d: %w", issueNum, err)
	}

	var issue issueJSON
	if err := json.Unmarshal(raw, &issue); err != nil {
		return Result{}, fmt.Errorf("classify: parse issue #%d JSON: %w", issueNum, err)
	}

	res := decide(issueNum, issue)

	if logErr := appendLogLine(logPath, res); logErr != nil {
		// Logging failure does not invalidate the classification — surface
		// it but still return the Result so callers can proceed.
		return res, fmt.Errorf("classify: append log %q: %w", logPath, logErr)
	}
	return res, nil
}

// decide applies the deterministic rule set to the parsed issue payload.
// It is pure (no IO) so the table-driven tests can call it directly when
// they want to bypass the GhRunner round-trip — but the public API still
// goes through Classify, which exercises the full path including logging.
func decide(issueNum int, issue issueJSON) Result {
	labels := normalizeLabels(issue.Labels)
	typeField := extractTypeField(issue.ProjectItems)

	res := Result{
		IssueNum:   issueNum,
		LabelsSeen: labels,
		TypeField:  typeField,
	}

	// Rule 1: Prefer the Projects v2 Type field when present.
	//   Bug → bug, Task → task, Chore → chore, Feature/Story (or any
	//   other non-empty value) → story. An unknown Type value is treated
	//   as "unsure" and falls through to Fallback rather than guessing.
	if typeField != "" {
		switch normalizeTypeField(typeField) {
		case "bug":
			res.Classification = Bug
			res.Route = FastPath
			res.Reasoning = fmt.Sprintf(
				"Projects v2 Type field = %q → bug (fast path).", typeField)
			return res
		case "task":
			res.Classification = Task
			res.Route = FastPath
			res.Reasoning = fmt.Sprintf(
				"Projects v2 Type field = %q → task (fast path).", typeField)
			return res
		case "chore":
			res.Classification = Chore
			res.Route = FastPath
			res.Reasoning = fmt.Sprintf(
				"Projects v2 Type field = %q → chore (fast path).", typeField)
			return res
		case "feature", "story", "enhancement":
			res.Classification = Story
			res.Route = FastPath
			res.Reasoning = fmt.Sprintf(
				"Projects v2 Type field = %q → story (fast path).", typeField)
			return res
		default:
			// Unknown Type value: do not guess. Per atdd-dispatcher.md
			// rule 4 ("if signals genuinely conflict … stop and ask"),
			// we route to fallback so the LLM can disambiguate.
			res.Route = Fallback
			res.Reasoning = fmt.Sprintf(
				"Projects v2 Type field = %q is not a recognised value "+
					"(expected Bug/Task/Chore/Feature/Story); routing to LLM fallback.",
				typeField)
			return res
		}
	}

	// Rule 2: No Type field — derive from labels. We collect every
	// type-bearing label and bail to Fallback if more than one distinct
	// classification is signalled.
	hits := classifyLabels(labels)
	switch len(hits) {
	case 0:
		// No type signal at all. The agent body's rule 3 says to fall
		// back to body-shape inspection, which v1 does not implement —
		// route to LLM fallback.
		res.Route = Fallback
		res.Reasoning = "No Projects v2 Type field and no type-bearing label; routing to LLM fallback for body-shape inspection."
		return res
	case 1:
		var only Classification
		var src string
		for c, src0 := range hits {
			only, src = c, src0
		}
		res.Classification = only
		res.Route = FastPath
		res.Reasoning = fmt.Sprintf(
			"Single canonical type label %q → %s (fast path).", src, only)
		return res
	default:
		// Multiple distinct classifications — genuine conflict per rule 4.
		var srcs []string
		for c, src := range hits {
			srcs = append(srcs, fmt.Sprintf("%s→%s", src, c))
		}
		sort.Strings(srcs)
		res.Route = Fallback
		res.Reasoning = fmt.Sprintf(
			"Conflicting type-bearing labels [%s]; routing to LLM fallback.",
			strings.Join(srcs, ", "))
		return res
	}
}

// normalizeLabels lowercases, deduplicates, and sorts the label names so
// downstream rule logic is order-independent and the LabelsSeen field is
// stable across runs.
func normalizeLabels(in []labelJSON) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, l := range in {
		name := strings.ToLower(strings.TrimSpace(l.Name))
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// normalizeTypeField returns the lowercased, trimmed Type-field value for
// switch comparison. The empty string passes through.
func normalizeTypeField(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// extractTypeField scans the projectItems array for a Type-field value.
// `gh` surfaces it under several shapes across versions; we accept the
// common ones — a bare string, an object with a `name` field, or an
// object with a `title` field.
func extractTypeField(items []projItemRaw) string {
	for _, it := range items {
		if len(it.Type) == 0 || string(it.Type) == "null" {
			continue
		}
		// Try string first.
		var s string
		if err := json.Unmarshal(it.Type, &s); err == nil && s != "" {
			return s
		}
		// Then object with name/title.
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(it.Type, &obj); err == nil {
			for _, k := range []string{"name", "title", "value"} {
				if v, ok := obj[k]; ok {
					var sv string
					if json.Unmarshal(v, &sv) == nil && sv != "" {
						return sv
					}
				}
			}
		}
	}
	return ""
}

// classifyLabels maps each type-bearing label to a Classification. The
// returned map is keyed by Classification (so identical signals collapse
// to one entry — `bug` plus `ui-bug` is still one bug signal); the value
// is the originating label, used for diagnostics.
//
// The token-set mirrors atdd-dispatcher.md rule 2: a label is a type
// signal if its lowercased name contains one of the canonical tokens.
// We probe in longest-first order so `chore-cleanup` is recognised as
// chore rather than mis-tokenised by a shorter token.
func classifyLabels(labels []string) map[Classification]string {
	hits := map[Classification]string{}

	// Token list ordered longest-first to avoid `feature` being shadowed
	// by a hypothetical `feat` token; current tokens have no overlap but
	// the ordering is cheap insurance.
	tokens := []struct {
		token string
		cls   Classification
	}{
		// task family — task-label-family prefixes signal task.
		{"system-api-redesign", Task},
		{"system-ui-redesign", Task},
		{"external-system-api-change", Task},
		{"refactor", Task},
		{"feature", Story},
		{"story", Story},
		{"chore", Chore},
		{"task", Task},
		{"bug", Bug},
	}

	for _, l := range labels {
		for _, t := range tokens {
			if strings.Contains(l, t.token) {
				// First match per label wins; longest-first ordering
				// ensures the most specific token claims the label.
				if _, exists := hits[t.cls]; !exists {
					hits[t.cls] = l
				}
				break
			}
		}
	}
	return hits
}

// appendLogLine appends one classify.log entry. Format:
//
//	<RFC3339 ts> issue=<N> classification=<X|inconclusive> route=<r> labels=[<csv>] type_field=<v>
func appendLogLine(path string, res Result) error {
	classification := string(res.Classification)
	if classification == "" {
		classification = "inconclusive"
	}
	line := fmt.Sprintf(
		"%s issue=%d classification=%s route=%s labels=[%s] type_field=%s\n",
		time.Now().UTC().Format(time.RFC3339),
		res.IssueNum,
		classification,
		string(res.Route),
		strings.Join(res.LabelsSeen, ","),
		res.TypeField,
	)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		return err
	}
	return nil
}
