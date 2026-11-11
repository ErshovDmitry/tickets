package cli

// Guard against re-hardcoded exit codes (T-0163).
//
// Violation: a bare integer literal returned from a package-level FuncDecl (function
// or method) with exactly one plain int result, duplicating a value declared by a
// package-level exit[A-Z]... untyped int constant. No numeric table is kept: the set
// is extracted once from production sources and drives the synthetic fixtures too.
// Never claimed as covered: "+1", int(0), a named result returned bare after an
// assignment, function literals, multi-result helpers, //go:build-tagged files
// (build constraints are not evaluated; none exist here today).

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// exitConstants maps a declared exit-code value to its constant name.
type exitConstants map[int64]string

var productionExitCodes exitConstants

// mustExitCodes extracts the production set once, failing loudly when it is empty
// or a documented constant did not survive extraction.
func mustExitCodes(t *testing.T) exitConstants {
	t.Helper()
	if productionExitCodes == nil {
		codes := make(exitConstants)
		scanDir(t, ".", codes)
		productionExitCodes = codes
	}
	if len(productionExitCodes) == 0 {
		t.Fatal("extracted exit-code set is empty: guard would be vacuous")
	}
	for _, code := range []int64{int64(exitOK), int64(exitError), int64(exitAmbiguous)} {
		if _, ok := productionExitCodes[code]; !ok {
			t.Fatalf("extracted exit-code set lacks %d: guard would be vacuous", code)
		}
	}
	return productionExitCodes
}

func isExitConstName(name string) bool {
	return len(name) > 4 && strings.HasPrefix(name, "exit") && name[4] >= 'A' && name[4] <= 'Z'
}

// collectDeclaredExitCodes adds file's matched constants to codes, failing loudly
// on an unsupported form (explicit type, extra names, no/some values, non-literal).
func collectDeclaredExitCodes(t *testing.T, codes exitConstants, file *ast.File) {
	t.Helper()
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			idx := slices.IndexFunc(vs.Names, func(id *ast.Ident) bool { return isExitConstName(id.Name) })
			if idx < 0 {
				continue
			}
			name := vs.Names[idx].Name
			if len(vs.Names) != 1 || vs.Type != nil || len(vs.Values) != 1 {
				t.Fatalf("const %s: unsupported declaration form", name)
			}
			lit, ok := vs.Values[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.INT {
				t.Fatalf("const %s: expected an untyped integer literal", name)
			}
			value, err := strconv.ParseInt(lit.Value, 0, 64)
			if err != nil {
				t.Fatalf("const %s: parse %s: %v", name, lit.Value, err)
			}
			codes[value] = name
		}
	}
}

// exitCodeViolation is a bare literal returned where a declared code is due.
type exitCodeViolation struct {
	file string
	line int
	code int64
	name string
}

func (v exitCodeViolation) String() string {
	return fmt.Sprintf("%s:%d: literal exit code %d, use %s", v.file, v.line, v.code, v.name)
}

// scanFileExitCodes reports bare literals returned from single-int FuncDecls and
// counts examined returns, so callers can prove the scan is not vacuous. Function
// literals are skipped: their returns are callbacks, not exit codes.
func scanFileExitCodes(fset *token.FileSet, file *ast.File, codes exitConstants) (violations []exitCodeViolation, eligible int) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !hasSingleIntResult(fn) {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.FuncLit:
				return false
			case *ast.ReturnStmt:
				eligible++
				if code, isLiteral := literalExitCode(n, codes); isLiteral {
					pos := fset.Position(n.Pos())
					violations = append(violations, exitCodeViolation{pos.Filename, pos.Line, code, codes[code]})
				}
			}
			return true
		})
	}
	return violations, eligible
}

// literalExitCode reports a bare integer literal (optionally parenthesised) that
// duplicates a declared code; base 0 catches 0x1, ast.Unparen catches (2).
func literalExitCode(ret *ast.ReturnStmt, codes exitConstants) (int64, bool) {
	if len(ret.Results) != 1 {
		return 0, false
	}
	lit, ok := ast.Unparen(ret.Results[0]).(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return 0, false
	}
	value, err := strconv.ParseInt(lit.Value, 0, 64)
	if _, declared := codes[value]; err != nil || !declared {
		return 0, false
	}
	return value, true
}

// hasSingleIntResult reports exactly one plain int result: unnamed type, single.
func hasSingleIntResult(fn *ast.FuncDecl) bool {
	if results := fn.Type.Results; results == nil || len(results.List) != 1 {
		return false
	}
	field := fn.Type.Results.List[0]
	ident, ok := field.Type.(*ast.Ident)
	return ok && ident.Name == "int" && len(field.Names) <= 1
}

// scanDir scans the non-test .go files directly inside dir (no recursion) and
// declares every constant before checking any return site, so declaration order
// cannot hide a violation. Only immediate subdirectories are guarded; deeper nested
// Go sources are out of scope. A .go file in a direct subdirectory fails loudly.
func scanDir(t *testing.T, dir string, codes exitConstants) (violations []exitCodeViolation, parsedFiles, eligibleReturns int) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %q: %v", dir, err)
	}
	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			assertNoGoSources(t, path)
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %q: %v", path, err)
		}
		collectDeclaredExitCodes(t, codes, file)
		files = append(files, file)
	}
	for _, file := range files {
		fileViolations, eligible := scanFileExitCodes(fset, file, codes)
		violations = append(violations, fileViolations...)
		eligibleReturns += eligible
	}
	return violations, len(files), eligibleReturns
}

func assertNoGoSources(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read subdir %q: %v", dir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			t.Fatalf("non-recursive guard would miss %q: widen the scan or the test scope", filepath.Join(dir, entry.Name()))
		}
	}
}

