package config

import (
	"fmt"
	"strings"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// FillRawFlagsFromYAML populates RawFlags's YAML-affecting fields from a
// loaded gh-optivem.yaml. `gh optivem init` calls this in runInit before
// ParseAndValidate so the rest of the init pipeline keeps consuming
// RawFlags exactly as it did under the old all-flags path.
//
// Per-invocation flags on f (--verify-level, --workdir, etc.) are left
// untouched. The yaml carries no per-invocation values.
//
// Hard-errors when the yaml is missing fields init needs to scaffold
// (system_name, architecture, project URL, langs, paths). The error
// message names the missing field and points at `gh optivem config init`
// so the operator has one canonical command to run.
func FillRawFlagsFromYAML(f *RawFlags, pc *projectconfig.Config) error {
	if pc == nil {
		return fmt.Errorf("gh-optivem.yaml is required for `gh optivem init`; run `gh optivem config init` first")
	}

	if pc.SystemName == "" {
		return missingYAMLField("system-name")
	}
	if pc.System.Architecture == "" {
		return missingYAMLField("system.architecture")
	}
	// project.url may be empty here: EnsureProjectBoard's Path A
	// (internal/steps/project.go) auto-creates the board, sets
	// cfg.ProjectURL, and the later WriteOptivemYAML step bakes it back
	// into gh-optivem.yaml.
	if pc.RepoStrategy == "" {
		return missingYAMLField("repo-strategy")
	}

	f.SystemName = pc.SystemName
	f.ProjectURL = pc.Project.URL
	f.Arch = mapArchFromYAML(pc.System.Architecture)
	f.RepoStrategy = mapRepoStrategyFromYAML(pc.RepoStrategy)
	f.License = pc.License
	if f.License == "" {
		// Schema accepts absent; init must commit to a value so the
		// scaffolded LICENSE file gets written. Mirror the flag default.
		f.License = projectconfig.LicenseMIT
	}
	f.Deploy = pc.Deploy
	if f.Deploy == "" {
		f.Deploy = projectconfig.DeployDocker
	}

	// Tier paths + langs.
	switch f.Arch {
	case "monolith":
		if pc.System.Path == "" || pc.System.Repo == "" || pc.System.Lang == "" {
			return missingYAMLField("system.{path,repo,lang}")
		}
		f.SystemPath = pc.System.Path
		f.Lang = pc.System.Lang
	case "multitier":
		if pc.System.Backend.IsEmpty() || pc.System.Frontend.IsEmpty() {
			return missingYAMLField("system.backend / system.frontend")
		}
		f.BackendPath = pc.System.Backend.Path
		f.FrontendPath = pc.System.Frontend.Path
		f.BackendLang = pc.System.Backend.Lang
		f.FrontendLang = pc.System.Frontend.Lang
	case "microservices":
		// Microservices is YAML-authored only (D7): the service list comes from
		// the backend-services: map (mirroring external-systems:), never from
		// flags. Project Validate has already enforced the shape (>=1 service,
		// per-service completeness, single frontend); fill the RawFlags carrier
		// in sorted-name order so the scaffolder iterates deterministically.
		if len(pc.System.BackendServices) == 0 {
			return missingYAMLField("system.backend-services")
		}
		if pc.System.Frontend.IsEmpty() {
			return missingYAMLField("system.frontend")
		}
		for _, name := range pc.BackendServiceNames() {
			svc := pc.System.BackendServices[name]
			f.BackendServices = append(f.BackendServices, BackendService{
				Name:         name,
				Path:         svc.Path,
				Lang:         svc.Lang,
				Repo:         svc.Repo,
				SonarProject: svc.SonarProject,
			})
		}
		f.FrontendPath = pc.System.Frontend.Path
		f.FrontendLang = pc.System.Frontend.Lang
		f.FrontendRepoSlug = pc.System.Frontend.Repo
	}

	if pc.SystemTest.IsEmpty() {
		return missingYAMLField("system-test")
	}
	f.SystemTestPath = pc.SystemTest.Path
	f.TestLang = pc.SystemTest.Lang

	// external-systems is operator-owned and omitted by `gh optivem init`
	// (plan 20260606-1356, option 1B): it is a per-system map with no flat
	// scaffold path to recover, so re-init neither requires nor reads it.

	owner, repo, err := workspaceRepoFromYAML(pc)
	if err != nil {
		return err
	}
	f.Owner = owner
	f.Repo = repo
	return nil
}

// workspaceRepoFromYAML extracts the workspace owner + base repo name
// from the tier-repo slugs. There is no top-level `repo:` field in
// gh-optivem.yaml — the workspace base name lives only in the tier
// slugs, derived per repo strategy:
//
//   - mono-repo: every tier shares the same `owner/repo` slug; split it.
//   - multi-repo monolith: system.repo is `owner/<repo>-system`; strip the
//     `-system` suffix.
//   - multi-repo multitier: system.backend.repo is `owner/<repo>-backend`;
//     strip the `-backend` suffix.
//
// This mirrors deriveMultirepoNames (the forward direction). A slug
// missing the expected suffix is a hard error — the yaml's per-tier
// repos were edited by hand into a shape `gh optivem init` cannot
// reconstruct, and silently guessing a base name would commit a wrong
// repo on GitHub.
func workspaceRepoFromYAML(pc *projectconfig.Config) (owner, repo string, err error) {
	// Pick the anchor tier slug + its multi-repo suffix per architecture:
	//   - monolith:      system.repo,          `-system`
	//   - multitier:     system.backend.repo,  `-backend`
	//   - microservices: system.frontend.repo, `-frontend`
	//     (the single frontend is the one stable tier — backend-services is a
	//     map of N services with no single backend slug to anchor on, D5).
	var slug, suffix, missingHint string
	switch pc.System.Architecture {
	case projectconfig.ArchMonolith:
		slug, suffix, missingHint = pc.System.Repo, "-system", "system.repo (monolith)"
	case projectconfig.ArchMicroservices:
		slug, suffix, missingHint = pc.System.Frontend.Repo, "-frontend", "system.frontend.repo (microservices)"
	default:
		slug, suffix, missingHint = pc.System.Backend.Repo, "-backend", "system.backend.repo (multitier)"
	}
	if slug == "" {
		return "", "", missingYAMLField(missingHint)
	}
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("gh-optivem.yaml: tier repo slug %q must be `owner/name`", slug)
	}
	owner = parts[0]
	name := parts[1]
	if pc.RepoStrategy == projectconfig.RepoStrategyMultiRepo {
		if !strings.HasSuffix(name, suffix) {
			return "", "", fmt.Errorf("gh-optivem.yaml: multi-repo tier slug %q must end in %q for `gh optivem init` to derive the workspace repo name; edit the slug or rerun `gh optivem config init`",
				slug, suffix)
		}
		name = strings.TrimSuffix(name, suffix)
	}
	return owner, name, nil
}

