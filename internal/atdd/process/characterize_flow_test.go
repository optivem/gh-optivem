package process_test

// Scripted walks of the characterize (legacy bulk baseline) flow, plan
// 20261110-1130 Step 4. The real process-flow is loaded and unrolled as the
// driver unrolls it (one channel, one external system); every gate is the real
// binding; actions and agents are scripted.
//
// Loop-hazard audit (feedback_statemachine_test_loop_hazard): the reachable
// loops are the use case loop (CHARACTERIZE_USE_CASE -> NEXT_USE_CASE, ends
// when the scripted cursor passes the last use case, exactly like the real
// action; a scripted deferral writes a new order at most once per use case),
// the diagram redo and the narrative / criteria / tests redos (max-visits 3
// each), whose scripted approvals always end in "approved" or hit the cap, and
// the verify fix loops (max-visits 2), whose scripted test outcomes converge
// to the expected result on the second run. Every validate passes and every command
// succeeds, so the execute-agent / execute-command FIX loops are never taken.
// As a last guard, the harness fails the walk after charMaxEvents events.

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/process"
	"github.com/optivem/gh-optivem/internal/atdd/process/gates"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

const charMaxEvents = 5000

// charUseCase is one use case's scripted behaviour.
type charUseCase struct {
	escc      bool // the criteria file declares ESCC (ticket-has-escc)
	dslPort   bool // the AT writer adds DSL port methods (dsl-port-changed)
	sysPort   bool // the DSL implementer changes system driver ports
	extPort   bool // the DSL implementer changes external driver ports (late discovery)
	ctDSLPort bool // the contract test writer changes DSL ports
	// Rejections per gated step ("narrative", "criteria", "tests").
	rejects map[string]int
	// The narrative finds these not-done dependencies (the first reorder
	// defers the use case behind them; one-shot).
	dependsOn []string
}

// charScenario scripts one characterize walk.
type charScenario struct {
	proposed string   // the diagram writer's proposed use case
	pilot    []string // order-pilot-use-cases' use-case-order
	scaling  []string // order-scaling-use-cases' use-case-order
	useCases map[string]*charUseCase
	realKind string // baked real-kind of the one external system "erp"
	// Queued outcomes per contract suite; an empty queue reads "pass".
	contractOutcomes map[string][]string
	// Nth (1-based) check-system-unchanged / check-no-assertion-removed
	// that reports a violation; 0 = never.
	systemChangedAt    int
	assertionRemovedAt int
	// Rejections of CONFIRM_USE_CASE_DIAGRAM before approval.
	rejectDiagram int
	// The characterization fixer edits a stand-in.
	fixerTouchesStandIns bool
}

// charRun is the observable trail of one walk.
type charRun struct {
	events []string // "action:<n>", "agent:<n>", "cmd:<task-name>", "commit:<layer>", "approve:<question>"
	// commands is every run-command line, in order.
	commands []string
	// agentParams is the ctx.Params each agent dispatch saw, in dispatch order.
	agentParams []agentDispatch
	// testRuns is every `system-test run`: suite and test-names.
	testRuns []testRun
	// wipNames is the test-names each remove-wip-gate saw.
	wipNames []string
	err      error
}

type agentDispatch struct {
	name     string
	params   map[string]string
	feedback string // State "feedback" at dispatch (the operator's redo feedback)
	useCase  string // State use-case-id at dispatch
}

type testRun struct{ suite, names string }

func (r *charRun) count(event string) int {
	n := 0
	for _, e := range r.events {
		if e == event {
			n++
		}
	}
	return n
}

func (r *charRun) countPrefix(prefix string) int {
	n := 0
	for _, e := range r.events {
		if strings.HasPrefix(e, prefix) {
			n++
		}
	}
	return n
}

func (r *charRun) index(event string) int { return slices.Index(r.events, event) }

func (r *charRun) commits() []string {
	var out []string
	for _, e := range r.events {
		if l, ok := strings.CutPrefix(e, "commit:"); ok {
			out = append(out, l)
		}
	}
	return out
}

// wantInOrder asserts that want is a subsequence of the trail.
func (r *charRun) wantInOrder(t *testing.T, want ...string) {
	t.Helper()
	i := 0
	for _, e := range r.events {
		if i < len(want) && e == want[i] {
			i++
		}
	}
	if i < len(want) {
		t.Errorf("event %q (step %d of %v) not found in order; trail:\n  %s", want[i], i+1, want, strings.Join(r.events, "\n  "))
	}
}

func newCharEngine(t *testing.T, realKind string) *statemachine.Engine {
	t.Helper()
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// As driver.unrollProjectTopology does for a one-channel project with one
	// external system.
	if err := eng.UnrollSystemChannels([]string{"api"}); err != nil {
		t.Fatalf("UnrollSystemChannels: %v", err)
	}
	if err := eng.UnrollSystemDriverAdapterChannels([]string{"api"}); err != nil {
		t.Fatalf("UnrollSystemDriverAdapterChannels: %v", err)
	}
	if err := eng.UnrollExternalSystems([]string{"erp"}, map[string]string{"erp": realKind}); err != nil {
		t.Fatalf("UnrollExternalSystems: %v", err)
	}
	return eng
}

