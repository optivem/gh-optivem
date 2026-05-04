// Bindings — Go implementations of every service-task `action:` referenced
// in docs/atdd/process/process-flow.yaml.
//
// Actions are the mechanical work of the pipeline: read the project board,
// move cards, classify the ticket, run a smoke test, commit a phase, etc.
// They wrap the existing helpers under runtime/board, runtime/classify, and
// runtime/release where one already exists; everything else is implemented
// directly in this file using the same shell-out + dependency-injection
// pattern (Deps with Gh / Git / Prompter / Stdout, all defaulting to real
// implementations when nil).
//
// Every action returns `statemachine.Outcome` with Err set on hard failures.
// User-driven aborts (e.g. answering "no" to "Can I commit?") also surface
// as Err so the engine halts the run — silent decline would route past a
// gate the user explicitly closed.
package actions

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/board"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/classify"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/release"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// Deps bundles the side-effecting collaborators every action may need. All
// fields are optional; a zero-value Deps falls back to real shell-outs and
// the OS stdin/stdout. Tests pass non-nil fakes for hermeticity.
type Deps struct {
	Gh         GhRunner
	Git        GitRunner
	Shell      ShellRunner // for compile-all.sh / test-all.sh / docker compose
	Prompter   Prompter
	Stdout     io.Writer
	Stderr     io.Writer
	ProjectURL string // optional — explicit override for board operations
	RepoPath   string // optional — defaults to current working directory
}

// Prompter is the same interface gates uses; redefined here so the actions
// package does not import gates (each registry stays self-contained).
type Prompter interface {
	Ask(prompt string) (string, error)
}

// GhRunner runs the `gh` CLI.
type GhRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// GitRunner runs the `git` CLI.
type GitRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ShellRunner runs an arbitrary command. We use a bash-style CommandLine
// (no argv split) because the academy's compile-all.sh / test-all.sh wrap
// multi-stage shell pipelines and the most testable contract is "give the
// fake the exact string you would type at a prompt".
type ShellRunner interface {
	Run(ctx context.Context, commandLine string) ([]byte, error)
}

func (d Deps) withDefaults() Deps {
	if d.Gh == nil {
		d.Gh = realGh{}
	}
	if d.Git == nil {
		d.Git = realGit{}
	}
	if d.Shell == nil {
		d.Shell = realShell{}
	}
	if d.Prompter == nil {
		d.Prompter = stdinPrompter{}
	}
	if d.Stdout == nil {
		d.Stdout = os.Stdout
	}
	if d.Stderr == nil {
		d.Stderr = os.Stderr
	}
	return d
}

// RegisterAll wires every YAML action name to its implementation.
func RegisterAll(r *Registry, deps Deps) {
	deps = deps.withDefaults()
	a := actions{deps: deps}
	r.Register("pick_top_ready", a.pickTopReady)
	r.Register("move_to_in_progress", a.moveToInProgress)
	r.Register("classify_ticket", a.classifyTicket)
	r.Register("move_to_in_acceptance", a.moveToInAcceptance)
	r.Register("run_smoke_test", a.runSmokeTest)
	r.Register("commit_onboarding", a.commitOnboarding)
	r.Register("compile_in_scope", a.compileInScope)
	r.Register("run_sample_suite", a.runSampleSuite)
	r.Register("print_drift_warning", a.printDriftWarning)
	r.Register("ask_can_i_commit", a.askCanICommit)
	r.Register("commit_phase", a.commitPhase)
	r.Register("tick_checklist", a.tickChecklist)
}

type actions struct {
	deps Deps
}

// ---------------------------------------------------------------------------
// Board-backed actions
// ---------------------------------------------------------------------------

