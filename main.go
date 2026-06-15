// gh-optivem: A gh CLI extension for pipeline project management.
//
// Usage:
//
//	Monolith:
//	  gh optivem init --owner acme --system-name "Page Turner" --repo page-turner \
//	      --arch monolith --monolith-lang java
//
//	Multitier:
//	  gh optivem init --owner acme --system-name "Page Turner" --repo page-turner \
//	      --arch multitier --backend-lang java --frontend-lang typescript
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/atdd/process/configcheck"
	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/config/configinit"
	"github.com/optivem/gh-optivem/internal/devworkflow/workspace"
	"github.com/optivem/gh-optivem/internal/kernel/approval"
	"github.com/optivem/gh-optivem/internal/kernel/cmdctx"
	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
	"github.com/optivem/gh-optivem/internal/kernel/shell"
	"github.com/optivem/gh-optivem/internal/kernel/version"
	"github.com/optivem/gh-optivem/internal/scaffolding/steps"
)

// bugReportLogMaxBytes is the inline log budget for a filed issue body.
// GitHub caps issue bodies at 65,536 chars; 50 KB leaves comfortable room
// for the metadata block plus the <details> wrapper. The whole log is
// embedded when it fits; otherwise the trailing 50 KB (trimmed to the next
// newline) is embedded with a "truncated" note.
const bugReportLogMaxBytes = 50 * 1024

const separator = "=========================================="

// projectConfigPath holds the value of the persistent root --config / -c
// flag (or empty when the user didn't pass it). Read by every subcommand
// that needs to locate gh-optivem.yaml — they pass it through
// projectconfig.ResolvePath for the flag > env > default cascade.
var projectConfigPath string

// autoFlag / confirmFlag back the root-level --auto / --confirm persistent
// flags. They feed approval.Resolve in PersistentPreRunE; the resulting
// Resolved is stashed on cmd.Context() via cmdctx.WithApproval so every
// y/n confirmation site reads the same policy snapshot regardless of
// where in the command tree it sits.
var (
	autoFlag    bool
	confirmFlag string
)

type stepDef struct {
	name      string
	phase     string
	alwaysRun bool
	// failHard short-circuits the whole pipeline on failure: subsequent steps
	// are skipped even if they are alwaysRun. Used by lint-style steps where a
	// failure means the local scaffold is broken and pushing it would just
	// publish bad output (e.g. invalid YAML in a scaffolded workflow).
	failHard bool
	fn       func()
}

func main() {
	// Handle --version before Cobra and asset-sync. One Write so a
	// downstream `| head -n 1` can't trigger SIGPIPE mid-template (Cobra's
	// text/template emits the value and the trailing newline as separate
	// syscalls; the second one races with head's exit). Also keeps
	// --version side-effect-free — no filesystem sync, no stderr notice.
	for _, a := range os.Args[1:] {
		if a == "--" {
			break
		}
		if a == "--version" {
			io.WriteString(os.Stdout, version.Full()+"\n")
			return
		}
	}

	// Show subcommands in registration order (workflow order), not alphabetical.
	// e.g. `config` lists init → validate → preflight, the sequence a user runs.
	cobra.EnableCommandSorting = false

	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// newRootCmd builds the top-level `optivem` Cobra command and attaches every
// subcommand. Cobra handles --help/-h on every level, --version at the root,
// shell completion (via the auto-added `completion` subcommand), and unknown-
// subcommand suggestions. Subcommands still call os.Exit on validation
// failures, so Execute returns an error only for Cobra-level problems
// (unknown flag, malformed args).
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "optivem",
		Short:   "Scaffold and operate Optivem pipeline projects",
		Version: version.Full(),
		// Subcommands print their own usage on validation errors via log.FatalExit;
		// avoid double-printing usage when Cobra returns an error.
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return resolveApprovalForCmd(cmd)
		},
	}
	cmd.SetVersionTemplate("{{.Version}}\n")
	// Pre-register --version without the -v shorthand so `init -v` keeps
	// resolving to --verbose. Cobra's InitDefaultVersionFlag detects the
	// flag is already present and skips adding its default `-v` alias.
	cmd.Flags().Bool("version", false, "Print version and exit")
	cmd.PersistentFlags().StringVarP(&projectConfigPath, "config", "c", "",
		"Path to gh-optivem.yaml (default: $GH_OPTIVEM_CONFIG or ./gh-optivem.yaml)")
	// --workspace is persistent at root because the cross-repo verbs
	// (commit / sync / actions / lint-history / stale-branches) all
	// consume it identically via workspace.Resolve. The flag is keyed off
	// the var in cross_repo_commands.go so the verbs read the same value
	// regardless of where in the command tree they were invoked from.
	cmd.PersistentFlags().StringVar(&workspaceFlagValue, "workspace", "",
		"Path to a directory containing a *.code-workspace file (default: $"+workspace.EnvVar+", walk-up from CWD, or single-repo fallback)")
	cmd.PersistentFlags().BoolVar(&autoFlag, "auto", false,
		"Auto-approve approvals below the --confirm threshold tier. Defaults to --confirm=human (truly autonomous). Env: "+approval.EnvAuto+".")
	cmd.PersistentFlags().StringVar(&confirmFlag, "confirm", "",
		"Threshold tier under --auto: this tier and above still prompt; lower tiers auto-yes. Valid: command, prod-agent, test-agent, prod-commit, test-commit, human. Default when --auto is set: human. Env: "+approval.EnvConfirm+".")

	// Help-text grouping. Without these, the new root verbs would
	// interleave alphabetically with the project verbs and the visual
	// distinction the rename creates would be lost. See plan decisions
	// log (2026-05-15) for the group choices.
	const (
		groupProjectOps   = "project"
		groupCrossRepoOps = "cross-repo"
		groupOther        = "other"
	)
	cmd.AddGroup(
		&cobra.Group{ID: groupProjectOps, Title: "Project ops (operate on the scaffolded project rooted at gh-optivem.yaml):"},
		&cobra.Group{ID: groupCrossRepoOps, Title: "Cross-repo ops (operate on every repo in the resolved scope — see --workspace):"},
		&cobra.Group{ID: groupOther, Title: "Other:"},
	)

	// withGroup attaches a GroupID to a command before AddCommand sees it.
	// Tiny helper to keep the AddCommand block readable.
	withGroup := func(c *cobra.Command, group string) *cobra.Command {
		c.GroupID = group
		return c
	}

	cmd.AddCommand(
		// Project ops — operate on the scaffolded project (one
		// gh-optivem.yaml, one repo or multitier-bundle).
		withGroup(newInitCmd(), groupProjectOps),
		withGroup(newConfigCmd(), groupProjectOps),
		withGroup(newSystemCmd(), groupProjectOps),
		withGroup(newTestCmd(), groupProjectOps),
		withGroup(newCompileCmd(), groupProjectOps),
		withGroup(newDoctorCmd(), groupProjectOps),

		// Cross-repo ops — env-derived scope (workspace folders or the
		// cwd repo). See cross_repo_commands.go for the cascade.
		withGroup(newCommitCmd(), groupCrossRepoOps),
		withGroup(newSyncCmd(), groupCrossRepoOps),
		withGroup(newActionsCmd(), groupCrossRepoOps),
		withGroup(newRateLimitCmd(), groupCrossRepoOps),
		// Hidden TBD-discipline reports — same scope cascade, but Hidden
		// at the root surface until their placement is decided. See
		// plan "Deferred follow-ups".
		withGroup(newLintHistoryCmd(), groupCrossRepoOps),
		withGroup(newStaleBranchesCmd(), groupCrossRepoOps),

		// Other — verbs that don't fit the above two buckets cleanly.
		withGroup(newImplementCmd(), groupOther),
		withGroup(newRunCmd(), groupOther),
		withGroup(newProcessCmd(), groupOther),
		withGroup(newArchitectureCmd(), groupOther),
		withGroup(newEnvironmentCmd(), groupOther),
		withGroup(newBranchCmd(), groupOther),
		withGroup(newPrCmd(), groupOther),
		withGroup(newHooksCmd(), groupOther),
		withGroup(newCleanupCmd(), groupOther),
		// `output` is the machine-facing ATDD agent output channel.
		// Called by an agent from inside its dispatched subprocess; not
		// useful to operators outside that context. Kept in groupOther
		// for visibility (the `gh optivem output --help` text spells out
		// the env-var dispatch contract for anyone who finds it).
		withGroup(newOutputCmd(), groupOther),
	)
	return cmd
}

