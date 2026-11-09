package config

import (
	"errors"
	"strings"
	"testing"
)

func TestClassifyProjectProbe(t *testing.T) {
	exit1 := errors.New("exit status 1")
	cases := []struct {
		name    string
		out     string
		runErr  error
		wantNil bool
		want    string
	}{
		{name: "exists", out: `{"data":{"user":{"projectV2":{"id":"PVT_x"}}}}`, wantNil: true},
		{name: "not found", out: `gh: Could not resolve to a ProjectV2 with the number 999. (NOT_FOUND)`, runErr: exit1, want: "not found or not accessible"},
		{name: "missing scope", out: `gh: Your token has not been granted the required scopes to execute this query. (INSUFFICIENT_SCOPES)`, runErr: exit1, want: "could not check project acme/999"},
		{name: "auth failure", out: `HTTP 401: Bad credentials`, runErr: exit1, want: "Bad credentials"},
		{name: "network failure", out: `dial tcp: lookup api.github.com: no such host`, runErr: exit1, want: "no such host"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyProjectProbe("acme", 999, tc.out, tc.runErr)
			if tc.wantNil {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
			if tc.name != "not found" && strings.Contains(err.Error(), "not found or not accessible") {
				t.Fatalf("indeterminate failure reported as not found: %v", err)
			}
		})
	}
}
