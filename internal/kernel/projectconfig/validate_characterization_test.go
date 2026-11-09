package projectconfig

import "testing"

func baseValidateConfig() *Config {
	return &Config{Project: Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"}}
}

func TestValidate_ExactMessages(t *testing.T) {
	tests := []struct {
		name string
		mut  func(c *Config)
		want string
	}{
		{"node-extras and replacements", func(c *Config) {
			c.NodeExtras = map[string]string{"n": "a.md"}
			c.NodeReplacements = map[string]string{"n": "b.md"}
		}, `config: node "n" appears in both node-extras and node-replacements (replace supersedes extras)`},
		{"license", func(c *Config) { c.License = "nope" }, ""},
		{"provider unknown", func(c *Config) { c.Project.Provider = "x" },
			`config: project.provider "x" must be one of "github", "markdown"`},
		{"github non-github url", func(c *Config) { c.Project.URL = "https://example.com/x" },
			`config: project.provider="github" but project.url="https://example.com/x" is not a github URL`},
		{"markdown http url", func(c *Config) {
			c.Project.Provider = ProviderMarkdown
			c.Project.URL = "http://x"
		}, `config: project.provider="markdown" but project.url="http://x" is an HTTP URL, not a directory path`},
		{"repos empty path", func(c *Config) { c.LocalRepos = []RepoEntry{{Path: ""}} },
			`config: repos[0].path is required`},
		{"repos duplicate", func(c *Config) { c.LocalRepos = []RepoEntry{{Path: "../a"}, {Path: "../a/"}} },
			`config: repos[1].path "../a/" appears more than once`},
		{"microservices needs services", func(c *Config) {
			c.System.Architecture = ArchMicroservices
		}, `config: system.architecture=microservices requires at least one system.backend-services entry`},
		{"monolith needs fields", func(c *Config) {
			c.System.Architecture = ArchMonolith
		}, `config: system.architecture=monolith requires system.{path,repo,lang} all set`},
		{"system-test inner-loop", func(c *Config) {
			c.SystemTest = TierSpec{Path: "st", Repo: "r", Lang: "java", InnerLoop: InnerLoop{Mode: "x"}}
		}, `config: system-test.inner-loop is not a supported field (inner-loop belongs on a system backend/frontend tier, service, monolith system, or component)`},
		{"backend adapter channels", func(c *Config) {
			c.System.Backend.SystemDriverAdapterChannels = map[string]string{"api": "x"}
		}, `config: system.backend.system-driver-adapter-channels is not a supported field (system-test-only; use system-test.system-driver-adapter-channels)`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := baseValidateConfig()
			tc.mut(c)
			err := c.Validate()
			if err == nil {
				t.Fatal("expected error")
			}
			if tc.want != "" && err.Error() != tc.want {
				t.Fatalf("got  %q\nwant %q", err.Error(), tc.want)
			}
		})
	}
}

func TestValidate_SystemTestPathsRules(t *testing.T) {
	key := CanonicalPathKeys()[0]
	tests := []struct {
		name  string
		paths map[string]string
		want  string
	}{
		{"not canonical", map[string]string{"zzz": "a"},
			`config: system-test.paths.zzz is not a canonical Family B key; see internal/projectconfig/path-keys.md for the supported set`},
		{"marker", map[string]string{key: "a/${x}"},
			`config: system-test.paths.` + key + ` "a/${x}" contains a ${...} marker; under SSoT, paths must be fully resolved (substitution is scaffold-time-only)`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := baseValidateConfig()
			c.SystemTest.Paths = tc.paths
			err := c.Validate()
			if err == nil || err.Error() != tc.want {
				t.Fatalf("got %v\nwant %q", err, tc.want)
			}
		})
	}
}
