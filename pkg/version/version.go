package version

// Version is the running MiniSky version.
//
// It is deliberately not the real number. The release build injects the git tag
// with -ldflags "-X minisky/pkg/version.Version=<tag>", so the tag is the single
// place a version is set and a build can never disagree with the tag it was cut
// from.
//
// v1.4.2 shipped reporting 1.4.1 because this was a literal that release.sh
// never updated, which made every bug report ambiguous about what was actually
// running.
var Version = "dev"

// IsRelease reports whether this binary came from a release build rather than a
// local `go build`.
func IsRelease() bool { return Version != "dev" }
