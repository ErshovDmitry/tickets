package main

// §7.4 build matrix: CGO_ENABLED=0 cross-builds of the six canonical
// targets into dist/. Gated behind TICKETS_CROSS_BUILD=1 so the ordinary
// `go test ./...` run stays hermetic and never writes into the repo.

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// distTarget is one build-matrix cell: GOOS/GOARCH and the canonical
// dist/ artifact name. Canonical windows/amd64 = ticket.exe; windows/arm64
// auxiliary = ticket-windows-arm64.exe; the rest = ticket-<os>-<arch>.
type distTarget struct {
	goos, goarch, out string
}

// distTargets is the full §7.4 six-target matrix.
var distTargets = []distTarget{
	{"linux", "amd64", "ticket-linux-amd64"},
	{"linux", "arm64", "ticket-linux-arm64"},
	{"darwin", "amd64", "ticket-darwin-amd64"},
	{"darwin", "arm64", "ticket-darwin-arm64"},
	{"windows", "amd64", "ticket.exe"},
	{"windows", "arm64", "ticket-windows-arm64.exe"},
}

// TestCrossBuildDist cross-compiles every target with CGO_ENABLED=0 and
// verifies a non-empty artifact at the canonical dist/ path.
func TestCrossBuildDist(t *testing.T) {
	if os.Getenv("TICKETS_CROSS_BUILD") == "" {
		t.Skip("TICKETS_CROSS_BUILD not set; skipping the §7.4 dist/ build matrix")
	}
	// dist/ is gitignored and a clean checkout has none; go build -o does
	// not create the artifact's parent directory itself.
	if err := os.MkdirAll(filepath.Join("..", "..", "dist"), 0o755); err != nil {
		t.Fatalf("create dist/: %v", err)
	}
	for _, tgt := range distTargets {
		t.Run(tgt.goos+"/"+tgt.goarch, func(t *testing.T) {
			out := filepath.Join("..", "..", "dist", tgt.out)
			cmd := exec.Command("go", "build", "-o", out, ".")
			cmd.Env = append(os.Environ(),
				"CGO_ENABLED=0",
				"GOOS="+tgt.goos,
				"GOARCH="+tgt.goarch,
			)
			if msg, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("go build %s/%s: %v\n%s", tgt.goos, tgt.goarch, err, msg)
			}
			info, err := os.Stat(out)
			if err != nil {
				t.Fatalf("artifact %s: %v", out, err)
			}
			if info.Size() == 0 {
				t.Fatalf("artifact %s is empty", out)
			}
		})
	}
}
