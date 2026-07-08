package actions

// RegisterAll wires every YAML action name to its implementation.
func RegisterAll(r *Registry, deps Deps) {
	deps = deps.withDefaults()
	a := actions{deps: deps}
	// Phase-scope enforcement Layer 2 (per plan 20260518-1144 item 5,
	// retargeted at process-flow.yaml node scope per plan 20260526-1536):
	// runs after the agent commits, diffs the working tree against the
	// writing-agent MID's allowed paths joined from
	// process-flow.yaml's inline node `write:` list + gh-optivem.yaml
	// paths:, and writes phase-scope-clean + phase-scope-violating-paths
	// to context. The downstream phase-scope-clean gate consumes the
	// boolean.
	r.Register("check-phase-scope", a.checkPhaseScope)
	// BPMN Phase D — LOW execute-command primitive. Reads ctx.Params["command"]
	// (the templated bash line, post-ExpandParams), appends --suite= and
	// --test= flags only when ctx.Params["suite"] / ctx.Params["test-names"]
	// are non-empty, shells out, and writes ctx.State["command-succeeded"].
	// For the `gh optivem system-test run` family it also stamps
	// ctx.State["test-outcome"] (pass|fail) so the verify-tests-pass /
	// verify-tests-fail gateways can route without a second shell-out.
	r.Register("run-command", a.runCommand)
	// BPMN Phase D — LOW execute-agent primitive's post-RUN_AGENT
	// validation step. Reads the per-dispatch JSONL outputs file (path
	// stashed at ctx.State["output-file-path"]; agent appends via `gh
	// optivem output write KEY=VAL`) and looks up the writing-agent
	// MID's declared OutputSpec list + write scope via Engine.Outputs /
	// Engine.Scope(task-name) (with originating-task-name fallback for
	// fix dispatches). Writes ctx.State["outputs-and-scopes-valid"] +
	// ctx.State["failure-kind"] (missing-output|scope-diff).
	r.Register("validate-outputs-and-scopes", a.validateOutputsAndScopes)
	// BPMN Phase D — LOW execute-agent primitive's pre-RUN_AGENT
	// baseline-capture step (per plan 20260526-1430). Snapshots the
	// dirty working tree into ctx.State[CtxKeyPreAgentFingerprint] so
	// the post-RUN_AGENT validate-outputs-and-scopes can diff against a
	// per-phase baseline instead of HEAD, eliminating cross-phase
	// false positives when several phases run back-to-back without an
	// intermediate commit. Action is wired into process-flow.yaml's
	// execute-agent subprocess (Item 4 in the same plan); the
	// dormant standalone action is registered here regardless so tests
	// and re-wirings find it.
	r.Register("snapshot-working-tree", a.snapshotWorkingTree)
	// Multi-external-system contract cycle (plan 20260615-0755). The
	// IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS anchor is statically
	// unrolled into one guarded clone per registered external system; these two
	// deterministic actions (no agent) drive that:
	//
	//   - validate-external-systems-registered runs ONCE before the clones and
	//     hard-errors if any external system touched by the port change is not
	//     registered (the no-silent-skip guarantee, plan 20260613-1835's #65
	//     fix, now seeing the whole changed-set against the whole registry).
	//   - resolve-external-system runs at the START of each clone: it reads the
	//     baked external-system-name to self-guard the clone (stamps
	//     external-system-touched for GATE_EXTERNAL_SYSTEM_TOUCHED) and copies
	//     the baked real-kind into gate-readable state for
	//     GATE_CONTRACT_REAL_RED_KIND. Replaces the retired
	//     identify-external-system action.
	r.Register("validate-external-systems-registered", a.validateExternalSystemsRegistered)
	r.Register("resolve-external-system", a.resolveExternalSystem)
	// Redesign-external entry guard (plan 20260622-1739 Step 4c). The
	// redesign-external-system-structure cycle runs no AT/CT cascade, so the only
	// source for which external system(s) the reshape targets is the ticket's
	// ESCC; this runs ONCE before the unrolled per-system clones and hard-errors
	// when the ticket declares no ESCC (else every clone's touched-guard is false
	// and the cycle silently no-ops). See external.go.
	r.Register("validate-redesign-external-requires-escc", a.validateRedesignExternalRequiresESCC)
	// Channel-aware system unroll (plan 20260619-1139). The per-channel system
	// (UnrollSystemChannels) and system-driver-adapter (UnrollSystemDriverAdapterChannels)
	// clones are guarded INSIDE the cycle, mirroring the external-system pattern
	// above:
	//
	//   - validate-channels-registered runs ONCE in shared-contract, after the RED
	//     acceptance verify wrote its per-channel reports, and hard-errors if any
	//     ticket acceptance test ran in NONE of the configured channels (the
	//     no-silent-skip guarantee — a test for an unconfigured channel must fail
	//     loud, never vanish).
	//   - resolve-channel runs at the START of each clone: it reads the baked
	//     `channel` and stamps channel-touched (for GATE_CHANNEL_TOUCHED) by
	//     reading the RED acceptance run's on-disk acceptance-<ch> report — no live
	//     run, no cache (decision #6).
	r.Register("validate-channels-registered", a.validateChannelsRegistered)
	r.Register("resolve-channel", a.resolveChannel)
	// MARK_* state-transition service tasks (per plan
	// 20260526-1220-fix-mark-ticket-state-transition-routing.md). Each
	// dispatches Tracker.SetStatus against the ticketing-system column the
	// canonical state maps to. Tracker.SetStatus is stringly-typed for
	// now; a typed state enum is separate future work.
	r.Register("move-to-in-refinement", a.moveToInRefinement)
	r.Register("move-to-ready", a.moveToReady)
	r.Register("move-to-in-progress", a.moveToInProgress)
	r.Register("move-to-in-acceptance", a.moveToInAcceptance)
	// PARSE_TICKET service task. Reads the raw body via Tracker.ReadBody and
	// runs intake.Parse, which enforces the closed-section contract (only the
	// canonical headings, no stray content) plus the shape rules (AC XOR
	// Checklist, Checklist-is-a-list, AC/ESCC Gherkin), then stashes each
	// section body into ctx.State for downstream prompt substitution. Per-kind
	// required-section enforcement happens at dispatch time via the load-bearing
	// placeholder check in clauderun.go.
	r.Register("parse-ticket", a.parseTicket)
	// verify-tests-pass no-progress guard (plan 20260615-1845 Step 4). Runs
	// on the gateway's fail branch before re-dispatching the fixer; stamps
	// fix-loop-progressing from a per-pass failure fingerprint so
	// GATE_FIX_PROGRESSING halts a fixer that keeps editing without changing
	// the failure. See fix_progress.go.
	r.Register("check-fix-progress", a.checkFixProgress)
	// Guard B — scope-exception categorizer (plan 20260620-2348, generalized to
	// a 3-way enum by plan 20260708-1038). Runs on the execute-agent
	// scope-exception==true branch, before GATE_SCOPE_EXCEPTION_NEEDS_ESCC.
	// Resolves the contract/stub and at-test Family B path families and stamps
	// scope-exception-kind to escc-undeclared | contradictory-tests | other,
	// routing to the matching loud halt (ESCC_UNDECLARED_HALT /
	// CONTRADICTORY_TESTS_HALT) instead of the generic STOP_SCOPE_VIOLATION.
	// See scope_exception.go.
	r.Register("categorize-scope-exception", a.categorizeScopeException)
}
