// Package buildinfo carries build-time metadata (version, commit, built-at)
// injected via -ldflags -X. The values live in their own package rather than
// package main so any consumer — the API handler that serves them, a CLI flag,
// a plugin — can read them without an import cycle.
//
// The defaults are deliberate: building outside a git checkout (e.g. from a
// release tarball) must produce explicit placeholders, never empty strings,
// and must never fail the build.
package buildinfo

// Version is the release tag when HEAD is exactly tagged, otherwise the short
// commit hash. Defaults to "dev" when built outside a git checkout.
var Version = "dev"

// Commit is the short commit hash the binary was built from. Defaults to
// "unknown" when built outside a git checkout.
var Commit = "unknown"

// BuiltAt is the UTC build timestamp (RFC3339). Defaults to "unknown" when it
// cannot be determined.
var BuiltAt = "unknown"

// Info is the authenticated instance-build payload.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"builtAt"`
}

// Get returns the build metadata for this binary.
func Get() Info {
	return Info{Version: Version, Commit: Commit, BuiltAt: BuiltAt}
}
