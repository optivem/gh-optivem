package steps

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/projectconfig"
	"github.com/optivem/gh-optivem/internal/promptio"
	"github.com/optivem/gh-optivem/internal/shell"
)

// canonicalStatusOptions is the full ATDD-ready Status set written into
// auto-created project boards (Path A). Replaces GitHub's default
// Todo / In Progress / Done.
var canonicalStatusOptions = []string{
	"Backlog", "Ready", "In Progress", "In Acceptance", "In QA", "Done",
}

// atddRequiredStatusOptions is the minimum set the ATDD pipeline reads/writes.
// Enforced on operator-supplied boards (Path B) without touching unrelated
// columns the operator may have added (Blocked, On hold, etc.).
var atddRequiredStatusOptions = []string{
	"Ready", "In Progress", "In Acceptance", "In QA",
}

// statusOptionColors picks a sensible ProjectV2 color per canonical option.
// GitHub's GraphQL API requires a color when (re)creating SingleSelect options.
// Unknown names fall back to GRAY.
var statusOptionColors = map[string]string{
	"Backlog":       "GRAY",
	"Ready":         "BLUE",
	"In Progress":   "YELLOW",
	"In Acceptance": "ORANGE",
	"In QA":         "PURPLE",
	"Done":          "GREEN",
}

// Test seams — production wires these to real shell helpers. Tests replace
// them to capture invocations and return canned responses without spawning
// processes or reaching GitHub.
//
// projectRunStdin uses MustRunStdinWithRetry (abort-on-fail). The single call
// site (setStatusOptions for the GraphQL updateProjectV2Field mutation) is
// already followed by log.Fatalf in EnsureProjectBoardAutoCreate, so the
// abort semantics are preserved one layer deeper.
//
// projectRun uses RunWithRetry so the gh project link call is transient-
// resilient. RunWithRetry shares Run's signature, so the seam swap is a
// one-line change with no caller updates.
var (
	projectRunCapture        = shell.RunCaptureWithRetry
	projectRunStdin          = shell.MustRunStdinWithRetry
	projectRun               = shell.RunWithRetry
	projectConfirmFn         = readProjectConfirmation
	projectLoadSourceConfig  = projectconfig.LoadFromPath
	projectWriteSourceConfig = projectconfig.WriteToPath
)

// ghProject identifies a GitHub Project (v2) by its API + URL fields.
// Owner is populated from the caller, not the JSON — `gh project list/create`
// emits owner as an object (`{"login":..., "type":...}`) which would fail
// to unmarshal into a string field.
type ghProject struct {
	ID     string
	Number int
	Title  string
	URL    string
	Owner  string `json:"-"`
}

type projectListResponse struct {
	Projects []ghProject `json:"projects"`
}

type projectFieldOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type projectField struct {
	ID      string               `json:"id"`
	Name    string               `json:"name"`
	Type    string               `json:"type"`
	Options []projectFieldOption `json:"options"`
}

type projectFieldListResponse struct {
	Fields []projectField `json:"fields"`
}

// EnsureProjectBoard creates or verifies the GitHub Project (v2) board the
// ATDD pipeline depends on. Two sub-paths:
//
//   - Path A — cfg.ProjectURL == "": list/create a project named cfg.SystemName
//     under cfg.Owner, replace the default Status set with the canonical six,
//     link the scaffolded repo(s), and write the URL back into cfg.ProjectURL
//     so the later WriteOptivemYAML step bakes it into gh-optivem.yaml. When
//     the source gh-optivem.yaml init read at startup had project.url empty,
//     the auto-created URL is also persisted back into that source file
//     (cfg.SourceConfigPath) so re-runs and future commands keyed off it
//     see the URL — reused-by-title runs (source URL already set) leave
//     the source file alone.
//
//   - Path B — cfg.ProjectURL set: validate the operator-supplied URL exposes
//     every ATDD-required Status option. If any are missing, prompt for
//     confirmation (or honour --yes) and additively add them — existing
//     options on the board are preserved.
//
// Short-circuits to a no-op when cfg.NoProject is true. The gh argument is
// accepted for signature consistency with sibling Setup* steps but unused —
// gh project commands take --owner/--repo per-call rather than a Repo binding.
func EnsureProjectBoard(cfg *config.Config, gh *shell.GitHub) {
	_ = gh // signature parity with sibling steps
	if cfg.NoProject {
		log.Info("Skipping project board step (--no-project)")
		return
	}
	if cfg.ProjectURL == "" {
		ensureProjectBoardAutoCreate(cfg)
		return
	}
	ensureProjectBoardSupplied(cfg)
}

