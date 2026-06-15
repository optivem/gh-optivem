package factory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker/factory"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// TestOpen_GitHubProviderRoutes constructs the github adapter for a
// well-formed projectV2 URL.
func TestOpen_GitHubProviderRoutes(t *testing.T) {
	t.Parallel()
	tr, err := factory.Open(context.Background(), projectconfig.Project{
		Provider: projectconfig.ProviderGitHub,
		URL:      "https://github.com/orgs/optivem/projects/20",
	})
	if err != nil {
		t.Fatalf("Open(github): %v", err)
	}
	if tr == nil {
		t.Fatal("Open(github): want non-nil Tracker, got nil")
	}
}

// TestOpen_MarkdownProviderRoutes constructs the markdown adapter for
// a relative directory path. The path doesn't need to exist for New
// to succeed — Verify is the layer that checks layout.
func TestOpen_MarkdownProviderRoutes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tr, err := factory.Open(context.Background(), projectconfig.Project{
		Provider: projectconfig.ProviderMarkdown,
		URL:      dir,
	})
	if err != nil {
		t.Fatalf("Open(markdown): %v", err)
	}
	if tr == nil {
		t.Fatal("Open(markdown): want non-nil Tracker, got nil")
	}
}

// TestOpen_EmptyProviderHintsAtMigrate names both the field and the
// migrate command so a pre-provider config has a one-shot fix path.
func TestOpen_EmptyProviderHintsAtMigrate(t *testing.T) {
	t.Parallel()
	_, err := factory.Open(context.Background(), projectconfig.Project{
		URL: "https://github.com/orgs/optivem/projects/20",
	})
	if err == nil {
		t.Fatal("Open(empty provider): want error, got nil")
	}
	if !strings.Contains(err.Error(), "project.provider is required") {
		t.Errorf("error should mention project.provider, got: %v", err)
	}
	if !strings.Contains(err.Error(), "config migrate") {
		t.Errorf("error should hint at `config migrate`, got: %v", err)
	}
}

// TestOpen_UnknownProviderNamesValue surfaces the bad value so the
// operator sees what to fix.
func TestOpen_UnknownProviderNamesValue(t *testing.T) {
	t.Parallel()
	_, err := factory.Open(context.Background(), projectconfig.Project{
		Provider: "jira",
		URL:      "https://example.atlassian.net/browse/SHOP-7",
	})
	if err == nil {
		t.Fatal("Open(unknown provider): want error, got nil")
	}
	if !strings.Contains(err.Error(), `"jira"`) {
		t.Errorf("error should name the bad value, got: %v", err)
	}
}
