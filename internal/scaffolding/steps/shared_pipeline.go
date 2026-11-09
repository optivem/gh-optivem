package steps

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/scaffolding/files"
	"github.com/optivem/gh-optivem/internal/scaffolding/templates"
)

// Shop's single-pipeline layout (shop plan 20261102-1855): every per-flavor
// stage workflow is a thin caller of a shared reusable workflow
// (.github/workflows/_*.yml), and the commit and cloud stages use composite
// actions under .github/actions/. The shared files derive every per-flavor
// path from the callers' architecture / language / test-language inputs
// (system/<arch>/<lang>/VERSION, docker/<testLang>/<arch>, system-test/<testLang>,
// gh-optivem-<arch>-<testLang>.yaml, <arch>-<lang>-<stage> environments,
// <arch>-<lang>-v tags). A scaffold is flattened to one flavor, so the scaffold
// keeps the callers' inputs (they still drive per-language harness setup, port
// digits and monolith/multitier branches) and rewrites those derived paths in
// the shared files to the flattened literals (sharedWorkflowRewrites).

const (
	sharedWorkflowPrefix = "_"
	actionsDirName       = "actions"
	dirGitHub            = ".github"
)

var (
	// sharedWorkflowUseRe matches a job-level call to a shared reusable workflow.
	sharedWorkflowUseRe = regexp.MustCompile(`(?m)^\s*uses:\s*['"]?\./\.github/workflows/(_[^\s'"/]+\.yml)`)
	// localActionUseRe matches a step-level use of a repo-local composite action.
	localActionUseRe = regexp.MustCompile(`(?m)^\s*(?:-\s*)?uses:\s*['"]?\./\.github/actions/([^\s'"/]+)`)
)

// copySharedPipeline copies into repoDir every shop shared reusable workflow
// (_*.yml) and every local composite action (.github/actions/<name>/) that the
// workflows already in repoDir reference, transitively (_qa-stage.yml calls
// _deploy-stage.yml; workflows and actions use local actions). Deriving the
// set from the copied callers keeps it exact per repo: a scaffold never ships
// shop's meta-release machinery (_prerelease-pipeline.yml,
// _meta-prerelease-pipeline.yml, create-meta-release-tag, ...), and a
// multirepo component repo gets only the commit-stage actions. A referenced
// file missing from shop is an error: the scaffold would not run.
func copySharedPipeline(shop, repoDir string) error {
	wfSrc := filepath.Join(shop, dirGitHub, "workflows")
	wfDst := filepath.Join(repoDir, dirGitHub, "workflows")
	actSrc := filepath.Join(shop, dirGitHub, actionsDirName)
	actDst := filepath.Join(repoDir, dirGitHub, actionsDirName)

	seenWf := map[string]bool{}
	seenAct := map[string]bool{}
	queue, err := listFilesWithExt(wfDst, ".yml")
	if err != nil {
		return err
	}
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		caller := filepath.Base(path)
		added, err := copyReferencedWorkflows(string(data), caller, wfSrc, wfDst, seenWf)
		if err != nil {
			return err
		}
		queue = append(queue, added...)
		added, err = copyReferencedActions(string(data), caller, actSrc, actDst, seenAct)
		if err != nil {
			return err
		}
		queue = append(queue, added...)
	}
	return nil
}

// copyReferencedWorkflows copies every not-yet-seen shared workflow that data
// calls from wfSrc to wfDst and returns the copied paths for scanning.
func copyReferencedWorkflows(data, caller, wfSrc, wfDst string, seen map[string]bool) ([]string, error) {
	var added []string
	for _, m := range sharedWorkflowUseRe.FindAllStringSubmatch(data, -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		src := filepath.Join(wfSrc, name)
		if _, err := os.Stat(src); err != nil {
			return nil, fmt.Errorf("%s calls shared workflow %s, which shop does not have", caller, name)
		}
		dst := filepath.Join(wfDst, name)
		if err := files.CopyFile(src, dst); err != nil {
			return nil, err
		}
		added = append(added, dst)
	}
	return added, nil
}

