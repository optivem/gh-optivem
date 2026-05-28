package approval

import (
	"bytes"
	"strings"
	"testing"
)

// envOf returns a stub env function backed by a map. Unset keys return "" —
// matching the production os.Getenv contract.
func envOf(pairs map[string]string) func(string) string {
	return func(k string) string { return pairs[k] }
}

func TestCategory_String(t *testing.T) {
	cases := map[Category]string{
		CategoryCommand:    "command",
		CategoryProdAgent:  "prod-agent",
		CategoryTestAgent:  "test-agent",
		CategoryProdCommit: "prod-commit",
		CategoryTestCommit: "test-commit",
		CategoryHuman:      "human",
	}
	for c, want := range cases {
		if got := c.String(); got != want {
			t.Errorf("Category(%d).String() = %q, want %q", c, got, want)
		}
	}
}

func TestParseCategory_RoundTrip(t *testing.T) {
	for _, c := range []Category{CategoryCommand, CategoryProdAgent, CategoryTestAgent, CategoryProdCommit, CategoryTestCommit, CategoryHuman} {
		got, err := ParseCategory(c.String())
		if err != nil {
			t.Errorf("ParseCategory(%q) err: %v", c.String(), err)
		}
		if got != c {
			t.Errorf("ParseCategory(%q) = %v, want %v", c.String(), got, c)
		}
	}
}

func TestParseCategory_CaseInsensitiveAndTrimmed(t *testing.T) {
	for _, s := range []string{"COMMAND", "Prod-Agent", "  test-agent  ", "Human"} {
		if _, err := ParseCategory(s); err != nil {
			t.Errorf("ParseCategory(%q) err: %v", s, err)
		}
	}
}

func TestParseCategory_Invalid_ErrorListsValidSet(t *testing.T) {
	_, err := ParseCategory("garbage")
	if err == nil {
		t.Fatal("expected error for invalid category")
	}
	msg := err.Error()
	for _, want := range []string{"command", "prod-agent", "test-agent", "prod-commit", "test-commit", "human"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not list %q", msg, want)
		}
	}
}

// Regression guard: operators carrying muscle memory from the old vocabulary
// (commit / fix / release / prompt) get a clear error naming the new tokens
// rather than silent acceptance.
func TestParseCategory_OldVocabularyErrors(t *testing.T) {
	for _, old := range []string{"commit", "fix", "release", "prompt"} {
		_, err := ParseCategory(old)
		if err == nil {
			t.Errorf("ParseCategory(%q): expected error for retired token", old)
		}
	}
}

func TestResolve_DefaultAllOff(t *testing.T) {
	r, err := Resolve(false, false, "", false, envOf(nil))
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if r.Auto {
		t.Error("Auto should be false by default")
	}
	if r.AutoSource != "default" {
		t.Errorf("AutoSource = %q, want default", r.AutoSource)
	}
	if r.ConfirmSource != "default" {
		t.Errorf("ConfirmSource = %q, want default", r.ConfirmSource)
	}
	// When Auto is off the floor is unused functionally; the zero value
	// (CategoryCommand) is fine and never short-circuits.
	if r.ConfirmFloor != CategoryCommand {
		t.Errorf("ConfirmFloor = %v, want zero (CategoryCommand)", r.ConfirmFloor)
	}
}

func TestResolve_AutoFlag_DefaultsToHuman(t *testing.T) {
	r, err := Resolve(true, true, "", false, envOf(nil))
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if !r.Auto {
		t.Fatal("Auto should be true")
	}
	if r.AutoSource != "flag" {
		t.Errorf("AutoSource = %q, want flag", r.AutoSource)
	}
	if r.ConfirmSource != "default" {
		t.Errorf("ConfirmSource = %q, want default", r.ConfirmSource)
	}
	if r.ConfirmFloor != CategoryHuman {
		t.Errorf("ConfirmFloor = %v, want CategoryHuman (truly autonomous default)", r.ConfirmFloor)
	}
}

func TestResolve_AutoEnv(t *testing.T) {
	r, err := Resolve(false, false, "", false, envOf(map[string]string{EnvAuto: "true"}))
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if !r.Auto {
		t.Error("Auto should be true from env")
	}
	if r.AutoSource != "env" {
		t.Errorf("AutoSource = %q, want env", r.AutoSource)
	}
}

