package cli

// version is the ticket version reported by the version command. The default
// "dev" is replaced at link time with the release tag via
// -ldflags "-X ticket/internal/cli.version=X.Y.Z". No commit hash or build
// date: YAGNI.
var version = "dev"

// versionLine returns the single version line printed by the version command
// and as the first line of usage.
func versionLine() string {
	return "ticket version " + version + "\n"
}
