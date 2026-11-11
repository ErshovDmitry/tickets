package releasecheck

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestStdlibOnlyImports guards the T-0173 invariants of the package: only
// stdlib imports, and never ticket/internal/cli or ticket/internal/domain
// (the package must stay UI-agnostic). It parses the package sources the
// same way exit-code-scan style guards do.
func TestStdlibOnlyImports(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, "\"")
			if strings.HasPrefix(path, "ticket/") {
				t.Errorf("%s: forbidden module-internal import %q", name, path)
			}
			if strings.Contains(strings.SplitN(path, "/", 2)[0], ".") {
				t.Errorf("%s: non-stdlib import %q", name, path)
			}
		}
	}
}
