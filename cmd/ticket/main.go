// Command ticket is the entrypoint of the tickets CLI.
// It wires the process environment into the cli package and exits
// with the code returned by cli.Run (§6 signature contract).
package main

import (
	"os"
	"strings"

	"ticket/internal/cli"
)

func main() {
	env := make(map[string]string, len(os.Environ()))
	for _, e := range os.Environ() {
		if i := strings.IndexByte(e, '='); i >= 0 {
			env[e[:i]] = e[i+1:]
		}
	}
	code := cli.Run(os.Args[1:], env, os.Stdout, os.Stderr)
	os.Exit(code)
}
