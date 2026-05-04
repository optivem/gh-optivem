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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

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
		mode     string
		wantWarn bool
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
		name     string
		shellErr error
		wantErr  bool
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

func TestClassifyTicket_NativeIssueType(t *testing.T) {
	for _, tc := range []struct {
		name           string
		issueTypeJSON  string
		wantTicketType string
		wantConfident  bool
	}{
		{name: "story", issueTypeJSON: `{"name":"Story"}`, wantTicketType: "story", wantConfident: true},
		{name: "bug", issueTypeJSON: `{"name":"Bug"}`, wantTicketType: "bug", wantConfident: true},
		{name: "task", issueTypeJSON: `{"name":"Task"}`, wantTicketType: "task", wantConfident: true},
		{name: "no_type_set", issueTypeJSON: `null`, wantTicketType: "", wantConfident: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gh := newFakeRunner(t, "gh")
			gh.on(
				[]string{"issue", "view", "42", "--json", "issueType", "--repo", "optivem/shop"},
				[]byte(fmt.Sprintf(`{"issueType":%s}`, tc.issueTypeJSON)),
				nil,
			)
			if tc.wantConfident {
				gh.on(
					[]string{"issue", "view", "42", "--json", "body", "--repo", "optivem/shop"},
					[]byte(`{"body":""}`),
					nil,
				)
			}
			a := newActions(Deps{Gh: gh, Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			ctx.Set("issue_num", "42")
			ctx.Set("issue_repo", "optivem/shop")

			out := a.classifyTicket(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected error: %v", out.Err)
			}
			if got := ctx.GetString("ticket_type"); got != tc.wantTicketType {
				t.Fatalf("ticket_type: got %q, want %q", got, tc.wantTicketType)
			}
			if got := ctx.Get("classify_confident"); got != tc.wantConfident {
				t.Fatalf("classify_confident: got %v, want %v", got, tc.wantConfident)
			}
		})
	}
}

func TestClassifyTicket_PrintsNamedSections(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "7", "--json", "issueType"},
		[]byte(`{"issueType":{"name":"Story"}}`),
		nil,
	)
	body := "Intro line.\n\n" +
		"## Acceptance Criteria\n\n- AC1\n- AC2\n\n" +
		"## Legacy Acceptance Criteria\n\n- legacy 1\n\n" +
		"## Checklist\n\n- [ ] Step\n"
	rawBody, _ := json.Marshal(body)
	gh.on(
		[]string{"issue", "view", "7", "--json", "body"},
		[]byte(fmt.Sprintf(`{"body":%s}`, rawBody)),
		nil,
	)
	var stdout bytes.Buffer
	a := newActions(Deps{Gh: gh, Prompter: &fakePrompter{}, Stdout: &stdout})
	ctx := statemachine.NewContext()
	ctx.Set("issue_num", "7")

	out := a.classifyTicket(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	got := stdout.String()
	for _, want := range []string{
		"Classified #7 as story.",
		"## Legacy Acceptance Criteria\n\n- legacy 1",
		"## Acceptance Criteria\n\n- AC1\n- AC2",
		"## Checklist\n\n- [ ] Step",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("stdout missing %q\nfull stdout:\n%s", want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// classifySubtype
// ---------------------------------------------------------------------------

func TestClassifySubtype_TaskWithSingleLabel(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "42", "--json", "labels", "--repo", "optivem/shop"},
		[]byte(`{"labels":[{"name":"area:billing"},{"name":"subtype:system-interface-redesign"}]}`),
		nil,
	)
	a := newActions(Deps{Gh: gh, Prompter: &fakePrompter{}})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "task")
	ctx.Set("issue_num", "42")
	ctx.Set("issue_repo", "optivem/shop")
	out := a.classifySubtype(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if got := ctx.GetString("subtype"); got != "system-interface-redesign" {
		t.Fatalf("subtype: got %q, want %q", got, "system-interface-redesign")
	}
}

func TestClassifySubtype_MissingLabelHalts(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "7", "--json", "labels"},
		[]byte(`{"labels":[{"name":"area:billing"}]}`),
		nil,
	)
	a := newActions(Deps{Gh: gh, Prompter: &fakePrompter{}})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "task")
	ctx.Set("issue_num", "7")
	out := a.classifySubtype(ctx)
	if out.Err == nil {
		t.Fatalf("expected error for missing subtype label")
	}
	if !strings.Contains(out.Err.Error(), "no subtype:*") {
		t.Fatalf("unexpected error message: %v", out.Err)
	}
}

func TestClassifySubtype_MultipleLabelsHalt(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "7", "--json", "labels"},
		[]byte(`{"labels":[{"name":"subtype:system-interface-redesign"},{"name":"subtype:system-implementation-change"}]}`),
		nil,
	)
	a := newActions(Deps{Gh: gh, Prompter: &fakePrompter{}})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "task")
	ctx.Set("issue_num", "7")
	out := a.classifySubtype(ctx)
	if out.Err == nil {
		t.Fatalf("expected error for multiple subtype labels")
	}
	if !strings.Contains(out.Err.Error(), "multiple subtype:*") {
		t.Fatalf("unexpected error message: %v", out.Err)
	}
}

