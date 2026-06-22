package projectconfig

import (
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Microservices: backend-services map (plan 20260615-1346)
//
// Reuses the writeConfig / javaPaths helpers defined in config_test.go (same
// package). Covers the config-model + validation seam (Steps 1-2): the
// name-keyed backend-services map, architecture exclusivity, per-service
// completeness, duplicate-location rejection, and per-service sonar-project.
// ---------------------------------------------------------------------------

// sampleMonoRepoMicroservices is the heterogeneous mono-repo shape from the
// plan: two backend services (one java, one dotnet) sharing one repo with
// distinct paths, plus the single typescript frontend.
const sampleMonoRepoMicroservices = `project:
  provider: github
  url: https://github.com/orgs/optivem/projects/20

repo-strategy: mono-repo

sonar:
  organization: optivem

system:
  architecture: microservices
  backend-services:
    orders:
      path: system/microservices/orders-java
      repo: optivem/shop
      lang: java
      sonar-project: optivem_shop-orders
    inventory:
      path: system/microservices/inventory-dotnet
      repo: optivem/shop
      lang: dotnet
      sonar-project: optivem_shop-inventory
  frontend:
    path: system/microservices/frontend-react
    repo: optivem/shop
    lang: typescript
    sonar-project: optivem_shop-frontend
  db-migration-path: system/db/migrations

system-test:
  path: system-test/java
  repo: optivem/shop
  lang: java
  sonar-project: optivem_shop-system-test
  paths:
    system-driver-port: system-test/java/src/main/java/testkit/driver/port/shop
    system-driver-adapter: system-test/java/src/main/java/testkit/driver/adapter/shop
    external-system-driver-port: system-test/java/src/main/java/testkit/external/port/shop
    external-system-driver-adapter: system-test/java/src/main/java/testkit/external/adapter/shop
    at-test: system-test/java/src/test/java/shop/latest/acceptance
    dsl-port: system-test/java/src/main/java/testkit/dsl/port/shop
    dsl-core: system-test/java/src/main/java/testkit/dsl/core/shop
    ct-test: system-test/java/src/test/java/shop/latest/contract
    system-driver-adapter-shared: system-test/java/src/main/java/testkit/driver/adapter/shop/shared
    common: system-test/java/src/main/java/testkit/common/shop
    domain-value-types: system-test/java/src/main/java/testkit/domainvaluetypes/shop
`

// validMicroservicesConfig returns a Config that passes Validate — the
// mono-repo heterogeneous shape above, as a struct. Negative tests mutate a
// fresh copy so each starts from a known-good baseline (mirrors how the
// arch-exclusivity tests build structs, but full so the late rules — sonar,
// paths — are reachable).
func validMicroservicesConfig() *Config {
	return &Config{
		Project:      Project{Provider: ProviderGitHub, URL: "https://github.com/orgs/optivem/projects/20"},
		RepoStrategy: RepoStrategyMonoRepo,
		Sonar:        Sonar{Organization: "optivem"},
		System: System{
			Architecture: ArchMicroservices,
			BackendServices: map[string]TierSpec{
				"orders":    {Path: "system/microservices/orders-java", Repo: "optivem/shop", Lang: LangJava, SonarProject: "optivem_shop-orders"},
				"inventory": {Path: "system/microservices/inventory-dotnet", Repo: "optivem/shop", Lang: LangDotnet, SonarProject: "optivem_shop-inventory"},
			},
			Frontend:        TierSpec{Path: "system/microservices/frontend-react", Repo: "optivem/shop", Lang: LangTypescript, SonarProject: "optivem_shop-frontend"},
			DbMigrationPath: "system/db/migrations",
		},
		SystemTest: TierSpec{
			Path: "system-test/java", Repo: "optivem/shop", Lang: LangJava,
			SonarProject: "optivem_shop-system-test",
			Paths:        javaPaths("system-test/java", "shop"),
		},
	}
}

func TestLoad_MicroservicesSampleValidates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, sampleMonoRepoMicroservices)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("microservices sample should validate, got: %v", err)
	}
	if got := cfg.BackendServiceNames(); len(got) != 2 || got[0] != "inventory" || got[1] != "orders" {
		t.Fatalf("BackendServiceNames: want sorted [inventory orders], got %v", got)
	}
	if got := cfg.Repos(); len(got) != 1 || got[0] != "optivem/shop" {
		t.Fatalf("Repos: want [optivem/shop], got %v", got)
	}
}

