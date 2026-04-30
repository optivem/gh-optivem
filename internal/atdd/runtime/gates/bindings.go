// Bindings — Go implementations of every gateway `binding:` referenced in
// docs/atdd/process/process-flow.yaml.
//
// Each binding is a `statemachine.NodeFn` that:
//
//   1. Reads the live Context for a pre-set value under the binding key. If
//      an upstream service task or intake agent has already declared the
//      result (e.g. classify_ticket sets ticket_type, run_smoke_test sets
//      smoke_test_passes), that value is returned verbatim. This is the
//      common path in production runs and lets transitions tests seed state
//      directly without touching shell-outs.
//
//   2. Falls back to a forward-looking user prompt when the value is absent.
//      Gates after a WRITE phase ("did the DSL interface change?") are
//      questions only the user can answer in v1 — git-diff inspection is a
//      v2 candidate. Prompt strings come from the YAML node descriptions so
//      the engine and the diagram stay in sync.
//
// Tests substitute fake Prompter / GhRunner / GitRunner implementations via
// Deps; production callers pass a Deps zero-value and the package falls back
// to real stdin / `gh` / `git`.
package gates

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// Deps bundles the side-effecting collaborators every binding may need.
// All fields are optional — a zero-value Deps falls back to real shell-outs
// and the OS stdin/stdout. Tests pass non-nil fakes for hermeticity.
type Deps struct {
	Gh       GhRunner
	Git      GitRunner
	Prompter Prompter
}

// Prompter asks the user one yes/no/value question and returns the trimmed
// reply. Implementations must surface I/O errors rather than silently
// returning the empty string — empty replies have meaning ("user pressed
// Enter") that the caller may want to interpret.
type Prompter interface {
	Ask(prompt string) (string, error)
}

// GhRunner runs the `gh` CLI. The default implementation is execGh.
type GhRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// GitRunner runs the `git` CLI. The default implementation is execGit.
type GitRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// withDefaults populates any nil collaborator with its real exec / stdin
// counterpart. Returns a copy so tests that pass a partial Deps observe the
// real defaults for unset fields rather than getting silently nil-Run errors.
func (d Deps) withDefaults() Deps {
	if d.Gh == nil {
		d.Gh = execGh{}
	}
	if d.Git == nil {
		d.Git = execGit{}
	}
	if d.Prompter == nil {
		d.Prompter = stdinPrompter{}
	}
	return d
}

// RegisterAll wires every YAML binding name to its Go implementation under
// the supplied registry. Call this once during driver startup before
// engine.Bind so the engine's GateFn lookup hits a populated registry.
//
// Listed in YAML order so a reader can scan top-to-bottom and confirm every
// `binding:` has a matching entry. Adding a new gate = one line here, one
// new method on bindings, plus a transitions-test case.
func RegisterAll(r *Registry, deps Deps) {
	deps = deps.withDefaults()
	b := bindings{deps: deps}
	r.Register("dsl_interface_changed", b.dslInterfaceChanged)
	r.Register("external_system_driver_interface_changed", b.externalSystemDriverInterfaceChanged)
	r.Register("system_driver_interface_changed", b.systemDriverInterfaceChanged)
	r.Register("ticket_type", b.ticketType)
	r.Register("legacy_coverage_section_present", b.legacyCoverageSectionPresent)
	r.Register("change_driven_ac_produced", b.changeDrivenACProduced)
	r.Register("external_system_driver_exists", b.externalSystemDriverExists)
	r.Register("external_system_test_instance_accessible", b.externalSystemTestInstanceAccessible)
	r.Register("smoke_test_passes", b.smokeTestPasses)
	r.Register("structural_test_mode", b.structuralTestMode)
}

// bindings is a thin closure-receiver so each method has access to deps
// without taking it as a parameter. Keeping the methods pure-as-possible
// (only Context + deps) lets the test suite exercise them through the same
// NodeFn contract the engine sees.
type bindings struct {
	deps Deps
}

// ---------------------------------------------------------------------------
// Boolean gates after WRITE phases — forward-looking, prompt-driven
// ---------------------------------------------------------------------------

func (b bindings) dslInterfaceChanged(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"dsl_interface_changed",
		"DSL interface changed in this phase? [y/N]: ")
}

func (b bindings) externalSystemDriverInterfaceChanged(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"external_system_driver_interface_changed",
		"External System Driver interface changed? [y/N]: ")
}

