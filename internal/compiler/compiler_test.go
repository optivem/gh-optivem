package compiler

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// fakeShell records every Run invocation and returns canned errors keyed
// by call index. Mirrors the fake-shell pattern used in
// internal/atdd/runtime/actions/bindings_test.go (fakeShell), tightened to
// also capture cwd because Compile dispatches into per-tier directories.
type fakeShell struct {
	calls []shellCall
	// errs[i] is returned for the (i+1)th call; len(errs) < len(calls)
	// implies "no error from that index onward".
	errs []error
}

type shellCall struct {
	cmd string
	cwd string
}

func (f *fakeShell) Run(cmd, cwd string) error {
	idx := len(f.calls)
	f.calls = append(f.calls, shellCall{cmd: cmd, cwd: cwd})
	if idx < len(f.errs) {
		return f.errs[idx]
	}
	return nil
}

func TestCompile_DispatchesPerLanguageCommandSequences(t *testing.T) {
	cases := []struct {
		name string
		lang string
		want []string
	}{
		{"dotnet runs a single dotnet build", projectconfig.LangDotnet, []string{"dotnet build"}},
		{"java runs gradlew compileJava only (no compileTestJava in the structural sweep)", projectconfig.LangJava, []string{`.\gradlew.bat compileJava`}},
		{"typescript runs npm ci before npx tsc to fix the `not the tsc command` failure", projectconfig.LangTypescript, []string{"npm ci", "npx tsc --noEmit"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sh := &fakeShell{}
			tier := projectconfig.TierSpec{Path: "system", Lang: tc.lang}
			if err := CompileWith(tier, ".", sh); err != nil {
				t.Fatalf("CompileWith returned error: %v", err)
			}
			gotCmds := make([]string, len(sh.calls))
			for i, c := range sh.calls {
				gotCmds[i] = c.cmd
			}
			if !reflect.DeepEqual(gotCmds, tc.want) {
				t.Fatalf("commands mismatch:\n got:  %v\n want: %v", gotCmds, tc.want)
			}
		})
	}
}

func TestCompile_RunsCommandsInTierCwd(t *testing.T) {
	sh := &fakeShell{}
	tier := projectconfig.TierSpec{Path: "backend", Lang: projectconfig.LangDotnet}
	if err := CompileWith(tier, "/repo", sh); err != nil {
		t.Fatalf("CompileWith returned error: %v", err)
	}
	if len(sh.calls) != 1 {
		t.Fatalf("expected 1 shell call, got %d", len(sh.calls))
	}
	want := filepath.Join("/repo", "backend")
	if sh.calls[0].cwd != want {
		t.Fatalf("cwd mismatch:\n got:  %q\n want: %q", sh.calls[0].cwd, want)
	}
}

func TestCompile_HaltsOnFirstNonZeroExit(t *testing.T) {
	// Typescript runs two commands; the first failure must abort before
	// the second runs (matches today's compile-all.sh — first error wins).
	boom := errors.New("npm install failed")
	sh := &fakeShell{errs: []error{boom}}
	tier := projectconfig.TierSpec{Path: "frontend", Lang: projectconfig.LangTypescript}
	err := CompileWith(tier, ".", sh)
	if err == nil {
		t.Fatal("expected error from first command, got nil")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("error chain missing underlying cause: %v", err)
	}
	if len(sh.calls) != 1 {
		t.Fatalf("expected only the first command to run, got %d calls: %v", len(sh.calls), sh.calls)
	}
}

func TestCompile_RejectsUnsupportedLang(t *testing.T) {
	sh := &fakeShell{}
	tier := projectconfig.TierSpec{Path: "system", Lang: "rust"}
	err := CompileWith(tier, ".", sh)
	if err == nil {
		t.Fatal("expected unsupported-lang error, got nil")
	}
	if len(sh.calls) != 0 {
		t.Fatalf("expected no shell calls for unsupported lang, got %v", sh.calls)
	}
}
