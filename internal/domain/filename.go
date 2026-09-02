package domain

import (
	"fmt"
	"regexp"
	"strconv"
)

// filenameRe matches canonical ticket file names: T-NNNN-<status>.md.
// Exactly four digits — five or more (T-00001-…) are rejected.
var filenameRe = regexp.MustCompile(`^T-(\d{4})-([a-z]+)\.md$`)

// Filename returns the canonical file name for a ticket: T-NNNN-<status>.md.
func Filename(n int, st Status) string {
	return fmt.Sprintf("T-%04d-%s.md", n, st)
}

// ParseFilename extracts the ticket number and status from a file name.
func ParseFilename(name string) (int, Status, error) {
	m := filenameRe.FindStringSubmatch(name)
	if m == nil {
		return 0, "", fmt.Errorf("имя файла %q не соответствует T-NNNN-<статус>.md", name)
	}
	n, _ := strconv.Atoi(m[1]) // regex guarantees exactly four digits
	return n, Status(m[2]), nil
}
