package store

// Store operation events (T-0110 W4): every mutation logs the lock lifecycle pair plus one terminal event; failures carry only number/to/stage, never comment/who/title/details/body.
import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"ticket/internal/domain"
	"ticket/internal/lock"
)

type entry struct {
	level slog.Level
	event string
	attrs map[string]string
}
type logRecorder struct {
	mu   sync.Mutex
	recs []entry
}

func (r *logRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *logRecorder) WithAttrs([]slog.Attr) slog.Handler       { return r }
func (r *logRecorder) WithGroup(string) slog.Handler            { return r }
func (r *logRecorder) Handle(_ context.Context, rec slog.Record) error {
	e := entry{rec.Level, rec.Message, map[string]string{}}
	rec.Attrs(func(a slog.Attr) bool { e.attrs[a.Key] = a.Value.String(); return true })
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recs = append(r.recs, e)
	return nil
}
func (r *logRecorder) all() []entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]entry(nil), r.recs...)
}
func (r *logRecorder) find(event string) (out []entry) {
	for _, e := range r.all() {
		if e.event == event {
			out = append(out, e)
		}
	}
	return
}
func (r *logRecorder) reset() { r.mu.Lock(); defer r.mu.Unlock(); r.recs = nil }
func kv(pairs ...string) map[string]string {
	m := make(map[string]string, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return m
}
func lockEv(s *Store, ev string) entry { return entry{slog.LevelDebug, ev, kv("path", s.lockPath())} }
func acq(s *Store) entry               { return lockEv(s, "store.lock.acquired") }
func rel(s *Store) entry               { return lockEv(s, "store.lock.released") }
func checkLog(t *testing.T, rec *logRecorder, wants ...entry) {
	t.Helper()
	if got := rec.all(); !reflect.DeepEqual(got, wants) {
		t.Fatalf("logged %v, want %v", got, wants)
	}
}
func must(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}
func mustFail(t *testing.T, err error) {
	if err == nil {
		t.Fatal("want an error")
	}
}
func newLoggingStore(t *testing.T) (*Store, string, *logRecorder) {
	rec := &logRecorder{}
	s, err := New(t.TempDir(), WithLogger(slog.New(rec)))
	must(t, err)
	return s, s.Dir, rec
}
func seed(t *testing.T, s *Store, rec *logRecorder, status domain.Status) {
	_, err := s.Create(fakeTicket(0))
	must(t, err)
	if status != domain.StatusOpen {
		_, _, err = s.SetStatus(1, status, "tester", "")
		must(t, err)
	}
	rec.reset()
}
func TestStoreLogging_CreateAssigned(t *testing.T) {
	s, _, rec := newLoggingStore(t)
	_, err := s.Create(fakeTicket(0))
	must(t, err)
	checkLog(t, rec, acq(s), rel(s), entry{slog.LevelInfo, "store.number.assigned", kv("number", "1")})
	if len(rec.find("store.number.assigned")) != 1 || rec.find("store.lock.acquired")[0].attrs["path"] != s.lockPath() {
		t.Fatalf("find helper disagrees with the records: %v", rec.all())
	}
}
func TestStoreLogging_LockError(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		s, _, rec := newLoggingStore(t)
		lk, err := lock.AcquireLocked(s.lockPath(), 0)
		must(t, err)
		t.Cleanup(func() { _ = lk.Release() })
		t.Setenv("TICKET_LOCK_TIMEOUT", "0")
		_, cerr := s.Create(fakeTicket(0))
		mustFail(t, cerr)
		checkLog(t, rec, entry{slog.LevelError, "store.lock.error", kv("path", s.lockPath(), "reason", "timeout")})
	})
	t.Run("open error", func(t *testing.T) {
		s, _, rec := newLoggingStore(t)
		must(t, os.Mkdir(s.lockPath(), 0o755))
		_, err := s.Create(fakeTicket(0))
		mustFail(t, err)
		checkLog(t, rec, entry{slog.LevelError, "store.lock.error", kv("path", s.lockPath(), "reason", "error")})
	})
}
func TestStoreLogging_LockFileReplaced_UnixOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows cannot unlink an open lock file")
	}
	s, _, rec := newLoggingStore(t)
	mustFail(t, s.withLock(func() error { return os.Remove(s.lockPath()) }))
	checkLog(t, rec, acq(s), entry{slog.LevelError, "store.lock.error", kv("path", s.lockPath(), "reason", "swapped")}, rel(s))
}
func TestStoreLogging_NumberRetry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory fixture yields ERROR_ACCESS_DENIED, not fs.ErrExist")
	}
	s, _, rec := newLoggingStore(t)
	must(t, os.Mkdir(filepath.Join(s.Dir, "T-0001-open.md"), 0o755))
	_, err := s.Create(fakeTicket(0))
	if !errors.Is(err, ErrCollision) {
		t.Fatalf("Create = %v, want ErrCollision", err)
	}
	wants := []entry{acq(s)}
	for i := 1; i <= MaxCreateAttempts; i++ {
		wants = append(wants, entry{slog.LevelDebug, "store.number.retry", kv("attempt", fmt.Sprint(i), "max", fmt.Sprint(MaxCreateAttempts), "number", "1")})
	}
	checkLog(t, rec, append(wants, rel(s))...)
	if n := len(rec.find("store.number.assigned")); n != 0 {
		t.Fatalf("assigned recorded %d times on an exhausted create", n)
	}
}

