package actions

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/intake"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
	"github.com/optivem/gh-optivem/internal/spec"
	"github.com/optivem/gh-optivem/internal/spec/generate"
)

// Per-use-case State of the characterize flow (plan 20261110-1130, decision
// "Per-use-case State"). The bulk passes run one layer over every use case
// before the next layer, so State a writer emitted for one use case (its
// test-names, its port-changed verdicts) is stale by the time the next layer
// reaches that use case. select-next-use-case therefore re-derives it from the
// files and the branch history each time.

// CtxKeySystemTestStartCommit is the system-test repo commit characterize
// started from (record-start-commits, kept on resume): the lower bound of the
// use case commits useCasePortChanges reads.
const CtxKeySystemTestStartCommit = "system-test-start-commit"

// stampUseCase stamps the State the layers read for use case id:
//
//   - use-case-id, and the ESCC keys from the criteria file's External System
//     Contract Criteria section (stampESCC, as parse-ticket stamps them).
//   - acceptance-criteria / scenario-criteria from the criteria file
//     (stampUseCaseCriteria).
//   - test-names: the acceptance tests tagged with the use case's criteria.
//   - dsl-port-changed, system-driver-port-changed, external-driver-port-changed
//     and external-driver-port-changed-paths: from the files under each port
//     directory that the use case's layer commits changed (useCasePortChanges).
//
// A use case with no criteria file yet (the criteria layer writes it after
// this runs) has no ESCC and no criteria; one with no tagged tests yet (before
// its acceptance test layer) has no test-names; one whose layers have not
// committed yet has no port change. A key without a value is unset rather
// than emptied, so a step that needs it halts on the strict ${...} expansion
// instead of running on a blank (an empty test-names runs the whole suite).
// Everything is computed before anything is stamped, so a failure leaves the
// State as it was.
func (a actions) stampUseCase(ctx *statemachine.Context, id string) error {
	cfg := a.deps.Config
	if cfg == nil {
		return fmt.Errorf("no gh-optivem config wired (driver bug)")
	}
	file, body, err := a.useCaseFile(id)
	if err != nil {
		return err
	}
	// The file is not a ticket body (front matter, narrative and business-rule
	// headings), so only its ESCC section goes through intake's parser, which
	// validates it and extracts the system names exactly as for a ticket.
	escc := intake.ExtractSection(body, intake.SectionExternalSystemContractCriteria).Body
	r, err := intake.ParseSections(map[string]string{intake.SectionExternalSystemContractCriteria: escc})
	if err != nil {
		return fmt.Errorf("use case %s: %w", id, err)
	}
	names, err := a.useCaseTestNames(cfg, []string{id})
	if err != nil {
		return fmt.Errorf("use case %s: %w", id, err)
	}
	ports, err := a.useCasePortChanges(ctx, cfg, id)
	if err != nil {
		return fmt.Errorf("use case %s: %w", id, err)
	}

	ctx.Set(CtxKeyUseCaseID, id)
	stampESCC(ctx, r.ExternalSystemContractCriteria)
	stampUseCaseCriteria(ctx, file, body)
	setOrUnset(ctx, "test-names", names)
	for _, src := range useCasePortChangeSources {
		ctx.Set(src.key, len(ports[src.key]) > 0)
	}
	setOrUnset(ctx, "external-driver-port-changed-paths", strings.Join(ports["external-driver-port-changed"], "\n"))
	return nil
}

// useCaseFile returns the spec-dir-relative path and the content (LF line
// endings) of the use case's criteria file, or "", "" when it has none yet.
func (a actions) useCaseFile(id string) (string, string, error) {
	ucs, dir, err := a.loadUseCases()
	if err != nil {
		return "", "", err
	}
	for _, uc := range ucs {
		if uc.ID != id {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(uc.File)))
		if err != nil {
			return "", "", err
		}
		return uc.File, strings.ReplaceAll(string(data), "\r\n", "\n"), nil
	}
	return "", "", nil
}

// useCaseCriteriaFile is the criteria file of the folder layout
// (UC-NNN-<slug>/acceptance-criteria.md), which holds the criteria with no
// "Acceptance criteria" heading of its own.
const useCaseCriteriaFile = "acceptance-criteria.md"

// stampUseCaseCriteria stamps what the acceptance test layer reads, as
// parse-ticket and the story merge stamp it for a ticket: acceptance-criteria
// (the "Acceptance criteria" section, or the folder layout's criteria file
// without its front matter and ESCC section) and scenario-criteria (one
// "<criterion text> => UC-NNN.M" line per criterion, rendered as the writer's
// use case traceability so it tags each test with its criterion).
func stampUseCaseCriteria(ctx *statemachine.Context, file, body string) {
	criteria := ""
	if s := intake.ExtractSection(body, intake.SectionAcceptanceCriteria); s.Found {
		criteria = s.Body
	} else if path.Base(file) == useCaseCriteriaFile {
		criteria = strings.TrimSpace(withoutESCCSection(withoutFrontMatter(body)))
	}
	var lines []string
	if file != "" {
		for _, c := range spec.Parse(file, body).Criteria {
			lines = append(lines, c.Text+" => "+c.ID)
		}
	}
	setOrUnset(ctx, "acceptance-criteria", criteria)
	setOrUnset(ctx, CtxKeyScenarioCriteria, strings.Join(lines, "\n"))
}

