package runner

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// composeConfigJSON returns `docker compose -f <composeFile> config --format
// json` for one stack. A package var so tests can substitute canned output and
// exercise the --external-only arg path without docker.
var composeConfigJSON = func(composeFile, cwd string) ([]byte, error) {
	out, err := dockerCaptureChecked(cwd, "compose", "-f", composeFile, "config", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("read compose config for %s: %w", composeFile, err)
	}
	return []byte(out), nil
}

// composeConfigDoc is the subset of `docker compose config --format json` the
// stand-in selection reads. build is either a string (short form) or an object
// with a context; volumes entries are long-form objects after `config`
// normalisation (short-form strings are tolerated and skipped).
type composeConfigDoc struct {
	Services map[string]struct {
		Build   json.RawMessage   `json:"build"`
		Volumes []json.RawMessage `json:"volumes"`
	} `json:"services"`
}

// SelectStandInServices returns, sorted, the compose services that are
// external-system stand-ins (simulators / stubs): every service whose build
// context or any bind-mount source resolves under one of standInDirs.
//
// configJSON is `docker compose config --format json` output. Relative build
// contexts / bind sources are resolved against composeDir (the compose project
// dir). Relative standInDirs are resolved against baseDir (the directory of
// gh-optivem.yaml, which the registry stub.path / simulator.path values are
// relative to). Paths are normalised (slashes, Clean, case-insensitive for
// drive-letter paths) so Windows and Unix spellings compare equal.
//
// Zero matches is a hard error naming the searched paths — an --external-only
// restart that recreates nothing would be a silent no-op.
func SelectStandInServices(configJSON []byte, composeDir, baseDir string, standInDirs []string) ([]string, error) {
	if len(standInDirs) == 0 {
		return nil, fmt.Errorf("no external-system stand-in paths configured: the external-systems: registry in gh-optivem.yaml declares no stub.path / simulator.path")
	}
	var doc composeConfigDoc
	if err := json.Unmarshal(configJSON, &doc); err != nil {
		return nil, fmt.Errorf("parse docker compose config JSON: %w", err)
	}
	dirs := make([]string, 0, len(standInDirs))
	for _, d := range standInDirs {
		dirs = append(dirs, normalizePath(resolveAgainst(baseDir, d)))
	}
	under := func(p string) bool {
		if p == "" {
			return false
		}
		n := normalizePath(resolveAgainst(composeDir, p))
		for _, d := range dirs {
			if n == d || strings.HasPrefix(n, strings.TrimSuffix(d, "/")+"/") {
				return true
			}
		}
		return false
	}

	var out []string
	for name, svc := range doc.Services {
		if under(buildContext(svc.Build)) {
			out = append(out, name)
			continue
		}
		for _, raw := range svc.Volumes {
			if src, ok := bindSource(raw); ok && under(src) {
				out = append(out, name)
				break
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--external-only: no compose service is an external-system stand-in; searched for services whose build context or bind-mount source lies under %s (compose project dir %s). "+
			"Check the external-systems: registry stub.path / simulator.path values in gh-optivem.yaml and the stand-in services' build contexts / volumes in the compose file",
			strings.Join(standInDirs, ", "), composeDir)
	}
	sort.Strings(out)
	return out, nil
}

// buildContext extracts the build context from a compose service's build
// field (string short form or {context: ...} object). Empty when absent.
func buildContext(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		Context string `json:"context"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Context
	}
	return ""
}

// bindSource returns the source of a long-form bind-mount volume entry.
// Named volumes, tmpfs, and unparseable entries report ok=false.
func bindSource(raw json.RawMessage) (string, bool) {
	var v struct {
		Type   string `json:"type"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", false
	}
	if v.Type != "bind" || v.Source == "" {
		return "", false
	}
	return v.Source, true
}

// resolveAgainst joins p onto base when p is relative (in either OS's
// spelling); absolute paths (Unix "/x" or Windows "C:\x") are returned as is.
func resolveAgainst(base, p string) string {
	if isAbsAnyOS(p) || base == "" {
		return p
	}
	return strings.TrimSuffix(toSlash(base), "/") + "/" + toSlash(p)
}

func isAbsAnyOS(p string) bool {
	s := toSlash(p)
	return strings.HasPrefix(s, "/") || hasDriveLetter(s) || filepath.IsAbs(p)
}

func hasDriveLetter(s string) bool {
	return len(s) >= 2 && s[1] == ':' &&
		((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z'))
}

func toSlash(p string) string { return strings.ReplaceAll(p, `\`, "/") }

// normalizePath canonicalises a path for prefix comparison: forward slashes,
// cleaned, and lower-cased when it carries a Windows drive letter (NTFS is
// case-insensitive, and docker/compose and the OS disagree on drive casing).
func normalizePath(p string) string {
	s := path.Clean(toSlash(p))
	if hasDriveLetter(s) {
		s = strings.ToLower(s)
	}
	return s
}

// buildExternalOnlyUpArgs returns the `docker compose up` args that rebuild and
// force-recreate only the given stand-in services. --no-deps keeps the system
// under test (and postgres) running untouched.
func buildExternalOnlyUpArgs(composeFile string, standIns []string) []string {
	args := []string{"-f", composeFile, "up", "-d", "--build", "--force-recreate", "--no-deps"}
	return append(args, standIns...)
}