// newInitCmd builds the `init` subcommand. Project-stable values
// (owner, repo, system-name, arch, repo-strategy, langs, paths,
// project-url, license, deploy) come from gh-optivem.yaml. When the file
// is missing on a TTY, init drops into the same interactive prompt as
// `gh optivem config init` and writes the file in place. The init
// command surface itself is per-invocation flags only.
func newInitCmd() *cobra.Command {
	f := &config.RawFlags{}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new pipeline project",
		Long: `Scaffold a new pipeline project: create the GitHub repo(s), apply the shop
template with naming substitutions, wire up the SonarCloud project(s), and
verify the pipeline up to the requested --verify-level.

Project-stable values are read from gh-optivem.yaml. On a TTY, a missing
file drops into the same interactive prompt as ` + "`gh optivem config init`" + `
(owner/repo, system-name, arch, repo-strategy, lang, project-url) and the
file is written in place before scaffolding continues. The init command
itself only takes per-invocation flags — verify-level, workdir, etc.

If project.url is empty, init will auto-create the project board and write
the URL back into gh-optivem.yaml.`,
		Example: `  gh optivem init`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runInit(cmd, f)
		},
	}
	config.BindInitFlags(cmd, f)
	return cmd
}

// loadProjectConfigForInit resolves the gh-optivem.yaml path using the
// shared flag > env > default cascade and produces a *projectconfig.Config
// for init to consume. The cascade has two halves keyed off
// projectconfig.ResolvePath's explicit return:
//
// Explicit path (operator passed --config / -c, or set GH_OPTIVEM_CONFIG):
// honour today's behaviour — write the YAML at that path if missing +
// YAML-affecting flags supplied; prompt-and-write at that path if missing
// + TTY; terse MissingFileError otherwise. The operator chose the on-disk
// location, so we always materialize there. Returns sourcePath = the
// absolute path so the project-board step can write the auto-created URL
// back into the same file.
//
// Default path (CWD/gh-optivem.yaml, no flag, no env var): keep the
// config in memory and let steps.WriteOptivemYAML be the sole on-disk
// writer (inside the scaffold dir). Pre-existing CWD files are still
// loaded and respected — operator-authored input is never silently
// relocated. Returns sourcePath = "" for the in-memory cases so
// downstream guards on SourceConfigPath == "" short-circuit correctly.
func loadProjectConfigForInit(flagPath string, f *config.RawFlags) (*projectconfig.Config, string, error) {
	path, explicit := projectconfig.ResolvePath(flagPath)
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", fmt.Errorf("resolve absolute path for %s: %w", path, err)
	}

	if explicit {
		if _, statErr := os.Stat(abs); errors.Is(statErr, fs.ErrNotExist) && hasYAMLAffectingFlags(f) {
			if _, err := configinit.Run(f, abs, false); err != nil {
				return nil, "", err
			}
		} else if err := configinit.EnsureExists(abs); err != nil {
			return nil, "", err
		}
		pc, err := configcheck.LoadFromPath(abs)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, "", projectconfig.MissingFileError(abs)
			}
			return nil, "", err
		}
		return pc, abs, nil
	}

	// Default path: respect a pre-existing CWD file (operator-authored input).
	if _, statErr := os.Stat(abs); statErr == nil {
		pc, err := configcheck.LoadFromPath(abs)
		if err != nil {
			return nil, "", err
		}
		return pc, abs, nil
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return nil, "", statErr
	}
	if hasYAMLAffectingFlags(f) {
		pc, err := configinit.BuildConfig(f)
		return pc, "", err
	}
	pc, _, err := configinit.EnsureExistsOrBuild(abs)
	return pc, "", err
}

