package domain

import (
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadFromFS(t *testing.T) {
	// No t.Parallel anywhere in this file: the tests replace/snapshot the
	// package-level dictionaries.
	tests := []struct {
		name       string
		build      func(t *testing.T) fs.FS
		wantErr    error
		wantNotErr error
		check      func(t *testing.T, c candidate)
	}{
		{
			name:  "successful discovery",
			build: func(t *testing.T) fs.FS { return embeddedLangsMapFS(t) },
			check: func(t *testing.T, c candidate) {
				t.Helper()
				if c.registry[LangRU] == nil || c.registry[LangEN] == nil {
					t.Fatalf("registry misses RU/EN: %v", c.registry)
				}
				if c.dictRU == nil || c.dictEN == nil {
					t.Fatal("dictRU/dictEN must be non-nil")
				}
				if len(c.allDicts) < 2 || c.allDicts[0].lang != LangRU || c.allDicts[1].lang != LangEN {
					t.Fatalf("allDicts order = %+v, want RU then EN", c.allDicts)
				}
			},
		},
		{
			name: "non-json entries are ignored",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/README.txt"] = &fstest.MapFile{Data: []byte("not a dictionary")}
				m["langs/nested"] = &fstest.MapFile{Mode: fs.ModeDir}
				return m
			},
			check: func(t *testing.T, c candidate) {
				t.Helper()
				if len(c.registry) != 2 {
					t.Fatalf("registry = %v, want exactly RU+EN", c.registry)
				}
			},
		},
		{
			name: "readdir failure is fatal",
			build: func(t *testing.T) fs.FS {
				return failingLangsFS{fsys: embeddedLangsMapFS(t), failDir: true}
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "missing EN is fatal",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				delete(m, "langs/en.json")
				return m
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "EN read error is fatal",
			build: func(t *testing.T) fs.FS {
				return failingLangsFS{
					fsys:     embeddedLangsMapFS(t),
					failRead: map[string]bool{"langs/en.json": true},
				}
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "malformed EN is fatal",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/en.json"] = &fstest.MapFile{Data: []byte("{")}
				return m
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "duplicate EN code is fatal",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/EN.json"] = &fstest.MapFile{Data: m["langs/en.json"].Data}
				return m
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "RU read error degrades with partial candidate",
			build: func(t *testing.T) fs.FS {
				return failingLangsFS{
					fsys:     embeddedLangsMapFS(t),
					failRead: map[string]bool{"langs/ru.json": true},
				}
			},
			wantErr:    ErrInitDegraded,
			wantNotErr: ErrInitFatal,
			check:      checkDegradedRU,
		},
		{
			name: "malformed RU degrades with partial candidate",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/ru.json"] = &fstest.MapFile{Data: []byte("{")}
				return m
			},
			wantErr:    ErrInitDegraded,
			wantNotErr: ErrInitFatal,
			check:      checkDegradedRU,
		},
		{
			name: "duplicate RU code is fatal",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/RU.json"] = &fstest.MapFile{Data: m["langs/ru.json"].Data}
				return m
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "duplicate stays fatal when the first copy is malformed",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				// RU.json (earlier in ReadDir order) is malformed, so the first
				// copy is actually parsed and fails; reaching RU.json must still
				// return the fatal duplicate class, not a degraded parse error.
				m["langs/RU.json"] = &fstest.MapFile{Data: []byte("{")}
				return m
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "duplicate third optional code is fatal",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/xx.json"] = &fstest.MapFile{Data: m["langs/en.json"].Data}
				m["langs/XX.json"] = &fstest.MapFile{Data: m["langs/en.json"].Data}
				return m
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "third optional languages follow EN alphabetically",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/de.json"] = &fstest.MapFile{Data: m["langs/en.json"].Data}
				m["langs/cs.json"] = &fstest.MapFile{Data: m["langs/en.json"].Data}
				return m
			},
			check: func(t *testing.T, c candidate) {
				t.Helper()
				if len(c.registry) != 4 {
					t.Fatalf("registry = %v, want RU+EN+cs+de", c.registry)
				}
				want := []Lang{LangRU, LangEN, "cs", "de"}
				if len(c.allDicts) != len(want) {
					t.Fatalf("allDicts = %+v, want %v", c.allDicts, want)
				}
				for i, lang := range want {
					if c.allDicts[i].lang != lang {
						t.Fatalf("allDicts[%d].lang = %q, want %q (order %v)", i, c.allDicts[i].lang, lang, want)
					}
					if c.allDicts[i].d == nil {
						t.Fatalf("allDicts[%d] has a nil dict", i)
					}
				}
			},
		},
		{
			name: "failed third optional language degrades and is not registered",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/xx.json"] = &fstest.MapFile{Data: []byte("{")}
				return m
			},
			wantErr:    ErrInitDegraded,
			wantNotErr: ErrInitFatal,
			check: func(t *testing.T, c candidate) {
				t.Helper()
				if _, ok := c.registry[Lang("xx")]; ok {
					t.Fatal("a broken third language must not be registered")
				}
				if len(c.registry) != 2 || c.dictRU == nil || c.dictEN == nil {
					t.Fatalf("registry = %v, want RU+EN only", c.registry)
				}
				if len(c.allDicts) != 2 {
					t.Fatalf("allDicts = %+v, want RU+EN only", c.allDicts)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := snapshotI18nGlobals()
			defer func() {
				if !reflect.DeepEqual(snapshotI18nGlobals(), before) {
					t.Fatal("loadFromFS mutated package globals")
				}
			}()

			cand, err := loadFromFS(tc.build(t))
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("loadFromFS error = %v, want nil", err)
				}
			} else {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("loadFromFS error = %v, want errors.Is(%v)", err, tc.wantErr)
				}
				if tc.wantNotErr != nil && errors.Is(err, tc.wantNotErr) {
					t.Fatalf("error %v also matches %v, classes must be exclusive", err, tc.wantNotErr)
				}
				if tc.wantErr == ErrInitDegraded && strings.Contains(err.Error(), "\n") {
					t.Fatalf("degraded error must be one line, got %q", err.Error())
				}
			}
			if tc.check != nil {
				tc.check(t, cand)
			}
		})
	}
}

func checkDegradedRU(t *testing.T, c candidate) {
	t.Helper()
	if c.dictEN == nil {
		t.Fatal("dictEN must stay non-nil in a degraded candidate")
	}
	if c.dictRU != nil {
		t.Fatal("dictRU must be nil when ru.json failed")
	}
	if _, ok := c.registry[LangRU]; ok {
		t.Fatal("a broken RU must not be registered (no alias)")
	}
	if len(c.allDicts) == 0 || c.allDicts[0].lang != LangEN {
		t.Fatalf("allDicts = %+v, want EN first when RU is unavailable", c.allDicts)
	}
}
