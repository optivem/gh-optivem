package main

import (
	"reflect"
	"testing"

	"github.com/optivem/gh-optivem/internal/build/componenttest"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

func TestDiscoverComponents_Monolith(t *testing.T) {
	cfg := &projectconfig.Config{System: projectconfig.System{
		Architecture: "monolith",
		Path:         "system/monolith/java",
		Lang:         "java",
	}}
	got := discoverComponents(cfg)
	want := []componenttest.Component{{Name: "monolith", Path: "system/monolith/java", Lang: "java"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("monolith discovery = %+v, want %+v", got, want)
	}
}

func TestDiscoverComponents_Multitier(t *testing.T) {
	cfg := &projectconfig.Config{System: projectconfig.System{
		Architecture: "multitier",
		Backend:      projectconfig.TierSpec{Path: "system/multitier/backend-java", Lang: "java"},
		Frontend:     projectconfig.TierSpec{Path: "system/multitier/frontend-react", Lang: "typescript"},
	}}
	got := discoverComponents(cfg)
	want := []componenttest.Component{
		{Name: "backend", Path: "system/multitier/backend-java", Lang: "java"},
		{Name: "frontend", Path: "system/multitier/frontend-react", Lang: "typescript"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("multitier discovery = %+v, want %+v", got, want)
	}
}

func TestDiscoverComponents_Microservices(t *testing.T) {
	cfg := &projectconfig.Config{System: projectconfig.System{
		Architecture: "microservices",
		Frontend:     projectconfig.TierSpec{Path: "fe", Lang: "typescript"},
		BackendServices: map[string]projectconfig.TierSpec{
			"orders":  {Path: "svc/orders", Lang: "java"},
			"billing": {Path: "svc/billing", Lang: "dotnet"},
		},
	}}
	got := discoverComponents(cfg)
	// Backend services are emitted in sorted name order, then the frontend.
	want := []componenttest.Component{
		{Name: "billing", Path: "svc/billing", Lang: "dotnet"},
		{Name: "orders", Path: "svc/orders", Lang: "java"},
		{Name: "frontend", Path: "fe", Lang: "typescript"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("microservices discovery = %+v, want %+v", got, want)
	}
}
