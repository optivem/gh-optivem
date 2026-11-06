package steps

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/kernel/log"
)

// Cloud setup: every scaffold gets shop's GCP setup and teardown scripts and
// the terraform/ directory they source (setup-gcp.sh sources terraform/neon.sh
// and terraform/state-backend.sh, then runs Terraform from terraform/). The
// cloud acceptance, QA and production stages are manual-dispatch only and
// fail fast without the GCP repo variables, so shipping the scripts costs
// nothing until the user runs them (gh-optivem docs/cloud-setup.md).

const (
	infoCopyingCloudSetup = "Copying cloud setup scripts and terraform/..."

	scriptSetupGCP           = "setup-gcp.sh"
	scriptTeardownGCP        = "teardown-gcp.sh"
	scriptSetupCustomDomains = "setup-custom-domains.sh"
	dirTerraform             = "terraform"

	// shopReferenceFlavor is the flavor shop's cloud scripts name in their
	// hints (the reference variant).
	shopReferenceFlavor = "multitier-java"

	// cloudSetupGuideURL is the scaffold user's guide to the cloud stages.
	cloudSetupGuideURL = "https://github.com/optivem/gh-optivem/blob/main/docs/cloud-setup.md"

	// systemNameMaxLen is terraform/main.tf's cut of the derived name.
	systemNameMaxLen = 14
)

// cloudSetupExecScripts are the cloud setup scripts that must stay executable
// in the scaffold (fixExecBits). terraform/*.sh are sourced, not executed.
var cloudSetupExecScripts = []string{scriptSetupGCP, scriptTeardownGCP, scriptSetupCustomDomains}

// copyCloudSetup copies setup-gcp.sh, teardown-gcp.sh and terraform/ into
// repoDir (the repo setup-gcp.sh is run from), plus setup-custom-domains.sh
// for multitier (shop's _acceptance-stage.yml accepts custom domains only for
// multitier). setup-gcp.sh runs it as ../setup-custom-domains.sh from
// terraform/, so it sits at the repo root beside setup-gcp.sh, as in shop.
// lang is the system (multitier: backend) language that names the Cloud Run
// services. Files missing from shop are skipped.
func copyCloudSetup(shop, repoDir, arch, lang string) {
	rewrites := cloudSetupRewrites(arch, lang)
	scripts := []string{scriptSetupGCP, scriptTeardownGCP}
	if arch == "multitier" {
		scripts = append(scripts, scriptSetupCustomDomains)
	}
	for _, name := range scripts {
		src := filepath.Join(shop, name)
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		s := string(data)
		for _, r := range rewrites {
			s = strings.ReplaceAll(s, r[0], r[1])
		}
		if err := os.WriteFile(filepath.Join(repoDir, name), []byte(s), 0o755); err != nil {
			log.Fatalf("Write %s: %v", name, err)
		}
	}
	src := filepath.Join(shop, dirTerraform)
	if dirExists(src) {
		if err := copyTerraformDir(src, filepath.Join(repoDir, dirTerraform)); err != nil {
			log.Fatalf("Copy %s/: %v", dirTerraform, err)
		}
	}
}

// cloudSetupRewrites points the scripts' shop-specific hints at the scaffold:
// the flattened acceptance-stage.yml, and (multitier) the scaffold's Cloud Run
// service names. The scaffold keeps the callers' architecture / language
// inputs, so its services are still named <system>-<arch>-<lang>-<stack>-...
// (_acceptance-stage.yml: p="${SYSTEM}-${PREFIX}-${stack}"); only the flavor
// segment changes. The service names go through a PREFIX variable so the
// literal "<arch>-<lang>-" (a forbidden template ref) never appears.
func cloudSetupRewrites(arch, lang string) [][2]string {
	r := [][2]string{
		{shopReferenceFlavor + suffixAcceptanceStage + ".yml", stageAcceptance + ".yml"},
	}
	if arch != "multitier" {
		return r
	}
	flavor := prefixMultitier + lang
	return append(r,
		[2]string{`SYSTEM="${SYSTEM:-$(gh variable get SYSTEM_NAME)}"`,
			`SYSTEM="${SYSTEM:-$(gh variable get SYSTEM_NAME)}"` + "\nPREFIX=\"" + flavor + "\"   # <architecture>-<language> of this repo's Cloud Run services"},
		[2]string{`"${SYSTEM}-` + shopReferenceFlavor + `-`, `"${SYSTEM}-${PREFIX}-`},
		[2]string{"of the reference variant\n# (" + shopReferenceFlavor + ")", "of this system"},
		[2]string{"one cloud acceptance run of " + shopReferenceFlavor, "one cloud acceptance run (acceptance-stage.yml)"},
	)
}