// hasYAMLAffectingFlags reports whether the operator supplied the minimum
// set of YAML-affecting flags `configinit.Run` needs to write a fresh
// gh-optivem.yaml. ValidateAndDeriveForYAML rejects anything less; checking
// here lets the CI path skip the interactive prompt without false-positiving
// when only a stray --license was passed.
func hasYAMLAffectingFlags(f *config.RawFlags) bool {
	return f != nil &&
		f.Owner != "" &&
		f.Repo != "" &&
		f.SystemName != "" &&
		f.Arch != "" &&
		f.RepoStrategy != ""
}

func runInit(cmd *cobra.Command, f *config.RawFlags) {
	pc, sourcePath, err := loadProjectConfigForInit(projectConfigPath, f)
	if err != nil {
		log.FatalExit(err.Error())
	}
	sourceProjectURLWasEmpty := pc.Project.URL == ""
	if err := config.FillRawFlagsFromYAML(f, pc); err != nil {
		log.FatalExit(err.Error())
	}
	cfg := config.ParseAndValidate(cmd, f)
	cfg.SourceConfigPath = sourcePath
	cfg.SourceProjectURLWasEmpty = sourceProjectURLWasEmpty
	if err := log.Init(cfg.Verbose, cfg.Quiet, cfg.LogFile); err != nil {
		log.FatalExit(err.Error())
	}
	defer log.Close()

	// Print update notice if a newer release is available. Notify-only — the
	// user upgrades explicitly via `gh extension upgrade optivem`.
	checkForUpdate(cfg)

	gh := shell.NewGitHub(cfg.FullRepo)
	sc := shell.NewSonarCloud(cfg.SonarToken, pc.Sonar.Organization)

	printBanner(cfg, pc)

	// failureNote is captured by the Commit-and-push closure so, if an earlier
	// step failed, the commit message can flag the push as a partial scaffold.
	var failureNote string
	allSteps := buildSteps(cfg, pc, gh, sc, &failureNote)
	errors, totalDuration := executeSteps(allSteps, &failureNote)

	printSummary(cfg, errors, totalDuration)

	// Keep the local scaffold dir on failure so it can be inspected.
	if errors > 0 {
		cfg.KeepLocal = true
	}
	steps.Cleanup(cfg)

	if errors > 0 {
		os.Exit(1)
	}
}

// Phase labels — printed as section headers by executeSteps when the phase changes.
const (
	phasePrepare          = "Prepare"
	phaseSetupRepo        = "Setup repository"
	phaseApplyTemplate    = "Apply template"
	phasePushScaffold     = "Push scaffold"
	phaseLintScaffold     = "Lint scaffold"
	phaseVerifyLocal      = "Verify local"
	phaseVerifyCommit     = "Verify commit stage"
	phaseVerifyAcceptance = "Verify acceptance stage"
	phaseVerifyQA         = "Verify QA stage"
	phaseVerifyProduction = "Verify production stage"
	phaseFinalize         = "Finalize"
)