// withoutFrontMatter drops a leading "---" ... "---" block.
func withoutFrontMatter(body string) string {
	lines := strings.Split(body, "\n")
	if strings.TrimSpace(lines[0]) != "---" {
		return body
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return body
}

// withoutESCCSection drops the External System Contract Criteria section,
// from its heading to the end of the file (where the format puts it).
func withoutESCCSection(body string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "##") && strings.EqualFold(strings.TrimSpace(strings.TrimLeft(t, "#")), intake.SectionExternalSystemContractCriteria) {
			return strings.Join(lines[:i], "\n")
		}
	}
	return body
}

// setOrUnset sets a non-empty value and unsets an empty one.
func setOrUnset[T string | []string](ctx *statemachine.Context, key string, v T) {
	if len(v) == 0 {
		ctx.Unset(key)
		return
	}
	ctx.Set(key, v)
}

// useCaseTestNames returns the acceptance tests under at-test (read where
// remove-wip-gate reads them) tagged with a criterion of one of the use
// cases. An at-test directory that does not exist yet holds no tests.
func (a actions) useCaseTestNames(cfg *projectconfig.Config, useCases []string) ([]string, error) {
	roots, err := ResolveLayerPaths([]string{"at-test"}, cfg)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, root := range roots {
		dir := root
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(a.deps.RepoPath, dir)
		}
		if _, err := os.Stat(dir); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		dirs = append(dirs, dir)
	}
	names, err := generate.TaggedTestNames(dirs, useCases)
	if err != nil {
		return nil, fmt.Errorf("read the tests tagged with %s: %w", strings.Join(useCases, ", "), err)
	}
	return names, nil
}

// useCasePortChangeSources maps each port-changed flag to the layer label of
// the commits that can change that port for a use case, and the port's layer
// key. The labels are the `layer:` values of the commits in
// write-and-verify-acceptance-test-code ("ACCEPTANCE TESTS${layer-suffix}")
// and implement-and-verify-dsl ("DSL${layer-suffix}") under characterize's
// layer-suffix " (${use-case-id})": the acceptance-test writer changes the DSL
// port and the DSL implementer the driver ports, the same writers whose
// verdicts set these flags on the ticket path.
var useCasePortChangeSources = []struct{ key, label, layer string }{
	{"dsl-port-changed", "ACCEPTANCE TESTS", "dsl-port"},
	{"system-driver-port-changed", "DSL", "system-driver-port"},
	{"external-driver-port-changed", "DSL", "external-system-driver-port"},
}

// useCasePortChanges returns, per port-changed flag, the files under the
// port's directory that the use case's layer commits changed since the
// system-test start commit. Directory-keyed like the writers' verdicts: any
// file under the port directory counts (interface, DTO, enum), not only
// methods. A commit belongs to the use case when its message carries
// " - <label> (<use case id>)", the commit process's
// "#${ticket-id} ${issue-title} - ${layer}"; a redone layer adds commits on
// top, so every matching commit counts.
func (a actions) useCasePortChanges(ctx *statemachine.Context, cfg *projectconfig.Config, id string) (map[string][]string, error) {
	start := ctx.GetString(CtxKeySystemTestStartCommit)
	if start == "" {
		return nil, fmt.Errorf("%s not in State; record-start-commits must run first", CtxKeySystemTestStartCommit)
	}
	dir, err := a.repoDir(cfg, cfg.SystemTest.Repo)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, src := range useCasePortChangeSources {
		roots, err := ResolveLayerPaths([]string{src.layer}, cfg)
		if err != nil {
			return nil, err
		}
		if len(roots) == 0 {
			continue
		}
		args := []string{"log", "--no-renames", "--name-only", "--format=", "--fixed-strings",
			"--grep= - " + src.label + " (" + id + ")", start + "..HEAD", "--"}
		raw, err := a.gitIn(dir, append(args, roots...)...)
		if err != nil {
			return nil, err
		}
		var files []string
		for _, f := range splitNonEmptyLines(string(raw)) {
			if pathInScope(cleanRelPath(f), roots) && !slices.Contains(files, f) {
				files = append(files, f)
			}
		}
		slices.Sort(files)
		out[src.key] = files
	}
	return out, nil
}

// stampCharacterizedTestNames runs before characterize's REMOVE_WIP_GATE. The
// acceptance test layer's test-names holds one use case's tests at a time, so
// it stamps test-names with the tests tagged with a criterion of any
// characterized use case (core-use-case plus other-use-cases): remove-wip-gate
// then un-gates every test the run wrote. No tagged test at all means the
// acceptance test layers wrote nothing to un-gate, a wiring or tagging bug.
func (a actions) stampCharacterizedTestNames(ctx *statemachine.Context) statemachine.Outcome {
	cfg := a.deps.Config
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("stamp-characterized-test-names: no gh-optivem config wired (driver bug)")}
	}
	core := ctx.GetString("core-use-case")
	ids := splitUseCaseIDs(core + "," + strings.Join(stringListState(ctx, "other-use-cases"), ","))
	if strings.TrimSpace(core) == "" {
		return statemachine.Outcome{Err: fmt.Errorf("stamp-characterized-test-names: core-use-case is not in State; the use case diagram writer must have run")}
	}
	names, err := a.useCaseTestNames(cfg, ids)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("stamp-characterized-test-names: %w", err)}
	}
	if len(names) == 0 {
		return statemachine.Outcome{Err: fmt.Errorf("stamp-characterized-test-names: no acceptance test is tagged with a criterion of %s; every characterization test must carry its UC-NNN.M tag", strings.Join(ids, ", "))}
	}
	ctx.Set("test-names", names)
	fmt.Fprintf(a.deps.Out.Phase, "%d characterization test(s) across %d use case(s) to un-gate.\n", len(names), len(ids))
	return statemachine.Outcome{}
}
