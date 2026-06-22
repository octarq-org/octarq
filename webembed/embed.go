// Package webembed embeds the built React dashboard into the binary.
//
// The dist directory is produced by `pnpm --dir web build`. A placeholder
// index.html is committed so the package always builds, even before the first
// frontend build.
package webembed

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the dist directory rooted at its top level.
func FS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