func buildSteps(cfg *config.Config, pc *projectconfig.Config, gh *shell.GitHub, sc *shell.SonarCloud, failureNote *string) []stepDef {
	allSteps := []stepDef{
		// PHASE: PREPARE — pull prerequisites (shop template) needed by all later phases
		{name: "Clone shop template", phase: phasePrepare, fn: func() { steps.CloneShopTemplate(cfg) }},

		// PHASE: SETUP REPOSITORY — remote infra for this repo
		{name: "Create repositories", phase: phaseSetupRepo, fn: func() { steps.CreateRepos(cfg, gh) }},
		{name: "Ensure project board", phase: phaseSetupRepo, fn: func() { steps.EnsureProjectBoard(cfg, gh) }},
		{name: "Setup environments", phase: phaseSetupRepo, fn: func() { steps.SetupEnvironments(cfg, gh) }},
		{name: "Seed subtype labels", phase: phaseSetupRepo, fn: func() { steps.SeedSubtypeLabels(cfg, gh) }},
		{name: "Setup variables and secrets", phase: phaseSetupRepo, fn: func() { steps.SetupVariablesAndSecrets(cfg, gh) }},

		// PHASE: APPLY TEMPLATE — clone, templatize locally
		// (Lint and push are split into their own phases below.)
		{name: "Clone repos", phase: phaseApplyTemplate, fn: func() { steps.CloneRepos(cfg, gh) }},
		{name: "Apply template", phase: phaseApplyTemplate, fn: func() { steps.ApplyTemplate(cfg) }},
		{name: "Replace repository references", phase: phaseApplyTemplate, fn: func() { steps.ReplaceRepoReferences(cfg) }},
		{name: "Replace namespaces", phase: phaseApplyTemplate, fn: func() { steps.ReplaceNamespaces(cfg) }},
		{name: "Replace system name", phase: phaseApplyTemplate, fn: func() { steps.ReplaceSystemName(cfg) }},
		{name: "Update README", phase: phaseApplyTemplate, fn: func() { steps.UpdateReadme(cfg) }},
		{name: "Write gh-optivem.yaml", phase: phaseApplyTemplate, fn: func() { steps.WriteOptivemYAML(cfg) }},
		{name: "Write LICENSE", phase: phaseApplyTemplate, fn: func() { steps.WriteLicense(cfg) }},
		{name: "Create SonarCloud projects", phase: phaseApplyTemplate, fn: func() { steps.CreateSonarCloudProjects(cfg, pc, sc) }},

		// Validation is grouped into the Apply template phase but runs last —
		// i.e. after Commit and push — so broken output is already visible in
		// the remote repo for troubleshooting.
		// TODO: re-enable after the template-name rewrite lands. The current rule sets flag
		// patterns (e.g. "system/monolith/" in docker-compose.local.monolith.*.yml) that the
		// upcoming rename will eliminate at the source, making these checks either trivially
		// satisfied or redundant. Rewriting the validators before the rename risks baking in
		// rules against names that are about to change.
		// {name: "Validate no leftover system names", phase: phaseApplyTemplate, fn: func() { steps.ValidateNoLeftoverSystemNames(cfg) }},
		// {name: "Validate no leftover shop template refs", phase: phaseApplyTemplate, fn: func() { steps.ValidateNoLeftoverShopRefs(cfg) }},
		// {name: "Validate no leftover template references", phase: phaseApplyTemplate, fn: func() { steps.ValidateNoLeftoverTemplateRefs(cfg) }},
	}

	// ATDD assets no longer install per-repo — they live in gh-optivem's
	// embedded asset tree (runtime/agents/ + runtime/shared/, fed to the
	// agent subprocess via argv). Consumer repos hold zero ATDD assets on
	// disk. The --no-atdd flag is retained for backward compatibility but
	// is now a no-op at init time.

	allSteps = append(allSteps, stepDef{
		name:      "Commit and push",
		phase:     phasePushScaffold,
		alwaysRun: true,
		fn:        func() { steps.CommitAndPush(cfg, *failureNote) },
	})

	// Lint scaffolded workflows AFTER push so a broken scaffold lands on the
	// remote where the author can inspect it — students hitting init issues need
	// the bad state visible for troubleshooting, not aborted locally. failHard
	// still skips the downstream verify phases when lint fails. Runs even at
	// --verify-level=none — output correctness is part of the scaffold, not a
	// verify-level concern.
	allSteps = append(allSteps, stepDef{
		name:     "Verify scaffolded workflows",
		phase:    phaseLintScaffold,
		failHard: true,
		fn:       func() { steps.VerifyScaffoldWorkflows(cfg) },
	})

	allSteps = append(allSteps, buildVerifySteps(cfg, gh)...)

	allSteps = append(allSteps,
		stepDef{name: "Print project registration", phase: phaseFinalize, fn: func() { steps.PrintRegistration(cfg) }},
	)

	return allSteps
}

// verifyLevelOrder maps --verify-level values to a rank. A step runs when the
// configured level's rank is >= the step's minimum rank.
var verifyLevelOrder = map[string]int{
	"none":       0,
	"local":      1,
	"commit":     2,
	"acceptance": 3,
	"qa":         4,
	"release":    5,
}

