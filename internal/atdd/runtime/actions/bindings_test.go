// Tests for actions/bindings.go.
//
// Strategy: every action is exercised through fake Gh / Git / Shell /
// Prompter runners so the suite is hermetic. Each test seeds the Context
// inputs the action documents, runs the NodeFn, and asserts:
//   - the Outcome (Err on aborts; clean on success);
//   - the Context state mutated by the action; and
//   - the side-effecting calls observed by the fakes (argv shape).
//
// The board-backed and release-backed actions are tested via the same
// canned-response fakes; no real network or shell is invoked.
package actions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// getwd / chdir wrap os.Getwd / os.Chdir with t.Fatalf on error so each test
// can switch CWD without four lines of plumbing per call site. classify
// writes ./classify.log relative to CWD; tests need a temp dir to avoid
// littering the repo.
func getwd() (string, error) { return os.Getwd() }

func chdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %q: %v", dir, err)
	}
}

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

type fakePrompter struct {
	answers []string
	asked   []string
}

func (f *fakePrompter) Ask(prompt string) (string, error) {
	f.asked = append(f.asked, prompt)
	if len(f.answers) == 0 {
		return "", errors.New("fakePrompter: no answers left")
	}
	a := f.answers[0]
	f.answers = f.answers[1:]
	return a, nil
}

type fakeRunner struct {
	t      *testing.T
	name   string
	calls  [][]string
	canned map[string]cannedResponse
}

type cannedResponse struct {
	out []byte
	err error
}

func newFakeRunner(t *testing.T, name string) *fakeRunner {
	return &fakeRunner{t: t, name: name, canned: map[string]cannedResponse{}}
}

func (f *fakeRunner) on(args []string, out []byte, err error) {
	f.canned[joinArgs(args)] = cannedResponse{out: out, err: err}
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	if r, ok := f.canned[joinArgs(args)]; ok {
		return r.out, r.err
	}
	f.t.Fatalf("%s: unexpected invocation %v (no canned response)", f.name, args)
	return nil, fmt.Errorf("unreachable")
}

func joinArgs(args []string) string {
	return strings.Join(args, "\x00")
}

type fakeShell struct {
	calls []string
	out   []byte
	err   error
}

func (f *fakeShell) Run(_ context.Context, cmd string) ([]byte, error) {
	f.calls = append(f.calls, cmd)
	return f.out, f.err
}

func newActions(deps Deps) actions {
	return actions{deps: deps.withDefaults()}
}

// ---------------------------------------------------------------------------
// askCanICommit
// ---------------------------------------------------------------------------

