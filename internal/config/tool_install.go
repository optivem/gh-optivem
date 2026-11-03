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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/optivem/gh-optivem/internal/kernel/log"
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

// installOutcome is what ensure did about one tool.
type installOutcome int

const (
	// outcomeAlreadyInstalled: the probe passed, nothing was touched.
	outcomeAlreadyInstalled installOutcome = iota
	// outcomeInstalled: setup installed it and the re-probe passed.
	outcomeInstalled
	// outcomeManual: setup does not automate this tool on this platform; the
	// caller prints the manual-install pointer. Not a failure: `verify`
	// reports the true state afterwards.
	outcomeManual
)

// installer carries the seams setup runs through, so the install flow can be
// exercised without a package manager.
type installer struct {
	// goos selects the backend. Only "windows" (winget) is automated; every
	// other value yields outcomeManual.
	goos string
	// lookPath resolves a command on the current process PATH.
	lookPath func(file string) (string, error)
	// run executes a command and returns its combined output.
	run func(name string, args ...string) (string, error)
	// refreshPath re-reads the persisted PATH into this process.
	refreshPath func() error
	// persistPath adds dir to the user's persisted PATH, idempotently, and
	// reports whether it changed anything.
	persistPath func(dir string) (added bool, err error)
	// say prints one line of progress.
	say func(format string, args ...any)
}

