//go:build windows

package store

// Windows-only coverage for the normalize attribute hook (T-0125). The POSIX
// default of prepareReplace is a no-op and is exercised by the store tests on
// every POSIX run; the Windows override can only be executed here, on a real
// Windows host (dser97): GOOS=windows go test -c ./internal/store, copy the
// .exe over, run it with "-test.run=TestPrepareReplaceWindows".

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// readAttrs returns the Windows attribute word of path.
func readAttrs(t *testing.T, path string) uint32 {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("UTF16PtrFromString: %v", err)
	}
	attrs, err := windows.GetFileAttributes(name)
	if err != nil {
		t.Fatalf("GetFileAttributes: %v", err)
	}
	return attrs
}

// TestPrepareReplaceWindows_ReadOnlyTarget pins the whole hook: the read-only
// attribute blocks MoveFileEx, so prepareReplaceWindows must clear it, the
// rename must then succeed, and restore must put the attribute back.
func TestPrepareReplaceWindows_ReadOnlyTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "T-0001-open.md")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(target)
	if err != nil {
		t.Fatalf("UTF16PtrFromString: %v", err)
	}
	if err := windows.SetFileAttributes(name, windows.FILE_ATTRIBUTE_READONLY); err != nil {
		t.Fatalf("SetFileAttributes: %v", err)
	}

	restore, err := prepareReplaceWindows(target)
	if err != nil {
		t.Fatalf("prepareReplaceWindows: %v", err)
	}
	if restore == nil {
		t.Fatal("restore = nil, want a callable func")
	}
	if attrs := readAttrs(t, target); attrs&windows.FILE_ATTRIBUTE_READONLY != 0 {
		t.Fatalf("attribute not cleared: %#x", attrs)
	}

	tmp := filepath.Join(dir, "T-0001-open.md.tmp")
	if err := os.WriteFile(tmp, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := renameFile(tmp, target); err != nil {
		t.Fatalf("rename over a read-only target: %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, []byte("new")) {
		t.Fatalf("content = %q, %v; want new", got, err)
	}
	if err := restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if attrs := readAttrs(t, target); attrs&windows.FILE_ATTRIBUTE_READONLY == 0 {
		t.Fatalf("attribute not restored: %#x", attrs)
	}
}

// TestPrepareReplaceWindows_WritableTarget pins the no-op path: a writable
// target needs no attribute change, and the returned restore is still
// callable (callers invoke it unconditionally).
func TestPrepareReplaceWindows_WritableTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "T-0002-open.md")
	if err := os.WriteFile(path, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := readAttrs(t, path)

	restore, err := prepareReplaceWindows(path)
	if err != nil {
		t.Fatalf("prepareReplaceWindows: %v", err)
	}
	if restore == nil {
		t.Fatal("restore = nil, want a callable func")
	}
	if err := restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if after := readAttrs(t, path); after != before {
		t.Errorf("attributes changed: %#x -> %#x", before, after)
	}
}
