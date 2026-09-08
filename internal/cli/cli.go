// Package cli implements the ticket command-line interface: argument
// routing, tickets-dir resolution, and shared wiring for the commands.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ticket/internal/domain"
	"ticket/internal/paths"
	"ticket/internal/store"
)

// Run extracts a leading global tickets-dir flag, then dispatches args[0] to the command handler and returns the process
// exit code. args must not include the program name (main strips argv[0]);
// unit callers pass the command name and its arguments directly. No command
// given means help (bash:147 cmd="${1:-help}"); an unknown command prints
// usage to stdout and exits 1 (bash:154). "version", "--version" and "-v"
// print the version line to stdout and exit 0 without dispatch: like help,
// they skip tickets-dir resolution and work outside a project; by the same
// token, the subcommand -h/--help interception (T-0041) below returns
// before dispatch and thus also skips tickets-dir resolution.
//
// Command handlers (cmd_new.go, cmd_list.go, cmd_show.go, cmd_set.go,
// cmd_archive.go) share the common shape
//
//	func cmdX(st *store.Store, args []string, who, project string, stdout, stderr io.Writer) int
//
// where cmdNew/cmdShow/cmdSet/cmdList take lang (the resolved ticket
// language: langFrom, TICKET_LANG → LC_ALL → LANG; new in T-0026, no bash
// precedent; since T-0036 a domain.Lang that also selects the file format
// of `new`; T-0040 added lang to cmdList for i18n noTickets/warnings),
// and cmdArchive omits it. st is built
// here from the tickets dir resolved ONCE, who is the resolved user
// (TICKET_WHO → USER → USERNAME → agent, bash:12) and project is
// filepath.Base(filepath.Dir(dir)) (bash:10).

// extractGlobalDir scans args for a leading -C/--tickets-dir flag and returns
// the resolved directory plus the remaining (post-flag) args. A trailing
// flag with no value (`-C` / `--tickets-dir` at the end) or with an empty
// value (`--tickets-dir=` / `-C ""` / `--tickets-dir ""`) is reported as
// err so Run can fail fast before command routing — silent fallthrough
// degraded `ticket -C` into help (exit 0, 47 lines of usage). Glued
// `-C=` is intentionally left to paths.Resolve (dir "=" → later error),
// and glued `-Cpath` keeps working as before.
func extractGlobalDir(args []string) (dir string, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--tickets-dir" || a == "-C":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("ticket: flag %s requires a value", a)
			}
			if args[i+1] == "" {
				return "", nil, fmt.Errorf("ticket: flag %s requires a non-empty value", a)
			}
			dir = args[i+1]
			i++
			continue
		case strings.HasPrefix(a, "--tickets-dir="):
			val := strings.SplitN(a, "=", 2)[1]
			if val == "" {
				return "", nil, fmt.Errorf("ticket: flag %s requires a non-empty value", a)
			}
			dir = val
			continue
		case strings.HasPrefix(a, "-C") && len(a) > 2:
			dir = a[2:]
			continue
		default:
			return dir, args[i:], nil
		}
	}
	return dir, args[len(args):], nil
}

func Run(args []string, env map[string]string, stdout, stderr io.Writer) int {
	flagDir, args, err := extractGlobalDir(args)
	if err != nil {
		// Missing/empty -C/--tickets-dir is a one-line user error: print and
		// exit 1 BEFORE any command routing (incl. help/version/unknown) —
		// matches the dispatch/paths error style.
		fmt.Fprintln(stderr, err)
		return 1
	}
	cmd := "help"
	if len(args) > 0 {
		cmd = args[0]
	}
	lang := langFrom(env)
	switch cmd {
	case "init":
		if len(args) > 1 {
			if args[1] == "-h" || args[1] == "--help" {
				usage(stdout, lang)
				return 0
			}
			usage(stdout, lang)
			return 1
		}
		return cmdInit(stdout, stderr)
	case "new", "list", "show", "set", "archive":
		// Subcommand help interception (T-0041): -h/--help as the FIRST
		// argument is never a valid positional (new: title; show/set/archive:
		// ticket number; list: filter), so print usage and exit 0. Only
		// args[1] is checked: flag values (-d text, -P project) and the
		// subsequent positionals (incl. the set comment) all land in args[2+],
		// so a --help there must NOT trigger help ("new Тикет -d \"see
		// --help\"" stays a real ticket creation).
		if len(args) > 1 && (args[1] == "-h" || args[1] == "--help") {
			usage(stdout, lang)
			return 0
		}
		return dispatch(cmd, args[1:], env, lang, flagDir, stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout, lang)
		return 0
	case "version", "--version", "-v":
		_, _ = io.WriteString(stdout, versionLine())
		return 0
	default:
		usage(stdout, lang)
		return 1
	}
}