// copyReferencedActions copies every not-yet-seen local composite action that
// data uses from actSrc to actDst and returns the copied action metadata files
// for scanning.
func copyReferencedActions(data, caller, actSrc, actDst string, seen map[string]bool) ([]string, error) {
	var added []string
	for _, m := range localActionUseRe.FindAllStringSubmatch(data, -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		src := filepath.Join(actSrc, name)
		if !dirExists(src) {
			return nil, fmt.Errorf("%s uses local action ./.github/actions/%s, which shop does not have", caller, name)
		}
		dst := filepath.Join(actDst, name)
		if err := files.CopyDir(src, dst); err != nil {
			return nil, err
		}
		for _, meta := range []string{"action.yml", "action.yaml"} {
			if p := filepath.Join(dst, meta); fileExists(p) {
				added = append(added, p)
			}
		}
	}
	return added, nil
}

func listFilesWithExt(dir, ext string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ext) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// sharedWorkflowRewrites rewrites the per-flavor paths, names and tag prefixes
// the shared _*.yml files derive from their inputs into the flattened scaffold
// layout. Applied to every workflow before the per-flavor content replacements
// (the patterns are expression text that only the shared files carry). Longer
// patterns come first where one is a prefix of another.
func sharedWorkflowRewrites() [][2]string {
	const (
		arch     = "${{ inputs.architecture }}"
		lang     = "${{ inputs.language }}"
		testLang = "${{ inputs.test-language || inputs.language }}"
		prefix   = "${{ env.PREFIX }}"
		legacy   = "${{ inputs.suite-set == 'legacy' && '-legacy' || '' }}"
	)
	return [][2]string{
		// One root VERSION, one docker/ dir, one system-test/ dir.
		{"VERSION_FILE: system/" + arch + "/" + lang + "/VERSION", "VERSION_FILE: VERSION"},
		{"COMPOSE_DIR: docker/" + testLang + "/" + arch, "COMPOSE_DIR: docker"},
		{`docker/${TEST_LANGUAGE}/${ARCHITECTURE}/systems.yaml`, "docker/systems.yaml"},
		{"system-test/${{ env.TEST_LANGUAGE }}", dirSystemTest},
		{"system-test/java", dirSystemTest},
		{"system-test/dotnet", dirSystemTest},
		{"system-test/typescript", dirSystemTest},
		// The canonical gh-optivem.yaml / gh-optivem.legacy.yaml pair.
		{"gh-optivem-" + arch + "-" + testLang + legacy + ".yaml",
			"gh-optivem${{ inputs.suite-set == 'legacy' && '.legacy' || '' }}.yaml"},
		{"gh-optivem-" + arch + "-" + testLang + ".yaml", "gh-optivem.yaml"},
		// Bare acceptance / qa / production environments (SetupEnvironments).
		{envPrefixYAML + arch + "-" + lang + "-${{ inputs.stage }}", "environment: ${{ inputs.stage }}"},
		{envPrefixYAML + arch + "-" + lang + "-", envPrefixYAML},
		{envPrefixYAML + prefix + "-", envPrefixYAML},
		// Unprefixed workflow file names and v<x.y.z> tags. PREFIX itself stays:
		// it still names the Cloud Run services and databases.
		{".github/workflows/" + prefix + "-", ".github/workflows/"},
		{prefix + "-v", "v"},
		{"prefix: " + prefix, "prefix: ''"},
		// Multitier component release tags were plain v<x.y.z> in the scaffold
		// before the restructure too (bump-patch-version watches "v").
		{"${{ fromJSON(inputs.images).backend }}-v", "v"},
		{"${{ fromJSON(inputs.images).frontend }}-v", "v"},
		// Component VERSION files: root VERSION (monolith), <component>/VERSION (multitier).
		{`file: (if $arch == "monolith" then "system/monolith/\($lang)/VERSION"
                       else "system/\($arch)/\(ltrimstr($arch + "-"))/VERSION" end)`,
			`file: (if $arch == "monolith" then "VERSION" else "\(split("/") | last)/VERSION" end)`},
		// Comments that would otherwise leave shop-only paths in the scaffold.
		{`(multitier: system/multitier/<image without "multitier-">/VERSION; monolith:
# system/monolith/<language>/VERSION)`, `(multitier: <component>/VERSION; monolith: VERSION)`},
	}
}