// pickTopReady reads the project's Ready column and writes the picked issue
// number, URL, repo, project ID, and item ID into Context state. Downstream
// gates and actions read those keys; the engine itself does not interpret
// them.
//
// On an empty Ready column the action surfaces ErrEmptyReady as Outcome.Err
// so the run halts with a "nothing to do" message. The caller (driver) can
// catch that specific sentinel and exit zero rather than crashing.
func (a actions) pickTopReady(ctx *statemachine.Context) statemachine.Outcome {
	pick, err := board.PickTopReady(context.Background(), board.Options{
		ProjectURL: a.deps.ProjectURL,
		RepoPath:   a.deps.RepoPath,
		GhRunner:   ghAdapter{a.deps.Gh},
	})
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("pick_top_ready: %w", err)}
	}
	ctx.Set("issue_num", strconv.Itoa(pick.IssueNum))
	ctx.Set("issue_url", pick.IssueURL)
	ctx.Set("issue_title", pick.Title)
	ctx.Set("issue_repo", pick.Repo)
	ctx.Set("project_id", pick.ProjectID)
	ctx.Set("item_id", pick.ItemID)
	fmt.Fprintf(a.deps.Stdout, "Picked top Ready: #%d %s (%s)\n", pick.IssueNum, pick.Title, pick.IssueURL)
	return statemachine.Outcome{}
}

// moveToInProgress changes the project item status to "In progress". Reads
// project_id and item_id from Context — populated by pick_top_ready in
// board mode, or seeded by the caller in specific-issue mode.
func (a actions) moveToInProgress(ctx *statemachine.Context) statemachine.Outcome {
	projectID := ctx.GetString("project_id")
	itemID := ctx.GetString("item_id")
	if projectID == "" || itemID == "" {
		// In specific-issue mode the caller may not have populated the
		// board IDs yet — try to resolve them lazily.
		issueNum, err := strconv.Atoi(ctx.GetString("issue_num"))
		if err != nil || issueNum <= 0 {
			return statemachine.Outcome{Err: fmt.Errorf("move_to_in_progress: project_id/item_id not set and issue_num is not a positive integer (%q)", ctx.GetString("issue_num"))}
		}
		// v1 surfaces the gap rather than guessing; the driver wires up
		// resolve-by-issue once Session 3 adds it. For now fail clearly.
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_progress: project_id/item_id not in Context (specific-issue mode requires explicit pre-resolution)")}
	}
	err := board.MoveToInProgress(context.Background(), projectID, itemID, board.Options{
		ProjectURL: a.deps.ProjectURL,
		RepoPath:   a.deps.RepoPath,
		GhRunner:   ghAdapter{a.deps.Gh},
	})
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_progress: %w", err)}
	}
	fmt.Fprintln(a.deps.Stdout, "Moved card to In progress.")
	return statemachine.Outcome{}
}

// moveToInAcceptance ticks every issue checklist box and changes the project
// item status to "In acceptance". Both halves are best-effort: we report
// each step, and a missing column / field is a hard fail (the column is
// part of the documented kanban shape).
func (a actions) moveToInAcceptance(ctx *statemachine.Context) statemachine.Outcome {
	if err := a.tickRemoteChecklist(ctx); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_acceptance: tick checklist: %w", err)}
	}
	projectID := ctx.GetString("project_id")
	itemID := ctx.GetString("item_id")
	if projectID == "" || itemID == "" {
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_acceptance: project_id/item_id not in Context")}
	}
	statusFieldID, optionID, err := lookupStatusOption(context.Background(), a.deps.Gh, ctx, "In acceptance")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_acceptance: %w", err)}
	}
	if _, err := a.deps.Gh.Run(context.Background(),
		"project", "item-edit",
		"--id", itemID,
		"--field-id", statusFieldID,
		"--project-id", projectID,
		"--single-select-option-id", optionID,
	); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_acceptance: gh project item-edit: %w", err)}
	}
	fmt.Fprintln(a.deps.Stdout, "Moved card to In acceptance.")
	return statemachine.Outcome{}
}

// ---------------------------------------------------------------------------
// Classification
// ---------------------------------------------------------------------------

