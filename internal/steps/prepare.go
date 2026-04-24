package steps

import (
	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
)

// CloneShopTemplate clones the optivem/shop template repository pinned to
// cfg.ShopRef into cfg.ShopPath (pre-computed during ParseAndValidate).
//
// The clone runs in every mode (including --dry-run) because every downstream
// step — validation, replacement, reference checks — needs the template on disk.
func CloneShopTemplate(cfg *config.Config) {
	if err := config.CloneShop(cfg.ShopRef, cfg.ShopPath); err != nil {
		log.Fatalf("Cannot clone shop repo: %v", err)
	}
}
