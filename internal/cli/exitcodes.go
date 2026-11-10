package cli

// Exit codes returned by cli.Run and the cmd* handlers.
//
// The CLI historically knew only two codes: 0 for success and 1 for every
// failure (usage errors, not-found, store errors, collisions). T-0161 gives
// both universal codes names — exitOK and exitError — so production code no
// longer carries bare 0/1 literals; the numbers themselves are unchanged.
// T-0157 adds a third, distinct code for a ticket number owned by more than
// one file: the caller must be able to tell «the number does not identify
// exactly one ticket» apart from a generic error so automation can react
// (e.g. surface the conflict for manual resolution instead of retrying).
const (
	// exitOK is returned when the command completed successfully.
	exitOK = 0

	// exitError is returned for any generic failure: usage error, missing
	// ticket, store error or an operation refused by the CLI.
	exitError = 1

	// exitAmbiguous is returned when more than one ticket file carries the
	// requested number (T-0157). Every conflicting absolute path is printed
	// to stderr; no mutation is performed.
	exitAmbiguous = 2
)
