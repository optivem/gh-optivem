// gitremote.go infers GitHub owner/repo from the local `origin` remote so
// the interactive missing-file prompt doesn't have to ask for values the
// repo's git config already knows. Limited to github.com URLs (https and
// ssh, with or without `.git`) — any other host or malformed value
// returns ok=false and the prompt falls back to its existing
// validated-input path.
package configinit

import (
	"os/exec"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
)

// runGitRemote is the shell-out hook InferOwnerRepo uses. Package-private
// var so tests can stub it without spawning git.
var runGitRemote = func(cwd string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	if cwd != "" {
		cmd.Dir = cwd
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// InferOwnerRepo runs `git remote get-url origin` in cwd, parses the
// result, and returns the GitHub owner + repo on success. Returns
// ok=false (with empty strings) when the directory is not a git repo,
// `origin` is unset, the URL is not on github.com, or the URL doesn't
// match a recognised shape.
//
// Recognised URL shapes:
//
//	https://github.com/<owner>/<repo>
//	https://github.com/<owner>/<repo>.git
//	git@github.com:<owner>/<repo>.git
//	git@github.com:<owner>/<repo>
//	ssh://git@github.com/<owner>/<repo>.git
//
// Anything else (Enterprise hosts, gitlab, malformed strings) → ok=false.
func InferOwnerRepo(cwd string) (owner, repo string, ok bool) {
	raw, err := runGitRemote(cwd)
	if err != nil || raw == "" {
		return "", "", false
	}
	return parseGitHubRemote(raw)
}

// parseGitHubRemote is the pure parsing core, exported for tests.
func parseGitHubRemote(raw string) (owner, repo string, ok bool) {
	rest, ok := stripGitHubHost(raw)
	if !ok {
		return "", "", false
	}
	rest = strings.TrimSuffix(rest, ".git")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	owner, repo = parts[0], parts[1]
	if owner == "" || repo == "" {
		return "", "", false
	}
	if msg := config.ValidateOwnerFormat(owner); msg != "" {
		return "", "", false
	}
	if msg := config.ValidateRepoFormat(repo); msg != "" {
		return "", "", false
	}
	return owner, repo, true
}

// stripGitHubHost recognises the supported URL prefixes and returns the
// `<owner>/<repo>[.git]` tail. Any other shape → ok=false.
func stripGitHubHost(raw string) (string, bool) {
	for _, prefix := range []string{
		"https://github.com/",
		"http://github.com/",
		"ssh://git@github.com/",
	} {
		if strings.HasPrefix(raw, prefix) {
			return raw[len(prefix):], true
		}
	}
	const scp = "git@github.com:"
	if strings.HasPrefix(raw, scp) {
		return raw[len(scp):], true
	}
	return "", false
}
