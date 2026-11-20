package api

import (
	"runtime/debug"
	"strings"
)

const revisionLength = 12

type BuildInfo struct {
	Version  string `json:"version,omitempty"`
	Revision string `json:"revision,omitempty"`
}

func ReadBuildInfo() BuildInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return BuildInfo{}
	}
	return buildInfoFields(info.Main.Version, info.Settings)
}

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
