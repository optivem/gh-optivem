package projectconfig

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestNoUpwardEngineImport is the backstop for seam #1 (plan
// 20260615-0749): projectconfig is a near-kernel domain leaf and must not
// reach up into the ATDD engine/runtime. The one rule that needed engine
// knowledge — the task-prompts known-name check — was relocated to
// internal/atdd/runtime/configcheck. This guard fails loudly if any file in
// this package re-introduces an internal/atdd/** import, so the backwards
// edge can't silently return.
func TestNoUpwardEngineImport(t *testing.T) {
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
			if strings.Contains(path, "github.com/optivem/gh-optivem/internal/atdd/") {
				t.Errorf("%s imports %q: projectconfig must not depend on the ATDD engine/runtime (seam #1). "+
					"Engine-derived rules belong in internal/atdd/runtime/configcheck, not here.", file, path)
			}
		}
	}
}