// copyTerraformDir copies shop's terraform/ without local Terraform state and
// working files (.terraform/, .terraform.lock.hcl, tfplan, *.tfstate*,
// terraform.tfvars, the kill-switch's .build/ archive): a shop checkout that
// has run setup-gcp.sh holds them, and they describe shop's GCP project, not
// the scaffold's.
func copyTerraformDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if rel != "." && isTerraformLocalState(info.Name()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func isTerraformLocalState(name string) bool {
	return strings.HasPrefix(name, ".terraform") ||
		name == "tfplan" ||
		name == ".build" ||
		name == "terraform.tfvars" ||
		strings.Contains(name, ".tfstate")
}

// isCloudSetupPath reports whether rel (relative to a scaffolded repo root)
// is a copied cloud setup file. Their text names Cloud Run services and
// flavors (<system>-multitier-typescript-production-frontend), which the
// scaffold's services still carry, so the leftover-template-ref guard skips
// them.
func isCloudSetupPath(rel string) bool {
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, dirTerraform+"/") {
		return true
	}
	for _, s := range cloudSetupExecScripts {
		if rel == s {
			return true
		}
	}
	return false
}

var nonSystemNameChar = regexp.MustCompile(`[^a-z0-9-]`)

// derivedSystemName mirrors shop's terraform/main.tf local.system_name for a
// repo name: lowercased, every character outside [a-z0-9-] replaced by "-",
// cut to 14 characters, leading/trailing "-" trimmed.
func derivedSystemName(repo string) string {
	s := nonSystemNameChar.ReplaceAllString(strings.ToLower(repo), "-")
	if len(s) > systemNameMaxLen {
		s = s[:systemNameMaxLen]
	}
	return strings.Trim(s, "-")
}

// cloudRunMaxSystemNameLen is the longest SYSTEM_NAME that keeps a multitier
// flavor's longest Cloud Run service name,
// <system>-multitier-<lang>-production-frontend, within Cloud Run's 49
// characters (shop setup-gcp.sh:6-7, terraform/main.tf:10-13). Monolith
// names are shorter and always fit the 14-character cut, as does
// multitier-java.
func cloudRunMaxSystemNameLen(lang string) int {
	const longestSuffix = len("-multitier-") + len("-production-frontend")
	return 49 - longestSuffix - len(lang)
}

// WarnCloudSystemName warns (never fails) when the SYSTEM_NAME setup-gcp.sh
// would derive from the repo name is too long for the scaffold's Cloud Run
// service names: over 8 characters for multitier-typescript, over 12 for
// multitier-dotnet. It does not pick a name; the user passes SYSTEM_NAME to
// setup-gcp.sh.
func WarnCloudSystemName(cfg *config.Config) {
	if msg := cloudSystemNameWarning(cfg.Repo, cfg.Arch, cfg.BackendLang); msg != "" {
		for _, line := range strings.Split(msg, "\n") {
			log.Warn(line)
		}
	}
}

// cloudSystemNameWarning returns the warning for repo, or "" when the derived
// SYSTEM_NAME fits the flavor's Cloud Run service names.
func cloudSystemNameWarning(repo, arch, lang string) string {
	if arch != "multitier" || lang == "" {
		return ""
	}
	name := derivedSystemName(repo)
	limit := cloudRunMaxSystemNameLen(lang)
	if len(name) <= limit {
		return ""
	}
	return fmt.Sprintf("Cloud setup: setup-gcp.sh would name this system %q (from the repo name), longer than %d characters, "+
		"so the multitier-%s production Cloud Run service names would exceed Cloud Run's 49-character limit and the production deploy would fail.\n"+
		"  The Docker pipeline is unaffected. Before deploying to Cloud Run, pick a name of 4-%d characters and run: SYSTEM_NAME=<short> ./setup-gcp.sh\n"+
		"  See %s", name, limit, lang, limit, cloudSetupGuideURL)
}
