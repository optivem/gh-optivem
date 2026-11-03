// Package config — install table for `environment setup`.
//
// Every local tool `environment verify` can find missing, and how to put it
// there. The install strings live here, next to the checks in tool_checks.go,
// and the checks' failure messages are built from the same constants, so the
// two cannot drift: the hint `verify` prints is the command `setup` runs.
//
// Tiers bound what setup attempts:
//
//  1. auto-installable, unattended — Go, actionlint, Git Bash, Node, Java, .NET
//  2. installable, a human finishes — gh (then `gh auth login`), Claude Code
//     (then run `claude` once and sign in)
//  3. human-only — Docker Desktop and the credentials. Not in this table:
//     setup detects and instructs, it never installs them.
package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// Install hints shared with the failure messages in tool_checks.go.
const (
	ghInstallURL         = "https://cli.github.com/"
	gitBashInstallURL    = "https://git-scm.com/download/win"
	actionlintGoPackage  = "github.com/rhysd/actionlint/cmd/actionlint@v1"
	claudeInstallURL     = "https://claude.com/claude-code"
	claudeWindowsInstall = "irm https://claude.ai/install.ps1 | iex"
	claudeUnixInstall    = "curl -fsSL https://claude.ai/install.sh | bash"
	goInstallURL         = "https://go.dev/dl/"
	npmInstallURL        = "https://nodejs.org/"
	dotnetInstallURL     = "https://dotnet.microsoft.com/download"
	javaInstallURL       = "https://adoptium.net/"
)

// installTier is how far setup can take a tool; see the package comment.
type installTier int

const (
	tierAuto          installTier = 1
	tierHumanFinishes installTier = 2
)

// installKind is the mechanism that installs a tool on Windows.
type installKind int

const (
	// installWinget: `winget install --id <winget> -e`.
	installWinget installKind = iota
	// installPowerShell: run the PowerShell command in installTool.script.
	installPowerShell
	// installGoInstall: `go install <goPackage>`; needs Go runnable first.
	installGoInstall
)

// installTool is one row of the table.
type installTool struct {
	label string
	// probe is the command that proves the tool runs (not merely that a file
	// of that name sits on PATH): probe[0] must resolve on PATH and the whole
	// command must exit 0.
	probe []string
	tier  installTier
	kind  installKind

	winget    string // winget package id (installWinget)
	script    string // PowerShell command (installPowerShell)
	goPackage string // package@version (installGoInstall)

	// docURL is where to install it by hand: the fallback on platforms setup
	// does not automate, and the pointer in a failure message.
	docURL string
	// unixHint overrides docURL as the manual-install pointer off Windows
	// when there is a one-line command to give instead of a page.
	unixHint string

	// pathDir returns a directory the install leaves the tool in that the
	// installer itself declines to add to PATH. Nil when the installer
	// handles PATH. goBin is the runner for `go env GOPATH`.
	pathDir func(r *installer) (string, error)

	// needs is the label of a tool that must run first (actionlint needs Go).
	needs string
}

// claudeBinDir is where claude.ai/install.ps1 drops claude.exe without
// touching PATH. Built from the user profile rather than $HOME because the
// installer installs relative to the Windows profile.
func claudeBinDir(*installer) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin"), nil
}

// goBinDir is $(go env GOPATH)/bin, where `go install` puts binaries and the
// Go installer does not add to PATH.
func goBinDir(r *installer) (string, error) {
	out, err := r.run("go", "env", "GOPATH")
	if err != nil {
		return "", err
	}
	gopath := strings.TrimSpace(out)
	if gopath == "" {
		return "", os.ErrNotExist
	}
	return filepath.Join(gopath, "bin"), nil
}

// installTable returns every installable tool, tier 1 and 2, in dependency
// order: Go before actionlint. Language toolchains are included only for the
// requested langs (deduped, unknown values skipped — same contract as
// compilerChecksFor).
func installTable(langs []string) []installTool {
	tools := []installTool{
		{label: "GitHub CLI", probe: []string{"gh", "--version"}, tier: tierHumanFinishes,
			kind: installWinget, winget: "GitHub.cli", docURL: ghInstallURL},
		{label: "Git Bash", probe: []string{"bash", "--version"}, tier: tierAuto,
			kind: installWinget, winget: "Git.Git", docURL: gitBashInstallURL},
		{label: "Go", probe: []string{"go", "version"}, tier: tierAuto,
			kind: installWinget, winget: "GoLang.Go", docURL: goInstallURL, pathDir: goBinDir},
		{label: "actionlint", probe: []string{"actionlint", "--version"}, tier: tierAuto,
			kind: installGoInstall, goPackage: actionlintGoPackage,
			docURL: "https://github.com/rhysd/actionlint", pathDir: goBinDir, needs: "Go"},
		{label: "Claude Code", probe: []string{"claude", "--version"}, tier: tierHumanFinishes,
			kind: installPowerShell, script: claudeWindowsInstall,
			docURL: claudeInstallURL, unixHint: claudeUnixInstall, pathDir: claudeBinDir},
	}
	seen := map[string]bool{}
	for _, l := range langs {
		if seen[l] {
			continue
		}
		seen[l] = true
		switch l {
		case projectconfig.LangTypescript:
			tools = append(tools, installTool{label: "Node.js", probe: []string{"npm", "--version"}, tier: tierAuto,
				kind: installWinget, winget: "OpenJS.NodeJS.22", docURL: npmInstallURL})
		case projectconfig.LangDotnet:
			tools = append(tools, installTool{label: ".NET SDK", probe: []string{"dotnet", "--version"}, tier: tierAuto,
				kind: installWinget, winget: "Microsoft.DotNet.SDK.8", docURL: dotnetInstallURL})
		case projectconfig.LangJava:
			tools = append(tools, installTool{label: "Java JDK", probe: []string{"java", "-version"}, tier: tierAuto,
				kind: installWinget, winget: "EclipseAdoptium.Temurin.21.JDK", docURL: javaInstallURL})
		}
	}
	return tools
}

// installer carries the seams setup runs through, so the install flow can be
// exercised without a package manager. Fields are added alongside the code
// that needs them.
type installer struct {
	// run executes a command and returns its combined output.
	run func(name string, args ...string) (string, error)
}
