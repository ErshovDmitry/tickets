package store

import (
	"errors"
	"io/fs"
)

// ParseWarning reasons. A reason is a fixed, machine-readable enum that
// names why a ticket file could not be used; it is assigned by the
// producer of the warning and never derived from ticket content, so it
// carries no ticket text and no secrets.
const (
	reasonIOError        = "io-error"
	reasonH1Mismatch     = "h1-mismatch"
	reasonNotRegularFile = "not-regular-file"
	reasonBadExtension   = "bad-extension"
	reasonBadName        = "bad-name"
	reasonFileSwapped    = "file-swapped"
	reasonEmptyFile      = "empty-file"
	reasonJournalDup     = "journal-dup"
	reasonDirRead        = "dir-read"
	reasonArchiveDir     = "archive-dir"
	reasonParse          = "parse"
)

// reasonOf maps an error raised while reading a ticket file to its
// reason. It walks the error chain (errors.Is / errors.As, so a wrapped
// cause still maps — Context7 /golang/go: %w creates Unwrap, %v does
// not) and falls back to reasonParse for an error it does not know.
// That fallback has no current producer: bad-name, journal-dup and
// dir-read are always assigned by their producer, never derived here.
func reasonOf(err error) string {
	switch {
	case errors.Is(err, errFileSwapped):
		return reasonFileSwapped
	case errors.Is(err, errEmptyTicket):
		return reasonEmptyFile
	case errors.Is(err, errNotRegularFile):
		return reasonNotRegularFile
	case errors.Is(err, errBadExtension):
		return reasonBadExtension
	}
	if _, ok := errors.AsType[*h1MismatchError](err); ok {
		return reasonH1Mismatch
	}
	if _, ok := errors.AsType[*fs.PathError](err); ok {
		return reasonIOError
	}
	return reasonParse
}

// archiveReason is reasonOf for the protected archive/ directory: an
// invalid archive path keeps its own reason, and an OS-level failure
// there is its own read problem rather than a ticket parse issue.
func archiveReason(err error) string {
	if errors.Is(err, ErrArchiveInvalid) {
		return reasonArchiveDir
	}
	if _, ok := errors.AsType[*fs.PathError](err); ok {
		return reasonIOError
	}
	return reasonOf(err)
}
