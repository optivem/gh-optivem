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
	"path/filepath"
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
		"ask_can_i_commit",
		"commit_phase",
		"tick_checklist",
		"select_tests",
		"run_tests",
		"compile_targeted",
		"run_targeted_tests",
		"disable_change_driven",
		"enable_change_driven",
		"verify_real_suite_passes",
	}
	for _, name := range want {
		if r.Lookup(name) == nil {
			t.Errorf("action %q not registered", name)
		}
	}
}

// ---------------------------------------------------------------------------
// runTests — tiered prompt
//
// Top-level menu: a/s/p/n/x. For s and p the action shells out to
// `gh optivem test system --list` to fetch the suite catalogue, then asks
// the operator which numbered suites (and, for p, which test names) to run.
// On a green run the loop offers another picker pass; on red it exits so
// the structural-cycle gateway can dispatch the fix agent.
// ---------------------------------------------------------------------------

const suiteListOut = "acceptance-api\nacceptance-ui\n"

// makeListResponse is the canned `--list` reply scriptedShell tests expect
// before any test-run command — the action prints its menu off this output.
func makeListResponse() scriptedResponse {
	return scriptedResponse{out: []byte(suiteListOut)}
}

// setupVerifyRepoLayout builds a minimal flat-layout fake repo under
// t.TempDir() with the two files ResolveSystemTestPaths probes for, and
// returns the repo root + the path-flag suffix that verifyPathFlags will
// produce. Tests pass the root via Deps.RepoPath and use the suffix to
// construct expected command strings (matches what the production code
// will emit, including spaces and quoting).
func setupVerifyRepoLayout(t *testing.T) (root, pathFlagsSuffix string) {
	t.Helper()
	root = t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docker", "system.json"), "{}")
	mustWriteFile(t, filepath.Join(root, "system-test", "tests-latest.json"), "{}")
	sys := filepath.Join(root, "docker", "system.json")
	tests := filepath.Join(root, "system-test", "tests-latest.json")
	pathFlagsSuffix = fmt.Sprintf(" --system-config %s --test-config %s",
		shellEscape(sys), shellEscape(tests))
	return root, pathFlagsSuffix
}

func TestVerifyRunTestsAfterDriver_XRejects(t *testing.T) {
	sh := &scriptedShell{t: t}
	p := &fakePrompter{answers: []string{"x"}}
	a := newActions(Deps{Shell: sh, Prompter: p})
	out := a.runTests(statemachine.NewContext())
	if out.Err == nil {
		t.Fatalf("expected error on reject")
	}
	if len(sh.calls) != 0 {
		t.Errorf("expected no shell calls on reject, got %v", sh.calls)
	}
}