// bindChar wires the scripted actions and agents and the real gates onto eng.
func bindChar(t *testing.T, eng *statemachine.Engine, sc *charScenario, run *charRun) {
	t.Helper()
	realGates := gates.New()
	gates.RegisterAll(realGates, gates.Deps{})

	cursors := map[string]int{}
	systemChecks, assertionChecks := 0, 0
	// The verify expectation of the last at-verify-expectation gate, consumed
	// by the next acceptance run (verify-tests-fail expects red).
	lastExpectation := ""

	record := func(e string) error {
		run.events = append(run.events, e)
		if len(run.events) > charMaxEvents {
			return fmt.Errorf("walk exceeded %d events (loop hazard); last %q", charMaxEvents, e)
		}
		return nil
	}
	uc := func(id string) *charUseCase {
		u, ok := sc.useCases[id]
		if !ok {
			u = &charUseCase{}
			sc.useCases[id] = u
		}
		return u
	}
	// stamp mimics select-next-use-case's per-use-case State re-derivation.
	stamp := func(ctx *statemachine.Context, id string) {
		u := uc(id)
		ctx.Set("use-case-id", id)
		ctx.Set("ticket-has-escc", u.escc)
		if u.escc {
			ctx.Set("escc-systems", []string{"erp"})
			ctx.Set("external-system-contract-criteria", "External System: erp ("+id+")")
		} else {
			ctx.Unset("external-system-contract-criteria")
		}
		ctx.Set("acceptance-criteria", "Scenario: "+id)
		ctx.Set("test-names", []string{"at_" + id})
		ctx.Set("dsl-port-changed", u.dslPort)
		ctx.Set("system-driver-port-changed", u.sysPort)
		ctx.Set("external-driver-port-changed", u.extPort)
	}

	eng.ActionFn = func(name string) statemachine.NodeFn {
		n := name
		return func(ctx *statemachine.Context) statemachine.Outcome {
			switch n {
			case "snapshot-working-tree", "validate-outputs-and-scopes", "summarize-review", "run-command":
				// Recorded below (or not at all: plumbing).
			default:
				if err := record("action:" + n); err != nil {
					return statemachine.Outcome{Err: err}
				}
			}
			switch n {
			case "record-start-commits", "delete-use-case-criteria",
				"validate-channels-registered", "validate-external-systems-registered",
				"snapshot-working-tree", "snapshot-stand-ins":
			case "remove-wip-gate":
				names, _ := ctx.Get("test-names").([]string)
				run.wipNames = append(run.wipNames, strings.Join(names, ","))
			case "order-pilot-use-cases":
				ctx.Set("use-case-order", strings.Join(sc.pilot, ", "))
				ctx.Set("pilot-use-cases", strings.Join(sc.pilot, ", "))
			case "order-scaling-use-cases":
				ctx.Set("use-case-order", strings.Join(sc.scaling, ", "))
			case "reorder-use-cases":
				// As the real action: deps first, then the current use case,
				// then the rest of the current order; a fresh cursor.
				cur := ctx.GetString("use-case-id")
				u := uc(cur)
				if len(u.dependsOn) == 0 {
					ctx.Set("use-case-deferred", false)
					break
				}
				order := append(slices.Clone(u.dependsOn), cur)
				ids := strings.FieldsFunc(ctx.GetString("use-case-order"), func(r rune) bool { return r == ',' || r == ' ' })
				for _, id := range ids[slices.Index(ids, cur)+1:] {
					if !slices.Contains(order, id) {
						order = append(order, id)
					}
				}
				u.dependsOn = nil
				delete(cursors, "characterize-use-case|"+strings.Join(order, ","))
				ctx.Set("use-case-order", strings.Join(order, ", "))
				ctx.Set("use-case-deferred", true)
			case "select-next-use-case":
				ids := strings.FieldsFunc(ctx.Params["use-cases"], func(r rune) bool { return r == ',' || r == ' ' })
				site := ctx.Params["layer-process"] + "|" + strings.Join(ids, ",")
				if cursors[site] >= len(ids) {
					ctx.Set("use-case-left", false)
					break
				}
				stamp(ctx, ids[cursors[site]])
				cursors[site]++
				ctx.Set("use-case-left", true)
			case "restamp-current-use-case":
				stamp(ctx, ctx.GetString("use-case-id"))
				ctx.Set("use-case-left", true)
			case "check-system-unchanged":
				systemChecks++
				ctx.Set("system-unchanged", systemChecks != sc.systemChangedAt)
			case "check-no-assertion-removed":
				assertionChecks++
				ctx.Set("no-assertion-removed", assertionChecks != sc.assertionRemovedAt)
			case "resolve-external-system":
				ctx.Set("external-system-touched", true)
				ctx.Set("real-kind", ctx.Params["real-kind"])
			case "resolve-channel":
				ctx.Set("channel-touched", true)
			case "validate-outputs-and-scopes":
				ctx.Set("outputs-and-scopes-valid", true)
			case "check-fix-progress":
				ctx.Set("fix-loop-progressing", true)
			case "check-stand-ins-changed":
				ctx.Set("stand-ins-changed", sc.fixerTouchesStandIns)
			case "summarize-review":
				ctx.Set("review-summary", "")
			case "run-command":
				cmd := ctx.Params["command"]
				run.commands = append(run.commands, cmd)
				ctx.Set("command-succeeded", true)
				switch {
				case strings.HasPrefix(cmd, "gh optivem commit"):
					if err := record("commit:" + ctx.Params["layer"]); err != nil {
						return statemachine.Outcome{Err: err}
					}
				default:
					if err := record("cmd:" + ctx.Params["task-name"]); err != nil {
						return statemachine.Outcome{Err: err}
					}
				}
				if strings.HasPrefix(cmd, "gh optivem system-test run") {
					suite := ctx.Params["suite"]
					run.testRuns = append(run.testRuns, testRun{suite: suite, names: ctx.Params["test-names"]})
					outcome := "pass"
					if q := sc.contractOutcomes[suite]; len(q) > 0 {
						outcome = q[0]
						sc.contractOutcomes[suite] = q[1:]
					} else if strings.HasPrefix(suite, "acceptance") && lastExpectation == "failure" {
						outcome = "fail"
					}
					lastExpectation = ""
					ctx.Set("test-outcome", outcome)
				}
			default:
				return statemachine.Outcome{Err: fmt.Errorf("unscripted action %q", n)}
			}
			return statemachine.Outcome{}
		}
	}

	eng.AgentFn = func(name string) statemachine.NodeFn {
		n := name
		return func(ctx *statemachine.Context) statemachine.Outcome {
			if n == "human" {
				q := ctx.Params["question"]
				outcome := "approved"
				step := ""
				switch {
				case strings.HasPrefix(q, "Approve the use case diagram"):
					if err := record("approve:diagram"); err != nil {
						return statemachine.Outcome{Err: err}
					}
					if sc.rejectDiagram > 0 {
						sc.rejectDiagram--
						outcome = "rejected"
					}
				case strings.HasPrefix(q, "Approve the narrative of use case "):
					step = "narrative"
				case strings.HasPrefix(q, "Approve the acceptance criteria of use case "):
					step = "criteria"
				case strings.HasPrefix(q, "Approve the acceptance tests of use case "):
					step = "tests"
				}
				if step != "" {
					id := ctx.GetString("use-case-id")
					if err := record("approve:" + step + ":" + id); err != nil {
						return statemachine.Outcome{Err: err}
					}
					if u := uc(id); u.rejects[step] > 0 {
						u.rejects[step]--
						outcome = "rejected"
					}
				}
				// As the driver: a redo gate's "no" stores the feedback, its
				// "yes" clears it.
				if ctx.Params["ask-for-changes"] == "true" {
					if outcome == "rejected" {
						ctx.Set("feedback", "redo "+q)
					} else {
						ctx.Unset("feedback")
					}
				}
				ctx.Set("approval-outcome", outcome)
				return statemachine.Outcome{}
			}
			if err := record("agent:" + n); err != nil {
				return statemachine.Outcome{Err: err}
			}
			run.agentParams = append(run.agentParams, agentDispatch{name: n, params: cloneParams(ctx.Params), feedback: ctx.GetString("feedback"), useCase: ctx.GetString("use-case-id")})
			id := ctx.GetString("use-case-id")
			switch n {
			case "legacy-use-case-diagram-writer":
				ctx.Set("proposed-use-case", sc.proposed)
				ctx.Set("proposed-use-case-reasons", "most used")
			case "legacy-use-case-criteria-writer":
				// The criteria writer adds the ESCC section for discovered systems.
				if ctx.Params["discovered-external-port-paths"] != "" {
					uc(ctx.Params["use-case-id"]).escc = true
				}
			case "acceptance-test-writer":
				ctx.Set("dsl-port-changed", uc(id).dslPort)
				ctx.Set("test-names", []string{"at_" + id})
			case "acceptance-test-reviewer", "dsl-reviewer", "system-driver-adapter-reviewer":
				ctx.Set("review-has-blocking-findings", false)
			case "dsl-implementer":
				if ctx.Params["suite"] == "acceptance" {
					u := uc(id)
					ctx.Set("system-driver-port-changed", u.sysPort)
					ctx.Set("external-driver-port-changed", u.extPort)
					if u.extPort {
						// validate-outputs-and-scopes preserves the paths.
						ctx.Set("external-driver-port-changed-paths", "driver/port/external/erp/ErpDriver.java")
						// The late discovery is one-shot: the use case now has its ESCC.
						u.extPort = false
					}
				} else {
					ctx.Set("system-driver-port-changed", false)
					ctx.Set("external-driver-port-changed", false)
				}
			case "contract-test-writer":
				ctx.Set("dsl-port-changed", uc(id).ctDSLPort)
				ctx.Set("test-names", []string{"ct_" + id})
			case "stub-fidelity-test-writer":
				ctx.Set("isolated-test-names", []string{})
			}
			return statemachine.Outcome{}
		}
	}

	eng.GateFn = func(name string) statemachine.NodeFn {
		fn := realGates.Lookup(name)
		if fn == nil {
			return nil
		}
		if name != "at-verify-expectation" {
			return fn
		}
		return func(ctx *statemachine.Context) statemachine.Outcome {
			out := fn(ctx)
			lastExpectation = out.Value
			return out
		}
	}
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
}

