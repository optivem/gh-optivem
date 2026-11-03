package main

import (
	"strings"
	"testing"
)

func TestEnvironmentSetupCmd_Wiring(t *testing.T) {
	env := newEnvironmentCmd()
	setup, _, err := env.Find([]string{"setup"})
	if err != nil || setup.Name() != "setup" {
		t.Fatalf("environment has no setup subcommand: %v", err)
	}
	verify, _, err := env.Find([]string{"verify"})
	if err != nil || verify.Name() != "verify" {
		t.Fatalf("environment has no verify subcommand: %v", err)
	}
}

// setup and verify must parse --lang identically, or a value one accepts the
// other rejects.
func TestEnvironmentSetupCmd_LangFlagMatchesVerify(t *testing.T) {
	setupFlag := newEnvironmentSetupCmd().Flags().Lookup("lang")
	verifyFlag := newEnvironmentVerifyCmd().Flags().Lookup("lang")
	if setupFlag == nil || verifyFlag == nil {
		t.Fatal("both commands must define --lang")
	}
	if setupFlag.Value.Type() != verifyFlag.Value.Type() {
		t.Errorf("--lang type differs: setup %s, verify %s", setupFlag.Value.Type(), verifyFlag.Value.Type())
	}
	if setupFlag.DefValue != verifyFlag.DefValue || setupFlag.Usage != verifyFlag.Usage {
		t.Errorf("--lang default/usage differ: setup (%q, %q), verify (%q, %q)",
			setupFlag.DefValue, setupFlag.Usage, verifyFlag.DefValue, verifyFlag.Usage)
	}
	for _, v := range []string{"java,dotnet", "typescript"} {
		if err := newEnvironmentSetupCmd().Flags().Set("lang", v); err != nil {
			t.Errorf("setup rejects --lang %s: %v", v, err)
		}
	}
}

func TestEnvironmentHelp_StatesWhichCommandWrites(t *testing.T) {
	if got := newEnvironmentVerifyCmd().Long; !strings.Contains(got, "only reads") {
		t.Error("verify --help must say it only reads")
	}
	if got := newEnvironmentSetupCmd().Long; !strings.Contains(got, "WRITES") {
		t.Error("setup --help must say it writes")
	}
}
