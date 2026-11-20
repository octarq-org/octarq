package buildinfo

var Version = "dev"

var Commit = "unknown"

var BuiltAt = "unknown"

type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"builtAt"`
}

func Get() Info {
	return Info{Version: Version, Commit: Commit, BuiltAt: BuiltAt}
}