// buildVerifySteps assembles the verify pipeline in a fixed order that mirrors
// the CI pipeline stages: local compile → local tests → local sonar → commit →
// acceptance (latest + legacy in parallel) → QA (stage + signoff) → production
// → bump-patch-version (called downstream of prod-stage, verified explicitly)
// → cleanup (dry-run, schedule-only workflows only fire when init runs).
// Each step is gated by --verify-level; local tests and local sonar can
// additionally be skipped with --no-local-tests / --no-local-sonar, and legacy
// is dropped everywhere via --no-legacy.
func buildVerifySteps(cfg *config.Config, gh *shell.GitHub) []stepDef {
	level := verifyLevelOrder[cfg.VerifyLevel]
	if level == 0 {
		log.Infof("Skipping workflow verification (--verify-level none)")
		return nil
	}

	var s []stepDef

	// Step 1: Compile system + tests. Runs at every level above none; the
	// workflow lint runs in phaseApplyTemplate before push, not here.
	s = append(s,
		stepDef{name: "Compile system", phase: phaseVerifyLocal, fn: func() { steps.VerifyCompileSystem(cfg) }},
		stepDef{name: "Compile tests", phase: phaseVerifyLocal, fn: func() { steps.VerifyCompileTests(cfg) }},
	)

	// Step 2: Local system lifecycle — build → setup tests → start → run
	// tests (latest, then legacy) → stop → clean. Mirrors the CLI verb order
	// (gh optivem system build / test setup / system start / test run /
	// system stop / system clean) so each phase line maps to one verb.
	if !cfg.NoLocalTests {
		s = append(s,
			stepDef{name: "Build system", phase: phaseVerifyLocal, fn: func() { steps.VerifyBuildSystem(cfg) }},
			stepDef{name: "Setup tests", phase: phaseVerifyLocal, fn: func() { steps.VerifySetupTests(cfg) }},
			stepDef{name: "Start system", phase: phaseVerifyLocal, fn: func() { steps.VerifyStartSystem(cfg) }},
			stepDef{name: "Run tests (latest)", phase: phaseVerifyLocal, fn: func() { steps.VerifyRunTests(cfg, "tests.yaml") }},
		)
		if !cfg.NoLegacy {
			s = append(s,
				stepDef{name: "Run tests (legacy)", phase: phaseVerifyLocal, fn: func() { steps.VerifyRunTests(cfg, "tests.legacy.yaml") }},
			)
		}
		s = append(s,
			stepDef{name: "Stop system", phase: phaseVerifyLocal, fn: func() { steps.VerifyStopSystem(cfg) }},
			stepDef{name: "Clean system", phase: phaseVerifyLocal, fn: func() { steps.VerifyCleanSystem(cfg) }},
		)
	}

	// Step 3: Local SonarCloud scan — per-component run-sonar.sh against the
	// SonarCloud project created in the apply-template phase.
	if !cfg.NoLocalSonar {
		s = append(s,
			stepDef{name: "Verify local SonarCloud scan", phase: phaseVerifyLocal, fn: func() { steps.VerifyLocalSonar(cfg) }},
		)
	}

	// Step 4: Commit stage CI.
	if level >= verifyLevelOrder["commit"] {
		s = append(s,
			stepDef{name: "Verify commit stage", phase: phaseVerifyCommit, fn: func() { steps.VerifyCommitStage(cfg, gh) }},
		)
	}

	// Step 5: Acceptance stage CI (latest + legacy parallel, legacy optional).
	if level >= verifyLevelOrder["acceptance"] {
		s = append(s,
			stepDef{name: "Verify acceptance stage", phase: phaseVerifyAcceptance, fn: func() { steps.VerifyAcceptanceStages(cfg, gh) }},
		)
	}

	// Step 6: QA stage CI (stage + signoff).
	if level >= verifyLevelOrder["qa"] {
		s = append(s,
			stepDef{name: "Verify QA stage", phase: phaseVerifyQA, fn: func() { steps.VerifyQA(cfg, gh) }},
		)
	}

	// Step 7: Production stage CI.
	if level >= verifyLevelOrder["release"] {
		s = append(s,
			stepDef{name: "Verify production stage", phase: phaseVerifyProduction, fn: func() { steps.VerifyProdStage(cfg, gh) }},
		)
	}

	// Step 8: Cleanup workflow (dry-run). cleanup.yml otherwise only fires on
	// schedule, so a syntax error or stale action reference would silently slip
	// past init and surface the next night.
	if level >= verifyLevelOrder["release"] {
		s = append(s,
			stepDef{name: "Verify cleanup workflow", phase: phaseVerifyProduction, fn: func() { steps.VerifyCleanup(cfg, gh) }},
		)
	}

	return s
}

func executeSteps(allSteps []stepDef, failureNote *string) (int, time.Duration) {
	errors := 0
	hardFailed := false
	totalStart := time.Now()
	totalByPhase := countStepsByPhase(allSteps)
	order := phaseOrder(allSteps)
	phaseIdx := make(map[string]int, len(order))
	for i, p := range order {
		phaseIdx[p] = i + 1
	}
	totalPhases := len(order)

	posInPhase := 0
	prevPhase := ""
	for _, s := range allSteps {
		// Skip non-alwaysRun steps once something has failed, so a broken
		// scaffold still pushes the partial output (for troubleshooting) but
		// stops running the downstream validate/verify steps. A failHard step
		// (e.g. workflow lint) escalates the skip to alwaysRun steps too — at
		// that point pushing the broken scaffold has no troubleshooting value,
		// only red runs on the new repo.
		if hardFailed {
			continue
		}
		if errors > 0 && !s.alwaysRun {
			continue
		}

		if s.phase != prevPhase {
			if s.phase != "" {
				log.PhaseHeader(phaseIdx[s.phase], totalPhases, s.phase)
			}
			prevPhase = s.phase
			posInPhase = 0
		}
		posInPhase++

		if !runStep(s, posInPhase, totalByPhase[s.phase], failureNote) {
			errors++
			if s.failHard {
				hardFailed = true
			}
		}
	}
	return errors, time.Since(totalStart)
}

func countStepsByPhase(allSteps []stepDef) map[string]int {
	totals := make(map[string]int)
	for _, s := range allSteps {
		totals[s.phase]++
	}
	return totals
}

// phaseOrder returns the distinct phase labels in order of first appearance.
func phaseOrder(allSteps []stepDef) []string {
	var order []string
	seen := make(map[string]bool)
	for _, s := range allSteps {
		if s.phase == "" || seen[s.phase] {
			continue
		}
		seen[s.phase] = true
		order = append(order, s.phase)
	}
	return order
}

// runStep runs a single step, recovering from panics. Returns true on success.
// On failure it sets *failureNote (unless already set by an earlier failure) so
// the later always-run commit step can flag the push as partial.
func runStep(s stepDef, pos, total int, failureNote *string) (ok bool) {
	ok = true
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Step failed: %s -- %v", s.name, r)
			if *failureNote == "" {
				*failureNote = fmt.Sprintf("%s step %d/%d: %s", s.phase, pos, total, s.name)
			}
			ok = false
		}
	}()
	stepStart := time.Now()
	s.fn()
	log.StepDone(pos, total, formatDuration(time.Since(stepStart)))
	return
}

