package runner

import (
	"os"
	"testing"
)

func TestExportKeycloakURLs(t *testing.T) {
	t.Setenv("KEYCLOAK_URL_STUB", "")
	t.Setenv("KEYCLOAK_URL_REAL", "http://override:1")
	sys := &SystemConfig{Systems: []SystemEntry{
		{Label: "stub", Components: []Component{
			{Name: "Backend API", URL: "http://localhost:8312/health"},
			{Name: "Keycloak", URL: "http://localhost:8392/realms/shop"},
		}},
		{Label: "real", Components: []Component{
			{Name: "Keycloak", URL: "http://localhost:8391/realms/shop"},
		}},
	}}

	exportKeycloakURLs(sys)

	if got := os.Getenv("KEYCLOAK_URL_STUB"); got != "http://localhost:8392" {
		t.Errorf("KEYCLOAK_URL_STUB = %q, want http://localhost:8392", got)
	}
	if got := os.Getenv("KEYCLOAK_URL_REAL"); got != "http://override:1" {
		t.Errorf("KEYCLOAK_URL_REAL = %q, want the pre-set override to win", got)
	}
}