func TestRoundTrip_PreservesBackendServices(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeConfig(t, dir, sampleMonoRepoMicroservices)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	out := t.TempDir()
	if err := Write(out, cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Load(out)
	if err != nil {
		t.Fatalf("Load after Write: %v", err)
	}
	if got.System.Architecture != ArchMicroservices ||
		!reflect.DeepEqual(got.System.BackendServices, cfg.System.BackendServices) ||
		!reflect.DeepEqual(got.System.Frontend, cfg.System.Frontend) {
		t.Fatalf("round-trip mismatch:\n got:  %+v\n want: %+v", got.System, cfg.System)
	}
}

func TestValidate_MicroservicesBaselineStructValidates(t *testing.T) {
	t.Parallel()
	if err := validMicroservicesConfig().Validate(); err != nil {
		t.Fatalf("baseline microservices config should validate, got: %v", err)
	}
}

func TestValidate_MicroservicesRequiresAtLeastOneService(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project: Project{Provider: ProviderGitHub},
		System: System{
			Architecture: ArchMicroservices,
			Frontend:     TierSpec{Path: "fe", Repo: "x/y", Lang: LangTypescript},
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "backend-services") {
		t.Fatalf("expected at-least-one-service error, got: %v", err)
	}
}

func TestValidate_MicroservicesRejectsMonolithFields(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project: Project{Provider: ProviderGitHub},
		System: System{
			Architecture: ArchMicroservices,
			Path:         "should-not-be-here",
			BackendServices: map[string]TierSpec{
				"orders": {Path: "be", Repo: "x/y", Lang: LangJava},
			},
			Frontend: TierSpec{Path: "fe", Repo: "x/y", Lang: LangTypescript},
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for microservices with system.path set, got nil")
	}
}

func TestValidate_MicroservicesRejectsSingularBackend(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project: Project{Provider: ProviderGitHub},
		System: System{
			Architecture: ArchMicroservices,
			Backend:      TierSpec{Path: "be", Repo: "x/y", Lang: LangJava},
			Frontend:     TierSpec{Path: "fe", Repo: "x/y", Lang: LangTypescript},
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "system.backend") {
		t.Fatalf("expected singular-backend rejection, got: %v", err)
	}
}

func TestValidate_MicroservicesRequiresFrontend(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project: Project{Provider: ProviderGitHub},
		System: System{
			Architecture: ArchMicroservices,
			BackendServices: map[string]TierSpec{
				"orders": {Path: "be", Repo: "x/y", Lang: LangJava},
			},
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "frontend") {
		t.Fatalf("expected requires-frontend error, got: %v", err)
	}
}

func TestValidate_MonolithRejectsBackendServices(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project: Project{Provider: ProviderGitHub},
		System: System{
			Architecture: ArchMonolith,
			Path:         "p", Repo: "x/y", Lang: LangJava,
			BackendServices: map[string]TierSpec{
				"orders": {Path: "be", Repo: "x/y", Lang: LangJava},
			},
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "backend-services") {
		t.Fatalf("expected monolith-rejects-backend-services error, got: %v", err)
	}
}

func TestValidate_MultitierRejectsBackendServices(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project: Project{Provider: ProviderGitHub},
		System: System{
			Architecture: ArchMultitier,
			Backend:      TierSpec{Path: "be", Repo: "x/y", Lang: LangJava},
			Frontend:     TierSpec{Path: "fe", Repo: "x/y", Lang: LangTypescript},
			BackendServices: map[string]TierSpec{
				"orders": {Path: "svc", Repo: "x/y", Lang: LangJava},
			},
		},
		SystemTest: TierSpec{Path: "t", Repo: "x/y", Lang: LangJava},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "backend-services") {
		t.Fatalf("expected multitier-rejects-backend-services error, got: %v", err)
	}
}