func TestVerifyRunTestsAfterDriver_NSkipsWithoutRunning(t *testing.T) {
	sh := &scriptedShell{t: t}
	p := &fakePrompter{answers: []string{"n"}}
	ctx := statemachine.NewContext()
	a := newActions(Deps{Shell: sh, Prompter: p})
	out := a.runTests(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if len(sh.calls) != 0 {
		t.Errorf("expected no shell calls on skip, got %v", sh.calls)
	}
	if out.Value != "" {
		t.Errorf("Outcome.Value: got %q, want empty", out.Value)
	}
	if _, ok := ctx.State["verify_class"]; ok {
		t.Errorf("ctx.verify_class set on skip")
	}
}

func TestVerifyRunTestsAfterDriver_ARunsAllSystemTests(t *testing.T) {
	root, flags := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{{out: []byte("PASS")}}}
	p := &fakePrompter{answers: []string{"a", "n"}} // [a]ll, then no-more-tests
	ctx := statemachine.NewContext()
	a := newActions(Deps{Shell: sh, Prompter: p, RepoPath: root})
	out := a.runTests(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	wantCall := "gh optivem test system" + flags
	if len(sh.calls) != 1 || sh.calls[0] != wantCall {
		t.Fatalf("expected single `%s` call, got %v", wantCall, sh.calls)
	}
	if out.Value != "ok" {
		t.Errorf("Outcome.Value: got %q, want ok", out.Value)
	}
}

func TestVerifyRunTestsAfterDriver_SRunsPickedSuites(t *testing.T) {
	// [s]ome suites → --list yields two suites, the user picks 1,2 → two
	// `--suite <id>` runs in pick order. Each shell-out gets the resolved
	// `--system-config` / `--test-config` flags appended.
	root, flags := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		makeListResponse(),
		{out: []byte("PASS")},
		{out: []byte("PASS")},
	}}
	p := &fakePrompter{answers: []string{"s", "1,2", "n"}}
	a := newActions(Deps{Shell: sh, Prompter: p, RepoPath: root})
	out := a.runTests(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	wantCalls := []string{
		"gh optivem test system --list" + flags,
		"gh optivem test system --suite acceptance-api" + flags,
		"gh optivem test system --suite acceptance-ui" + flags,
	}
	if len(sh.calls) != len(wantCalls) {
		t.Fatalf("calls: got %v, want %v", sh.calls, wantCalls)
	}
	for i, want := range wantCalls {
		if sh.calls[i] != want {
			t.Errorf("call[%d]: got %q, want %q", i, sh.calls[i], want)
		}
	}
}

func TestVerifyRunTestsAfterDriver_PRunsSpecificTests(t *testing.T) {
	// [p]ick → --list, pick suite 2, type "T1, T2" → one combined run. The
	// path flags suffix every shell-out (the runner needs them to find
	// system.json / tests.json under the resolved layout).
	root, flags := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		makeListResponse(),
		{out: []byte("PASS")},
	}}
	p := &fakePrompter{answers: []string{"p", "2", "T1, T2", "n"}}
	a := newActions(Deps{Shell: sh, Prompter: p, RepoPath: root})
	out := a.runTests(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	wantCalls := []string{
		"gh optivem test system --list" + flags,
		"gh optivem test system --suite acceptance-ui --test T1 --test T2" + flags,
	}
	if len(sh.calls) != len(wantCalls) {
		t.Fatalf("calls: got %v, want %v", sh.calls, wantCalls)
	}
	for i, want := range wantCalls {
		if sh.calls[i] != want {
			t.Errorf("call[%d]: got %q, want %q", i, sh.calls[i], want)
		}
	}
}

func TestVerifyRunTestsAfterDriver_LoopsOnGreen(t *testing.T) {
	// First [a]ll → green → user says "y" to "Run more?" → second [a]ll →
	// green → "n" exits. Two test-system calls observed.
	root, _ := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("PASS")},
		{out: []byte("PASS")},
	}}
	p := &fakePrompter{answers: []string{"a", "y", "a", "n"}}
	a := newActions(Deps{Shell: sh, Prompter: p, RepoPath: root})
	out := a.runTests(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if len(sh.calls) != 2 {
		t.Fatalf("expected two test-system calls, got %d: %v", len(sh.calls), sh.calls)
	}
	// The second-prompt sequence: top-menu, more?, top-menu, more?.
	if got := len(p.asked); got != 4 {
		t.Errorf("prompt count: got %d, want 4 (top, more?, top, more?)", got)
	}
}

func TestVerifyRunTestsAfterDriver_ExitsLoopOnRed(t *testing.T) {
	// [a]ll → red → action returns immediately, the "Run more?" prompt is
	// never asked (gateway will dispatch fix agent after a human STOP).
	root, _ := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("--- FAIL: TestThing\n"), err: errors.New("exit status 1")},
	}}
	p := &fakePrompter{answers: []string{"a"}}
	ctx := statemachine.NewContext()
	a := newActions(Deps{Shell: sh, Prompter: p, RepoPath: root})
	out := a.runTests(ctx)
	if out.Err != nil {
		t.Fatalf("verify is feedback, not gating; got Err: %v", out.Err)
	}
	if out.Value != "red" {
		t.Errorf("Outcome.Value: got %q, want red", out.Value)
	}
	if got := ctx.GetString("verify_class"); got != "red" {
		t.Errorf("ctx verify_class: got %q, want red", got)
	}
	if len(p.asked) != 1 {
		t.Errorf("expected exactly the top-level prompt; asked: %v", p.asked)
	}
}

