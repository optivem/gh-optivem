// config_commands.go wires the `gh optivem config …` subcommands into the
// root Cobra command. The `config` namespace owns operations that read or
// write gh-optivem.yaml — the central per-project config file produced by
// `gh optivem init` and consumed by `gh optivem implement`.
//
//	gh optivem config init      — write a fresh gh-optivem.yaml from CLI flags
//	gh optivem config validate  — parse <CWD>/gh-optivem.yaml and validate it
//	gh optivem config preflight — validate + check on-disk layout exists
//	gh optivem config migrate   — back-fill required fields onto a pre-schema-bump config
//
// `config init` reuses the same render path as `gh optivem init`
// (steps.WriteOptivemYAMLToPath / config.ValidateAndDeriveForYAML) so a new
// YAML-affecting flag flows to both surfaces with no per-command duplication.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/preflight"
	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/configinit"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// newConfigCmd builds the `gh optivem config` parent. The parent has no Run,
// so invoking it without a subcommand prints help (Cobra default).
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage gh-optivem.yaml in a consumer repo",
		Long: `Manage gh-optivem.yaml — the per-project configuration file consumed by
the ATDD pipeline (project URL, repo strategy, scope axes).

Normally produced by ` + "`gh optivem init`" + `; these subcommands let you
write or validate the file standalone (e.g. retrofitting it into a
non-scaffolded repo, or re-validating after a hand edit).`,
	}
	cmd.AddCommand(
		newConfigInitCmd(),
		newConfigValidateCmd(),
		newConfigPreflightCmd(),
		newConfigMigrateCmd(),
	)
	return cmd
}

// newConfigInitCmd implements `gh optivem config init`. Writes a fresh
// gh-optivem.yaml from CLI flags. Refuses to overwrite an existing file
// unless --force is passed (the file may be hand-edited; silent overwrite
// is a foot-gun).
//
// TODO: document the standalone retrofit flow (running `config init`
// from inside a hand-rolled, non-scaffolded repo) in the README once
// the UX is validated. For now the README leads with `gh optivem init`,
// which folds in the same prompt via configinit.EnsureExists.
//
// Validations run before the file is written, in two phases: (1) format
// (owner naming rules, license key, arch/repo-strategy enums, project
// URL shape) and (2) existence (owner resolves as a real GitHub user or
// org; project URL — when supplied — resolves to a real Project v2 the
// caller can read). The interactive prompt path shares the same
// validators, so flag-driven and interactive `config init` produce the
// same accept/reject decisions on every field.
//
// Target path precedence: persistent --config / -c (or $GH_OPTIVEM_CONFIG)
// > --dir > current working directory. --config names an exact target
// file (any filename); --dir names a parent directory and the canonical
// `gh-optivem.yaml` filename is appended.
func newConfigInitCmd() *cobra.Command {
	f := &config.RawFlags{}
	var (
		force bool
		dir   string
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write a fresh gh-optivem.yaml in the current repo",
		Long: `Write a fresh gh-optivem.yaml from the supplied flags.

Target path precedence: --config <path> (also honored as $GH_OPTIVEM_CONFIG)
> --dir <dir> (writes <dir>/gh-optivem.yaml) > current working directory.

Refuses to overwrite an existing file unless --force is passed. The
file is the single source of truth for the tool and may be hand-edited;
silent overwrite would be a foot-gun.`,
		Example: `  # Monolith, Java
  gh optivem config init --owner acme --repo page-turner \
      --arch monolith --repo-strategy monorepo --monolith-lang java \
      --project-url https://github.com/orgs/acme/projects/1

  # Write to a non-default filename (for repos with a multi-combination matrix)
  gh optivem -c ./gh-optivem.monolith-java.yaml config init --owner acme ...

  # Overwrite an existing file
  gh optivem config init --owner acme ... --force`,
		Run: func(cmd *cobra.Command, args []string) {
			yamlPath, err := configinit.ResolveTarget(projectConfigPath, dir)
			exitOnError(err)
			// No required YAML flags + TTY → drop into the same Prompt path
			// EnsureExists uses for missing-file recovery. Non-TTY falls
			// through to configinit.Run and surfaces the existing
			// "required flags" error from ValidateAndDeriveForYAML.
			if noRequiredConfigInitFlagsSet(f) && isatty.IsTerminal(os.Stdin.Fd()) {
				// Fail fast before entering the prompt session: otherwise
				// the operator fills in every field only for runWithBanner
				// to refuse at the very end. Same error string the flag
				// path produces — runWithBanner still re-checks under the
				// covers so this isn't load-bearing for correctness, just
				// UX.
				if _, statErr := os.Stat(yamlPath); statErr == nil && !force {
					exitOnError(fmt.Errorf("%s already exists; pass --force to overwrite", yamlPath))
				}
				if force {
					fmt.Fprintf(os.Stderr, "Overwriting %s interactively (--force).\n", yamlPath)
				} else {
					fmt.Fprintf(os.Stderr, "Creating %s interactively.\n", yamlPath)
				}
				prompted, perr := configinit.Prompt(os.Stdin, os.Stderr)
				exitOnError(perr)
				written, werr := configinit.RunWithBanner(prompted, yamlPath, force, configinit.Banner)
				exitOnError(werr)
				fmt.Printf("Wrote %s\n", written)
				return
			}
			written, err := configinit.Run(f, yamlPath, force)
			exitOnError(err)
			fmt.Printf("Wrote %s\n", written)
		},
	}
	config.BindConfigInitFlags(cmd, f)
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing gh-optivem.yaml")
	cmd.Flags().StringVar(&dir, "dir", "", "Directory to write gh-optivem.yaml into (ignored if --config is set; default: current working directory)")
	return cmd
}