func cloneParams(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// runChar runs process name on a fresh, unrolled engine.
func runChar(t *testing.T, name string, sc *charScenario, setup func(ctx *statemachine.Context)) *charRun {
	t.Helper()
	if sc.useCases == nil {
		sc.useCases = map[string]*charUseCase{}
	}
	if sc.contractOutcomes == nil {
		sc.contractOutcomes = map[string][]string{}
	}
	if sc.realKind == "" {
		sc.realKind = "simulator"
	}
	eng := newCharEngine(t, sc.realKind)
	run := &charRun{}
	bindChar(t, eng, sc, run)
	ctx := statemachine.NewContext()
	ctx.Set("ticket-id", "1")
	ctx.Set("issue-title", "Characterize")
	if setup != nil {
		setup(ctx)
	}
	run.err = eng.RunProcess(name, ctx)
	return run
}

// happyScenario: the pilot use case calls the external system and changes the
// DSL and the system driver ports; the scaling use case changes only the DSL.
func happyScenario() *charScenario {
	return &charScenario{
		proposed: "UC-001",
		pilot:    []string{"UC-001"},
		scaling:  []string{"UC-002"},
		useCases: map[string]*charUseCase{
			"UC-001": {escc: true, dslPort: true, sysPort: true, ctDSLPort: true},
			"UC-002": {dslPort: true},
		},
	}
}

// 1. The characterize happy path, end to end: the pilot use case gated, the
// scaling one unattended, no go-ahead gate in between, each use case's WIP
// gate removal and criteria deletion committed when its tests are green, then
// the final safety gate and the system stop.
func TestCharacterize_HappyPath(t *testing.T) {
	run := runChar(t, "characterize", happyScenario(), nil)
	if run.err != nil {
		t.Fatalf("characterize: %v\ntrail:\n  %s", run.err, strings.Join(run.events, "\n  "))
	}

	run.wantInOrder(t,
		"action:record-start-commits",
		"cmd:start-system",
		"agent:legacy-use-case-diagram-writer",
		"commit:USE CASE DIAGRAM",
		"action:check-system-unchanged",
		"approve:diagram",
		"action:order-pilot-use-cases",
		// Pilot use case: each step, then its human gate.
		"agent:legacy-use-case-narrative-writer", "commit:USE CASE NARRATIVE (UC-001)",
		"action:check-system-unchanged", "approve:narrative:UC-001",
		"action:reorder-use-cases",
		"agent:legacy-use-case-criteria-writer", "commit:USE CASE CRITERIA (UC-001)",
		"action:check-system-unchanged", "approve:criteria:UC-001",
		"agent:contract-test-writer",
		"agent:acceptance-test-writer", "commit:ACCEPTANCE TESTS (UC-001)",
		"agent:dsl-implementer", "commit:DSL (UC-001)",
		"agent:system-driver-adapter-implementer", "commit:SYSTEM DRIVER ADAPTERS (UC-001) (api)",
		"approve:tests:UC-001",
		"action:remove-wip-gate", "commit:WIP GATE REMOVAL (UC-001)",
		"action:delete-use-case-criteria", "commit:CRITERIA DELETION (UC-001)",
		// Scaling, unattended.
		"action:order-scaling-use-cases",
		"commit:USE CASE NARRATIVE (UC-002)",
		"commit:USE CASE CRITERIA (UC-002)",
		"commit:ACCEPTANCE TESTS (UC-002)",
		"commit:DSL (UC-002)",
		"commit:WIP GATE REMOVAL (UC-002)",
		"commit:CRITERIA DELETION (UC-002)",
		// Tail.
		"action:check-system-unchanged",
		"action:check-no-assertion-removed",
		"cmd:stop-system",
	)
	if last := run.events[len(run.events)-1]; last != "cmd:stop-system" {
		t.Errorf("the run must end with the system stop; trail ends %q", last)
	}

	wantCommits := []string{
		"USE CASE DIAGRAM",
		"USE CASE NARRATIVE (UC-001)",
		"USE CASE CRITERIA (UC-001)",
		"DSL (external: erp)",
		"ACCEPTANCE TESTS (UC-001)",
		"DSL (UC-001)",
		"SYSTEM DRIVER ADAPTERS (UC-001) (api)",
		"WIP GATE REMOVAL (UC-001)",
		"CRITERIA DELETION (UC-001)",
		"USE CASE NARRATIVE (UC-002)",
		"USE CASE CRITERIA (UC-002)",
		"ACCEPTANCE TESTS (UC-002)",
		"DSL (UC-002)",
		"WIP GATE REMOVAL (UC-002)",
		"CRITERIA DELETION (UC-002)",
	}
	if got := run.commits(); !slices.Equal(got, wantCommits) {
		t.Errorf("commits = %q, want %q", got, wantCommits)
	}

	// Gated only for the pilot: 3 approvals for UC-001, none for UC-002, and
	// no go-ahead gate between pilot and scaling.
	wantApprovals := []string{"approve:diagram", "approve:narrative:UC-001", "approve:criteria:UC-001", "approve:tests:UC-001"}
	var approvals []string
	for _, e := range run.events {
		if strings.HasPrefix(e, "approve:") {
			approvals = append(approvals, e)
		}
	}
	if !slices.Equal(approvals, wantApprovals) {
		t.Errorf("approvals = %q, want %q", approvals, wantApprovals)
	}
	// Safety gate after the diagram, after each use case's narrative,
	// criteria and 4 test layers (6 x 2), and the final one.
	if got := run.count("action:check-system-unchanged"); got != 14 {
		t.Errorf("check-system-unchanged ran %d times, want 14", got)
	}
	// Each use case's WIP gate removal gets that use case's tests only.
	if !slices.Equal(run.wipNames, []string{"at_UC-001", "at_UC-002"}) {
		t.Errorf("remove-wip-gate test-names = %q, want one use case each", run.wipNames)
	}
	// UC-002 changes no system driver port: its adapter layer is skipped.
	if got := run.count("agent:system-driver-adapter-implementer"); got != 1 {
		t.Errorf("system-driver-adapter-implementer dispatched %d times, want 1 (UC-002 has no port change)", got)
	}
	// UC-002 has no ESCC: one contract cycle in total.
	if got := run.count("agent:contract-test-writer"); got != 1 {
		t.Errorf("contract-test-writer dispatched %d times, want 1", got)
	}
	// Nothing rejected: no agent saw feedback.
	for _, d := range run.agentParams {
		if d.feedback != "" {
			t.Errorf("%s saw feedback %q on an all-approved run", d.name, d.feedback)
		}
	}
	// No per-ticket fixer anywhere.
	for _, a := range []string{"agent:unexpected-failing-tests-fixer", "agent:unexpected-passing-tests-fixer"} {
		if run.count(a) != 0 {
			t.Errorf("%s dispatched during characterize", a)
		}
	}
}

// 2. "System started once": one cold start before any agent, no rebuild, no
// full restart; stand-ins are recreated only by the external-only restart.
//
// The harness cannot tell the cold start from the inner start-system probes
// (write-and-verify-acceptance-test-code, implement-test-layer,
// START_SYSTEM_BEFORE_STUB_PROBE): all dispatch the same idempotent
// `gh optivem system start`. So it asserts: exactly one start-system before
// the first agent, every start-system is that exact command (no --restart),
// and no build-system / start-system-restart at all.
func TestCharacterize_SystemStartedOnce(t *testing.T) {
	scenarios := map[string]*charScenario{
		"happy path": happyScenario(),
		// Reds that drive every reconcile refresh site and the fixer's
		// stand-in restart.
		"simulator and stub reds, fixer touches stand-ins": func() *charScenario {
			sc := happyScenario()
			sc.contractOutcomes = map[string][]string{
				"contract-real": {"fail", "fail"}, // probe red, then a verify red for the fixer
				"contract-stub": {"fail"},
			}
			sc.fixerTouchesStandIns = true
			return sc
		}(),
		"test-instance red": func() *charScenario {
			sc := happyScenario()
			sc.realKind = "test-instance"
			sc.contractOutcomes = map[string][]string{"contract-real": {"fail", "fail"}}
			sc.fixerTouchesStandIns = true
			return sc
		}(),
	}
	for name, sc := range scenarios {
		t.Run(name, func(t *testing.T) {
			run := runChar(t, "characterize", sc, nil)
			if run.err != nil {
				t.Fatalf("characterize: %v\ntrail:\n  %s", run.err, strings.Join(run.events, "\n  "))
			}
			firstAgent := slices.IndexFunc(run.events, func(e string) bool { return strings.HasPrefix(e, "agent:") })
			if n := run.count("cmd:start-system"); n == 0 || run.index("cmd:start-system") > firstAgent {
				t.Fatalf("no start-system before the first agent; trail:\n  %s", strings.Join(run.events, "\n  "))
			}
			if n := slices.Index(run.events[run.index("cmd:start-system")+1:firstAgent], "cmd:start-system"); n >= 0 {
				t.Error("more than one start-system before the first agent")
			}
			for _, e := range []string{"cmd:build-system", "cmd:start-system-restart"} {
				if c := run.count(e); c != 0 {
					t.Errorf("%s dispatched %d time(s) during characterize, want 0", e, c)
				}
			}
			for _, c := range run.commands {
				if strings.Contains(c, "--restart") && c != "gh optivem system start --restart --external-only" {
					t.Errorf("command %q restarts more than the stand-ins", c)
				}
				if strings.HasPrefix(c, "gh optivem system start") && c != "gh optivem system start" && c != "gh optivem system start --restart --external-only" {
					t.Errorf("unexpected system start variant %q", c)
				}
				if strings.HasPrefix(c, "gh optivem system build") {
					t.Errorf("system build %q dispatched during characterize", c)
				}
			}
			if run.count("cmd:restart-external-systems") == 0 {
				t.Error("restart-external-systems never dispatched; the external layer should refresh the stand-ins")
			}
			if run.count("cmd:stop-system") != 1 {
				t.Errorf("stop-system dispatched %d times, want 1", run.count("cmd:stop-system"))
			}
		})
	}
}

// The stand-in restart counts: one per reconcile refresh site reached, plus one
// per characterization-fixer pass that touched a stand-in.
func TestCharacterize_StandInRestartPerRefreshSite(t *testing.T) {
	sc := happyScenario()
	sc.contractOutcomes = map[string][]string{
		"contract-real": {"fail", "fail"}, // probe red (simulator), verify red once (fixer)
		"contract-stub": {"fail"},         // probe red → stubs
	}
	sc.fixerTouchesStandIns = true
	run := runChar(t, "characterize", sc, nil)
	if run.err != nil {
		t.Fatalf("characterize: %v", run.err)
	}
	// Refresh sites: after driver, after simulator, after stubs = 3; plus the
	// fixer pass in the contract-real verify = 1.
	if got := run.count("cmd:restart-external-systems"); got != 4 {
		t.Errorf("restart-external-systems = %d, want 4 (3 refresh sites + 1 fixer pass)\ntrail:\n  %s", got, strings.Join(run.events, "\n  "))
	}
	if got := run.count("agent:characterization-test-fixer"); got != 1 {
		t.Errorf("characterization-test-fixer = %d, want 1", got)
	}
}

// 3. Both refresh-external-producer branches.
func TestRefreshExternalProducer_Branches(t *testing.T) {
	cases := []struct {
		characterize *bool
		want         []string
	}{
		{characterize: ptr(true), want: []string{"cmd:restart-external-systems"}},
		{characterize: ptr(false), want: []string{"cmd:build-system", "cmd:start-system-restart"}},
		// Entry points with no preset (refine-ticket, refactor, a --target
		// slice) read unset as false: the per-ticket behaviour.
		{characterize: nil, want: []string{"cmd:build-system", "cmd:start-system-restart"}},
	}
	for _, c := range cases {
		name := "unset"
		if c.characterize != nil {
			name = fmt.Sprint(*c.characterize)
		}
		t.Run("characterize="+name, func(t *testing.T) {
			run := runChar(t, "refresh-external-producer", &charScenario{}, func(ctx *statemachine.Context) {
				if c.characterize != nil {
					ctx.Set("characterize", *c.characterize)
				}
			})
			if run.err != nil {
				t.Fatalf("refresh-external-producer: %v", run.err)
			}
			if !slices.Equal(run.events, c.want) {
				t.Errorf("trail = %q, want %q", run.events, c.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

// 4. The characterize gateway in both verify processes.
func TestVerifyTests_CharacterizeGatewayPicksFixer(t *testing.T) {
	cases := []struct {
		proc         string
		firstOutcome string // routes into the fixer
		characterize bool
		wantAgent    string
		wantTask     string
	}{
		{"verify-tests-pass", "fail", true, "characterization-test-fixer", "fix-characterization-failing-tests"},
		{"verify-tests-pass", "fail", false, "unexpected-failing-tests-fixer", "fix-unexpected-failing-tests"},
		{"verify-tests-fail", "pass", true, "characterization-test-fixer", "fix-characterization-passing-tests"},
		{"verify-tests-fail", "pass", false, "unexpected-passing-tests-fixer", "fix-unexpected-passing-tests"},
	}
	for _, c := range cases {
		for _, standIns := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/characterize=%v/stand-ins-changed=%v", c.proc, c.characterize, standIns), func(t *testing.T) {
				converged := "pass"
				if c.firstOutcome == "pass" {
					converged = "fail"
				}
				sc := &charScenario{
					contractOutcomes:     map[string][]string{"acceptance": {c.firstOutcome, converged}},
					fixerTouchesStandIns: standIns,
				}
				run := runChar(t, c.proc, sc, func(ctx *statemachine.Context) {
					ctx.Set("characterize", c.characterize)
					ctx.Params["suite"] = "acceptance"
					ctx.Params["test-names"] = "at_UC-001"
				})
				if run.err != nil {
					t.Fatalf("%s: %v\ntrail: %q", c.proc, run.err, run.events)
				}
				var agents []string
				for _, d := range run.agentParams {
					agents = append(agents, d.name)
				}
				if !slices.Equal(agents, []string{c.wantAgent}) {
					t.Fatalf("agents = %q, want [%s]", agents, c.wantAgent)
				}
				if got := run.agentParams[0].params["task-name"]; got != c.wantTask {
					t.Errorf("task-name = %q, want %q", got, c.wantTask)
				}
				if c.wantAgent == "characterization-test-fixer" {
					if c.proc == "verify-tests-fail" {
						if got := run.agentParams[0].params["verify-failure-output"]; got == "" {
							t.Error("the passing-tests fixer gets no verify-failure-output (a passing run leaves none in State)")
						}
					}
					wantRestart := 0
					if standIns {
						wantRestart = 1
					}
					if got := run.count("cmd:restart-external-systems"); got != wantRestart {
						t.Errorf("restart-external-systems = %d, want %d", got, wantRestart)
					}
					run.wantInOrder(t, "action:snapshot-stand-ins", "agent:characterization-test-fixer", "action:check-stand-ins-changed")
				} else if run.count("cmd:restart-external-systems") != 0 {
					t.Error("per-ticket fixer must not restart the stand-ins")
				}
				if got := run.count("cmd:run-tests"); got != 2 {
					t.Errorf("run-tests = %d, want 2 (red, fix, re-run)", got)
				}
			})
		}
	}
}

// 5. GATE_CONTRACT_REAL_RED_KIND's test-instance edge.
func TestReconcile_TestInstanceRedRoutesOnCharacterize(t *testing.T) {
	for _, characterize := range []bool{true, false} {
		t.Run(fmt.Sprintf("characterize=%v", characterize), func(t *testing.T) {
			sc := &charScenario{
				realKind: "test-instance",
				// Probe red; while characterizing, the verify is red once more
				// (the fixer runs) and then green.
				contractOutcomes: map[string][]string{"contract-real": {"fail", "fail"}},
			}
			run := runChar(t, "reconcile-external-contract-producer", sc, func(ctx *statemachine.Context) {
				ctx.Set("characterize", characterize)
				ctx.Set("real-kind", "test-instance")
				ctx.Set("test-names", []string{"ct_UC-001"})
			})
			if !characterize {
				if run.err == nil || !strings.Contains(run.err.Error(), "CONTRACT_REAL_UPSTREAM_GAP_HALT") {
					t.Fatalf("err = %v, want CONTRACT_REAL_UPSTREAM_GAP_HALT", run.err)
				}
				if n := len(run.agentParams); n != 0 {
					t.Errorf("%d agent(s) dispatched before the upstream-gap halt, want 0", n)
				}
				return
			}
			if run.err != nil {
				t.Fatalf("reconcile: %v\ntrail: %q", run.err, run.events)
			}
			var realRuns int
			for _, r := range run.testRuns {
				if r.suite == "contract-real" {
					realRuns++
				}
			}
			// probe + verify red + verify green
			if realRuns != 3 {
				t.Errorf("contract-real runs = %d, want 3 (probe, red verify, green verify)", realRuns)
			}
			if run.count("agent:characterization-test-fixer") != 1 {
				t.Errorf("characterization-test-fixer = %d, want 1", run.count("agent:characterization-test-fixer"))
			}
			if run.count("agent:external-system-real-simulator-implementer") != 0 {
				t.Error("a test instance is not ours: no simulator implementer")
			}
		})
	}
	eng := loadSnapshot(t)
	rec := eng.Processes["reconcile-external-contract-producer"]
	wantEdge(t, rec, "GATE_CONTRACT_REAL_RED_KIND", "GATE_CHARACTERIZE", "real-kind == test-instance")
	wantEdge(t, rec, "GATE_CHARACTERIZE", "CONTRACT_REAL_UPSTREAM_GAP_HALT", "characterize == false")
	wantEdge(t, rec, "GATE_CHARACTERIZE", "VERIFY_TESTS_PASS_CONTRACT_REAL_TEST_INSTANCE", "characterize == true")
	if p := rec.Nodes["VERIFY_TESTS_PASS_CONTRACT_REAL_TEST_INSTANCE"].Raw.Params["suite"]; p != "contract-real" {
		t.Errorf("test-instance verify suite = %q, want contract-real", p)
	}
}

// lateDiscoverySetup seeds the State characterize-test-layers' RESTAMP_BEFORE_DSL
// leaves before the DSL layer runs for UC-001.
func lateDiscoverySetup(ctx *statemachine.Context) {
	ctx.Set("characterize", true)
	ctx.Set("use-case-id", "UC-001")
	ctx.Set("ticket-has-escc", false)
	ctx.Set("dsl-port-changed", true)
	ctx.Set("test-names", []string{"at_UC-001"})
	ctx.Set("acceptance-criteria", "Scenario: UC-001")
}

// 6. The late-discovery route in characterize-dsl-layer, and its false branch.
func TestCharacterizeDSLLayer_LateExternalDiscovery(t *testing.T) {
	t.Run("external driver port changed", func(t *testing.T) {
		sc := &charScenario{useCases: map[string]*charUseCase{
			"UC-001": {dslPort: true, extPort: true},
		}}
		run := runChar(t, "characterize-dsl-layer", sc, lateDiscoverySetup)
		if run.err != nil {
			t.Fatalf("characterize-dsl-layer: %v\ntrail:\n  %s", run.err, strings.Join(run.events, "\n  "))
		}
		run.wantInOrder(t,
			"agent:dsl-implementer",
			"commit:DSL (UC-001)",
			"agent:legacy-use-case-narrative-writer",
			"agent:legacy-use-case-criteria-writer",
			"commit:USE CASE (UC-001)",
			"action:restamp-current-use-case",
			"action:validate-external-systems-registered",
			"agent:contract-test-writer",
			"cmd:restart-external-systems",
		)
		// Both writers get the discovered port paths and the use case.
		for _, name := range []string{"legacy-use-case-narrative-writer", "legacy-use-case-criteria-writer"} {
			var writer *agentDispatch
			for i := range run.agentParams {
				if run.agentParams[i].name == name {
					writer = &run.agentParams[i]
				}
			}
			if writer == nil {
				t.Fatalf("%s not dispatched", name)
			}
			if got := writer.params["discovered-external-port-paths"]; got != "driver/port/external/erp/ErpDriver.java" {
				t.Errorf("%s discovered-external-port-paths = %q", name, got)
			}
			if got := writer.params["use-case-id"]; got != "UC-001" {
				t.Errorf("%s use-case-id = %q, want UC-001", name, got)
			}
		}
		// The DSL verify expected red (an external driver is pending); the
		// route ends with the acceptance tests verified green.
		last := run.testRuns[len(run.testRuns)-1]
		if last.suite != "acceptance" {
			t.Errorf("last test run suite = %q, want acceptance (VERIFY_TESTS_PASS_ACCEPTANCE)", last.suite)
		}
		if run.events[len(run.events)-1] != "cmd:run-tests" {
			t.Errorf("route must end with the acceptance verify; trail ends %q", run.events[len(run.events)-1])
		}
		if n := run.countPrefix("agent:characterization-test-fixer"); n != 0 {
			t.Errorf("fixer dispatched %d times on a clean route", n)
		}
	})
	t.Run("no external driver port change", func(t *testing.T) {
		sc := &charScenario{useCases: map[string]*charUseCase{"UC-001": {dslPort: true}}}
		run := runChar(t, "characterize-dsl-layer", sc, lateDiscoverySetup)
		if run.err != nil {
			t.Fatalf("characterize-dsl-layer: %v", run.err)
		}
		for _, e := range []string{"agent:legacy-use-case-narrative-writer", "agent:legacy-use-case-criteria-writer", "action:restamp-current-use-case", "agent:contract-test-writer"} {
			if run.count(e) != 0 {
				t.Errorf("%s ran without an external driver port change", e)
			}
		}
		if got := run.commits(); !slices.Equal(got, []string{"DSL (UC-001)"}) {
			t.Errorf("commits = %q, want [DSL (UC-001)]", got)
		}
	})
	t.Run("no DSL port change", func(t *testing.T) {
		sc := &charScenario{useCases: map[string]*charUseCase{"UC-001": {}}}
		run := runChar(t, "characterize-dsl-layer", sc, func(ctx *statemachine.Context) {
			lateDiscoverySetup(ctx)
			ctx.Set("dsl-port-changed", false)
		})
		if run.err != nil {
			t.Fatalf("characterize-dsl-layer: %v", run.err)
		}
		if len(run.events) != 0 {
			t.Errorf("trail = %q, want nothing", run.events)
		}
	})
	eng := loadSnapshot(t)
	dsl := eng.Processes["characterize-dsl-layer"]
	wantEdge(t, dsl, "GATE_EXTERNAL_DRIVER_PORT_CHANGED", "ADD_ESCC_FOR_DISCOVERED_SYSTEMS", "external-driver-port-changed == true")
	wantEdge(t, dsl, "GATE_EXTERNAL_DRIVER_PORT_CHANGED", "CHARACTERIZE_DSL_LAYER_END", "external-driver-port-changed == false")
	wantEdge(t, dsl, "ADD_ESCC_FOR_DISCOVERED_SYSTEMS", "RESTAMP_USE_CASE", "")
	wantEdge(t, dsl, "RESTAMP_USE_CASE", "EXTERNAL_LAYER", "")
	wantEdge(t, dsl, "EXTERNAL_LAYER", "RESTAMP_USE_CASE_AFTER_EXTERNAL", "")
	wantEdge(t, dsl, "RESTAMP_USE_CASE_AFTER_EXTERNAL", "VERIFY_TESTS_PASS_ACCEPTANCE", "")
	wantEdge(t, dsl, "VERIFY_TESTS_PASS_ACCEPTANCE", "CHARACTERIZE_DSL_LAYER_END", "")
	if got := dsl.Nodes["IMPLEMENT_AND_VERIFY_DSL"].Raw.Params["verify-pending-on"]; got != "drivers" {
		t.Errorf("DSL layer verify-pending-on = %q, want drivers", got)
	}
}

// The late-discovery route's final verify must run the use case's acceptance
// tests. The external layer's contract-test-writer emits `test-names` (its
// contract tests) into the shared State, and VERIFY_TESTS_PASS_ACCEPTANCE binds
// `test-names: ${test-names}` from State, so RESTAMP_USE_CASE_AFTER_EXTERNAL
// must re-derive the acceptance tests first.
func TestCharacterizeDSLLayer_LateDiscoveryVerifiesUseCaseAcceptanceTests(t *testing.T) {
	sc := &charScenario{useCases: map[string]*charUseCase{"UC-001": {dslPort: true, extPort: true}}}
	run := runChar(t, "characterize-dsl-layer", sc, lateDiscoverySetup)
	if run.err != nil {
		t.Fatalf("characterize-dsl-layer: %v", run.err)
	}
	last := run.testRuns[len(run.testRuns)-1]
	if last.suite != "acceptance" || last.names != "at_UC-001" {
		t.Errorf("final verify = suite %q test-names %q, want acceptance / at_UC-001", last.suite, last.names)
	}
}

// 7. Safety gate halts, with no fix agent.
func TestVerifySystemUnchanged_Halts(t *testing.T) {
	cases := []struct {
		name               string
		systemChangedAt    int
		assertionRemovedAt int
		wantHalt           string
		wantAssertionCheck bool
	}{
		{"system changed", 1, 0, "SYSTEM_CHANGED_HALT", false},
		{"assertion removed", 0, 1, "ASSERTION_REMOVED_HALT", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sc := &charScenario{systemChangedAt: c.systemChangedAt, assertionRemovedAt: c.assertionRemovedAt}
			run := runChar(t, "verify-system-unchanged", sc, nil)
			if run.err == nil || !strings.Contains(run.err.Error(), c.wantHalt) {
				t.Fatalf("err = %v, want %s", run.err, c.wantHalt)
			}
			if got := run.count("action:check-no-assertion-removed") == 1; got != c.wantAssertionCheck {
				t.Errorf("check-no-assertion-removed ran = %v, want %v", got, c.wantAssertionCheck)
			}
			if len(run.agentParams) != 0 || len(run.commands) != 0 {
				t.Errorf("safety gate dispatched agents %v / commands %q, want none", run.agentParams, run.commands)
			}
		})
	}
	t.Run("clean", func(t *testing.T) {
		run := runChar(t, "verify-system-unchanged", &charScenario{}, nil)
		if run.err != nil {
			t.Fatalf("verify-system-unchanged: %v", run.err)
		}
	})
}

// Inside the full walk, a safety-gate halt stops the run where it fires: no fix
// agent, no further layer, the system left running.
func TestCharacterize_SafetyGateHaltStopsRun(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(sc *charScenario)
		wantHalt string
	}{
		// 2nd check = after the pilot use case's narrative commit.
		{"system changed", func(sc *charScenario) { sc.systemChangedAt = 2 }, "SYSTEM_CHANGED_HALT"},
		{"assertion removed", func(sc *charScenario) { sc.assertionRemovedAt = 2 }, "ASSERTION_REMOVED_HALT"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sc := happyScenario()
			c.mutate(sc)
			run := runChar(t, "characterize", sc, nil)
			if run.err == nil || !strings.Contains(run.err.Error(), c.wantHalt) {
				t.Fatalf("err = %v, want %s", run.err, c.wantHalt)
			}
			last := run.events[len(run.events)-1]
			if last != "action:check-system-unchanged" && last != "action:check-no-assertion-removed" {
				t.Errorf("run continued after the safety gate; trail ends %q", last)
			}
			for _, a := range []string{"agent:characterization-test-fixer", "agent:unexpected-failing-tests-fixer", "agent:acceptance-test-writer"} {
				if run.count(a) != 0 {
					t.Errorf("%s dispatched; the halt must come before any further agent", a)
				}
			}
			if run.count("cmd:stop-system") != 0 {
				t.Error("stop-system ran after a halt (the system is left running on a halt)")
			}
		})
	}
}

// 8. The pilot's three gates: a "no" redoes the step with the feedback (the
// redone agents see it, the next "yes" clears it), the cap halts; scaling use
// cases are never gated, whatever their scripted rejections.
func TestCharacterizeUseCase_GatesRedoWithFeedback(t *testing.T) {
	cases := []struct {
		step    string
		rejects int
		// The agents the step's redo dispatches again.
		redone  []string
		wantErr string
	}{
		{step: "narrative", rejects: 1, redone: []string{"legacy-use-case-narrative-writer"}},
		{step: "criteria", rejects: 1, redone: []string{"legacy-use-case-criteria-writer"}},
		{step: "tests", rejects: 1, redone: []string{"contract-test-writer", "acceptance-test-writer", "dsl-implementer", "system-driver-adapter-implementer"}},
		{step: "narrative", rejects: 3, redone: []string{"legacy-use-case-narrative-writer"}, wantErr: "NARRATIVE_APPROVAL_EXHAUSTED"},
		{step: "criteria", rejects: 3, redone: []string{"legacy-use-case-criteria-writer"}, wantErr: "CRITERIA_APPROVAL_EXHAUSTED"},
		{step: "tests", rejects: 3, redone: []string{"acceptance-test-writer"}, wantErr: "TESTS_APPROVAL_EXHAUSTED"},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s rejected %d", c.step, c.rejects), func(t *testing.T) {
			sc := happyScenario()
			sc.useCases["UC-001"].rejects = map[string]int{c.step: c.rejects}
			// Scaling is never gated: these rejections must not fire.
			sc.useCases["UC-002"].rejects = map[string]int{"narrative": 1, "criteria": 1, "tests": 1}
			run := runChar(t, "characterize", sc, nil)
			if c.wantErr != "" {
				if run.err == nil || !strings.Contains(run.err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want %s", run.err, c.wantErr)
				}
				// Cap 3: three approvals, then the 4th arrival halts.
				if got := run.count("approve:" + c.step + ":UC-001"); got != 3 {
					t.Errorf("%s approvals = %d, want 3", c.step, got)
				}
				for _, a := range c.redone {
					if got := run.count("agent:" + a); got != 4 {
						t.Errorf("%s dispatched %d times, want 4", a, got)
					}
				}
				if run.count("cmd:stop-system") != 0 {
					t.Error("stop-system ran after a halt")
				}
				return
			}
			if run.err != nil {
				t.Fatalf("characterize: %v\ntrail:\n  %s", run.err, strings.Join(run.events, "\n  "))
			}
			if got := run.count("approve:" + c.step + ":UC-001"); got != 2 {
				t.Errorf("%s approvals = %d, want 2 (declined, then approved)", c.step, got)
			}
			for _, e := range run.events {
				if strings.HasSuffix(e, ":UC-002") && strings.HasPrefix(e, "approve:") {
					t.Errorf("scaling use case gated: %s", e)
				}
			}
			// The redone agents run twice as often for UC-001 (the DSL
			// implementer runs in the external and the DSL layer); only the
			// redo's dispatches see the feedback, cleared once approved.
			for _, a := range c.redone {
				var seen []string
				for _, d := range run.agentParams {
					if d.name == a && d.useCase == "UC-001" {
						seen = append(seen, d.feedback)
					}
				}
				half := len(seen) / 2
				ok := len(seen) >= 2 && len(seen)%2 == 0
				for i, f := range seen {
					if (i < half) != (f == "") || (i >= half && !strings.HasPrefix(f, "redo Approve the ")) {
						ok = false
					}
				}
				if !ok {
					t.Errorf("%s feedback per UC-001 dispatch = %q, want the first pass without, the redo with feedback", a, seen)
				}
			}
			for _, d := range run.agentParams {
				if d.feedback != "" && d.useCase == "UC-002" {
					t.Errorf("%s still saw feedback for the scaling use case", d.name)
				}
			}
			// The redo is a new commit on top.
			label := map[string]string{
				"narrative": "USE CASE NARRATIVE (UC-001)",
				"criteria":  "USE CASE CRITERIA (UC-001)",
				"tests":     "ACCEPTANCE TESTS (UC-001)",
			}[c.step]
			if got := run.count("commit:" + label); got != 2 {
				t.Errorf("commit %q = %d, want 2", label, got)
			}
		})
	}
}

