package actions

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// WIP-gate removal at Green (plan 20261109-1300-remove-wip-gate-at-green).
//
// The acceptance-test writer adds a WIP gate to every acceptance test it adds
// (runtime/shared/wip-gate-<lang>.md). The gate keys on GH_OPTIVEM_RUN_WIP_TESTS,
// which only the orchestrator's own verify runs set (command.go), so a gated
// test is skipped by CI, local and IDE runs. Once the ticket's acceptance tests
// pass on every touched channel, remove-wip-gate strips the gate from exactly
// those tests (the AT writer's test-names), so the pipeline runs them from then
// on. Deterministic — no agent. The shapes matched below are the ones the
// wip-gate-<lang>.md snippets prescribe; keep the two in sync.

const wipGateEnvVar = wipTestsEnvVar

var (
	javaMethodRE   = regexp.MustCompile(`\bvoid\s+([A-Za-z_]\w*)\s*\(`)
	csharpMethodRE = regexp.MustCompile(`\b(?:Task|void)\s+([A-Za-z_]\w*)\s*\(`)
	tsTestRE       = regexp.MustCompile("\\btest\\(\\s*['\"`]([A-Za-z_]\\w*)['\"`]")
)

const javaWipGateImport = "import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;"

// wipGateExtensions maps the system-test language to the source-file
// extension its acceptance tests use.
var wipGateExtensions = map[string]string{
	projectconfig.LangJava:       ".java",
	projectconfig.LangDotnet:     ".cs",
	projectconfig.LangTypescript: ".ts",
}

// removeWipGate is the REMOVE_WIP_GATE service task.
func (a actions) removeWipGate(ctx *statemachine.Context) statemachine.Outcome {
	cfg := a.deps.Config
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("remove-wip-gate: no gh-optivem config wired (driver bug)")}
	}
	names := splitTestNames(ctx.GetString("test-names"))
	if len(names) == 0 {
		return statemachine.Outcome{Err: fmt.Errorf("remove-wip-gate: test-names is empty; the acceptance-test writer reports every test it added or modified, so there is nothing to un-gate only if that output was lost")}
	}
	lang := cfg.SystemTest.Lang
	ext, ok := wipGateExtensions[lang]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("remove-wip-gate: unsupported system-test language %q (want java, dotnet or typescript)", lang)}
	}
	roots, err := ResolveLayerPaths([]string{"at-test"}, cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("remove-wip-gate: %w", err)}
	}

	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	found := map[string]bool{}
	var stripped []string
	for _, root := range roots {
		dir := root
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(a.deps.RepoPath, dir)
		}
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ext {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out, fileStripped, fileFound := StripWipGate(lang, string(src), want)
			for _, n := range fileFound {
				found[n] = true
			}
			if len(fileStripped) == 0 {
				return nil
			}
			stripped = append(stripped, fileStripped...)
			return os.WriteFile(path, []byte(out), 0o644)
		})
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("remove-wip-gate: scanning %s: %w", dir, err)}
		}
	}

	var missing []string
	for _, n := range names {
		if !found[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return statemachine.Outcome{Err: fmt.Errorf("remove-wip-gate: test(s) reported by the acceptance-test writer not found under %s: %s", strings.Join(roots, ", "), strings.Join(missing, ", "))}
	}
	slices.Sort(stripped)
	if len(stripped) == 0 {
		fmt.Fprintf(a.deps.Out.Phase, "No WIP gate to remove (%d ticket test(s) were not gated).\n", len(names))
	} else {
		fmt.Fprintf(a.deps.Out.Phase, "Removed the WIP gate from %d test(s): %s\n", len(stripped), strings.Join(stripped, ", "))
	}
	return statemachine.Outcome{}
}

// StripWipGate removes the WIP gate from the test methods named in want.
// It returns the new source, the names whose gate was removed, and every
// wanted name that was found in the source (gated or not). A found test
// without a gate is left as is: the AT writer gates only the tests it adds,
// not pre-existing tests it reconciled.
func StripWipGate(lang, src string, want map[string]bool) (string, []string, []string) {
	lines := strings.Split(src, "\n")
	drop := map[int]bool{}
	var stripped, found []string

	isGateLine := func(i int, marker string) bool {
		t := strings.TrimSpace(lines[i])
		return strings.Contains(t, marker) && strings.Contains(t, wipGateEnvVar)
	}

	switch lang {
	case projectconfig.LangJava:
		for i, l := range lines {
			m := javaMethodRE.FindStringSubmatch(l)
			if m == nil || !want[m[1]] {
				continue
			}
			found = append(found, m[1])
			for j := i - 1; j >= 0 && strings.HasPrefix(strings.TrimSpace(lines[j]), "@"); j-- {
				if isGateLine(j, "@EnabledIfEnvironmentVariable(") {
					drop[j] = true
					stripped = append(stripped, m[1])
					break
				}
			}
		}
	case projectconfig.LangDotnet:
		for i, l := range lines {
			m := csharpMethodRE.FindStringSubmatch(l)
			if m == nil || !want[m[1]] {
				continue
			}
			found = append(found, m[1])
			// The gate is the first statement of the body.
			k := i
			for k < len(lines) && !strings.Contains(lines[k], "{") {
				k++
			}
			k++
			for k < len(lines) && strings.TrimSpace(lines[k]) == "" {
				k++
			}
			if k >= len(lines) || !isGateLine(k, "Skip.IfNot(") {
				continue
			}
			drop[k] = true
			stripped = append(stripped, m[1])
			// [SkippableTheory]/[SkippableFact] were only needed for the gate.
			for j := i - 1; j >= 0 && strings.HasPrefix(strings.TrimSpace(lines[j]), "["); j-- {
				lines[j] = strings.Replace(lines[j], "[SkippableTheory]", "[Theory]", 1)
				lines[j] = strings.Replace(lines[j], "[SkippableFact]", "[Fact]", 1)
			}
		}
	case projectconfig.LangTypescript:
		for i, l := range lines {
			m := tsTestRE.FindStringSubmatch(l)
			if m == nil || !want[m[1]] {
				continue
			}
			found = append(found, m[1])
			k := i + 1
			for k < len(lines) && strings.TrimSpace(lines[k]) == "" {
				k++
			}
			if k < len(lines) && isGateLine(k, "test.skip(") {
				drop[k] = true
				stripped = append(stripped, m[1])
			}
		}
	}

	if len(drop) == 0 {
		return src, nil, found
	}
	out := make([]string, 0, len(lines))
	for i, l := range lines {
		if !drop[i] {
			out = append(out, l)
		}
	}
	result := strings.Join(out, "\n")
	if lang == projectconfig.LangJava && !strings.Contains(result, "@EnabledIfEnvironmentVariable") {
		result = removeLine(result, javaWipGateImport)
	}
	return result, stripped, found
}

// removeLine drops every line whose trimmed content equals target.
func removeLine(src, target string) string {
	lines := strings.Split(src, "\n")
	out := lines[:0]
	for _, l := range lines {
		if strings.TrimSpace(l) == target {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
