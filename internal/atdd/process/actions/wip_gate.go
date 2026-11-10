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
	"github.com/optivem/gh-optivem/internal/spec/generate"
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
	// tsTestStartRE matches a test declaration: plain `test(`, or the
	// parameterised `test.each(cases)(` / `test.eachAlsoFirstRow(cases)(`
	// (group 1 is set for the parameterised forms).
	tsTestStartRE = regexp.MustCompile(`\btest(\.each|\.eachAlsoFirstRow)?\(`)
	// tsTitleRE matches the title string at the start of the remaining text.
	tsTitleRE = regexp.MustCompile("^\\s*['\"`]([^'\"`]+)['\"`]")
)

// tsTestLookahead bounds how many lines after a test declaration are searched
// for its title and the start of its body.
const tsTestLookahead = 8

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
	in, err := a.wipGateInputs(ctx)
	if err != nil {
		return statemachine.Outcome{Err: err}
	}

	want := map[string]bool{}
	for _, n := range in.names {
		want[n] = true
	}
	found := map[string]bool{}
	var stripped []string
	for _, root := range in.roots {
		dir := root
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(a.deps.RepoPath, dir)
		}
		if err := stripWipGateUnder(dir, in.lang, in.ext, want, found, &stripped); err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("remove-wip-gate: scanning %s: %w", dir, err)}
		}
	}

	if missing := missingTestNames(in.names, found); len(missing) > 0 {
		return statemachine.Outcome{Err: fmt.Errorf("remove-wip-gate: test(s) reported by the acceptance-test writer not found under %s: %s", strings.Join(in.roots, ", "), strings.Join(missing, ", "))}
	}
	slices.Sort(stripped)
	if len(stripped) == 0 {
		fmt.Fprintf(a.deps.Out.Phase, "No WIP gate to remove (%d ticket test(s) were not gated).\n", len(in.names))
	} else {
		fmt.Fprintf(a.deps.Out.Phase, "Removed the WIP gate from %d test(s): %s\n", len(stripped), strings.Join(stripped, ", "))
	}
	return statemachine.Outcome{}
}

// wipGateInput is the validated input of REMOVE_WIP_GATE.
type wipGateInput struct {
	names []string
	lang  string
	ext   string
	roots []string
}

// wipGateInputs validates the wired config and the test-names output, and
// resolves the system-test language, its file extension and the
// acceptance-test roots.
func (a actions) wipGateInputs(ctx *statemachine.Context) (wipGateInput, error) {
	cfg := a.deps.Config
	if cfg == nil {
		return wipGateInput{}, fmt.Errorf("remove-wip-gate: no gh-optivem config wired (driver bug)")
	}
	names := splitTestNames(ctx.GetString("test-names"))
	if len(names) == 0 {
		return wipGateInput{}, fmt.Errorf("remove-wip-gate: test-names is empty; the acceptance-test writer reports every test it added or modified, so there is nothing to un-gate only if that output was lost")
	}
	lang := cfg.SystemTest.Lang
	ext, ok := wipGateExtensions[lang]
	if !ok {
		return wipGateInput{}, fmt.Errorf("remove-wip-gate: unsupported system-test language %q (want java, dotnet or typescript)", lang)
	}
	roots, err := ResolveLayerPaths([]string{"at-test"}, cfg)
	if err != nil {
		return wipGateInput{}, fmt.Errorf("remove-wip-gate: %w", err)
	}
	return wipGateInput{names: names, lang: lang, ext: ext, roots: roots}, nil
}

// stripWipGateUnder strips the WIP gate of the wanted tests from every
// ext-file under dir, rewriting only the files that changed. It records every
// wanted name it sees in found and appends the un-gated names to stripped.
func stripWipGateUnder(dir, lang, ext string, want, found map[string]bool, stripped *[]string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
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
		*stripped = append(*stripped, fileStripped...)
		return os.WriteFile(path, []byte(out), 0o644)
	})
}

// missingTestNames returns, in order, the names that are not in found.
func missingTestNames(names []string, found map[string]bool) []string {
	var missing []string
	for _, n := range names {
		if !found[n] {
			missing = append(missing, n)
		}
	}
	return missing
}

// StripWipGate removes the WIP gate from the test methods named in want.
// It returns the new source, the names whose gate was removed, and every
// wanted name that was found in the source (gated or not). A found test
// without a gate is left as is: the AT writer gates only the tests it adds,
// not pre-existing tests it reconciled.
func StripWipGate(lang, src string, want map[string]bool) (string, []string, []string) {
	s := &wipGateScan{lines: strings.Split(src, "\n"), want: want, drop: map[int]bool{}}

	switch lang {
	case projectconfig.LangJava:
		s.stripJava()
	case projectconfig.LangDotnet:
		s.stripDotnet()
	case projectconfig.LangTypescript:
		s.stripTypescript()
	}

	if len(s.drop) == 0 {
		return src, nil, s.found
	}
	result := strings.Join(s.keptLines(), "\n")
	if lang == projectconfig.LangJava && !strings.Contains(result, "@EnabledIfEnvironmentVariable") {
		result = removeLine(result, javaWipGateImport)
	}
	return result, s.stripped, s.found
}

// wipGateScan is the per-source state of StripWipGate: the source lines, the
// wanted test names, the gate lines to drop, and the names stripped and found
// so far.
type wipGateScan struct {
	lines    []string
	want     map[string]bool
	drop     map[int]bool
	stripped []string
	found    []string
}