func (b bindings) systemDriverInterfaceChanged(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"system_driver_interface_changed",
		"System Driver interface changed? [y/N]: ")
}

// ---------------------------------------------------------------------------
// String / enum gates — backed by upstream actions, with prompt fallback
// ---------------------------------------------------------------------------

// ticketType reads the classification produced by the classify_ticket service
// task. Range mirrors the YAML predicates: story | bug | chore |
// system-api-task | system-ui-task | external-api-task. The action is
// expected to write the canonical lowercased form.
func (b bindings) ticketType(ctx *statemachine.Context) statemachine.Outcome {
	v := ctx.GetString("ticket_type")
	if v != "" {
		return statemachine.Outcome{Value: v}
	}
	answer, err := b.deps.Prompter.Ask(
		"Ticket type? (story | bug | chore | system-api-task | system-ui-task | external-api-task): ")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("ticket_type: %w", err)}
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	switch answer {
	case "story", "bug", "chore",
		"system-api-task", "system-ui-task", "external-api-task":
		return statemachine.Outcome{Value: answer}
	default:
		return statemachine.Outcome{Err: fmt.Errorf("ticket_type: unrecognised value %q", answer)}
	}
}

// structuralTestMode prompts for the TEST gate's three-way choice. Always
// asks the user — there is no upstream action that pre-decides this. Empty
// reply (Enter) defaults to `compile` because that is the safest option that
// still surfaces drift without consuming the docker stack.
func (b bindings) structuralTestMode(ctx *statemachine.Context) statemachine.Outcome {
	if v := ctx.GetString("structural_test_mode"); v != "" {
		return statemachine.Outcome{Value: v}
	}
	answer, err := b.deps.Prompter.Ask("TEST mode? (full | compile | skip) [compile]: ")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("structural_test_mode: %w", err)}
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "" {
		answer = "compile"
	}
	switch answer {
	case "full", "compile", "skip":
		return statemachine.Outcome{Value: answer}
	default:
		return statemachine.Outcome{Err: fmt.Errorf("structural_test_mode: unrecognised value %q", answer)}
	}
}

// ---------------------------------------------------------------------------
// Boolean gates — backed by upstream actions or one-off prompts
// ---------------------------------------------------------------------------

// legacyCoverageSectionPresent reads the issue body via `gh issue view` and
// scans for an H2 heading named `Legacy Coverage`. Falls back to a prompt
// when no issue number is in the Context (off-board mode).
func (b bindings) legacyCoverageSectionPresent(ctx *statemachine.Context) statemachine.Outcome {
	if v := ctx.Get("legacy_coverage_section_present"); v != nil {
		return outcomeFromBoolish(v)
	}
	issueNum := ctx.GetString("issue_num")
	if issueNum == "" {
		return b.boolGate(ctx,
			"legacy_coverage_section_present",
			"Legacy Coverage section present in the issue? [y/N]: ")
	}
	args := []string{"issue", "view", issueNum, "--json", "body"}
	if repo := ctx.GetString("issue_repo"); repo != "" {
		args = append(args, "--repo", repo)
	}
	out, err := b.deps.Gh.Run(context.Background(), args...)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("legacy_coverage_section_present: gh issue view: %w", err)}
	}
	body := extractIssueBody(out)
	return statemachine.Outcome{Bool: containsLegacyCoverageHeading(body)}
}

// changeDrivenACProduced is set by the intake agent's COMMIT output (atdd-
// story / atdd-bug / atdd-task / atdd-chore decide whether their scenarios
// are change-driven). v1 falls back to a prompt for runs where the Context
// is unset (manual replays, transitions tests bypassing intake).
func (b bindings) changeDrivenACProduced(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"change_driven_ac_produced",
		"Change-driven AC produced by the intake agent? [y/N]: ")
}

// externalSystemDriverExists is asked once at the top of the onboarding
// sub-flow. The semantic check is "does this repo already have a driver
// implementation for the external system?" — file-system inspection is
// possible but the path schema varies per consumer, so v1 prompts.
func (b bindings) externalSystemDriverExists(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"external_system_driver_exists",
		"External System Driver already exists in this repo? [y/N]: ")
}

// externalSystemTestInstanceAccessible is the second onboarding gate.
// Same prompt-or-Context shape as the driver-exists check.
func (b bindings) externalSystemTestInstanceAccessible(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"external_system_test_instance_accessible",
		"Test instance of the external system accessible? [y/N]: ")
}

