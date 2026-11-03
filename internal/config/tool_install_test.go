package config

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

func TestInstallTable_EntriesAreComplete(t *testing.T) {
	all := installTable([]string{projectconfig.LangJava, projectconfig.LangDotnet, projectconfig.LangTypescript})
	labels := map[string]int{}
	for i, tool := range all {
		labels[tool.label] = i
		if tool.label == "" || len(tool.probe) == 0 || tool.docURL == "" {
			t.Errorf("%+v: needs a label, a probe and a docURL", tool)
		}
		if tool.tier != tierAuto && tool.tier != tierHumanFinishes {
			t.Errorf("%s: tier %d is not 1 or 2", tool.label, tool.tier)
		}
		switch tool.kind {
		case installWinget:
			if tool.winget == "" {
				t.Errorf("%s: winget kind without a winget id", tool.label)
			}
		case installPowerShell:
			if tool.script == "" {
				t.Errorf("%s: powershell kind without a script", tool.label)
			}
		case installGoInstall:
			if tool.goPackage == "" {
				t.Errorf("%s: go-install kind without a package", tool.label)
			}
		}
	}
	for _, want := range []string{"GitHub CLI", "Git Bash", "Go", "actionlint", "Claude Code", "Node.js", ".NET SDK", "Java JDK"} {
		if _, ok := labels[want]; !ok {
			t.Errorf("table is missing %q", want)
		}
	}
	// Go must precede actionlint, which is installed with it.
	if labels["Go"] > labels["actionlint"] {
		t.Error("Go must be installed before actionlint")
	}
	for _, tool := range all {
		if tool.needs != "" {
			if _, ok := labels[tool.needs]; !ok {
				t.Errorf("%s needs %q, which is not in the table", tool.label, tool.needs)
			}
		}
	}
}

func TestInstallTable_LangsAreGatedAndDeduped(t *testing.T) {
	base := len(installTable(nil))
	if got := len(installTable([]string{projectconfig.LangJava, projectconfig.LangJava, "cobol"})); got != base+1 {
		t.Errorf("java twice plus an unknown lang added %d tools, want 1", got-base)
	}
}

func TestInstallTable_HintsAgreeWithVerifyMessages(t *testing.T) {
	t.Setenv("PATH", "")
	err := verifyActionlint()
	if err == nil || !strings.Contains(err.Error(), actionlintGoPackage) {
		t.Errorf("actionlint failure %v should carry the install package", err)
	}
	if !strings.Contains(ghNotFoundErr().Error(), ghInstallURL) {
		t.Error("gh failure should carry ghInstallURL")
	}
}

// fakeMachine scripts an installer: the tool becomes visible once "installed",
// and every command run is recorded.
type fakeMachine struct {
	goos         string
	visible      bool // the probe resolves and runs
	inDB         bool // winget's package database lists it
	installOK    bool // the install makes it visible
	installErr   error
	refreshShows bool // refreshPath makes a listed tool visible
	calls        []string
	pathPersists []string
}

func (m *fakeMachine) installer() *installer {
	return &installer{
		goos: m.goos,
		lookPath: func(file string) (string, error) {
			if m.visible {
				return file, nil
			}
			return "", errors.New("not found")
		},
		run: func(name string, args ...string) (string, error) {
			m.calls = append(m.calls, name+" "+strings.Join(args, " "))
			switch {
			case name == "winget" && args[0] == "list":
				if m.inDB {
					return "", nil
				}
				return "", errors.New("exit status 20")
			case name == "winget" && args[0] == "install":
				if m.installOK {
					m.visible, m.inDB = true, true
				}
				return "install output", m.installErr
			}
			return "v1.0", nil
		},
		refreshPath: func() error {
			if m.refreshShows && m.inDB {
				m.visible = true
			}
			return nil
		},
		persistPath: func(dir string) (bool, error) {
			m.pathPersists = append(m.pathPersists, dir)
			return true, nil
		},
		say: func(string, ...any) {},
	}
}

func (m *fakeMachine) attemptedInstall() bool {
	for _, c := range m.calls {
		if strings.HasPrefix(c, "winget install") {
			return true
		}
	}
	return false
}

