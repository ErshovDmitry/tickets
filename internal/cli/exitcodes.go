package cli

// Exit codes returned by cli.Run and the cmd* handlers.
//
// The CLI historically knew only two codes: 0 for success and 1 for every
// failure (usage errors, not-found, store errors, collisions). T-0157 adds
// a distinct code for a ticket number owned by more than one file: the
// caller must be able to tell «the number does not identify exactly one
// ticket» apart from a generic error so automation can react (e.g. surface
// the conflict for manual resolution instead of retrying).
const (
	// exitAmbiguous is returned when more than one ticket file carries the
	// requested number (T-0157). Every conflicting absolute path is printed
	// to stderr; no mutation is performed.
	exitAmbiguous = 2
)