// newConfigValidateCmd implements `gh optivem config validate`. Reads the
// gh-optivem.yaml at the path resolved by the persistent --config / -c flag
// (or $GH_OPTIVEM_CONFIG, or cwd) and runs it through projectconfig.Validate
// (LoadFromPath invokes Validate internally — successful load = valid file).
// Surfaces the existing-but-otherwise-unreachable Validate capability so
// anyone hand-editing the YAML can check it before running implement-ticket.
func newConfigValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate gh-optivem.yaml in the current repo",
		Long: `Validate gh-optivem.yaml against the projectconfig schema. Exits 0
when valid, non-zero with the validation error otherwise.

The target file is resolved via the persistent --config / -c flag
(or $GH_OPTIVEM_CONFIG, or ./gh-optivem.yaml).

Coverage includes the SonarCloud block (when system.architecture is
set): sonar.organization plus sonar_project on every code tier (system
or backend+frontend, plus system_test) must be present. The YAML is
the source of truth for these keys — the scaffolder seeds defaults via
DeriveSonarProjects but the values may be hand-edited afterwards (e.g.
multi-stack reference repos that need per-variant SonarCloud projects
the single-stack deriver cannot express).`,
		Example: `  gh optivem config validate
  gh optivem -c ./gh-optivem.myrepo-monolith.yaml config validate`,
		Run: func(cmd *cobra.Command, args []string) {
			path, _ := projectconfig.ResolvePath(projectConfigPath)
			validated, err := runConfigValidate(path)
			exitOnError(err)
			fmt.Printf("%s is valid\n", validated)
		},
	}
	return cmd
}

// noRequiredConfigInitFlagsSet reports whether the operator passed none of
// the five required YAML-affecting flags. Trigger for `config init` to
// drop into the interactive Prompt (on a TTY) instead of erroring with
// "required flags: --owner, --repo, …". Mirrors the precondition in
// config.ValidateAndDeriveForYAML.
func noRequiredConfigInitFlagsSet(f *config.RawFlags) bool {
	return f.Owner == "" && f.Repo == "" && f.SystemName == "" && f.Arch == "" && f.RepoStrategy == ""
}

// runConfigValidate is the testable core of `gh optivem config validate`. It
// runs EnsureExists (which on a TTY offers to create the file
// interactively) and then validates via projectconfig.LoadFromPath.
// Missing file on a non-TTY returns the terse error pointing the user at
// `gh optivem config init`.
func runConfigValidate(yamlPath string) (string, error) {
	if err := configinit.EnsureExists(yamlPath); err != nil {
		return "", err
	}
	if _, err := projectconfig.LoadFromPath(yamlPath); err != nil {
		return "", err
	}
	return yamlPath, nil
}

