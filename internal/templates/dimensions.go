package templates

import (
	"os"
	"path/filepath"
	"strings"
)

// StripFixedDimensions fixes up the scaffolded system-test/ directory after
// SelectDockerCompose has flattened shop's per-arch subdir into the parent.
//
// Shop now organizes per-arch content under system-test/<lang>/<arch>/ (compose
// files, arch config PS1, arch README), so most dimension stripping has already
// been done at shop-level. What remains is script surgery on the scaffolded
// Run-SystemTests.ps1 and its arch-agnostic README, since those are shared
// across shop's archs via a parameter:
//
//  1. Run-SystemTests.ps1:
//     - Remove the [ValidateSet("multitier","monolith")] + $Architecture parameter.
//     - Remove the .system/config.json auto-detect fallback block.
//     - Drop the $Architecture/ prefix from the compose-filename template (files
//       sit at the sibling level now, not under an arch subdir).
//     - Drop the $Architecture/ prefix from the dot-source path of the per-arch
//       config (moved to the same level by SelectDockerCompose).
//     - Drop the $Architecture / prefix from the per-mode display heading.
//  2. system-test/README.md (arch-agnostic): strip the "selects arch via
//     -Architecture" prose and the links to arch subdirs (which no longer exist).
func StripFixedDimensions(testDst string) {
	stripRunSystemTestsPS1(testDst)
	stripSystemTestReadme(testDst)
}

func stripRunSystemTestsPS1(testDst string) {
	path := filepath.Join(testDst, "Run-SystemTests.ps1")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	// Shop's PS1 files ship with CRLF. Our patterns are written with LF; normalize
	// on read, apply transforms, write back as LF (scaffolded repo is fresh so no
	// existing line-ending convention to preserve).
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	content = stripArchParameter(content)
	content = stripArchAutoDetect(content)
	content = stripArchFromComposeFilename(content)
	content = stripArchFromDotSource(content)
	content = stripArchFromDisplayHeading(content)
	os.WriteFile(path, []byte(content), 0644)
}

// stripArchParameter removes the ValidateSet + $Architecture parameter block
// that shop uses to let `-Architecture monolith|multitier` pick at runtime.
func stripArchParameter(content string) string {
	old := "    [ValidateSet(\"multitier\", \"monolith\")]\n    [string]$Architecture,\n"
	return strings.Replace(content, old, "", 1)
}

// stripArchAutoDetect removes shop's optional .system/config.json fallback
// (the block that lets a scaffolded repo auto-detect its arch). In the new
// flow that block is unreachable anyway — the $Architecture parameter has been
// removed, so `if (-not $Architecture)` is always true and we simply drop the
// whole conditional.
func stripArchAutoDetect(content string) string {
	start := "# Auto-detect architecture from project config if not specified"
	end := "Use -Architecture monolith|multitier\"\n    }\n}\n"
	startIdx := strings.Index(content, start)
	if startIdx < 0 {
		return content
	}
	endIdx := strings.Index(content[startIdx:], end)
	if endIdx < 0 {
		return content
	}
	return content[:startIdx] + content[startIdx+endIdx+len(end):]
}

// stripArchFromComposeFilename drops the $Architecture subdir from the compose
// filename template, since the scaffolded repo flattens that subdir:
//
//	"$Architecture/docker-compose.$Mode.$ExternalMode.yml"
//	→ "docker-compose.$Mode.$ExternalMode.yml"
func stripArchFromComposeFilename(content string) string {
	return strings.Replace(content,
		`"$Architecture/docker-compose.$Mode.$ExternalMode.yml"`,
		`"docker-compose.$Mode.$ExternalMode.yml"`,
		1)
}

// stripArchFromDotSource drops the $Architecture subdir from the arch-config
// dot-source path, since that config file has been moved up to the sibling
// level by SelectDockerCompose.
func stripArchFromDotSource(content string) string {
	return strings.Replace(content,
		`"$PSScriptRoot/$Architecture/Run-SystemTests.Config.Architecture.ps1"`,
		`"$PSScriptRoot/Run-SystemTests.Config.Architecture.ps1"`,
		1)
}

// stripArchFromDisplayHeading rewrites the per-mode banner heading so it no
// longer interpolates the removed $Architecture variable:
//
//	Write-Heading -Text "System: $Architecture / $($externalMode.ToUpper())"
//	→ Write-Heading -Text "System: $($externalMode.ToUpper())"
func stripArchFromDisplayHeading(content string) string {
	return strings.Replace(content,
		`"System: $Architecture / $($externalMode.ToUpper())"`,
		`"System: $($externalMode.ToUpper())"`,
		1)
}

// stripSystemTestReadme removes the architecture-choice machinery from shop's
// arch-agnostic system-test/README.md:
//
//   - The "The entry-point script Run-SystemTests.ps1 accepts -Architecture …"
//     paragraph (the script no longer takes that parameter).
//   - The "## Architectures" section with links to monolith/ and multitier/
//     subdirectories (those dirs don't exist in the scaffolded repo).
func stripSystemTestReadme(testDst string) {
	path := filepath.Join(testDst, "README.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	// Normalize line endings — shop ships both LF and CRLF READMEs.
	content := strings.ReplaceAll(string(data), "\r\n", "\n")

	start := "## Architectures\n"
	end := "\n## Available Suite IDs\n"
	s := strings.Index(content, start)
	e := strings.Index(content, end)
	if s >= 0 && e > s {
		content = content[:s] + content[e+1:]
	}
	os.WriteFile(path, []byte(content), 0644)
}
