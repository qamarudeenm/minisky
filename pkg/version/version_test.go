package version

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The version must not be a literal anyone has to remember to bump. v1.4.2
// shipped reporting 1.4.1 precisely because it was one, and release.sh never
// touched it — so every bug report against that build was ambiguous.
func TestVersionIsNotHardcodedToAReleaseNumber(t *testing.T) {
	if regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(Version) {
		t.Errorf("Version is the literal %q; it must default to a dev placeholder "+
			"and be injected from the git tag at build time", Version)
	}
	if IsRelease() {
		t.Error("an uninjected build must not claim to be a release")
	}
}

// Guard the mechanism itself: if the ldflags go missing, releases quietly start
// reporting "dev" and we are back to a version nobody can trust.
func TestEveryReleaseBuildInjectsTheVersion(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(root, ".goreleaser.yaml"))
	if err != nil {
		t.Skipf("no .goreleaser.yaml to check: %v", err)
	}

	text := string(config)
	builds := strings.Count(text, "main: ./cmd/minisky")
	injections := strings.Count(text, "-X minisky/pkg/version.Version={{.Version}}")

	if builds == 0 {
		t.Fatal("found no builds in .goreleaser.yaml")
	}
	if injections != builds {
		t.Errorf("%d build(s) declared but only %d inject the version; "+
			"every platform must report the tag it was built from", builds, injections)
	}
}