// newConfigPreflightCmd implements `gh optivem config preflight`. Runs the
// same schema validation as `config validate`, then the on-disk preflight
// check (every declared repo and tier path actually exists in the
// workspace). Surfaces the late "preflight failed" errors that otherwise
// only fire deep inside `implement`.
//
// Schema-only validation stays on `config validate`: that command must keep
// passing for the half-built state right after `gh optivem config init`,
// where the YAML is well-formed but the sibling repos haven't been cloned
// yet. `preflight` is the stronger contract — "I'm about to actually use
// this config" — and is expected to fail in that intermediate state.
func newConfigPreflightCmd() *cobra.Command {
	var workspace string
	cmd := &cobra.Command{
		Use:   "preflight",
		Short: "Validate gh-optivem.yaml and check its declared paths exist on disk",
		Long: `Run schema validation (same as ` + "`config validate`" + `) and additionally
verify that every repo and tier path declared in gh-optivem.yaml resolves
to a real directory on disk. This is the same check ` + "`implement`" + `
runs at startup — run it standalone to catch missing clones or mistyped
paths before kicking off a pipeline.

Exits 0 when both schema and on-disk layout check out, non-zero with one
multi-line error block listing every failure otherwise.`,
		Example: `  gh optivem config preflight
  gh optivem config preflight --workspace /abs/path/to/workspace
  gh optivem -c ./gh-optivem.myrepo-monolith.yaml config preflight`,
		Run: func(cmd *cobra.Command, args []string) {
			path, _ := projectconfig.ResolvePath(projectConfigPath)
			cwd, err := os.Getwd()
			exitOnError(err)
			validated, err := runConfigPreflight(path, func(cfg *projectconfig.Config) (preflight.Options, error) {
				return defaultPreflightOptions(cfg, workspace, cwd)
			})
			exitOnError(err)
			fmt.Printf("%s passes preflight\n", validated)
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root containing one clone per repo (default: parent directory of CWD). Each clone dir must be named after the repo-name component of its slug; symlink outliers into place.")
	return cmd
}

// newConfigMigrateCmd implements `gh optivem config migrate`. Idempotently
// back-fills required fields onto a pre-schema-bump gh-optivem.yaml:
//
//   - project.provider, inferred from the existing project.url shape
//     (https://github.com/... → github; everything else → markdown).
//   - repos:, the project-internal repo path list consumed by the
//     workspace scope cascade. Inferred from the tier repo slugs for
//     multi-repo projects; mono-repo projects are left untouched
//     (single-repo behavior already covers them).
//   - external-driver key rename (plan 20260519-0704): renames the
//     pre-rename keys under paths: to their post-rename forms. Hard
//     errors when both old and new keys are present.
//   - SSoT path model: joins system.sut_namespace into each paths:<key>
//     value and into system.path, then deletes system.sut_namespace.
//     Applied when system.sut_namespace is present.
//
// When no back-fill applies the file is left untouched and the command
// reports a no-op. Designed to be safe to run repeatedly.
//
// Comment preservation: the file is rewritten via yaml.v3's Node API so
// existing comments and key ordering survive — operators who hand-edited
// their gh-optivem.yaml don't lose context on re-run.
func newConfigMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Back-fill required fields onto an existing gh-optivem.yaml",
		Long: `Migrate gh-optivem.yaml to the current schema. Today:

  • Adds project.provider (` + "`github`" + ` or ` + "`markdown`" + `), inferred from the existing
    project.url shape — https://github.com/... → github; everything else
    → markdown.
  • Adds repos:, the project-internal repo path list consumed by the
    workspace scope cascade. Inferred from the tier repo slugs for
    multi-repo projects (one ../<repo-name> entry per distinct tier
    slug). Mono-repo projects keep their existing single-repo behavior
    and the field is left absent.
  • Renames the pre-rename external-driver keys under system_test.paths:
    to their post-rename forms (` + "`external_driver_port`" + ` →
    ` + "`external_system_driver_port`" + `, ` + "`external_driver_adapter`" + ` →
    ` + "`external_system_driver_adapter`" + `). Hard errors when both old and
    new keys are present — the operator must resolve the ambiguity manually.
  • Migrates to the SSoT path model: joins system.sut_namespace into each
    system_test.paths:<key> value and into system.path, then deletes
    system.sut_namespace. Applied when system.sut_namespace is present in
    the file.

The command is idempotent: when no back-fill applies the file is left
untouched and the command reports "no migration needed".

Existing comments and key ordering are preserved so hand-edited files
keep their context.`,
		Example: `  gh optivem config migrate
  gh optivem -c ./gh-optivem.myrepo.yaml config migrate`,
		Run: func(cmd *cobra.Command, args []string) {
			path, _ := projectconfig.ResolvePath(projectConfigPath)
			changed, err := runConfigMigrate(path)
			exitOnError(err)
			if changed {
				fmt.Printf("%s migrated\n", path)
			} else {
				fmt.Printf("%s already up to date\n", path)
			}
		},
	}
	return cmd
}

