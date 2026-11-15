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
// same way exit-code-scan style guards do; a dir without production sources
// (a cross-compiled test binary) skips instead of passing vacuously.
func TestStdlibOnlyImports(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	parsed := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed++
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
	if parsed == 0 {
		t.Skip("no production Go sources in the package dir: the import guard has nothing to inspect")
	}
}