func printSummary(cfg *config.Config, errors int, totalDuration time.Duration) {
	fmt.Println()
	fmt.Println(separator)
	if errors > 0 {
		log.Errorf("Setup completed with %d error(s) in %s", errors, formatDuration(totalDuration))
		fileIt := false
		if cfg.BugReport {
			fileIt = confirmBugReport(cfg)
		} else {
			fileIt = offerBugReport(cfg)
		}
		if fileIt {
			createBugReport(cfg, errors)
		}
	} else {
		log.Successf("All steps passed! Completed in %s", formatDuration(totalDuration))
	}
	fmt.Println()
	fmt.Printf("  System:     %s\n", cfg.SystemName)
	fmt.Printf("  Repository: https://github.com/%s\n", cfg.FullRepo)
	fmt.Printf("  Actions:    https://github.com/%s/actions\n", cfg.FullRepo)
	if cfg.ProjectURL != "" {
		fmt.Printf("  Project:    %s\n", cfg.ProjectURL)
	}
	fmt.Printf("  Log file:   %s\n", cfg.LogFile)
	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			fmt.Printf("  Backend:    https://github.com/%s\n", cfg.BackendFullRepo)
			fmt.Printf("  Frontend:   https://github.com/%s\n", cfg.FrontendFullRepo)
		} else {
			fmt.Printf("  System:     https://github.com/%s\n", cfg.SystemFullRepo)
		}
	}
	fmt.Println()
}

// resolveApprovalForCmd runs in PersistentPreRunE: build a Resolved policy
// from the --auto / --confirm flags and env vars, stash it on cmd.Context
// for every confirmation call site that reads via cmdctx.Approval, and
// publish the same policy into the configinit package-level slot (the
// configinit prompt path doesn't get a *cobra.Command and reads the
// approval back as a package global instead). Emits a one-line stderr
// banner when Auto is on so the operator can see which source the policy
// came from.
func resolveApprovalForCmd(cmd *cobra.Command) error {
	r, err := approval.Resolve(
		autoFlag,
		flagChanged(cmd, "auto"),
		confirmFlag,
		flagChanged(cmd, "confirm"),
		os.Getenv,
	)
	if err != nil {
		return err
	}
	cmd.SetContext(cmdctx.WithApproval(cmd.Context(), r))
	configinit.SetApproval(r)
	if r.Auto {
		fmt.Fprintf(cmd.ErrOrStderr(), "Auto: true (auto-source: %s, confirm-source: %s → floor=%s)\n",
			r.AutoSource, r.ConfirmSource, r.ConfirmFloorString())
	}
	return nil
}

// flagChanged reports whether the named flag was explicitly set on the
// executing command (or any ancestor's persistent flags). Returns false
// when the flag is undefined — neither Auto nor Confirm should be
// undefined, but the nil check keeps PersistentPreRunE robust against a
// caller (e.g. a future subcommand that overrides the root's flag set)
// who forgets to inherit.
func flagChanged(cmd *cobra.Command, name string) bool {
	if f := cmd.Flag(name); f != nil {
		return f.Changed
	}
	return false
}

// confirmBugReport shows what will be sent and asks the user to confirm.
// Returns true only on an explicit "y"/"yes". On a non-tty (CI, piped input)
// the opt-in flag alone is treated as consent and it returns true without
// prompting.
func confirmBugReport(cfg *config.Config) bool {
	fmt.Println()
	fmt.Println(separator)
	fmt.Println("  Bug report — review before filing")
	fmt.Println(separator)
	fmt.Println()
	fmt.Println("  On confirm, the following will be filed upstream:")
	fmt.Println()
	fmt.Printf("  - Issue created at: https://github.com/optivem/gh-optivem/issues\n")
	fmt.Printf("  - Linking to your repo: https://github.com/%s\n", cfg.FullRepo)
	fmt.Printf("  - Log file: %s\n", cfg.LogFile)
	fmt.Println()

	if cfg.AssumeYes {
		log.Infof("Proceeding without confirmation (--yes).")
		return true
	}

	if !isInteractive() {
		log.Infof("Non-interactive session detected; proceeding with --report-bug opt-in.")
		return true
	}

	// TODO(non-implement-tiering): placeholder; proper tier assignment
	// deferred to the follow-up plan. See plan
	// 20260528-0930-approval-tier-ladder.md §D5.
	ok, err := approval.Confirm(cfg.Approval, approval.CategoryHuman, os.Stdin, os.Stdout, "  Proceed?")
	if err != nil {
		log.Warnf("Could not read confirmation: %v. Skipping bug report.", err)
		return false
	}
	return ok
}

// offerBugReport prompts the user (after a failure) whether they want to file
// a bug report. Used when --report-bug was NOT set explicitly — opt-out by
// default. Skipped without prompting in unattended modes (--yes or
// non-interactive stdin), so failures in CI don't pester or auto-file silently.
// Shows the same "what will be sent" details as confirmBugReport so the user
// can decide informed before answering.
func offerBugReport(cfg *config.Config) bool {
	if cfg.AssumeYes {
		return false // unattended: don't pester; user must opt in via --report-bug
	}
	if !isInteractive() {
		return false // can't prompt
	}

	fmt.Println()
	fmt.Println(separator)
	fmt.Println("  Bug report — share this failure with the maintainers?")
	fmt.Println(separator)
	fmt.Println()
	fmt.Println("  On confirm, the following will be filed upstream:")
	fmt.Println()
	fmt.Printf("  - Issue created at: https://github.com/optivem/gh-optivem/issues\n")
	fmt.Printf("  - Linking to your repo: https://github.com/%s\n", cfg.FullRepo)
	fmt.Printf("  - Log file: %s\n", cfg.LogFile)
	fmt.Println()
	// TODO(non-implement-tiering): placeholder; proper tier assignment
	// deferred to the follow-up plan. See plan
	// 20260528-0930-approval-tier-ladder.md §D5.
	ok, err := approval.Confirm(cfg.Approval, approval.CategoryHuman, os.Stdin, os.Stdout, "  File a bug report?")
	if err != nil {
		log.Warnf("Could not read confirmation: %v. Skipping bug report.", err)
		return false
	}
	return ok
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// readLogForBugReport returns the log contents to embed in a bug-report body,
// capped at bugReportLogMaxBytes so the issue body stays under GitHub's
// 65 KB limit. When the log is small enough, returns the whole thing
// (truncated=false). Otherwise returns the trailing bugReportLogMaxBytes,
// trimmed forward to the next newline so embedding never starts mid-line
// (truncated=true). Returns ("", false) when the log is unreadable or empty.
func readLogForBugReport(path string) (content string, truncated bool) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return "", false
	}
	if len(data) <= bugReportLogMaxBytes {
		return strings.TrimRight(string(data), "\n"), false
	}
	tail := data[len(data)-bugReportLogMaxBytes:]
	if idx := bytes.IndexByte(tail, '\n'); idx != -1 && idx < len(tail)-1 {
		tail = tail[idx+1:]
	}
	return strings.TrimRight(string(tail), "\n"), true
}

