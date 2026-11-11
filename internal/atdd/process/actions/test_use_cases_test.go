package actions

import (
	"bytes"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

func TestUseCaseSelection_ReducesToUseCasesDeduplicated(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"", ""},
		{"  ", ""},
		{"UC-001", "UC-001"},
		{"UC-001.1, UC-001.2", "UC-001"},
		{"UC-003.2,UC-001,UC-003", "UC-003,UC-001"},
		{"[UC-002 UC-004.1]", "UC-002,UC-004"},
	}
	for _, tc := range cases {
		got, err := useCaseSelection(tc.raw)
		if err != nil {
			t.Fatalf("useCaseSelection(%q): %v", tc.raw, err)
		}
		if strings.Join(got, ",") != tc.want {
			t.Errorf("useCaseSelection(%q) = %v, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestUseCaseSelection_RejectsMalformedID(t *testing.T) {
	for _, raw := range []string{"OrderService", "UC-1.x", "uc-001", "UC-001,checkout"} {
		if _, err := useCaseSelection(raw); err == nil {
			t.Errorf("useCaseSelection(%q): want an error", raw)
		}
	}
}

func TestResolveTestUseCases(t *testing.T) {
	cases := []struct {
		name    string
		param   *string // test-use-cases bound by the caller (nil = not bound)
		merged  string  // the story's merged-criteria ("" = not a story)
		want    string
		wantErr bool
	}{
		{name: "story: merged criteria reduced to use cases", merged: "UC-001.1, UC-001.2, UC-004.3", want: "UC-001,UC-004"},
		{name: "no story, no review: whole suite", want: ""},
		{name: "review plan wins over the story", param: ptr("UC-007"), merged: "UC-001.1", want: "UC-007"},
		{name: "review plan naming no use case: whole suite, even in a story", param: ptr(""), merged: "UC-001.1", want: ""},
		{name: "malformed review id fails loud", param: ptr("checkout"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newActions(Deps{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
			ctx := statemachine.NewContext()
			if tc.param != nil {
				ctx.Params[ParamTestUseCases] = *tc.param
			}
			if tc.merged != "" {
				ctx.Set(CtxKeyMergedCriteria, tc.merged)
			}
			out := a.resolveTestUseCases(ctx)
			if tc.wantErr {
				if out.Err == nil {
					t.Fatal("want an error")
				}
				return
			}
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			got, set := ctx.State[CtxKeyRefactorTestUseCases]
			if !set {
				t.Fatalf("%s not stamped (the cycle's verify binding would not resolve)", CtxKeyRefactorTestUseCases)
			}
			if got != tc.want {
				t.Errorf("%s = %q, want %q", CtxKeyRefactorTestUseCases, got, tc.want)
			}
		})
	}
}

func ptr(s string) *string { return &s }

func TestSummarizeRefactoringPlan_RefactorUseCaseIDs(t *testing.T) {
	cases := []struct {
		name        string
		ids         any // reviewer output (nil = omitted)
		wantIDs     string
		wantSummary string
	}{
		{name: "omitted: whole suite", wantIDs: "", wantSummary: "Verified on: the whole suite"},
		{name: "string-list output, criterion ids reduced", ids: []string{"UC-001.2", "UC-003", "UC-001"}, wantIDs: "UC-001,UC-003", wantSummary: "Verified on: the tests of UC-001, UC-003"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newActions(Deps{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
			ctx := statemachine.NewContext()
			ctx.Set(CtxKeyRefactoringNeeded, true)
			ctx.Set(CtxKeySystemRefactoringPlan, "OrderService.java: extract OrderValidator")
			if tc.ids != nil {
				ctx.Set(CtxKeyRefactorUseCaseIDs, tc.ids)
			}
			if out := a.summarizeRefactoringPlan(ctx); out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			got, set := ctx.State[CtxKeyRefactorUseCaseIDs]
			if !set || got != tc.wantIDs {
				t.Errorf("%s = %v (set=%v), want %q stamped", CtxKeyRefactorUseCaseIDs, got, set, tc.wantIDs)
			}
			if s := ctx.GetString(CtxKeyRefactoringPlanSummary); !strings.Contains(s, tc.wantSummary) {
				t.Errorf("summary missing %q:\n%s", tc.wantSummary, s)
			}
		})
	}
}

func TestSummarizeRefactoringPlan_RefactorUseCaseIDsFailLoud(t *testing.T) {
	cases := []struct {
		name   string
		needed bool
		ids    string
	}{
		{name: "malformed id", needed: true, ids: "the checkout flow"},
		{name: "use cases without a plan", needed: false, ids: "UC-001"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newActions(Deps{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
			ctx := statemachine.NewContext()
			ctx.Set(CtxKeyRefactoringNeeded, tc.needed)
			if tc.needed {
				ctx.Set(CtxKeySystemRefactoringPlan, "OrderService.java: extract OrderValidator")
			}
			ctx.Set(CtxKeyRefactorUseCaseIDs, tc.ids)
			out := a.summarizeRefactoringPlan(ctx)
			if out.Err == nil || !strings.Contains(out.Err.Error(), CtxKeyRefactorUseCaseIDs) {
				t.Fatalf("Err = %v, want a %s error", out.Err, CtxKeyRefactorUseCaseIDs)
			}
		})
	}
}