// TestStoreLogging_SilentExits: validation, not-found and number-limit exits log only the withLock pair; the non-EEXIST Create open failure has no portable fixture (root/permission tricks only).
func TestStoreLogging_SilentExits(t *testing.T) {
	s, _, rec := newLoggingStore(t)
	seed(t, s, rec, domain.StatusOpen)
	must(t, os.WriteFile(filepath.Join(s.Dir, "T-9999-open.md"), nil, 0o644))
	if _, err := s.Create(fakeTicket(0)); err == nil || !strings.Contains(err.Error(), "4-digit") {
		t.Fatalf("Create = %v, want the 4-digit number-limit error", err)
	}
	_, _, err := s.SetStatus(1, domain.StatusWip, "tester", "bad\ncomment")
	mustFail(t, err)
	_, _, err = s.SetStatus(42, domain.StatusWip, "tester", "")
	mustFail(t, err)
	_, err = s.Archive(42, "tester")
	mustFail(t, err)
	for _, e := range rec.all() {
		if e.event != "store.lock.acquired" && e.event != "store.lock.released" {
			t.Fatalf("earlier exit logged %q: %v", e.event, rec.all())
		}
	}
	if len(rec.find("store.lock.acquired")) != 4 || len(rec.find("store.lock.released")) != 4 {
		t.Fatalf("want one lock pair per call: %v", rec.all())
	}
}
func TestStoreLogging_SetCommit(t *testing.T) {
	s, dir, rec := newLoggingStore(t)
	seed(t, s, rec, domain.StatusOpen)
	_, _, err := s.SetStatus(1, domain.StatusWip, "who-SECRET", "comment-SECRET")
	must(t, err)
	path := filepath.Join(dir, "T-0001-wip.md")
	checkLog(t, rec, acq(s), entry{slog.LevelInfo, "store.set.commit", kv("number", "1", "from", "open", "to", "wip", "path", path)}, rel(s))
	if strings.Contains(fmt.Sprint(rec.all()), "SECRET") {
		t.Fatalf("comment/who leaked into the log: %v", rec.all())
	}
}
func TestStoreLogging_SetAppend(t *testing.T) {
	s, dir, rec := newLoggingStore(t)
	seed(t, s, rec, domain.StatusOpen)
	_, _, err := s.SetStatus(1, domain.StatusOpen, "who-SECRET", "comment-SECRET")
	must(t, err)
	path := filepath.Join(dir, "T-0001-open.md")
	checkLog(t, rec, acq(s), entry{slog.LevelInfo, "store.set.append", kv("number", "1", "status", "open", "path", path)}, rel(s))
	if strings.Contains(fmt.Sprint(rec.all()), "SECRET") {
		t.Fatalf("comment/who leaked into the log: %v", rec.all())
	}
}
func TestStoreLogging_ArchiveCommit(t *testing.T) {
	s, dir, rec := newLoggingStore(t)
	seed(t, s, rec, domain.StatusDone)
	got, err := s.Archive(1, "who-SECRET")
	must(t, err)
	target := filepath.Join(dir, "archive", "T-0001-done.md")
	checkLog(t, rec, acq(s), entry{slog.LevelInfo, "store.archive.commit", kv("number", "1", "status", "done", "path", target)}, rel(s))
	if got != target || strings.Contains(fmt.Sprint(rec.all()), "SECRET") {
		t.Fatalf("Archive = %q, want %q without a who leak", got, target)
	}
}
