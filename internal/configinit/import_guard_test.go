package configinit

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestNoScaffoldingImport is the backstop for seam #2: configinit (the
// Config module) must not reach into the scaffolding package. The
// optivem.yaml builder it needs lives in internal/config/optivemyaml and
// the gitignore helper in internal/kernel/gitignore. This guard fails
// loudly if any file in this package re-introduces an
// internal/scaffolding/** import, so the backwards edge can't silently
// return.
func TestNoScaffoldingImport(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob package files: %v", err)
	}
	fset := token.NewFileSet()
	for _, file := range files {
		af, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, imp := range af.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("%s: unquote import %q: %v", file, imp.Path.Value, err)
			}
			if strings.Contains(path, "github.com/optivem/gh-optivem/internal/scaffolding/") {
				t.Errorf("%s imports %q: configinit must not depend on scaffolding (seam #2). "+
					"The optivem.yaml builder belongs in internal/config/optivemyaml and the "+
					"gitignore helper in internal/kernel/gitignore, not via scaffolding.", file, path)
			}
		}
	}
}
