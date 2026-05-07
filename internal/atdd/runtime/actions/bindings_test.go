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
	"github.com/optivem/gh-optivem/internal/atdd/runtime/testselect"
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
			out := a.reportDriftWarning(ctx)
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
	ctx.Params["change_type"] = "AT - RED - TEST"
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
	ctx.Params["change_type"] = "AT - RED - TEST"
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
// classifyTicketType
// ---------------------------------------------------------------------------

func TestClassifyTicketType_NativeIssueType(t *testing.T) {
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
		{name: "unsupported_type", issueTypeJSON: `{"name":"Spike"}`, wantTicketType: "", wantConfident: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gh := newFakeRunner(t, "gh")
			gh.on(
				[]string{
					"api", "graphql",
					"-f", "owner=optivem",
					"-f", "name=shop",
					"-F", "number=42",
					"-f", "query=" + classifyIssueTypeQuery,
				},
				[]byte(fmt.Sprintf(`{"data":{"repository":{"issue":{"issueType":%s}}}}`, tc.issueTypeJSON)),
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

			out := a.readTicketType(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected error: %v", out.Err)
			}
			if got := ctx.GetString("ticket_type"); got != tc.wantTicketType {
				t.Fatalf("ticket_type: got %q, want %q", got, tc.wantTicketType)
			}
			if got := ctx.Get("ticket_type_recognized"); got != tc.wantConfident {
				t.Fatalf("ticket_type_recognized: got %v, want %v", got, tc.wantConfident)
			}
		})
	}
}

func TestClassifyTicketType_PrintsNamedSections(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{
			"api", "graphql",
			"-f", "owner=optivem",
			"-f", "name=shop",
			"-F", "number=7",
			"-f", "query=" + classifyIssueTypeQuery,
		},
		[]byte(`{"data":{"repository":{"issue":{"issueType":{"name":"Story"}}}}}`),
		nil,
	)
	body := "Intro line.\n\n" +
		"## Acceptance Criteria\n\n- AC1\n- AC2\n\n" +
		"## Legacy Acceptance Criteria\n\n- legacy 1\n\n" +
		"## Checklist\n\n- [ ] Step\n"
	rawBody, _ := json.Marshal(body)
	gh.on(
		[]string{"issue", "view", "7", "--json", "body", "--repo", "optivem/shop"},
		[]byte(fmt.Sprintf(`{"body":%s}`, rawBody)),
		nil,
	)
	var stdout bytes.Buffer
	a := newActions(Deps{Gh: gh, Prompter: &fakePrompter{}, Stdout: &stdout})
	ctx := statemachine.NewContext()
	ctx.Set("issue_num", "7")
	ctx.Set("issue_repo", "optivem/shop")

	out := a.readTicketType(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	got := stdout.String()
	for _, want := range []string{
		"Read ticket type for #7: story.",
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
// reportIntakeSummary
// ---------------------------------------------------------------------------

func TestReportIntakeSummary_PrintsCanonicalLines(t *testing.T) {
	var stdout bytes.Buffer
	a := newActions(Deps{Stdout: &stdout})
	ctx := statemachine.NewContext()
	ctx.Set("issue_num", "42")
	ctx.Set("ticket_type", "task")
	ctx.Set("subtype", "system-interface-redesign")
	ctx.Set("change_type", "system-interface-redesign")
	ctx.Set("parsed_section_names", []string{"Checklist", "Legacy Acceptance Criteria"})

	out := a.reportIntakeSummary(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	got := stdout.String()
	for _, want := range []string{
		"Intake summary:",
		"ticket: #42",
		"ticket_type: task",
		"subtype: system-interface-redesign",
		"change_type: system-interface-redesign",
		"parsed sections: Checklist, Legacy Acceptance Criteria",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("stdout missing %q\nfull stdout:\n%s", want, got)
		}
	}
}

func TestReportIntakeSummary_OmitsFieldsWhenAbsent(t *testing.T) {
	// Story has no subtype, and change_type may not yet be set in older
	// callers. The summary should still print without crashing or emitting
	// blank "subtype: " lines.
	var stdout bytes.Buffer
	a := newActions(Deps{Stdout: &stdout})
	ctx := statemachine.NewContext()
	ctx.Set("issue_num", "7")
	ctx.Set("ticket_type", "story")
	ctx.Set("parsed_section_names", []string{"Acceptance Criteria"})

	out := a.reportIntakeSummary(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	got := stdout.String()
	if strings.Contains(got, "subtype:") {
		t.Errorf("stdout should not include subtype line for story:\n%s", got)
	}
	if strings.Contains(got, "change_type:") {
		t.Errorf("stdout should not include change_type line when unset:\n%s", got)
	}
	for _, want := range []string{
		"ticket: #7",
		"ticket_type: story",
		"parsed sections: Acceptance Criteria",
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
	out := a.readSubtype(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if got := ctx.GetString("subtype"); got != "system-interface-redesign" {
		t.Fatalf("subtype: got %q, want %q", got, "system-interface-redesign")
	}
	if got := ctx.Get("subtype_ok"); got != true {
		t.Fatalf("subtype_ok: got %v, want true", got)
	}
}

func TestClassifySubtype_MissingLabel_RoutesToStop(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "7", "--json", "labels"},
		[]byte(`{"labels":[{"name":"area:billing"}]}`),
		nil,
	)
	var stderr bytes.Buffer
	a := newActions(Deps{Gh: gh, Prompter: &fakePrompter{}, Stderr: &stderr})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "task")
	ctx.Set("issue_num", "7")
	out := a.readSubtype(ctx)
	if out.Err != nil {
		t.Fatalf("expected clean Outcome (STOP, not hard error), got: %v", out.Err)
	}
	if got := ctx.Get("subtype_ok"); got != false {
		t.Fatalf("subtype_ok: got %v, want false", got)
	}
	if !strings.Contains(stderr.String(), "no subtype:*") {
		t.Fatalf("expected stderr message about missing subtype, got: %q", stderr.String())
	}
}

func TestClassifySubtype_MultipleLabels_RoutesToStop(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "7", "--json", "labels"},
		[]byte(`{"labels":[{"name":"subtype:system-interface-redesign"},{"name":"subtype:system-implementation-change"}]}`),
		nil,
	)
	var stderr bytes.Buffer
	a := newActions(Deps{Gh: gh, Prompter: &fakePrompter{}, Stderr: &stderr})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "task")
	ctx.Set("issue_num", "7")
	out := a.readSubtype(ctx)
	if out.Err != nil {
		t.Fatalf("expected clean Outcome (STOP, not hard error), got: %v", out.Err)
	}
	if got := ctx.Get("subtype_ok"); got != false {
		t.Fatalf("subtype_ok: got %v, want false", got)
	}
	if !strings.Contains(stderr.String(), "multiple subtype:*") {
		t.Fatalf("expected stderr message about multiple subtypes, got: %q", stderr.String())
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
	if got := ctx.GetString("ticket_checklist"); got != "" {
		t.Fatalf("ticket_checklist: got %q, want empty (story has no Checklist section)", got)
	}
}

