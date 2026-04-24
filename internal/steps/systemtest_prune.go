package steps

import (
	"os"
	"path/filepath"
	"strings"
)

// PruneSystemTestArch strips the other-architecture's content from the
// scaffolded system-test directory. Shop's Run-SystemTests.ps1 and README.md
// document both monolith and multitier side-by-side; a scaffolded single-arch
// repo only needs its chosen arch.
func PruneSystemTestArch(systemTestDir, arch string) {
	otherArch := otherArchOf(arch)
	pruneRunSystemTests(filepath.Join(systemTestDir, "Run-SystemTests.ps1"), arch, otherArch)
	pruneSystemTestReadme(filepath.Join(systemTestDir, "README.md"), arch, otherArch)
}

func otherArchOf(arch string) string {
	if arch == "monolith" {
		return "multitier"
	}
	return "monolith"
}

// pruneRunSystemTests rewrites the scaffolded Run-SystemTests.ps1 so only the
// chosen architecture's hashtable branch, ValidateSet entry, and error-message
// hint remain. The other branch is dead code at runtime but leaks in greps.
func pruneRunSystemTests(path, arch, otherArch string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := string(data)
	content = stripPowerShellArchBlock(content, otherArch)
	content = strings.Replace(content,
		`[ValidateSet("multitier", "monolith")]`,
		`[ValidateSet("`+arch+`")]`, 1)
	content = strings.Replace(content,
		"Use -Architecture monolith|multitier",
		"Use -Architecture "+arch, 1)
	os.WriteFile(path, []byte(content), 0644)
}

// stripPowerShellArchBlock removes a PowerShell hashtable entry of the form
// `"<archName>" = @{ ... }` by brace-counting from the entry's opening line
// until they balance. The block's keys and values contain no `{` or `}` in
// strings, so a simple count is safe here.
func stripPowerShellArchBlock(content, archName string) string {
	lines := splitLinesKeepEndings(content)
	startIdx := findArchBlockStart(lines, archName)
	if startIdx < 0 {
		return content
	}
	endIdx := findBalancedEnd(lines, startIdx)
	if endIdx < 0 {
		return content
	}
	// Trim one adjacent blank line so we don't leave a double-blank gap.
	// Prefer the trailing blank (keeps leading spacing intact for the block
	// that remains); fall back to the leading blank if the stripped block
	// was the last entry in the hashtable.
	if endIdx+1 < len(lines) && isBlankLine(lines[endIdx+1]) {
		endIdx++
	} else if startIdx > 0 && isBlankLine(lines[startIdx-1]) {
		startIdx--
	}
	kept := make([]string, 0, len(lines)-(endIdx-startIdx+1))
	kept = append(kept, lines[:startIdx]...)
	kept = append(kept, lines[endIdx+1:]...)
	return strings.Join(kept, "")
}

// splitLinesKeepEndings splits on "\n" but keeps the terminator attached to
// each line so the join is byte-for-byte identical (except the stripped range)
// regardless of LF / CRLF endings.
func splitLinesKeepEndings(s string) []string {
	var out []string
	for {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			if s != "" {
				out = append(out, s)
			}
			return out
		}
		out = append(out, s[:i+1])
		s = s[i+1:]
	}
}

func isBlankLine(line string) bool {
	return strings.TrimSpace(line) == ""
}

func findArchBlockStart(lines []string, archName string) int {
	needle := `"` + archName + `" = @{`
	for i, line := range lines {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}

func findBalancedEnd(lines []string, startIdx int) int {
	depth := 0
	for i := startIdx; i < len(lines); i++ {
		depth += strings.Count(lines[i], "{") - strings.Count(lines[i], "}")
		if depth == 0 && i > startIdx {
			return i
		}
	}
	return -1
}

// pruneSystemTestReadme strips example blocks for the other architecture from
// the scaffolded system-test/README.md, and rewrites any remaining
// "-Architecture <otherArch>" arguments to the chosen arch.
func pruneSystemTestReadme(path, arch, otherArch string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	out := pruneReadmeLines(lines, arch, otherArch)
	os.WriteFile(path, []byte(strings.Join(out, "\n")), 0644)
}

// readmeSkipState tracks a small state machine used while pruning README
// sections belonging to the other architecture:
//
//	normal       → emit the line as-is
//	afterHeading → saw "Run all latest test suites (<otherArch>):", skip lines
//	               until we consume the opening code fence
//	inFence      → inside the fenced code block, skip until closing fence
//	afterFence   → just saw the closing fence, skip one trailing blank line
type readmeSkipState int

const (
	readmeNormal readmeSkipState = iota
	readmeAfterHeading
	readmeInFence
	readmeAfterFence
)

func pruneReadmeLines(lines []string, arch, otherArch string) []string {
	out := make([]string, 0, len(lines))
	state := readmeNormal
	headingMarker := "(" + otherArch + "):"
	legacyMarker := "-Architecture " + otherArch + " -Legacy"
	archFlagOld := "-Architecture " + otherArch
	archFlagNew := "-Architecture " + arch

	for _, raw := range lines {
		trimmed := strings.TrimRight(raw, "\r")
		if handled, nextState := stepReadmeSkipState(state, trimmed); handled {
			state = nextState
			continue
		}
		// Match the other-arch section heading (e.g. "(multitier):") and start
		// skipping its paragraph + fenced code block.
		if strings.Contains(trimmed, headingMarker) {
			state = readmeAfterHeading
			continue
		}
		// Drop the legacy bullet that targets only the other arch.
		if strings.Contains(trimmed, legacyMarker) {
			continue
		}
		// Any remaining "-Architecture <otherArch>" references (e.g. specific
		// suite / rebuild examples) get retargeted to the chosen arch.
		out = append(out, strings.ReplaceAll(raw, archFlagOld, archFlagNew))
	}
	return out
}

// stepReadmeSkipState advances the skip state machine for each line. Returns
// (handled=true) when the caller should skip emitting the current line.
func stepReadmeSkipState(state readmeSkipState, trimmed string) (bool, readmeSkipState) {
	switch state {
	case readmeAfterHeading:
		if strings.HasPrefix(trimmed, "```") {
			return true, readmeInFence
		}
		return true, readmeAfterHeading
	case readmeInFence:
		if strings.HasPrefix(trimmed, "```") {
			return true, readmeAfterFence
		}
		return true, readmeInFence
	case readmeAfterFence:
		if isBlankLine(trimmed) {
			return true, readmeNormal
		}
		return false, readmeNormal
	default:
		return false, readmeNormal
	}
}