// ensureProjectBoardAutoCreate is Path A. Creates a project (or reuses an
// existing one with the same title), forces the canonical Status set, and
// links the scaffolded repos.
func ensureProjectBoardAutoCreate(cfg *config.Config) {
	log.Infof("Ensuring project board for %s...", cfg.SystemName)

	pr, created, err := findOrCreateProject(cfg.Owner, cfg.SystemName)
	if err != nil {
		log.Fatalf("project board: %v", err)
	}
	if created {
		log.Successf("Created project board: %s", pr.URL)
	} else {
		log.Successf("Reusing existing project board: %s", pr.URL)
	}

	statusField, err := loadStatusField(pr)
	if err != nil {
		log.Fatalf("project board: %v", err)
	}
	added, err := setStatusOptions(statusField.ID, canonicalStatusOptions)
	if err != nil {
		log.Fatalf("project board: set Status options: %v", err)
	}
	if len(added) > 0 {
		log.Successf("Set project Status options: %s", strings.Join(added, ", "))
	}

	for _, repo := range reposToLink(cfg) {
		linkRepoToProject(pr, repo)
	}

	cfg.ProjectURL = pr.URL
	persistProjectURLToSourceConfig(cfg)
}

// persistProjectURLToSourceConfig writes cfg.ProjectURL back into the
// gh-optivem.yaml file init read at startup (cfg.SourceConfigPath), so a
// re-run sees the URL and the source file matches what was actually
// provisioned on GitHub. No-ops in two cases: source already had a URL
// (reused-by-title runs leave the file alone — no churn), and
// SourceConfigPath unset.
//
// SourceConfigPath == "" is the normal case for default-path init runs
// (no CWD copy by design — see internal/config/config.go's
// SourceConfigPath doc); the write-back here is a re-run convenience for
// operators who chose --config / $GH_OPTIVEM_CONFIG or had a pre-existing
// CWD file, not a load-bearing step.
//
// Marshalling is non-preserving: comments and key order in the source
// file are dropped on rewrite. Acceptable tradeoff — the same yaml
// emitter is what `config init` produces, and a partial-state failure
// (project created on GitHub but not recorded locally) is worse than a
// cosmetic reformat. log.Fatalf on any load/write error aborts init
// rather than leave the operator with a desynchronised source file.
func persistProjectURLToSourceConfig(cfg *config.Config) {
	if !cfg.SourceProjectURLWasEmpty {
		return
	}
	if cfg.SourceConfigPath == "" {
		return
	}
	pc, err := projectLoadSourceConfig(cfg.SourceConfigPath)
	if err != nil {
		log.Fatalf("persist project URL to %s: %v", cfg.SourceConfigPath, err)
	}
	pc.Project.URL = cfg.ProjectURL
	if err := projectWriteSourceConfig(cfg.SourceConfigPath, pc); err != nil {
		log.Fatalf("persist project URL to %s: %v", cfg.SourceConfigPath, err)
	}
	log.Successf("Wrote project URL back to %s", cfg.SourceConfigPath)
}

