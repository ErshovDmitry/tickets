package cli

// T-0159 A2: `ticket recover <number> <new-file> [--yes]` — the CLI half of
// recovery from an interrupted status commit (internal/store/recover.go does
// the proof and the single removal). The command never guesses the file to
// keep: the basename must be typed explicitly (it comes from the ambiguity
// diagnostics), a non-terminal run without --yes refuses before touching the
// tree, and the store's typed errors are rendered here in the ticket language.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// recoverPromptTTY reports whether an interactive keep/discard prompt may be
// shown. It is a seam for white-box tests: they replace it instead of
// allocating a real pseudo-terminal. A redirected, piped or /dev/null stdin
// is not a terminal (both are character devices, hence the os.DevNull
// comparison), so `ticket recover` refuses instead of deleting a file
// on a stdin that can never answer.
var recoverPromptTTY = func() bool {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	if nullFi, statErr := os.Stat(os.DevNull); statErr == nil && os.SameFile(fi, nullFi) {
		return false
	}
	return true
}

// recoverStrings holds the localized recover texts. The store carries no UI
// text (its errors are English internal descriptions), so the RU/EN pairs
// live next to the command that renders them.
type recoverStrings struct {
	header       string // printf: number
	keepFmt      string // printf: path
	discardFmt   string // printf: path
	discardNone  string
	ask          string
	refused      string
	cancelled    string
	badNameFmt   string // printf: number, basename
	keepOldFmt   string // printf: number, keep, other
	sameStateFmt string // printf: number
	divergentFmt string // printf: number, name, other
	selectionFmt string // printf: number, name
}

// recoverStringsFor returns the message set for lang; every language other
// than Russian falls back to English.
func recoverStringsFor(lang domain.Lang) recoverStrings {
	if lang == domain.LangRU {
		return recoverStrings{
			header:       "ticket %d: прерванная смена статуса",
			keepFmt:      "  сохранить: %s",
			discardFmt:   "  удалить:   %s",
			discardNone:  "  удалить:   (не определено)",
			ask:          "Удалить устаревший файл? yes/no: ",
			refused:      "ticket: нет терминала для подтверждения; повторите с --yes (ничего не изменено)",
			cancelled:    "ticket: восстановление отменено, ничего не изменено",
			badNameFmt:   "ticket: имя файла должно быть базовым именем тикета %d (например T-%04d-done.md), получено %q",
			keepOldFmt:   "ticket: %d: выбран устаревший файл %s; сначала восстановите %s, затем откатите статус командой set",
			sameStateFmt: "ticket: %d: у файлов одинаковый статус в имени; восстановление невозможно, ничего не изменено",
			divergentFmt: "ticket: %d: файл %s не является продолжением %s (содержимое расходится); восстановление невозможно, ничего не изменено",
			selectionFmt: "ticket: %d: файл %s не входит в пару конфликтующих файлов; ничего не изменено",
		}
	}
	return recoverStrings{
		header:       "ticket %d: interrupted status change",
		keepFmt:      "  keep:    %s",
		discardFmt:   "  discard: %s",
		discardNone:  "  discard: (not identified)",
		ask:          "Delete the stale file? yes/no: ",
		refused:      "ticket: no terminal to confirm on; re-run with --yes (nothing was changed)",
		cancelled:    "ticket: recovery cancelled, nothing was changed",
		badNameFmt:   "ticket: the file name must be a ticket %d basename (for example T-%04d-done.md), got %q",
		keepOldFmt:   "ticket: %d: selected %s is the stale file; recover %s first, then roll back with set",
		sameStateFmt: "ticket: %d: both files carry the same status in their names; recovery is impossible, nothing was changed",
		divergentFmt: "ticket: %d: %s is not the continuation of %s (contents diverge); recovery is impossible, nothing was changed",
		selectionFmt: "ticket: %d: %s is not part of the conflicting pair; nothing was changed",
	}
}

// cmdRecover implements `recover <number> <new-file> [--yes]`. Exit codes:
// 0 success including an idempotent repeat, 1 usage/refusal/not-found/I-O and
// the keep-old or single-file state errors, exitAmbiguous for a number that
// is not cleanly recoverable (more than two files, diverging contents, equal
// statuses). Nothing is mutated before confirmation.
func cmdRecover(st *store.Store, args []string, _, _ string, lang domain.Lang, stdout, stderr io.Writer) int {
	msg := recoverStringsFor(lang)
	if len(args) < 2 || len(args) > 3 {
		printUsage(stdout, stderr, lang)
		return 1
	}
	yes := len(args) == 3 && args[2] == "--yes"
	if len(args) == 3 && !yes {
		printUsage(stdout, stderr, lang)
		return 1
	}
	// Same positional guard as set/show (T-0068): a flag in the <number> or
	// <new-file> slot is a usage error, reported for the FIRST offender.
	for _, a := range args[:2] {
		if strings.HasPrefix(a, "-") {
			fmt.Fprintln(stderr, domain.ErrUnknownFlag(lang, a))
			return 1
		}
	}
	n, ok := parseTicketNumber(args[0])
	if !ok {
		return notFound(stderr, lang, args[0])
	}
	keepBase := args[1]
	if !validRecoverBase(n, keepBase) {
		fmt.Fprintf(stderr, msg.badNameFmt, n, n, keepBase)
		return 1
	}
	if !yes {
		if !recoverPromptTTY() {
			fmt.Fprintln(stderr, msg.refused)
			return 1
		}
		if !confirmRecover(st, n, keepBase, msg, stdout) {
			fmt.Fprintln(stderr, msg.cancelled)
			return 1
		}
	}
	kept, err := st.Recover(n, keepBase)
	if err != nil {
		return recoverError(st, stderr, lang, args[0], err)
	}
	fmt.Fprintln(stdout, kept)
	return 0
}

