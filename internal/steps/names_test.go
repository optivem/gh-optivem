package steps

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var placeholderRegex = regexp.MustCompile(`\$\{([^}]+)\}`)

func templatePlaceholders(tmpl string) []string {
	matches := placeholderRegex.FindAllStringSubmatch(tmpl, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

// fullVars contains every recognized placeholder. Validation tests expand
// every Names entry against this map to assert nothing is left unsubstituted.
var fullVars = map[string]string{
	"lang":         "go",
	"backendLang":  "dotnet",
	"frontendLang": "react",
	"testLang":     "java",
	"arch":         "monolith",
	"stage":        "acceptance-stage",
	"stageSuffix":  "-cloud",
}

// TestNamesUseRecognizedPlaceholders asserts every placeholder appearing in
// any Names field is in recognizedPlaceholders. Catches typos like ${langg}
// or ${LANG} at test time.
func TestNamesUseRecognizedPlaceholders(t *testing.T) {
	forEachNameField(func(field, tmpl string) {
		for _, p := range templatePlaceholders(tmpl) {
			if !recognizedPlaceholders[p] {
				t.Errorf("Names.%s: unknown placeholder ${%s} in %q", field, p, tmpl)
			}
		}
	})
}

// TestNamesExpandFully asserts that with a full vars map, no ${...} survives
// expansion. Catches drift between recognizedPlaceholders and template text.
func TestNamesExpandFully(t *testing.T) {
	forEachNameField(func(field, tmpl string) {
		out := Expand(tmpl, fullVars)
		if strings.Contains(out, "${") {
			t.Errorf("Names.%s: %q expanded to %q with surviving placeholder", field, tmpl, out)
		}
	})
}

// TestExpandPanicsOnUnknownPlaceholder ensures Expand surfaces typos at
// runtime even when called with a template built outside Names.
func TestExpandPanicsOnUnknownPlaceholder(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Expand did not panic on unknown placeholder")
		}
	}()
	Expand("hello ${nonsense}", fullVars)
}

// TestNamesRepresentativeExpansion locks in canonical expansions so any
// behavioural change to a registry entry surfaces in CI.
func TestNamesRepresentativeExpansion(t *testing.T) {
	cases := []struct {
		name     string
		template string
		vars     map[string]string
		want     string
	}{
		{
			name:     "MonolithBumpPatchVersionWf",
			template: Names.MonolithBumpPatchVersionWf,
			vars:     map[string]string{"lang": "go"},
			want:     "monolith-go-bump-patch-version.yml",
		},
		{
			name:     "MonolithPipelineStageWf docker",
			template: Names.MonolithPipelineStageWf,
			vars:     map[string]string{"testLang": "java", "stage": "acceptance-stage", "stageSuffix": ""},
			want:     "monolith-java-acceptance-stage.yml",
		},
		{
			name:     "MonolithPipelineStageWf cloud-run",
			template: Names.MonolithPipelineStageWf,
			vars:     map[string]string{"testLang": "java", "stage": "qa-stage", "stageSuffix": "-cloud"},
			want:     "monolith-java-qa-stage-cloud.yml",
		},
		{
			name:     "MultitierBackendCommitStageWf",
			template: Names.MultitierBackendCommitStageWf,
			vars:     map[string]string{"backendLang": "dotnet"},
			want:     "multitier-backend-dotnet-commit-stage.yml",
		},
		{
			name:     "MultitierFrontendCommitStageWf",
			template: Names.MultitierFrontendCommitStageWf,
			vars:     map[string]string{"frontendLang": "react"},
			want:     "multitier-frontend-react-commit-stage.yml",
		},
		{
			name:     "ShopSystemMultitierBackend",
			template: Names.ShopSystemMultitierBackend,
			vars:     map[string]string{"backendLang": "dotnet"},
			want:     "system/multitier/backend-dotnet",
		},
		{
			name:     "ShopVersionFile",
			template: Names.ShopVersionFile,
			vars:     map[string]string{"arch": "monolith", "lang": "go"},
			want:     "system/monolith/go/VERSION",
		},
		{
			name:     "DestPipelineStageWf",
			template: Names.DestPipelineStageWf,
			vars:     map[string]string{"stage": "qa-signoff"},
			want:     "qa-signoff.yml",
		},
		{
			name:     "MonolithImageRef",
			template: Names.MonolithImageRef,
			vars:     map[string]string{"lang": "typescript"},
			want:     "monolith-system-typescript",
		},
	}
	for _, tc := range cases {
		got := Expand(tc.template, tc.vars)
		if got != tc.want {
			t.Errorf("%s: Expand=%q, want %q", tc.name, got, tc.want)
		}
	}
}

// forEachNameField walks the anonymous Names struct via reflection and calls
// fn(fieldName, value) for every string field.
func forEachNameField(fn func(field, value string)) {
	v := reflect.ValueOf(Names)
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() != reflect.String {
			continue
		}
		fn(typ.Field(i).Name, f.String())
	}
}
