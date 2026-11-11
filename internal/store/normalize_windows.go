//go:build windows

package store

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func init() {
	prepareReplace = prepareReplaceWindows
}

// prepareReplace clears the read-only attribute of path so the following
// rename may replace it, and returns a restore func that puts the original
// attributes back (a no-op when the file never was read-only).
//
// The attribute must go BEFORE the rename: MoveFileEx(MOVEFILE_REPLACE_EXISTING)
// over a read-only target fails with ERROR_ACCESS_DENIED, which renameFile
// classifies as transient and retries for its full two-second budget
// (rename_windows.go) before giving up — a whole run's worth of latency for a
// condition that never clears by itself. Every other attribute a ticket file
// can carry (hidden, archive, system) does not block the replacement, so only
// the read-only bit is touched.
//
// The returned restore func is never nil on success, so callers may invoke it
// unconditionally after the rename, on the success and on the failure path.
//
// It is installed as the package hook: prepareReplace has a no-op default in
// store_normalize.go, and init replaces it on Windows.
func prepareReplaceWindows(path string) (func() error, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("store: path %s: %w", path, err)
	}
	attrs, err := windows.GetFileAttributes(name)
	if err != nil {
		return nil, fmt.Errorf("store: read attributes %s: %w", path, err)
	}
	if attrs&windows.FILE_ATTRIBUTE_READONLY == 0 {
		return func() error { return nil }, nil
	}
	if err := windows.SetFileAttributes(name, attrs&^windows.FILE_ATTRIBUTE_READONLY); err != nil {
		return nil, fmt.Errorf("store: clear read-only %s: %w", path, err)
	}
	restore := func() error {
		if rerr := windows.SetFileAttributes(name, attrs); rerr != nil {
			return fmt.Errorf("store: restore attributes %s: %w", path, rerr)
		}
		return nil
	}
	return restore, nil
}