func TestVerifyRunTestsAfterDriver_UnknownChoiceReprompts(t *testing.T) {
	sh := &scriptedShell{t: t}
	p := &fakePrompter{answers: []string{"q", "n"}}
	a := newActions(Deps{Shell: sh, Prompter: p, Stderr: &bytes.Buffer{}})
	out := a.runTests(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if len(sh.calls) != 0 {
		t.Errorf("expected no shell calls, got %v", sh.calls)
	}
}

func TestParsePickNumbers(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		max  int
		want []int
		err  bool
	}{
		{name: "single", in: "2", max: 3, want: []int{2}},
		{name: "list", in: "1,3", max: 3, want: []int{1, 3}},
		{name: "spaces", in: " 1 , 2 ", max: 3, want: []int{1, 2}},
		{name: "dedupes", in: "1,2,2,1", max: 3, want: []int{1, 2}},
		{name: "empty", in: "", max: 3, want: nil},
		{name: "bad_token", in: "1,foo", max: 3, err: true},
		{name: "out_of_range", in: "4", max: 3, err: true},
		{name: "zero", in: "0", max: 3, err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePickNumbers(tc.in, tc.max)
			if tc.err {
				if err == nil {
					t.Fatalf("want error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// runTests — class plumbing
//
// The verify action stamps Outcome.Value and ctx.State with one of {ok,
// red, infra} so the trace banner and the structural-cycle gateway can
// route on the classification rather than re-parsing the inline output.
// ---------------------------------------------------------------------------

func TestVerifyRunTestsAfterDriver_StampsOKWhenAllSucceed(t *testing.T) {
	root, _ := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{{out: []byte("ok")}}}
	p := &fakePrompter{answers: []string{"a", "n"}}
	ctx := statemachine.NewContext()
	a := newActions(Deps{Shell: sh, Prompter: p, RepoPath: root})
	out := a.runTests(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if out.Value != "ok" {
		t.Errorf("Outcome.Value: got %q, want ok", out.Value)
	}
	if got := ctx.GetString("verify_class"); got != "ok" {
		t.Errorf("ctx verify_class: got %q, want ok", got)
	}
	results, _ := ctx.Get("verify_results").([]verifyCommandResult)
	if len(results) != 1 || results[0].Class != classOK {
		t.Errorf("ctx verify_results: got %#v, want one classOK result", results)
	}
}

func TestVerifyRunTestsAfterDriver_StampsRedOnTestFailure(t *testing.T) {
	// Non-nil err with neutral output → no infra pattern → classRed.
	root, _ := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("--- FAIL: TestThing\n"), err: errors.New("exit status 1")},
	}}
	p := &fakePrompter{answers: []string{"a"}} // red exits the loop, no "more?"
	ctx := statemachine.NewContext()
	a := newActions(Deps{Shell: sh, Prompter: p, RepoPath: root})
	out := a.runTests(ctx)
	if out.Err != nil {
		t.Fatalf("expected no Outcome.Err (verify is feedback, not gating); got %v", out.Err)
	}
	if out.Value != "red" {
		t.Errorf("Outcome.Value: got %q, want red", out.Value)
	}
	if got := ctx.GetString("verify_class"); got != "red" {
		t.Errorf("ctx verify_class: got %q, want red", got)
	}
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
	// Infra-class result halts with Outcome.Err and prints the detailed
	// banner so the operator sees *which* runner-side problem fired and
	// the command tried. Without this halt the structural cycle would
	// silently advance into APPROVE_STRUCTURAL_CHANGE with zero verify signal.
	//
	// This case simulates the runner reaching out and failing for reasons
	// the path resolution can't catch — e.g. missing toolchain, docker
	// daemon down. We seed the layout so resolution succeeds and the
	// shell-out actually runs, then the runner-side error fingerprints
	// `missing executable`.
	root, _ := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{
			out: []byte(`exec: "node": executable file not found in $PATH`),
			err: errors.New("exit status 1"),
		},
	}}
	p := &fakePrompter{answers: []string{"a"}}
	ctx := statemachine.NewContext()
	var stderr bytes.Buffer
	a := newActions(Deps{
		Shell:    sh,
		Prompter: p,
		Stderr:   &stderr,
		RepoPath: root,
	})
	out := a.runTests(ctx)
	if out.Err == nil {
		t.Fatalf("expected Outcome.Err for infra-class halt; got nil")
	}
	if !strings.Contains(out.Err.Error(), "infra") {
		t.Errorf("Outcome.Err should mention infra; got: %v", out.Err)
	}
	if got := ctx.GetString("verify_class"); got != "infra" {
		t.Errorf("ctx verify_class: got %q, want %q", got, "infra")
	}
	se := stderr.String()
	for _, want := range []string{
		"runner failed before any test ran",
		"missing executable",
		"executable file not found",
		"gh optivem test system",
		"Cwd:    " + root,
	} {
		if !strings.Contains(se, want) {
			t.Errorf("infra halt banner missing %q\nfull stderr:\n%s", want, se)
		}
	}
}

// TestVerifyRunTestsAfterDriver_HaltsOnUnresolvablePaths covers the case
// where ResolveSystemTestPaths returns an error — neither flat nor
// templated layout matches under RepoPath. The action halts with an
// infra-class diagnostic *before* shelling out, because running with the
// runner's broken `./system.json` defaults would only manufacture noise.
func TestVerifyRunTestsAfterDriver_HaltsOnUnresolvablePaths(t *testing.T) {
	root := t.TempDir() // empty — no docker/, no system-test/
	sh := &scriptedShell{t: t}
	p := &fakePrompter{answers: []string{"a"}}
	ctx := statemachine.NewContext()
	var stderr bytes.Buffer
	a := newActions(Deps{
		Shell:    sh,
		Prompter: p,
		Stderr:   &stderr,
		RepoPath: root,
	})
	out := a.runTests(ctx)
	if out.Err == nil {
		t.Fatalf("expected Outcome.Err when paths cannot be resolved")
	}
	if got := ctx.GetString("verify_class"); got != "infra" {
		t.Errorf("ctx verify_class: got %q, want %q", got, "infra")
	}
	if len(sh.calls) != 0 {
		t.Errorf("expected no shell calls when path resolution fails; got %v", sh.calls)
	}
	if !strings.Contains(stderr.String(), "verify path resolution failed") {
		t.Errorf("expected 'verify path resolution failed' banner; stderr=\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "20260505-220100-verify-runs-from-wrong-cwd") {
		t.Errorf("banner should reference the cwd-bug plan; stderr=\n%s", stderr.String())
	}
}

// TestVerifyRunTestsAfterDriver_HaltsOnInfraWhenStderrIsInError covers
// the realShell shape: stdout is empty and the runner's error line is
// folded into err.Error() (`shell %q: <inner> (stderr: <captured>)`).
// Earlier code passed only string(out) to classifyShellErr, blinding the
// infra-pattern table to that text and silently mis-routing infra
// failures as red. Path resolution succeeds here (the layout is set up),
// so the runner is actually invoked — and the runner-side error in err
// must still classify as infra.
func TestVerifyRunTestsAfterDriver_HaltsOnInfraWhenStderrIsInError(t *testing.T) {
	root, _ := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{
			// out is empty — exactly what cmd.Output() returns for a
			// runner that wrote only to stderr. The infra fingerprint
			// is reachable only via err.Error(). Use a docker-daemon
			// fingerprint here so the test exercises the err-text scan
			// path rather than retracing the path-resolution case
			// (which is covered by HaltsOnUnresolvablePaths above).
			out: nil,
			err: errors.New(`shell "gh optivem test system": exit status 1 (stderr: Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?)`),
		},
	}}
	p := &fakePrompter{answers: []string{"a"}}
	ctx := statemachine.NewContext()
	var stderr bytes.Buffer
	a := newActions(Deps{
		Shell:    sh,
		Prompter: p,
		Stderr:   &stderr,
		RepoPath: root,
	})
	out := a.runTests(ctx)
	if out.Err == nil {
		t.Fatalf("expected Outcome.Err for infra-class halt; got nil")
	}
	if got := ctx.GetString("verify_class"); got != "infra" {
		t.Errorf("ctx verify_class: got %q, want %q", got, "infra")
	}
	if !strings.Contains(stderr.String(), "docker daemon unreachable") {
		t.Errorf("infra banner should label fingerprint; stderr=\n%s", stderr.String())
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
	root, flags := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("AssertionError: expected 1 was 0"), err: errors.New("exit 1")},
		{out: []byte("AssertionError: expected 2 was 0"), err: errors.New("exit 1")},
	}}
	a := newActions(Deps{Shell: sh, RepoPath: root})
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
		"gh optivem test system --suite '<acceptance-api>' --test shouldFooSucceed" + flags,
		"gh optivem test system --suite '<acceptance-api>' --test shouldBarSucceed" + flags,
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
			root, _ := setupVerifyRepoLayout(t)
			sh := &scriptedShell{t: t, scripted: []scriptedResponse{
				{out: []byte(marker), err: errors.New("exit 1")},
			}}
			a := newActions(Deps{Shell: sh, RepoPath: root})
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
	root, _ := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("OK"), err: nil},
	}}
	a := newActions(Deps{Shell: sh, RepoPath: root})
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
	root, _ := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("AssertionError"), err: errors.New("exit 1")},
		{out: []byte("compilation failed"), err: errors.New("exit 1")},
	}}
	a := newActions(Deps{Shell: sh, RepoPath: root})
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