func createBugReport(cfg *config.Config, errorCount int) {
	lang := cfg.Lang
	if cfg.Arch == "multitier" {
		lang = fmt.Sprintf("backend=%s, frontend=%s", cfg.BackendLang, cfg.FrontendLang)
	}

	title := fmt.Sprintf("Scaffolding failure: %s (%s, %s)", cfg.SystemName, cfg.Arch, cfg.RepoStrategy)
	body := fmt.Sprintf(
		"Scaffolding failed with %d error(s).\n\n"+
			"- **System:** %s\n"+
			"- **Architecture:** %s\n"+
			"- **Repo strategy:** %s\n"+
			"- **Language:** %s\n"+
			"- **Test language:** %s\n"+
			"- **Repository:** https://github.com/%s\n",
		errorCount, cfg.SystemName, cfg.Arch, cfg.RepoStrategy,
		lang, cfg.TestLang, cfg.FullRepo)
	if content, truncated := readLogForBugReport(cfg.LogFile); content != "" {
		summary := fmt.Sprintf("Log (%s)", cfg.LogFile)
		if truncated {
			summary = fmt.Sprintf("Last %d KB of log — truncated; full log at %s",
				bugReportLogMaxBytes/1024, cfg.LogFile)
		}
		body += fmt.Sprintf("\n<details>\n<summary>%s</summary>\n\n```\n%s\n```\n</details>\n",
			summary, content)
	}

	bodyFile, err := os.CreateTemp("", "gh-issue-body-*.md")
	if err != nil {
		log.Infof("WARN: Failed to create temp file for bug report: %v", err)
		return
	}
	defer os.Remove(bodyFile.Name())
	if _, err := bodyFile.WriteString(body); err != nil {
		log.Warnf("Failed to write bug-report body to %s: %v — skipping issue creation.", bodyFile.Name(), err)
		return
	}
	if err := bodyFile.Close(); err != nil {
		log.Warnf("Failed to close bug-report body file %s: %v — skipping issue creation.", bodyFile.Name(), err)
		return
	}

	out, err := shell.RunWithRetry(
		fmt.Sprintf(`gh issue create --repo optivem/gh-optivem --title %q --body-file %s`, title, bodyFile.Name()),
		false, "")
	if err != nil {
		log.Infof("WARN: Failed to create bug report: %v\n%s", err, out)
	} else {
		log.Successf("Bug report created: %s", strings.TrimSpace(out))
	}
}

// checkForUpdate queries the latest release and, when the running binary is
// outdated, prints a notice telling the user to run `gh extension upgrade optivem`.
// Notify-only — never auto-upgrades; users decide when to upgrade.
func checkForUpdate(cfg *config.Config) {
	if strings.HasPrefix(version.Version, "dev") {
		return // skip check for development builds (e.g. "dev", "dev-44d9d27", "dev-44d9d27-dirty")
	}

	cmd := exec.Command("gh", "api", "repos/optivem/gh-optivem/releases/latest", "--jq", ".tag_name")
	out, err := cmd.Output()
	if err != nil {
		return // fail silently — don't block usage if offline or rate-limited
	}
	latest := strings.TrimSpace(string(out))
	if latest == "" || latest == version.Version {
		return
	}

	log.Warnf("Update available: %s → %s. Run `gh extension upgrade optivem` to upgrade.", version.Version, latest)
}

