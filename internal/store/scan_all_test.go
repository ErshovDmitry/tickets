package store

// T-0241 code review R2.4: the warning attribution rule of the show read path.
// The end-to-end shape (a rejected same-named file in archive/ plus a readable
// one in s.Dir) needs a non-regular entry and therefore a platform symlink, so
// the rule is pinned directly on the scan value instead.

import (
	"errors"
	"testing"
)

// TestLocatedScanAttributableWarningsByDirectory pins the R2.4 rule: a warning
// is attributed only when it comes from the located file's OWN directory AND
// names its base name. A same-named file in archive/ (a rejected symlink, a
// bad entry) is a different file and must never explain the read in s.Dir.
func TestLocatedScanAttributableWarningsByDirectory(t *testing.T) {
	const name = "T-0001-open.md"
	mainWarn := ParseWarning{Name: name, Err: errors.New("main-dir")}
	archiveWarn := ParseWarning{Name: name, Err: errors.New("archive-dir")}
	sc := locatedScan{dirs: []scannedDir{
		{dir: "/t", warns: []ParseWarning{mainWarn, {Name: "T-0002-open.md", Err: errors.New("other-file")}}},
		{dir: "/t/archive", warns: []ParseWarning{archiveWarn}},
	}}
	loc := locatedFile{fileEntry: fileEntry{Name: name}, dir: "/t"}

	got := sc.attributableWarnings(loc)
	if len(got) != 1 {
		t.Fatalf("attributableWarnings = %v, want exactly the one main-dir warning", got)
	}
	if !errors.Is(got[0].Err, mainWarn.Err) {
		t.Errorf("attributed %v, want the s.Dir warning (the archive one is another file)", got[0])
	}

	// The same file name resolved inside archive/ gets the archive warning.
	loc.dir = "/t/archive"
	got = sc.attributableWarnings(loc)
	if len(got) != 1 || !errors.Is(got[0].Err, archiveWarn.Err) {
		t.Errorf("attributableWarnings for archive = %v, want the archive-dir warning", got)
	}
}
