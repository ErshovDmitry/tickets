//go:build unix

package store

import (
	"os"
	"syscall"
)

// openNoFollowBlock opens path read-only with O_NOFOLLOW|O_NONBLOCK.
// O_NONBLOCK makes a read-only open of a FIFO return immediately
// instead of blocking until a writer connects (open(2)) — without it a
// ticket file swapped for a FIFO mid-scan hangs the reader forever.
// On regular files the flag is a no-op for reading. O_NOFOLLOW fails
// the open with ELOOP when the final path component is a symlink, so
// an outside target behind a swapped-in link is never opened. The
// Lstat precheck and the opened-handle SameFile guard in openValidated
// stay in force.
func openNoFollowBlock(path string) (*os.File, error) {
	return os.OpenFile(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
