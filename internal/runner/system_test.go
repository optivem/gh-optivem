package runner

import "testing"

func TestTransientNetRE(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{
			name: "MCR 403 manifest fetch",
			msg:  "unexpected status from HEAD request to https://mcr.microsoft.com/v2/dotnet/aspnet/manifests/8.0: 403 Forbidden",
			want: true,
		},
		{
			name: "buildx failed to resolve source metadata",
			msg:  "failed to solve: failed to resolve source metadata for mcr.microsoft.com/dotnet/aspnet:8.0",
			want: true,
		},
		{
			name: "Docker Hub rate limit (lowercase)",
			msg:  "toomanyrequests: You have reached your pull rate limit",
			want: true,
		},
		{
			name: "Docker Hub rate limit (HTTP phrase)",
			msg:  "received unexpected HTTP status: 429 Too Many Requests",
			want: true,
		},
		{
			name: "manifest unknown",
			msg:  "manifest unknown: manifest tag not found",
			want: true,
		},
		{
			name: "ECONNRESET",
			msg:  "read tcp: connection reset by peer ECONNRESET",
			want: true,
		},
		{
			name: "503 Service Unavailable",
			msg:  "Service Unavailable from upstream registry",
			want: true,
		},
		{
			name: "non-network exit code",
			msg:  "exit status 1",
			want: false,
		},
		{
			name: "yaml parse error",
			msg:  "yaml: line 12: did not find expected key",
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := transientNetRE.MatchString(c.msg); got != c.want {
				t.Errorf("MatchString(%q) = %v, want %v", c.msg, got, c.want)
			}
		})
	}
}