// ensureProjectBoardSupplied is Path B. Verifies the operator-supplied URL
// exposes the ATDD-required Status options; prompts before mutating.
func ensureProjectBoardSupplied(cfg *config.Config) {
	log.Infof("Verifying project board %s...", cfg.ProjectURL)

	owner, number, err := parseProjectURL(cfg.ProjectURL)
	if err != nil {
		log.Fatalf("--project-url %q: %v", cfg.ProjectURL, err)
	}

	pr := &ghProject{Owner: owner, Number: number, URL: cfg.ProjectURL}
	statusField, err := loadStatusField(pr)
	if err != nil {
		log.Fatalf("project board: %v", err)
	}

	currentNames := make([]string, 0, len(statusField.Options))
	for _, o := range statusField.Options {
		currentNames = append(currentNames, o.Name)
	}
	missing := missingOptions(currentNames, atddRequiredStatusOptions)

	if len(missing) == 0 {
		log.Successf("Project board verified: all required statuses present (%s)", cfg.ProjectURL)
		return
	}

	if !projectConfirmFn(cfg, missing) {
		log.Fatalf("%s", projectAddDeclinedMessage(missing))
	}

	merged := append([]string{}, currentNames...)
	merged = append(merged, missing...)
	added, err := setStatusOptions(statusField.ID, merged)
	if err != nil {
		log.Fatalf("project board: add missing statuses: %v", err)
	}
	if len(added) > 0 {
		log.Successf("Added project statuses: %s", strings.Join(added, ", "))
	}
}

// reposToLink returns the repos that should be linked to a Path A project
// board. Mirrors the WriteOptivemYAML repo-slug logic so the linked repos
// match the system-test/externals scope written into gh-optivem.yaml.
func reposToLink(cfg *config.Config) []string {
	if cfg.RepoStrategy != "multirepo" {
		return []string{cfg.FullRepo}
	}
	if cfg.Arch == "multitier" {
		return []string{cfg.BackendFullRepo, cfg.FrontendFullRepo}
	}
	return []string{cfg.SystemFullRepo}
}

// findOrCreateProject reuses an existing project under owner whose title
// matches systemName (case-sensitive — matches CreateRepo's identity check),
// or creates a new one. Returns the resolved project and whether it was
// freshly created.
func findOrCreateProject(owner, systemName string) (*ghProject, bool, error) {
	listOut, err := projectRunCapture(
		fmt.Sprintf("gh project list --owner %s --format json --limit 200", owner), "")
	if err != nil {
		return nil, false, fmt.Errorf("list projects: %w", err)
	}
	var listResp projectListResponse
	if err := json.Unmarshal([]byte(listOut), &listResp); err != nil {
		return nil, false, fmt.Errorf("parse project list: %w; raw=%q", err, listOut)
	}
	for i := range listResp.Projects {
		if listResp.Projects[i].Title == systemName {
			pr := listResp.Projects[i]
			pr.Owner = owner
			return &pr, false, nil
		}
	}

	createOut, err := projectRunCapture(
		fmt.Sprintf("gh project create --owner %s --title %q --format json", owner, systemName), "")
	if err != nil {
		return nil, false, fmt.Errorf("create project: %w", err)
	}
	var pr ghProject
	if err := json.Unmarshal([]byte(createOut), &pr); err != nil {
		return nil, false, fmt.Errorf("parse project create: %w; raw=%q", err, createOut)
	}
	if pr.URL == "" {
		return nil, false, fmt.Errorf("create project returned empty URL; raw=%q", createOut)
	}
	pr.Owner = owner
	return &pr, true, nil
}

// loadStatusField fetches the project's field list and returns the built-in
// Status SingleSelect field. Errors if absent — every ProjectV2 ships with
// one by default, so absence indicates either an API-shape change or a
// bespoke field replacement we cannot safely automate around.
func loadStatusField(pr *ghProject) (*projectField, error) {
	out, err := projectRunCapture(
		fmt.Sprintf("gh project field-list %d --owner %s --format json --limit 200", pr.Number, pr.Owner), "")
	if err != nil {
		return nil, fmt.Errorf("list project fields: %w", err)
	}
	var resp projectFieldListResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, fmt.Errorf("parse project field list: %w; raw=%q", err, out)
	}
	for i := range resp.Fields {
		if resp.Fields[i].Name == "Status" {
			return &resp.Fields[i], nil
		}
	}
	return nil, fmt.Errorf("project %s has no built-in Status field", pr.URL)
}

