package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	folderCriteria  = "---\nid: UC-001\ntitle: Place Order\n---\n\n```gherkin\nFeature: Place Order\n\n@UC-001.1\nScenario: apply a coupon\n  When I order\n  Then the price drops by the discount (BR-2)\n\n@UC-001.2 @narrative-4a\nScenario: reject zero\n  When I order 0\n  Then it is rejected\n```\n"
	folderNarrative = "# UC-001 Place Order\n\n## Main Success Scenario\n1. The customer orders.\n2. The system prices the order ([BR-1 to BR-2](business-rules.md)).\n\n## Extensions\n- 4a. The quantity is not positive:\n    - The system rejects it.\n"
	folderRules     = "# Business Rules: Place Order\n\n- BR-1. Base price = unit price x quantity.\n- BR-2. Subtotal = base price - discount.\n"
)

// folderRepo builds a repo root with UC-001 in the folder-per-use-case layout.
func folderRepo(t *testing.T, criteria, narrative, rules string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "use-cases", "UC-001-place-order")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"acceptance-criteria.md": criteria, useCaseNarrativeFile: narrative, businessRulesFile: rules} {
		if content == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func verifyFolderMerge(t *testing.T, root string) error {
	t.Helper()
	ctx := mergeCtx(storyNewBehaviour)
	ctx.Set(ctxKeyScenarioMap, []string{"apply a coupon => UC-001.1"})
	return newActions(Deps{RepoPath: root}).verifyStoryMerge(ctx).Err
}

func TestVerifyStoryMerge_UseCaseFormat(t *testing.T) {
	t.Run("folder layout with resolved tags and rules passes", func(t *testing.T) {
		if err := verifyFolderMerge(t, folderRepo(t, folderCriteria, folderNarrative, folderRules)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a narrative tag naming no extension is a gap", func(t *testing.T) {
		criteria := strings.Replace(folderCriteria, "@narrative-4a", "@narrative-9z", 1)
		err := verifyFolderMerge(t, folderRepo(t, criteria, folderNarrative, folderRules))
		if err == nil || !strings.Contains(err.Error(), "@narrative-9z") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("a business rule referenced but not defined is a gap", func(t *testing.T) {
		rules := strings.Replace(folderRules, "- BR-2. Subtotal = base price - discount.\n", "", 1)
		err := verifyFolderMerge(t, folderRepo(t, folderCriteria, folderNarrative, rules))
		if err == nil || !strings.Contains(err.Error(), "BR-2 is referenced") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("missing narrative file leaves narrative tags unresolved", func(t *testing.T) {
		err := verifyFolderMerge(t, folderRepo(t, folderCriteria, "", folderRules))
		if err == nil || !strings.Contains(err.Error(), "use-case-narrative.md") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("flat layout resolves tags and rules within the one file", func(t *testing.T) {
		flat := folderCriteria + "\n" + folderNarrative + "\n## Business rules\n- BR-1. Base price.\n- BR-2. Subtotal.\n"
		root := mergeRepo(t, flat)
		if err := verifyFolderMerge(t, root); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("use cases the story does not touch are not checked", func(t *testing.T) {
		root := folderRepo(t, folderCriteria, folderNarrative, folderRules)
		other := "---\nid: UC-002\ntitle: Other\n---\n\n- UC-002.1: Mentions BR-9 with no rules file.\n"
		if err := os.WriteFile(filepath.Join(root, "docs", "use-cases", "UC-002-other.md"), []byte(other), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := verifyFolderMerge(t, root); err != nil {
			t.Fatal(err)
		}
	})
}
