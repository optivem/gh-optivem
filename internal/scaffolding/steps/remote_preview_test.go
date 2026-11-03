package steps

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
)

type remoteStub struct {
	resp  map[string]string
	errs  map[string]error
	calls []string
}

func installRemoteStub(t *testing.T, resp map[string]string, errs map[string]error) *remoteStub {
	t.Helper()
	s := &remoteStub{resp: resp, errs: errs}
	prev := remoteRunCapture
	remoteRunCapture = func(cmd, _ string) (string, error) {
		s.calls = append(s.calls, cmd)
		for k, e := range s.errs {
			if strings.Contains(cmd, k) {
				return "", e
			}
		}
		for k, v := range s.resp {
			if strings.Contains(cmd, k) {
				return v, nil
			}
		}
		return "", errors.New("stub: unmatched " + cmd)
	}
	t.Cleanup(func() { remoteRunCapture = prev })
	return s
}

func findEntry(t *testing.T, es []RemoteEntry, kind, name string) RemoteEntry {
	t.Helper()
	for _, e := range es {
		if e.Kind == kind && e.Name == name {
			return e
		}
	}
	t.Fatalf("no %s %q in %+v", kind, name, es)
	return RemoteEntry{}
}

func existingCfg() *config.Config {
	return &config.Config{
		Owner: "acme", FullRepo: "acme/shop", SystemName: "Shop",
		RepoStrategy: "monorepo", Arch: "monolith", NoProject: true,
		DockerHubUsername: "dh", DockerHubToken: "SECRET-VALUE",
		PreExistingRepos: map[string]bool{"acme/shop": true},
	}
}

func TestProbeRemote_ClassesAndNeverPrintsSecrets(t *testing.T) {
	stub := installRemoteStub(t, map[string]string{
		"/environments":      "qa\nstaging\n",
		"gh label list":      "subtype:system-interface-redesign\t1D76DB\tStructural change to a system Driver Adapter\nsubtype:system-implementation-refactoring\tffffff\told text\n",
		"/actions/variables": "DOCKERHUB_USERNAME\tother\n",
		"/actions/secrets":   "DOCKERHUB_TOKEN\n",
	}, nil)

	got, err := ProbeRemote(existingCfg(), RemoteTargets{})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		kind, name string
		class      RemoteClass
	}{
		{"environment", "qa", RemoteReuse},
		{"environment", "acceptance", RemoteCreate},
		{"label", "subtype:system-interface-redesign", RemoteReuse},
		{"label", "subtype:system-implementation-refactoring", RemoteDiffers},
		{"label", "subtype:external-system-interface-redesign", RemoteCreate},
		{"variable", "DOCKERHUB_USERNAME", RemoteDiffers},
		{"secret", "DOCKERHUB_TOKEN", RemoteDiffers},
		{"secret", "SONAR_TOKEN", RemoteCreate},
	}
	for _, c := range cases {
		if e := findEntry(t, got, c.kind, c.name); e.Class != c.class {
			t.Errorf("%s %s: class %s, want %s", c.kind, c.name, e.Class, c.class)
		}
	}
	for _, e := range got {
		if strings.Contains(e.Detail, "SECRET-VALUE") {
			t.Errorf("secret value leaked in detail: %+v", e)
		}
	}
	if d := findEntry(t, got, "variable", "DOCKERHUB_USERNAME").Detail; !strings.Contains(d, "other") || !strings.Contains(d, "dh") {
		t.Errorf("variable detail should show the change, got %q", d)
	}
	for _, c := range stub.calls {
		if strings.Contains(c, "-X") || strings.Contains(c, "set ") || strings.Contains(c, "create") {
			t.Errorf("probe issued a write-looking command: %s", c)
		}
	}
}

func TestProbeRemote_NewRepoIsAllCreateWithoutCalls(t *testing.T) {
	stub := installRemoteStub(t, nil, nil)
	cfg := existingCfg()
	cfg.PreExistingRepos = nil

	got, err := ProbeRemote(cfg, RemoteTargets{})
	if err != nil {
		t.Fatal(err)
	}
	if len(stub.calls) != 0 {
		t.Errorf("repo does not exist yet; expected no calls, got %v", stub.calls)
	}
	for _, e := range got {
		if e.Class != RemoteCreate {
			t.Errorf("%+v should be create", e)
		}
	}
}

func TestProbeRemote_IndeterminateFailsHard(t *testing.T) {
	for _, tc := range []struct{ name, errText, wantHint string }{
		{"forbidden", "exit status 1: gh: Resource not accessible (HTTP 403)", "permission"},
		{"unauthorized", "exit status 1: gh: Bad credentials (HTTP 401)", "token"},
		{"server error", "exit status 1: gh: Server Error (HTTP 502)", "retry"},
		{"network", "exit status 1: dial tcp: no such host", "network"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			installRemoteStub(t, nil, map[string]error{"/environments": errors.New(tc.errText)})
			got, err := ProbeRemote(existingCfg(), RemoteTargets{})
			if err == nil {
				t.Fatalf("expected error, got entries %+v", got)
			}
			msg := err.Error()
			if !strings.Contains(msg, "acme/shop") || !strings.Contains(msg, "NOT \"absent\"") || !strings.Contains(strings.ToLower(msg), tc.wantHint) {
				t.Errorf("error must name resource, refuse 'absent', and hint at the fix; got %q", msg)
			}
		})
	}
}

func TestProbeRemote_MalformedRowFailsHard(t *testing.T) {
	installRemoteStub(t, map[string]string{
		"/environments": "qa\n",
		"gh label list": "garbage-without-tabs\n",
	}, nil)
	if _, err := ProbeRemote(existingCfg(), RemoteTargets{}); err == nil {
		t.Fatal("malformed label row must fail, not be treated as absent")
	}
}

