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

// Run dispatches args[0] to the command handler and returns the process
// exit code. args must not include argv[0]: main passes os.Args[1:], unit
// callers pass the command name and its arguments directly. No command
// given means help (bash:147 cmd="${1:-help}"); an unknown command prints
// usage to stdout and exits 1 (bash:154). "version", "--version" and "-v"
// print the version line to stdout and exit 0 without dispatch: like help,
// they skip tickets-dir resolution and work outside a project.
//
// Command handlers (cmd_new.go, cmd_list.go, cmd_show.go, cmd_set.go,
// cmd_archive.go) share the common shape
//
//	func cmdX(st *store.Store, args []string, who, project string, stdout, stderr io.Writer) int
//
// where cmdNew/cmdShow/cmdSet additionally take lang (the resolved ticket
// language: langFrom, TICKET_LANG → LC_ALL → LANG; new in T-0026, no bash
// precedent; since T-0036 a domain.Lang that also selects the file format
// of `new`) after project, and cmdList/cmdArchive omit it. st is built
// here from the tickets dir resolved ONCE, who is the resolved user
// (TICKET_WHO → USER → USERNAME → agent, bash:12) and project is
// filepath.Base(filepath.Dir(dir)) (bash:10).
func Run(args []string, env map[string]string, stdout, stderr io.Writer) int {
	cmd := "help"
	if len(args) > 0 {
		cmd = args[0]
	}
	lang := langFrom(env)
	switch cmd {
	case "new", "list", "show", "set", "archive":
		return dispatch(cmd, args[1:], env, lang, stdout, stderr)
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

// dispatch resolves the tickets dir once and invokes the command handler.
// lang is passed only to the handlers that can print usage (new/show/set);
// list/archive never print it.
func dispatch(cmd string, args []string, env map[string]string, lang domain.Lang, stdout, stderr io.Writer) int {
	// Getwd/Executable failures are handled by paths.Resolve input checks.
	cwd, _ := os.Getwd()
	exe, _ := os.Executable()
	dir, err := paths.Resolve(env, cwd, exe)
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
	project := filepath.Base(filepath.Dir(dir))
	switch cmd {
	case "new":
		return cmdNew(st, args, who, project, lang, stdout, stderr)
	case "list":
		return cmdList(st, args, who, project, stdout, stderr)
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
// "ru_RU" → ru). A known "ru"/"en" prefix selects that language; any
// other non-empty locale falls back to LangEN (T-0036 deliberate change:
// unknown locales used to get the Russian text).
func langPrefix(v string) domain.Lang {
	if i := strings.IndexAny(v, "_."); i >= 0 {
		v = v[:i]
	}
	switch strings.ToLower(v) {
	case "ru":
		return domain.LangRU
	default: // "en" and unknown locales
		return domain.LangEN
	}
}