func TestAskCanICommit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  string
		wantErr bool
	}{
		{name: "yes", answer: "y", wantErr: false},
		{name: "no", answer: "n", wantErr: true},
		{name: "empty_is_no", answer: "", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakePrompter{answers: []string{tc.answer}}
			a := newActions(Deps{Prompter: p})
			out := a.askCanICommit(statemachine.NewContext())
			if (out.Err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got %v", tc.wantErr, out.Err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// runSmokeTest
// ---------------------------------------------------------------------------

func TestRunSmokeTest_RecordsBoolInContext(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer string
		want   bool
	}{
		{name: "passes", answer: "y", want: true},
		{name: "fails", answer: "n", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakePrompter{answers: []string{tc.answer}}
			a := newActions(Deps{Prompter: p})
			ctx := statemachine.NewContext()
			out := a.runSmokeTest(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected error: %v", out.Err)
			}
			if out.Bool != tc.want {
				t.Fatalf("Outcome.Bool: got %v, want %v", out.Bool, tc.want)
			}
			if got := ctx.Get("smoke_test_passes"); got != tc.want {
				t.Fatalf("Context smoke_test_passes: got %v, want %v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// printDriftWarning
// ---------------------------------------------------------------------------

func TestPrintDriftWarning_OnlyPrintsForCompileMode(t *testing.T) {
	for _, tc := range []struct {
		mode      string
		wantWarn  bool
	}{
		{mode: "compile", wantWarn: true},
		{mode: "full", wantWarn: false},
		{mode: "skip", wantWarn: false},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			var stderr strings.Builder
			a := newActions(Deps{Stderr: &stderr})
			ctx := statemachine.NewContext()
			ctx.Set("structural_test_mode", tc.mode)
			out := a.printDriftWarning(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected error: %v", out.Err)
			}
			gotWarn := strings.Contains(stderr.String(), "DRIFT WARNING")
			if gotWarn != tc.wantWarn {
				t.Fatalf("warn=%v, want %v (stderr: %q)", gotWarn, tc.wantWarn, stderr.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// compileInScope / runSampleSuite
// ---------------------------------------------------------------------------

func TestCompileInScope_RunsScriptAndPropagatesError(t *testing.T) {
	for _, tc := range []struct {
		name    string
		shellErr error
		wantErr bool
	}{
		{name: "success", shellErr: nil, wantErr: false},
		{name: "failure", shellErr: errors.New("compile failed"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := &fakeShell{out: []byte("ok"), err: tc.shellErr}
			var stdout, stderr strings.Builder
			a := newActions(Deps{Shell: sh, Stdout: &stdout, Stderr: &stderr})
			out := a.compileInScope(statemachine.NewContext())
			if (out.Err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got %v", tc.wantErr, out.Err)
			}
			if len(sh.calls) != 1 || sh.calls[0] != "./compile-all.sh" {
				t.Fatalf("unexpected shell calls: %v", sh.calls)
			}
		})
	}
}

func TestRunSampleSuite_ShellCommand(t *testing.T) {
	sh := &fakeShell{out: []byte("ok")}
	a := newActions(Deps{Shell: sh})
	out := a.runSampleSuite(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if len(sh.calls) != 1 || sh.calls[0] != "./test-all.sh --sample" {
		t.Fatalf("unexpected shell calls: %v", sh.calls)
	}
}

// ---------------------------------------------------------------------------
// commitPhase / commitOnboarding (exercise the release.Confirmer wiring)
// ---------------------------------------------------------------------------

func TestCommitPhase_SuccessPath(t *testing.T) {
	git := newFakeRunner(t, "git")
	git.on([]string{"add", "-A"}, nil, nil)
	git.on([]string{"status", "--short"}, []byte(" M file.go\n"), nil)
	git.on([]string{"commit", "-m", "Register Customer | AT - RED - TEST"}, nil, nil)
	p := &fakePrompter{answers: []string{"y"}}
	a := newActions(Deps{Git: git, Prompter: p})
	ctx := statemachine.NewContext()
	ctx.Set("issue_title", "Register Customer")
	ctx.Params["phase"] = "AT - RED - TEST"
	out := a.commitPhase(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	// Verify commit -m happened with the expected message.
	want := []string{"commit", "-m", "Register Customer | AT - RED - TEST"}
	found := false
	for _, c := range git.calls {
		if joinArgs(c) == joinArgs(want) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected git commit call with message, got calls: %v", git.calls)
	}
}

func TestCommitPhase_DeclineHaltsRun(t *testing.T) {
	git := newFakeRunner(t, "git")
	git.on([]string{"add", "-A"}, nil, nil)
	git.on([]string{"status", "--short"}, []byte(""), nil)
	p := &fakePrompter{answers: []string{"n"}}
	a := newActions(Deps{Git: git, Prompter: p})
	ctx := statemachine.NewContext()
	ctx.Set("issue_title", "Register Customer")
	ctx.Params["phase"] = "AT - RED - TEST"
	out := a.commitPhase(ctx)
	if out.Err == nil {
		t.Fatalf("expected error on decline")
	}
}

func TestCommitOnboarding_UsesExternalSystemName(t *testing.T) {
	git := newFakeRunner(t, "git")
	git.on([]string{"add", "-A"}, nil, nil)
	git.on([]string{"status", "--short"}, []byte(""), nil)
	git.on([]string{"commit", "-m", "External System Onboarding | acme-inventory"}, nil, nil)
	p := &fakePrompter{answers: []string{"y"}}
	a := newActions(Deps{Git: git, Prompter: p})
	ctx := statemachine.NewContext()
	ctx.Set("external_system_name", "acme-inventory")
	out := a.commitOnboarding(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
}

// ---------------------------------------------------------------------------
// classifyTicket
// ---------------------------------------------------------------------------

func TestClassifyTicket_StoryFastPath(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	// classify.Classify calls `gh issue view <N> --json …` with optional --repo.
	gh.on(
		[]string{"issue", "view", "42", "--json", "number,title,labels,projectItems", "--repo", "optivem/shop"},
		[]byte(`{"number":42,"title":"Register customer","labels":[{"name":"story"}],"projectItems":[]}`),
		nil,
	)
	a := newActions(Deps{
		Gh:       gh,
		Prompter: &fakePrompter{},
	})
	ctx := statemachine.NewContext()
	ctx.Set("issue_num", "42")
	ctx.Set("issue_repo", "optivem/shop")
	// classify also writes to ./classify.log, so chdir to a tempdir first.
	tmp := t.TempDir()
	cwd, _ := getwd()
	defer chdir(t, cwd)
	chdir(t, tmp)

	out := a.classifyTicket(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if got := ctx.GetString("ticket_type"); got != "story" {
		t.Fatalf("ticket_type: got %q, want %q", got, "story")
	}
	if got := ctx.GetString("change_type"); got != "behavior" {
		t.Fatalf("change_type: got %q, want %q", got, "behavior")
	}
	for _, k := range []string{"change_subtype", "change_scope", "change_channel"} {
		if got := ctx.GetString(k); got != "" {
			t.Fatalf("%s: got %q, want empty for story", k, got)
		}
	}
}

func TestClassifyTicket_TaskPromptsForSubtype(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "42", "--json", "number,title,labels,projectItems"},
		[]byte(`{"number":42,"title":"Move /products to v2","labels":[{"name":"task"}],"projectItems":[]}`),
		nil,
	)
	p := &fakePrompter{answers: []string{"system-api-task"}}
	a := newActions(Deps{Gh: gh, Prompter: p})
	ctx := statemachine.NewContext()
	ctx.Set("issue_num", "42")

	tmp := t.TempDir()
	cwd, _ := getwd()
	defer chdir(t, cwd)
	chdir(t, tmp)

	out := a.classifyTicket(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if got := ctx.GetString("ticket_type"); got != "system-api-task" {
		t.Fatalf("ticket_type: got %q, want system-api-task", got)
	}
	wantClassification := map[string]string{
		"change_type":    "structure",
		"change_subtype": "interface",
		"change_scope":   "system",
		"change_channel": "api",
	}
	for k, want := range wantClassification {
		if got := ctx.GetString(k); got != want {
			t.Fatalf("%s: got %q, want %q", k, got, want)
		}
	}
}

func TestClassificationFromTicketType(t *testing.T) {
	// Lock the ticket_type → classification mapping. The four classification
	// fields drive run_cycle and da_cycle dispatch; unmapped axes must stay
	// empty so the gate's prompt fallback can be reached if ever needed.
	for _, tc := range []struct {
		ticketType string
		want       map[string]string
	}{
		{ticketType: "story", want: map[string]string{"change_type": "behavior"}},
		{ticketType: "bug", want: map[string]string{"change_type": "behavior"}},
		{ticketType: "chore", want: map[string]string{"change_type": "structure", "change_subtype": "implementation"}},
		{ticketType: "system-api-task", want: map[string]string{"change_type": "structure", "change_subtype": "interface", "change_scope": "system", "change_channel": "api"}},
		{ticketType: "system-ui-task", want: map[string]string{"change_type": "structure", "change_subtype": "interface", "change_scope": "system", "change_channel": "ui"}},
		{ticketType: "external-api-task", want: map[string]string{"change_type": "structure", "change_subtype": "interface", "change_scope": "external_system"}},
		{ticketType: "unknown", want: nil},
	} {
		t.Run(tc.ticketType, func(t *testing.T) {
			got := classificationFromTicketType(tc.ticketType)
			if len(got) != len(tc.want) {
				t.Fatalf("classification: got %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("%s: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// tickAllCheckboxes (helper)
// ---------------------------------------------------------------------------

func TestTickAllCheckboxes(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{name: "single_unchecked", in: "- [ ] Foo", want: "- [x] Foo"},
		{name: "already_checked_passthrough", in: "- [x] Bar", want: "- [x] Bar"},
		{name: "mixed", in: "- [ ] One\n- [x] Two\n- [ ] Three", want: "- [x] One\n- [x] Two\n- [x] Three"},
		{name: "indented", in: "  - [ ] Indent", want: "  - [x] Indent"},
		{name: "asterisk_form", in: "* [ ] Star", want: "* [x] Star"},
		{name: "no_checkboxes", in: "Plain prose.", want: "Plain prose."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tickAllCheckboxes(tc.in)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// findStatusOption
// ---------------------------------------------------------------------------

func TestFindStatusOption(t *testing.T) {
	raw := []byte(`{
		"fields": [
			{"id":"FID-status","name":"Status","options":[
				{"id":"OPT-ready","name":"Ready"},
				{"id":"OPT-inprog","name":"In progress"},
				{"id":"OPT-inacc","name":"In acceptance"}
			]}
		]
	}`)
	cases := []struct {
		want string
		opt  string
	}{
		{want: "OPT-inacc", opt: "In acceptance"},
		{want: "OPT-inprog", opt: "in progress"},
		{want: "OPT-ready", opt: "Ready"},
	}
	for _, tc := range cases {
		t.Run(tc.opt, func(t *testing.T) {
			fid, oid, err := findStatusOption(raw, tc.opt)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fid != "FID-status" {
				t.Fatalf("fieldID: got %q, want FID-status", fid)
			}
			if oid != tc.want {
				t.Fatalf("optID: got %q, want %q", oid, tc.want)
			}
		})
	}
}

func TestFindStatusOption_Missing(t *testing.T) {
	raw := []byte(`{"fields":[{"id":"X","name":"Status","options":[{"id":"OPT","name":"Ready"}]}]}`)
	_, _, err := findStatusOption(raw, "Done")
	if err == nil {
		t.Fatalf("expected error for missing option")
	}
}

// ---------------------------------------------------------------------------
// RegisterAll wiring
// ---------------------------------------------------------------------------

func TestRegisterAll_AllActionsRegistered(t *testing.T) {
	r := New()
	RegisterAll(r, Deps{
		Prompter: &fakePrompter{},
		Gh:       newFakeRunner(t, "gh"),
		Git:      newFakeRunner(t, "git"),
		Shell:    &fakeShell{},
	})
	want := []string{
		"pick_top_ready",
		"move_to_in_progress",
		"classify_ticket",
		"move_to_in_acceptance",
		"run_smoke_test",
		"commit_onboarding",
		"compile_in_scope",
		"run_sample_suite",
		"print_drift_warning",
		"ask_can_i_commit",
		"commit_phase",
		"tick_checklist",
	}
	for _, name := range want {
		if r.Lookup(name) == nil {
			t.Errorf("action %q not registered", name)
		}
	}
}