func TestExtractSubtypeLabels(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "single", raw: `{"labels":[{"name":"subtype:foo"}]}`, want: []string{"foo"}},
		{name: "many_with_other_labels", raw: `{"labels":[{"name":"area:x"},{"name":"subtype:bar"},{"name":"subtype:baz"}]}`, want: []string{"bar", "baz"}},
		{name: "none", raw: `{"labels":[{"name":"area:x"}]}`, want: nil},
		{name: "empty_array", raw: `{"labels":[]}`, want: nil},
		{name: "no_labels_key", raw: `{"title":"x"}`, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractSubtypeLabels([]byte(tc.raw))
			if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseTicketBody
// ---------------------------------------------------------------------------

func TestParseTicketBody_StorySuccess(t *testing.T) {
	body := "## Acceptance Criteria\n\nScenario: ok\n  Given x\n  When y\n  Then z\n\n## Legacy Acceptance Criteria\n\n- old\n"
	rawBody, _ := json.Marshal(body)
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "42", "--json", "body", "--repo", "optivem/shop"},
		[]byte(fmt.Sprintf(`{"body":%s}`, rawBody)),
		nil,
	)
	a := newActions(Deps{Gh: gh, Prompter: &fakePrompter{}})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "story")
	ctx.Set("issue_num", "42")
	ctx.Set("issue_repo", "optivem/shop")
	out := a.parseTicketBody(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if got := ctx.Get("parse_ok"); got != true {
		t.Fatalf("parse_ok: got %v, want true", got)
	}
	if got := ctx.Get("legacy_acceptance_criteria_section_present"); got != true {
		t.Fatalf("legacy_acceptance_criteria_section_present: got %v, want true", got)
	}
}

func TestParseTicketBody_TaskMissingChecklistSetsParseOkFalse(t *testing.T) {
	body := "## Description\n\nNo checklist.\n"
	rawBody, _ := json.Marshal(body)
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "7", "--json", "body"},
		[]byte(fmt.Sprintf(`{"body":%s}`, rawBody)),
		nil,
	)
	var stderr bytes.Buffer
	a := newActions(Deps{Gh: gh, Prompter: &fakePrompter{}, Stderr: &stderr})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "task")
	ctx.Set("issue_num", "7")
	out := a.parseTicketBody(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected hard error: %v", out.Err)
	}
	if got := ctx.Get("parse_ok"); got != false {
		t.Fatalf("parse_ok: got %v, want false", got)
	}
	if !strings.Contains(stderr.String(), "Checklist") {
		t.Fatalf("stderr should mention Checklist: %q", stderr.String())
	}
}

func TestParseTicketBody_StoryWithoutLegacyACSetsLegacyFalse(t *testing.T) {
	body := "## Acceptance Criteria\n\nScenario: ok\n"
	rawBody, _ := json.Marshal(body)
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "1", "--json", "body"},
		[]byte(fmt.Sprintf(`{"body":%s}`, rawBody)),
		nil,
	)
	a := newActions(Deps{Gh: gh, Prompter: &fakePrompter{}})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "story")
	ctx.Set("issue_num", "1")
	out := a.parseTicketBody(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if got := ctx.Get("parse_ok"); got != true {
		t.Fatalf("parse_ok: got %v, want true", got)
	}
	if got := ctx.Get("legacy_acceptance_criteria_section_present"); got != false {
		t.Fatalf("legacy_acceptance_criteria_section_present: got %v, want false", got)
	}
}

func TestParseTicketBody_EmptyTicketTypeFailsHard(t *testing.T) {
	a := newActions(Deps{Gh: newFakeRunner(t, "gh"), Prompter: &fakePrompter{}})
	ctx := statemachine.NewContext()
	ctx.Set("issue_num", "1")
	out := a.parseTicketBody(ctx)
	if out.Err == nil {
		t.Fatalf("expected hard error for empty ticket_type")
	}
}

func TestExtractIssueType(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "story", raw: `{"issueType":{"name":"Story"}}`, want: "Story"},
		{name: "bug_with_extra_fields", raw: `{"issueType":{"id":"abc","name":"Bug","description":"x"}}`, want: "Bug"},
		{name: "null_type", raw: `{"issueType":null}`, want: ""},
		{name: "missing_field", raw: `{"title":"x"}`, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := extractIssueType([]byte(tc.raw))
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// extractIssueSection (helper)
// ---------------------------------------------------------------------------

func TestExtractIssueSection(t *testing.T) {
	body := "Intro.\n\n" +
		"## Acceptance Criteria\n\n- AC1\n- AC2\n\n" +
		"### Sub heading\n\nnested content\n\n" +
		"## Checklist\n\n- [ ] step\n"
	for _, tc := range []struct {
		name    string
		heading string
		want    string
		ok      bool
	}{
		{name: "ac_with_nested", heading: "Acceptance Criteria", want: "- AC1\n- AC2\n\n### Sub heading\n\nnested content", ok: true},
		{name: "checklist", heading: "Checklist", want: "- [ ] step", ok: true},
		{name: "case_insensitive", heading: "checklist", want: "- [ ] step", ok: true},
		{name: "absent", heading: "Legacy Acceptance Criteria", ok: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractIssueSection(body, tc.heading)
			if ok != tc.ok {
				t.Fatalf("ok: got %v, want %v", ok, tc.ok)
			}
			if got != tc.want {
				t.Fatalf("got %q\nwant %q", got, tc.want)
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
		"classify_subtype",
		"parse_ticket_body",
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