// classifyTicket runs the deterministic fast-path classifier and writes
// `ticket_type` into Context. When the classifier returns FastPath, the
// stored value is the canonical class (story | bug | task | chore). For
// `task` we additionally prompt for the subtype because the YAML predicates
// branch on system-api-task / system-ui-task / external-api-task — the
// fast-path package only commits to the top level.
//
// Fallback (LLM dispatcher) is currently surfaced as a hard error so the
// run halts and the user can manually classify; Session 3 wires the agent
// dispatch.
func (a actions) classifyTicket(ctx *statemachine.Context) statemachine.Outcome {
	issueNum, err := strconv.Atoi(ctx.GetString("issue_num"))
	if err != nil || issueNum <= 0 {
		return statemachine.Outcome{Err: fmt.Errorf("classify_ticket: issue_num not set or not a positive integer (%q)", ctx.GetString("issue_num"))}
	}
	res, err := classify.Classify(context.Background(), issueNum, classify.Options{
		Repo:     ctx.GetString("issue_repo"),
		GhRunner: ghClassifyAdapter{a.deps.Gh},
	})
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("classify_ticket: %w", err)}
	}
	if res.Route == classify.Fallback {
		return statemachine.Outcome{Err: fmt.Errorf("classify_ticket: fallback required (%s); LLM dispatcher not wired in v1", res.Reasoning)}
	}
	final := string(res.Classification)
	if res.Classification == classify.Task {
		// Auto-resolve the subtype when exactly one of the known subtype
		// labels is on the issue — the renamed shop labels (system-api-task,
		// system-ui-task, external-api-task) double as both the type signal
		// and the routing value, so prompting again would be redundant.
		knownSubtypes := []string{"system-api-task", "system-ui-task", "external-api-task"}
		var matched []string
		for _, l := range res.LabelsSeen {
			for _, k := range knownSubtypes {
				if l == k {
					matched = append(matched, l)
					break
				}
			}
		}
		switch len(matched) {
		case 1:
			final = matched[0]
		case 0:
			subtype, err := a.deps.Prompter.Ask(
				"Task subtype? (system-api-task | system-ui-task | external-api-task): ")
			if err != nil {
				return statemachine.Outcome{Err: fmt.Errorf("classify_ticket: %w", err)}
			}
			subtype = strings.ToLower(strings.TrimSpace(subtype))
			switch subtype {
			case "system-api-task", "system-ui-task", "external-api-task":
				final = subtype
			default:
				return statemachine.Outcome{Err: fmt.Errorf("classify_ticket: unrecognised task subtype %q", subtype)}
			}
		default:
			return statemachine.Outcome{Err: fmt.Errorf("classify_ticket: multiple subtype labels on issue (%s); resolve manually", strings.Join(matched, ", "))}
		}
	}
	ctx.Set("ticket_type", final)
	for k, v := range classificationFromTicketType(final) {
		ctx.Set(k, v)
	}
	fmt.Fprintf(a.deps.Stdout, "Classified #%d as %s.\n", issueNum, final)
	a.printClassifiedSections(ctx, issueNum)
	return statemachine.Outcome{}
}

