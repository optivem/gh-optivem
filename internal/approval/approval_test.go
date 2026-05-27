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
		CategoryCommit:  "commit",
		CategoryFix:     "fix",
		CategoryRelease: "release",
		CategoryPrompt:  "prompt",
		CategoryHuman:   "human",
	}
	for c, want := range cases {
		if got := c.String(); got != want {
			t.Errorf("Category(%d).String() = %q, want %q", c, got, want)
		}
	}
}

func TestParseCategory_RoundTrip(t *testing.T) {
	for _, c := range []Category{CategoryCommit, CategoryFix, CategoryRelease, CategoryPrompt, CategoryHuman} {
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
	for _, s := range []string{"COMMIT", "Commit", "  fix  ", "Release"} {
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
	for _, want := range []string{"commit", "fix", "release", "prompt", "human"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not list %q", msg, want)
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
	// Human invariant: always in ConfirmSet, even when Auto is off.
	if !r.ConfirmSet[CategoryHuman] {
		t.Error("CategoryHuman must always be in ConfirmSet")
	}
}

func TestResolve_AutoFlag_DefaultsToCommitFix(t *testing.T) {
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
	want := map[Category]bool{CategoryCommit: true, CategoryFix: true, CategoryHuman: true}
	for c, w := range want {
		if r.ConfirmSet[c] != w {
			t.Errorf("ConfirmSet[%s] = %v, want %v", c, r.ConfirmSet[c], w)
		}
	}
	if r.ConfirmSet[CategoryRelease] || r.ConfirmSet[CategoryPrompt] {
		t.Errorf("release/prompt should NOT be in default exclusion: %+v", r.ConfirmSet)
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

func TestResolve_ConfirmFlag_ExplicitEmpty(t *testing.T) {
	r, err := Resolve(true, true, "", true, envOf(nil))
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if r.ConfirmSource != "flag" {
		t.Errorf("ConfirmSource = %q, want flag", r.ConfirmSource)
	}
	// Explicit empty --confirm= means "no operator categories, only the
	// implicit human." This is the true-autonomous mode example.
	for _, c := range []Category{CategoryCommit, CategoryFix, CategoryRelease, CategoryPrompt} {
		if r.ConfirmSet[c] {
			t.Errorf("ConfirmSet[%s] should be false under --confirm=", c)
		}
	}
	if !r.ConfirmSet[CategoryHuman] {
		t.Error("human must remain implicit")
	}
}

func TestResolve_ConfirmFlag_Custom(t *testing.T) {
	r, err := Resolve(true, true, "fix,release", true, envOf(nil))
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	want := map[Category]bool{
		CategoryCommit:  false,
		CategoryFix:     true,
		CategoryRelease: true,
		CategoryPrompt:  false,
		CategoryHuman:   true,
	}
	for c, w := range want {
		if r.ConfirmSet[c] != w {
			t.Errorf("ConfirmSet[%s] = %v, want %v", c, r.ConfirmSet[c], w)
		}
	}
}

func TestResolve_ConfirmEnv(t *testing.T) {
	r, err := Resolve(true, true, "", false, envOf(map[string]string{EnvConfirm: "fix"}))
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if r.ConfirmSource != "env" {
		t.Errorf("ConfirmSource = %q, want env", r.ConfirmSource)
	}
	if !r.ConfirmSet[CategoryFix] {
		t.Error("fix should be in confirm set")
	}
	if r.ConfirmSet[CategoryCommit] {
		t.Error("commit should NOT be in confirm set when env narrows to just fix")
	}
}

func TestResolve_ConfirmFlagOverridesEnv(t *testing.T) {
	r, err := Resolve(true, true, "release", true, envOf(map[string]string{EnvConfirm: "fix"}))
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if r.ConfirmSource != "flag" {
		t.Errorf("ConfirmSource = %q, want flag", r.ConfirmSource)
	}
	if r.ConfirmSet[CategoryFix] {
		t.Error("env fix must not leak through when --confirm=release is set")
	}
	if !r.ConfirmSet[CategoryRelease] {
		t.Error("release should be in confirm set from flag")
	}
}

func TestResolve_InvalidCategory_Errors(t *testing.T) {
	_, err := Resolve(true, true, "commit,garbage,fix", true, envOf(nil))
	if err == nil {
		t.Fatal("expected error for invalid category in --confirm")
	}
	if !strings.Contains(err.Error(), "garbage") {
		t.Errorf("error should mention the offending token: %v", err)
	}
}

func TestResolve_AutoOff_ConfirmRawHasNoEffectAtConfirmTime(t *testing.T) {
	// When Auto is false the confirm set is unused functionally, but the
	// human invariant still holds and parsing still applies.
	r, err := Resolve(false, false, "fix", true, envOf(nil))
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if r.Auto {
		t.Error("Auto should be false")
	}
	// Confirm always prompts when Auto is off — even for categories not in
	// the confirm set.
	ok, _ := Confirm(r, CategoryPrompt, strings.NewReader("y\n"), &bytes.Buffer{}, "Go?")
	if !ok {
		t.Error("expected prompt to have been read")
	}
}

func TestResolve_AutoOff_NoDefaultConfirmList(t *testing.T) {
	// When Auto is false the default exclusion list is not materialised —
	// commit/fix should not appear in ConfirmSet just because the default
	// names them.
	r, err := Resolve(false, false, "", false, envOf(nil))
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if r.ConfirmSet[CategoryCommit] {
		t.Error("commit must not appear in ConfirmSet when Auto is off")
	}
	if r.ConfirmSet[CategoryFix] {
		t.Error("fix must not appear in ConfirmSet when Auto is off")
	}
}

func TestResolve_ConfirmList_TrimsAndSkipsBlanks(t *testing.T) {
	r, err := Resolve(true, true, "  commit , , fix  ", true, envOf(nil))
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if !r.ConfirmSet[CategoryCommit] || !r.ConfirmSet[CategoryFix] {
		t.Errorf("expected commit + fix, got %+v", r.ConfirmSet)
	}
}

func TestConfirm_ShortCircuitsWhenAutoAndNotInConfirmSet(t *testing.T) {
	r, _ := Resolve(true, true, "commit,fix", true, envOf(nil))
	// "Empty" reader — a real read would block; short-circuit must avoid it.
	ok, err := Confirm(r, CategoryPrompt, strings.NewReader(""), &bytes.Buffer{}, "Anything?")
	if err != nil {
		t.Fatalf("Confirm err: %v", err)
	}
	if !ok {
		t.Error("expected short-circuit true for prompt category under --auto")
	}
}

func TestConfirm_PromptsWhenAutoAndInConfirmSet(t *testing.T) {
	r, _ := Resolve(true, true, "commit,fix", true, envOf(nil))
	// Commit is in confirm set; must consume the reader.
	var out bytes.Buffer
	ok, err := Confirm(r, CategoryCommit, strings.NewReader("y\n"), &out, "Commit?")
	if err != nil {
		t.Fatalf("Confirm err: %v", err)
	}
	if !ok {
		t.Error("expected true from y input")
	}
	if !strings.Contains(out.String(), "Commit? [y/n]:") {
		t.Errorf("expected prompt in output, got %q", out.String())
	}
}

func TestConfirm_HumanNeverShortCircuits(t *testing.T) {
	// Even with --confirm= (truly autonomous), human-STOPs must still
	// prompt. This is the load-bearing invariant of the design.
	r, _ := Resolve(true, true, "", true, envOf(nil))
	if r.ConfirmSet[CategoryHuman] != true {
		t.Fatal("setup invariant: human must be in confirm set")
	}
	var out bytes.Buffer
	ok, err := Confirm(r, CategoryHuman, strings.NewReader("y\n"), &out, "Human STOP?")
	if err != nil {
		t.Fatalf("Confirm err: %v", err)
	}
	if !ok {
		t.Error("expected y to be read")
	}
	if !strings.Contains(out.String(), "Human STOP?") {
		t.Error("human prompt should have been written to out — short-circuit must not have fired")
	}
}

func TestConfirm_DoesNotShortCircuitWhenAutoOff(t *testing.T) {
	r, _ := Resolve(false, false, "", false, envOf(nil))
	var out bytes.Buffer
	ok, err := Confirm(r, CategoryPrompt, strings.NewReader("n\n"), &out, "Anything?")
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
	r, _ := Resolve(true, true, "commit", true, envOf(nil))
	asker := &stubAsker{answers: []string{"n"}}
	ok, err := ConfirmVia(r, CategoryPrompt, asker, &bytes.Buffer{}, "Anything?")
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

func TestConfirmVia_PromptsWhenInConfirmSet(t *testing.T) {
	r, _ := Resolve(true, true, "commit", true, envOf(nil))
	asker := &stubAsker{answers: []string{"y"}}
	ok, err := ConfirmVia(r, CategoryCommit, asker, &bytes.Buffer{}, "Commit?")
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

func TestResolved_ConfirmListString_OmitsImplicitHumanAndIsSorted(t *testing.T) {
	r, _ := Resolve(true, true, "release,commit,fix", true, envOf(nil))
	got := r.ConfirmListString()
	// Categories are sorted by underlying int value (the const declaration
	// order): commit, fix, release, prompt, human. Human is dropped.
	want := "commit,fix,release"
	if got != want {
		t.Errorf("ConfirmListString() = %q, want %q", got, want)
	}
}

func TestResolved_ConfirmListString_EmptyWhenOnlyHuman(t *testing.T) {
	r, _ := Resolve(true, true, "", true, envOf(nil))
	if got := r.ConfirmListString(); got != "" {
		t.Errorf("ConfirmListString() = %q, want empty", got)
	}
}