// ---------------------------------------------------------------------------
// enableChangeDriven — inverse of disableChangeDriven, used by AT GREEN to
// re-enable tests that the prior RED phase disabled.
// ---------------------------------------------------------------------------

func TestEnableChangeDriven_RequiresAllInputs(t *testing.T) {
	for _, tc := range []struct {
		name       string
		setup      func(*statemachine.Context)
		wantSubstr string
	}{
		{name: "no_language", setup: func(ctx *statemachine.Context) {}, wantSubstr: "language"},
		{name: "no_reason", setup: func(ctx *statemachine.Context) {
			ctx.Set("language", "java")
		}, wantSubstr: "disable_reason"},
		{name: "no_targets", setup: func(ctx *statemachine.Context) {
			ctx.Set("language", "java")
			ctx.Set("disable_reason", "AT - RED - SYSTEM DRIVER")
		}, wantSubstr: "disable_targets"},
		{name: "targets_wrong_type", setup: func(ctx *statemachine.Context) {
			ctx.Set("language", "java")
			ctx.Set("disable_reason", "AT - RED - SYSTEM DRIVER")
			ctx.State["disable_targets"] = "not-a-slice"
		}, wantSubstr: "[]string"},
		{name: "targets_empty", setup: func(ctx *statemachine.Context) {
			ctx.Set("language", "java")
			ctx.Set("disable_reason", "AT - RED - SYSTEM DRIVER")
			ctx.State["disable_targets"] = []string{}
		}, wantSubstr: "empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newActions(Deps{})
			ctx := statemachine.NewContext()
			tc.setup(ctx)
			out := a.enableChangeDriven(ctx)
			if out.Err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(out.Err.Error(), tc.wantSubstr) {
				t.Fatalf("error %v does not mention %q", out.Err, tc.wantSubstr)
			}
		})
	}
}