<<<<<<< HEAD
// printClassifiedSections fetches the issue body and prints the three
// canonical sections users want to see after classification: Legacy
// Acceptance Criteria, Acceptance Criteria, and Checklist. Best-effort —
// fetch failures and missing sections are silent. Each section is printed
// only when present in the body.
func (a actions) printClassifiedSections(ctx *statemachine.Context, issueNum int) {
	args := []string{"issue", "view", strconv.Itoa(issueNum), "--json", "body"}
	if repo := ctx.GetString("issue_repo"); repo != "" {
		args = append(args, "--repo", repo)
	}
	out, err := a.deps.Gh.Run(context.Background(), args...)
	if err != nil {
		return
	}
	body := extractIssueBody(out)
	for _, heading := range []string{"Legacy Acceptance Criteria", "Acceptance Criteria", "Checklist"} {
		section, ok := extractIssueSection(body, heading)
		if !ok {
			continue
		}
		fmt.Fprintf(a.deps.Stdout, "\n## %s\n\n%s\n", heading, section)
	}
=======
// classificationFromTicketType maps the deterministic ticket_type into the
// change classification consumed by run_cycle / da_cycle:
//
//	change_type     behavior | structure
//	change_subtype  interface | implementation  (only when type == structure)
//	change_scope    system | external_system    (only when subtype == interface)
//	change_channel  api | ui                    (only when scope == system)
//
// Axes that do not apply for a given ticket_type are omitted from the map so
// the gate's binding falls back to its prompt path if ever reached.
func classificationFromTicketType(ticketType string) map[string]string {
	switch ticketType {
	case "story", "bug":
		return map[string]string{"change_type": "behavior"}
	case "chore":
		return map[string]string{"change_type": "structure", "change_subtype": "implementation"}
	case "system-api-task":
		return map[string]string{"change_type": "structure", "change_subtype": "interface", "change_scope": "system", "change_channel": "api"}
	case "system-ui-task":
		return map[string]string{"change_type": "structure", "change_subtype": "interface", "change_scope": "system", "change_channel": "ui"}
	case "external-api-task":
		return map[string]string{"change_type": "structure", "change_subtype": "interface", "change_scope": "external_system"}
	}
	return nil
>>>>>>> 0597e55cbe89f5d620b4cd45fef37e5af661d379
}

// ---------------------------------------------------------------------------
// Smoke test
// ---------------------------------------------------------------------------

// runSmokeTest prompts the user to run the smoke test and report the result.
// v1 ships with a prompt rather than a real docker invocation because the
// stack-up command is repo-specific (`gh optivem run system --system-config …`).
// The Outcome's Bool is also recorded under `smoke_test_passes` so the
// downstream gateway reads it back through the standard wrapGateway path.
func (a actions) runSmokeTest(ctx *statemachine.Context) statemachine.Outcome {
	answer, err := a.deps.Prompter.Ask("Run the smoke test now and report the result: did it pass? [y/N]: ")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("run_smoke_test: %w", err)}
	}
	yes, ok := parseYesNo(answer)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("run_smoke_test: unrecognised yes/no answer %q", strings.TrimSpace(answer))}
	}
	ctx.Set("smoke_test_passes", yes)
	if yes {
		fmt.Fprintln(a.deps.Stdout, "Smoke test passed.")
	} else {
		fmt.Fprintln(a.deps.Stderr, "Smoke test failed — flow will route to ASK_SUPPORT.")
	}
	return statemachine.Outcome{Bool: yes}
}

// ---------------------------------------------------------------------------
// Commit / release actions
// ---------------------------------------------------------------------------

// commitOnboarding creates a single commit named after the external system
// being onboarded. The system name comes from Context["external_system_name"]
// (set by the onboarding sub-flow's intake) or falls back to a prompt.
func (a actions) commitOnboarding(ctx *statemachine.Context) statemachine.Outcome {
	name := ctx.GetString("external_system_name")
	if name == "" {
		ans, err := a.deps.Prompter.Ask("External system name (for commit message): ")
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("commit_onboarding: %w", err)}
		}
		name = strings.TrimSpace(ans)
		if name == "" {
			return statemachine.Outcome{Err: fmt.Errorf("commit_onboarding: external system name is required")}
		}
	}
	msg := fmt.Sprintf("External System Onboarding | %s", name)
	if err := release.Commit(context.Background(), release.CommitOptions{
		Message:   msg,
		Confirm:   a.confirmer(),
		GitRunner: gitReleaseAdapter{a.deps.Git},
		Stdout:    a.deps.Stdout,
	}); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("commit_onboarding: %w", err)}
	}
	return statemachine.Outcome{}
}

// commitPhase creates the phase commit. The message format mirrors
// cycles.md: `<Ticket Title> | <PHASE NAME>`. Both pieces come from
// Context — issue_title from pick_top_ready / move_to_in_progress, phase
// from the call_activity params (substituted into the action's params at
// dispatch time).
func (a actions) commitPhase(ctx *statemachine.Context) statemachine.Outcome {
	title := ctx.GetString("issue_title")
	if title == "" {
		title = "<unknown ticket>"
	}
	phase := ctx.Params["phase"]
	if phase == "" {
		phase = "PHASE"
	}
	msg := fmt.Sprintf("%s | %s", title, phase)
	if err := release.Commit(context.Background(), release.CommitOptions{
		Message:   msg,
		Confirm:   a.confirmer(),
		GitRunner: gitReleaseAdapter{a.deps.Git},
		Stdout:    a.deps.Stdout,
	}); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("commit_phase: %w", err)}
	}
	return statemachine.Outcome{}
}

