package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// withRunSeams swaps the Run seams for the duration of one test. These tests
// must not run in parallel: the seams are package-level state.
func withRunSeams(t *testing.T, initFn func() error, langFn func(map[string]string) domain.Lang) {
	t.Helper()
	prevInit, prevLang := domainInitFn, langFromFn
	domainInitFn, langFromFn = initFn, langFn
	t.Cleanup(func() { domainInitFn, langFromFn = prevInit, prevLang })
}

func TestRunVersionBypassesInitAndLang(t *testing.T) {
	initCalls, langCalls := 0, 0
	withRunSeams(t,
		func() error { initCalls++; return errors.New("init must not be called") },
		func(map[string]string) domain.Lang { langCalls++; return domain.LangEN },
	)

	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}} {
		var stdout, stderr bytes.Buffer
		code := Run(args, map[string]string{"TICKET_LANG": "en"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("Run(%q) = %d, want 0", args, code)
		}
		if got, want := stdout.String(), "ticket version dev\n"; got != want {
			t.Fatalf("Run(%q) stdout = %q, want %q", args, got, want)
		}
		if stderr.Len() != 0 {
			t.Fatalf("Run(%q) stderr = %q, want empty", args, stderr.String())
		}
	}
	if initCalls != 0 {
		t.Fatalf("domain init called %d times on version paths, want 0", initCalls)
	}
	if langCalls != 0 {
		t.Fatalf("langFrom called %d times on version paths, want 0", langCalls)
	}
}

func TestRunFatalInitStopsBeforeLangAndUsage(t *testing.T) {
	langCalls := 0
	initCalls := 0
	fatal := fmt.Errorf("%w: en.json broken", domain.ErrInitFatal)
	withRunSeams(t,
		func() error { initCalls++; return fatal },
		func(map[string]string) domain.Lang { langCalls++; return domain.LangRU },
	)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"help"}, map[string]string{"TICKET_LANG": "ru"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(help) = %d, want 1 on fatal init", code)
	}
	if initCalls != 1 {
		t.Fatalf("domain init called %d times, want 1", initCalls)
	}
	if langCalls != 0 {
		t.Fatalf("langFrom called %d times after fatal init, want 0", langCalls)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty (no usage on fatal init)", stdout.String())
	}
	if got, want := stderr.String(), fatal.Error()+"\n"; got != want {
		t.Fatalf("stderr = %q, want exactly one plain line %q", got, want)
	}
	if strings.Count(strings.TrimSuffix(stderr.String(), "\n"), "\n") != 0 {
		t.Fatalf("stderr must be a single line, got %q", stderr.String())
	}
}

func TestRunDegradedInitWarnsOnceAndContinues(t *testing.T) {
	degraded := fmt.Errorf("%w: ru.json broken", domain.ErrInitDegraded)
	withRunSeams(t,
		func() error { return degraded },
		func(map[string]string) domain.Lang { return domain.LangRU },
	)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"help"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(help) = %d, want 0 on degraded init", code)
	}
	if got, want := stderr.String(), degraded.Error()+"\n"; got != want {
		t.Fatalf("stderr = %q, want one warning %q", got, want)
	}
	if got, want := stdout.String(), versionLine()+usageTextRU; got != want {
		t.Fatalf("stdout = %d bytes, want version line + RU usage (%d bytes)", len(got), len(want))
	}
}

func TestRunResolvesLangOnceForHelpAndDispatch(t *testing.T) {
	langCalls := 0
	withRunSeams(t,
		func() error { return nil },
		func(map[string]string) domain.Lang { langCalls++; return domain.LangRU },
	)

	cases := []struct {
		args     []string
		wantCode int
	}{
		{[]string{"help"}, 0},
		{[]string{"show", "-h"}, 0},
		{[]string{"unknown"}, 1},
	}
	for _, tc := range cases {
		langCalls = 0
		var stdout, stderr bytes.Buffer
		if code := Run(tc.args, nil, &stdout, &stderr); code != tc.wantCode {
			t.Fatalf("Run(%q) = %d, want %d", tc.args, code, tc.wantCode)
		}
		if langCalls != 1 {
			t.Fatalf("Run(%q) resolved lang %d times, want 1", tc.args, langCalls)
		}
	}
}
