package steps

import (
	"context"
	"fmt"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/kernel/shell"
)

// RemoteClass says what init would do to one remote resource.
type RemoteClass string

const (
	// RemoteCreate: the resource is demonstrably absent. Would be created.
	RemoteCreate RemoteClass = "create"
	// RemoteReuse: the resource exists and already matches. No change.
	RemoteReuse RemoteClass = "reuse"
	// RemoteDiffers: the resource exists but init would change it. Detail says how.
	RemoteDiffers RemoteClass = "differs"
)

// RemoteEntry is the classification of one remote resource.
type RemoteEntry struct {
	// Kind is the resource type: environment, variable, secret, label,
	// project-board, sonar-org or sonar-project.
	Kind string
	// Repo is the owning repository for repo-scoped resources, empty otherwise.
	Repo  string
	Name  string
	Class RemoteClass
	// Detail describes a RemoteDiffers change. Variable values are shown;
	// secret values are never read or printed.
	Detail string
}

// remoteRunCapture is the seam the repo-scoped probes read through. Tests
// replace it to return canned responses without reaching GitHub.
var remoteRunCapture = shell.RunCaptureWithRetry

// SonarProber is the read-only slice of shell.SonarCloud the probes use.
type SonarProber interface {
	OrgExists(ctx context.Context, key string) (bool, error)
	ProjectExists(ctx context.Context, key string) (bool, error)
}

// RemoteTargets carries what the probes need beyond cfg.
type RemoteTargets struct {
	SonarOrg         string
	SonarProjectKeys []string
	Sonar            SonarProber
}

// probeError reports a probe that could not reach a definitive answer. It is
// never folded into "absent": a caller that sees an error knows nothing about
// the resource.
func probeError(kind, name string, err error) error {
	msg := err.Error()
	hint := "check network connectivity and retry"
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "http 401") || strings.Contains(lower, "bad credentials") || strings.Contains(lower, "unauthorized"):
		hint = "the token is invalid or expired; re-authenticate (gh auth login) or refresh the token"
	case strings.Contains(lower, "http 403") || strings.Contains(lower, "forbidden") || strings.Contains(lower, "not authorized"):
		hint = "the token lacks permission to read this resource; grant the required scope (repo / admin:repo_hook / project) and retry"
	case strings.Contains(lower, "http 5"):
		hint = "the service returned a server error after retries; retry later"
	}
	return fmt.Errorf("cannot determine whether %s %q exists (probe failed, this is NOT \"absent\"): %v\n  likely fix: %s", kind, name, err, hint)
}

func classifyPresence(present bool) RemoteClass {
	if present {
		return RemoteReuse
	}
	return RemoteCreate
}

// ProbeRemote classifies every remote resource init would touch, read-only.
// Repos not in cfg.PreExistingRepos do not exist yet, so everything on them is
// reported as create without any call. The first indeterminate probe aborts
// with an error naming the resource and the likely fix.
func ProbeRemote(cfg *config.Config, targets RemoteTargets) ([]RemoteEntry, error) {
	var out []RemoteEntry

	envs, err := probeEnvironments(cfg)
	if err != nil {
		return nil, err
	}
	out = append(out, envs...)

	labels, err := probeLabels(cfg)
	if err != nil {
		return nil, err
	}
	out = append(out, labels...)

	for _, plan := range variableSecretPlan(cfg) {
		entries, err := probeSettings(cfg, plan)
		if err != nil {
			return nil, err
		}
		out = append(out, entries...)
	}

	board, err := probeProjectBoard(cfg)
	if err != nil {
		return nil, err
	}
	out = append(out, board...)

	sonar, err := probeSonar(targets)
	if err != nil {
		return nil, err
	}
	out = append(out, sonar...)

	return out, nil
}

func probeEnvironments(cfg *config.Config) ([]RemoteEntry, error) {
	present := map[string]bool{}
	if cfg.PreExistingRepos[cfg.FullRepo] {
		names, err := ghLines(fmt.Sprintf("gh api repos/%s/environments --paginate --jq .environments[].name", cfg.FullRepo))
		if err != nil {
			return nil, probeError("environments of repository", cfg.FullRepo, err)
		}
		for _, n := range names {
			present[n] = true
		}
	}
	var out []RemoteEntry
	for _, e := range scaffoldEnvironments {
		out = append(out, RemoteEntry{Kind: "environment", Repo: cfg.FullRepo, Name: e, Class: classifyPresence(present[e])})
	}
	return out, nil
}

func probeLabels(cfg *config.Config) ([]RemoteEntry, error) {
	type label struct{ color, description string }
	have := map[string]label{}
	if cfg.PreExistingRepos[cfg.FullRepo] {
		lines, err := ghLines(fmt.Sprintf(
			"gh label list --repo %s --limit 1000 --json name,color,description --jq '.[] | [.name,.color,.description] | @tsv'", cfg.FullRepo))
		if err != nil {
			return nil, probeError("labels of repository", cfg.FullRepo, err)
		}
		for _, ln := range lines {
			f := strings.SplitN(ln, "\t", 3)
			if len(f) != 3 {
				return nil, probeError("labels of repository", cfg.FullRepo, fmt.Errorf("malformed label row %q", ln))
			}
			have[f[0]] = label{color: strings.ToLower(f[1]), description: f[2]}
		}
	}
	var out []RemoteEntry
	for _, l := range subtypeLabels {
		e := RemoteEntry{Kind: "label", Repo: cfg.FullRepo, Name: l.name}
		cur, ok := have[l.name]
		switch {
		case !ok:
			e.Class = RemoteCreate
		case cur.color == strings.ToLower(l.color) && cur.description == l.description:
			e.Class = RemoteReuse
		default:
			e.Class = RemoteDiffers
			e.Detail = fmt.Sprintf("color %s -> %s, description %q -> %q", cur.color, l.color, cur.description, l.description)
		}
		out = append(out, e)
	}
	return out, nil
}

