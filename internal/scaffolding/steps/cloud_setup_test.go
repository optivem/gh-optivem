package steps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeShopCloudSetup writes shop's cloud setup files (shapes from shop
// meta-v1.0.224) plus the local Terraform state a shop checkout that ran
// setup-gcp.sh holds.
func fakeShopCloudSetup(t *testing.T) string {
	t.Helper()
	shop := t.TempDir()
	for path, body := range map[string]string{
		"setup-gcp.sh": "source \"$(dirname \"$0\")/terraform/neon.sh\"\n" +
			"  PROJECT_ID=\"$PROJECT_ID\" ../setup-custom-domains.sh   # cwd is terraform/\n" +
			"echo \"    - Trigger workflow: gh workflow run multitier-java-acceptance-stage.yml\"\n",
		"teardown-gcp.sh": "source \"$(dirname \"$0\")/terraform/state-backend.sh\"\n",
		"setup-custom-domains.sh": "# Maps the permanent acceptance (stub) and E2E (real) environments of the reference variant\n" +
			"# (multitier-java) to custom domains, and sets the repo variables the acceptance stage reads.\n" +
			"#   - one cloud acceptance run of multitier-java, so the services to map exist\n" +
			"SYSTEM=\"${SYSTEM:-$(gh variable get SYSTEM_NAME)}\"\n" +
			"    echo \"     Run the cloud acceptance stage once (gh workflow run multitier-java-acceptance-stage.yml), then re-run this script.\"\n" +
			"ensure_mapping ACC_DOMAIN \"${ACC_DOMAIN:-}\" \"${SYSTEM}-multitier-java-stub-frontend\"\n" +
			"ensure_mapping E2E_DOMAIN \"${E2E_DOMAIN:-}\" \"${SYSTEM}-multitier-java-real-frontend\"\n",
		"terraform/neon.sh":                        "neon() { :; }\n",
		"terraform/state-backend.sh":               "state() { :; }\n",
		"terraform/main.tf":                        "# <system>-multitier-typescript-real-frontend\n",
		"terraform/README.md":                      "# Terraform\n",
		"terraform/billing-kill-switch/main.py":    "pass\n",
		"terraform/.terraform/providers/x":         "local\n",
		"terraform/.terraform.lock.hcl":            "lock\n",
		"terraform/tfplan":                         "plan\n",
		"terraform/terraform.tfstate":              "{}\n",
		"terraform/terraform.tfstate.backup":       "{}\n",
		"terraform/terraform.tfvars":               "secret = 1\n",
		"terraform/.build/billing-kill-switch.zip": "zip\n",
	} {
		writeTestFile(t, filepath.Join(shop, path), body)
	}
	return shop
}

func TestCopyCloudSetup_Monolith(t *testing.T) {
	shop := fakeShopCloudSetup(t)
	repo := t.TempDir()
	copyCloudSetup(shop, repo, "monolith", "java")

	for _, want := range []string{
		"setup-gcp.sh", "teardown-gcp.sh",
		"terraform/neon.sh", "terraform/state-backend.sh", "terraform/main.tf",
		"terraform/README.md", "terraform/billing-kill-switch/main.py",
	} {
		if !fileExists(filepath.Join(repo, want)) {
			t.Errorf("missing %s", want)
		}
	}
	for _, unwanted := range []string{
		"setup-custom-domains.sh", // multitier only
		"terraform/.terraform", "terraform/.terraform.lock.hcl", "terraform/tfplan",
		"terraform/terraform.tfstate", "terraform/terraform.tfstate.backup",
		"terraform/terraform.tfvars", "terraform/.build",
	} {
		if _, err := os.Stat(filepath.Join(repo, unwanted)); err == nil {
			t.Errorf("unexpected %s", unwanted)
		}
	}
	for _, exe := range []string{"setup-gcp.sh", "teardown-gcp.sh"} {
		info, err := os.Stat(filepath.Join(repo, exe))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o100 == 0 {
			t.Errorf("%s is not executable: %v", exe, info.Mode())
		}
	}
	setup, _ := os.ReadFile(filepath.Join(repo, "setup-gcp.sh"))
	if !strings.Contains(string(setup), "gh workflow run acceptance-stage.yml") ||
		strings.Contains(string(setup), "multitier-java") {
		t.Errorf("setup-gcp.sh trigger hint not rewritten:\n%s", setup)
	}
}