func TestParseTicketBody_TaskSuccessPopulatesChecklist(t *testing.T) {
	checklist := "- [x] Rename \"New Order\" card to \"Place Order\"\n- [x] Rename SKU aria-label to \"Product SKU\""
	body := "## Checklist\n\n" + checklist + "\n"
	rawBody, _ := json.Marshal(body)
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "61", "--json", "body", "--repo", "optivem/shop"},
		[]byte(fmt.Sprintf(`{"body":%s}`, rawBody)),
		nil,
	)
	var stdout bytes.Buffer
	// Autonomous: true so the all-[x] guard doesn't prompt — this test
	// covers the success-path wiring (parse_ok / ticket_checklist /
	// summary block); the prompt branches have dedicated tests below.
	a := newActions(Deps{Gh: gh, Prompter: &fakePrompter{}, Stdout: &stdout, Autonomous: true})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "task")
	ctx.Set("issue_num", "61")
	ctx.Set("issue_repo", "optivem/shop")
	out := a.parseTicketBody(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if got := ctx.Get("parse_ok"); got != true {
		t.Fatalf("parse_ok: got %v, want true", got)
	}
	if got := ctx.GetString("ticket_checklist"); got != checklist {
		t.Fatalf("ticket_checklist: got %q, want %q", got, checklist)
	}
	wantLines := []string{
		"Parsed #61 (task): all required sections present.",
		"Checklist (2 items, 2 already [x]):",
		`  [x] Rename "New Order" card to "Place Order"`,
		`  [x] Rename SKU aria-label to "Product SKU"`,
	}
	for _, line := range wantLines {
		if !strings.Contains(stdout.String(), line) {
			t.Fatalf("stdout missing %q\ngot:\n%s", line, stdout.String())
		}
	}
}

func TestParseTicketBody_TaskAllUncheckedPrintsZeroChecked(t *testing.T) {
	body := "## Checklist\n\n- [ ] Rename foo\n- [ ] Move bar\n- [ ] Delete baz\n"
	rawBody, _ := json.Marshal(body)
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "100", "--json", "body"},
		[]byte(fmt.Sprintf(`{"body":%s}`, rawBody)),
		nil,
	)
	var stdout bytes.Buffer
	a := newActions(Deps{Gh: gh, Prompter: &fakePrompter{}, Stdout: &stdout})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "task")
	ctx.Set("issue_num", "100")
	if out := a.parseTicketBody(ctx); out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !strings.Contains(stdout.String(), "Checklist (3 items, 0 already [x]):") {
		t.Fatalf("missing zero-checked header:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "(already done)") {
		t.Fatalf("(already done) suffix should only appear in the mixed case:\n%s", stdout.String())
	}
}

func TestParseTicketBody_TaskMixedPrintsAlreadyDoneSuffix(t *testing.T) {
	body := "## Checklist\n\n- [x] Rename foo\n- [ ] Move bar\n- [ ] Delete baz\n"
	rawBody, _ := json.Marshal(body)
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "101", "--json", "body"},
		[]byte(fmt.Sprintf(`{"body":%s}`, rawBody)),
		nil,
	)
	var stdout bytes.Buffer
	a := newActions(Deps{Gh: gh, Prompter: &fakePrompter{}, Stdout: &stdout})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "task")
	ctx.Set("issue_num", "101")
	if out := a.parseTicketBody(ctx); out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	wantLines := []string{
		"Checklist (3 items, 1 already [x]):",
		"  [x] Rename foo (already done)",
		"  [ ] Move bar",
		"  [ ] Delete baz",
	}
	for _, line := range wantLines {
		if !strings.Contains(stdout.String(), line) {
			t.Fatalf("stdout missing %q\ngot:\n%s", line, stdout.String())
		}
	}
}

func TestParseTicketBody_AllCheckedInteractiveYesProceeds(t *testing.T) {
	body := "## Checklist\n\n- [x] One\n- [x] Two\n"
	rawBody, _ := json.Marshal(body)
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "61", "--json", "body"},
		[]byte(fmt.Sprintf(`{"body":%s}`, rawBody)),
		nil,
	)
	prompter := &fakePrompter{answers: []string{"y"}}
	var stdout bytes.Buffer
	a := newActions(Deps{Gh: gh, Prompter: prompter, Stdout: &stdout})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "task")
	ctx.Set("issue_num", "61")
	if out := a.parseTicketBody(ctx); out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if got := ctx.Get("parse_ok"); got != true {
		t.Fatalf("parse_ok: got %v, want true (operator proceeded)", got)
	}
	if len(prompter.asked) != 1 {
		t.Fatalf("expected 1 prompt, got %d: %q", len(prompter.asked), prompter.asked)
	}
	if !strings.Contains(stdout.String(), "All 2 checklist items are already marked [x].") {
		t.Fatalf("expected ambiguity preamble on stdout:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "Skipped #") {
		t.Fatalf("should not log skip when operator said yes:\n%s", stdout.String())
	}
}

func TestParseTicketBody_AllCheckedInteractiveDefaultNoSkips(t *testing.T) {
	body := "## Checklist\n\n- [x] One\n- [x] Two\n"
	rawBody, _ := json.Marshal(body)
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "61", "--json", "body"},
		[]byte(fmt.Sprintf(`{"body":%s}`, rawBody)),
		nil,
	)
	// Empty answer → default-N per parseYesNo.
	prompter := &fakePrompter{answers: []string{""}}
	var stdout bytes.Buffer
	a := newActions(Deps{Gh: gh, Prompter: prompter, Stdout: &stdout})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "task")
	ctx.Set("issue_num", "61")
	out := a.parseTicketBody(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected hard error: %v", out.Err)
	}
	if got := ctx.Get("parse_ok"); got != false {
		t.Fatalf("parse_ok: got %v, want false (operator skipped)", got)
	}
	if !strings.Contains(stdout.String(), "Skipped #61 per operator request") {
		t.Fatalf("expected skip line on stdout:\n%s", stdout.String())
	}
}