func execRun(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

// probe reports whether the tool runs: its command resolves on PATH and its
// version command exits 0. Asked of the tool itself rather than of
// `command -v`, which is true of a half-finished install or a shim whose
// target has gone.
func (r *installer) probe(t installTool) error {
	if _, err := r.lookPath(t.probe[0]); err != nil {
		return fmt.Errorf("%s not found on PATH", t.probe[0])
	}
	if out, err := r.run(t.probe[0], t.probe[1:]...); err != nil {
		return fmt.Errorf("'%s' failed: %w\n%s", strings.Join(t.probe, " "), err, strings.TrimSpace(out))
	}
	return nil
}

// packageInstalled asks winget's package database whether id is installed.
// A PATH-free answer, which is the point: `winget list --id <id> -e` exits 0
// when installed and non-zero otherwise, whatever this process's PATH holds.
func (r *installer) packageInstalled(id string) bool {
	_, err := r.run("winget", "list", "--id", id, "-e")
	return err == nil
}

// ensure makes t runnable, or says why it could not.
//
// Windows flow, each step carrying the reasoning of a real clean-room failure:
//
//  1. Probe first; a runnable tool is left alone.
//  2. If not on PATH but winget's database lists it, the usual cause is a PATH
//     that predates the install: refresh and ask again. Installing a package
//     that is already present cannot succeed (winget answers "no upgrade
//     applicable"), so a naive install-then-check reports a false failure.
//  3. Install. winget's exit status is advisory only: several non-zero codes
//     leave the package installed, so the re-probe decides, and the status is
//     kept for the error message so a real failure names its cause.
//  4. Refresh PATH (a fresh install stays "command not found" in the process
//     that ran it) and re-probe. Success is asserted by a tool that runs,
//     never inferred from an exit code or a database row.
func (r *installer) ensure(t installTool) (installOutcome, error) {
	if r.goos != "windows" {
		if r.probe(t) == nil {
			return outcomeAlreadyInstalled, nil
		}
		return outcomeManual, nil
	}
	if r.probe(t) == nil {
		return outcomeAlreadyInstalled, nil
	}
	// Installed earlier into a directory the installer never put on PATH
	// (Claude Code's ~/.local/bin, Go's GOPATH/bin): fix PATH rather than
	// reinstalling. A failure to resolve the directory is not an error here -
	// the tool may simply be absent, and the install below follows.
	if dir, derr := r.installDir(t); derr == nil && dirExists(dir) {
		if err := r.ensurePath(dir); err != nil {
			return 0, fmt.Errorf("%s: %w", t.label, err)
		}
		if r.probe(t) == nil {
			return outcomeAlreadyInstalled, nil
		}
	}
	if t.kind == installWinget && r.packageInstalled(t.winget) {
		r.say("%s: in the winget package database (%s) but not runnable from this shell - refreshing PATH and asking again.", t.label, t.winget)
		if err := r.refreshPath(); err != nil {
			return 0, fmt.Errorf("%s: could not refresh PATH: %w", t.label, err)
		}
		if r.probe(t) == nil {
			return outcomeAlreadyInstalled, nil
		}
		r.say("%s (%s) still does not run after the PATH refresh - installing over it.", t.label, t.winget)
	}

	r.say("%s: installing ...", t.label)
	var out string
	var runErr error
	switch t.kind {
	case installWinget:
		// Without the --accept-* flags winget stops on a licence prompt the
		// first time it sees a source or package - on a fresh machine, always.
		out, runErr = r.run("winget", "install", "--id", t.winget, "-e", "--source", "winget",
			"--accept-source-agreements", "--accept-package-agreements")
	case installPowerShell:
		out, runErr = r.run("powershell.exe", "-NoProfile", "-Command", t.script)
	case installGoInstall:
		out, runErr = r.run("go", "install", t.goPackage)
	}
	if runErr != nil {
		r.say("%s: install command exited with an error (%v); the re-probe decides.", t.label, runErr)
	}
	if err := r.refreshPath(); err != nil {
		return 0, fmt.Errorf("%s: could not refresh PATH after install: %w", t.label, err)
	}
	// Declined-PATH installers: the directory exists now, so put it on PATH
	// before the re-probe. A failure to resolve it is only fatal if the
	// re-probe fails too (it then names both causes).
	var pathErr error
	if dir, derr := r.installDir(t); derr != nil {
		if !errors.Is(derr, errNoPathDir) {
			pathErr = derr
		}
	} else if err := r.ensurePath(dir); err != nil {
		pathErr = err
	}
	if perr := r.probe(t); perr != nil {
		if pathErr != nil {
			perr = fmt.Errorf("%v; PATH fix also failed: %v", perr, pathErr)
		}
		return 0, fmt.Errorf("%s is still not runnable after the install (%v; install exit: %v).\n    Install output:\n%s\n    Install by hand: %s",
			t.label, perr, runErr, strings.TrimSpace(out), t.docURL)
	}
	return outcomeInstalled, nil
}

// newInstaller wires the real seams.
func newInstaller(say func(format string, args ...any)) *installer {
	return &installer{
		goos:        runtime.GOOS,
		lookPath:    exec.LookPath,
		run:         execRun,
		refreshPath: refreshProcessPath,
		persistPath: persistUserPath,
		say:         say,
	}
}

// mergePath appends to current every entry of persisted that current does not
// already hold. Comparison ignores a trailing separator and, when fold is
// set, case (Windows paths). The order of current is preserved, so nothing
// already resolving can be shadowed by the merge.
func mergePath(current, persisted, sep string, fold bool) string {
	norm := func(p string) string {
		p = strings.TrimRight(strings.TrimSpace(p), `\/`)
		if fold {
			p = strings.ToLower(p)
		}
		return p
	}
	have := map[string]bool{}
	for _, e := range strings.Split(current, sep) {
		if e = norm(e); e != "" {
			have[e] = true
		}
	}
	merged := current
	for _, e := range strings.Split(persisted, sep) {
		n := norm(e)
		if n == "" || have[n] {
			continue
		}
		have[n] = true
		if merged != "" && !strings.HasSuffix(merged, sep) {
			merged += sep
		}
		merged += strings.TrimSpace(e)
	}
	return merged
}

// refreshProcessPath re-reads the machine and user PATH from the registry
// (installers write there; this process keeps the copy it started with) and
// merges any new entries into the process PATH. An empty read is an error:
// merging nothing is indistinguishable from a successful no-op, and the
// later lookups would then fail for the wrong reason.
func refreshProcessPath() error {
	out, err := execRun("powershell.exe", "-NoProfile", "-Command",
		`[Environment]::ExpandEnvironmentVariables([Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [Environment]::GetEnvironmentVariable("Path","User"))`)
	if err != nil {
		return fmt.Errorf("reading PATH back from the registry: %w\n%s", err, strings.TrimSpace(out))
	}
	persisted := strings.TrimSpace(out)
	if persisted == "" || persisted == ";" {
		return fmt.Errorf("PATH read back from the registry was empty")
	}
	return os.Setenv("PATH", mergePath(os.Getenv("PATH"), persisted, ";", true))
}

// installDir returns the declined-PATH directory for t, or ("", nil) when the
// tool's installer handles PATH itself.
func (r *installer) installDir(t installTool) (string, error) {
	if t.pathDir == nil {
		return "", errNoPathDir
	}
	return t.pathDir(r)
}

var errNoPathDir = errors.New("tool has no declined-PATH directory")

func dirExists(dir string) bool {
	st, err := os.Stat(dir)
	return err == nil && st.IsDir()
}

// ensurePath makes dir resolve in this run and in every future shell, and says
// exactly what it did and where. Only dir is ever named: the rest of the PATH
// value is neither read back to the user nor logged.
func (r *installer) ensurePath(dir string) error {
	added, err := r.persistPath(dir)
	if err != nil {
		return fmt.Errorf("could not add %s to the user PATH: %w", dir, err)
	}
	if added {
		r.say("PATH: added %s to the user PATH (HKCU\\Environment, value Path).", dir)
	} else {
		r.say("PATH: %s is already on the user PATH.", dir)
	}
	return os.Setenv("PATH", mergePath(os.Getenv("PATH"), dir, ";", true))
}

// persistUserPath appends dir to the user PATH in HKCU\Environment unless an
// equivalent entry (case-insensitive, trailing backslash ignored) is already
// there. Only the user scope is written: no elevation, and machine PATH is
// not ours to edit. Note [Environment]::SetEnvironmentVariable stores the
// value as a plain string, so a %VAR% reference in the user PATH is kept as
// written but no longer re-expanded by the registry type.
func persistUserPath(dir string) (bool, error) {
	quoted := strings.ReplaceAll(dir, "'", "''")
	script := `$d='` + quoted + `'; ` +
		`$cur=[Environment]::GetEnvironmentVariable('Path','User'); if ($null -eq $cur) { $cur='' }; ` +
		`$has=$false; foreach ($e in $cur -split ';') { if ($e.Trim().TrimEnd('\') -ieq $d.TrimEnd('\')) { $has=$true } }; ` +
		`if ($has) { 'present' } else { ` +
		`[Environment]::SetEnvironmentVariable('Path', ($cur.TrimEnd(';') + ';' + $d).TrimStart(';'), 'User'); 'added' }`
	out, err := execRun("powershell.exe", "-NoProfile", "-Command", script)
	if err != nil {
		return false, fmt.Errorf("%w\n%s", err, strings.TrimSpace(out))
	}
	switch strings.TrimSpace(out) {
	case "added":
		return true, nil
	case "present":
		return false, nil
	}
	return false, fmt.Errorf("unexpected output from the PATH update: %q", strings.TrimSpace(out))
}

// SetupEnvironment is `gh optivem environment setup`: install every tier-1 and
// tier-2 tool that is missing, fix the PATH entries the upstream installers
// decline to write, then run the read-only VerifyEnvironment to report the
// true remaining state, and finish with what only a human can do.
//
// The returned error covers installs that were attempted and failed, or whose
// prerequisite failed. Items that are merely outstanding for the human
// (Docker, sign-in, credentials) are reported, not returned: they are the
// expected end state, not a malfunction of setup.
func SetupEnvironment(langs []string) error {
	r := newInstaller(func(format string, args ...any) { log.Infof(format, args...) })
	return setupEnvironment(r, langs, VerifyEnvironment)
}

// setupEnvironment is the testable form: the installer and the closing verify
// are supplied by the caller.
func setupEnvironment(r *installer, langs []string, verify func([]string) error) error {
	skipClaude, err := claudeCheckSkipped()
	if err != nil {
		return err
	}

	log.Section("Installing:")
	var failures []string
	var manual []installTool
	failed := map[string]bool{}
	for _, t := range installTable(langs) {
		if t.label == "Claude Code" && skipClaude {
			log.Info("Skipping Claude Code (" + skipClaudeCheckEnv + " set).")
			continue
		}
		if t.needs != "" && failed[t.needs] {
			failed[t.label] = true
			failures = append(failures, fmt.Sprintf("%s: not attempted - it needs %s, which could not be installed.", t.label, t.needs))
			log.Errorf("  %s: skipped (needs %s)", t.label, t.needs)
			continue
		}
		outcome, err := r.ensure(t)
		switch {
		case err != nil:
			failed[t.label] = true
			failures = append(failures, err.Error())
			log.Errorf("  %s: FAILED", t.label)
		case outcome == outcomeAlreadyInstalled:
			log.Successf("  %s: already installed", t.label)
		case outcome == outcomeInstalled:
			log.Successf("  %s: installed", t.label)
		case outcome == outcomeManual:
			manual = append(manual, t)
			log.Warnf("  %s: not automated on %s", t.label, r.goos)
		}
	}

	log.Section("Checking the result:")
	verifyErr := verify(langs)

	printOutstanding(r, manual, skipClaude, verifyErr)

	if len(failures) > 0 {
		return fmt.Errorf("%d install(s) failed:\n  %s", len(failures), strings.Join(failures, "\n  "))
	}
	return nil
}

// printOutstanding prints only what is genuinely left for the human.
func printOutstanding(r *installer, manual []installTool, skipClaude bool, verifyErr error) {
	log.Section("What is left for you:")
	left := false
	item := func(format string, args ...any) {
		left = true
		log.Infof("  - "+format, args...)
	}
	for _, t := range manual {
		hint := t.docURL
		if t.unixHint != "" {
			hint = t.unixHint
		}
		item("Install %s (not automated on %s): %s", t.label, r.goos, hint)
	}
	if verifyDockerInstalled() != nil || verifyDockerRunning() != nil {
		item("Docker: install and start Docker Desktop - setup never installs it (licence acceptance, WSL2, reboot): %s", "https://www.docker.com/products/docker-desktop")
	}
	var scopes []scopeStatus
	var scopeErr error
	if verifyGhInstalled() == nil {
		if verifyGhAuth(&scopes, &scopeErr) != nil {
			item("GitHub: run `gh auth login -s project,workflow`")
		} else if scopeErr != nil {
			item("GitHub: %s", strings.ReplaceAll(scopeErr.Error(), "\n    ", " "))
		}
	}
	if !skipClaude {
		item("Claude Code sign-in: run `claude` once and sign in. Setup cannot tell whether you already have - `claude` has no non-interactive sign-in check.")
	}
	if missing := MissingRequiredEnvVars(); len(missing) > 0 {
		item("Set credentials (see docs/setup.md#environment-variables): %s", strings.Join(missing, ", "))
	}
	if !left {
		log.Success("  nothing")
	}
	if verifyErr == nil {
		log.Success("`gh optivem environment verify` passes.")
	} else {
		log.Info("Re-run `gh optivem environment verify` once the items above are done.")
	}
}