// A narrative that finds a dependency not done yet defers its use case behind
// it: the dependency runs through every step first, then the deferred use
// case starts again from its narrative (its first narrative commit stays).
func TestCharacterize_ReorderDefersBehindDependency(t *testing.T) {
	sc := &charScenario{
		proposed: "UC-002",
		pilot:    []string{"UC-002"},
		scaling:  []string{"UC-004"},
		useCases: map[string]*charUseCase{
			"UC-002": {dslPort: true, dependsOn: []string{"UC-003"}},
			"UC-003": {dslPort: true},
			"UC-004": {dslPort: true},
		},
	}
	run := runChar(t, "characterize", sc, nil)
	if run.err != nil {
		t.Fatalf("characterize: %v\ntrail:\n  %s", run.err, strings.Join(run.events, "\n  "))
	}
	wantCommits := []string{
		"USE CASE DIAGRAM",
		"USE CASE NARRATIVE (UC-002)",
		"USE CASE NARRATIVE (UC-003)",
		"USE CASE CRITERIA (UC-003)",
		"ACCEPTANCE TESTS (UC-003)",
		"DSL (UC-003)",
		"WIP GATE REMOVAL (UC-003)",
		"CRITERIA DELETION (UC-003)",
		"USE CASE NARRATIVE (UC-002)",
		"USE CASE CRITERIA (UC-002)",
		"ACCEPTANCE TESTS (UC-002)",
		"DSL (UC-002)",
		"WIP GATE REMOVAL (UC-002)",
		"CRITERIA DELETION (UC-002)",
		"USE CASE NARRATIVE (UC-004)",
		"USE CASE CRITERIA (UC-004)",
		"ACCEPTANCE TESTS (UC-004)",
		"DSL (UC-004)",
		"WIP GATE REMOVAL (UC-004)",
		"CRITERIA DELETION (UC-004)",
	}
	if got := run.commits(); !slices.Equal(got, wantCommits) {
		t.Errorf("commits = %q, want %q", got, wantCommits)
	}
	// The deferred use case's criteria writer does not run before the
	// re-order; the dependency, added during the pilot, is gated like it.
	for _, want := range []string{"approve:narrative:UC-002", "approve:narrative:UC-003", "approve:criteria:UC-003", "approve:tests:UC-003", "approve:tests:UC-002"} {
		if run.count(want) == 0 {
			t.Errorf("%s missing: the pilot (incl. a dependency found during it) is gated", want)
		}
	}
	if run.count("approve:narrative:UC-002") != 2 {
		t.Errorf("approve:narrative:UC-002 = %d, want 2 (before and after the deferral)", run.count("approve:narrative:UC-002"))
	}
	if run.countPrefix("approve:") != 1+1+3+3 {
		t.Errorf("approvals = %d, want 8 (diagram, UC-002 narrative, UC-003 x3, UC-002 x3 again)", run.countPrefix("approve:"))
	}
	if run.count("action:reorder-use-cases") != 4 {
		t.Errorf("reorder-use-cases = %d, want 4 (UC-002 deferred, UC-003, UC-002, UC-004)", run.count("action:reorder-use-cases"))
	}
}