// runConfigMigrate is the testable core of `gh optivem config migrate`. It
// reads <path> via yaml.v3 Node round-trip (so comments survive),
// back-fills the supported migration targets (project.provider, repos:),
// and writes the file back when anything changed. Returns (changed,
// err): changed=false means "no-op, file untouched."
//
// Each back-fill step is independent — provider and repos may be added
// in the same run (a config older than both bumps) or in separate runs
// (a config that already has provider but predates repos:).
//
// Ordering: the external-driver key-rename pass runs FIRST so the
// downstream gap-fill and SSoT join see the post-rename names — this
// way a pre-rename pre-SSoT config can land both rename and SSoT join
// in one migrate pass without the SSoT join treating the renamed entry
// as "missing" and re-back-filling it as a duplicate.
func runConfigMigrate(path string) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("config migrate: path is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, projectconfig.MissingFileError(path)
		}
		return false, fmt.Errorf("config migrate: read %s: %w", path, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return false, fmt.Errorf("config migrate: parse %s: %w", path, err)
	}
	doc := documentMappingNode(&root)
	if doc == nil {
		return false, fmt.Errorf("config migrate: %s: top-level document is not a mapping", path)
	}
	projectNode := mappingValue(doc, "project")
	if projectNode == nil || projectNode.Kind != yaml.MappingNode {
		return false, fmt.Errorf("config migrate: %s: missing project block (run `gh optivem config init` to seed it)", path)
	}

	changed := false

	// Back-fill project.provider when absent.
	if mappingValue(projectNode, "provider") == nil {
		url := scalarValue(mappingValue(projectNode, "url"))
		provider := inferProvider(url)
		prependMappingEntry(projectNode, "provider", provider)
		changed = true
	}

	// Back-fill repos: when absent on a multi-repo project. inferRepos
	// returns nil for configs the field shouldn't touch (mono-repo,
	// missing architecture, no tier slugs) so this branch is a no-op for
	// every layout that doesn't need iteration.
	if mappingValue(doc, "repos") == nil {
		paths := inferRepos(doc)
		if len(paths) > 0 {
			appendReposEntry(doc, paths)
			changed = true
		}
	}

	// Rename the pre-rename external-driver keys per plan 20260519-0704
	// (`external_driver_port` → `external_system_driver_port`,
	// `external_driver_adapter` → `external_system_driver_adapter`).
	// Runs before the gap-fill so the back-fill sees the post-rename
	// names and does not re-add the renamed entry as a duplicate.
	// Errors if both old and new keys are present — that's an
	// ambiguous migration state that needs human resolution.
	renamed, err := renameExternalDriverKeys(doc)
	if err != nil {
		return false, fmt.Errorf("config migrate: %s: %w", path, err)
	}
	if renamed {
		changed = true
	}

	// Per doctrine, migrate does NOT back-fill missing canonical Family B
	// keys with DefaultPaths values — `paths:` is explicit-only, the
	// operator owns every value. A config that's missing canonical keys
	// surfaces at the next `gh optivem` invocation through
	// projectconfig.Validate Rule 22a, which names the gap and points at
	// internal/projectconfig/path-keys.md for the supported set.

	// SSoT join (per plan 20260518-1530 item 6). Applies iff
	// system.sut_namespace is present:
	//
	//   - For each entry in paths:, fold sut_namespace into the value.
	//     Entries matching the pre-SSoT default are rewritten to the
	//     post-SSoT default (so Java's at-test/ct-test get the package
	//     segment baked into the right position); other testkit entries
	//     get sut_namespace appended; at-test/ct-test customized values
	//     are left untouched (the operator owns the package-structure
	//     decision).
	//   - system.path gets sut_namespace appended.
	//   - The system.sut_namespace field is removed.
	//
	// Idempotent — once system.sut_namespace is absent, this branch is
	// a no-op.
	if joinSSoTPaths(doc) {
		changed = true
	}

	if !changed {
		return false, nil
	}
	out, err := yaml.Marshal(&root)
	if err != nil {
		return false, fmt.Errorf("config migrate: marshal: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return false, fmt.Errorf("config migrate: write %s: %w", path, err)
	}
	return true, nil
}

