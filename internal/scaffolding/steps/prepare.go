package steps

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/kernel/log"
)

// CloneShopTemplate clones the optivem/shop template repository pinned to
// cfg.ShopRef into cfg.ShopPath (pre-computed during ParseAndValidate), then
// refuses shop refs whose workflow layout the scaffolder cannot reproduce —
// before any GitHub repo is created.
func CloneShopTemplate(cfg *config.Config) {
	if err := config.CloneShop(cfg.ShopRef, cfg.ShopPath); err != nil {
		log.Fatalf("Cannot clone shop repo: %v", err)
	}
	if err := checkShopWorkflowLayout(cfg); err != nil {
		log.Fatalf("Unsupported shop ref %s: %v", cfg.ShopRef, err)
	}
}

// checkShopWorkflowLayout refuses shop refs that predate shop's
// single-pipeline layout (shop plan 20261102-1855, on shop main since
// 2026-11-06): the scaffolder copies the per-flavor thin callers
// (<arch>-<testLang>-acceptance-stage-docker-compose.yml, ...) and the shared
// _*.yml workflows they call, and old refs (meta-v1.0.223 and earlier, with
// full per-flavor stage workflows and -cloud twins) have neither. Supporting
// both layouts would mean two copy lists and two rewrite sets; old refs are
// refused instead, before any GitHub repo is created. Microservices scaffolds
// copy no pipeline workflows, so they are not checked.
func checkShopWorkflowLayout(cfg *config.Config) error {
	if cfg.Arch != "monolith" && cfg.Arch != "multitier" {
		return nil
	}
	tmpl := Names.MultitierPipelineStageWf
	if cfg.Arch == "monolith" {
		tmpl = Names.MonolithPipelineStageWf
	}
	required := []string{
		Expand(tmpl, MergeVars(VarsForCfg(cfg), map[string]string{"stage": stageAcceptanceDockerCompose})),
		"_acceptance-stage-docker-compose.yml",
	}
	var missing []string
	for _, name := range required {
		if _, err := os.Stat(filepath.Join(cfg.ShopPath, ".github", "workflows", name)); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("it predates shop's single-pipeline workflow layout (missing %s), which this gh-optivem version scaffolds; "+
		"use a newer shop ref (e.g. --shop-ref main, or a meta-v* release cut after 2026-11-06) or an older gh-optivem release",
		strings.Join(missing, ", "))
}