func TestEnableChangeDriven_ShellsOncePerTarget(t *testing.T) {
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("ok")},
		{out: []byte("ok")},
	}}
	a := newActions(Deps{Shell: sh})
	ctx := statemachine.NewContext()
	ctx.Set("language", "java")
	ctx.Set("disable_reason", "AT - RED - SYSTEM DRIVER")
	ctx.State["disable_targets"] = []string{
		"src/test/java/A.java:shouldFooSucceed",
		"src/test/java/B.java:shouldBarSucceed",
	}
	out := a.enableChangeDriven(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	want := []string{
		"./enable-test.sh java 'AT - RED - SYSTEM DRIVER' src/test/java/A.java:shouldFooSucceed",
		"./enable-test.sh java 'AT - RED - SYSTEM DRIVER' src/test/java/B.java:shouldBarSucceed",
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

func TestEnableChangeDriven_FirstFailureHaltsRun(t *testing.T) {
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("could not parse"), err: errors.New("exit 2")},
	}}
	a := newActions(Deps{Shell: sh})
	ctx := statemachine.NewContext()
	ctx.Set("language", "typescript")
	ctx.Set("disable_reason", "AT - RED - SYSTEM DRIVER")
	ctx.State["disable_targets"] = []string{
		"system-test/typescript/A.spec.ts:should_foo",
		"system-test/typescript/B.spec.ts:should_bar",
	}
	out := a.enableChangeDriven(ctx)
	if out.Err == nil {
		t.Fatalf("expected error on first-target failure")
	}
	if !strings.Contains(out.Err.Error(), "A.spec.ts:should_foo") {
		t.Fatalf("error should name the failing target: %v", out.Err)
	}
	if len(sh.calls) != 1 {
		t.Fatalf("got %d calls, want 1 (must halt on first failure)", len(sh.calls))
	}
}