func TestResolve_AutoFlagOverridesEnv(t *testing.T) {
	r, err := Resolve(false, true, "", false, envOf(map[string]string{EnvAuto: "true"}))
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if r.Auto {
		t.Error("flag --auto=false must override env GH_OPTIVEM_AUTO=true")
	}
	if r.AutoSource != "flag" {
		t.Errorf("AutoSource = %q, want flag", r.AutoSource)
	}
}

func TestResolve_AutoEnvFalsyDoesNotEnable(t *testing.T) {
	for _, v := range []string{"false", "no", "0", "bogus", ""} {
		r, err := Resolve(false, false, "", false, envOf(map[string]string{EnvAuto: v}))
		if err != nil {
			t.Fatalf("Resolve(env=%q) err: %v", v, err)
		}
		if r.Auto {
			t.Errorf("env=%q must not enable Auto", v)
		}
	}
}

func TestResolve_ConfirmFlag_ExplicitTier(t *testing.T) {
	r, err := Resolve(true, true, "prod-commit", true, envOf(nil))
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if r.ConfirmSource != "flag" {
		t.Errorf("ConfirmSource = %q, want flag", r.ConfirmSource)
	}
	if r.ConfirmFloor != CategoryProdCommit {
		t.Errorf("ConfirmFloor = %v, want CategoryProdCommit", r.ConfirmFloor)
	}
}

func TestResolve_ConfirmFlag_MultiTokenErrors(t *testing.T) {
	_, err := Resolve(true, true, "prod-commit,test-commit", true, envOf(nil))
	if err == nil {
		t.Fatal("expected error for multi-token --confirm (threshold is single tier)")
	}
	if !strings.Contains(err.Error(), "single tier") {
		t.Errorf("error should explain single-tier requirement: %v", err)
	}
}

func TestResolve_ConfirmEnv(t *testing.T) {
	r, err := Resolve(true, true, "", false, envOf(map[string]string{EnvConfirm: "test-agent"}))
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if r.ConfirmSource != "env" {
		t.Errorf("ConfirmSource = %q, want env", r.ConfirmSource)
	}
	if r.ConfirmFloor != CategoryTestAgent {
		t.Errorf("ConfirmFloor = %v, want CategoryTestAgent", r.ConfirmFloor)
	}
}

func TestResolve_ConfirmFlagOverridesEnv(t *testing.T) {
	r, err := Resolve(true, true, "command", true, envOf(map[string]string{EnvConfirm: "human"}))
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if r.ConfirmSource != "flag" {
		t.Errorf("ConfirmSource = %q, want flag", r.ConfirmSource)
	}
	if r.ConfirmFloor != CategoryCommand {
		t.Errorf("ConfirmFloor = %v, want CategoryCommand (flag wins)", r.ConfirmFloor)
	}
}

func TestResolve_InvalidCategory_Errors(t *testing.T) {
	_, err := Resolve(true, true, "garbage", true, envOf(nil))
	if err == nil {
		t.Fatal("expected error for invalid category in --confirm")
	}
	if !strings.Contains(err.Error(), "garbage") {
		t.Errorf("error should mention the offending token: %v", err)
	}
}

func TestResolve_AutoOff_FloorIsUnusedAtConfirmTime(t *testing.T) {
	// When Auto is false every site prompts regardless of floor.
	r, err := Resolve(false, false, "test-agent", true, envOf(nil))
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if r.Auto {
		t.Error("Auto should be false")
	}
	// Confirm always prompts when Auto is off — even below the explicit floor.
	ok, _ := Confirm(r, CategoryCommand, strings.NewReader("y\n"), &bytes.Buffer{}, "Go?")
	if !ok {
		t.Error("expected prompt to have been read")
	}
}

func TestConfirm_ShortCircuit_BelowFloor(t *testing.T) {
	// Floor = test-commit. Tiers below (command, prod-agent, test-agent,
	// prod-commit) all auto-yes; tiers at/above (test-commit, human) prompt.
	r, _ := Resolve(true, true, "test-commit", true, envOf(nil))
	for _, c := range []Category{CategoryCommand, CategoryProdAgent, CategoryTestAgent, CategoryProdCommit} {
		ok, err := Confirm(r, c, strings.NewReader(""), &bytes.Buffer{}, "Anything?")
		if err != nil {
			t.Fatalf("Confirm(%s) err: %v", c, err)
		}
		if !ok {
			t.Errorf("expected short-circuit true for %s below floor=test-commit", c)
		}
	}
}

