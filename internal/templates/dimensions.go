package templates

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/optivem/gh-optivem/internal/files"
	"github.com/optivem/gh-optivem/internal/log"
)

// StripFixedDimensions removes the test-lang and architecture dimensions from
// pipeline artifacts in a scaffolded repo. Those dimensions are fixed at scaffold
// time (single test-lang, single arch), so their presence in filenames, compose
// project names, container names, workflow refs, and the PowerShell config
// hashtable is redundant.
//
// Call this after SelectDockerCompose (which already drops the compose files for
// the non-selected arch and renames the kept ones to drop the .<arch>. segment)
// and BEFORE the ReplaceRepoReferences infra replacement pass (which swaps
// "shop-" → "<repo>-").
//
// Transformations (keepArch is e.g. "monolith", removeArch e.g. "multitier",
// testLang e.g. "typescript"):
//
//  1. Compose file "name:" lines:
//     name: shop-<testLang>-<keepArch>-<mode> → name: shop-<mode>
//
//  2. Workflow "compose-file:" refs:
//     docker-compose.<scope>.<keepArch>.<mode>.yml → docker-compose.<scope>.<mode>.yml
//
//  3. Run-SystemTests.ps1:
//     - Strip [ValidateSet("multitier","monolith")] + $Architecture parameter
//     - Strip the .system/config.json auto-detection block
//     - Rewrite $script:ComposeFile template to drop $Architecture
//     - Prune $AllSystemConfig: delete the removeArch block, flatten keepArch
//       block (remove wrapping + dedent contents)
//     - Rewrite ContainerName values: shop-<testLang>-<keepArch>-<mode> → shop-<mode>
//     - Rewrite $SystemConfig lookup: $AllSystemConfig[$Architecture] → $AllSystemConfig
func StripFixedDimensions(repoDir, testDst, keepArch, removeArch, testLang string) {
	stripComposeNames(testDst, keepArch, testLang)
	stripWorkflowComposeRefs(repoDir, keepArch)
	stripRunSystemTestsPS1(testDst, keepArch, removeArch, testLang)
}

// stripComposeNames rewrites `name: shop-<lang>-<arch>-<mode>` → `name: shop-<mode>`
// in every compose file under testDst. After this, the existing shop→repo rewrite
// pass produces `name: <repo>-<mode>`.
func stripComposeNames(testDst, keepArch, testLang string) {
	for _, mode := range []string{"real", "stub"} {
		old := "name: shop-" + testLang + "-" + keepArch + "-" + mode
		new := "name: shop-" + mode
		n := files.ReplaceInTree(testDst, old, new, []string{ymlExt})
		if n > 0 {
			log.Successf("Stripped dim from compose name: %s → %s (%d files)", old, new, n)
		}
	}
}

// stripWorkflowComposeRefs rewrites `compose-file: docker-compose.<scope>.<arch>.<mode>.yml`
// → `compose-file: docker-compose.<scope>.<mode>.yml` in every workflow file.
func stripWorkflowComposeRefs(repoDir, keepArch string) {
	for _, scope := range []string{"local", "pipeline"} {
		for _, mode := range []string{"real", "stub"} {
			old := "docker-compose." + scope + "." + keepArch + "." + mode + ymlExt
			new := "docker-compose." + scope + "." + mode + ymlExt
			forEachWorkflowYml(repoDir, func(path string) {
				files.ReplaceInFile(path, old, new)
			})
		}
	}
}

// stripRunSystemTestsPS1 rewrites system-test/Run-SystemTests.ps1 to remove the
// $Architecture parameter and all references to the fixed arch/test-lang.
func stripRunSystemTestsPS1(testDst, keepArch, removeArch, testLang string) {
	path := filepath.Join(testDst, "Run-SystemTests.ps1")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := string(data)

	content = stripArchParameter(content)
	content = stripArchAutoDetect(content)
	content = stripArchFromComposeFilename(content)
	content = pruneAllSystemConfig(content, keepArch, removeArch)
	content = stripLangArchFromContainerNames(content, keepArch, testLang)
	content = stripArchFromSystemConfigLookup(content)

	os.WriteFile(path, []byte(content), 0644)
}

