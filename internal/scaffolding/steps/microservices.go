package steps

import (
	"path/filepath"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/scaffolding/files"
	"github.com/optivem/gh-optivem/internal/kernel/log"
)

// applyMicroservices scaffolds the microservices backend: one backend scaffold
// per declared service location (N backends), then the single frontend (D5).
//
// It is the architecture-additive sibling of applyMonolith* / applyMultitier*:
// monolith assumes one SystemPath, multitier one BackendPath; microservices
// loops cfg.BackendServices (the YAML-authored backend-services: map, sorted by
// FillRawFlagsFromYAML) and reuses the single-backend copy primitive per
// service. Each service brings its own location/path, language, and repo (D2/D3
// heterogeneous, per-repo), so the loop body is "the multitier backend, once
// per service" rather than a new parallel codepath.
//
// The single-backend architectures are untouched — this arm only runs when
// cfg.Arch == "microservices", reached from ApplyTemplate's switch.
func applyMicroservices(cfg *config.Config) {
	log.Info("Scaffolding microservices backend services...")
	ScaffoldBackendServices(cfg)

	log.Info("Copying frontend code...")
	scaffoldMicroservicesFrontend(cfg)

	log.Success("Applied template files (microservices)")
}

// ScaffoldBackendServices materializes one backend scaffold per service in
// cfg.BackendServices, copying each service's source from the shop template for
// that service's language into the service's own repo directory at its
// repo-relative path. N declared service locations produce N backend scaffolds.
//
// Per-service source: shop's system/multitier/backend-<lang> is the only
// per-language backend template the shop ref carries; each service reuses it
// for its declared language, landing at the service's own Path. The destination
// path comes deterministically from the service's TierSpec.Path (authored in
// gh-optivem.yaml) — no per-language stem guessing, no layout fallback.
//
// Exported so the scaffolding test can drive it directly (it produces the
// per-service directory tree the N-backends assertion checks) without standing
// up the GitHub / docker tail of the full pipeline.
func ScaffoldBackendServices(cfg *config.Config) {
	for _, svc := range cfg.BackendServices {
		dst := backendServiceDir(cfg, svc)
		src := filepath.Join(cfg.ShopPath, "system", "multitier", "backend-"+svc.Lang)
		log.Infof("  service %q (%s) -> %s", svc.Name, svc.Lang, dst)
		if err := files.CopyDir(src, dst); err != nil {
			log.Fatalf("scaffold backend service %q: copy %s -> %s: %v", svc.Name, src, dst, err)
		}
	}
}

// backendServiceDir resolves the on-disk destination for a service's backend
// code: <repo-dir>/<service-path>. The repo dir is the shared scaffold clone in
// mono-repo (every service shares one repo, distinct paths) or the service's
// own clone dir in multi-repo (one repo per service). RepoDir is honoured when
// already resolved (set after clone); otherwise it falls back to the per-repo
// derivation from cfg.WorkDir.
func backendServiceDir(cfg *config.Config, svc config.BackendService) string {
	return filepath.Join(backendServiceRepoDir(cfg, svc), svc.Path)
}

// backendServiceRepoDir returns the local clone directory a service's code is
// written into. Mono-repo: the shared cfg.RepoDir. Multi-repo: the service's
// own clone dir — pre-resolved on the carrier (svc.RepoDir) when set, else
// derived from cfg.WorkDir + the service name (matching resolveCloneDirs'
// repo-<component> convention).
func backendServiceRepoDir(cfg *config.Config, svc config.BackendService) string {
	if cfg.RepoStrategy != "multirepo" {
		return cfg.RepoDir
	}
	if svc.RepoDir != "" {
		return svc.RepoDir
	}
	return filepath.Join(cfg.WorkDir, "repo-"+svc.Name)
}

// scaffoldMicroservicesFrontend copies the single frontend (D5) from shop's
// React template into the frontend repo dir at cfg.FrontendPath. Mirrors the
// multitier frontend copy; microservices multiplicity is backend-only.
func scaffoldMicroservicesFrontend(cfg *config.Config) {
	dst := filepath.Join(microservicesFrontendRepoDir(cfg), cfg.FrontendPath)
	src := filepath.Join(cfg.ShopPath, "system", "multitier", "frontend-react")
	if err := files.CopyDir(src, dst); err != nil {
		log.Fatalf("scaffold microservices frontend: copy %s -> %s: %v", src, dst, err)
	}
}

// microservicesFrontendRepoDir returns the local clone dir the frontend lands
// in: the shared cfg.RepoDir (mono-repo) or the frontend's own clone dir
// (multi-repo), following the same convention as backendServiceRepoDir.
func microservicesFrontendRepoDir(cfg *config.Config) string {
	if cfg.RepoStrategy != "multirepo" {
		return cfg.RepoDir
	}
	if cfg.FrontendRepoDir != "" {
		return cfg.FrontendRepoDir
	}
	return filepath.Join(cfg.WorkDir, "repo-frontend")
}
