package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// maxProbeAttempts caps probeWritable's unique-name retries.
const maxProbeAttempts = 5

// probeWritable checks that dir accepts file creation. The probe creates
// a UNIQUE O_EXCL file (.write-probe.<pid>-<nanos>) and removes it again,
// so a pre-existing .write-probe — or any other user file — is never
// opened for writing, truncated, or deleted (T-0007). An ErrExist just
// means the unique name collided; the probe retries with a fresh
// timestamp. Creation alone proves write access, nothing is written.
func probeWritable(dir string) error {
	var lastErr error
	for attempt := 0; attempt < maxProbeAttempts; attempt++ {
		probe := filepath.Join(dir, fmt.Sprintf(".write-probe.%d-%d", os.Getpid(), nanoNow()))
		f, err := os.OpenFile(probe, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			if errors.Is(err, fs.ErrExist) {
				lastErr = err
				continue
			}
			return fmt.Errorf("store: probe write %s: %w", probe, err)
		}
		cerr := f.Close()
		rerr := os.Remove(probe)
		if cerr != nil {
			return fmt.Errorf("store: probe close %s: %w", probe, cerr)
		}
		if rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			return fmt.Errorf("store: probe remove %s: %w", probe, rerr)
		}
		return nil
	}
	return fmt.Errorf("store: probe name after %d attempts, last err: %v", maxProbeAttempts, lastErr)
}
