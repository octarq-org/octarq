package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/octarq-org/octarq/openapi"
)

func main() {
	outDir := "../octarq-pro/web/public"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	outPath := filepath.Join(outDir, "openapi.json")
	f, err := os.Create(outPath)
	if err != nil {
		fmt.Printf("Error creating openapi.json: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	// In the standalone generator, no plugins are passed (nil)
	if err := openapi.Generate(f, nil); err != nil {
		fmt.Printf("Error generating spec: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("OpenAPI specification written successfully to: %s\n", outPath)
}
