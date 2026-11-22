package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/octarq-org/octarq/server/plugins/llmgateway"
)

func main() {
	outPath := flag.String("out", "", "output path")
	srcURL := flag.String("src", "https://models.dev/api.json", "models.dev source url")
	minModels := flag.Int("min", 4000, "minimum models count")
	flag.Parse()

	if *outPath == "" {
		fmt.Fprintln(os.Stderr, "error: -out is required")
		os.Exit(1)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(*srcURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching %s: %v\n", *srcURL, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "error: fetch returned HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading response: %v\n", err)
		os.Exit(1)
	}

	gzData, err := llmgateway.BundleFromModelsDev(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error bundling models: %v\n", err)
		os.Exit(1)
	}

	cat, err := llmgateway.ParseBundleGz(gzData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing bundle: %v\n", err)
		os.Exit(1)
	}

	count := cat.ModelCount()
	if count < *minModels {
		fmt.Fprintf(os.Stderr, "error: models count %d < min %d\n", count, *minModels)
		os.Exit(1)
	}

	if len(gzData) >= 200*1024 {
		fmt.Fprintf(os.Stderr, "error: bundle size %d >= 200KB\n", len(gzData))
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(*outPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*outPath, gzData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing output file: %v\n", err)
		os.Exit(1)
	}
}