// askCanICommit is the explicit "ask before every commit" gate from user
// memory. A `no` answer halts the run; the user can resume from the same
// node after fixing whatever they found wrong.
func (a actions) askCanICommit(ctx *statemachine.Context) statemachine.Outcome {
	ans, err := a.deps.Prompter.Ask("Can I commit? [y/N]: ")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("ask_can_i_commit: %w", err)}
	}
	yes, ok := parseYesNo(ans)
	if !ok || !yes {
		return statemachine.Outcome{Err: errors.New("ask_can_i_commit: user declined commit; halting run")}
	}
	return statemachine.Outcome{}
}

// ---------------------------------------------------------------------------
// Test-mode actions
// ---------------------------------------------------------------------------

// compileInScope runs the canonical compile sweep. v1 calls compile-all.sh
// from the repo root unconditionally; per-language scoping is a v2 nicety
// (would require knowing the in-scope languages, which the structural cycle
// does not yet expose).
func (a actions) compileInScope(ctx *statemachine.Context) statemachine.Outcome {
	cmdLine := "./compile-all.sh"
	out, err := a.deps.Shell.Run(context.Background(), cmdLine)
	if err != nil {
		fmt.Fprintln(a.deps.Stderr, string(out))
		return statemachine.Outcome{Err: fmt.Errorf("compile_in_scope: %w", err)}
	}
	if len(out) > 0 {
		fmt.Fprintln(a.deps.Stdout, string(out))
	}
	return statemachine.Outcome{}
}

// runSampleSuite runs the canonical sample-suite sweep. v1 calls test-all.sh
// with a --sample flag (matching the README's documented sample run).
func (a actions) runSampleSuite(ctx *statemachine.Context) statemachine.Outcome {
	cmdLine := "./test-all.sh --sample"
	out, err := a.deps.Shell.Run(context.Background(), cmdLine)
	if err != nil {
		fmt.Fprintln(a.deps.Stderr, string(out))
		return statemachine.Outcome{Err: fmt.Errorf("run_sample_suite: %w", err)}
	}
	if len(out) > 0 {
		fmt.Fprintln(a.deps.Stdout, string(out))
	}
	return statemachine.Outcome{}
}

// printDriftWarning emits a one-line reminder when only the compile sweep
// ran (no sample tests). The warning text is informational; the engine
// keeps moving regardless.
func (a actions) printDriftWarning(ctx *statemachine.Context) statemachine.Outcome {
	mode := ctx.GetString("structural_test_mode")
	if mode == "compile" {
		fmt.Fprintln(a.deps.Stderr,
			"DRIFT WARNING: compile-only TEST mode skipped sample suites — run `./test-all.sh --sample` before merging.")
	}
	return statemachine.Outcome{}
}

// ---------------------------------------------------------------------------
// Checklist
// ---------------------------------------------------------------------------

// tickChecklist updates the GitHub issue body to mark every `- [ ]` item as
// `- [x]`. It is a soft action: a missing issue body or a no-op body is
// fine. Errors from `gh issue edit` halt the run because they indicate a
// permission or auth problem.
func (a actions) tickChecklist(ctx *statemachine.Context) statemachine.Outcome {
	if err := a.tickRemoteChecklist(ctx); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("tick_checklist: %w", err)}
	}
	return statemachine.Outcome{}
}

