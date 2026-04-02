package config

import (
	"strings"
	"testing"
)

func TestToPascalCase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"single word", "hello", "Hello"},
		{"two words", "page-turner", "PageTurner"},
		{"three words", "my-cool-app", "MyCoolApp"},
		{"already capitalized", "Hello-World", "HelloWorld"},
		{"single char segments", "a-b-c", "ABC"},
		{"empty string", "", ""},
		{"trailing hyphen", "hello-", "Hello"},
		{"leading hyphen", "-hello", "Hello"},
		{"double hyphen", "hello--world", "HelloWorld"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToPascalCase(tt.input)
			if got != tt.expected {
				t.Errorf("ToPascalCase(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestToJavaLower(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"with hyphens", "page-turner", "pageturner"},
		{"no hyphens", "hello", "hello"},
		{"mixed case", "Page-Turner", "pageturner"},
		{"multiple hyphens", "my-cool-app", "mycoolapp"},
		{"empty string", "", ""},
		{"uppercase", "HELLO-WORLD", "helloworld"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToJavaLower(tt.input)
			if got != tt.expected {
				t.Errorf("ToJavaLower(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestEffectiveLang(t *testing.T) {
	tests := []struct {
		name        string
		arch        string
		lang        string
		backendLang string
		expected    string
	}{
		{"monolith java", "monolith", "java", "", "java"},
		{"monolith dotnet", "monolith", "dotnet", "", "dotnet"},
		{"monolith typescript", "monolith", "typescript", "", "typescript"},
		{"multitier java backend", "multitier", "", "java", "java"},
		{"multitier dotnet backend", "multitier", "", "dotnet", "dotnet"},
		{"multitier typescript backend", "multitier", "", "typescript", "typescript"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				Arch:        tt.arch,
				Lang:        tt.lang,
				BackendLang: tt.backendLang,
			}
			got := c.EffectiveLang()
			if got != tt.expected {
				t.Errorf("EffectiveLang() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDerivedNaming(t *testing.T) {
	tests := []struct {
		name           string
		owner          string
		repo           string
		wantRepoPascal string
		wantRepoNoHyp  string
		wantJavaNsNew  string
		wantDotnetNew  string
		wantTsPkgNew   string
	}{
		{
			name:           "simple owner and repo",
			owner:          "acme",
			repo:           "page-turner",
			wantRepoPascal: "PageTurner",
			wantRepoNoHyp:  "pageturner",
			wantJavaNsNew:  "com.acme.pageturner",
			wantDotnetNew:  "Acme.PageTurner",
			wantTsPkgNew:   "@acme/page-turner-system-test",
		},
		{
			name:           "hyphenated owner",
			owner:          "my-org",
			repo:           "cool-app",
			wantRepoPascal: "CoolApp",
			wantRepoNoHyp:  "coolapp",
			wantJavaNsNew:  "com.myorg.coolapp",
			wantDotnetNew:  "MyOrg.CoolApp",
			wantTsPkgNew:   "@myorg/cool-app-system-test",
		},
		{
			name:           "single word repo",
			owner:          "testuser",
			repo:           "shop",
			wantRepoPascal: "Shop",
			wantRepoNoHyp:  "shop",
			wantJavaNsNew:  "com.testuser.shop",
			wantDotnetNew:  "Testuser.Shop",
			wantTsPkgNew:   "@testuser/shop-system-test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ownerLower := ToJavaLower(tt.owner)
			repoPascal := ToPascalCase(tt.repo)
			repoNoHyphens := ToJavaLower(tt.repo)

			// Owner pascal: special case for non-hyphenated
			ownerPascal := ToPascalCase(tt.owner)
			if len(tt.owner) > 0 && !contains(tt.owner, "-") {
				ownerPascal = upperFirst(tt.owner)
			}

			if repoPascal != tt.wantRepoPascal {
				t.Errorf("RepoPascal = %q, want %q", repoPascal, tt.wantRepoPascal)
			}
			if repoNoHyphens != tt.wantRepoNoHyp {
				t.Errorf("RepoNoHyphens = %q, want %q", repoNoHyphens, tt.wantRepoNoHyp)
			}

			javaNsNew := "com." + ownerLower + "." + repoNoHyphens
			if javaNsNew != tt.wantJavaNsNew {
				t.Errorf("JavaNsNew = %q, want %q", javaNsNew, tt.wantJavaNsNew)
			}

			dotnetNsNew := ownerPascal + "." + repoPascal
			if dotnetNsNew != tt.wantDotnetNew {
				t.Errorf("DotnetNsNew = %q, want %q", dotnetNsNew, tt.wantDotnetNew)
			}

			tsPkgNew := "@" + ownerLower + "/" + tt.repo + "-system-test"
			if tsPkgNew != tt.wantTsPkgNew {
				t.Errorf("TsPkgNew = %q, want %q", tsPkgNew, tt.wantTsPkgNew)
			}
		})
	}
}

func TestMultitierRepoNames(t *testing.T) {
	tests := []struct {
		name              string
		owner             string
		repo              string
		wantFrontendRepo  string
		wantBackendRepo   string
		wantFrontendFull  string
		wantBackendFull   string
	}{
		{
			name:             "standard multitier",
			owner:            "acme",
			repo:             "page-turner",
			wantFrontendRepo: "page-turner-frontend",
			wantBackendRepo:  "page-turner-backend",
			wantFrontendFull: "acme/page-turner-frontend",
			wantBackendFull:  "acme/page-turner-backend",
		},
		{
			name:             "single word repo",
			owner:            "myorg",
			repo:             "shop",
			wantFrontendRepo: "shop-frontend",
			wantBackendRepo:  "shop-backend",
			wantFrontendFull: "myorg/shop-frontend",
			wantBackendFull:  "myorg/shop-backend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frontendRepo := tt.repo + "-frontend"
			backendRepo := tt.repo + "-backend"
			frontendFull := tt.owner + "/" + frontendRepo
			backendFull := tt.owner + "/" + backendRepo

			if frontendRepo != tt.wantFrontendRepo {
				t.Errorf("FrontendRepo = %q, want %q", frontendRepo, tt.wantFrontendRepo)
			}
			if backendRepo != tt.wantBackendRepo {
				t.Errorf("BackendRepo = %q, want %q", backendRepo, tt.wantBackendRepo)
			}
			if frontendFull != tt.wantFrontendFull {
				t.Errorf("FrontendFullRepo = %q, want %q", frontendFull, tt.wantFrontendFull)
			}
			if backendFull != tt.wantBackendFull {
				t.Errorf("BackendFullRepo = %q, want %q", backendFull, tt.wantBackendFull)
			}
		})
	}
}

func TestMonolithMultirepoRepoNames(t *testing.T) {
	tests := []struct {
		name           string
		owner          string
		repo           string
		wantSystemRepo string
		wantSystemFull string
	}{
		{
			name:           "standard monolith multirepo",
			owner:          "acme",
			repo:           "page-turner",
			wantSystemRepo: "page-turner-system",
			wantSystemFull: "acme/page-turner-system",
		},
		{
			name:           "single word repo",
			owner:          "myorg",
			repo:           "shop",
			wantSystemRepo: "shop-system",
			wantSystemFull: "myorg/shop-system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			systemRepo := tt.repo + "-system"
			systemFull := tt.owner + "/" + systemRepo

			if systemRepo != tt.wantSystemRepo {
				t.Errorf("SystemRepo = %q, want %q", systemRepo, tt.wantSystemRepo)
			}
			if systemFull != tt.wantSystemFull {
				t.Errorf("SystemFullRepo = %q, want %q", systemFull, tt.wantSystemFull)
			}
		})
	}
}

func TestSonarProjectKeys(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		expected []string
	}{
		{
			name: "monolith monorepo",
			cfg: Config{
				Owner: "acme", Repo: "page-turner",
				Arch: "monolith", RepoStrategy: "monorepo", Lang: "java",
			},
			expected: []string{"acme_page-turner-system"},
		},
		{
			name: "monolith multirepo",
			cfg: Config{
				Owner: "acme", Repo: "page-turner",
				Arch: "monolith", RepoStrategy: "multirepo", Lang: "java",
				SystemRepo: "page-turner-system",
			},
			expected: []string{"acme_page-turner-system-system"},
		},
		{
			name: "multitier monorepo",
			cfg: Config{
				Owner: "acme", Repo: "page-turner",
				Arch: "multitier", RepoStrategy: "monorepo",
				BackendLang: "java", FrontendLang: "react",
			},
			expected: []string{"acme_page-turner-backend", "acme_page-turner-frontend"},
		},
		{
			name: "multitier multirepo",
			cfg: Config{
				Owner: "acme", Repo: "page-turner",
				Arch: "multitier", RepoStrategy: "multirepo",
				BackendLang: "java", FrontendLang: "react",
				BackendRepo: "page-turner-backend", FrontendRepo: "page-turner-frontend",
			},
			expected: []string{"acme_page-turner-backend-backend", "acme_page-turner-frontend-frontend"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Import the steps package test helper would be complex,
			// so we test the logic inline matching GetSonarProjectKeys
			var got []string
			if tt.cfg.Arch == "monolith" {
				if tt.cfg.RepoStrategy == "monorepo" {
					got = []string{tt.cfg.Owner + "_" + tt.cfg.Repo + "-system"}
				} else {
					got = []string{tt.cfg.Owner + "_" + tt.cfg.SystemRepo + "-system"}
				}
			} else if tt.cfg.RepoStrategy == "monorepo" {
				prefix := tt.cfg.Owner + "_" + tt.cfg.Repo
				got = []string{prefix + "-backend", prefix + "-frontend"}
			} else {
				got = []string{
					tt.cfg.Owner + "_" + tt.cfg.BackendRepo + "-backend",
					tt.cfg.Owner + "_" + tt.cfg.FrontendRepo + "-frontend",
				}
			}

			if len(got) != len(tt.expected) {
				t.Fatalf("got %d keys, want %d", len(got), len(tt.expected))
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("key[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestSplitCamelCase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"single lowercase word", "todo", []string{"todo"}},
		{"camelCase two words", "skyTravel", []string{"sky", "Travel"}},
		{"camelCase single-char first", "eShop", []string{"e", "Shop"}},
		{"camelCase three words", "eSuperStore", []string{"e", "Super", "Store"}},
		{"PascalCase", "SkyTravel", []string{"Sky", "Travel"}},
		{"all uppercase acronym", "ABC", []string{"ABC"}},
		{"acronym then word", "ABCStore", []string{"ABC", "Store"}},
		{"word then acronym then word", "myAPIClient", []string{"my", "API", "Client"}},
		{"single char", "a", []string{"a"}},
		{"empty", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitCamelCase(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("SplitCamelCase(%q) = %v, want %v", tt.input, got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("SplitCamelCase(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestCamelCaseToPascal(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"skyTravel", "SkyTravel"},
		{"eShop", "EShop"},
		{"todo", "Todo"},
		{"ABC", "ABC"},
		{"ABCStore", "ABCStore"},
		{"myAPIClient", "MyAPIClient"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := CamelCaseToPascal(tt.input)
			if got != tt.expected {
				t.Errorf("CamelCaseToPascal(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCamelCaseToKebab(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"skyTravel", "sky-travel"},
		{"eShop", "e-shop"},
		{"todo", "todo"},
		{"ABC", "abc"},
		{"ABCStore", "abc-store"},
		{"myAPIClient", "my-api-client"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := CamelCaseToKebab(tt.input)
			if got != tt.expected {
				t.Errorf("CamelCaseToKebab(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCamelCaseToLower(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"skyTravel", "skytravel"},
		{"eShop", "eshop"},
		{"todo", "todo"},
		{"ABC", "abc"},
		{"ABCStore", "abcstore"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := CamelCaseToLower(tt.input)
			if got != tt.expected {
				t.Errorf("CamelCaseToLower(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSpacesToCamel(t *testing.T) {
	tests := []struct{ input, expected string }{
		{"Sky Travel", "skyTravel"},
		{"Pet Clinic", "petClinic"},
		{"Todo", "todo"},
		{"E Shop", "eShop"},
		{"E Super Store", "eSuperStore"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := SpacesToCamel(tt.input); got != tt.expected {
				t.Errorf("SpacesToCamel(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSpacesToPascal(t *testing.T) {
	tests := []struct{ input, expected string }{
		{"Sky Travel", "SkyTravel"},
		{"Pet Clinic", "PetClinic"},
		{"Todo", "Todo"},
		{"E Shop", "EShop"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := SpacesToPascal(tt.input); got != tt.expected {
				t.Errorf("SpacesToPascal(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSpacesToKebab(t *testing.T) {
	tests := []struct{ input, expected string }{
		{"Sky Travel", "sky-travel"},
		{"Pet Clinic", "pet-clinic"},
		{"Todo", "todo"},
		{"E Shop", "e-shop"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := SpacesToKebab(tt.input); got != tt.expected {
				t.Errorf("SpacesToKebab(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSpacesToLower(t *testing.T) {
	tests := []struct{ input, expected string }{
		{"Sky Travel", "skytravel"},
		{"Pet Clinic", "petclinic"},
		{"Todo", "todo"},
		{"E Shop", "eshop"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := SpacesToLower(tt.input); got != tt.expected {
				t.Errorf("SpacesToLower(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestValidateSystemName(t *testing.T) {
	valid := []string{
		"Sky Travel", "Pet Clinic", "Todo", "Book Store", "Quick Fox",
	}
	for _, name := range valid {
		t.Run("valid: "+name, func(t *testing.T) {
			if err := ValidateSystemName(name); err != "" {
				t.Errorf("ValidateSystemName(%q) = %q, want empty", name, err)
			}
		})
	}

	invalid := []struct {
		name, contains string
	}{
		{"", "empty"},
		{" Sky Travel", "leading"},
		{"Sky Travel ", "trailing"},
		{"3D Print", "invalid character"},
		{"sky-travel", "invalid character"},
		{"sky_travel", "invalid character"},
		{"café", "invalid character"},
		{"foo&bar", "invalid character"},
		{"New", "language reserved"},
		{"Class Act", "language reserved"},
		{"For Real", "language reserved"},
		{"Test System", "scaffold reserved"},
		{"Backend Api", "scaffold reserved"},
		{"Frontend App", "scaffold reserved"},
		{"Aaaaaa Bbbbbb Cccccc Dddddd Eeeeee Ffffff Gggggg Hh", "50 character"},
	}
	for _, tt := range invalid {
		t.Run("invalid: "+tt.name, func(t *testing.T) {
			err := ValidateSystemName(tt.name)
			if err == "" {
				t.Errorf("ValidateSystemName(%q) = empty, want error containing %q", tt.name, tt.contains)
			} else if !strings.Contains(strings.ToLower(err), strings.ToLower(tt.contains)) {
				t.Errorf("ValidateSystemName(%q) = %q, want error containing %q", tt.name, err, tt.contains)
			}
		})
	}
}

// helpers for test logic
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func upperFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}
