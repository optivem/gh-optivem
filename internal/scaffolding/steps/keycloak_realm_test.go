package steps

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/optivem/gh-optivem/internal/scaffolding/files"
	"github.com/optivem/gh-optivem/internal/scaffolding/templates"
)

// shopCheckout returns the sibling shop checkout, skipping the test when it is
// absent. The Keycloak tests assert against shop's real compose text, not
// synthetic strings, so a shop-side edit to the mount line fails here first.
func shopCheckout(t *testing.T) string {
	t.Helper()
	shop, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "shop"))
	if err != nil {
		t.Fatalf("resolve shop path: %v", err)
	}
	if !dirExists(filepath.Join(shop, "docker")) {
		t.Skipf("shop checkout not found at %s", shop)
	}
	return shop
}

// scaffoldComposeFromShop mimics the scaffold's docker/ layout for one
// lang/arch: flattened compose files plus the shared docker/keycloak copy.
func scaffoldComposeFromShop(t *testing.T, shop, lang, arch string) string {
	t.Helper()
	repo := t.TempDir()
	vars := map[string]string{"testLang": lang, "arch": arch}
	dockerDst := filepath.Join(repo, Names.TargetDockerDir)
	files.CopyDir(filepath.Join(shop, Expand(Names.ShopDockerDir, vars)), dockerDst)
	if dirExists(filepath.Join(shop, Names.ShopKeycloakDir)) {
		files.CopyDir(filepath.Join(shop, Names.ShopKeycloakDir), filepath.Join(dockerDst, "keycloak"))
	}
	return repo
}

func TestKeycloakRealmMountsResolveAfterRewrite(t *testing.T) {
	shop := shopCheckout(t)
	for _, lang := range []string{"java", "dotnet", "typescript"} {
		for _, arch := range []string{"monolith", "multitier"} {
			t.Run(lang+"/"+arch, func(t *testing.T) {
				if !dirExists(filepath.Join(shop, "docker", lang, arch)) {
					t.Skipf("no shop docker dir for %s/%s", lang, arch)
				}
				repo := scaffoldComposeFromShop(t, shop, lang, arch)

				// Before the rewrite, shop's literal escapes the repo.
				before, err := findKeycloakRealmMountViolations(repo)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(before) == 0 {
					t.Fatalf("shop compose has no Keycloak realm mount; test no longer exercises the guard")
				}

				var pairs [][2]string
				if arch == "monolith" {
					pairs = monolithDockerComposeReplacements(lang, lang)
				} else {
					pairs = multitierDockerComposeReplacements(lang, "react", lang)
				}
				templates.FixupDockerComposeContent(repo, pairs)

				after, err := findKeycloakRealmMountViolations(repo)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(after) != 0 {
					t.Errorf("want no violations after rewrite, got %+v", after)
				}
			})
		}
	}
}

func TestFindKeycloakRealmMountViolationsMissingRealm(t *testing.T) {
	repo := t.TempDir()
	compose := "services:\n  keycloak:\n    volumes:\n      - ./keycloak/shop-realm.json:/opt/keycloak/data/import/shop-realm.json:ro\n"
	writeRepoFile(t, repo, "docker/docker-compose.local.stub.yml", compose)

	violations, err := findKeycloakRealmMountViolations(repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 1 || violations[0].Literal != "./keycloak/shop-realm.json" {
		t.Fatalf("want 1 violation for the missing realm, got %+v", violations)
	}

	if err := os.MkdirAll(filepath.Join(repo, "docker", "keycloak"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, repo, "docker/keycloak/shop-realm.json", "{}")
	violations, err = findKeycloakRealmMountViolations(repo)
	if err != nil || len(violations) != 0 {
		t.Errorf("want clean once the realm exists, got %+v, err %v", violations, err)
	}
}
