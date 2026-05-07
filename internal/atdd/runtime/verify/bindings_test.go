// Tests for verify/bindings.go.
//
// Coverage:
//   - requireHeadMatches accepts/rejects HEAD messages by phase suffix.
//   - WrapAll inserts the binding into every process and surfaces Pre errors
//     through Outcome.Err.
//   - Soft-skip paths (no commits / no git) return nil rather than failing.
package verify

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

type fakeGit struct {
	out []byte
	err error
}

func (f fakeGit) Run(_ context.Context, _ ...string) ([]byte, error) {
	return f.out, f.err
}

// ---------------------------------------------------------------------------
// requireHeadMatches
// ---------------------------------------------------------------------------

func TestRequireHeadMatches(t *testing.T) {
	for _, tc := range []struct {
		name    string
		head    string
		phase   string
		wantErr bool
	}{
		{
			name:  "exact_phase_suffix",
			head:  "Register Customer | AT - RED - TEST",
			phase: "AT - RED - TEST",
		},
		{
			name:  "with_issue_prefix",
			head:  "#42 | Register Customer | AT - RED - TEST",
			phase: "AT - RED - TEST",
		},
		{
			name:  "trailing_whitespace",
			head:  "Register Customer | AT - RED - TEST   \n",
			phase: "AT - RED - TEST",
		},
		{
			name:    "wrong_phase",
			head:    "Register Customer | AT - RED - DSL",
			phase:   "AT - RED - TEST",
			wantErr: true,
		},
		{
			name:    "phase_in_body_not_suffix",
			head:    "AT - RED - TEST changed several files\n\nUnrelated work.",
			phase:   "AT - RED - TEST",
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := Deps{Git: fakeGit{out: []byte(tc.head)}}.withDefaults()
			err := requireHeadMatches(deps, tc.phase, "AT_RED_DSL")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err: got %v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestRequireHeadMatches_SoftSkipOnFreshRepo(t *testing.T) {
	for _, msg := range []string{
		"fatal: your current branch 'main' does not have any commits yet",
		"fatal: not a git repository (or any of the parent directories): .git",
		"exec: \"git\": executable file not found in $PATH",
	} {
		t.Run(msg, func(t *testing.T) {
			deps := Deps{Git: fakeGit{err: errors.New(msg)}}.withDefaults()
			if err := requireHeadMatches(deps, "AT - RED - TEST", "AT_RED_DSL"); err != nil {
				t.Fatalf("expected nil (soft-skip), got %v", err)
			}
		})
	}
}

func TestRequireHeadMatches_HardError(t *testing.T) {
	deps := Deps{Git: fakeGit{err: fmt.Errorf("permission denied")}}.withDefaults()
	if err := requireHeadMatches(deps, "AT - RED - TEST", "AT_RED_DSL"); err == nil {
		t.Fatalf("expected error for non-soft failure")
	}
}

func TestRequireHeadMatches_EmptyOutput(t *testing.T) {
	deps := Deps{Git: fakeGit{out: []byte("")}}.withDefaults()
	if err := requireHeadMatches(deps, "AT - RED - TEST", "AT_RED_DSL"); err != nil {
		t.Fatalf("expected soft-skip on empty HEAD, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// WrapAll
// ---------------------------------------------------------------------------

func TestWrapAll_AppliesPreCheckToATRedDsl(t *testing.T) {
	// Build a tiny engine with a single process containing AT_RED_DSL.
	eng := &statemachine.Engine{
		Processes: map[string]*statemachine.Process{
			"at_cycle": {
				Name:  "at_cycle",
				Start: "AT_RED_DSL",
				Nodes: map[string]statemachine.Node{
					"AT_RED_DSL": {
						ID:   "AT_RED_DSL",
						Kind: statemachine.UserTask,
						Fn: func(_ *statemachine.Context) statemachine.Outcome {
							return statemachine.Outcome{}
						},
					},
				},
				OutgoingByNode: map[string][]statemachine.Edge{},
			},
		},
	}

	// Wrong-phase HEAD should cause the wrapped Fn to surface an error.
	WrapAll(eng, Deps{Git: fakeGit{out: []byte("Foo | AT - RED - DSL")}})
	out := eng.Processes["at_cycle"].Nodes["AT_RED_DSL"].Fn(statemachine.NewContext())
	if out.Err == nil {
		t.Fatalf("expected verify error, got Outcome %+v", out)
	}
	if !strings.Contains(out.Err.Error(), "AT - RED - TEST") {
		t.Fatalf("error should reference required phase: %v", out.Err)
	}
}

func TestWrapAll_LeavesUntargetedNodesUntouched(t *testing.T) {
	called := false
	eng := &statemachine.Engine{
		Processes: map[string]*statemachine.Process{
			"main": {
				Name:  "main",
				Start: "OTHER_NODE",
				Nodes: map[string]statemachine.Node{
					"OTHER_NODE": {
						ID:   "OTHER_NODE",
						Kind: statemachine.UserTask,
						Fn: func(_ *statemachine.Context) statemachine.Outcome {
							called = true
							return statemachine.Outcome{}
						},
					},
				},
				OutgoingByNode: map[string][]statemachine.Edge{},
			},
		},
	}
	WrapAll(eng, Deps{Git: fakeGit{out: []byte("anything")}})
	out := eng.Processes["main"].Nodes["OTHER_NODE"].Fn(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !called {
		t.Fatalf("original Fn was not called — Wrap shouldn't touch unbound nodes")
	}
}

// ---------------------------------------------------------------------------
// Bindings table
// ---------------------------------------------------------------------------

func TestBindings_TableContents(t *testing.T) {
	bs := Bindings(Deps{Git: fakeGit{}})
	want := []string{"AT_RED_DSL", "CT_RED_DSL", "CT_RED_EXTERNAL_DRIVER"}
	for _, id := range want {
		b, ok := bs[id]
		if !ok {
			t.Errorf("binding %q missing", id)
			continue
		}
		if b.Pre == nil {
			t.Errorf("binding %q: Pre is nil", id)
		}
	}
}