// Pilot covers every use case: scaling has nothing to do; the run still ends
// with the final safety gate and the stop.
func TestCharacterize_EmptyScaling(t *testing.T) {
	sc := happyScenario()
	sc.scaling = nil
	run := runChar(t, "characterize", sc, nil)
	if run.err != nil {
		t.Fatalf("characterize: %v", run.err)
	}
	if run.count("agent:legacy-use-case-narrative-writer") != 1 {
		t.Errorf("narrative writer = %d, want 1 (pilot only)", run.count("agent:legacy-use-case-narrative-writer"))
	}
	run.wantInOrder(t, "commit:CRITERIA DELETION (UC-001)", "action:order-scaling-use-cases", "action:check-system-unchanged", "cmd:stop-system")
}

// The wiring the walks rely on, pinned on the graph.
func TestCharacterize_Wiring(t *testing.T) {
	eng := loadSnapshot(t)
	ch := eng.Processes["characterize"]
	wantEdge(t, ch, "GATE_USE_CASE_DIAGRAM_CONFIRMED", "ORDER_PILOT_USE_CASES", "approval-outcome == approved")
	wantEdge(t, ch, "GATE_USE_CASE_DIAGRAM_CONFIRMED", "WRITE_USE_CASE_DIAGRAM", "approval-outcome == rejected")
	wantEdge(t, ch, "ORDER_PILOT_USE_CASES", "PILOT", "")
	wantEdge(t, ch, "PILOT", "ORDER_SCALING_USE_CASES", "")
	wantEdge(t, ch, "ORDER_SCALING_USE_CASES", "SCALING", "")
	wantEdge(t, ch, "SCALING", "FINAL_SAFETY_GATE", "")
	wantEdge(t, ch, "FINAL_SAFETY_GATE", "STOP_SYSTEM", "")
	wantEdge(t, ch, "STOP_SYSTEM", "CHARACTERIZE_END", "")
	if g := ch.Nodes["PILOT"].Raw.Params["gated"]; g != "true" {
		t.Errorf("PILOT gated = %q, want true", g)
	}
	if g := ch.Nodes["SCALING"].Raw.Params["gated"]; g != "false" {
		t.Errorf("SCALING gated = %q, want false", g)
	}
	loop := eng.Processes["characterize-use-cases"]
	if u := loop.Nodes["NEXT_USE_CASE"].Raw.Params["use-cases"]; u != "${use-case-order}" {
		t.Errorf("NEXT_USE_CASE use-cases = %q, want ${use-case-order} (re-read on every pass)", u)
	}
	wantEdge(t, loop, "CHARACTERIZE_USE_CASE", "NEXT_USE_CASE", "")
	uc := eng.Processes["characterize-use-case"]
	wantEdge(t, uc, "GATE_USE_CASE_DEFERRED", "USE_CASE_DEFERRED_END", "use-case-deferred == true")
	wantEdge(t, uc, "GATE_NARRATIVE_APPROVED", "WRITE_NARRATIVE", "approval-outcome == rejected")
	wantEdge(t, uc, "GATE_CRITERIA_APPROVED", "WRITE_CRITERIA", "approval-outcome == rejected")
	wantEdge(t, eng.Processes["characterize-test-layers"], "GATE_TESTS_APPROVED", "RESTAMP_BEFORE_EXTERNAL", "approval-outcome == rejected")
}

