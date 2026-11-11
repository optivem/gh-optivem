package process_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/process"
	"github.com/optivem/gh-optivem/internal/atdd/process/actions"
	"github.com/optivem/gh-optivem/internal/atdd/process/gates"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// issuePrompter answers HUMAN_REVIEW's per-issue prompts: round r (1-based)
// gets issues[r-1] then an empty line; rounds past the list get only the
// empty line.
type issuePrompter struct {
	issues [][]string
	round  int
	next   int
}

func (p *issuePrompter) Ask(prompt string) (string, error) {
	if strings.HasPrefix(prompt, "Issue 1 ") {
		p.round++
		p.next = 0
	}
	if p.round <= len(p.issues) && p.next < len(p.issues[p.round-1]) {
		a := p.issues[p.round-1][p.next]
		p.next++
		return a + "\n", nil
	}
	return "\n", nil
}

// codeReviewScenario is one synthetic Code & Architecture Review.
type codeReviewScenario struct {
	systemPlan   string     // reviewer's system-refactoring-plan ("" = omitted)
	testPlan     string     // reviewer's test-refactoring-plan ("" = omitted)
	rejectPlan   bool       // the human declines APPROVE_PLAN
	humanIssues  [][]string // issues per HUMAN_REVIEW round
	noVerdictKey bool       // reviewer omits refactoring-needed
	useCaseIDs   []string   // reviewer's refactor-use-case-ids (nil = omitted)
	merged       string     // the story's merged-criteria in State ("" = none)
	process      string     // process to run ("" = review-code-and-architecture)
}

// codeReviewRun is the observable trail of one walk.
type codeReviewRun struct {
	agents        []string          // agent dispatches, in order
	checklists    map[string]string // checklist param seen per refactorer
	issuesSeen    []string          // human-review-issues at each updater dispatch
	planQuestion  string            // APPROVE_PLAN's question
	commits       []string          // commit layers, in order
	fullSuiteRuns int               // verify runs with no suite/test/use-case filter
	useCaseRuns   []string          // --use-case selection of each verify run ("" = whole suite)
	err           error
}