// tickRemoteChecklist is the shared helper used by both tick_checklist and
// move_to_in_acceptance — both want every checkbox ticked when the cycle
// completes. Splitting it out lets move_to_in_acceptance call it inline
// without dispatching the action twice.
func (a actions) tickRemoteChecklist(ctx *statemachine.Context) error {
	issueNum, err := strconv.Atoi(ctx.GetString("issue_num"))
	if err != nil || issueNum <= 0 {
		// Nothing to tick — happens in transitions tests and in dry-runs.
		return nil
	}
	args := []string{"issue", "view", strconv.Itoa(issueNum), "--json", "body"}
	if repo := ctx.GetString("issue_repo"); repo != "" {
		args = append(args, "--repo", repo)
	}
	out, err := a.deps.Gh.Run(context.Background(), args...)
	if err != nil {
		return fmt.Errorf("gh issue view: %w", err)
	}
	body := extractIssueBody(out)
	updated := tickAllCheckboxes(body)
	if updated == body {
		return nil // no `- [ ]` items, nothing to do
	}
	editArgs := []string{"issue", "edit", strconv.Itoa(issueNum), "--body", updated}
	if repo := ctx.GetString("issue_repo"); repo != "" {
		editArgs = append(editArgs, "--repo", repo)
	}
	if _, err := a.deps.Gh.Run(context.Background(), editArgs...); err != nil {
		return fmt.Errorf("gh issue edit: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// confirmer adapts the actions.Prompter into a release.Confirmer. The
// release package owns the explicit "ask before every commit" gate and
// requires a Confirmer at the type level; we route every commit through
// the same prompter that owns the rest of the actions' I/O.
func (a actions) confirmer() release.Confirmer {
	return func(prompt string) (bool, error) {
		ans, err := a.deps.Prompter.Ask(prompt)
		if err != nil {
			return false, err
		}
		yes, _ := parseYesNo(ans)
		return yes, nil
	}
}

// parseYesNo is a tiny duplicate of the gates helper. We do not import gates
// (would create a cycle of registries) — the cost of a five-line copy is
// negligible.
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

// extractIssueSection finds an H2-or-deeper markdown heading whose text
// matches `heading` (case-insensitive, exact match after dropping leading
// hashes). Returns the section's contents — every line after the heading
// up to (but not including) the next heading at the same depth or
// shallower, with surrounding blank lines trimmed. Returns ok=false when
// the heading is absent.
func extractIssueSection(body, heading string) (string, bool) {
	lines := strings.Split(body, "\n")
	startIdx := -1
	startDepth := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		depth := 0
		for depth < len(trimmed) && trimmed[depth] == '#' {
			depth++
		}
		if depth < 2 {
			continue
		}
		text := strings.TrimSpace(trimmed[depth:])
		if strings.EqualFold(text, heading) {
			startIdx = i + 1
			startDepth = depth
			break
		}
	}
	if startIdx < 0 {
		return "", false
	}
	endIdx := len(lines)
	for i := startIdx; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		depth := 0
		for depth < len(trimmed) && trimmed[depth] == '#' {
			depth++
		}
		if depth <= startDepth {
			endIdx = i
			break
		}
	}
	section := strings.Trim(strings.Join(lines[startIdx:endIdx], "\n"), "\n")
	if section == "" {
		return "", false
	}
	return section, true
}

// extractIssueBody pulls .body out of `gh issue view --json body`. Same
// minimal parser as gates.extractIssueBody — duplicated to keep the
// packages independent.
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

// tickAllCheckboxes returns a copy of body with every `- [ ]` rewritten as
// `- [x]`. Casing of the bracket content is preserved: only an empty box
// (or a box with a single space) is considered unchecked. Already-ticked
// items pass through untouched so the operation is idempotent.
func tickAllCheckboxes(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		indent := line[:len(line)-len(trimmed)]
		if !strings.HasPrefix(trimmed, "- [ ]") && !strings.HasPrefix(trimmed, "* [ ]") {
			continue
		}
		// Replace the first `[ ]` with `[x]`, keeping marker and indent.
		updated := strings.Replace(trimmed, "[ ]", "[x]", 1)
		lines[i] = indent + updated
	}
	return strings.Join(lines, "\n")
}

// lookupStatusOption is a smaller, action-local cousin of board's
// lookupStatusField — it accepts an arbitrary option name (e.g. "In
// acceptance") instead of being hard-coded to "In progress". Returns the
// Status field ID and the requested option ID.
func lookupStatusOption(ctx context.Context, gh GhRunner, sCtx *statemachine.Context, optionName string) (fieldID, optionID string, err error) {
	owner, number, err := projectOwnerAndNumber(sCtx)
	if err != nil {
		return "", "", err
	}
	out, err := gh.Run(ctx, "project", "field-list", strconv.Itoa(number), "--owner", owner, "--format", "json")
	if err != nil {
		return "", "", fmt.Errorf("gh project field-list: %w", err)
	}
	field, optionID, err := findStatusOption(out, optionName)
	if err != nil {
		return "", "", err
	}
	return field, optionID, nil
}