func TestValidate_MicroservicesRejectsDuplicateServiceLocation(t *testing.T) {
	t.Parallel()
	cfg := validMicroservicesConfig()
	// Point inventory at orders' exact repo+path — a genuine collision.
	cfg.System.BackendServices["inventory"] = TierSpec{
		Path: "system/microservices/orders-java", Repo: "optivem/shop",
		Lang: LangDotnet, SonarProject: "optivem_shop-inventory",
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "same location") {
		t.Fatalf("expected duplicate-location rejection, got: %v", err)
	}
}

func TestValidate_MicroservicesAcceptsMultiRepoSharedPath(t *testing.T) {
	t.Parallel()
	// Multi-repo: every service lives at path "." in its own repo. Same path,
	// distinct repos — legitimate, must NOT trip the duplicate-location rule.
	cfg := validMicroservicesConfig()
	cfg.RepoStrategy = RepoStrategyMultiRepo
	cfg.System.BackendServices = map[string]TierSpec{
		"orders":    {Path: ".", Repo: "optivem/shop-orders", Lang: LangJava, SonarProject: "optivem_shop-orders"},
		"inventory": {Path: ".", Repo: "optivem/shop-inventory", Lang: LangDotnet, SonarProject: "optivem_shop-inventory"},
	}
	cfg.System.Frontend = TierSpec{Path: ".", Repo: "optivem/shop-frontend", Lang: LangTypescript, SonarProject: "optivem_shop-frontend"}
	cfg.SystemTest.Repo = "optivem/shop-orders"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("multi-repo shared path across distinct repos should validate, got: %v", err)
	}
}

func TestValidate_MicroservicesRejectsServiceMissingLang(t *testing.T) {
	t.Parallel()
	cfg := validMicroservicesConfig()
	cfg.System.BackendServices["orders"] = TierSpec{
		Path: "system/microservices/orders-java", Repo: "optivem/shop",
		SonarProject: "optivem_shop-orders", // lang missing
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for service missing lang, got nil")
	}
}

func TestValidate_MicroservicesRequiresServiceSonarProject(t *testing.T) {
	t.Parallel()
	cfg := validMicroservicesConfig()
	cfg.System.BackendServices["orders"] = TierSpec{
		Path: "system/microservices/orders-java", Repo: "optivem/shop", Lang: LangJava,
		// sonar-project missing
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "sonar-project") {
		t.Fatalf("expected per-service sonar-project requirement, got: %v", err)
	}
}

func TestRepos_IncludesBackendServiceRepos(t *testing.T) {
	t.Parallel()
	cfg := validMicroservicesConfig()
	cfg.RepoStrategy = RepoStrategyMultiRepo
	cfg.System.BackendServices = map[string]TierSpec{
		"orders":    {Path: ".", Repo: "x/orders", Lang: LangJava, SonarProject: "p-orders"},
		"inventory": {Path: ".", Repo: "x/inventory", Lang: LangDotnet, SonarProject: "p-inventory"},
	}
	cfg.System.Frontend = TierSpec{Path: ".", Repo: "x/frontend", Lang: LangTypescript, SonarProject: "p-frontend"}
	cfg.SystemTest.Repo = "x/orders"
	got := cfg.Repos()
	want := []string{"x/frontend", "x/inventory", "x/orders"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Repos: got %v, want %v", got, want)
	}
}

func TestBackendServiceNames_SortedAndNilSafe(t *testing.T) {
	t.Parallel()
	var nilCfg *Config
	if got := nilCfg.BackendServiceNames(); got != nil {
		t.Errorf("nil cfg.BackendServiceNames() should be nil, got %v", got)
	}
	cfg := &Config{System: System{BackendServices: map[string]TierSpec{
		"orders": {}, "inventory": {}, "shipping": {},
	}}}
	got := cfg.BackendServiceNames()
	want := []string{"inventory", "orders", "shipping"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BackendServiceNames: got %v, want sorted %v", got, want)
	}
}

func TestValidate_MicroservicesArchInEnumErrorMessage(t *testing.T) {
	t.Parallel()
	cfg := &Config{System: System{Architecture: "serverless"}}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), ArchMicroservices) {
		t.Fatalf("unknown-arch error should list microservices, got: %v", err)
	}
}