var wingetTool = installTool{label: "Thing", probe: []string{"thing", "--version"}, tier: tierAuto,
	kind: installWinget, winget: "Vendor.Thing", docURL: "https://example.test/thing"}

func TestEnsure_AlreadyInstalledIsANoOp(t *testing.T) {
	m := &fakeMachine{goos: "windows", visible: true}
	got, err := m.installer().ensure(wingetTool)
	if err != nil || got != outcomeAlreadyInstalled {
		t.Fatalf("got (%v, %v), want already-installed", got, err)
	}
	if m.attemptedInstall() || len(m.pathPersists) != 0 {
		t.Errorf("a runnable tool must not be touched; calls=%v path=%v", m.calls, m.pathPersists)
	}
}

func TestEnsure_NonZeroWingetExitWithPassingReprobeIsSuccess(t *testing.T) {
	m := &fakeMachine{goos: "windows", installOK: true, installErr: errors.New("exit status 0x8a150109")}
	got, err := m.installer().ensure(wingetTool)
	if err != nil || got != outcomeInstalled {
		t.Fatalf("got (%v, %v), want installed despite the advisory exit status", got, err)
	}
}

func TestEnsure_InstallThatLeavesToolUnrunnableFailsLoudly(t *testing.T) {
	m := &fakeMachine{goos: "windows", installErr: errors.New("exit status 1")}
	_, err := m.installer().ensure(wingetTool)
	if err == nil {
		t.Fatal("want an error when the tool is still not runnable")
	}
	for _, want := range []string{"Thing", "exit status 1", "install output", wingetTool.docURL} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

func TestEnsure_InPackageDBButStalePathRefreshesInsteadOfReinstalling(t *testing.T) {
	m := &fakeMachine{goos: "windows", inDB: true, refreshShows: true}
	got, err := m.installer().ensure(wingetTool)
	if err != nil || got != outcomeAlreadyInstalled {
		t.Fatalf("got (%v, %v), want already-installed after a PATH refresh", got, err)
	}
	if m.attemptedInstall() {
		t.Error("installing over a package that is already present cannot succeed; must not be attempted")
	}
}

func TestEnsure_OtherPlatformsAreManualNotAnError(t *testing.T) {
	m := &fakeMachine{goos: "linux"}
	got, err := m.installer().ensure(wingetTool)
	if err != nil || got != outcomeManual {
		t.Fatalf("got (%v, %v), want manual", got, err)
	}
	if m.attemptedInstall() {
		t.Errorf("nothing may be installed off Windows; calls=%v", m.calls)
	}
}

func TestEnsure_DeclinedPathDirIsPersistedWithoutReinstalling(t *testing.T) {
	dir := t.TempDir() // exists: the tool is installed, only PATH is missing
	tool := wingetTool
	tool.pathDir = func(*installer) (string, error) { return dir, nil }

	m := &fakeMachine{goos: "windows"}
	r := m.installer()
	r.lookPath = func(file string) (string, error) {
		if len(m.pathPersists) > 0 {
			return file, nil
		}
		return "", errors.New("not found")
	}
	t.Setenv("PATH", os.Getenv("PATH"))
	got, err := r.ensure(tool)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if got != outcomeAlreadyInstalled || m.attemptedInstall() {
		t.Errorf("got %v attemptedInstall=%v, want a PATH-only fix", got, m.attemptedInstall())
	}
	if len(m.pathPersists) != 1 || m.pathPersists[0] != dir {
		t.Errorf("persisted %v, want [%s]", m.pathPersists, dir)
	}
}

func TestMergePath(t *testing.T) {
	cases := []struct{ name, cur, add, want string }{
		{"appends new", `C:\a;C:\b`, `C:\c`, `C:\a;C:\b;C:\c`},
		{"case and trailing slash ignored", `C:\A\;C:\b`, `c:\a`, `C:\A\;C:\b`},
		{"empty current", ``, `C:\c`, `C:\c`},
		{"never reorders", `C:\b;C:\a`, `C:\a;C:\z`, `C:\b;C:\a;C:\z`},
		{"blank entries skipped", `C:\a`, `;;`, `C:\a`},
	}
	for _, c := range cases {
		if got := mergePath(c.cur, c.add, ";", true); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