// multirepoSharedWorkflowRewrites points the shared files at images published
// by the component repos: BASE drops the repo segment and each caller's images
// map names <repo>/<image>, so "$BASE/<value>" is ghcr.io/<owner>/<repo>/<image>.
// The component VERSION entries gain the component repo, read cross-repo with
// WORKFLOW_TOKEN. images maps role ("system", "backend", "frontend") to repo.
func multirepoSharedWorkflowRewrites(owner string, images map[string]string) [][2]string {
	r := [][2]string{
		{"BASE: ghcr.io/${{ github.repository_owner }}/${{ github.event.repository.name }}", "BASE: ghcr.io/${{ github.repository_owner }}"},
		{`file: (if $arch == "monolith" then "VERSION" else "\(split("/") | last)/VERSION" end)`,
			`file: (if $arch == "monolith" then "VERSION" else "\(split("/") | last)/VERSION" end),
                repo: "` + owner + `/\(split("/") | first)"`},
		{"entries: ${{ steps.component-entries.outputs.entries }}",
			"entries: ${{ steps.component-entries.outputs.entries }}\n          token: ${{ secrets.WORKFLOW_TOKEN }}"},
	}
	roles := make([]string, 0, len(images))
	for role := range images {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	for _, role := range roles {
		r = append(r, [2]string{
			`"` + role + `":"` + role + `"`,
			`"` + role + `":"` + images[role] + "/" + role + `"`,
		})
	}
	return r
}

// leftoverSharedWorkflowRefs are expression fragments that must not survive
// sharedWorkflowRewrites: each would resolve to a shop-only path, environment
// or tag in the scaffold. Checked by forbiddenTemplateRefs.
var leftoverSharedWorkflowRefs = []string{
	"system/${{",
	"docker/${{",
	"gh-optivem-${{",
	"docker/${TEST_LANGUAGE}",
	"${{ env.PREFIX }}-v",
	"system-test/${{",
}

// projectPathRewrites maps shop project directories (the commit-stage
// actions' `project` values) to the scaffold's flattened ones.
func projectPathRewrites(arch, lang string) [][2]string {
	if arch == "monolith" {
		return [][2]string{{systemMonolithDir + lang, Names.TargetSystemDir}}
	}
	return [][2]string{
		{systemMultitierBackend + lang, Names.TargetBackendDir},
		{systemMultitierDir + "frontend-react", Names.TargetFrontendDir},
	}
}

var (
	// projectDescRe matches the shop-path examples in the commit-stage
	// actions' `project` input description.
	projectDescRe = regexp.MustCompile(`Project directory, e\.g\. [^']*'`)
	// caseArmRe matches a bash case-arm header: one or more |-separated
	// patterns, a closing paren, then the rest of the line.
	caseArmRe = regexp.MustCompile(`^(\s*)([A-Za-z0-9_./-]+(?:\|[A-Za-z0-9_./-]+)*)\)(.*)$`)
)

// fixupLocalActions rewrites the copied composite actions for the scaffold:
// the commit-stage actions (compile, unit, component, lint) switch on shop
// project paths, so this flavor's paths become the flattened ones and the
// other flavors' case arms are dropped; build-shared-images builds db-migrate
// from system/db, which the scaffold keeps at db/ (copyDbImage).
func fixupLocalActions(repoDir string, rewrites [][2]string) {
	root := filepath.Join(repoDir, dirGitHub, actionsDirName)
	if !dirExists(root) {
		return
	}
	all := append([][2]string{{`"$SOURCE" system/db`, `"$SOURCE" db`}}, rewrites...)
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || files.IsBinaryFile(info.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		s := projectDescRe.ReplaceAllString(string(data), "Project directory: system (monolith), backend or frontend (multitier).'")
		for _, r := range all {
			s = strings.ReplaceAll(s, r[0], r[1])
		}
		s = pruneForeignCaseArms(s)
		if s != string(data) {
			if err := os.WriteFile(path, []byte(s), info.Mode()); err != nil {
				log.Fatalf("Rewrite %s: %v", path, err)
			}
		}
		return nil
	})
}

// isForeignProject reports whether a case pattern is a shop project path the
// scaffold does not have (another flavor's system/<arch>/<lang>).
func isForeignProject(p string) bool {
	return strings.HasPrefix(p, systemMonolithDir) || strings.HasPrefix(p, systemMultitierDir)
}

