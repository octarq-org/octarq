// Command openapi-gen prints octarq's OpenAPI specification to stdout.
//
// The document is generated from the live handler registrations (see package
// openapi), so it describes exactly what the binary serves. This tool composes
// the open-source Core plugin set only — a distribution composing extra plugins
// can generate its spec via that distribution's openapi command.
//
// CI regenerates website/public/openapi.json with `go run . openapi` and fails
// the build when the committed artifact differs, so this is also the command to
// run locally before pushing a route change:
//
//	go run . openapi > website/public/openapi.json
package main

import (
	"fmt"
	"os"

	"github.com/octarq-org/octarq/openapi"
	"github.com/octarq-org/octarq/plugins/builtin"
)

func main() {
	if err := openapi.Generate(os.Stdout, builtin.Default()); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating spec: %v\n", err)
		os.Exit(1)
	}
}
