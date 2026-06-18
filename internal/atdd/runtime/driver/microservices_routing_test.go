// Driver routing tests for the microservices backend (plan 20260615-1346
// Step 4 / Step 6). A microservices system has N backend services, each
// reachable on its own route/base-location keyed by its service name (the
// `backend-services` map key, D4). These tests assert the driver generalizes
// the single multitier `Backend Route` into one route per service, and that a
// single-backend project (monolith / multitier) keeps resolving exactly one
// route as before.
package driver

import (
	"bytes"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// microservicesRoutingConfig returns a microservices Config with two backend
// services (one java, one dotnet) sharing one mono-repo with distinct paths,
// plus the single typescript frontend — the heterogeneous shape from the plan.
// Only the fields the routing surface reads (architecture + backend-services +
// frontend) are populated; the test exercises backendServiceRoutes / printConfig
// directly, not Validate, so the sonar / paths / system-test scaffolding the
// full config carries is omitted.
func microservicesRoutingConfig() *projectconfig.Config {
	return &projectconfig.Config{
		System: projectconfig.System{
			Architecture: projectconfig.ArchMicroservices,
			BackendServices: map[string]projectconfig.TierSpec{
				"orders": {
					Path: "system/microservices/orders-java",
					Repo: "optivem/shop",
					Lang: projectconfig.LangJava,
				},
				"inventory": {
					Path: "system/microservices/inventory-dotnet",
					Repo: "optivem/shop",
					Lang: projectconfig.LangDotnet,
				},
			},
			Frontend: projectconfig.TierSpec{
				Path: "system/microservices/frontend-react",
				Repo: "optivem/shop",
				Lang: projectconfig.LangTypescript,
			},
		},
	}
}

// TestBackendServiceRoutes_TwoServicesResolveDistinctRoutesByName is the
// ride-along Step 6 test: two declared services resolve to two distinct
// routes/base-locations keyed by their service names.
func TestBackendServiceRoutes_TwoServicesResolveDistinctRoutesByName(t *testing.T) {
	cfg := microservicesRoutingConfig()

	routes := backendServiceRoutes(cfg)

	if len(routes) != 2 {
		t.Fatalf("expected 2 backend routes, got %d: %v", len(routes), routes)
	}

	// Each service is keyed by its own name and carries its own base-location
	// (path/repo) and language — the two routes are distinct.
	orders, ok := routes["orders"]
	if !ok {
		t.Fatalf("expected a route keyed by service name %q; got keys %v", "orders", routes)
	}
	inventory, ok := routes["inventory"]
	if !ok {
		t.Fatalf("expected a route keyed by service name %q; got keys %v", "inventory", routes)
	}

	if orders.Path != "system/microservices/orders-java" {
		t.Errorf("orders route base-location = %q, want %q", orders.Path, "system/microservices/orders-java")
	}
	if inventory.Path != "system/microservices/inventory-dotnet" {
		t.Errorf("inventory route base-location = %q, want %q", inventory.Path, "system/microservices/inventory-dotnet")
	}
	if orders.Path == inventory.Path {
		t.Errorf("the two services resolved to the same base-location %q; routes must be distinct", orders.Path)
	}
	// Heterogeneous: each route carries its own language (D2).
	if orders.Lang != projectconfig.LangJava {
		t.Errorf("orders route lang = %q, want %q", orders.Lang, projectconfig.LangJava)
	}
	if inventory.Lang != projectconfig.LangDotnet {
		t.Errorf("inventory route lang = %q, want %q", inventory.Lang, projectconfig.LangDotnet)
	}
}

// TestBackendServiceRoutes_SingleBackendIsNil pins the degenerate / unchanged
// arm: monolith and multitier have exactly one backend resolved directly off
// System / System.Backend, never through the per-service route map, so
// backendServiceRoutes returns nil for them (and for a nil cfg).
func TestBackendServiceRoutes_SingleBackendIsNil(t *testing.T) {
	cases := []struct {
		name string
		cfg  *projectconfig.Config
	}{
		{"nil-config", nil},
		{"monolith", &projectconfig.Config{System: projectconfig.System{
			Architecture: projectconfig.ArchMonolith,
			Path:         "system/monolith",
			Repo:         "optivem/shop",
			Lang:         projectconfig.LangJava,
		}}},
		{"multitier", &projectconfig.Config{System: projectconfig.System{
			Architecture: projectconfig.ArchMultitier,
			Backend: projectconfig.TierSpec{
				Path: "system/multitier/backend",
				Repo: "optivem/shop",
				Lang: projectconfig.LangJava,
			},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if routes := backendServiceRoutes(tc.cfg); routes != nil {
				t.Errorf("backendServiceRoutes(%s) = %v, want nil (single backend resolves off System/System.Backend)", tc.name, routes)
			}
		})
	}
}

// TestPrintConfig_MicroservicesEmitsPerServiceRoute checks the operator-facing
// banner emits one `backend <name>:` route line per declared service (keyed by
// service name) plus the single frontend — the microservices generalization of
// the single multitier `backend:` line.
func TestPrintConfig_MicroservicesEmitsPerServiceRoute(t *testing.T) {
	cfg := microservicesRoutingConfig()

	var buf bytes.Buffer
	printConfig(&buf, Options{}, cfg, "/repo")
	out := buf.String()

	for _, want := range []string{
		"architecture:  microservices",
		"backend orders: system/microservices/orders-java (lang: java, repo: optivem/shop)",
		"backend inventory: system/microservices/inventory-dotnet (lang: dotnet, repo: optivem/shop)",
		"frontend:      system/microservices/frontend-react (lang: typescript, repo: optivem/shop)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("printConfig output missing %q\n--- full output ---\n%s", want, out)
		}
	}
}