// inferRepos reads the document mapping and returns the project-internal
// repo paths to write into a back-filled repos: field. Returns nil for
// configs where repos: should remain absent: mono-repo (single-repo
// behavior already covers them), absent or unknown repo_strategy, or no
// tier slug pairs to enumerate.
//
// The inference is structural — it reads only existing fields from the
// node tree, no file I/O. For each non-empty tier repo slug (system,
// system.backend, system.frontend, system_test, plus external_systems'
// stubs/simulators) the function adds one entry per *distinct* slug as
// ../<repo-name(slug)> — matching repolocator's sibling-folder
// convention. Entries are deduplicated and returned in the order the
// tiers appear in the schema so the resulting repos: list is stable.
func inferRepos(doc *yaml.Node) []string {
	strategy := scalarValue(mappingValue(doc, "repo_strategy"))
	if strategy != projectconfig.RepoStrategyMultiRepo {
		return nil
	}

	systemNode := mappingValue(doc, "system")
	testNode := mappingValue(doc, "system_test")
	externalNode := mappingValue(doc, "external_systems")

	var slugs []string
	collect := func(s string) {
		if s != "" {
			slugs = append(slugs, s)
		}
	}
	if systemNode != nil && systemNode.Kind == yaml.MappingNode {
		collect(scalarValue(mappingValue(systemNode, "repo")))
		if backend := mappingValue(systemNode, "backend"); backend != nil {
			collect(scalarValue(mappingValue(backend, "repo")))
		}
		if frontend := mappingValue(systemNode, "frontend"); frontend != nil {
			collect(scalarValue(mappingValue(frontend, "repo")))
		}
	}
	if testNode != nil && testNode.Kind == yaml.MappingNode {
		collect(scalarValue(mappingValue(testNode, "repo")))
	}
	if externalNode != nil && externalNode.Kind == yaml.MappingNode {
		if stubs := mappingValue(externalNode, "stubs"); stubs != nil {
			collect(scalarValue(mappingValue(stubs, "repo")))
		}
		if sims := mappingValue(externalNode, "simulators"); sims != nil {
			collect(scalarValue(mappingValue(sims, "repo")))
		}
	}

	seen := map[string]struct{}{}
	var paths []string
	for _, slug := range slugs {
		name := slug
		if idx := strings.LastIndex(slug, "/"); idx >= 0 && idx < len(slug)-1 {
			name = slug[idx+1:]
		}
		p := "../" + name
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}
	return paths
}

// joinSSoTPaths folds the legacy `system.sut_namespace` value into the
// SSoT path shape and removes the field, per plan 20260518-1530 item 6.
// Returns true when anything changed (the caller flips changed=true and
// re-marshals).
//
// When system.sut_namespace is absent the function is a no-op — making
// it idempotent and safe to run on a post-SSoT config. Behaviour:
//
//  1. For each entry in paths:, fold sut_namespace into the value:
//     entries matching DefaultPaths(testLang, systemTestPath, "") (the
//     pre-SSoT shape) are rewritten to DefaultPaths(testLang,
//     systemTestPath, ns) (the post-SSoT shape — this is the only
//     branch that produces a correct Java at-test/ct-test, where ns is
//     a middle package segment rather than a trailing append). Other
//     testkit entries get `/ns` appended on top of the customisation.
//     at-test/ct-test customised values are left untouched — the
//     operator owns the package-structure decision once they've
//     diverged from the default.
//  2. system.path is rewritten to <current> + "/" + ns.
//  3. system.sut_namespace is removed.
func joinSSoTPaths(doc *yaml.Node) bool {
	systemNode := mappingValue(doc, "system")
	if systemNode == nil || systemNode.Kind != yaml.MappingNode {
		return false
	}
	ns := scalarValue(mappingValue(systemNode, "sut_namespace"))
	if ns == "" {
		return false
	}

	// Pre/post defaults for the default-match branch. We re-derive
	// (testLang, systemTestPath) from the doc rather than from a
	// loaded Config — node-level access is what every other migrate
	// step uses and keeps this function offline from projectconfig.Load.
	testLang, systemTestPath := readTestLangAndPath(doc)
	preDefaults := projectconfig.DefaultPaths(testLang, systemTestPath, "")
	postDefaults := projectconfig.DefaultPaths(testLang, systemTestPath, ns)

	if pathsNode := systemTestPathsNode(doc); pathsNode != nil {
		for i := 0; i+1 < len(pathsNode.Content); i += 2 {
			keyNode := pathsNode.Content[i]
			valNode := pathsNode.Content[i+1]
			if valNode.Kind != yaml.ScalarNode {
				continue
			}
			key := keyNode.Value
			cur := valNode.Value
			// Default-match branch — works for all eight canonical keys
			// and is the only branch that produces a correct Java
			// at-test/ct-test (middle-segment package).
			if preDefaults != nil && postDefaults != nil && cur == preDefaults[key] {
				valNode.Value = postDefaults[key]
				continue
			}
			// at-test/ct-test customised values are left untouched.
			// They're sut_namespace-free on TS/dotnet doctrine, and on
			// Java the operator's customisation already encodes the
			// package decision; naive `+/ns` would corrupt the path.
			if key == "at-test" || key == "ct-test" {
				continue
			}
			valNode.Value = cur + "/" + ns
		}
	}

	if pathValueNode := mappingValue(systemNode, "path"); pathValueNode != nil && pathValueNode.Kind == yaml.ScalarNode && pathValueNode.Value != "" {
		pathValueNode.Value = pathValueNode.Value + "/" + ns
	}

	removeMappingEntry(systemNode, "sut_namespace")
	return true
}