func TestParseTicketBody_AllCheckedAutonomousWarnsAndProceeds(t *testing.T) {
	body := "## Checklist\n\n- [x] One\n- [x] Two\n"
	rawBody, _ := json.Marshal(body)
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "61", "--json", "body"},
		[]byte(fmt.Sprintf(`{"body":%s}`, rawBody)),
		nil,
	)
	prompter := &fakePrompter{} // no answers — must NOT be asked
	var stdout, stderr bytes.Buffer
	a := newActions(Deps{Gh: gh, Prompter: prompter, Stdout: &stdout, Stderr: &stderr, Autonomous: true})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "task")
	ctx.Set("issue_num", "61")
	if out := a.parseTicketBody(ctx); out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if got := ctx.Get("parse_ok"); got != true {
		t.Fatalf("parse_ok: got %v, want true (autonomous proceeds)", got)
	}
	if len(prompter.asked) != 0 {
		t.Fatalf("autonomous mode must not prompt; got asks: %q", prompter.asked)
	}
	if !strings.Contains(stderr.String(), "warning: all 2 checklist items are already marked [x]") {
		t.Fatalf("expected autonomous-mode warning on stderr:\n%s", stderr.String())
	}
}

func TestParseTicketBody_PartiallyCheckedDoesNotPrompt(t *testing.T) {
	body := "## Checklist\n\n- [x] One\n- [ ] Two\n"
	rawBody, _ := json.Marshal(body)
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "61", "--json", "body"},
		[]byte(fmt.Sprintf(`{"body":%s}`, rawBody)),
		nil,
	)
	prompter := &fakePrompter{} // no answers — must NOT be asked
	var stdout bytes.Buffer
	a := newActions(Deps{Gh: gh, Prompter: prompter, Stdout: &stdout})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "task")
	ctx.Set("issue_num", "61")
	if out := a.parseTicketBody(ctx); out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if len(prompter.asked) != 0 {
		t.Fatalf("partially-checked checklist must not prompt; got asks: %q", prompter.asked)
	}
	if got := ctx.Get("parse_ok"); got != true {
		t.Fatalf("parse_ok: got %v, want true", got)
	}
}

func TestParseTicketBody_StoryHasNoChecklistSummary(t *testing.T) {
	body := "## Acceptance Criteria\n\nScenario: ok\n"
	rawBody, _ := json.Marshal(body)
	gh := newFakeRunner(t, "gh")
	gh.on(
		[]string{"issue", "view", "1", "--json", "body"},
		[]byte(fmt.Sprintf(`{"body":%s}`, rawBody)),
		nil,
	)
	var stdout bytes.Buffer
	a := newActions(Deps{Gh: gh, Prompter: &fakePrompter{}, Stdout: &stdout})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "story")
	ctx.Set("issue_num", "1")
	if out := a.parseTicketBody(ctx); out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if strings.Contains(stdout.String(), "Checklist (") {
		t.Fatalf("story should not print checklist summary:\n%s", stdout.String())
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
		"read_ticket_type",
		"read_subtype",
		"parse_ticket_body",
		"report_intake_summary",
		"move_to_in_acceptance",
		"run_smoke_test",
		"commit_onboarding",
		"compile_in_scope",
		"run_sample_suite",
		"report_drift_warning",
		"ask_can_i_commit",
		"commit_phase",
		"tick_checklist",
		"verify_run_tests_after_driver",
		"compile_targeted",
		"run_targeted_tests",
		"disable_change_driven",
	}
	for _, name := range want {
		if r.Lookup(name) == nil {
			t.Errorf("action %q not registered", name)
		}
	}
}

// ---------------------------------------------------------------------------
// writeVerifySummary
// ---------------------------------------------------------------------------

func TestWriteVerifySummary_DriverAdapterChangedBlock(t *testing.T) {
	// Three changed adapter files across the three languages, all touching
	// a method that maps to the same DSL → test (the page-object case the
	// selector now bridges). The print order is alphabetical by file.
	res := testselect.Result{
		Changed: []testselect.ChangedMethod{
			{
				File:   "system-test/typescript/src/testkit/driver/adapter/myShop/ui/client/pages/NewOrderPage.ts",
				Method: "inputSku",
				Layer:  "shop",
				Lang:   "typescript",
			},
			{
				File:   "system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/myshop/ui/client/pages/NewOrderPage.java",
				Method: "inputSku",
				Layer:  "shop",
				Lang:   "java",
			},
			{
				File:   "system-test/dotnet/Driver.Adapter/MyShop/Ui/Client/Pages/NewOrderPage.cs",
				Method: "InputSku",
				Layer:  "shop",
				Lang:   "dotnet",
			},
		},
		Selections: []testselect.Selection{
			{Suite: "acceptance-ui", Tests: []string{"PlaceOrderPositiveTest.shouldPlaceOrder"}},
		},
	}

	var buf bytes.Buffer
	writeVerifySummary(&buf, res)
	got := buf.String()

	wantHeader := "Driver-adapter (3 file(s) changed):"
	if !strings.Contains(got, wantHeader) {
		t.Errorf("missing header %q in:\n%s", wantHeader, got)
	}
	wantLines := []string{
		"  - system-test/dotnet/Driver.Adapter/MyShop/Ui/Client/Pages/NewOrderPage.cs — InputSku",
		"  - system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/myshop/ui/client/pages/NewOrderPage.java — inputSku",
		"  - system-test/typescript/src/testkit/driver/adapter/myShop/ui/client/pages/NewOrderPage.ts — inputSku",
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want) {
			t.Errorf("missing line %q in:\n%s", want, got)
		}
	}
	// Ordering: the changed-block must appear before the selected-tests
	// block (which the operator reads next).
	driverIdx := strings.Index(got, wantHeader)
	selectedIdx := strings.Index(got, "Selected tests for verification")
	if driverIdx < 0 || selectedIdx < 0 || driverIdx > selectedIdx {
		t.Errorf("expected driver-adapter block before selected-tests block; got:\n%s", got)
	}
}

