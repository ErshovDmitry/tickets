package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// detailsStore returns a writable store on a fresh sandbox dir.
func detailsStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New(%q): %v", dir, err)
	}
	return st, dir
}

// withStdin replaces the stdinReader seam for the test and restores the
// previous value afterwards (no leak into other tests).
func withStdin(t *testing.T, r io.Reader) {
	t.Helper()
	old := stdinReader
	stdinReader = r
	t.Cleanup(func() { stdinReader = old })
}

// failingReader always fails with its error.
type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

// fileReadMessage builds the expected localized message of a failed file
// read: the path once and the cause once (never the path twice).
func fileReadMessage(t *testing.T, lang domain.Lang, path string) string {
	t.Helper()
	_, err := os.ReadFile(path)
	var pe *fs.PathError
	if !errors.As(err, &pe) {
		t.Fatalf("read error %v is not a *fs.PathError", err)
	}
	return domain.ErrDetailsFileRead(lang, path, pe.Err) + "\n"
}

// largeDetails builds a numbered details list larger than 64 KiB. No line
// starts with "## ", so the payload can never be mistaken for a header.
func largeDetails() string {
	var b strings.Builder
	for i := 1; b.Len() <= 64*1024; i++ {
		fmt.Fprintf(&b, "%d. Строка деталей №%d — байты сохраняются без изменений.\n", i, i)
	}
	return b.String()
}

// createTicketWithDetails creates a ticket through cmdNew and returns the
// bytes of the created file.
func createTicketWithDetails(t *testing.T, detailsArg string, lang domain.Lang) []byte {
	t.Helper()
	st, _ := detailsStore(t)
	var stdout, stderr bytes.Buffer
	rc := cmdNew(st, []string{"Тикет с деталями", "-d", detailsArg}, "tester", "prj", lang, &stdout, &stderr)
	if rc != 0 {
		t.Fatalf("cmdNew(-d %q) = %d, want 0; stderr: %q", detailsArg, rc, stderr.String())
	}
	data, err := os.ReadFile(strings.TrimSpace(stdout.String()))
	if err != nil {
		t.Fatalf("read created ticket: %v", err)
	}
	return data
}

// assertNoTicketFiles fails when the sandbox holds a markdown file: no
// ticket means no creation journal entry either.
func assertNoTicketFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read sandbox %q: %v", dir, err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			t.Fatalf("ticket file %q created despite the details failure", e.Name())
		}
	}
}

// TestResolveDetailsSources pins the successful forms: "@file", "@@name"
// (the file NAMED "@name", with its OWN content), a relative path, literal text and an empty file
// — all byte-exact.
func TestResolveDetailsSources(t *testing.T) {
	// The "@name" fixture MUST differ from "details.txt": identical bodies
	// would make the "@@name" case unable to prove which file was read.
	atNameContent := "3. содержимое файла @name\n"
	content := "1. первая строка\n2. вторая строка\n"
	cases := []struct {
		name string
		arg  func(t *testing.T, dir string) string
		want string
	}{
		{"file marker", func(_ *testing.T, dir string) string { return "@" + filepath.Join(dir, "details.txt") }, content},
		{"double at is not an escape", func(t *testing.T, dir string) string { t.Chdir(dir); return "@@name" }, atNameContent},
		{"relative path", func(t *testing.T, dir string) string { t.Chdir(dir); return "@details.txt" }, content},
		{"literal text", func(*testing.T, string) string { return "обычный текст" }, "обычный текст"},
		{"literal with at inside", func(*testing.T, string) string { return "почта user@example.com" }, "почта user@example.com"},
		{"literal dash is not stdin", func(*testing.T, string) string { return "-x" }, "-x"},
		{"empty file", func(_ *testing.T, dir string) string { return "@" + filepath.Join(dir, "empty.txt") }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			fixtures := map[string]string{"details.txt": content, "@name": atNameContent, "empty.txt": ""}
			for name, body := range fixtures {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
					t.Fatalf("write fixture %q: %v", name, err)
				}
			}
			var stderr bytes.Buffer
			got, ok := resolveDetails(tc.arg(t, dir), domain.LangRU, &stderr)
			if !ok {
				t.Fatalf("resolveDetails failed: %q", stderr.String())
			}
			if got != tc.want || stderr.Len() != 0 {
				t.Fatalf("details = %q, want %q; stderr = %q", got, tc.want, stderr.String())
			}
		})
	}
}

// TestResolveDetailsFailures pins the failure messages: a bare "@" carries
// no path, a failed file read names the path once and keeps the cause.
func TestResolveDetailsFailures(t *testing.T) {
	t.Run("bare at", func(t *testing.T) {
		for _, lang := range []domain.Lang{domain.LangRU, domain.LangEN} {
			var stderr bytes.Buffer
			got, ok := resolveDetails("@", lang, &stderr)
			if ok || got != "" {
				t.Fatalf("resolveDetails(@) = %q, %v; want failure", got, ok)
			}
			if want := domain.ErrDetailsAtBare(lang) + "\n"; stderr.String() != want {
				t.Fatalf("stderr = %q, want %q", stderr.String(), want)
			}
		}
	})
	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.txt")
		var stderr bytes.Buffer
		got, ok := resolveDetails("@"+path, domain.LangRU, &stderr)
		if ok || got != "" {
			t.Fatalf("resolveDetails(@missing) = %q, %v; want failure", got, ok)
		}
		if want := fileReadMessage(t, domain.LangRU, path); stderr.String() != want {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
		if n := strings.Count(stderr.String(), path); n != 1 {
			t.Fatalf("path %q appears %d times, want once", path, n)
		}
	})
}

