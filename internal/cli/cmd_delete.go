package cli

// T-0189 W2-A1: `ticket delete <number|filename> [--yes] [--force]` — the CLI
// half of soft-delete (internal/store/delete.go does the resolve/revalidate
// and the single move into tickets/deleted/). Exactly one ref — a number or
// an exact T-NNNN-<status>.md base name — resolved across main+archive+deleted;
// flags come only after the ref, in any order. Without --force only a
// main-directory open ticket is deletable; without --yes a terminal yes/no
// confirmation is required (the shared recover TTY/stdin seams). Nothing is
// mutated before confirmation, and a repeat is an error, never a no-op success.

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// maxDeleteRefNumber mirrors store.maxTicketNumber: the largest number that
// fits the 4-digit T-NNNN file name. Keep in sync with store/store_lock.go.
const maxDeleteRefNumber = 9999

// cmdDelete implements `delete <number|filename> [--yes] [--force]`. Exit codes:
// 0 success (prints the deleted file's path), 1 usage/refusal/not-found/
// already-deleted/collision/I-O, exitAmbiguous for a ref carried by several
// files. The pre-check resolves and guards before any prompt; Delete then
// re-resolves and revalidates under the store lock before mutating.
// Signature per wave-1 dispatch contract (wiki 8bd93a4e, A1).
func cmdDelete(st *store.Store, args []string, who, _ string, lang domain.Lang, stdout, stderr io.Writer) int {
	ref, yes, force, ok := parseDeleteArgs(args, lang, stdout, stderr)
	if !ok {
		return exitError
	}
	if !validDeleteRef(ref) {
		fmt.Fprintln(stderr, domain.ErrDeleteInvalidRef(lang, ref))
		return exitError
	}
	cand, err := st.ResolveDelete(ref)
	if err != nil {
		return deleteError(st, stderr, lang, ref, err)
	}
	if code, refused := deleteGuard(cand, st.Dir, force, lang, stderr); refused {
		return code
	}
	if !yes {
		if !recoverPromptTTY() {
			fmt.Fprintln(stderr, domain.DeleteRefused(lang))
			return exitError
		}
		fmt.Fprintln(stdout, domain.DeletePrompt(lang, cand.Path))
		_, _ = io.WriteString(stdout, domain.DeleteAsk(lang))
		if !answerIsYes(stdinReader) {
			fmt.Fprintln(stderr, domain.DeleteCancelled(lang))
			return exitError
		}
	}
	deletedPath, err := st.Delete(ref, force, who)
	if err != nil {
		return deleteError(st, stderr, lang, ref, err)
	}
	fmt.Fprintln(stdout, deletedPath)
	return exitOK
}

// parseDeleteArgs extracts the single ref and the --yes/--force flags. Flags
// are accepted only after the ref, in any order, each at most once; a glued
// (--yes=x), unknown or repeated flag and any extra positional are usage
// errors. A flag in the ref slot itself is the first-offender report of
// T-0068 (ErrUnknownFlagList, the command-agnostic text), not usage. ok=false
// means the error was already printed.
func parseDeleteArgs(args []string, lang domain.Lang, stdout, stderr io.Writer) (ref string, yes, force, ok bool) {
	if len(args) == 0 {
		printUsage(stdout, stderr, lang)
		return "", false, false, false
	}
	ref = args[0]
	if strings.HasPrefix(ref, "-") {
		fmt.Fprintln(stderr, domain.ErrUnknownFlagList(lang, ref))
		return "", false, false, false
	}
	for _, a := range args[1:] {
		switch a {
		case "--yes":
			if yes {
				printUsage(stdout, stderr, lang)
				return "", false, false, false
			}
			yes = true
		case "--force":
			if force {
				printUsage(stdout, stderr, lang)
				return "", false, false, false
			}
			force = true
		default:
			printUsage(stdout, stderr, lang)
			return "", false, false, false
		}
	}
	return ref, yes, force, true
}

// validDeleteRef reports whether ref is a ticket number or an exact ticket
// base name, mirroring store.resolveDeleteRef's form check (no separators;
// a canonical base name, else strict digits within 1..maxDeleteRefNumber)
// so an invalid ref is refused with a localized message before any scan.
func validDeleteRef(ref string) bool {
	if strings.ContainsAny(ref, "/\\") {
		return false
	}
	if _, _, err := domain.ParseFilename(ref); err == nil {
		return true
	}
	if ref == "" {
		return false
	}
	for _, r := range ref {
		if r < '0' || r > '9' {
			return false
		}
	}
	n, err := strconv.Atoi(ref)
	return err == nil && n >= 1 && n <= maxDeleteRefNumber
}

// deleteGuard applies the pre-mutation guard to the resolved candidate:
// a deleted/ file is already-deleted, and without --force anything but a
// main-directory open ticket refuses — both before the prompt. It mirrors
// store.deleteOneLocked's guards (dir derived from the absolute Path).
// refused=true carries the exit code to return.
func deleteGuard(cand store.DeleteCandidate, storeDir string, force bool, lang domain.Lang, stderr io.Writer) (int, bool) {
	dir := filepath.Dir(cand.Path)
	if dir == filepath.Join(storeDir, "deleted") {
		fmt.Fprintln(stderr, domain.ErrDeleteAlreadyDeleted(lang))
		return exitError, true
	}
	if !force && (dir != storeDir || cand.Status != domain.StatusOpen) {
		fmt.Fprintln(stderr, domain.ErrDeleteOnlyOpen(lang, string(cand.Status)))
		return exitError, true
	}
	return exitOK, false
}

// deleteError maps a ResolveDelete/Delete failure to the pinned exit codes
// and localized messages. The store carries no UI text; ambiguity wins with
// exitAmbiguous, the typed guards and collision render their own messages,
// and read-only/wrapped I/O fall through to the single `ticket: ` prefix.
func deleteError(st *store.Store, stderr io.Writer, lang domain.Lang, ref string, err error) int {
	var amb *store.AmbiguousNumberError
	var notOpen *store.NotOpenError
	var collision *store.CollisionError
	var journal *store.ErrInvalidJournalInput
	switch {
	case errors.As(err, &amb):
		return reportAmbiguous(stderr, lang, amb)
	case errors.Is(err, store.ErrAlreadyDeleted):
		fmt.Fprintln(stderr, domain.ErrDeleteAlreadyDeleted(lang))
	case errors.As(err, &notOpen):
		fmt.Fprintln(stderr, domain.ErrDeleteOnlyOpen(lang, string(notOpen.Status)))
	case errors.As(err, &collision):
		fmt.Fprintln(stderr, domain.ErrFileAlreadyExists(lang, collision.Target))
	case errors.Is(err, store.ErrNotFound):
		return notFoundHinted(st, stderr, lang, ref)
	case errors.As(err, &journal):
		fmt.Fprintln(stderr, domain.ErrJournalNewline(lang))
	default:
		fmt.Fprintf(stderr, "ticket: %v\n", err)
	}
	return exitError
}