// pruneForeignCaseArms drops shop-only project paths from bash case arms. An
// arm keeps its scaffold patterns; an arm left with none is removed up to and
// including its terminating ";;" line.
func pruneForeignCaseArms(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		m := caseArmRe.FindStringSubmatch(lines[i])
		if m == nil || !strings.Contains(m[2], "/") {
			out = append(out, lines[i])
			continue
		}
		if kept := scaffoldPatterns(m[2]); len(kept) > 0 {
			out = append(out, m[1]+strings.Join(kept, "|")+")"+m[3])
			continue
		}
		// Skip the whole arm.
		i = armEnd(lines, i)
	}
	return strings.Join(out, "\n")
}

// scaffoldPatterns returns the |-separated case patterns that are not
// shop-only project paths.
func scaffoldPatterns(patterns string) []string {
	var kept []string
	for _, p := range strings.Split(patterns, "|") {
		if !isForeignProject(p) {
			kept = append(kept, p)
		}
	}
	return kept
}

// armEnd returns the index of the ";;" line terminating the case arm that
// starts at i, or len(lines) when there is none.
func armEnd(lines []string, i int) int {
	for ; i < len(lines); i++ {
		if strings.Contains(lines[i], ";;") {
			break
		}
	}
	return i
}

// legacyCronAnchor is the trigger block of shop's legacy Docker Compose
// caller, which shop's prerelease pipeline dispatches. Scaffolds have no
// prerelease pipeline, so the scaffold gives the legacy gate the same hourly
// schedule as the latest gate (and the old acceptance-stage-legacy.yml).
const (
	legacyCronAnchor      = "on:\n  workflow_dispatch:\n"
	legacyCronReplacement = "on:\n  schedule:\n    - cron: '0 * * * *'  # Run every hour\n  workflow_dispatch:\n"
	legacyCommentShop     = "Dispatched by the prerelease pipeline (no cron)."
	legacyCommentScaffold = "Runs hourly, like the latest gate."
	legacyDropShop        = "deletes this file\n# and the acceptance-stage-legacy job from its prerelease pipeline."
	legacyDropScaffold    = "deletes this file\n# (and its legacy system tests)."
)

// scheduleLegacyGate adds the hourly schedule to the scaffolded legacy Docker
// Compose caller, if it was scaffolded.
func scheduleLegacyGate(repoDir string) {
	path := filepath.Join(repoDir, Names.TargetWorkflowsDir, Names.DestLegacyAcceptWf)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	s := string(data)
	if !strings.Contains(s, legacyCronAnchor) {
		log.Warnf("%s: trigger block not found; the legacy acceptance gate runs on manual dispatch only", Names.DestLegacyAcceptWf)
		return
	}
	s = strings.Replace(s, legacyCronAnchor, legacyCronReplacement, 1)
	s = strings.Replace(s, legacyCommentShop, legacyCommentScaffold, 1)
	s = strings.Replace(s, legacyDropShop, legacyDropScaffold, 1)
	if err := os.WriteFile(path, []byte(s), 0644); err != nil {
		log.Fatalf("Write %s: %v", path, err)
	}
}

// copyDbImage copies shop's db-migrate image Dockerfile (system/db/Dockerfile,
// which COPYs migrations/) to <dst>/db/, beside the migrations copyDbMigrations
// lands there, so build-shared-images can build it from db/.
func copyDbImage(shop, dst string) {
	src := filepath.Join(shop, "system", "db", "Dockerfile")
	if fileExists(src) {
		if err := os.MkdirAll(filepath.Join(dst, "db"), 0o755); err != nil {
			log.Fatalf("Create db dir: %v", err)
		}
		files.CopyFile(src, filepath.Join(dst, "db", "Dockerfile"))
	}
}

// scaffoldPipeline is the shared tail of every root/monorepo workflow copy:
// copy the shared workflows and local actions the callers reference, rewrite
// the shared files to the flattened layout, then fix up the local actions.
func scaffoldPipeline(shop, repoDir string, projectRewrites [][2]string) {
	if err := copySharedPipeline(shop, repoDir); err != nil {
		log.Fatalf("Copy shared workflows and actions: %v", err)
	}
	templates.FixupWorkflowContent(repoDir, sharedWorkflowRewrites())
	fixupLocalActions(repoDir, projectRewrites)
}