// runCodeReview drives review-code-and-architecture with synthetic agents.
// The review's own actions and gates, and the approval / reviewer gates of
// execute-agent, are the real implementations, so the test pins the YAML
// wiring and the Go code together.
//
// Loop-hazard audit: the reachable back-edges are execute-agent FIX ->
// RUN_AGENT (never taken: validation passes), REQUEST_REVISION -> RUN_AGENT
// (never taken: no reviewer configured), execute-command FIX -> RUN_COMMAND
// (never taken: every command succeeds), verify-tests-pass
// GATE_FIX_FLOW_APPROVED -> RUN_TESTS (never taken: tests pass), and
// COMMIT_GUIDELINES -> HUMAN_REVIEW (bounded by max-visits 3 on
// UPDATE_GUIDELINES, then GUIDELINES_LOOP_EXHAUSTED). The walk is bounded for
// any scenario.
func runCodeReview(t *testing.T, sc codeReviewScenario) codeReviewRun {
	t.Helper()
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	prompter := &issuePrompter{issues: sc.humanIssues}
	realActions := actions.New()
	actions.RegisterAll(realActions, actions.Deps{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Prompter: prompter})
	realGates := gates.New()
	gates.RegisterAll(realGates, gates.Deps{})

	run := codeReviewRun{checklists: map[string]string{}}
	eng.ActionFn = func(name string) statemachine.NodeFn {
		switch name {
		case "reset-code-review", "summarize-refactoring-plan", "record-human-review-issues", "resolve-test-use-cases":
			return realActions.Lookup(name)
		}
		return func(ctx *statemachine.Context) statemachine.Outcome {
			switch name {
			case "validate-outputs-and-scopes":
				ctx.Set("outputs-and-scopes-valid", true)
			case "resolve-channel":
				ctx.Set("channel-touched", true)
			case "resolve-inner-loop":
				ctx.Set("inner-loop-mode", "none")
			case "run-command":
				ctx.Set("command-succeeded", true)
				cmd := ctx.Params["command"]
				if strings.HasPrefix(cmd, "gh optivem system-test run") {
					ctx.Set("test-outcome", "pass")
					useCases := ctx.Params[actions.ParamTestUseCases]
					run.useCaseRuns = append(run.useCaseRuns, useCases)
					if ctx.Params["suite"] == "" && ctx.Params["test-names"] == "" && useCases == "" {
						run.fullSuiteRuns++
					}
				}
				if strings.HasPrefix(cmd, "gh optivem commit") {
					run.commits = append(run.commits, ctx.Params["layer"])
				}
			}
			return statemachine.Outcome{}
		}
	}
	eng.AgentFn = func(name string) statemachine.NodeFn {
		n := name
		return func(ctx *statemachine.Context) statemachine.Outcome {
			if n != "human" {
				run.agents = append(run.agents, n)
			}
			switch n {
			case "code-architecture-reviewer":
				needed := sc.systemPlan != "" || sc.testPlan != ""
				if !sc.noVerdictKey {
					ctx.Set("refactoring-needed", needed)
				}
				ctx.Set("findings", "OrderService.place does validation and persistence")
				if sc.systemPlan != "" {
					ctx.Set("system-refactoring-plan", sc.systemPlan)
				}
				if sc.testPlan != "" {
					ctx.Set("test-refactoring-plan", sc.testPlan)
				}
				if sc.useCaseIDs != nil {
					ctx.Set("refactor-use-case-ids", sc.useCaseIDs)
				}
			case "system-refactorer", "test-refactorer":
				run.checklists[n] = ctx.Params["checklist"]
			case "guidelines-updater":
				run.issuesSeen = append(run.issuesSeen, ctx.Params["human-review-issues"])
			case "human":
				q := ctx.Params["question"]
				outcome := "approved"
				if strings.HasPrefix(q, "Approve this refactoring plan?") {
					run.planQuestion = q
					if sc.rejectPlan {
						outcome = "rejected"
					}
				}
				ctx.Set("approval-outcome", outcome)
			}
			return statemachine.Outcome{}
		}
	}
	eng.GateFn = func(name string) statemachine.NodeFn {
		switch name {
		case "approval-outcome", "return-on-post-rejection", "reviewer-configured",
			"refactoring-needed", "system-refactoring-plan-present",
			"test-refactoring-plan-present", "human-review-issues-found":
			return realGates.Lookup(name)
		default:
			return capLoopGateFn()(name)
		}
	}
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	ctx := statemachine.NewContext()
	// The commit message names the run's ticket.
	ctx.Set("ticket-id", "7")
	ctx.Set("issue-title", "Code review")
	if sc.merged != "" {
		ctx.Set(actions.CtxKeyMergedCriteria, sc.merged)
	}
	process := sc.process
	if process == "" {
		process = "review-code-and-architecture"
	}
	run.err = eng.RunProcess(process, ctx)
	return run
}

func countAgent(agents []string, name string) int {
	n := 0
	for _, a := range agents {
		if a == name {
			n++
		}
	}
	return n
}

// TestCodeReview_ApprovedPlanRunsBothRefactorers: the approved plan's halves
// reach the two refactorers as their checklist, each half runs the full suite
// and is committed, and an empty human review ends the loop.
func TestCodeReview_ApprovedPlanRunsBothRefactorers(t *testing.T) {
	run := runCodeReview(t, codeReviewScenario{
		systemPlan: "OrderService.java: extract OrderValidator",
		testPlan:   "OrderTest.java: inline the unused helper",
	})
	if run.err != nil {
		t.Fatalf("RunProcess: %v", run.err)
	}
	want := []string{"code-architecture-reviewer", "system-refactorer", "test-refactorer"}
	if strings.Join(run.agents, ",") != strings.Join(want, ",") {
		t.Errorf("agents = %v, want %v", run.agents, want)
	}
	if run.checklists["system-refactorer"] != "OrderService.java: extract OrderValidator" {
		t.Errorf("system-refactorer checklist = %q", run.checklists["system-refactorer"])
	}
	if run.checklists["test-refactorer"] != "OrderTest.java: inline the unused helper" {
		t.Errorf("test-refactorer checklist = %q", run.checklists["test-refactorer"])
	}
	for _, s := range []string{"Findings:", "OrderService.place does validation", "System refactoring plan:", "extract OrderValidator", "Test refactoring plan:", "inline the unused helper"} {
		if !strings.Contains(run.planQuestion, s) {
			t.Errorf("APPROVE_PLAN question missing %q:\n%s", s, run.planQuestion)
		}
	}
	if strings.Join(run.commits, ",") != "SYSTEM,TESTS" {
		t.Errorf("commits = %v, want [SYSTEM TESTS]", run.commits)
	}
	if run.fullSuiteRuns != 2 {
		t.Errorf("full-suite runs = %d, want 2", run.fullSuiteRuns)
	}
}

