// Command led is a single-binary domain / short-link / email management
// service (link · email · domain). It serves an embedded React dashboard,
// a JSON API, and a short-link redirector from one process.
//
// This is the open-core binary: it runs the app with no Pro plugins. The
// commercial build (private led-core module) reuses the same app package and
// registers additional plugins before Run — see the plugin package.
package main

import (
	"context"
	"log"

	"github.com/Jungley8/led/app"
)

func main() {
	a, err := app.New()
	if err != nil {
		log.Fatalf("init: %v", err)
	}
	if err := a.Run(context.Background()); err != nil {
		log.Fatalf("run: %v", err)
	}
}
