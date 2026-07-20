package main

import "github.com/octarq-org/octarq/plugin"

// customPlugins returns extra third-party backend plugins composed into this
// build, on top of the OSS core. The committed default is empty so a plain
// `go build` ships no extra plugins.
//
// It is the backend half of the build-time composition seam: `make plugin-build`
// (see cmd/octarq-build) REGENERATES this file from the OCTARQ_PLUGINS manifest,
// adding an aliased import + `&Plugin{}` entry per plugin. Do not edit by hand
// when using that tooling — it overwrites the whole file.
//
// Convention: every Octarq plugin's Go package exports a `Plugin` type that
// satisfies plugin.Plugin, so the generator can wire any plugin by import path
// alone (see the octarq-plugin-template).
func customPlugins() []plugin.Plugin { return nil }