// validRecoverBase reports whether base is a bare basename naming ticket n
// with a known status; a path (any separator) is never accepted.
func validRecoverBase(n int, base string) bool {
	if base == "" || strings.ContainsAny(base, `/\\`) {
		return false
	}
	kn, kst, err := domain.ParseFilename(base)
	if err != nil || kn != n {
		return false
	}
	_, err = domain.ParseStatus(string(kst))
	return err == nil
}

// confirmRecover prints what recovery keeps and deletes, then reads one
// yes/no answer from the shared stdin seam. Anything but an explicit yes is a
// refusal, so a stray newline can never delete a file.
func confirmRecover(st *store.Store, n int, keepBase string, msg recoverStrings, stdout io.Writer) bool {
	keepPath, discardPath := recoverPair(st, n, keepBase)
	if keepPath == "" {
		keepPath = keepBase
	}
	fmt.Fprintf(stdout, msg.header+"\n", n)
	fmt.Fprintf(stdout, msg.keepFmt+"\n", keepPath)
	if discardPath == "" {
		fmt.Fprintln(stdout, msg.discardNone)
	} else {
		fmt.Fprintf(stdout, msg.discardFmt+"\n", discardPath)
	}
	_, _ = io.WriteString(stdout, msg.ask)
	return answerIsYes(stdinReader)
}

// recoverPair names the two conflicting files so the prompt can show them.
// It is read-only: the store's own refusal stays the authority. An unknown
// shape (no pair, a single file, more than two files) returns empty paths.
func recoverPair(st *store.Store, n int, keepBase string) (keepPath, discardPath string) {
	conflicts, err := st.Duplicates()
	if err != nil {
		return "", ""
	}
	for _, c := range conflicts {
		if c.Number != n || len(c.Paths) != 2 {
			continue
		}
		for _, p := range c.Paths {
			if filepath.Base(p) == keepBase {
				keepPath = p
			} else {
				discardPath = p
			}
		}
	}
	return keepPath, discardPath
}

// answerIsYes reads one line from r and reports whether it is an explicit
// confirmation (y/yes/д/да, case-insensitive).
func answerIsYes(r io.Reader) bool {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes", "д", "да":
		return true
	}
	return false
}

// recoverError maps a store failure to the pinned exit codes and a localized
// message. A typed RecoveryStateError is a state refusal (nothing changed).
func recoverError(st *store.Store, stderr io.Writer, lang domain.Lang, numArg string, err error) int {
	var amb *store.AmbiguousNumberError
	var rse *store.RecoveryStateError
	switch {
	case errors.As(err, &amb):
		return reportAmbiguous(stderr, lang, amb)
	case errors.As(err, &rse):
		return reportRecoveryState(stderr, lang, rse)
	case errors.Is(err, store.ErrNotFound):
		return notFoundHinted(st, stderr, lang, numArg)
	default:
		fmt.Fprintf(stderr, "ticket: %v\n", err)
		return 1
	}
}

// reportRecoveryState renders a typed state refusal. Diverging contents and
// equal statuses mean the number is not recoverable at all (exitAmbiguous);
// a keep-old or foreign selection is a plain refusal (exit 1).
func reportRecoveryState(stderr io.Writer, lang domain.Lang, rse *store.RecoveryStateError) int {
	msg := recoverStringsFor(lang)
	switch rse.Reason {
	case store.RecoverReasonSameStatus:
		fmt.Fprintf(stderr, msg.sameStateFmt+"\n", rse.Number)
		return exitAmbiguous
	case store.RecoverReasonDivergent:
		fmt.Fprintf(stderr, msg.divergentFmt+"\n", rse.Number, rse.Keep, rse.Other)
		return exitAmbiguous
	case store.RecoverReasonKeepOld:
		fmt.Fprintf(stderr, msg.keepOldFmt+"\n", rse.Number, rse.Keep, rse.Other)
		return 1
	default: // selection-not-in-pair, single-mismatch
		fmt.Fprintf(stderr, msg.selectionFmt+"\n", rse.Number, rse.Keep)
		return 1
	}
}
