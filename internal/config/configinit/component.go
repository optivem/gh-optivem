// component.go owns the `kind: component` half of `gh optivem config init`.
//
// It deliberately does NOT route through config.ValidateAndDeriveForYAML /
// optivemyaml.BuildOptivemYAML like the system half does. That pipeline exists
// to derive a whole SUT — Sonar project keys per tier, the canonical Family B
// paths: block, the flat scaffold tier layout — from owner + repo + arch +
// repo-strategy. A component project has none of those inputs and none of
// those outputs: it declares one unit of code and nothing else. Threading a
// component through the system pipeline would mean teaching every derivation
// step to no-op, which is the cascade this whole feature exists to remove.
//
// Both the flag path and the interactive path funnel through BuildComponent so
// they cannot drift, and both validate through projectconfig.Validate — the
// same rules `gh optivem config validate` applies — so an accept/reject
// decision is identical whichever surface produced the value.
package configinit

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/kernel/gitignore"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// ComponentFlags is the input set for a kind: component config — the four
// component fields plus the tracker identity every project carries
// regardless of kind.
//
// Provider is left to the caller to default (the CLI binds it to github,
// matching the system path's hardcoded ProviderGitHub); an empty value is
// rejected by projectconfig.Validate Rule 19 rather than silently defaulted
// here, so a caller that forgets is told.
type ComponentFlags struct {
	Name       string
	Path       string
	Repo       string
	Lang       string
	Provider   string
	ProjectURL string
}

// BuildComponent renders ComponentFlags as a validated
// *projectconfig.Config. Validation is projectconfig.Validate itself rather
// than a parallel copy of its rules — the file this writes must be one that
// `gh optivem config validate` accepts, and the only way to guarantee that is
// to run the same function.
func BuildComponent(f ComponentFlags) (*projectconfig.Config, error) {
	pc := &projectconfig.Config{
		Kind: projectconfig.KindComponent,
		Project: projectconfig.Project{
			Provider: f.Provider,
			URL:      f.ProjectURL,
		},
		Component: projectconfig.Component{
			Name: f.Name,
			Path: f.Path,
			Repo: f.Repo,
			Lang: f.Lang,
		},
	}
	if err := pc.Validate(); err != nil {
		return nil, err
	}
	return pc, nil
}

// RunComponent is the testable core of `gh optivem config init --kind
// component`: build, refuse to clobber, write, and emit the same .gitignore
// side-effect the system path emits (the runtime's .gh-optivem/ state dir is
// a foot-gun if committed, and that is true of a component project too).
// Returns yamlPath on success.
func RunComponent(f ComponentFlags, yamlPath string, force bool) (string, error) {
	pc, err := BuildComponent(f)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(yamlPath); err == nil && !force {
		return "", fmt.Errorf("%s already exists; pass --force to overwrite", yamlPath)
	}
	if err := projectconfig.WriteToPath(yamlPath, pc); err != nil {
		return "", err
	}
	if err := gitignore.EnsureLine(filepath.Dir(yamlPath), ".gh-optivem/"); err != nil {
		return "", fmt.Errorf("ensure .gitignore: %w", err)
	}
	return yamlPath, nil
}

// PromptComponent collects the kind: component field set interactively.
// Mirrors Prompt's contract exactly — one line at a time from in, prompts and
// validator errors echoed to out, a bad value re-asks just that field — and
// runs the same validators the flag path runs, so the two surfaces cannot
// diverge on what they accept.
//
// It asks four questions, not nine: a component project has no architecture,
// no repo-strategy, no test language, and no license or deploy target to
// choose. That is the whole point of the kind.
func PromptComponent(in io.Reader, out io.Writer) (ComponentFlags, error) {
	r := bufio.NewReader(in)
	f := ComponentFlags{Provider: projectconfig.ProviderGitHub}

	fmt.Fprintln(out, "Enter values for each prompt; bad input re-asks.")

	if err := ask(r, out, "Component name (the --component handle), e.g. backend-clean-java", func(v string) (string, error) {
		if v == "" {
			return "", fmt.Errorf("must not be empty")
		}
		f.Name = v
		return v, nil
	}); err != nil {
		return ComponentFlags{}, err
	}
	if err := ask(r, out, "Repo-relative path to the component, e.g. system/multitier/backend-clean-java", func(v string) (string, error) {
		f.Path = v
		// Validate the path through the schema itself: build a throwaway
		// config carrying just this field and read back the rule that fires.
		if err := (&projectconfig.Config{Kind: projectconfig.KindComponent,
			Component: projectconfig.Component{Name: "x", Path: v, Repo: "x/x", Lang: projectconfig.LangJava},
			Project:   projectconfig.Project{Provider: projectconfig.ProviderGitHub}}).Validate(); err != nil {
			return "", err
		}
		return v, nil
	}); err != nil {
		return ComponentFlags{}, err
	}
	if err := ask(r, out, "Repo slug the component lives in, e.g. acme/shop", func(v string) (string, error) {
		if v == "" {
			return "", fmt.Errorf("must not be empty")
		}
		f.Repo = v
		return v, nil
	}); err != nil {
		return ComponentFlags{}, err
	}
	if err := askChoice(r, out, "Component language", langChoices, 0, func(v string) error {
		if msg := config.ValidateBackendLang(v); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		f.Lang = v
		return nil
	}); err != nil {
		return ComponentFlags{}, err
	}
	return f, nil
}