func intFuncSource(body string) string { return "package p\n\nfunc f() int {\n\t" + body + "\n}\n" }

func writeSource(t *testing.T, path, src string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func TestScanFileExitCodes(t *testing.T) {
	codes := mustExitCodes(t)
	cases := []struct {
		name         string
		src          string
		wantCodes    []int64
		wantLines    []int
		wantEligible int
	}{
		{name: "literal zero", src: intFuncSource("return 0"), wantCodes: []int64{int64(exitOK)}, wantLines: []int{4}, wantEligible: 1},
		{name: "literal one", src: intFuncSource("return 1"), wantCodes: []int64{int64(exitError)}, wantLines: []int{4}, wantEligible: 1},
		{name: "ambiguous code", src: intFuncSource("return 2"), wantCodes: []int64{int64(exitAmbiguous)}, wantLines: []int{4}, wantEligible: 1},
		{name: "hex literal", src: intFuncSource("return 0x2"), wantCodes: []int64{int64(exitAmbiguous)}, wantLines: []int{4}, wantEligible: 1},
		{name: "parenthesised literal", src: intFuncSource("return (2)"), wantCodes: []int64{int64(exitAmbiguous)}, wantLines: []int{4}, wantEligible: 1},
		{name: "nested if return one", src: intFuncSource("if exitOK == 0 {\n\t\treturn 1\n\t}\n\treturn exitOK"), wantCodes: []int64{int64(exitError)}, wantLines: []int{5}, wantEligible: 2},
		{name: "method literal one", src: "package p\n\ntype t struct{}\n\nfunc (t) f() int {\n\treturn 1\n}\n", wantCodes: []int64{int64(exitError)}, wantLines: []int{6}, wantEligible: 1},
		{name: "non exit code", src: intFuncSource("return 10"), wantEligible: 1},
		{name: "negative literal", src: intFuncSource("return -1"), wantEligible: 1},
		{name: "float literal", src: intFuncSource("return 0.0"), wantEligible: 1},
		{name: "named exit constant", src: intFuncSource("return exitError"), wantEligible: 1},
		{name: "comment mentions return one", src: intFuncSource("// return 1\n\treturn exitOK"), wantEligible: 1},
		{name: "assigned variable", src: intFuncSource("code := 0\n\treturn code"), wantEligible: 1},
		{name: "unary plus is undetected", src: intFuncSource("return +2"), wantEligible: 1},
		{name: "conversion is undetected", src: intFuncSource("return int(2)"), wantEligible: 1},
		{name: "named result bare return is undetected", src: "package p\n\nfunc f() (n int) {\n\tn = 2\n\treturn\n}\n", wantEligible: 1},
		{name: "named int result type", src: "package p\n\nfunc f() myInt { return 0 }"},
		{name: "int and error results", src: "package p\n\nfunc f() (int, error) { return 0, nil }"},
		{name: "two int results", src: "package p\n\nfunc f() (int, int) { return 0, 1 }"},
		{name: "function literal callback", src: "package p\n\nfunc f() int {\n\tg := func() int { return 1 }\n\treturn g()\n}", wantEligible: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "sample.go", tc.src, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parse sample: %v\nsource:\n%s", err, tc.src)
			}
			violations, eligible := scanFileExitCodes(fset, file, codes)
			if eligible != tc.wantEligible {
				t.Fatalf("eligible returns = %d, want %d", eligible, tc.wantEligible)
			}
			got, gotLines := []int64{}, []int{}
			for _, v := range violations {
				got, gotLines = append(got, v.code), append(gotLines, v.line)
			}
			if !slices.Equal(got, tc.wantCodes) || !slices.Equal(gotLines, tc.wantLines) {
				t.Fatalf("flagged %v at %v, want %v at %v", got, gotLines, tc.wantCodes, tc.wantLines)
			}
		})
	}
}

func TestScanDirExitCodesFlagsReintroduction(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, filepath.Join(dir, "good.go"), intFuncSource("return exitOK"))
	writeSource(t, filepath.Join(dir, "bad.go"), intFuncSource("return 2"))
	writeSource(t, filepath.Join(dir, "bad_test.go"), intFuncSource("return 0"))
	// zz_decl.go sorts after bad.go and the caller set is empty: only a two-pass scan flags this.
	writeSource(t, filepath.Join(dir, "zz_decl.go"), "package p\n\nconst exitAmbiguous = 2\n")

	violations, parsedFiles, eligible := scanDir(t, dir, make(exitConstants))
	if parsedFiles != 3 || eligible != 2 {
		t.Fatalf("parsed %d files / %d returns, want 3 / 2 (test files must be skipped)", parsedFiles, eligible)
	}
	if len(violations) != 1 || violations[0].code != int64(exitAmbiguous) {
		t.Fatalf("violations = %v, want exactly one %d from bad.go", violations, int64(exitAmbiguous))
	}
	if base := filepath.Base(violations[0].file); base != "bad.go" {
		t.Fatalf("violation file = %q, want bad.go", base)
	}
}

// TestScanDirExitCodesProductionTree guards the package itself. baselineParsedFiles
// is the non-test file count when the guard landed: a floor, so new production files
// are allowed while a scan that stops seeing the package fails.
func TestScanDirExitCodesProductionTree(t *testing.T) {
	const baselineParsedFiles = 13
	codes := mustExitCodes(t)
	violations, parsedFiles, eligible := scanDir(t, ".", codes)
	if parsedFiles < baselineParsedFiles || eligible == 0 {
		t.Fatalf("scan too narrow (%d files, want >= %d; %d returns): guard would be vacuous", parsedFiles, baselineParsedFiles, eligible)
	}
	if len(violations) > 0 {
		t.Fatalf("literal exit codes reintroduced:\n%v", violations)
	}
}