// dispatch resolves the tickets dir once using flag, environment, then cwd.
// lang is passed to handlers that print usage or i18n messages
// (new/show/set/list); archive omits it.
func dispatch(cmd string, args []string, env map[string]string, lang domain.Lang, flagDir string, stdout, stderr io.Writer) int {
	// Getwd/Executable failures are handled by paths.Resolve input checks.
	cwd, _ := os.Getwd()
	dir, err := paths.Resolve(env, cwd, flagDir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	// Only mutating commands open the Store writable (write probe);
	// list/show use NewReadOnly, which never mutates the tickets dir.
	var st *store.Store
	if cmd == "new" || cmd == "set" || cmd == "archive" {
		st, err = store.New(dir)
	} else { // list, show
		st, err = store.NewReadOnly(dir)
	}
	if err != nil {
		// store errors are user-facing and self-prefixed ("store: ...");
		// adding die()'s "ticket: " here would double-prefix (wave-1 #1).
		fmt.Fprintln(stderr, err)
		return 1
	}
	who := whoFrom(env)
	// T-0065: defense-in-depth. Reject CR/LF in TICKET_WHO/USER before
	// dispatching the command — the store also guards (it's the truth),
	// but a localized message is friendlier than the store's
	// "store: journal who must not contain a line break" line. cmdSet
	// and cmdArchive re-validate the comment and the merged `who` per
	// the same contract.
	if strings.ContainsAny(who, "\r\n") {
		fmt.Fprintln(stderr, domain.ErrJournalNewline(lang))
		return 1
	}
	project := filepath.Base(filepath.Dir(dir))
	switch cmd {
	case "new":
		return cmdNew(st, args, who, project, lang, stdout, stderr)
	case "list":
		return cmdList(st, args, who, project, lang, stdout, stderr)
	case "show":
		return cmdShow(st, args, who, project, lang, stdout, stderr)
	case "set":
		return cmdSet(st, args, who, project, lang, stdout, stderr)
	default: // "archive"
		return cmdArchive(st, args, who, project, stdout, stderr)
	}
}

// whoFrom resolves the current user: TICKET_WHO, then USER, then USERNAME,
// then "agent" (bash:12 WHO="${TICKET_WHO:-${USER:-agent}}"; USERNAME
// covers Windows hosts where USER is normally unset).
func whoFrom(env map[string]string) string {
	if who := env["TICKET_WHO"]; who != "" {
		return who
	}
	if who := env["USER"]; who != "" {
		return who
	}
	if who := env["USERNAME"]; who != "" {
		return who
	}
	return "agent"
}

// langFrom resolves the ticket language (T-0036: one language for both
// the new-ticket file format and the usage text): TICKET_LANG, then
// LC_ALL, then LANG. An empty value counts as unset and falls through to
// the next variable; the first non-empty one decides via langPrefix. All
// unset defaults to LangRU (language selection itself is new in T-0026
// and has no bash precedent).
func langFrom(env map[string]string) domain.Lang {
	for _, key := range []string{"TICKET_LANG", "LC_ALL", "LANG"} {
		if v := env[key]; v != "" {
			return langPrefix(v)
		}
	}
	return domain.LangRU
}

// langPrefix maps a locale value to a domain.Lang: the prefix up to the
// first '_' or '.' decides, case-insensitive ("en_US.UTF-8" → en,
// "ru_RU" → ru). If the prefix matches a known language in the registry,
// that language is selected; otherwise falls back to LangEN (T-0036
// deliberate change: unknown locales used to get the Russian text).
func langPrefix(v string) domain.Lang {
	if i := strings.IndexAny(v, "_."); i >= 0 {
		v = v[:i]
	}
	lang := domain.Lang(strings.ToLower(v))
	if domain.IsKnownLang(lang) {
		return lang
	}
	return domain.LangEN // fallback for unknown locales
}
