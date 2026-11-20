package webembed

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

func FS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