// missingOptions returns the entries in required that are not present in
// current (case-insensitive comparison, mirroring equalStatus in the ATDD
// runtime board package).
func missingOptions(current, required []string) []string {
	have := make(map[string]bool, len(current))
	for _, c := range current {
		have[strings.ToLower(c)] = true
	}
	var missing []string
	for _, r := range required {
		if !have[strings.ToLower(r)] {
			missing = append(missing, r)
		}
	}
	return missing
}

// setStatusOptions overwrites the Status field's option set with the names
// given. GitHub's updateProjectV2Field mutation has no per-option ID slot,
// so this is a wholesale replace — Path A relies on this to drop Todo / In
// Progress / Done; Path B passes existing names alongside new ones to
// effectively achieve an additive merge by name.
//
// Returns the names that were added relative to the canonical default set,
// for display.
func setStatusOptions(fieldID string, names []string) ([]string, error) {
	body, err := buildSetStatusOptionsRequest(fieldID, names)
	if err != nil {
		return nil, fmt.Errorf("build mutation: %w", err)
	}
	// projectRunStdin = shell.MustRunStdinWithRetry: aborts on hard-fail or
	// after retries are exhausted. The setStatusOptions caller already does
	// log.Fatalf on error returned from this function, so the abort semantics
	// are preserved (the wrapper just moves them one layer deeper). The
	// returned error path remains for buildSetStatusOptionsRequest failures
	// upstream.
	projectRunStdin("gh api graphql --input -", body, "")
	return names, nil
}

// buildSetStatusOptionsRequest renders the GraphQL request body for
// updateProjectV2Field with the given Status option names. Each option
// gets its mapped color (statusOptionColors) or GRAY as a fallback. The
// description field is required by the input type but accepts empty.
func buildSetStatusOptionsRequest(fieldID string, names []string) (string, error) {
	type optionInput struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Description string `json:"description"`
	}
	opts := make([]optionInput, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		key := strings.ToLower(n)
		if seen[key] {
			continue
		}
		seen[key] = true
		color, ok := statusOptionColors[n]
		if !ok {
			color = "GRAY"
		}
		opts = append(opts, optionInput{Name: n, Color: color, Description: ""})
	}
	req := map[string]any{
		"query": "mutation($fieldId:ID!,$options:[ProjectV2SingleSelectFieldOptionInput!]!){" +
			"updateProjectV2Field(input:{fieldId:$fieldId,singleSelectOptions:$options}){" +
			"projectV2Field{__typename}}}",
		"variables": map[string]any{
			"fieldId": fieldID,
			"options": opts,
		},
	}
	b, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// linkRepoToProject runs `gh project link` and tolerates "already linked"
// responses gracefully — re-runs against an existing project should be no-ops,
// not failures.
func linkRepoToProject(pr *ghProject, fullRepo string) {
	cmd := fmt.Sprintf("gh project link %d --owner %s --repo %s", pr.Number, pr.Owner, fullRepo)
	out, err := projectRun(cmd, false, "")
	if err != nil {
		if isAlreadyLinkedOutput(out) {
			log.Infof("Repo %s already linked to project %s — skipping", fullRepo, pr.URL)
			return
		}
		log.Fatalf("link repo %s to project %s: %v\n%s", fullRepo, pr.URL, err, out)
	}
	log.Successf("Linked %s to project %s", fullRepo, pr.URL)
}

// isAlreadyLinkedOutput probes the gh CLI's link error for a "already linked"
// signal. The CLI does not return a stable status code for this case, so we
// match on substrings — both "already linked" and "Repository ... already"
// have shown up in the wild across gh versions.
func isAlreadyLinkedOutput(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, "already linked") ||
		strings.Contains(lower, "already exists")
}