// findStatusOption parses field-list JSON and returns (statusFieldID,
// matching optionID). Hand-decoded to avoid importing encoding/json into
// every action — keeps the file small. The shape is well-known.
func findStatusOption(raw []byte, optionName string) (string, string, error) {
	// Find the Status field object.
	statusBlock, ok := findFieldBlock(raw, "Status")
	if !ok {
		return "", "", fmt.Errorf("project has no Status field")
	}
	id, ok := jsonFieldString(statusBlock, "id")
	if !ok {
		return "", "", fmt.Errorf("Status field is missing id")
	}
	// Walk the options array within the Status block.
	options, ok := jsonFieldRaw(statusBlock, "options")
	if !ok {
		return "", "", fmt.Errorf("Status field has no options array")
	}
	target := strings.ToLower(strings.TrimSpace(optionName))
	for _, opt := range splitJSONArray(options) {
		name, _ := jsonFieldString(opt, "name")
		if strings.ToLower(strings.TrimSpace(name)) == target {
			optID, _ := jsonFieldString(opt, "id")
			return id, optID, nil
		}
	}
	return "", "", fmt.Errorf("Status field has no %q option", optionName)
}

// projectOwnerAndNumber resolves the project owner + number from Context.
// Prefers an explicit project_url; otherwise reconstructs from issue_repo
// when it follows the canonical owner/repo form (delegating to gh project
// list via the board package would be cleaner — Session 3 wires that up).
func projectOwnerAndNumber(ctx *statemachine.Context) (string, int, error) {
	if u := ctx.GetString("project_url"); u != "" {
		return parseProjectURL(u)
	}
	return "", 0, fmt.Errorf("project_url not in Context")
}

func parseProjectURL(url string) (string, int, error) {
	const orgPrefix = "https://github.com/orgs/"
	const userPrefix = "https://github.com/users/"
	var rest string
	switch {
	case strings.HasPrefix(url, orgPrefix):
		rest = url[len(orgPrefix):]
	case strings.HasPrefix(url, userPrefix):
		rest = url[len(userPrefix):]
	default:
		return "", 0, fmt.Errorf("not a canonical project URL: %q", url)
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 3 || parts[1] != "projects" {
		return "", 0, fmt.Errorf("not a canonical project URL: %q", url)
	}
	n, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", 0, fmt.Errorf("project number: %w", err)
	}
	return parts[0], n, nil
}

// findFieldBlock returns the JSON object slice for the field with the given
// name. It performs a permissive scan — splits the top-level "fields"
// array, then matches each object's "name". Sufficient for the well-known
// `gh project field-list` shape.
func findFieldBlock(raw []byte, fieldName string) ([]byte, bool) {
	fields, ok := jsonFieldRaw(raw, "fields")
	if !ok {
		return nil, false
	}
	target := strings.ToLower(strings.TrimSpace(fieldName))
	for _, obj := range splitJSONArray(fields) {
		name, _ := jsonFieldString(obj, "name")
		if strings.ToLower(strings.TrimSpace(name)) == target {
			return obj, true
		}
	}
	return nil, false
}