// stripArchParameter removes the ValidateSet + $Architecture parameter block.
// Shop has this structure:
//
//	[ValidateSet("multitier", "monolith")]
//	[string]$Architecture,
func stripArchParameter(content string) string {
	old := "    [ValidateSet(\"multitier\", \"monolith\")]\n    [string]$Architecture,\n"
	return strings.Replace(content, old, "", 1)
}

// stripArchAutoDetect removes the .system/config.json auto-detection block.
// Shop has this structure (between the param block and the $TestConfigFileName
// line):
//
//	# Auto-detect architecture from project config if not specified
//	if (-not $Architecture) {
//	    $configPath = Join-Path (Split-Path $PSScriptRoot -Parent) ".system" "config.json"
//	    if (Test-Path $configPath) {
//	        $projectConfig = Get-Content $configPath -Raw | ConvertFrom-Json
//	        $Architecture = $projectConfig.architecture
//	    } else {
//	        throw "Architecture not specified and .system/config.json not found. Use -Architecture monolith|multitier"
//	    }
//	}
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

// stripArchFromComposeFilename rewrites the compose filename template to drop
// the $Architecture segment:
//
//	$script:ComposeFile = "docker-compose.$Mode.$Architecture.$ExternalMode.yml"
//	→ $script:ComposeFile = "docker-compose.$Mode.$ExternalMode.yml"
func stripArchFromComposeFilename(content string) string {
	return strings.Replace(content,
		`"docker-compose.$Mode.$Architecture.$ExternalMode.yml"`,
		`"docker-compose.$Mode.$ExternalMode.yml"`,
		1)
}

// pruneAllSystemConfig deletes the removeArch sub-block from $AllSystemConfig
// and flattens the keepArch sub-block (strips the `"<keepArch>" = @{ ... }`
// wrapper and dedents the inner content by 4 spaces).
//
// Relies on shop's consistent indentation: each arch block opens with
// `    "<arch>" = @{` at 4-space indent and closes with exactly `    }` at
// the same indent. A blank line separates the two arch blocks.
func pruneAllSystemConfig(content, keepArch, removeArch string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))

	removeOpen := `    "` + removeArch + `" = @{`
	keepOpen := `    "` + keepArch + `" = @{`
	archClose := "    }"

	i := 0
	for i < len(lines) {
		line := lines[i]

		if line == removeOpen {
			// Skip this line and everything up to and including the matching close.
			j := i + 1
			for j < len(lines) && lines[j] != archClose {
				j++
			}
			// j is now the close line; skip it too.
			j++
			// Also consume a single trailing blank line if present.
			if j < len(lines) && strings.TrimSpace(lines[j]) == "" {
				j++
			}
			i = j
			continue
		}

		if line == keepOpen {
			// Skip the opening wrapper line, dedent inner content, skip the closer.
			j := i + 1
			for j < len(lines) && lines[j] != archClose {
				out = append(out, dedentPS(lines[j]))
				j++
			}
			// Skip the closer.
			i = j + 1
			continue
		}

		out = append(out, line)
		i++
	}
	return strings.Join(out, "\n")
}

// dedentPS removes 4 leading spaces from a PowerShell line. If the line is
// shorter than 4 leading spaces (blank or barely indented), returns it as-is.
func dedentPS(line string) string {
	if strings.HasPrefix(line, "    ") {
		return line[4:]
	}
	return line
}

// stripLangArchFromContainerNames rewrites ContainerName values in the
// (now-flattened) $AllSystemConfig:
//
//	ContainerName = "shop-<testLang>-<keepArch>-<mode>"
//	→ ContainerName = "shop-<mode>"
func stripLangArchFromContainerNames(content, keepArch, testLang string) string {
	for _, mode := range []string{"real", "stub"} {
		old := `ContainerName = "shop-` + testLang + `-` + keepArch + `-` + mode + `"`
		new := `ContainerName = "shop-` + mode + `"`
		content = strings.ReplaceAll(content, old, new)
	}
	return content
}

// stripArchFromSystemConfigLookup rewrites the $SystemConfig lookup to no
// longer index by $Architecture (which no longer exists as a variable):
//
//	$SystemConfig = $AllSystemConfig[$Architecture]
//	→ $SystemConfig = $AllSystemConfig
func stripArchFromSystemConfigLookup(content string) string {
	return strings.Replace(content,
		"$SystemConfig = $AllSystemConfig[$Architecture]",
		"$SystemConfig = $AllSystemConfig",
		1)
}