// ---------------------------------------------------------------------------
// runTargetedTests — green-phase additions: tests_pass write, rebuild flag
// ---------------------------------------------------------------------------

func TestRunTargetedTests_AllPassWritesTestsPassTrue(t *testing.T) {
	root, _ := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("OK"), err: nil},
		{out: []byte("OK"), err: nil},
	}}
	a := newActions(Deps{Shell: sh, RepoPath: root})
	ctx := statemachine.NewContext()
	ctx.Set("suite", "<acceptance-api>")
	ctx.State["test_names"] = []string{"a", "b"}
	out := a.runTargetedTests(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if got := ctx.Get("tests_pass"); got != true {
		t.Fatalf("tests_pass: got %v, want true", got)
	}
	if got := ctx.Get("tests_failed_runtime"); got != false {
		t.Fatalf("tests_failed_runtime: got %v, want false", got)
	}
}

func TestRunTargetedTests_AnyFailureWritesTestsPassFalse(t *testing.T) {
	root, _ := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("OK"), err: nil},
		{out: []byte("AssertionError"), err: errors.New("exit 1")},
	}}
	a := newActions(Deps{Shell: sh, RepoPath: root})
	ctx := statemachine.NewContext()
	ctx.Set("suite", "<acceptance-api>")
	ctx.State["test_names"] = []string{"a", "b"}
	out := a.runTargetedTests(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if got := ctx.Get("tests_pass"); got != false {
		t.Fatalf("tests_pass: got %v, want false (any failure must demote)", got)
	}
}

func TestRunTargetedTests_RebuildParamPassesFlag(t *testing.T) {
	root, flags := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("OK"), err: nil},
	}}
	a := newActions(Deps{Shell: sh, RepoPath: root})
	ctx := statemachine.NewContext()
	ctx.Set("suite", "<acceptance-api>")
	ctx.State["test_names"] = []string{"x"}
	ctx.Params["rebuild_before_run"] = "true"
	out := a.runTargetedTests(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	want := "gh optivem test system --rebuild --suite '<acceptance-api>' --test x" + flags
	if len(sh.calls) != 1 || sh.calls[0] != want {
		t.Fatalf("got %v, want %q", sh.calls, want)
	}
}