// jsonFieldRaw returns the raw bytes of a top-level JSON field's value.
// Handles strings, objects, and arrays via brace/bracket counting. Stops
// short of full JSON parsing — sufficient for our two-level use.
func jsonFieldRaw(raw []byte, key string) ([]byte, bool) {
	needle := []byte(`"` + key + `"`)
	idx := bytes.Index(raw, needle)
	if idx < 0 {
		return nil, false
	}
	rest := raw[idx+len(needle):]
	colon := bytes.IndexByte(rest, ':')
	if colon < 0 {
		return nil, false
	}
	rest = bytes.TrimLeft(rest[colon+1:], " \t\r\n")
	if len(rest) == 0 {
		return nil, false
	}
	switch rest[0] {
	case '"':
		// Skip the value; not-an-object-or-array but useful to recognise.
		end := matchString(rest)
		if end < 0 {
			return nil, false
		}
		return rest[:end+1], true
	case '{':
		end := matchBraced(rest, '{', '}')
		if end < 0 {
			return nil, false
		}
		return rest[:end+1], true
	case '[':
		end := matchBraced(rest, '[', ']')
		if end < 0 {
			return nil, false
		}
		return rest[:end+1], true
	default:
		// Bare token (number, true, false, null) — read until comma/brace.
		end := bytes.IndexAny(rest, ",}]")
		if end < 0 {
			return rest, true
		}
		return rest[:end], true
	}
}

func jsonFieldString(raw []byte, key string) (string, bool) {
	val, ok := jsonFieldRaw(raw, key)
	if !ok {
		return "", false
	}
	if len(val) < 2 || val[0] != '"' || val[len(val)-1] != '"' {
		return "", false
	}
	return string(val[1 : len(val)-1]), true
}

func matchString(s []byte) int {
	for i := 1; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			continue
		}
		if s[i] == '"' {
			return i
		}
	}
	return -1
}

func matchBraced(s []byte, open, close byte) int {
	depth := 0
	inStr := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitJSONArray(arr []byte) [][]byte {
	if len(arr) < 2 || arr[0] != '[' || arr[len(arr)-1] != ']' {
		return nil
	}
	body := arr[1 : len(arr)-1]
	var out [][]byte
	depth := 0
	inStr := false
	start := 0
	for i := 0; i < len(body); i++ {
		c := body[i]
		if inStr {
			if c == '\\' && i+1 < len(body) {
				i++
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, bytes.TrimSpace(body[start:i]))
				start = i + 1
			}
		}
	}
	tail := bytes.TrimSpace(body[start:])
	if len(tail) > 0 {
		out = append(out, tail)
	}
	return out
}

// ---------------------------------------------------------------------------
// Adapter shims (different runner interfaces across packages must not leak)
// ---------------------------------------------------------------------------

// ghAdapter / ghClassifyAdapter / gitReleaseAdapter exist because each
// underlying package (board, classify, release) defines its own GhRunner
// / GitRunner interface — Go's structural typing means we can wrap once
// instead of teaching every package to depend on a shared runner type.
// The wrappers are zero-cost.
type ghAdapter struct{ inner GhRunner }

func (g ghAdapter) Run(ctx context.Context, args ...string) ([]byte, error) {
	return g.inner.Run(ctx, args...)
}

type ghClassifyAdapter struct{ inner GhRunner }

func (g ghClassifyAdapter) Run(ctx context.Context, args ...string) ([]byte, error) {
	return g.inner.Run(ctx, args...)
}

type gitReleaseAdapter struct{ inner GitRunner }

func (g gitReleaseAdapter) Run(ctx context.Context, args ...string) ([]byte, error) {
	return g.inner.Run(ctx, args...)
}

// ---------------------------------------------------------------------------
// Default exec runners + stdin prompter
// ---------------------------------------------------------------------------

type realGh struct{}

func (realGh) Run(ctx context.Context, args ...string) ([]byte, error) {
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

type realGit struct{}

func (realGit) Run(ctx context.Context, args ...string) ([]byte, error) {
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

type realShell struct{}

func (realShell) Run(ctx context.Context, commandLine string) ([]byte, error) {
	// We deliberately route through the user's shell so command lines like
	// `./test-all.sh --sample` and `bash -lc compile-all.sh` work uniformly.
	// On Windows, gh-optivem ships against bash via the Git Bash shim; if
	// that is missing the user gets a clear "executable file not found"
	// from os/exec.
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "bash"
	}
	cmd := exec.CommandContext(ctx, shell, "-c", commandLine)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("shell %q: %w (stderr: %s)",
				commandLine, err, strings.TrimSpace(stderr.String()))
		}
		return out, fmt.Errorf("shell %q: %w", commandLine, err)
	}
	return out, nil
}

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
