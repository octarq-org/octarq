package main

import (
	"fmt"
	"os"

	"github.com/octarq-org/octarq/server/openapi"
	"github.com/octarq-org/octarq/server/plugins/builtin"
)

func main() {
	if err := openapi.Generate(os.Stdout, builtin.Default()); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating spec: %v\n", err)
		os.Exit(1)
	}
}
