//go:build windows

package store

import "os"

// openNoFollowBlock opens path read-only. The FIFO open-blocking attack
// vector does not exist through filesystem paths on Windows (named
// pipes live under the \\.\pipe\ device namespace, not regular
// directories), so plain os.Open suffices. Residual risk: os.Open
// follows symlinks — the Lstat precheck and the opened-handle SameFile
// guard in openValidated remain the TOCTOU containment.
func openNoFollowBlock(path string) (*os.File, error) {
	return os.Open(path)
}