// smokeTestPasses is set by the run_smoke_test service task's Outcome.
// When called without that upstream having run (e.g. a transitions test
// driving the gateway directly), fall back to a prompt for the sake of
// hand-debugging.
func (b bindings) smokeTestPasses(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"smoke_test_passes",
		"Smoke test passed? [y/N]: ")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// boolGate is the canonical Context-or-prompt shape: read an existing value,
// otherwise ask the user a yes/no question and coerce the answer.
func (b bindings) boolGate(ctx *statemachine.Context, key, prompt string) statemachine.Outcome {
	if v, ok := ctx.State[key]; ok {
		return outcomeFromBoolish(v)
	}
	answer, err := b.deps.Prompter.Ask(prompt)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("%s: %w", key, err)}
	}
	yes, ok := parseYesNo(answer)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("%s: unrecognised yes/no answer %q", key, strings.TrimSpace(answer))}
	}
	return statemachine.Outcome{Bool: yes}
}

// outcomeFromBoolish coerces an arbitrary Context value to an Outcome. We
// accept native bool, the strings "true"/"false"/"yes"/"no", and anything
// else that GetString-style coercion produces — robust to upstream service
// tasks that store the value in any of those forms.
func outcomeFromBoolish(v any) statemachine.Outcome {
	switch t := v.(type) {
	case bool:
		return statemachine.Outcome{Bool: t}
	case string:
		yes, ok := parseYesNo(t)
		if !ok {
			// Treat the string as already-canonical; the engine writes
			// whatever it was set to back to State, so unrecognised text
			// surfaces upstream rather than silently flipping false.
			return statemachine.Outcome{Value: t}
		}
		return statemachine.Outcome{Bool: yes}
	default:
		return statemachine.Outcome{Bool: false}
	}
}

// parseYesNo accepts the yes/no shorthands the academy prompts use across
// the rest of the toolchain: "y", "yes", "true", "1" → true; "n", "no",
// "false", "0", "" → false. Unrecognised replies fail the second-return
// flag so the caller can surface "unrecognised answer" instead of silently
// counting them as `no`.
func parseYesNo(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes", "true", "1":
		return true, true
	case "n", "no", "false", "0", "":
		return false, true
	default:
		return false, false
	}
}

// extractIssueBody pulls .body out of a `gh issue view --json body` payload
// without dragging encoding/json into the gate body — keeps the dependency
// surface small. The format is well-defined: a JSON object with a string
// "body" field. We do a minimal hand-parse to find the body value.
func extractIssueBody(raw []byte) string {
	const key = `"body"`
	idx := bytes.Index(raw, []byte(key))
	if idx < 0 {
		return ""
	}
	rest := raw[idx+len(key):]
	colon := bytes.IndexByte(rest, ':')
	if colon < 0 {
		return ""
	}
	rest = bytes.TrimLeft(rest[colon+1:], " \t\r\n")
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	rest = rest[1:]
	// Walk until the matching closing quote, honouring \\ escapes and \" escapes.
	var sb strings.Builder
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if c == '\\' && i+1 < len(rest) {
			next := rest[i+1]
			switch next {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			default:
				sb.WriteByte(next)
			}
			i++
			continue
		}
		if c == '"' {
			return sb.String()
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// containsLegacyCoverageHeading scans an issue body for an H2 (or deeper)
// markdown heading whose text matches "Legacy Coverage" case-insensitively.
// Per cycles.md, the section is conventionally `## Legacy Coverage`.
func containsLegacyCoverageHeading(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Drop leading hashes and any single space separator.
		text := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		if strings.EqualFold(text, "Legacy Coverage") {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Default exec runners + stdin prompter
// ---------------------------------------------------------------------------

type execGh struct{}

func (execGh) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("gh %s: %w (stderr: %s)",
				strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return out, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

type execGit struct{}

func (execGit) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("git %s: %w (stderr: %s)",
				strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return out, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// stdinPrompter is the default Prompter — writes the prompt to stderr (so
// it does not contaminate any stdout-captured output downstream of the
// driver) and reads a single line from stdin.
type stdinPrompter struct{}

func (stdinPrompter) Ask(prompt string) (string, error) {
	if _, err := fmt.Fprint(os.Stderr, prompt); err != nil {
		return "", err
	}
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return line, nil
}