// The diagram redo: a declined diagram re-runs the writer; the cap halts.
func TestCharacterize_DiagramRedoAndCap(t *testing.T) {
	t.Run("declined once", func(t *testing.T) {
		sc := happyScenario()
		sc.rejectDiagram = 1
		run := runChar(t, "characterize", sc, nil)
		if run.err != nil {
			t.Fatalf("characterize: %v", run.err)
		}
		if got := run.count("agent:legacy-use-case-diagram-writer"); got != 2 {
			t.Errorf("diagram writer = %d, want 2", got)
		}
	})
	t.Run("declined at cap", func(t *testing.T) {
		sc := happyScenario()
		sc.rejectDiagram = 3
		run := runChar(t, "characterize", sc, nil)
		if run.err == nil || !strings.Contains(run.err.Error(), "USE_CASE_DIAGRAM_CONFIRMATION_EXHAUSTED") {
			t.Fatalf("err = %v, want USE_CASE_DIAGRAM_CONFIRMATION_EXHAUSTED", run.err)
		}
		if got := run.count("agent:legacy-use-case-diagram-writer"); got != 4 {
			t.Errorf("diagram writer = %d, want 4 (cap 3 approvals, the 4th writer pass then halts at the confirm)", got)
		}
		if run.count("agent:legacy-use-case-narrative-writer") != 0 {
			t.Error("a use case ran without confirmed diagrams")
		}
	})
}