func printBanner(cfg *config.Config, pc *projectconfig.Config) {
	fmt.Println()
	fmt.Println(separator)
	fmt.Println("  Pipeline Project Setup")
	fmt.Println(separator)

	fmt.Println()
	fmt.Println("  From gh-optivem.yaml")
	fmt.Println()
	log.Infof("owner:             %s", cfg.Owner)
	log.Infof("repo:              %s", cfg.Repo)
	log.Infof("system-name:       %s", cfg.SystemName)
	log.Infof("system.architecture: %s", cfg.Arch)
	log.Infof("repo-strategy:     %s", cfg.RepoStrategy)
	log.Infof("system.lang:       %s", cfg.Lang)
	log.Infof("system.backend.lang: %s", cfg.BackendLang)
	log.Infof("system.frontend.lang: %s", cfg.FrontendLang)
	log.Infof("system-test.lang:  %s", cfg.TestLang)
	log.Infof("license:           %s", cfg.License)
	log.Infof("deploy:            %s", cfg.Deploy)

	fmt.Println()
	fmt.Println("  Per-invocation flags")
	fmt.Println()
	tag := func(name string) string {
		if cfg.UserSetFlags[name] {
			return ""
		}
		return " (default)"
	}
	log.Infof("--verify-level:      %s%s", cfg.Raw.VerifyLevel, tag("verify-level"))
	log.Infof("--no-legacy:         %v%s", cfg.NoLegacy, tag("no-legacy"))
	log.Infof("--no-local-tests:    %v%s", cfg.NoLocalTests, tag("no-local-tests"))
	log.Infof("--no-local-sonar:    %v%s", cfg.NoLocalSonar, tag("no-local-sonar"))
	log.Infof("--no-project:        %v%s", cfg.NoProject, tag("no-project"))
	log.Infof("--shop-ref:        %s%s", cfg.Raw.ShopRef, tag("shop-ref"))
	log.Infof("--keep-local:      %v%s", cfg.Raw.KeepLocal, tag("keep-local"))
	log.Infof("--report-bug:      %v%s", cfg.BugReport, tag("report-bug"))
	log.Infof("--yes:             %v%s", cfg.AssumeYes, tag("yes"))
	log.Infof("--workdir:         %s%s", cfg.Raw.WorkDir, tag("workdir"))
	log.Infof("--log-file:        %s%s", cfg.LogFile, tag("log-file"))
	log.Infof("--verbose:         %v%s", cfg.Verbose, tag("verbose"))
	log.Infof("--quiet:           %v%s", cfg.Quiet, tag("quiet"))

	fmt.Println()
	fmt.Println("  Derived values")
	fmt.Println()
	log.Infof("Full repo:       %s", cfg.FullRepo)
	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "monolith" {
			log.Infof("System repo:     %s", cfg.SystemFullRepo)
		} else {
			log.Infof("Backend repo:    %s", cfg.BackendFullRepo)
			log.Infof("Frontend repo:   %s", cfg.FrontendFullRepo)
		}
	}
	log.Infof("Test lang:       %s", cfg.TestLang)
	log.Infof("Verify level:    %s", cfg.VerifyLevel)
	log.Infof("Cleanup local:   %v  (--keep-local to keep)", !cfg.KeepLocal)
	log.Infof("System pascal:   %s", cfg.SysNamePascalNew)
	log.Infof("System camel:    %s", cfg.SysNameCamelNew)
	log.Infof("System kebab:    %s", cfg.SysNameKebabNew)
	log.Infof("System lower:    %s", cfg.SysNameLowerNew)
	log.Infof("Java ns:         %s", cfg.JavaNsNew)
	log.Infof(".NET ns:         %s", cfg.DotnetNsNew)
	log.Infof("TS package:      %s", cfg.TsPkgNew)
	for _, key := range collectBannerSonarKeys(pc) {
		log.Infof("Sonar key:       %s", key)
	}

	fmt.Println()
	fmt.Println("  Environment")
	fmt.Println()
	log.Infof("gh-optivem:      %s", version.Version)
	log.Infof("gh CLI:          %s", ghCLIVersion())
	log.Infof("Shop ref:        %s", cfg.ShopRef)

	fmt.Println()
	fmt.Println("  Will create")
	fmt.Println()
	log.Infof("Environments:    acceptance, qa, production")
	log.Infof("Variables:       DOCKERHUB_USERNAME")
	log.Infof("Secrets:         %s", strings.Join(willCreateSecrets(cfg), ", "))
	if !cfg.NoProject {
		switch {
		case cfg.ProjectURL == "":
			log.Infof("Project board:   auto-create with canonical status set")
		default:
			log.Infof("Project board:   verify required statuses on supplied URL — may add missing options")
		}
	}

	fmt.Println()
	fmt.Println("  Local clones")
	fmt.Println()
	log.Infof("Workdir:         %s", cfg.WorkDir)
	log.Infof("Shop:            %s", cfg.ShopPath)
	log.Infof("Repo:            %s", cfg.RepoDir)
	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			log.Infof("Frontend repo:   %s", cfg.FrontendRepoDir)
			log.Infof("Backend repo:    %s", cfg.BackendRepoDir)
		} else {
			log.Infof("System repo:     %s", cfg.SystemRepoDir)
		}
	}
	fmt.Println()
}

// willCreateSecrets lists the GitHub Actions secrets this scaffold will set,
// in the order SetupVariablesAndSecrets writes them.
func willCreateSecrets(cfg *config.Config) []string {
	return []string{"DOCKERHUB_TOKEN", "SONAR_TOKEN", "WORKFLOW_TOKEN", "GHCR_TOKEN"}
}

// collectBannerSonarKeys returns the per-code-tier SonarCloud project keys
// in the order they appear in gh-optivem.yaml: system (monolith) or
// backend+frontend (multitier), followed by system-test. The banner prints
// each so the operator can see at a glance which SonarCloud projects the
// scaffold will create.
func collectBannerSonarKeys(pc *projectconfig.Config) []string {
	if pc == nil {
		return nil
	}
	var out []string
	if pc.System.SonarProject != "" {
		out = append(out, pc.System.SonarProject)
	}
	if pc.System.Backend.SonarProject != "" {
		out = append(out, pc.System.Backend.SonarProject)
	}
	if pc.System.Frontend.SonarProject != "" {
		out = append(out, pc.System.Frontend.SonarProject)
	}
	if pc.SystemTest.SonarProject != "" {
		out = append(out, pc.SystemTest.SonarProject)
	}
	return out
}

// ghCLIVersion returns the first line of `gh --version` (e.g. "gh version 2.50.0 ...").
// Returns "unknown" if gh is missing or the call fails — the scaffold still
// proceeds, and later steps will surface a clearer error if gh is actually
// unavailable.
func ghCLIVersion() string {
	out, err := exec.Command("gh", "--version").Output()
	if err != nil {
		return "unknown"
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(line)
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