// mapArchFromYAML translates projectconfig spelling (monolith|multitier)
// to internal/config's RawFlags spelling. The two are already identical;
// the function is here to document the seam and to fail loudly if the
// yaml carries an unexpected value (Validate should catch this first,
// but defense in depth costs nothing).
func mapArchFromYAML(arch string) string {
	switch arch {
	case projectconfig.ArchMonolith:
		return "monolith"
	case projectconfig.ArchMultitier:
		return "multitier"
	case projectconfig.ArchMicroservices:
		return "microservices"
	default:
		return arch
	}
}

// mapRepoStrategyFromYAML translates projectconfig spelling
// (mono-repo|multi-repo) to RawFlags spelling (monorepo|multirepo). The
// schema and the init flag historically picked different separators; the
// translation lives at the load boundary so the rest of init keeps
// seeing the flag spelling it always saw.
func mapRepoStrategyFromYAML(s string) string {
	switch s {
	case projectconfig.RepoStrategyMonoRepo:
		return "monorepo"
	case projectconfig.RepoStrategyMultiRepo:
		return "multirepo"
	default:
		return s
	}
}

func missingYAMLField(field string) error {
	return fmt.Errorf("gh-optivem.yaml is missing required field %s for `gh optivem init`; rerun `gh optivem config init` or hand-edit the file", field)
}