// readTestLangAndPath returns the (testLang, systemTestPath) pair the
// SSoT join uses to look up DefaultPaths. Mirrors inferPathDefaults's
// resolution (system_test.lang with fallback to system.lang for
// monolith) — refactored to a sibling helper rather than reused to
// keep the gap-fill and SSoT-join helpers independent.
func readTestLangAndPath(doc *yaml.Node) (string, string) {
	systemTestNode := mappingValue(doc, "system_test")
	if systemTestNode == nil || systemTestNode.Kind != yaml.MappingNode {
		return "", ""
	}
	systemTestPath := scalarValue(mappingValue(systemTestNode, "path"))
	testLang := scalarValue(mappingValue(systemTestNode, "lang"))
	if testLang == "" {
		if systemNode := mappingValue(doc, "system"); systemNode != nil {
			testLang = scalarValue(mappingValue(systemNode, "lang"))
		}
	}
	return testLang, systemTestPath
}

// renameExternalDriverKeys renames the pre-rename external-driver keys
// (`external_driver_port`, `external_driver_adapter`) in the `paths:`
// block to their post-rename forms (`external_system_driver_port`,
// `external_system_driver_adapter`) per plan 20260519-0704. Returns
// true when any key was renamed.
//
// Rules per pair {old, new}:
//   - old present, new absent → rename the key node in place, preserving
//     the value, any inline comments, and the position within the
//     mapping. This is the canonical migration path.
//   - both present → hard error: the config is in an ambiguous state
//     and cannot be migrated deterministically. The operator must pick
//     a winner manually.
//   - only new present → no-op (already migrated).
//   - neither present → no-op.
//
// Comment preservation: editing only the Key node's Value (leaving the
// adjacent value node and the surrounding mapping structure untouched)
// keeps yaml.v3's HeadComment / LineComment / FootComment fields in
// place across the rewrite.
func renameExternalDriverKeys(doc *yaml.Node) (bool, error) {
	pathsNode := systemTestPathsNode(doc)
	if pathsNode == nil {
		return false, nil
	}
	changed := false
	for oldKey, newKey := range projectconfig.ExternalDriverKeyRenames {
		oldNode := mappingKeyNode(pathsNode, oldKey)
		newNode := mappingKeyNode(pathsNode, newKey)
		switch {
		case oldNode != nil && newNode != nil:
			return false, fmt.Errorf(
				"system_test.paths.%s and system_test.paths.%s both present — ambiguous migration state. "+
					"Remove one entry (keep the one with your intended value) and re-run migrate",
				oldKey, newKey)
		case oldNode != nil:
			oldNode.Value = newKey
			changed = true
		}
	}
	return changed, nil
}

