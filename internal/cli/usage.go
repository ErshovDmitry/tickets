package cli

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"ticket/internal/domain"
)

//go:embed templates/usage.*.txt
var usageEmbedFS embed.FS

// usageFS is the production filesystem for the usage templates. It is a
// variable rather than the embed.FS directly so usageFrom can be exercised
// against a MapFS or a failing filesystem without touching the binary.
var usageFS fs.FS = usageEmbedFS

// usageInitErr records a usage-template preload failure. The init() preload
// only feeds the usageTextRU/usageTextEN compatibility variables; rendering
// always re-reads the templates in usageFrom. A broken binary degrades to an
// error instead of a panic.
var usageInitErr error

// usageTextRU and usageTextEN are the embedded usage messages, populated
// from usageFS in init() for compatibility with existing tests.
// templates/usage.ru.txt descends from the pre-Go bash-era usage message
// (not part of this repository; docs_api/ keeps only a stale copy of another revision) but has long
// diverged from it — ticket init, -P on new and list, --json on list only,
// the global -C/--tickets-dir flag; usage.en.txt is a translation with an
// identical line layout. The T-0036 ticket-migration section was removed in
// T-0095, so neither file documents a migration recipe any more. The version
// line is prepended at runtime (T-0028), not in the templates.
var usageTextRU string
var usageTextEN string

func init() {
	var errs []error
	if data, err := fs.ReadFile(usageFS, "templates/usage.ru.txt"); err != nil {
		errs = append(errs, fmt.Errorf("usage: read usage.ru.txt: %w", err))
	} else {
		usageTextRU = string(data)
	}
	if data, err := fs.ReadFile(usageFS, "templates/usage.en.txt"); err != nil {
		errs = append(errs, fmt.Errorf("usage: read usage.en.txt: %w", err))
	} else {
		usageTextEN = string(data)
	}
	usageInitErr = errors.Join(errs...)
}

// usage writes the version line followed by the usage message for lang to w.
// The language is selected by the lang parameter (LangRU or LangEN); an
// unavailable language falls back to English with a non-nil error.
func usage(w io.Writer, lang domain.Lang) error {
	return usageFrom(w, lang, usageFS, usageInitErr)
}

// writeUsageChunk performs one usage write. A returned error and a short
// write are both failures: a partial write must never look like success. The
// wrap is stage-neutral on purpose: the caller prefixes the stage name, so a
// single "usage:" prefix is added exactly once per diagnostic.
func writeUsageChunk(w io.Writer, chunk []byte) error {
	n, err := w.Write(chunk)
	if err != nil {
		return fmt.Errorf("write chunk: %w", err)
	}
	if n != len(chunk) {
		return io.ErrShortWrite
	}
	return nil
}

// usageFrom renders the usage text from fsys.
//
// The selected template is read first. Only when that read FAILS is the EN
// template read as a fallback; a readable selected template never opens the
// EN file (so an EN read error can never be silently discarded). Both reads
// still complete BEFORE anything is written, so the fallback matrix never
// leaves a partially decided stdout:
//   - selected template readable          → version line + selected text;
//   - selected unreadable, EN readable    → version line + EN text + error;
//   - neither readable                    → no write at all + error.
//
// The version line and the template body are TWO separate writes, each with
// its own error and short-write check, so a failure is attributed to the
// stage that produced it. A short write is reported as io.ErrShortWrite,
// because a partial write must still be a failure for the caller.
//
// initErr is the stored compatibility-preload failure. It is unrelated to a
// readable selected template, so it is joined ONLY on the selected-read /
// fallback error path: a valid selected render returns nil even when initErr
// is set, and only a real read/write failure makes it observable.
func usageFrom(w io.Writer, lang domain.Lang, fsys fs.FS, initErr error) error {
	var errs []error

	selectedName := "templates/usage." + string(lang) + ".txt"
	selected, selErr := fs.ReadFile(fsys, selectedName)

	// EN is read ONLY on the fallback path: a valid selected read must not
	// touch the EN template at all, so its error can never be discarded.
	var en []byte
	var enErr error
	if selErr != nil {
		// Wrap right here so the failure always names the requested
		// language and its path, whichever fallback branch reports it.
		selErr = fmt.Errorf("usage: selected template for language %q (%s): %w", string(lang), selectedName, selErr)
		en, enErr = fs.ReadFile(fsys, "templates/usage.en.txt")
		if enErr != nil {
			enErr = fmt.Errorf("usage: EN template templates/usage.en.txt: %w", enErr)
		}
	}

	joinInitErr := func() {
		if initErr != nil {
			errs = append(errs, initErr)
		}
	}

	var text []byte
	switch {
	case selErr == nil:
		text = selected
	case enErr == nil:
		// Fall back to EN, but keep the failed selected read as an error
		// that still names the requested language and its path so the
		// caller can print one diagnostic and exit non-zero.
		text = en
		joinInitErr()
		errs = append(errs, selErr)
	default:
		joinInitErr()
		errs = append(errs, selErr, enErr)
		return errors.Join(errs...)
	}

	// Stage 1: version line. Stage 2: the template body. Both are attempted
	// so each stage's failure is checked and reported independently.
	if err := writeUsageChunk(w, []byte(versionLine())); err != nil {
		errs = append(errs, fmt.Errorf("usage: write version line: %w", err))
	}
	if err := writeUsageChunk(w, text); err != nil {
		errs = append(errs, fmt.Errorf("usage: write body: %w", err))
	}
	return errors.Join(errs...)
}

// printUsage writes the usage text and turns any usageFrom error into one
// stderr diagnostic plus exit code 1. Every usage call site goes through it,
// so stdout keeps exactly the usageFrom matrix and success codes stay 0.
func printUsage(stdout, stderr io.Writer, lang domain.Lang) int {
	if err := usage(stdout, lang); err != nil {
		// A usage diagnostic is contractually ONE physical stderr line, but
		// errors.Join separates its members with newlines: fold them into
		// spaces before writing.
		msg := strings.ReplaceAll(err.Error(), "\n", " ")
		fmt.Fprintln(stderr, msg)
		return 1
	}
	return 0
}
