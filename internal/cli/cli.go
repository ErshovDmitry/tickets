// Package cli implements the ticket command-line interface: argument
// routing, tickets-dir resolution, and shared wiring for the commands.
package cli

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
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
// given means help; an unknown command prints usage to stdout and exits 1.
// The leading global flags (-C/--tickets-dir and --verbose) are consumed here.
// "version", "--version" and "-v"
// print the version line to stdout and exit 0 without dispatch: like help,
// they skip tickets-dir resolution and work outside a project; by the same
// token, the subcommand -h/--help interception (T-0041) below returns
// before dispatch and thus also skips tickets-dir resolution.
//
// Command handlers (cmd_new.go, cmd_list.go, cmd_show.go, cmd_set.go,
// cmd_archive.go, cmd_recover.go) share the common shape
//
//	func cmdX(st *store.Store, args []string, who, project string, stdout, stderr io.Writer) int
//
// where cmdNew/cmdShow/cmdSet/cmdList take lang (the resolved ticket
// language: langFrom, TICKET_LANG → LC_ALL → LANG; new in T-0026, with no
// predecessor in the pre-Go era; since T-0036 a domain.Lang that also selects the file format
// of `new`; T-0040 added lang to cmdList for i18n noTickets/warnings),
// and so does cmdArchive since T-0080. st is built
// here from the tickets dir resolved ONCE, who is the resolved user
// (TICKET_WHO → USER → USERNAME → agent) and project is
// filepath.Base(filepath.Dir(dir)).

// extractGlobalFlags scans args for the LEADING global flags and returns the
// tickets dir, the --verbose switch and the remaining (post-flag) args. The
// scan stops at the first non-flag token, so a --verbose AFTER the command
// stays a subcommand argument. A trailing flag with no value (`-C` /
// `--tickets-dir` at the end) or with an empty value (`--tickets-dir=` /
// `-C ""` / `--tickets-dir ""`) is reported as err so Run can fail fast
// before command routing — silent fallthrough degraded `ticket -C` into help
// (exit 0, 47 lines of usage). Glued `-C=` is intentionally left to
// paths.Resolve (dir "=" → later error), and glued `-Cpath` keeps working as
// before. A -C value is taken blindly, so `-C --verbose` swallows the switch
// as a dir value.
func extractGlobalFlags(args []string) (dir string, verbose bool, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--verbose":
			verbose = true
			continue
		case a == "--tickets-dir" || a == "-C":
			if i+1 >= len(args) {
				return "", false, nil, fmt.Errorf("ticket: flag %s requires a value", a)
			}
			if args[i+1] == "" {
				return "", false, nil, fmt.Errorf("ticket: flag %s requires a non-empty value", a)
			}
			dir = args[i+1]
			i++
			continue
		case strings.HasPrefix(a, "--tickets-dir="):
			val := strings.SplitN(a, "=", 2)[1]
			if val == "" {
				return "", false, nil, fmt.Errorf("ticket: flag %s requires a non-empty value", a)
			}
			dir = val
			continue
		case strings.HasPrefix(a, "-C") && len(a) > 2:
			dir = a[2:]
			continue
		default:
			return dir, verbose, args[i:], nil
		}
	}
	return dir, verbose, args[len(args):], nil
}

// domainInitFn and langFromFn are seams for white-box tests: they let the
// Run ordering contract (version bypass → domain.Init → langFrom) be
// asserted without re-initialising the domain package or the locale.
var (
	domainInitFn = domain.Init
	langFromFn   = langFrom
)

// isKnownLangFn is a seam for white-box tests: it reports whether a language
// dictionary was actually registered by domain.Init, letting the default
// language selection be asserted against a degraded (RU-missing) registry
// that the cli package cannot construct directly.
var isKnownLangFn = domain.IsKnownLang

// stdinReader is the source of the details text for `-d -` (read the
// details from stdin). It is a seam for white-box tests: they replace it
// with a fixture reader and restore the previous value afterwards.
var stdinReader io.Reader = os.Stdin

