package api

import (
	"runtime/debug"
	"strings"
)

// revisionLength is how much of a commit hash the status page shows. Twelve
// characters identify a commit in every git UI, and the short form keeps the
// public status answer small.
const revisionLength = 12

// BuildInfo names the binary that answers a request. A build that carries no
// module version and no VCS stamp (for example a local `go build
// -buildvcs=false`) reports empty values, so the status page hides the line
// instead of printing a guess.
type BuildInfo struct {
	Version  string `json:"version,omitempty"`
	Revision string `json:"revision,omitempty"`
}

// ReadBuildInfo reports the version and revision of the running binary. The Go
// toolchain stamps both into the binary, so a deployed instance can answer
// which build it runs without a configuration value.
func ReadBuildInfo() BuildInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return BuildInfo{}
	}
	return buildInfoFields(info.Main.Version, info.Settings)
}

// buildInfoFields turns the main module version and the toolchain settings
// into the reported pair. "(devel)" means the build came from a working
// directory rather than a released module, so it is not a version.
func buildInfoFields(mainVersion string, settings []debug.BuildSetting) BuildInfo {
	var out BuildInfo
	if v := strings.TrimSpace(mainVersion); v != "" && v != "(devel)" {
		out.Version = v
	}
	for _, s := range settings {
		if s.Key != "vcs.revision" {
			continue
		}
		rev := strings.TrimSpace(s.Value)
		if len(rev) > revisionLength {
			rev = rev[:revisionLength]
		}
		out.Revision = rev
	}
	return out
}
