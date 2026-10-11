package main

import (
	"os"
	"testing"
)

func TestCoverageLiftMain(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Calling main with --help executes newRootCmd().Execute() successfully without os.Exit(1)
	os.Args = []string{"plugin-gen", "--help"}
	main()
}