func TestRunTargetedTests_NoRebuildParamOmitsFlag(t *testing.T) {
	root, flags := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("OK"), err: nil},
	}}
	a := newActions(Deps{Shell: sh, RepoPath: root})
	ctx := statemachine.NewContext()
	ctx.Set("suite", "<acceptance-api>")
	ctx.State["test_names"] = []string{"x"}
	out := a.runTargetedTests(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	want := "gh optivem test system --suite '<acceptance-api>' --test x" + flags
	if len(sh.calls) != 1 || sh.calls[0] != want {
		t.Fatalf("got %v, want %q", sh.calls, want)
	}
}

// ---------------------------------------------------------------------------
// verifyRealSuitePasses — optional CT real-vs-stub verification
// ---------------------------------------------------------------------------

func TestVerifyRealSuitePasses_RequiresParamAndTestNames(t *testing.T) {
	t.Run("no_param", func(t *testing.T) {
		a := newActions(Deps{})
		ctx := statemachine.NewContext()
		ctx.State["test_names"] = []string{"x"}
		out := a.verifyRealSuitePasses(ctx)
		if out.Err == nil || !strings.Contains(out.Err.Error(), "verify_real_suite") {
			t.Fatalf("expected verify_real_suite error, got %v", out.Err)
		}
	})
	t.Run("whitespace_param", func(t *testing.T) {
		a := newActions(Deps{})
		ctx := statemachine.NewContext()
		ctx.Params["verify_real_suite"] = "   "
		ctx.State["test_names"] = []string{"x"}
		out := a.verifyRealSuitePasses(ctx)
		if out.Err == nil || !strings.Contains(out.Err.Error(), "verify_real_suite") {
			t.Fatalf("expected verify_real_suite error, got %v", out.Err)
		}
	})
	t.Run("no_test_names", func(t *testing.T) {
		a := newActions(Deps{})
		ctx := statemachine.NewContext()
		ctx.Params["verify_real_suite"] = "<suite-contract-real>"
		out := a.verifyRealSuitePasses(ctx)
		if out.Err == nil || !strings.Contains(out.Err.Error(), "test_names") {
			t.Fatalf("expected test_names error, got %v", out.Err)
		}
	})
	t.Run("test_names_wrong_type", func(t *testing.T) {
		a := newActions(Deps{})
		ctx := statemachine.NewContext()
		ctx.Params["verify_real_suite"] = "<suite-contract-real>"
		ctx.State["test_names"] = "not-a-slice"
		out := a.verifyRealSuitePasses(ctx)
		if out.Err == nil || !strings.Contains(out.Err.Error(), "[]string") {
			t.Fatalf("expected []string error, got %v", out.Err)
		}
	})
	t.Run("test_names_empty", func(t *testing.T) {
		a := newActions(Deps{})
		ctx := statemachine.NewContext()
		ctx.Params["verify_real_suite"] = "<suite-contract-real>"
		ctx.State["test_names"] = []string{}
		out := a.verifyRealSuitePasses(ctx)
		if out.Err == nil || !strings.Contains(out.Err.Error(), "empty") {
			t.Fatalf("expected empty error, got %v", out.Err)
		}
	})
}

