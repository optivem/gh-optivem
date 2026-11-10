package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

func acPolicyCtx(merged string) *statemachine.Context {
	ctx := statemachine.NewContext()
	ctx.Set(CtxKeyMergedCriteria, merged)
	return ctx
}

func readUseCase(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "docs", "use-cases", "UC-001-place-order.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestApplyAcPolicy_DeletesMergedCriteria(t *testing.T) {
	root := mergeRepo(t, mergeTestUseCase)
	a := newActions(Deps{RepoPath: root})
	if out := a.applyAcPolicy(acPolicyCtx("UC-001.2")); out.Err != nil {
		t.Fatal(out.Err)
	}
	got := readUseCase(t, root)
	if strings.Contains(got, "UC-001.2") || !strings.Contains(got, "- UC-001.1: A valid order is placed.") || !strings.Contains(got, "title: Place Order") {
		t.Fatalf("use case after the policy:\n%s", got)
	}
}

func TestApplyAcPolicy_DeletesGherkinScenariosInFolderLayout(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "use-cases", "UC-001-place-order")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	criteria := "---\nid: UC-001\ntitle: Place Order\n---\n\n```gherkin\nFeature: Place Order\n\n@UC-001.1\nScenario: Placed\n  Then placed\n\n@UC-001.2\nScenario: Rejected\n  Then rejected\n```\n"
	narrative := "1. The customer places an order.\n"
	if err := os.WriteFile(filepath.Join(dir, "acceptance-criteria.md"), []byte(criteria), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "use-case-narrative.md"), []byte(narrative), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newActions(Deps{RepoPath: root})
	if out := a.applyAcPolicy(acPolicyCtx("UC-001.1, UC-001.2")); out.Err != nil {
		t.Fatal(out.Err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "acceptance-criteria.md"))
	if strings.Contains(string(got), "Scenario") || !strings.Contains(string(got), "Feature: Place Order") {
		t.Fatalf("criteria after the policy:\n%s", got)
	}
	if n, _ := os.ReadFile(filepath.Join(dir, "use-case-narrative.md")); string(n) != narrative {
		t.Fatalf("the narrative must be untouched, got %q", n)
	}
}

func TestApplyAcPolicy_FailsLoud(t *testing.T) {
	root := mergeRepo(t, mergeTestUseCase)
	a := newActions(Deps{RepoPath: root})
	if out := a.applyAcPolicy(acPolicyCtx("")); out.Err == nil || !strings.Contains(out.Err.Error(), CtxKeyMergedCriteria) {
		t.Fatalf("empty merged-criteria: err = %v", out.Err)
	}
	out := a.applyAcPolicy(acPolicyCtx("UC-001.1, UC-001.7"))
	if out.Err == nil || !strings.Contains(out.Err.Error(), "UC-001.7") {
		t.Fatalf("unknown criterion: err = %v", out.Err)
	}
	if got := readUseCase(t, root); got != mergeTestUseCase {
		t.Fatalf("a failed policy must write nothing, got:\n%s", got)
	}
}

// characterize's per-use-case deletion: every criterion of the current use
// case goes, the rest of the file stays; a second run (resume before the
// commit) is a no-op; no use case selected is an error.
func TestDeleteUseCaseCriteria(t *testing.T) {
	root := mergeRepo(t, mergeTestUseCase)
	a := newActions(Deps{RepoPath: root})
	ctx := statemachine.NewContext()
	ctx.Set(CtxKeyUseCaseID, "UC-001")
	if out := a.deleteUseCaseCriteria(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	got := readUseCase(t, root)
	if strings.Contains(got, "UC-001.1") || strings.Contains(got, "UC-001.2") || !strings.Contains(got, "title: Place Order") {
		t.Fatalf("use case after the deletion:\n%s", got)
	}
	if out := a.deleteUseCaseCriteria(ctx); out.Err != nil {
		t.Fatalf("second run must be a no-op: %v", out.Err)
	}
	if again := readUseCase(t, root); again != got {
		t.Fatalf("second run changed the file:\n%s", again)
	}
	if out := a.deleteUseCaseCriteria(statemachine.NewContext()); out.Err == nil || !strings.Contains(out.Err.Error(), CtxKeyUseCaseID) {
		t.Fatalf("no use case selected: err = %v", out.Err)
	}
}
