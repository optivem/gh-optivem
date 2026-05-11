// prompt.go drives the interactive flag-collection used when EnsureExists
// hits a missing gh-optivem.yaml on a TTY. The set of questions mirrors
// config.BindConfigInitFlags (the explicit-paths contract `config init`
// already enforces) — no scaffolder-style flat-layout defaults, no git-
// remote inference, no env-token reads. Each answer is validated against
// the same validator the `config init` command runs after Cobra parsing,
// so a bad value re-asks just that field rather than discarding the whole
// session.
package configinit

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
)

// Prompt collects the full `config init` required-flag set interactively
// and returns a populated RawFlags. Reads one line at a time from in;
// echoes prompts and validator errors to out. Re-asks the same field on
// validation failure; prior answers in the session are kept. Returns
// io.EOF (wrapped) if stdin closes mid-session, so callers can fall back
// to the terse error.
func Prompt(in io.Reader, out io.Writer) (*config.RawFlags, error) {
	r := bufio.NewReader(in)
	f := &config.RawFlags{}

	fmt.Fprintln(out, "No gh-optivem.yaml found — let's create one. Enter values for each prompt; bad input re-asks.")

	// --owner: GitHub username/org, validated against GitHub's naming rules.
	if err := ask(r, out, "GitHub owner (user or org)", func(v string) (string, error) {
		if msg := config.ValidateOwnerFormat(v); msg != "" {
			return "", fmt.Errorf("%s", msg)
		}
		f.Owner = v
		return v, nil
	}); err != nil {
		return nil, err
	}

	// --repo: repository name (without owner prefix).
	if err := ask(r, out, "Repository name (without owner prefix)", func(v string) (string, error) {
		if msg := config.ValidateRepoFormat(v); msg != "" {
			return "", fmt.Errorf("%s", msg)
		}
		f.Repo = v
		return v, nil
	}); err != nil {
		return nil, err
	}

	// --arch: monolith | multitier — gates the lang and path branches.
	if err := ask(r, out, "Architecture (monolith|multitier)", func(v string) (string, error) {
		if v != "monolith" && v != "multitier" {
			return "", fmt.Errorf("must be 'monolith' or 'multitier'")
		}
		f.Arch = v
		return v, nil
	}); err != nil {
		return nil, err
	}

	// --repo-strategy: monorepo | multirepo. Independent of arch.
	if err := ask(r, out, "Repo strategy (monorepo|multirepo)", func(v string) (string, error) {
		if v != "monorepo" && v != "multirepo" {
			return "", fmt.Errorf("must be 'monorepo' or 'multirepo'")
		}
		f.RepoStrategy = v
		return v, nil
	}); err != nil {
		return nil, err
	}

	if err := promptLangFlags(r, out, f); err != nil {
		return nil, err
	}
	if err := promptPathFlags(r, out, f); err != nil {
		return nil, err
	}

	// --project-url: required. The Validate rule in projectconfig rejects
	// an empty url, so accepting it here would only push the failure to
	// the write step a few lines later.
	if err := ask(r, out, "GitHub Project URL", func(v string) (string, error) {
		if !strings.HasPrefix(v, "https://github.com/") {
			return "", fmt.Errorf("must be a https://github.com/... URL")
		}
		f.ProjectURL = v
		return v, nil
	}); err != nil {
		return nil, err
	}

	return f, nil
}

