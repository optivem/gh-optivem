package build

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestBuildImportsKernelOnly is the cycle backstop for the shared build module
// (parent seams #3 steps -> compiler,runner and #4 preflight -> runner). build
// is a leaf shared module: Scaffolding, Process, and the CLI may depend on it,
// so it must never depend back up on them. This guard walks the whole
// internal/build/** subtree and fails loudly if any non-test file imports a
// project package outside internal/kernel/** and internal/build/** itself,
// which is what keeps build from growing a back-edge and turning the shared
// dependency into an import cycle. Intra-build seams (steps -> compiler,runner;
// preflight -> runner; componenttest -> runner) are leaf-internal and allowed.
func TestBuildImportsKernelOnly(t *testing.T) {
	t.Parallel()
	const (
		projectPrefix = "github.com/optivem/gh-optivem/internal/"
		kernelPrefix  = "github.com/optivem/gh-optivem/internal/kernel/"
		buildPrefix   = "github.com/optivem/gh-optivem/internal/build/"
	)
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		af, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range af.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("%s: unquote import %q: %v", path, imp.Path.Value, err)
			}
			if strings.HasPrefix(p, projectPrefix) &&
				!strings.HasPrefix(p, kernelPrefix) && !strings.HasPrefix(p, buildPrefix) {
				t.Errorf("%s imports %q: internal/build/** may import only internal/kernel/** "+
					"or other internal/build/** packages (build is a shared leaf module; "+
					"importing Scaffolding/Process would create a cycle).", path, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/build: %v", err)
	}
}