// TestCodeReview_PlanUseCasesNarrowEveryVerifyRun: the use cases the review
// plan names (criterion ids reduced to their use case) narrow both refactor
// halves to those use cases' tests and override the story's use cases. The
// guideline round stays on the whole suite (the updater fixes other
// occurrences, which can land in any use case).
func TestCodeReview_PlanUseCasesNarrowEveryVerifyRun(t *testing.T) {
	run := runCodeReview(t, codeReviewScenario{
		systemPlan:  "OrderService.java: extract OrderValidator",
		testPlan:    "OrderTest.java: inline the unused helper",
		useCaseIDs:  []string{"UC-003.2", "UC-001"},
		merged:      "UC-009.1",
		humanIssues: [][]string{{"OrderService.java: magic number"}},
	})
	if run.err != nil {
		t.Fatalf("RunProcess: %v", run.err)
	}
	want := "UC-003,UC-001|UC-003,UC-001|"
	if got := strings.Join(run.useCaseRuns, "|"); got != want {
		t.Errorf("verify-run use cases = %q, want %q (system half, test half, guideline round)", got, want)
	}
	if !strings.Contains(run.planQuestion, "Verified on: the tests of UC-003, UC-001") {
		t.Errorf("APPROVE_PLAN question does not name the verified use cases:\n%s", run.planQuestion)
	}
}

// TestCodeReview_PlanWithoutUseCasesRunsWholeSuiteEvenInAStory: a review plan
// naming no use case (cross-cutting) runs the whole suite, even when the
// story in flight has use cases.
func TestCodeReview_PlanWithoutUseCasesRunsWholeSuiteEvenInAStory(t *testing.T) {
	run := runCodeReview(t, codeReviewScenario{
		systemPlan:  "OrderService.java: extract OrderValidator",
		merged:      "UC-009.1",
		humanIssues: [][]string{{"OrderService.java: magic number"}},
	})
	if run.err != nil {
		t.Fatalf("RunProcess: %v", run.err)
	}
	if got := strings.Join(run.useCaseRuns, "|"); got != "|" || run.fullSuiteRuns != 2 {
		t.Errorf("verify-run use cases = %q, full-suite runs = %d; want two whole-suite runs", got, run.fullSuiteRuns)
	}
}

// TestCodeReview_SystemOnlyPlanSkipsTestRefactorer: a plan with only a
// system half never dispatches the test-refactorer.
func TestCodeReview_SystemOnlyPlanSkipsTestRefactorer(t *testing.T) {
	run := runCodeReview(t, codeReviewScenario{systemPlan: "OrderService.java: rename calc to calculateTotal"})
	if run.err != nil {
		t.Fatalf("RunProcess: %v", run.err)
	}
	if countAgent(run.agents, "test-refactorer") != 0 || countAgent(run.agents, "system-refactorer") != 1 {
		t.Errorf("agents = %v, want one system-refactorer and no test-refactorer", run.agents)
	}
}

// TestCodeReview_DeclinedPlanChangesNothing: a "no" at APPROVE_PLAN ends the
// review without running a refactorer, a commit or the human review.
func TestCodeReview_DeclinedPlanChangesNothing(t *testing.T) {
	run := runCodeReview(t, codeReviewScenario{systemPlan: "OrderService.java: extract OrderValidator", rejectPlan: true})
	if run.err != nil {
		t.Fatalf("RunProcess: %v", run.err)
	}
	if len(run.agents) != 1 || len(run.commits) != 0 {
		t.Errorf("agents = %v, commits = %v; want only the reviewer and no commit", run.agents, run.commits)
	}
}

