package projectconfig

import (
	"strings"
	"testing"
)

func TestDefaultChannels_IsApiUiAndCopy(t *testing.T) {
	t.Parallel()
	got := DefaultChannels()
	if len(got) != 2 || got[0] != ChannelAPI || got[1] != ChannelUI {
		t.Fatalf("DefaultChannels() = %v, want [api ui]", got)
	}
	// Mutating the returned slice must not corrupt the canonical set.
	got[0] = "mutated"
	if again := DefaultChannels(); again[0] != ChannelAPI {
		t.Fatalf("DefaultChannels() leaked its backing array: got %v", again)
	}
}

// TestExecutionChannels_APIBeforeUI pins the implementation order (plan
// 20261109-1257): API before UI whatever the config lists, subsets kept.
func TestExecutionChannels_APIBeforeUI(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want []string
	}{
		{[]string{ChannelUI, ChannelAPI}, []string{ChannelAPI, ChannelUI}},
		{[]string{ChannelAPI, ChannelUI}, []string{ChannelAPI, ChannelUI}},
		{[]string{ChannelUI}, []string{ChannelUI}},
		{nil, []string{}},
	}
	for _, c := range cases {
		got := (&Config{Channels: c.in}).ExecutionChannels()
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("ExecutionChannels(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestValidate_Channels_AcceptsSupportedSubsets(t *testing.T) {
	t.Parallel()
	for _, channels := range [][]string{
		nil,                     // absent — optional field
		{ChannelAPI, ChannelUI}, // full shop set
		{ChannelAPI},            // API-only project
		{ChannelUI, ChannelAPI}, // reordered (order = impl order)
	} {
		cfg := &Config{Project: Project{Provider: ProviderGitHub}, Channels: channels}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() rejected valid channels %v: %v", channels, err)
		}
	}
}

func TestValidate_Channels_RejectsUppercaseWithDidYouMean(t *testing.T) {
	t.Parallel()
	cfg := &Config{Project: Project{Provider: ProviderGitHub}, Channels: []string{"API"}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for uppercase channel token, got nil")
	}
	if !strings.Contains(err.Error(), `did you mean "api"`) {
		t.Fatalf("expected did-you-mean hint, got: %v", err)
	}
}

func TestValidate_Channels_RejectsUnknownToken(t *testing.T) {
	t.Parallel()
	cfg := &Config{Project: Project{Provider: ProviderGitHub}, Channels: []string{"grpc"}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unknown channel token, got nil")
	}
	if !strings.Contains(err.Error(), "must be one of") {
		t.Fatalf("expected supported-set hint, got: %v", err)
	}
}

func TestValidate_Channels_RejectsDuplicate(t *testing.T) {
	t.Parallel()
	cfg := &Config{Project: Project{Provider: ProviderGitHub}, Channels: []string{ChannelAPI, ChannelAPI}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for duplicate channel token, got nil")
	}
}

func TestValidate_Channels_RejectsEmptyToken(t *testing.T) {
	t.Parallel()
	cfg := &Config{Project: Project{Provider: ProviderGitHub}, Channels: []string{""}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty channel token, got nil")
	}
}
