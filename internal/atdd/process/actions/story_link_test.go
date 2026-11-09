package actions

import (
	"errors"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

const linkTestBody = "## Acceptance Criteria\n\nScenario: x\n  Given a\n  When b\n  Then c\n"

func linkCtx(merged string) *statemachine.Context {
	ctx := statemachine.NewContext()
	seedIssue(ctx)
	ctx.Set(CtxKeyMergedCriteria, merged)
	return ctx
}

func TestLinkStoryCriteria_WritesSection(t *testing.T) {
	tk := &fakeTracker{body: linkTestBody}
	a := newActions(Deps{Tracker: tk})
	if out := a.linkStoryCriteria(linkCtx("UC-001.1, UC-001.3")); out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if len(tk.written) != 1 {
		t.Fatalf("WriteBody calls = %d, want 1", len(tk.written))
	}
	want := linkTestBody + "\n## Use Case Criteria\n\n- UC-001.1\n- UC-001.3\n"
	if tk.written[0] != want {
		t.Errorf("written body:\n%q\nwant:\n%q", tk.written[0], want)
	}
}

// A re-run finds the section already holding the same ids and writes nothing.
func TestLinkStoryCriteria_RerunIsIdempotent(t *testing.T) {
	tk := &fakeTracker{body: linkTestBody}
	a := newActions(Deps{Tracker: tk})
	for range 2 {
		if out := a.linkStoryCriteria(linkCtx("UC-001.1")); out.Err != nil {
			t.Fatalf("unexpected error: %v", out.Err)
		}
	}
	if len(tk.written) != 1 {
		t.Fatalf("WriteBody calls = %d, want 1 (the re-run changes nothing)", len(tk.written))
	}
}

// A changed id set replaces the section instead of appending a second one.
func TestLinkStoryCriteria_ReplacesExistingSection(t *testing.T) {
	tk := &fakeTracker{body: linkTestBody + "\n## Use Case Criteria\n\n- UC-009.9\n"}
	a := newActions(Deps{Tracker: tk})
	if out := a.linkStoryCriteria(linkCtx("UC-001.2")); out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	got := tk.written[0]
	if strings.Count(got, "## Use Case Criteria") != 1 || strings.Contains(got, "UC-009.9") || !strings.Contains(got, "- UC-001.2") {
		t.Errorf("section not replaced: %q", got)
	}
}

func TestLinkStoryCriteria_WriteFailureFailsLoudly(t *testing.T) {
	tk := &fakeTracker{body: linkTestBody, writeErr: errors.New("HTTP 403")}
	a := newActions(Deps{Tracker: tk})
	out := a.linkStoryCriteria(linkCtx("UC-001.1"))
	if out.Err == nil || !strings.Contains(out.Err.Error(), "HTTP 403") {
		t.Fatalf("Err = %v, want the write failure", out.Err)
	}
}

func TestLinkStoryCriteria_ReadFailureFailsLoudly(t *testing.T) {
	tk := &fakeTracker{readErr: errors.New("HTTP 502")}
	a := newActions(Deps{Tracker: tk})
	out := a.linkStoryCriteria(linkCtx("UC-001.1"))
	if out.Err == nil || !strings.Contains(out.Err.Error(), "HTTP 502") || len(tk.written) != 0 {
		t.Fatalf("Err = %v, written = %d; want the read failure and no write", out.Err, len(tk.written))
	}
}

func TestLinkStoryCriteria_NoMergedCriteriaFails(t *testing.T) {
	tk := &fakeTracker{body: linkTestBody}
	a := newActions(Deps{Tracker: tk})
	out := a.linkStoryCriteria(linkCtx(""))
	if out.Err == nil || !strings.Contains(out.Err.Error(), CtxKeyMergedCriteria) {
		t.Fatalf("Err = %v, want a merged-criteria error", out.Err)
	}
}