func TestWriteVerifySummary_MultipleMethodsPerFile(t *testing.T) {
	// Two methods edited in the same file → the line collapses them with
	// a comma-separated list, sorted and deduplicated.
	res := testselect.Result{
		Changed: []testselect.ChangedMethod{
			{File: "a/b.ts", Method: "beta", Lang: "typescript"},
			{File: "a/b.ts", Method: "alpha", Lang: "typescript"},
			{File: "a/b.ts", Method: "alpha", Lang: "typescript"}, // duplicate
		},
	}
	var buf bytes.Buffer
	writeVerifySummary(&buf, res)
	got := buf.String()

	wantLine := "  - a/b.ts — alpha, beta"
	if !strings.Contains(got, wantLine) {
		t.Errorf("missing line %q in:\n%s", wantLine, got)
	}
	if !strings.Contains(got, "Driver-adapter (1 file(s) changed):") {
		t.Errorf("expected single-file header, got:\n%s", got)
	}
}

func TestWriteVerifySummary_NoChangedFiles_OmitsBlock(t *testing.T) {
	// A degenerate result (no Changed entries) should not print the header
	// — the existing Selected/Unmapped output stays untouched.
	res := testselect.Result{
		Selections: []testselect.Selection{
			{Suite: "acceptance-api", Tests: []string{"X.y"}},
		},
	}
	var buf bytes.Buffer
	writeVerifySummary(&buf, res)
	got := buf.String()

	if strings.Contains(got, "Driver-adapter (") {
		t.Errorf("did not expect driver-adapter block, got:\n%s", got)
	}
	if !strings.Contains(got, "Selected tests for verification (1):") {
		t.Errorf("expected selected-tests block, got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// verifyRunTestsAfterDriver — prompt dispatch
// ---------------------------------------------------------------------------

// fakeVerifyDeps is a small bundle of canned selector outputs and a fake
// shell to capture the test-run commands. Used to drive the prompt
// dispatcher without touching git or testselect.
type fakeVerifyDeps struct {
	selectResult testselect.Result
	tracerResult testselect.TracerResult
	selectErr    error
	tracerErr    error
}

func (f *fakeVerifyDeps) Select(repoRoot, baseRef string) (testselect.Result, error) {
	return f.selectResult, f.selectErr
}

func (f *fakeVerifyDeps) SelectTracer(repoRoot, baseRef string) (testselect.TracerResult, error) {
	return f.tracerResult, f.tracerErr
}

func makeAffectedSet() testselect.Result {
	return testselect.Result{
		Changed: []testselect.ChangedMethod{
			{File: "system-test/typescript/src/testkit/driver/adapter/myShop/ui/client/pages/NewOrderPage.ts",
				Method: "inputSku", Layer: "shop", Lang: "typescript"},
		},
		Selections: []testselect.Selection{
			{Suite: "acceptance-ui", Tests: []string{"PlaceOrderPositiveTest.shouldPlaceOrder", "PlaceOrderNegativeTest.shouldRejectInvalidSku"}},
		},
	}
}

func makeTracer() testselect.TracerResult {
	return testselect.TracerResult{
		Changed: []testselect.ChangedMethod{
			{File: "system-test/typescript/src/testkit/driver/adapter/myShop/ui/client/pages/NewOrderPage.ts",
				Method: "inputSku", Layer: "shop", Lang: "typescript"},
		},
		Selections: []testselect.TracerSelection{
			{
				Suite:         "acceptance-ui",
				Test:          "PlaceOrderPositiveTest.shouldPlaceOrder",
				DSLMethod:     "whenPlacingOrder",
				PortMethod:    "placeOrder",
				AdapterFile:   "system-test/typescript/src/testkit/driver/adapter/myShop/ui/client/pages/NewOrderPage.ts",
				AdapterMethod: "inputSku",
				Stage:         "when",
			},
		},
	}
}

func TestVerifyRunTestsAfterDriver_EmptyInputRunsTracer(t *testing.T) {
	fv := &fakeVerifyDeps{
		selectResult: makeAffectedSet(),
		tracerResult: makeTracer(),
	}
	sh := &fakeShell{out: []byte("ok")}
	p := &fakePrompter{answers: []string{""}} // empty input → tracer default
	var stdout, stderr bytes.Buffer
	a := newActions(Deps{
		Shell:        sh,
		Prompter:     p,
		Stdout:       &stdout,
		Stderr:       &stderr,
		Select:       fv.Select,
		SelectTracer: fv.SelectTracer,
	})
	out := a.verifyRunTestsAfterDriver(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if len(sh.calls) != 1 {
		t.Fatalf("expected 1 shell call (tracer test), got %d: %v", len(sh.calls), sh.calls)
	}
	want := "gh optivem test system --suite acceptance-ui --test PlaceOrderPositiveTest.shouldPlaceOrder"
	if sh.calls[0] != want {
		t.Errorf("call: got %q want %q", sh.calls[0], want)
	}
	if !strings.Contains(stdout.String(), "Tracer selections (1)") {
		t.Errorf("stdout missing tracer summary header, got:\n%s", stdout.String())
	}
}

func TestVerifyRunTestsAfterDriver_TRunsTracer(t *testing.T) {
	fv := &fakeVerifyDeps{
		selectResult: makeAffectedSet(),
		tracerResult: makeTracer(),
	}
	sh := &fakeShell{out: []byte("ok")}
	p := &fakePrompter{answers: []string{"t"}}
	a := newActions(Deps{
		Shell:        sh,
		Prompter:     p,
		Select:       fv.Select,
		SelectTracer: fv.SelectTracer,
	})
	out := a.verifyRunTestsAfterDriver(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if len(sh.calls) != 1 {
		t.Fatalf("expected 1 shell call, got %d: %v", len(sh.calls), sh.calls)
	}
	if !strings.Contains(sh.calls[0], "--test PlaceOrderPositiveTest.shouldPlaceOrder") {
		t.Errorf("call: got %q (no --test flag)", sh.calls[0])
	}
}

func TestVerifyRunTestsAfterDriver_RRunsAffectedSet(t *testing.T) {
	fv := &fakeVerifyDeps{
		selectResult: makeAffectedSet(),
		tracerResult: makeTracer(),
	}
	sh := &fakeShell{out: []byte("ok")}
	p := &fakePrompter{answers: []string{"r"}}
	a := newActions(Deps{
		Shell:        sh,
		Prompter:     p,
		Select:       fv.Select,
		SelectTracer: fv.SelectTracer,
	})
	out := a.verifyRunTestsAfterDriver(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	// Two affected-set tests → two shell calls, both --test flagged.
	if len(sh.calls) != 2 {
		t.Fatalf("expected 2 shell calls, got %d: %v", len(sh.calls), sh.calls)
	}
	for _, c := range sh.calls {
		if !strings.Contains(c, "--test ") {
			t.Errorf("call: %q has no --test flag (should be per-test, not full-suite)", c)
		}
	}
}

func TestVerifyRunTestsAfterDriver_TracerUnmapped_FallsBackToFullSuite(t *testing.T) {
	tracer := makeTracer()
	tracer.Unmapped = []testselect.ChangedMethod{
		{File: "some/orphan/path.ts", Method: "doStuff", Layer: "shop", Lang: "typescript"},
	}
	fv := &fakeVerifyDeps{
		selectResult: makeAffectedSet(),
		tracerResult: tracer,
	}
	sh := &fakeShell{out: []byte("ok")}
	p := &fakePrompter{answers: []string{"t"}}
	var stderr bytes.Buffer
	a := newActions(Deps{
		Shell:        sh,
		Prompter:     p,
		Stderr:       &stderr,
		Select:       fv.Select,
		SelectTracer: fv.SelectTracer,
	})
	out := a.verifyRunTestsAfterDriver(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	// Fallback path runs each suite as a whole — one shell call (no --test
	// flag), since the affected set has one suite.
	if len(sh.calls) != 1 {
		t.Fatalf("expected 1 full-suite call, got %d: %v", len(sh.calls), sh.calls)
	}
	if strings.Contains(sh.calls[0], "--test") {
		t.Errorf("expected full-suite call (no --test), got %q", sh.calls[0])
	}
	if !strings.Contains(sh.calls[0], "--suite acceptance-ui") {
		t.Errorf("expected --suite acceptance-ui, got %q", sh.calls[0])
	}
	if !strings.Contains(stderr.String(), "tracer could not stage") {
		t.Errorf("expected tracer warning in stderr, got:\n%s", stderr.String())
	}
}

func TestVerifyRunTestsAfterDriver_FRunsFullSuite(t *testing.T) {
	fv := &fakeVerifyDeps{
		selectResult: makeAffectedSet(),
		tracerResult: makeTracer(),
	}
	sh := &fakeShell{out: []byte("ok")}
	p := &fakePrompter{answers: []string{"f"}}
	a := newActions(Deps{
		Shell:        sh,
		Prompter:     p,
		Select:       fv.Select,
		SelectTracer: fv.SelectTracer,
	})
	out := a.verifyRunTestsAfterDriver(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if len(sh.calls) != 1 {
		t.Fatalf("expected 1 full-suite call, got %d: %v", len(sh.calls), sh.calls)
	}
	if strings.Contains(sh.calls[0], "--test") {
		t.Errorf("expected no --test on full-suite call, got %q", sh.calls[0])
	}
}

func TestVerifyRunTestsAfterDriver_AApprovesWithoutRunning(t *testing.T) {
	fv := &fakeVerifyDeps{
		selectResult: makeAffectedSet(),
		tracerResult: makeTracer(),
	}
	sh := &fakeShell{}
	p := &fakePrompter{answers: []string{"a"}}
	a := newActions(Deps{
		Shell:        sh,
		Prompter:     p,
		Select:       fv.Select,
		SelectTracer: fv.SelectTracer,
	})
	out := a.verifyRunTestsAfterDriver(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if len(sh.calls) != 0 {
		t.Errorf("expected no shell calls on approve, got %v", sh.calls)
	}
}

func TestVerifyRunTestsAfterDriver_XRejects(t *testing.T) {
	fv := &fakeVerifyDeps{
		selectResult: makeAffectedSet(),
		tracerResult: makeTracer(),
	}
	sh := &fakeShell{}
	p := &fakePrompter{answers: []string{"x"}}
	a := newActions(Deps{
		Shell:        sh,
		Prompter:     p,
		Select:       fv.Select,
		SelectTracer: fv.SelectTracer,
	})
	out := a.verifyRunTestsAfterDriver(statemachine.NewContext())
	if out.Err == nil {
		t.Fatalf("expected error on reject")
	}
	if len(sh.calls) != 0 {
		t.Errorf("expected no shell calls on reject, got %v", sh.calls)
	}
}

func TestWriteTracerSummary_ChainShape(t *testing.T) {
	tracer := makeTracer()
	var buf bytes.Buffer
	writeTracerSummary(&buf, tracer)
	got := buf.String()
	for _, want := range []string{
		"Tracer selections (1):",
		"inputSku (system-test/typescript/src/testkit/driver/adapter/myShop/ui/client/pages/NewOrderPage.ts)",
		"→ port placeOrder",
		"→ DSL whenPlacingOrder (when)",
		"→ test PlaceOrderPositiveTest.shouldPlaceOrder (acceptance-ui)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q\nfull:\n%s", want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// verifyRunTestsAfterDriver — class plumbing
//
// The verify action stamps Outcome.Value and ctx.State with one of {ok,
// red, infra} so the trace banner (Item 6) and the structural-cycle
// gateway (Item 3 of the verify-failure-dispatch plan) can route on the
// classification rather than re-parsing the inline output.
// ---------------------------------------------------------------------------

func TestVerifyRunTestsAfterDriver_StampsOKWhenAllSucceed(t *testing.T) {
	fv := &fakeVerifyDeps{
		selectResult: makeAffectedSet(),
		tracerResult: makeTracer(),
	}
	sh := &fakeShell{out: []byte("ok")} // err=nil → classOK
	p := &fakePrompter{answers: []string{""}}
	ctx := statemachine.NewContext()
	a := newActions(Deps{
		Shell:        sh,
		Prompter:     p,
		Select:       fv.Select,
		SelectTracer: fv.SelectTracer,
	})
	out := a.verifyRunTestsAfterDriver(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if out.Value != "ok" {
		t.Errorf("Outcome.Value: got %q, want %q", out.Value, "ok")
	}
	if got := ctx.GetString("verify_class"); got != "ok" {
		t.Errorf("ctx verify_class: got %q, want %q", got, "ok")
	}
	results, _ := ctx.Get("verify_results").([]verifyCommandResult)
	if len(results) != 1 || results[0].Class != classOK {
		t.Errorf("ctx verify_results: got %#v, want one classOK result", results)
	}
}

func TestVerifyRunTestsAfterDriver_StampsRedOnTestFailure(t *testing.T) {
	fv := &fakeVerifyDeps{
		selectResult: makeAffectedSet(),
		tracerResult: makeTracer(),
	}
	// Non-nil err with neutral output → no infra pattern → classRed.
	sh := &fakeShell{out: []byte("--- FAIL: TestThing\n"), err: errors.New("exit status 1")}
	p := &fakePrompter{answers: []string{""}}
	ctx := statemachine.NewContext()
	a := newActions(Deps{
		Shell:        sh,
		Prompter:     p,
		Select:       fv.Select,
		SelectTracer: fv.SelectTracer,
	})
	out := a.verifyRunTestsAfterDriver(ctx)
	if out.Err != nil {
		t.Fatalf("expected no Outcome.Err (verify is feedback, not gating); got %v", out.Err)
	}
	if out.Value != "red" {
		t.Errorf("Outcome.Value: got %q, want %q", out.Value, "red")
	}
	if got := ctx.GetString("verify_class"); got != "red" {
		t.Errorf("ctx verify_class: got %q, want %q", got, "red")
	}
	// verify_results_text is the substitution body for the fix-verify
	// agent prompt's ${verify_results} placeholder. Must contain the
	// failed command and the runner's captured stdout/stderr so the
	// fix agent has the same signal the operator saw inline.
	resultsText := ctx.GetString("verify_results_text")
	for _, want := range []string{
		"gh optivem test system",
		"--- FAIL: TestThing",
		"Classification: red",
	} {
		if !strings.Contains(resultsText, want) {
			t.Errorf("verify_results_text missing %q\nfull:\n%s", want, resultsText)
		}
	}
}

func TestVerifyRunTestsAfterDriver_HaltsOnInfraWithDiagnostic(t *testing.T) {
	// Item 5 of the verify-failure-dispatch plan: an infra-class result
	// halts the run with Outcome.Err and prints the detailed banner so
	// the operator sees *which* runner-side problem fired (here the
	// missing-system-config / cwd-bug fingerprint), the command tried,
	// and the cwd. Without this halt the structural cycle silently
	// advanced into STOP_STRUCT_REVIEW with zero verify signal.
	fv := &fakeVerifyDeps{
		selectResult: makeAffectedSet(),
		tracerResult: makeTracer(),
	}
	// The exact stderr the user saw in the morning trace — should match
	// the "missing system config" infra row.
	sh := &fakeShell{
		out: []byte("ERROR: read system config ./system.json: open ./system.json: The system cannot find the file specified."),
		err: errors.New("exit status 1"),
	}
	p := &fakePrompter{answers: []string{""}}
	ctx := statemachine.NewContext()
	var stderr bytes.Buffer
	a := newActions(Deps{
		Shell:        sh,
		Prompter:     p,
		Stderr:       &stderr,
		RepoPath:     "/tmp/sandbox",
		Select:       fv.Select,
		SelectTracer: fv.SelectTracer,
	})
	out := a.verifyRunTestsAfterDriver(ctx)
	if out.Err == nil {
		t.Fatalf("expected Outcome.Err for infra-class halt; got nil")
	}
	if !strings.Contains(out.Err.Error(), "infra") {
		t.Errorf("Outcome.Err should mention infra; got: %v", out.Err)
	}
	// ctx.verify_class is still stamped — downstream gates / fix agents
	// (Items 3 & 4) can read the class even on the halt path.
	if got := ctx.GetString("verify_class"); got != "infra" {
		t.Errorf("ctx verify_class: got %q, want %q", got, "infra")
	}
	// Banner content: the matched label, the runner's leading line,
	// the command, the cwd, and the cross-link to the cwd-bug plan.
	se := stderr.String()
	for _, want := range []string{
		"runner failed before any test ran",
		"missing system config",
		"ERROR: read system config",
		"gh optivem test system",
		"Cwd:    /tmp/sandbox",
		"plans/20260505-220100-verify-runs-from-wrong-cwd.md",
	} {
		if !strings.Contains(se, want) {
			t.Errorf("infra halt banner missing %q\nfull stderr:\n%s", want, se)
		}
	}
}

func TestAggregateVerifyClass_InfraDominatesRedDominatesOK(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []failureClass
		want failureClass
	}{
		{name: "empty_is_ok", in: nil, want: classOK},
		{name: "all_ok", in: []failureClass{classOK, classOK}, want: classOK},
		{name: "any_red", in: []failureClass{classOK, classRed, classOK}, want: classRed},
		{name: "infra_dominates_red", in: []failureClass{classRed, classInfra, classOK}, want: classInfra},
		{name: "infra_alone", in: []failureClass{classInfra}, want: classInfra},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results := make([]verifyCommandResult, len(tc.in))
			for i, c := range tc.in {
				results[i] = verifyCommandResult{Class: c}
			}
			if got := aggregateVerifyClass(results); got != tc.want {
				t.Errorf("aggregateVerifyClass(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestVerifyRunTestsAfterDriver_ApprovePathDoesNotStampValue(t *testing.T) {
	// "a" choice runs no commands; nothing was verified, so the trace
	// should keep its honest "(no result)" rendering rather than
	// claiming a class.
	fv := &fakeVerifyDeps{
		selectResult: makeAffectedSet(),
		tracerResult: makeTracer(),
	}
	sh := &fakeShell{}
	p := &fakePrompter{answers: []string{"a"}}
	ctx := statemachine.NewContext()
	a := newActions(Deps{
		Shell:        sh,
		Prompter:     p,
		Select:       fv.Select,
		SelectTracer: fv.SelectTracer,
	})
	out := a.verifyRunTestsAfterDriver(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if out.Value != "" {
		t.Errorf("Outcome.Value: got %q, want empty (no commands ran)", out.Value)
	}
	if _, ok := ctx.State["verify_class"]; ok {
		t.Errorf("ctx.verify_class set on approve-without-running")
	}
}

// ---------------------------------------------------------------------------
// red_phase_cycle infrastructure (Step 1 of the AT/CT split)
// ---------------------------------------------------------------------------

// scriptedShell returns each scripted response in order. Used by the
// red_phase_cycle action tests where one action issues multiple shell-outs
// (one per test name / disable target) and we want to assert the exact
// command sequence and surface different fail/succeed outcomes per call.
type scriptedShell struct {
	t        *testing.T
	calls    []string
	scripted []scriptedResponse
}

type scriptedResponse struct {
	out []byte
	err error
}

func (s *scriptedShell) Run(_ context.Context, cmd string) ([]byte, error) {
	s.calls = append(s.calls, cmd)
	if len(s.scripted) == 0 {
		s.t.Fatalf("scriptedShell: unexpected call %q (no responses left)", cmd)
		return nil, errors.New("unreachable")
	}
	r := s.scripted[0]
	s.scripted = s.scripted[1:]
	return r.out, r.err
}

func TestCompileTargeted_ScopeRequired(t *testing.T) {
	a := newActions(Deps{})
	out := a.compileTargeted(statemachine.NewContext())
	if out.Err == nil {
		t.Fatalf("expected error when scope is missing")
	}
	if !strings.Contains(out.Err.Error(), "scope") {
		t.Fatalf("error message does not mention scope: %v", out.Err)
	}
}

func TestCompileTargeted_PassWritesContextAndDoesNotErr(t *testing.T) {
	sh := &fakeShell{out: []byte("BUILD SUCCESSFUL")}
	a := newActions(Deps{Shell: sh})
	ctx := statemachine.NewContext()
	ctx.Set("scope", "shop/api")
	out := a.compileTargeted(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !out.Bool {
		t.Fatalf("Bool: got false, want true")
	}
	if got := ctx.Get("compile_ok"); got != true {
		t.Fatalf("compile_ok: got %v, want true", got)
	}
	if len(sh.calls) != 1 || sh.calls[0] != "./compile-targeted.sh shop/api" {
		t.Fatalf("unexpected shell calls: %v", sh.calls)
	}
}

func TestCompileTargeted_ShellEscapesScope(t *testing.T) {
	sh := &fakeShell{}
	a := newActions(Deps{Shell: sh})
	ctx := statemachine.NewContext()
	ctx.Set("scope", "path with spaces/x")
	_ = a.compileTargeted(ctx)
	if len(sh.calls) != 1 || sh.calls[0] != "./compile-targeted.sh 'path with spaces/x'" {
		t.Fatalf("unexpected shell call (no escaping?): %v", sh.calls)
	}
}

func TestCompileTargeted_FailureRoutesNotErrors(t *testing.T) {
	sh := &fakeShell{out: []byte("compilation failed"), err: errors.New("exit 1")}
	a := newActions(Deps{Shell: sh})
	ctx := statemachine.NewContext()
	ctx.Set("scope", "shop/api")
	out := a.compileTargeted(ctx)
	// Compile failure must NOT surface as Err — the gate routes the
	// compile-failed loop and the run continues.
	if out.Err != nil {
		t.Fatalf("compile failure should route, not halt: %v", out.Err)
	}
	if out.Bool {
		t.Fatalf("Bool: got true, want false")
	}
	if got := ctx.Get("compile_ok"); got != false {
		t.Fatalf("compile_ok: got %v, want false", got)
	}
}

func TestRunTargetedTests_RequiresSuiteAndTestNames(t *testing.T) {
	t.Run("no_suite", func(t *testing.T) {
		a := newActions(Deps{})
		ctx := statemachine.NewContext()
		ctx.State["test_names"] = []string{"x"}
		out := a.runTargetedTests(ctx)
		if out.Err == nil || !strings.Contains(out.Err.Error(), "suite") {
			t.Fatalf("expected suite error, got %v", out.Err)
		}
	})
	t.Run("no_test_names", func(t *testing.T) {
		a := newActions(Deps{})
		ctx := statemachine.NewContext()
		ctx.Set("suite", "<acceptance-api>")
		out := a.runTargetedTests(ctx)
		if out.Err == nil || !strings.Contains(out.Err.Error(), "test_names") {
			t.Fatalf("expected test_names error, got %v", out.Err)
		}
	})
	t.Run("test_names_wrong_type", func(t *testing.T) {
		a := newActions(Deps{})
		ctx := statemachine.NewContext()
		ctx.Set("suite", "<acceptance-api>")
		ctx.State["test_names"] = "not-a-slice"
		out := a.runTargetedTests(ctx)
		if out.Err == nil || !strings.Contains(out.Err.Error(), "[]string") {
			t.Fatalf("expected []string error, got %v", out.Err)
		}
	})
	t.Run("test_names_empty", func(t *testing.T) {
		a := newActions(Deps{})
		ctx := statemachine.NewContext()
		ctx.Set("suite", "<acceptance-api>")
		ctx.State["test_names"] = []string{}
		out := a.runTargetedTests(ctx)
		if out.Err == nil || !strings.Contains(out.Err.Error(), "empty") {
			t.Fatalf("expected empty error, got %v", out.Err)
		}
	})
}

func TestRunTargetedTests_AllRuntimeFailuresWritesTrue(t *testing.T) {
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("AssertionError: expected 1 was 0"), err: errors.New("exit 1")},
		{out: []byte("AssertionError: expected 2 was 0"), err: errors.New("exit 1")},
	}}
	a := newActions(Deps{Shell: sh})
	ctx := statemachine.NewContext()
	ctx.Set("suite", "<acceptance-api>")
	ctx.State["test_names"] = []string{"shouldFooSucceed", "shouldBarSucceed"}
	out := a.runTargetedTests(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !out.Bool {
		t.Fatalf("Bool: got false, want true")
	}
	if got := ctx.Get("tests_failed_runtime"); got != true {
		t.Fatalf("tests_failed_runtime: got %v, want true", got)
	}
	want := []string{
		"gh optivem test system --suite '<acceptance-api>' --test shouldFooSucceed",
		"gh optivem test system --suite '<acceptance-api>' --test shouldBarSucceed",
	}
	if len(sh.calls) != len(want) {
		t.Fatalf("got %d calls, want %d: %v", len(sh.calls), len(want), sh.calls)
	}
	for i, w := range want {
		if sh.calls[i] != w {
			t.Errorf("call[%d]: got %q, want %q", i, sh.calls[i], w)
		}
	}
}

func TestRunTargetedTests_CompileFailureWritesFalse(t *testing.T) {
	for _, marker := range []string{
		"compilation failed",
		"cannot find symbol method foo()",
		"error CS0103: 'whenPlacingOrder' does not exist",
		"error TS2304: Cannot find name 'register'",
		"syntax error",
	} {
		t.Run(marker, func(t *testing.T) {
			sh := &scriptedShell{t: t, scripted: []scriptedResponse{
				{out: []byte(marker), err: errors.New("exit 1")},
			}}
			a := newActions(Deps{Shell: sh})
			ctx := statemachine.NewContext()
			ctx.Set("suite", "<acceptance-api>")
			ctx.State["test_names"] = []string{"x"}
			out := a.runTargetedTests(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected error: %v", out.Err)
			}
			if out.Bool {
				t.Fatalf("Bool: got true, want false (compile failure must demote)")
			}
			if got := ctx.Get("tests_failed_runtime"); got != false {
				t.Fatalf("tests_failed_runtime: got %v, want false", got)
			}
		})
	}
}

func TestRunTargetedTests_AllPassesWritesFalse(t *testing.T) {
	// "All tests pass" is also a not-yet-RED state — runtime failure was
	// not observed so we cannot route to DISABLE.
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("OK"), err: nil},
	}}
	a := newActions(Deps{Shell: sh})
	ctx := statemachine.NewContext()
	ctx.Set("suite", "<acceptance-api>")
	ctx.State["test_names"] = []string{"x"}
	out := a.runTargetedTests(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if out.Bool {
		t.Fatalf("Bool: got true, want false")
	}
}

func TestRunTargetedTests_MixedRuntimeAndCompileWritesFalse(t *testing.T) {
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("AssertionError"), err: errors.New("exit 1")},
		{out: []byte("compilation failed"), err: errors.New("exit 1")},
	}}
	a := newActions(Deps{Shell: sh})
	ctx := statemachine.NewContext()
	ctx.Set("suite", "<acceptance-api>")
	ctx.State["test_names"] = []string{"a", "b"}
	out := a.runTargetedTests(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if out.Bool {
		t.Fatalf("Bool: got true, want false (any compile failure must demote)")
	}
}

func TestDisableChangeDriven_RequiresAllInputs(t *testing.T) {
	for _, tc := range []struct {
		name        string
		setup       func(*statemachine.Context)
		wantSubstr  string
	}{
		{
			name:       "no_language",
			setup:      func(ctx *statemachine.Context) {},
			wantSubstr: "language",
		},
		{
			name: "no_reason",
			setup: func(ctx *statemachine.Context) {
				ctx.Set("language", "java")
			},
			wantSubstr: "disable_reason",
		},
		{
			name: "no_targets",
			setup: func(ctx *statemachine.Context) {
				ctx.Set("language", "java")
				ctx.Set("disable_reason", "AT - RED - TEST")
			},
			wantSubstr: "disable_targets",
		},
		{
			name: "targets_wrong_type",
			setup: func(ctx *statemachine.Context) {
				ctx.Set("language", "java")
				ctx.Set("disable_reason", "AT - RED - TEST")
				ctx.State["disable_targets"] = "not-a-slice"
			},
			wantSubstr: "[]string",
		},
		{
			name: "targets_empty",
			setup: func(ctx *statemachine.Context) {
				ctx.Set("language", "java")
				ctx.Set("disable_reason", "AT - RED - TEST")
				ctx.State["disable_targets"] = []string{}
			},
			wantSubstr: "empty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newActions(Deps{})
			ctx := statemachine.NewContext()
			tc.setup(ctx)
			out := a.disableChangeDriven(ctx)
			if out.Err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(out.Err.Error(), tc.wantSubstr) {
				t.Fatalf("error %v does not mention %q", out.Err, tc.wantSubstr)
			}
		})
	}
}

func TestDisableChangeDriven_ShellsOncePerTarget(t *testing.T) {
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("ok")},
		{out: []byte("ok")},
	}}
	a := newActions(Deps{Shell: sh})
	ctx := statemachine.NewContext()
	ctx.Set("language", "java")
	ctx.Set("disable_reason", "AT - RED - TEST")
	ctx.State["disable_targets"] = []string{
		"src/test/java/A.java:shouldFooSucceed",
		"src/test/java/B.java:shouldBarSucceed",
	}
	out := a.disableChangeDriven(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	want := []string{
		"./disable-test.sh java 'AT - RED - TEST' src/test/java/A.java:shouldFooSucceed",
		"./disable-test.sh java 'AT - RED - TEST' src/test/java/B.java:shouldBarSucceed",
	}
	if len(sh.calls) != len(want) {
		t.Fatalf("got %d calls, want %d: %v", len(sh.calls), len(want), sh.calls)
	}
	for i, w := range want {
		if sh.calls[i] != w {
			t.Errorf("call[%d]: got %q, want %q", i, sh.calls[i], w)
		}
	}
}

func TestDisableChangeDriven_FirstFailureHaltsRun(t *testing.T) {
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("could not parse method"), err: errors.New("exit 2")},
	}}
	a := newActions(Deps{Shell: sh})
	ctx := statemachine.NewContext()
	ctx.Set("language", "csharp")
	ctx.Set("disable_reason", "CT - RED - TEST")
	ctx.State["disable_targets"] = []string{
		"Tests/A.cs:Should_Fail_When",
		"Tests/B.cs:Should_Other_Fail",
	}
	out := a.disableChangeDriven(ctx)
	if out.Err == nil {
		t.Fatalf("expected error on first-target failure")
	}
	if !strings.Contains(out.Err.Error(), "Tests/A.cs:Should_Fail_When") {
		t.Fatalf("error should name the failing target: %v", out.Err)
	}
	if len(sh.calls) != 1 {
		t.Fatalf("got %d calls, want 1 (must halt on first failure): %v", len(sh.calls), sh.calls)
	}
}

