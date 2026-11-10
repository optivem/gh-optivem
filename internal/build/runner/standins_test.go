package runner

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// shopLikeConfig mirrors the shape of `docker compose config --format json`
// for a shop stack: the SUT builds from system/, the simulator builds from
// external-systems/simulators, the WireMock stub bind-mounts
// external-systems/stubs, postgres uses a named volume.
const shopLikeConfigUnix = `{
  "name": "shop",
  "services": {
    "postgres": {"image": "postgres:16", "volumes": [{"type": "volume", "source": "pgdata", "target": "/var/lib/postgresql/data"}]},
    "system": {"build": {"context": "/repo/shop/system/monolith/java", "dockerfile": "Dockerfile"}},
    "external-system-simulators": {"build": {"context": "/repo/shop/external-systems/simulators"}},
    "external-system-stubs": {"image": "wiremock/wiremock", "volumes": [{"type": "bind", "source": "/repo/shop/external-systems/stubs", "target": "/home/wiremock"}]}
  }
}`

func TestSelectStandInServices(t *testing.T) {
	cases := []struct {
		name       string
		json       string
		composeDir string
		baseDir    string
		dirs       []string
		want       []string
		wantErr    string
	}{
		{
			name:    "build context and bind mount both match, SUT and postgres excluded",
			json:    shopLikeConfigUnix,
			baseDir: "/repo/shop",
			dirs:    []string{"external-systems/simulators", "external-systems/stubs"},
			want:    []string{"external-system-simulators", "external-system-stubs"},
		},
		{
			name:    "build-context match only",
			json:    shopLikeConfigUnix,
			baseDir: "/repo/shop",
			dirs:    []string{"external-systems/simulators"},
			want:    []string{"external-system-simulators"},
		},
		{
			name:    "bind-mount match only",
			json:    shopLikeConfigUnix,
			baseDir: "/repo/shop",
			dirs:    []string{"external-systems/stubs"},
			want:    []string{"external-system-stubs"},
		},
		{
			name:    "sibling dir with shared prefix does not match",
			json:    `{"services": {"x": {"build": {"context": "/repo/shop/external-systems/stubs-old"}}}}`,
			baseDir: "/repo/shop",
			dirs:    []string{"external-systems/stubs"},
			wantErr: "no compose service is an external-system stand-in",
		},
		{
			name:    "zero matches is a hard error naming the searched paths",
			json:    shopLikeConfigUnix,
			baseDir: "/repo/shop",
			dirs:    []string{"elsewhere/stubs"},
			wantErr: "elsewhere/stubs",
		},
		{
			name:    "no stand-in paths configured",
			json:    shopLikeConfigUnix,
			baseDir: "/repo/shop",
			dirs:    nil,
			wantErr: "no external-system stand-in paths configured",
		},
		{
			name: "windows paths: backslashes and drive-letter case normalised",
			json: `{"services": {
				"system": {"build": {"context": "C:\\GitHub\\shop\\system\\monolith\\java"}},
				"external-system-stubs": {"volumes": [{"type": "bind", "source": "c:\\GitHub\\shop\\external-systems\\stubs\\mappings", "target": "/home/wiremock"}]},
				"external-system-simulators": {"build": "C:/GitHub/shop/external-systems/simulators"}
			}}`,
			baseDir: `C:\GitHub\shop`,
			dirs:    []string{"external-systems/stubs", `external-systems\simulators`},
			want:    []string{"external-system-simulators", "external-system-stubs"},
		},
		{
			name:       "relative build context resolves against compose project dir",
			json:       `{"services": {"sim": {"build": {"context": "../../../external-systems/simulators"}}, "system": {"build": {"context": "../../../system"}}}}`,
			composeDir: "/repo/shop/docker/java/monolith",
			baseDir:    "/repo/shop",
			dirs:       []string{"external-systems/simulators"},
			want:       []string{"sim"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := SelectStandInServices([]byte(c.json), c.composeDir, c.baseDir, c.dirs)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestSelectStandInServices_MalformedJSON(t *testing.T) {
	if _, err := SelectStandInServices([]byte("not json"), "", "/repo", []string{"stubs"}); err == nil {
		t.Fatal("expected parse error")
	}
}

// TestUpComposeArgs_ExternalOnly drives the --restart --external-only arg path
// with a substituted compose-config runner (no docker): only stand-ins appear,
// --no-deps is present, the SUT and postgres are absent.
func TestUpComposeArgs_ExternalOnly(t *testing.T) {
	orig := composeConfigJSON
	t.Cleanup(func() { composeConfigJSON = orig })
	composeConfigJSON = func(composeFile, cwd string) ([]byte, error) {
		return []byte(shopLikeConfigUnix), nil
	}

	opts := SystemOptions{
		Restart:        true,
		ExternalOnly:   true,
		StandInPaths:   []string{"external-systems/simulators", "external-systems/stubs"},
		StandInBaseDir: "/repo/shop",
	}
	args, err := upComposeArgs(SystemEntry{Label: "shop", ComposeFile: "docker-compose.yml"}, t.TempDir(), opts)
	if err != nil {
		t.Fatalf("upComposeArgs: %v", err)
	}
	got := strings.Join(args, " ")
	want := "-f docker-compose.yml up -d --build --force-recreate --no-deps external-system-simulators external-system-stubs"
	if got != want {
		t.Errorf("args = %q, want %q", got, want)
	}
	for _, absent := range []string{" system", "postgres"} {
		if strings.HasSuffix(got, absent) || strings.Contains(got, absent+" ") {
			t.Errorf("args must not contain %q: %q", absent, got)
		}
	}
}

func TestUpComposeArgs_ExternalOnly_ConfigErrorSurfaces(t *testing.T) {
	orig := composeConfigJSON
	t.Cleanup(func() { composeConfigJSON = orig })
	composeConfigJSON = func(string, string) ([]byte, error) { return nil, errors.New("daemon down") }

	opts := SystemOptions{Restart: true, ExternalOnly: true, StandInPaths: []string{"stubs"}, StandInBaseDir: "/r"}
	if _, err := upComposeArgs(SystemEntry{ComposeFile: "c.yml"}, t.TempDir(), opts); err == nil || !strings.Contains(err.Error(), "daemon down") {
		t.Fatalf("err = %v, want daemon down", err)
	}
}

func TestSystemOptionsValidate(t *testing.T) {
	cases := []struct {
		name    string
		opts    SystemOptions
		wantErr string
	}{
		{name: "plain start ok", opts: SystemOptions{}},
		{name: "restart ok", opts: SystemOptions{Restart: true}},
		{name: "external-only without restart", opts: SystemOptions{ExternalOnly: true, StandInPaths: []string{"s"}}, wantErr: "requires --restart"},
		{name: "external-only without paths", opts: SystemOptions{Restart: true, ExternalOnly: true}, wantErr: "no external-system stand-in paths"},
		{name: "external-only with restart and paths ok", opts: SystemOptions{Restart: true, ExternalOnly: true, StandInPaths: []string{"s"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.opts.Validate()
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, c.wantErr)
			}
		})
	}
}

// TestUp_ExternalOnlyWithoutRestartFailsBeforeDocker: Up validates options
// before touching any stack.
func TestUp_ExternalOnlyWithoutRestartFailsBeforeDocker(t *testing.T) {
	sys := &SystemConfig{Systems: []SystemEntry{{Label: "x", ComposeFile: "c.yml"}}}
	err := Up(sys, t.TempDir(), SystemOptions{ExternalOnly: true, StandInPaths: []string{"s"}})
	if err == nil || !strings.Contains(err.Error(), "requires --restart") {
		t.Fatalf("err = %v, want requires --restart", err)
	}
}