// promptLangFlags asks the lang questions that apply to the chosen arch.
// Monolith needs --monolith-lang; multitier needs --backend-lang +
// --frontend-lang. The valid set matches resolveLangsForYAML.
func promptLangFlags(r *bufio.Reader, out io.Writer, f *config.RawFlags) error {
	validBackend := func(v string) bool {
		return v == "java" || v == "dotnet" || v == "typescript"
	}
	switch f.Arch {
	case "monolith":
		return ask(r, out, "Monolith language (java|dotnet|typescript)", func(v string) (string, error) {
			if !validBackend(v) {
				return "", fmt.Errorf("must be 'java', 'dotnet', or 'typescript'")
			}
			f.Lang = v
			return v, nil
		})
	case "multitier":
		if err := ask(r, out, "Backend language (java|dotnet|typescript)", func(v string) (string, error) {
			if !validBackend(v) {
				return "", fmt.Errorf("must be 'java', 'dotnet', or 'typescript'")
			}
			f.BackendLang = v
			return v, nil
		}); err != nil {
			return err
		}
		return ask(r, out, "Frontend language (react)", func(v string) (string, error) {
			if v != "react" {
				return "", fmt.Errorf("must be 'react'")
			}
			f.FrontendLang = v
			return v, nil
		})
	}
	return fmt.Errorf("unreachable: arch %q passed validation but no lang branch", f.Arch)
}

// promptPathFlags asks the path questions that apply to the chosen arch.
// Mirrors validatePathFlagsForYAML's required set: system-test, stubs,
// simulators always; system-path on monolith; backend-path + frontend-path
// on multitier.
func promptPathFlags(r *bufio.Reader, out io.Writer, f *config.RawFlags) error {
	nonEmpty := func(name string) func(string) (string, error) {
		return func(v string) (string, error) {
			if v == "" {
				return "", fmt.Errorf("--%s is required", name)
			}
			return v, nil
		}
	}
	if err := ask(r, out, "System tests path (repo-relative)", func(v string) (string, error) {
		out, err := nonEmpty("system-test-path")(v)
		if err != nil {
			return "", err
		}
		f.SystemTestPath = out
		return out, nil
	}); err != nil {
		return err
	}
	if err := ask(r, out, "Stubs path (repo-relative)", func(v string) (string, error) {
		out, err := nonEmpty("stubs-path")(v)
		if err != nil {
			return "", err
		}
		f.StubsPath = out
		return out, nil
	}); err != nil {
		return err
	}
	if err := ask(r, out, "Simulators path (repo-relative)", func(v string) (string, error) {
		out, err := nonEmpty("simulators-path")(v)
		if err != nil {
			return "", err
		}
		f.SimulatorsPath = out
		return out, nil
	}); err != nil {
		return err
	}
	switch f.Arch {
	case "monolith":
		return ask(r, out, "System path (repo-relative)", func(v string) (string, error) {
			out, err := nonEmpty("system-path")(v)
			if err != nil {
				return "", err
			}
			f.SystemPath = out
			return out, nil
		})
	case "multitier":
		if err := ask(r, out, "Backend path (repo-relative)", func(v string) (string, error) {
			out, err := nonEmpty("backend-path")(v)
			if err != nil {
				return "", err
			}
			f.BackendPath = out
			return out, nil
		}); err != nil {
			return err
		}
		return ask(r, out, "Frontend path (repo-relative)", func(v string) (string, error) {
			out, err := nonEmpty("frontend-path")(v)
			if err != nil {
				return "", err
			}
			f.FrontendPath = out
			return out, nil
		})
	}
	return nil
}

// ask prints a single prompt and loops until accept returns nil. The
// caller's accept closure both validates and stores the value into the
// RawFlags. EOF (no more input — pipe closed, ^D, tests running out of
// fixture lines) is surfaced verbatim so the missing-file recovery can
// abort with the terse-error fallback rather than spin forever.
func ask(r *bufio.Reader, out io.Writer, label string, accept func(string) (string, error)) error {
	for {
		fmt.Fprintf(out, "  %s: ", label)
		line, err := r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read %s: %w", label, err)
		}
		v := strings.TrimSpace(line)
		if v == "" {
			fmt.Fprintln(out, "    value cannot be empty")
			continue
		}
		if _, err := accept(v); err != nil {
			fmt.Fprintf(out, "    %s\n", err)
			continue
		}
		return nil
	}
}