// parseProjectURL extracts owner + project number from a GitHub Project (v2)
// URL. Accepts both user-owned and org-owned forms:
//
//	https://github.com/users/<owner>/projects/<n>
//	https://github.com/orgs/<owner>/projects/<n>
func parseProjectURL(s string) (owner string, number int, err error) {
	u, perr := url.Parse(s)
	if perr != nil {
		return "", 0, fmt.Errorf("malformed URL: %w", perr)
	}
	if u.Host != "github.com" {
		return "", 0, fmt.Errorf("expected host github.com, got %q", u.Host)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 4 || (parts[0] != "users" && parts[0] != "orgs") || parts[2] != "projects" {
		return "", 0, fmt.Errorf("expected URL of form https://github.com/{users|orgs}/<owner>/projects/<n>")
	}
	n, cerr := strconv.Atoi(parts[3])
	if cerr != nil || n <= 0 {
		return "", 0, fmt.Errorf("project number must be a positive integer: %s", parts[3])
	}
	return parts[1], n, nil
}

// readProjectConfirmation prompts the operator before adding missing Status
// options on a supplied --project-url. Honours --yes for unattended runs;
// returns false on any other input or when stdin is not readable (a non-TTY
// CI run without --yes lands here, and the caller fails with guidance to
// pass --yes or --no-project).
func readProjectConfirmation(cfg *config.Config, missing []string) bool {
	printMissingOptionsBanner(cfg.ProjectURL, missing)
	if cfg.AssumeYes {
		log.Info("Proceeding without confirmation (--yes).")
		return true
	}
	ok, err := promptio.ConfirmYN(os.Stdin, os.Stdout, "  Add missing statuses?")
	if err != nil {
		return false
	}
	return ok
}

func printMissingOptionsBanner(projectURL string, missing []string) {
	fmt.Println()
	fmt.Println("==========================================")
	fmt.Println("  Project board missing required statuses")
	fmt.Println("==========================================")
	fmt.Println()
	fmt.Printf("  The project at %s is missing required ATDD statuses:\n\n", projectURL)
	for _, m := range missing {
		fmt.Printf("    - %s\n", m)
	}
	fmt.Println()
	fmt.Println("  To proceed, gh-optivem needs to add these options to the project's Status field.")
	fmt.Println("  Existing options will be preserved. No options will be renamed or removed.")
	fmt.Println()
}

// projectAddDeclinedMessage builds the abort message when the operator
// declines the Path B prompt. Differentiates by which option(s) were
// declined so the operator knows whether the missing column is
// runtime-critical (Ready/In Progress/In Acceptance — read/written by
// internal/atdd/runtime/tracker) or workflow-critical (In QA — needed
// for the QA hand-off lane but not by the runtime today).
func projectAddDeclinedMessage(missing []string) string {
	var atddRuntime, qaOnly []string
	for _, m := range missing {
		switch strings.ToLower(m) {
		case "in qa":
			qaOnly = append(qaOnly, m)
		default:
			atddRuntime = append(atddRuntime, m)
		}
	}
	var b strings.Builder
	b.WriteString("Aborted: declined to add required project Status options.\n")
	if len(atddRuntime) > 0 {
		fmt.Fprintf(&b, "  Missing runtime-critical: %s\n", strings.Join(atddRuntime, ", "))
		b.WriteString("    ATDD runtime will fail without these — the pipeline reads/writes them directly.\n")
	}
	if len(qaOnly) > 0 {
		fmt.Fprintf(&b, "  Missing workflow column: %s\n", strings.Join(qaOnly, ", "))
		b.WriteString("    ATDD runtime does not read/write this today, but the broader board workflow expects it for the QA hand-off lane.\n")
	}
	b.WriteString("  Re-run with --yes to bypass the prompt, --no-project to skip the board step entirely, or add the options yourself via the GitHub UI.")
	return b.String()
}
