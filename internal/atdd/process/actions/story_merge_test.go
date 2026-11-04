package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

const mergeTestUseCase = "---\nid: UC-001\ntitle: Place Order\n---\n\n- UC-001.1: A valid order is placed.\n- UC-001.2: A zero quantity is rejected.\n"

// mergeRepo builds a repo root holding the given use case file (empty content
// means no use case directory at all).
func mergeRepo(t *testing.T, useCase string) string {
	t.Helper()
	root := t.TempDir()
	if useCase != "" {
		dir := filepath.Join(root, "docs", "use-cases")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "UC-001-place-order.md"), []byte(useCase), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func mergeCtx(criteria string) *statemachine.Context {
	ctx := statemachine.NewContext()
	ctx.Set("acceptance-criteria", criteria)
	return ctx
}

const (
	storyMatchingUseCases = "@UC-001.1\nScenario: place a valid order\n  Given a product\n  When I order it\n  Then it is placed\n\n@UC-001.2\nScenario: reject zero\n  Given a product\n  When I order 0\n  Then it is rejected\n"
	storyNewBehaviour     = "Scenario: apply a coupon\n  Given a coupon\n  When I order\n  Then the price drops\n"
)

func TestCheckStoryMergeNeeded_NoOpWhenScenariosNameExistingCriteria(t *testing.T) {
	a := newActions(Deps{RepoPath: mergeRepo(t, mergeTestUseCase)})
	ctx := mergeCtx(storyMatchingUseCases)
	if out := a.checkStoryMergeNeeded(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if need, _ := ctx.Get(CtxKeyStoryMergeNeeded).(bool); need {
		t.Fatal("a story matching the use cases must not need a merge (and so no approval)")
	}
	if got := ctx.GetString(CtxKeyMergedCriteria); got != "UC-001.1, UC-001.2" {
		t.Errorf("merged-criteria = %q", got)
	}
	want := "place a valid order => UC-001.1\nreject zero => UC-001.2"
	if got := ctx.GetString(CtxKeyScenarioCriteria); got != want {
		t.Errorf("scenario-criteria = %q, want %q", got, want)
	}
}

func TestCheckStoryMergeNeeded_NewBehaviourNeedsMerge(t *testing.T) {
	a := newActions(Deps{RepoPath: mergeRepo(t, mergeTestUseCase)})
	ctx := mergeCtx(storyNewBehaviour)
	if out := a.checkStoryMergeNeeded(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if need, _ := ctx.Get(CtxKeyStoryMergeNeeded).(bool); !need {
		t.Fatal("a story adding behaviour must be merged")
	}
}

func TestCheckStoryMergeNeeded_RepoWithoutUseCasesNeedsMerge(t *testing.T) {
	a := newActions(Deps{RepoPath: mergeRepo(t, "")})
	ctx := mergeCtx(storyMatchingUseCases)
	if out := a.checkStoryMergeNeeded(ctx); out.Err != nil {
		t.Fatalf("a missing spec dir is the 'no use cases yet' state, not an error: %v", out.Err)
	}
	if need, _ := ctx.Get(CtxKeyStoryMergeNeeded).(bool); !need {
		t.Fatal("references to criteria that do not exist cannot be a no-op")
	}
}

func TestCheckStoryMergeNeeded_FailsLoudOnBrokenInputs(t *testing.T) {
	a := newActions(Deps{RepoPath: mergeRepo(t, mergeTestUseCase)})
	if out := a.checkStoryMergeNeeded(mergeCtx("  ")); out.Err == nil {
		t.Error("a story without criteria must error")
	}
	root := mergeRepo(t, mergeTestUseCase)
	if err := os.WriteFile(filepath.Join(root, "spec.yaml"), []byte("bogusKey: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out := newActions(Deps{RepoPath: root}).checkStoryMergeNeeded(mergeCtx(storyNewBehaviour)); out.Err == nil || !strings.Contains(out.Err.Error(), "spec.yaml") {
		t.Errorf("a malformed spec.yaml must surface, got %v", out.Err)
	}
}

func TestVerifyStoryMerge(t *testing.T) {
	merged := mergeTestUseCase + "- UC-001.3: A coupon lowers the price.\n"
	story := storyNewBehaviour

	t.Run("complete merge passes and stamps traceability", func(t *testing.T) {
		a := newActions(Deps{RepoPath: mergeRepo(t, merged)})
		ctx := mergeCtx(story)
		ctx.Set(ctxKeyScenarioMap, []string{"apply a coupon => UC-001.3"})
		if out := a.verifyStoryMerge(ctx); out.Err != nil {
			t.Fatal(out.Err)
		}
		if got := ctx.GetString(CtxKeyMergedCriteria); got != "UC-001.3" {
			t.Errorf("merged-criteria = %q", got)
		}
		if got := ctx.GetString(CtxKeyScenarioCriteria); got != "apply a coupon => UC-001.3" {
			t.Errorf("scenario-criteria = %q", got)
		}
	})

	t.Run("an unmapped scenario is a loud gap", func(t *testing.T) {
		a := newActions(Deps{RepoPath: mergeRepo(t, merged)})
		ctx := mergeCtx(story + "\nScenario: another\n  Given x\n  When y\n  Then z\n")
		ctx.Set(ctxKeyScenarioMap, []string{"apply a coupon => UC-001.3"})
		out := a.verifyStoryMerge(ctx)
		if out.Err == nil || !strings.Contains(out.Err.Error(), `"another"`) {
			t.Fatalf("expected a gap naming the scenario, got %v", out.Err)
		}
	})

	t.Run("a mapped criterion missing from the use cases is a gap", func(t *testing.T) {
		a := newActions(Deps{RepoPath: mergeRepo(t, mergeTestUseCase)})
		ctx := mergeCtx(story)
		ctx.Set(ctxKeyScenarioMap, []string{"apply a coupon => UC-001.3"})
		if out := a.verifyStoryMerge(ctx); out.Err == nil || !strings.Contains(out.Err.Error(), "UC-001.3") {
			t.Fatalf("got %v", out.Err)
		}
	})

	t.Run("an unresolved contradiction halts with the agent's list", func(t *testing.T) {
		a := newActions(Deps{RepoPath: mergeRepo(t, merged)})
		ctx := mergeCtx(story)
		ctx.Set(ctxKeyContradictions, []string{"apply a coupon conflicts with UC-001.1"})
		ctx.Set(ctxKeyScenarioMap, []string{"apply a coupon => UC-001.3"})
		if out := a.verifyStoryMerge(ctx); out.Err == nil || !strings.Contains(out.Err.Error(), "conflicts with UC-001.1") {
			t.Fatalf("got %v", out.Err)
		}
	})

	t.Run("missing scenario-map is rejected", func(t *testing.T) {
		a := newActions(Deps{RepoPath: mergeRepo(t, merged)})
		if out := a.verifyStoryMerge(mergeCtx(story)); out.Err == nil {
			t.Fatal("no scenario-map must fail the check")
		}
	})

	t.Run("invalid merged use cases fail the check", func(t *testing.T) {
		a := newActions(Deps{RepoPath: mergeRepo(t, merged+"- UC-001.3: duplicate id.\n")})
		ctx := mergeCtx(story)
		ctx.Set(ctxKeyScenarioMap, []string{"apply a coupon => UC-001.3"})
		if out := a.verifyStoryMerge(ctx); out.Err == nil || !strings.Contains(out.Err.Error(), "duplicate-id") {
			t.Fatalf("got %v", out.Err)
		}
	})
}