// systemTestPathsNode returns the `paths:` mapping nested under
// `system_test:` in the document, or nil when either node is missing
// or not a mapping. Single source for the two migrate-time passes that
// edit `system_test.paths:` (rename + SSoT join) so neither encodes the
// nesting path twice.
func systemTestPathsNode(doc *yaml.Node) *yaml.Node {
	systemTestNode := mappingValue(doc, "system_test")
	if systemTestNode == nil || systemTestNode.Kind != yaml.MappingNode {
		return nil
	}
	pathsNode := mappingValue(systemTestNode, "paths")
	if pathsNode == nil || pathsNode.Kind != yaml.MappingNode {
		return nil
	}
	return pathsNode
}

// mappingKeyNode returns the key node paired with `key` inside m (the
// key node itself, not its value), or nil when key is absent. Mirrors
// mappingValue but returns the other half of the pair so callers that
// need to edit the key in place (e.g. renames) get a handle to it.
func mappingKeyNode(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i]
		}
	}
	return nil
}

// removeMappingEntry deletes the (key, value) pair from m's Content
// where the key node's Value matches key. No-op if absent. Used by the
// SSoT join to retire system.sut_namespace once it has been folded
// into the per-key paths and system.path.
func removeMappingEntry(m *yaml.Node, key string) {
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

// appendReposEntry inserts a `repos:` key at the end of the document
// mapping so back-filled multi-repo configs end with the new list
// rather than burying it among existing keys. Each path becomes a
// {path: <value>} block-style mapping inside the sequence so the
// emitted YAML matches the hand-written shape:
//
//	repos:
//	  - path: ../page-turner-backend
//	  - path: ../page-turner-frontend
//	  - path: ../page-turner-tests
func appendReposEntry(doc *yaml.Node, paths []string) {
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "repos"}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: 0}
	for _, p := range paths {
		entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: 0}
		entry.Content = []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "path"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: p},
		}
		seq.Content = append(seq.Content, entry)
	}
	doc.Content = append(doc.Content, keyNode, seq)
}

// inferProvider picks the provider that matches the existing project.url.
// HTTPS github URLs route to github; an empty url, a relative path, or
// any non-github URL routes to markdown (the escape-hatch backend).
func inferProvider(url string) string {
	if strings.HasPrefix(url, "https://github.com/") || strings.HasPrefix(url, "http://github.com/") {
		return projectconfig.ProviderGitHub
	}
	return projectconfig.ProviderMarkdown
}

// documentMappingNode returns the top-level mapping inside a yaml.Node
// returned from yaml.Unmarshal. Unmarshal wraps the document in a
// DocumentNode whose first content child is the actual root mapping.
func documentMappingNode(root *yaml.Node) *yaml.Node {
	if root == nil {
		return nil
	}
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil
	}
	return root
}

// mappingValue returns the value node paired with key inside m, or nil
// when key is absent. Mapping nodes store keys and values as adjacent
// pairs in Content (key0, value0, key1, value1, …).
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// scalarValue returns n.Value when n is a non-nil scalar, else "".
func scalarValue(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

// prependMappingEntry inserts a (key, value) pair at the start of m's
// Content so the new field appears before any existing keys. Used so a
// back-filled `provider:` lands above `url:` in the rewritten file —
// operators reading the diff see the new field at the top of the
// project block, not buried at the end.
func prependMappingEntry(m *yaml.Node, key, value string) {
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	m.Content = append([]*yaml.Node{keyNode, valNode}, m.Content...)
}

// runConfigPreflight is the testable core of `gh optivem config preflight`.
// Mirrors runConfigValidate's EnsureExists + LoadFromPath chain, then
// delegates to preflight.Run for the on-disk + remote check.
//
// optsFor is a factory invoked once cfg has loaded successfully: the
// cobra command path returns defaultPreflightOptions (real remote
// clients + SONAR_TOKEN check); tests return a bare Options with just
// Workspace/Cwd set, so the test surface stays offline. The factory
// gets to see cfg so it can decide whether SonarCloud wiring applies
// (cfg.Sonar.Organization set) and surface a clean SONAR_TOKEN-missing
// error before any remote calls fire.
func runConfigPreflight(yamlPath string, optsFor func(*projectconfig.Config) (preflight.Options, error)) (string, error) {
	if err := configinit.EnsureExists(yamlPath); err != nil {
		return "", err
	}
	cfg, err := projectconfig.LoadFromPath(yamlPath)
	if err != nil {
		return "", err
	}
	opts, err := optsFor(cfg)
	if err != nil {
		return "", err
	}
	if err := preflight.Run(context.Background(), cfg, opts); err != nil {
		return "", err
	}
	return yamlPath, nil
}