func TestCopyCloudSetup_MultitierCustomDomains(t *testing.T) {
	for _, lang := range []string{"java", "dotnet", "typescript"} {
		t.Run(lang, func(t *testing.T) {
			shop := fakeShopCloudSetup(t)
			repo := t.TempDir()
			copyCloudSetup(shop, repo, "multitier", lang)

			path := filepath.Join(repo, "setup-custom-domains.sh")
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("setup-custom-domains.sh not scaffolded: %v", err)
			}
			// Shop has it at 0644, but setup-gcp.sh executes it directly.
			if info.Mode().Perm()&0o100 == 0 {
				t.Errorf("setup-custom-domains.sh is not executable: %v", info.Mode())
			}
			data, _ := os.ReadFile(path)
			s := string(data)
			for _, want := range []string{
				`PREFIX="multitier-` + lang + `"`,
				`"${SYSTEM}-${PREFIX}-stub-frontend"`,
				`"${SYSTEM}-${PREFIX}-real-frontend"`,
				"gh workflow run acceptance-stage.yml",
				"environments of this system to custom domains",
				"one cloud acceptance run (acceptance-stage.yml)",
			} {
				if !strings.Contains(s, want) {
					t.Errorf("setup-custom-domains.sh lacks %q:\n%s", want, s)
				}
			}
			// No shop flavor left, and no "<arch>-<lang>-" literal the
			// leftover-template-ref guard forbids.
			leftovers := []string{"multitier-" + lang + "-", "multitier-java-acceptance-stage"}
			if lang != "java" {
				leftovers = append(leftovers, "multitier-java")
			}
			for _, leftover := range leftovers {
				if strings.Contains(s, leftover) {
					t.Errorf("setup-custom-domains.sh still contains %q:\n%s", leftover, s)
				}
			}
		})
	}
}

func TestIsCloudSetupPath(t *testing.T) {
	for rel, want := range map[string]bool{
		"terraform/main.tf":       true,
		"terraform/README.md":     true,
		"setup-gcp.sh":            true,
		"teardown-gcp.sh":         true,
		"setup-custom-domains.sh": true,
		"backend/setup-gcp.sh":    false,
		"docs/terraform.md":       false,
		".github/workflows/x.yml": false,
	} {
		if got := isCloudSetupPath(rel); got != want {
			t.Errorf("isCloudSetupPath(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestDerivedSystemName(t *testing.T) {
	// Mirrors shop terraform/main.tf:7.
	for repo, want := range map[string]string{
		"shop":                   "shop",
		"My_Shop.App":            "my-shop-app",
		"my-shop-multitier-java": "my-shop-multit",
		"abcdefghijklm-xyz":      "abcdefghijklm",
		"-abc-":                  "abc",
	} {
		if got := derivedSystemName(repo); got != want {
			t.Errorf("derivedSystemName(%q) = %q, want %q", repo, got, want)
		}
	}
}

func TestCloudRunMaxSystemNameLen(t *testing.T) {
	// shop setup-gcp.sh:6-7: 8 for multitier-typescript, 12 for multitier-dotnet.
	for lang, want := range map[string]int{"typescript": 8, "dotnet": 12, "java": 14} {
		if got := cloudRunMaxSystemNameLen(lang); got != want {
			t.Errorf("cloudRunMaxSystemNameLen(%q) = %d, want %d", lang, got, want)
		}
	}
}

func TestCloudSystemNameWarning(t *testing.T) {
	const long = "my-shop-multitier-x" // derives "my-shop-multit" (14 chars)
	for _, tc := range []struct {
		repo, arch, lang string
		warn             bool
	}{
		{long, "monolith", "", false},
		{long, "multitier", "java", false},
		{long, "multitier", "dotnet", true},
		{long, "multitier", "typescript", true},
		{"myshop12", "multitier", "typescript", false},
		{"myshop123", "multitier", "typescript", true},
		{"myshop123", "multitier", "dotnet", false},
		{"my-shop-1234", "multitier", "dotnet", false},
		{"my-shop-12345", "multitier", "dotnet", true},
	} {
		msg := cloudSystemNameWarning(tc.repo, tc.arch, tc.lang)
		if (msg != "") != tc.warn {
			t.Errorf("%s %s-%s: warn = %v, want %v (%s)", tc.repo, tc.arch, tc.lang, msg != "", tc.warn, msg)
		}
	}
	msg := cloudSystemNameWarning(long, "multitier", "typescript")
	for _, want := range []string{
		`"my-shop-multit"`,
		"longer than 8 characters",
		"SYSTEM_NAME=<short> ./setup-gcp.sh",
		"https://github.com/optivem/gh-optivem/blob/main/docs/cloud-setup.md",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("warning lacks %q:\n%s", want, msg)
		}
	}
}

// TestScaffoldEnvironmentRefs checks the environment-reference check the
// shop-checkout test applies to scaffolded workflows.
func TestScaffoldEnvironmentRefs(t *testing.T) {
	ok := strings.Join([]string{
		"    environment: acceptance",
		"    environment: ${{ inputs.stage }}",
		"          environment: qa",
		"  #         environment: monolith-java-qa", // comment: ignored
	}, "\n")
	if bad := unknownEnvironmentRefs(ok); len(bad) != 0 {
		t.Errorf("unexpected unknown refs: %v", bad)
	}
	bad := unknownEnvironmentRefs("    environment: ${{ inputs.architecture }}-${{ inputs.language }}-acceptance\n      environment: multitier-java-qa\n")
	if len(bad) != 2 {
		t.Errorf("want 2 unknown refs, got %v", bad)
	}
}
