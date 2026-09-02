// Package cli implements the ticket command-line interface: argument
// routing, tickets-dir resolution, and shared wiring for the commands.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"ticket/internal/paths"
	"ticket/internal/store"
)

// Run dispatches args[0] to the command handler and returns the process
// exit code. args must not include argv[0]: main passes os.Args[1:], unit
// callers pass the command name and its arguments directly. No command
// given means help (bash:147 cmd="${1:-help}"); an unknown command prints
// usage to stdout and exits 1 (bash:154).
//
// Command handlers (cmd_new.go, cmd_list.go, cmd_show.go, cmd_set.go) share
// the uniform signature
//
//	func cmdX(st *store.Store, args []string, who, project string, stdout, stderr io.Writer) int
//
// where st is built here from the tickets dir resolved ONCE, who is the
// resolved user (TICKET_WHO → USER → USERNAME → agent, bash:12) and
// project is filepath.Base(filepath.Dir(dir)) (bash:10).
func Run(args []string, env map[string]string, stdout, stderr io.Writer) int {
	cmd := "help"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "new", "list", "show", "set", "archive":
		return dispatch(cmd, args[1:], env, stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	default:
		usage(stdout)
		return 1
	}
}

// dispatch resolves the tickets dir once and invokes the command handler.
func dispatch(cmd string, args []string, env map[string]string, stdout, stderr io.Writer) int {
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
		return cmdNew(st, args, who, project, stdout, stderr)
	case "list":
		return cmdList(st, args, who, project, stdout, stderr)
	case "show":
		return cmdShow(st, args, who, project, stdout, stderr)
	case "set":
		return cmdSet(st, args, who, project, stdout, stderr)
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