// TestCodeReview_NoPlanGoesToHumanReview: refactoring-needed=false skips
// APPROVE_PLAN and the refactorers, but the human still reviews; an issue
// they find goes to the guidelines-updater, is verified on the full suite and
// committed, and the next empty review ends the loop.
func TestCodeReview_NoPlanGoesToHumanReview(t *testing.T) {
	run := runCodeReview(t, codeReviewScenario{
		humanIssues: [][]string{{"OrderController.java: business rule in the controller", "PriceCalc.java: magic number 0.2"}},
	})
	if run.err != nil {
		t.Fatalf("RunProcess: %v", run.err)
	}
	if run.planQuestion != "" {
		t.Errorf("APPROVE_PLAN asked without a plan: %q", run.planQuestion)
	}
	want := []string{"code-architecture-reviewer", "guidelines-updater"}
	if strings.Join(run.agents, ",") != strings.Join(want, ",") {
		t.Errorf("agents = %v, want %v", run.agents, want)
	}
	if len(run.issuesSeen) != 1 || run.issuesSeen[0] != "OrderController.java: business rule in the controller\nPriceCalc.java: magic number 0.2" {
		t.Errorf("updater saw issues %q, want both, one per line", run.issuesSeen)
	}
	if strings.Join(run.commits, ",") != "GUIDELINES" || run.fullSuiteRuns != 1 {
		t.Errorf("commits = %v, full-suite runs = %d; want [GUIDELINES] and 1", run.commits, run.fullSuiteRuns)
	}
}

// TestCodeReview_GuidelinesLoopCapHalts: a human who keeps finding issues
// gets exactly 3 guideline rounds, each committed, then the run halts at
// GUIDELINES_LOOP_EXHAUSTED.
func TestCodeReview_GuidelinesLoopCapHalts(t *testing.T) {
	issues := [][]string{{"a"}, {"b"}, {"c"}, {"d"}, {"e"}}
	run := runCodeReview(t, codeReviewScenario{humanIssues: issues})
	if run.err == nil || !strings.Contains(run.err.Error(), "GUIDELINES_LOOP_EXHAUSTED") {
		t.Fatalf("RunProcess err = %v, want halt at GUIDELINES_LOOP_EXHAUSTED", run.err)
	}
	if countAgent(run.agents, "guidelines-updater") != 3 || len(run.commits) != 3 {
		t.Errorf("updater=%d commits=%d, want 3/3", countAgent(run.agents, "guidelines-updater"), len(run.commits))
	}
}

// TestCodeReview_MissingVerdictFails: a reviewer that omits the required
// refactoring-needed verdict fails the run instead of reading as "nothing to
// refactor".
func TestCodeReview_MissingVerdictFails(t *testing.T) {
	run := runCodeReview(t, codeReviewScenario{noVerdictKey: true})
	if run.err == nil || !strings.Contains(run.err.Error(), "refactoring-needed") {
		t.Fatalf("RunProcess err = %v, want a refactoring-needed error", run.err)
	}
}

// TestCodeReview_VerdictPlanMismatchFails: refactoring-needed=true with no
// plan fails at SUMMARIZE_REFACTORING_PLAN rather than approving nothing.
func TestCodeReview_VerdictPlanMismatchFails(t *testing.T) {
	a := actions.New()
	actions.RegisterAll(a, actions.Deps{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Set("refactoring-needed", true)
	out := a.Lookup("summarize-refactoring-plan")(ctx)
	if out.Err == nil || !strings.Contains(out.Err.Error(), "neither") {
		t.Fatalf("Err = %v, want the verdict/plan mismatch error", out.Err)
	}
}

// TestRefactorCycles_StoryUseCasesNarrowTheVerifyRun pins the story/menu
// path of plan 20261111-1023: entered outside the review, the refactor cycles
// verify on the story's use cases (merged-criteria reduced to use cases), and
// on the whole suite when there is no story (bug, refactor ticket, ad hoc).
func TestRefactorCycles_StoryUseCasesNarrowTheVerifyRun(t *testing.T) {
	for _, process := range []string{"refactor-system-structure", "refactor-test-structure"} {
		for _, tc := range []struct {
			merged string
			want   string
		}{
			{merged: "UC-001.1, UC-001.2, UC-004.3", want: "UC-001,UC-004"},
			{merged: "", want: ""},
		} {
			run := runCodeReview(t, codeReviewScenario{process: process, merged: tc.merged})
			if run.err != nil {
				t.Fatalf("%s (merged=%q): RunProcess: %v", process, tc.merged, run.err)
			}
			if len(run.useCaseRuns) != 1 || run.useCaseRuns[0] != tc.want {
				t.Errorf("%s (merged=%q): verify-run use cases = %q, want one run on %q", process, tc.merged, run.useCaseRuns, tc.want)
			}
		}
	}
}
