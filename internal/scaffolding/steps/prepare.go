package steps

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// sharedReusableWorkflowRe matches a job-level call to one of shop's shared
// `_*.yml` reusable workflows (e.g. `uses: ./.github/workflows/_qa-stage.yml`).
var sharedReusableWorkflowRe = regexp.MustCompile(`(?m)^\s*uses:\s*['"]?\./\.github/workflows/(_[^\s'"]+\.yml)`)

// checkShopWorkflowLayout fails when a pipeline-stage workflow the scaffolder
// copies (see pipelineWorkflows) is a thin caller of a shared `_*.yml`
// reusable workflow. The scaffolder copies none of those (nor the
// .github/actions/* they use), and they derive paths from shop's
// system/<arch>/<lang> layout, so such a scaffold is broken: shop's
// single-pipeline restructure (shop plan 20261102-1855, 2026-11-03/04) made
// every <flavor>-{acceptance,qa,prod}-stage.yml such a caller; support for it
// is tracked in shop plans/backlog/p1-ci-pipeline/20261103-1536-gh-optivem-scaffolds-single-pipeline.md.
func checkShopWorkflowLayout(cfg *config.Config) error {
	tmpl := Names.MultitierPipelineStageWf
	if cfg.Arch == "monolith" {
		tmpl = Names.MonolithPipelineStageWf
	}
	var offending []string
	for src := range pipelineWorkflows(tmpl, VarsForCfg(cfg)) {
		data, err := os.ReadFile(filepath.Join(cfg.ShopPath, ".github", "workflows", src))
		if err != nil {
			continue // absent sources are reported by CopyWorkflows
		}
		for _, m := range sharedReusableWorkflowRe.FindAllStringSubmatch(string(data), -1) {
			offending = append(offending, src+" -> "+m[1])
		}
	}
	if len(offending) == 0 {
		return nil
	}
	sort.Strings(offending)
	return fmt.Errorf("its pipeline-stage workflows call shared reusable workflows that gh-optivem does not scaffold yet (%s); "+
		"use the default shop ref (latest meta-v* release) or one predating shop's single-pipeline restructure",
		strings.Join(offending, ", "))
}