func TestVerifyRealSuitePasses_AllPassWritesTrue(t *testing.T) {
	root, flags := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("OK"), err: nil},
		{out: []byte("OK"), err: nil},
	}}
	a := newActions(Deps{Shell: sh, RepoPath: root})
	ctx := statemachine.NewContext()
	ctx.Params["verify_real_suite"] = "<suite-contract-real>"
	ctx.State["test_names"] = []string{"shouldFooSucceed", "shouldBarSucceed"}
	out := a.verifyRealSuitePasses(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !out.Bool {
		t.Fatalf("Bool: got false, want true")
	}
	if got := ctx.Get("verify_real_pass"); got != true {
		t.Fatalf("verify_real_pass: got %v, want true", got)
	}
	want := []string{
		"gh optivem test system --suite '<suite-contract-real>' --test shouldFooSucceed" + flags,
		"gh optivem test system --suite '<suite-contract-real>' --test shouldBarSucceed" + flags,
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

func TestVerifyRealSuitePasses_AnyFailWritesFalse(t *testing.T) {
	// Real-suite verification is a contract gate: classification of the
	// failure does not matter, only that one happened. Mix a runtime
	// failure with a pass to confirm the action does not require all
	// failures to share a class.
	root, _ := setupVerifyRepoLayout(t)
	sh := &scriptedShell{t: t, scripted: []scriptedResponse{
		{out: []byte("OK"), err: nil},
		{out: []byte("AssertionError"), err: errors.New("exit 1")},
	}}
	a := newActions(Deps{Shell: sh, RepoPath: root})
	ctx := statemachine.NewContext()
	ctx.Params["verify_real_suite"] = "<suite-contract-real>"
	ctx.State["test_names"] = []string{"shouldFooPass", "shouldBarPass"}
	out := a.verifyRealSuitePasses(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if out.Bool {
		t.Fatalf("Bool: got true, want false (any failure must demote)")
	}
	if got := ctx.Get("verify_real_pass"); got != false {
		t.Fatalf("verify_real_pass: got %v, want false", got)
	}
	if len(sh.calls) != 2 {
		t.Fatalf("got %d calls, want 2 (action runs every test even after a failure): %v", len(sh.calls), sh.calls)
	}
}

// TestRunTargetedTests_HaltsOnUnresolvablePaths and the verify_real twin
// cover the case where ResolveSystemTestPaths returns an error: action
// halts via Outcome.Err rather than running the runner with broken
// `./system.json` defaults. Unlike runTests this halt is
// a plain Outcome.Err (no infra-class banner) — these actions are
// deterministic mechanics without a verify-style halt protocol.
func TestRunTargetedTests_HaltsOnUnresolvablePaths(t *testing.T) {
	root := t.TempDir() // empty — no docker/, no system-test/
	sh := &scriptedShell{t: t}
	a := newActions(Deps{Shell: sh, RepoPath: root})
	ctx := statemachine.NewContext()
	ctx.Set("suite", "<acceptance-api>")
	ctx.State["test_names"] = []string{"x"}
	out := a.runTargetedTests(ctx)
	if out.Err == nil {
		t.Fatalf("expected Outcome.Err when paths cannot be resolved")
	}
	if !strings.Contains(out.Err.Error(), "could not locate system.json") {
		t.Errorf("Err should mention path resolution; got %v", out.Err)
	}
	if len(sh.calls) != 0 {
		t.Errorf("expected no shell calls when path resolution fails; got %v", sh.calls)
	}
}

func TestVerifyRealSuitePasses_HaltsOnUnresolvablePaths(t *testing.T) {
	root := t.TempDir()
	sh := &scriptedShell{t: t}
	a := newActions(Deps{Shell: sh, RepoPath: root})
	ctx := statemachine.NewContext()
	ctx.Params["verify_real_suite"] = "<suite-contract-real>"
	ctx.State["test_names"] = []string{"x"}
	out := a.verifyRealSuitePasses(ctx)
	if out.Err == nil {
		t.Fatalf("expected Outcome.Err when paths cannot be resolved")
	}
	if !strings.Contains(out.Err.Error(), "could not locate system.json") {
		t.Errorf("Err should mention path resolution; got %v", out.Err)
	}
	if len(sh.calls) != 0 {
		t.Errorf("expected no shell calls when path resolution fails; got %v", sh.calls)
	}
}