func TestProbeRemote_ProjectBoard(t *testing.T) {
	base := map[string]string{
		"/environments": "", "gh label list": "", "/actions/variables": "", "/actions/secrets": "",
	}
	t.Run("absent -> create", func(t *testing.T) {
		installRemoteStub(t, base, nil)
		stub := newStubRunner(t)
		stub.captureResp["project list"] = `{"projects":[]}`
		stub.install()
		cfg := existingCfg()
		cfg.NoProject = false
		got, err := ProbeRemote(cfg, RemoteTargets{})
		if err != nil {
			t.Fatal(err)
		}
		if e := findEntry(t, got, "project-board", "Shop"); e.Class != RemoteCreate {
			t.Errorf("got %s", e.Class)
		}
		if n := len(stub.calledViaContaining("RunCapture", "project create")); n != 0 {
			t.Errorf("probe must not create, got %d create calls", n)
		}
	})
	t.Run("present missing statuses -> differs", func(t *testing.T) {
		installRemoteStub(t, base, nil)
		stub := newStubRunner(t)
		stub.captureResp["project list"] = `{"projects":[{"id":"P","number":3,"title":"Shop","url":"https://github.com/users/acme/projects/3"}]}`
		stub.captureResp["project field-list"] = `{"fields":[{"id":"F","name":"Status","options":[{"id":"1","name":"Ready"}]}]}`
		stub.install()
		cfg := existingCfg()
		cfg.NoProject = false
		got, err := ProbeRemote(cfg, RemoteTargets{})
		if err != nil {
			t.Fatal(err)
		}
		if e := findEntry(t, got, "project-board", "Shop"); e.Class != RemoteDiffers || !strings.Contains(e.Detail, "Backlog") {
			t.Errorf("got %+v", e)
		}
	})
	t.Run("list auth failure -> error", func(t *testing.T) {
		installRemoteStub(t, base, nil)
		stub := newStubRunner(t)
		stub.captureResp["project list"] = ""
		stub.captureErr["project list"] = errors.New("exit status 1: gh: missing required scopes [read:project] (HTTP 403)")
		stub.install()
		cfg := existingCfg()
		cfg.NoProject = false
		if _, err := ProbeRemote(cfg, RemoteTargets{}); err == nil || !strings.Contains(err.Error(), "project board") {
			t.Fatalf("expected project board probe error, got %v", err)
		}
	})
	t.Run("malformed list -> error", func(t *testing.T) {
		installRemoteStub(t, base, nil)
		stub := newStubRunner(t)
		stub.captureResp["project list"] = "<html>"
		stub.install()
		cfg := existingCfg()
		cfg.NoProject = false
		if _, err := ProbeRemote(cfg, RemoteTargets{}); err == nil {
			t.Fatal("malformed response must fail")
		}
	})
}

type fakeSonar struct {
	orgs, projects map[string]bool
	orgErr         error
	projErr        map[string]error
}

func (f fakeSonar) OrgExists(_ context.Context, k string) (bool, error) {
	return f.orgs[k], f.orgErr
}
func (f fakeSonar) ProjectExists(_ context.Context, k string) (bool, error) {
	return f.projects[k], f.projErr[k]
}

func TestProbeRemote_Sonar(t *testing.T) {
	base := map[string]string{
		"/environments": "", "gh label list": "", "/actions/variables": "", "/actions/secrets": "",
	}
	t.Run("classes", func(t *testing.T) {
		installRemoteStub(t, base, nil)
		got, err := ProbeRemote(existingCfg(), RemoteTargets{
			SonarOrg:         "acme",
			SonarProjectKeys: []string{"acme_shop", "acme_shop-st"},
			Sonar:            fakeSonar{orgs: map[string]bool{"acme": true}, projects: map[string]bool{"acme_shop": true}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if findEntry(t, got, "sonar-org", "acme").Class != RemoteReuse ||
			findEntry(t, got, "sonar-project", "acme_shop").Class != RemoteReuse ||
			findEntry(t, got, "sonar-project", "acme_shop-st").Class != RemoteCreate {
			t.Errorf("unexpected classes: %+v", got)
		}
	})
	t.Run("auth failure is an error not absent", func(t *testing.T) {
		installRemoteStub(t, base, nil)
		_, err := ProbeRemote(existingCfg(), RemoteTargets{
			SonarOrg:         "acme",
			SonarProjectKeys: []string{"acme_shop"},
			Sonar: fakeSonar{orgs: map[string]bool{"acme": true},
				projErr: map[string]error{"acme_shop": errors.New("sonarcloud: components/show acme_shop: HTTP 401: unauthorized")}},
		})
		if err == nil || !strings.Contains(err.Error(), "acme_shop") {
			t.Fatalf("expected error naming the project, got %v", err)
		}
	})
}

func TestVariableSecretPlan_Multirepo(t *testing.T) {
	cfg := &config.Config{
		FullRepo: "a/shop", RepoStrategy: "multirepo", Arch: "multitier",
		FrontendFullRepo: "a/fe", BackendFullRepo: "a/be",
	}
	plans := variableSecretPlan(cfg)
	if len(plans) != 3 {
		t.Fatalf("want 3 plans, got %d", len(plans))
	}
	hasRepoToken := func(p settingsPlan) bool {
		for _, s := range p.Secrets {
			if s.name == "REPO_TOKEN" {
				return true
			}
		}
		return false
	}
	if !hasRepoToken(plans[0]) || !hasRepoToken(plans[1]) || hasRepoToken(plans[2]) {
		t.Errorf("REPO_TOKEN should be on main and frontend only: %+v", plans)
	}
}