func Run(args []string, env map[string]string, stdout, stderr io.Writer) int {
	flagDir, verbose, args, err := extractGlobalFlags(args)
	if err != nil {
		// Missing/empty -C/--tickets-dir is a one-line user error: print and
		// exit 1 BEFORE any command routing (incl. help/version/unknown) —
		// matches the dispatch/paths error style.
		fmt.Fprintln(stderr, err)
		return exitError
	}
	cmd := "help"
	if len(args) > 0 {
		cmd = args[0]
	}
	// Version bypass runs before i18n initialisation and language
	// resolution: `ticket version` must keep working even if the embedded
	// dictionaries are broken, and it must not consult the locale at all.
	switch cmd {
	case "version", "--version", "-v":
		_, _ = io.WriteString(stdout, versionLine())
		return exitOK
	}

	// i18n initialisation precedes langFrom and every usage path. A fatal
	// failure prints exactly one plain line and exits 1 before the locale is
	// resolved or any template is touched; a degraded failure warns once and
	// continues (help still exits 0, unknown languages fall back to EN).
	if err := domainInitFn(); err != nil {
		fmt.Fprintln(stderr, err)
		if !errors.Is(err, domain.ErrInitDegraded) {
			return exitError
		}
	}

	lang := langFromFn(env)
	switch cmd {
	case "init":
		if len(args) > 1 {
			if args[1] == "-h" || args[1] == "--help" {
				return printUsage(stdout, stderr, lang)
			}
			printUsage(stdout, stderr, lang)
			return exitError
		}
		return cmdInit(lang, stdout, stderr)
	case "new", "list", "show", "set", "archive", "recover", "normalize":
		// Subcommand help interception (T-0041): -h/--help as the FIRST
		// argument is never a valid positional (new: title; show/set/archive/
		// recover: ticket number; list: filter), so print usage and exit 0. Only
		// args[1] is checked: flag values (-d text, -P project) and the
		// subsequent positionals (incl. the set comment) all land in args[2+],
		// so a --help there must NOT trigger help ("new Тикет -d \"see
		// --help\"" stays a real ticket creation).
		if len(args) > 1 && (args[1] == "-h" || args[1] == "--help") {
			return printUsage(stdout, stderr, lang)
		}
		return runLogged(cmd, args[1:], env, lang, flagDir, verbose, stdout, stderr)
	case "help", "-h", "--help":
		return printUsage(stdout, stderr, lang)
	default:
		printUsage(stdout, stderr, lang)
		return exitError
	}
}

// runLogged runs one scoped command with its own logger: the logger opens
// first, the command lifecycle is logged around the dispatch, and a close
// error is reported after the command output without changing the exit code.
// The logger stays off unless --verbose or a non-empty TICKET_LOG is present.
func runLogged(cmd string, args []string, env map[string]string, lang domain.Lang, flagDir string, verbose bool, stdout, stderr io.Writer) int {
	lg, closeLog, err := newLoggerFn(stderr, env, verbose)
	if err != nil {
		_, _ = io.WriteString(stderr, err.Error()+"\n")
		return exitError
	}
	defer func() {
		if cerr := closeLog(); cerr != nil {
			_, _ = io.WriteString(stderr, cerr.Error()+"\n")
		}
	}()
	lg.Info("command.start", "command", cmd)
	return dispatch(cmd, args, env, lang, flagDir, lg, stdout, stderr)
}

// dispatch resolves the tickets dir once using flag, environment, then cwd.
// lang is passed to handlers that print usage or i18n messages
// (new/show/set/list/archive).
func dispatch(cmd string, args []string, env map[string]string, lang domain.Lang, flagDir string, lg *slog.Logger, stdout, stderr io.Writer) int {
	// Getwd/Executable failures are handled by paths.Resolve input checks.
	cwd, _ := os.Getwd()
	src := paths.Source(env, flagDir)
	dir, err := paths.Resolve(env, cwd, flagDir)
	if err != nil {
		lg.Error("dir.resolve.fail", "source", src)
		fmt.Fprintln(stderr, err)
		return exitError
	}
	lg.Debug("dir.resolve", "source", src, "dir", dir)
	// The normalize command parses its own arguments here, before the store is opened:
	// --dry-run must not take the write probe (it also works on a read-only
	// tickets dir), so the parse result decides which Store to open. This is
	// the only parse site — cmdNormalize receives the flags, never the args.
	var nf normalizeFlags
	if cmd == "normalize" {
		var msg string
		if nf, msg = parseNormalizeFlags(args, lang); msg != "" {
			fmt.Fprintln(stderr, msg)
			return exitError
		}
	}
	// Only mutating commands open the Store writable (write probe); list/show
	// and normalize --dry-run use NewReadOnly.
	var st *store.Store
	if cmd == "new" || cmd == "set" || cmd == "archive" || cmd == "recover" || cmd == "normalize" && !nf.dryRun {
		st, err = store.New(dir, store.WithLogger(lg))
	} else { // list, show, normalize --dry-run
		st, err = store.NewReadOnly(dir, store.WithLogger(lg))
	}
	if err != nil {
		lg.Error("store.open", "dir", dir)
		// store errors are user-facing and self-prefixed ("store: ...");
		// adding die()'s "ticket: " here would double-prefix (wave-1 #1).
		fmt.Fprintln(stderr, err)
		return exitError
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
		return exitError
	}
	project := filepath.Base(filepath.Dir(dir))
	var code int
	switch cmd {
	case "new":
		code = cmdNew(st, args, who, project, lang, stdout, stderr)
	case "list":
		code = cmdList(st, args, who, project, lang, stdout, stderr)
	case "show":
		code = cmdShow(st, args, who, project, lang, stdout, stderr)
	case "set":
		code = cmdSet(st, args, who, project, lang, stdout, stderr)
	case "recover":
		code = cmdRecover(st, args, who, project, lang, stdout, stderr)
	case "normalize":
		code = cmdNormalize(st, nf, lang, stdout, stderr)
	default: // "archive"
		code = cmdArchive(st, args, who, project, lang, stdout, stderr)
	}
	lg.Info("command.done", "command", cmd, "code", code)
	return code
}