func probeSettings(cfg *config.Config, plan settingsPlan) ([]RemoteEntry, error) {
	vars := map[string]string{}
	secrets := map[string]bool{}
	if cfg.PreExistingRepos[plan.Repo] {
		varLines, err := ghLines(fmt.Sprintf("gh api repos/%s/actions/variables --paginate --jq '.variables[] | [.name,.value] | @tsv'", plan.Repo))
		if err != nil {
			return nil, probeError("Actions variables of repository", plan.Repo, err)
		}
		for _, ln := range varLines {
			f := strings.SplitN(ln, "\t", 2)
			if len(f) != 2 {
				return nil, probeError("Actions variables of repository", plan.Repo, fmt.Errorf("malformed variable row %q", ln))
			}
			vars[f[0]] = f[1]
		}
		secretNames, err := ghLines(fmt.Sprintf("gh api repos/%s/actions/secrets --paginate --jq .secrets[].name", plan.Repo))
		if err != nil {
			return nil, probeError("Actions secrets of repository", plan.Repo, err)
		}
		for _, n := range secretNames {
			secrets[n] = true
		}
	}
	var out []RemoteEntry
	for _, v := range plan.Variables {
		e := RemoteEntry{Kind: "variable", Repo: plan.Repo, Name: v.name}
		cur, ok := vars[v.name]
		switch {
		case !ok:
			e.Class = RemoteCreate
		case cur == v.value:
			e.Class = RemoteReuse
		default:
			e.Class = RemoteDiffers
			e.Detail = fmt.Sprintf("%q -> %q", cur, v.value)
		}
		out = append(out, e)
	}
	for _, s := range plan.Secrets {
		e := RemoteEntry{Kind: "secret", Repo: plan.Repo, Name: s.name}
		if secrets[s.name] {
			// GitHub never returns secret values, so an existing secret cannot be
			// compared; setting it always replaces it.
			e.Class = RemoteDiffers
			e.Detail = "existing value will be replaced (values are not readable or shown)"
		} else {
			e.Class = RemoteCreate
		}
		out = append(out, e)
	}
	return out, nil
}

func probeProjectBoard(cfg *config.Config) ([]RemoteEntry, error) {
	if cfg.NoProject {
		return nil, nil
	}
	var pr *ghProject
	required := canonicalStatusOptions
	name := cfg.SystemName
	if cfg.ProjectURL == "" {
		found, err := findProjectByTitle(cfg.Owner, cfg.SystemName)
		if err != nil {
			return nil, probeError("project board", cfg.SystemName, err)
		}
		if found == nil {
			return []RemoteEntry{{Kind: "project-board", Name: name, Class: RemoteCreate}}, nil
		}
		pr = found
	} else {
		owner, number, err := parseProjectURL(cfg.ProjectURL)
		if err != nil {
			return nil, fmt.Errorf("--project-url %q: %v", cfg.ProjectURL, err)
		}
		pr = &ghProject{Owner: owner, Number: number, URL: cfg.ProjectURL}
		required = atddRequiredStatusOptions
		name = cfg.ProjectURL
	}
	field, err := loadStatusField(pr)
	if err != nil {
		return nil, probeError("project board", name, err)
	}
	current := make([]string, 0, len(field.Options))
	for _, o := range field.Options {
		current = append(current, o.Name)
	}
	e := RemoteEntry{Kind: "project-board", Name: name, Class: RemoteReuse}
	if missing := missingOptions(current, required); len(missing) > 0 {
		e.Class = RemoteDiffers
		e.Detail = "Status options to add: " + strings.Join(missing, ", ")
	}
	return []RemoteEntry{e}, nil
}

func probeSonar(t RemoteTargets) ([]RemoteEntry, error) {
	if t.Sonar == nil {
		return nil, nil
	}
	ctx := context.Background()
	var out []RemoteEntry
	if t.SonarOrg != "" {
		ok, err := t.Sonar.OrgExists(ctx, t.SonarOrg)
		if err != nil {
			return nil, probeError("SonarCloud organization", t.SonarOrg, err)
		}
		out = append(out, RemoteEntry{Kind: "sonar-org", Name: t.SonarOrg, Class: classifyPresence(ok)})
	}
	for _, key := range t.SonarProjectKeys {
		ok, err := t.Sonar.ProjectExists(ctx, key)
		if err != nil {
			return nil, probeError("SonarCloud project", key, err)
		}
		out = append(out, RemoteEntry{Kind: "sonar-project", Name: key, Class: classifyPresence(ok)})
	}
	return out, nil
}

// ghLines runs a read-only gh command and returns its non-empty output lines.
func ghLines(cmd string) ([]string, error) {
	raw, err := remoteRunCapture(cmd, "")
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, ln := range strings.Split(raw, "\n") {
		ln = strings.TrimRight(ln, "\r")
		if ln != "" {
			lines = append(lines, ln)
		}
	}
	return lines, nil
}