// TestResolveDetailsStdin pins the "-" form, the failure of a broken stdin
// and the restoration of the seam.
func TestResolveDetailsStdin(t *testing.T) {
	t.Run("dash", func(t *testing.T) {
		withStdin(t, strings.NewReader("текст из stdin\n"))
		var stderr bytes.Buffer
		got, ok := resolveDetails("-", domain.LangRU, &stderr)
		if !ok || got != "текст из stdin\n" {
			t.Fatalf("resolveDetails(-) = %q, %v; stderr %q", got, ok, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("stderr = %q, want empty", stderr.String())
		}
	})
	t.Run("empty stdin", func(t *testing.T) {
		withStdin(t, strings.NewReader(""))
		got, ok := resolveDetails("-", domain.LangRU, io.Discard)
		if !ok || got != "" {
			t.Fatalf("resolveDetails(-) = %q, %v; want empty details", got, ok)
		}
	})
	t.Run("read failure", func(t *testing.T) {
		readErr := errors.New("stdin сломан")
		withStdin(t, failingReader{err: readErr})
		var stderr bytes.Buffer
		got, ok := resolveDetails("-", domain.LangRU, &stderr)
		if ok || got != "" {
			t.Fatalf("resolveDetails(-) = %q, %v; want failure", got, ok)
		}
		if want := domain.ErrDetailsStdinRead(domain.LangRU, readErr) + "\n"; stderr.String() != want {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	})
	t.Run("seam restored", func(t *testing.T) {
		t.Run("replace", func(t *testing.T) {
			withStdin(t, strings.NewReader("x"))
			if stdinReader == os.Stdin {
				t.Fatal("stdinReader seam was not replaced")
			}
		})
		if stdinReader != os.Stdin {
			t.Fatalf("stdinReader seam not restored: %v", stdinReader)
		}
	})
}

// TestNewDetailsFailureExitsOneNoTicketNoJournal pins the user-visible
// contract: exit code 1, a localized message and NO ticket file (hence no
// journal entry) in both languages.
func TestNewDetailsFailureExitsOneNoTicketNoJournal(t *testing.T) {
	for _, lang := range []domain.Lang{domain.LangRU, domain.LangEN} {
		t.Run(string(lang)+" bare at", func(t *testing.T) {
			st, dir := detailsStore(t)
			var stderr bytes.Buffer
			rc := cmdNew(st, []string{"Тикет", "-d", "@"}, "tester", "prj", lang, io.Discard, &stderr)
			if rc != 1 {
				t.Fatalf("cmdNew = %d, want 1", rc)
			}
			if want := domain.ErrDetailsAtBare(lang) + "\n"; stderr.String() != want {
				t.Fatalf("stderr = %q, want %q", stderr.String(), want)
			}
			assertNoTicketFiles(t, dir)
		})
		t.Run(string(lang)+" stdin read failure", func(t *testing.T) {
			st, dir := detailsStore(t)
			readErr := errors.New("stdin сломан")
			withStdin(t, failingReader{err: readErr})
			var stderr bytes.Buffer
			rc := cmdNew(st, []string{"Тикет", "-d", "-"}, "tester", "prj", lang, io.Discard, &stderr)
			if rc != 1 {
				t.Fatalf("cmdNew = %d, want 1", rc)
			}
			if want := domain.ErrDetailsStdinRead(lang, readErr) + "\n"; stderr.String() != want {
				t.Fatalf("stderr = %q, want %q", stderr.String(), want)
			}
			assertNoTicketFiles(t, dir)
		})
		t.Run(string(lang)+" missing file", func(t *testing.T) {
			st, dir := detailsStore(t)
			path := filepath.Join(dir, "missing.txt")
			var stderr bytes.Buffer
			rc := cmdNew(st, []string{"Тикет", "-d", "@" + path}, "tester", "prj", lang, io.Discard, &stderr)
			if rc != 1 {
				t.Fatalf("cmdNew = %d, want 1", rc)
			}
			if want := fileReadMessage(t, lang, path); stderr.String() != want {
				t.Fatalf("stderr = %q, want %q", stderr.String(), want)
			}
			assertNoTicketFiles(t, dir)
		})
	}
}

// TestNewDetailsRussianMultilinePreserved pins that a multiline Russian
// details text reaches the created file unchanged.
func TestNewDetailsRussianMultilinePreserved(t *testing.T) {
	details := "1. Первая строка — internal/cli/cmd_new.go:1; решение принято.\n" +
		"2. Вторая строка\n" +
		"3. Третья строка\n"
	data := createTicketWithDetails(t, details, domain.LangRU)
	if !bytes.Contains(data, []byte(details)) {
		t.Fatalf("created ticket does not hold the details verbatim:\n%s", data)
	}
}

// TestNewDetailsLargePayloadPreserved pins that details larger than 64 KiB
// are stored byte-exact — from a file and from stdin.
func TestNewDetailsLargePayloadPreserved(t *testing.T) {
	payload := largeDetails()
	if len(payload) <= 64*1024 {
		t.Fatalf("payload is %d bytes, want > 64 KiB", len(payload))
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	data := createTicketWithDetails(t, "@"+path, domain.LangRU)
	if !bytes.Contains(data, []byte(payload)) {
		t.Fatalf("created ticket does not hold the file payload verbatim (%d bytes)", len(payload))
	}
	withStdin(t, strings.NewReader(payload))
	data = createTicketWithDetails(t, "-", domain.LangRU)
	if !bytes.Contains(data, []byte(payload)) {
		t.Fatalf("created ticket does not hold the stdin payload verbatim (%d bytes)", len(payload))
	}
}