// isGateLine reports whether line i carries marker and the WIP env var.
func (s *wipGateScan) isGateLine(i int, marker string) bool {
	t := strings.TrimSpace(s.lines[i])
	return strings.Contains(t, marker) && strings.Contains(t, wipGateEnvVar)
}

// wantedTest returns the test name re declares on line l when it is wanted,
// recording it as found.
func (s *wipGateScan) wantedTest(re *regexp.Regexp, l string) (string, bool) {
	m := re.FindStringSubmatch(l)
	if m == nil || !s.want[m[1]] {
		return "", false
	}
	s.found = append(s.found, m[1])
	return m[1], true
}

// markGate drops gate line k and records name as stripped.
func (s *wipGateScan) markGate(k int, name string) {
	s.drop[k] = true
	s.stripped = append(s.stripped, name)
}

// skipBlank returns the first index >= k whose line is not blank.
func (s *wipGateScan) skipBlank(k int) int {
	for k < len(s.lines) && strings.TrimSpace(s.lines[k]) == "" {
		k++
	}
	return k
}

func (s *wipGateScan) stripJava() {
	for i, l := range s.lines {
		name, ok := s.wantedTest(javaMethodRE, l)
		if !ok {
			continue
		}
		for j := i - 1; j >= 0 && strings.HasPrefix(strings.TrimSpace(s.lines[j]), "@"); j-- {
			if s.isGateLine(j, "@EnabledIfEnvironmentVariable(") {
				s.markGate(j, name)
				break
			}
		}
	}
}

func (s *wipGateScan) stripDotnet() {
	for i, l := range s.lines {
		name, ok := s.wantedTest(csharpMethodRE, l)
		if !ok {
			continue
		}
		// The gate is the first statement of the body.
		k := i
		for k < len(s.lines) && !strings.Contains(s.lines[k], "{") {
			k++
		}
		k = s.skipBlank(k + 1)
		if k >= len(s.lines) || !s.isGateLine(k, "Skip.IfNot(") {
			continue
		}
		s.markGate(k, name)
		s.unskipAttributes(i)
	}
}

// unskipAttributes rewrites the attribute block above the method on line i:
// [SkippableTheory]/[SkippableFact] were only needed for the gate.
func (s *wipGateScan) unskipAttributes(i int) {
	for j := i - 1; j >= 0 && strings.HasPrefix(strings.TrimSpace(s.lines[j]), "["); j-- {
		s.lines[j] = strings.Replace(s.lines[j], "[SkippableTheory]", "[Theory]", 1)
		s.lines[j] = strings.Replace(s.lines[j], "[SkippableFact]", "[Fact]", 1)
	}
}

// stripTypescript handles plain `test('name', async (...) => {` and the
// parameterised `test.each(cases)(` / `test.eachAlsoFirstRow(cases)(` forms,
// whose title may sit on the next line, carry a `_$param` suffix (compared
// stripped, as generate.TaggedTestNames reports it), and be followed by
// `criteria(...)` before the callback. The gate is the first statement of the
// callback body.
func (s *wipGateScan) stripTypescript() {
	for i, l := range s.lines {
		title, ok := s.tsTitle(i, l)
		if !ok {
			continue
		}
		name := title
		if !s.want[name] {
			name = generate.StripTitleParams(title)
		}
		if !s.want[name] {
			continue
		}
		s.found = append(s.found, name)
		body, ok := s.tsBodyStart(i)
		if !ok {
			continue
		}
		k := s.skipBlank(body + 1)
		if k < len(s.lines) && s.isGateLine(k, "test.skip(") {
			s.markGate(k, name)
		}
	}
}

// tsTitle returns the title of the test declared on line i, if any.
func (s *wipGateScan) tsTitle(i int, l string) (string, bool) {
	loc := tsTestStartRE.FindStringSubmatchIndex(l)
	if loc == nil {
		return "", false
	}
	rest := l[loc[1]:]
	if loc[2] >= 0 { // parameterised: the title follows the closing `)(`
		cut := strings.LastIndex(rest, ")(")
		if cut < 0 {
			return "", false
		}
		rest = rest[cut+2:]
	}
	if strings.TrimSpace(rest) == "" {
		k := s.skipBlank(i + 1)
		if k >= len(s.lines) {
			return "", false
		}
		rest = s.lines[k]
	}
	m := tsTitleRE.FindStringSubmatch(rest)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// tsBodyStart returns the line, from i on, that opens the test callback body
// (`=> {`), stopping at the next test declaration.
func (s *wipGateScan) tsBodyStart(i int) (int, bool) {
	for k := i; k < len(s.lines) && k <= i+tsTestLookahead; k++ {
		if k > i && tsTestStartRE.MatchString(s.lines[k]) {
			return 0, false
		}
		if strings.Contains(s.lines[k], "=>") && strings.HasSuffix(strings.TrimSpace(s.lines[k]), "{") {
			return k, true
		}
	}
	return 0, false
}

// keptLines returns the source lines not marked for dropping.
func (s *wipGateScan) keptLines() []string {
	out := make([]string, 0, len(s.lines))
	for i, l := range s.lines {
		if !s.drop[i] {
			out = append(out, l)
		}
	}
	return out
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