func TestConfirm_Prompts_AtOrAboveFloor(t *testing.T) {
	// Floor = prod-commit. prod-commit / test-commit / human all prompt.
	r, _ := Resolve(true, true, "prod-commit", true, envOf(nil))
	for _, c := range []Category{CategoryProdCommit, CategoryTestCommit, CategoryHuman} {
		var out bytes.Buffer
		ok, err := Confirm(r, c, strings.NewReader("y\n"), &out, "Go?")
		if err != nil {
			t.Fatalf("Confirm(%s) err: %v", c, err)
		}
		if !ok {
			t.Errorf("expected y for %s at/above floor=prod-commit", c)
		}
		if !strings.Contains(out.String(), "Go?") {
			t.Errorf("%s should have written the prompt (no short-circuit at/above floor)", c)
		}
	}
}

func TestConfirm_HumanNeverShortCircuits(t *testing.T) {
	// Even with --confirm=command (lowest floor, most permissive auto), human-
	// tier sites must still prompt. This is the load-bearing invariant of the
	// design.
	r, _ := Resolve(true, true, "command", true, envOf(nil))
	var out bytes.Buffer
	ok, err := Confirm(r, CategoryHuman, strings.NewReader("y\n"), &out, "Human STOP?")
	if err != nil {
		t.Fatalf("Confirm err: %v", err)
	}
	if !ok {
		t.Error("expected y to be read")
	}
	if !strings.Contains(out.String(), "Human STOP?") {
		t.Error("human prompt should have been written — short-circuit must not have fired")
	}
}

func TestConfirm_DoesNotShortCircuitWhenAutoOff(t *testing.T) {
	r, _ := Resolve(false, false, "", false, envOf(nil))
	var out bytes.Buffer
	ok, err := Confirm(r, CategoryCommand, strings.NewReader("n\n"), &out, "Anything?")
	if err != nil {
		t.Fatalf("Confirm err: %v", err)
	}
	if ok {
		t.Error("expected false from n input")
	}
	if !strings.Contains(out.String(), "Anything?") {
		t.Error("prompt should be written when Auto is off")
	}
}

// stubAsker is a minimal promptio.Asker for ConfirmVia testing.
type stubAsker struct {
	answers []string
	calls   int
}

func (s *stubAsker) Ask(prompt string) (string, error) {
	if s.calls >= len(s.answers) {
		return "", nil
	}
	out := s.answers[s.calls]
	s.calls++
	return out, nil
}

func TestConfirmVia_ShortCircuitDoesNotCallAsker(t *testing.T) {
	r, _ := Resolve(true, true, "human", true, envOf(nil))
	asker := &stubAsker{answers: []string{"n"}}
	ok, err := ConfirmVia(r, CategoryCommand, asker, &bytes.Buffer{}, "Anything?")
	if err != nil {
		t.Fatalf("ConfirmVia err: %v", err)
	}
	if !ok {
		t.Error("expected short-circuit true")
	}
	if asker.calls != 0 {
		t.Errorf("expected asker not to be called, got %d call(s)", asker.calls)
	}
}

func TestConfirmVia_PromptsAtOrAboveFloor(t *testing.T) {
	r, _ := Resolve(true, true, "human", true, envOf(nil))
	asker := &stubAsker{answers: []string{"y"}}
	ok, err := ConfirmVia(r, CategoryHuman, asker, &bytes.Buffer{}, "STOP?")
	if err != nil {
		t.Fatalf("ConfirmVia err: %v", err)
	}
	if !ok {
		t.Error("expected true from asker y")
	}
	if asker.calls != 1 {
		t.Errorf("expected exactly 1 asker call, got %d", asker.calls)
	}
}

func TestResolved_ConfirmFloorString(t *testing.T) {
	r, _ := Resolve(true, true, "prod-commit", true, envOf(nil))
	if got := r.ConfirmFloorString(); got != "prod-commit" {
		t.Errorf("ConfirmFloorString() = %q, want %q", got, "prod-commit")
	}
}
