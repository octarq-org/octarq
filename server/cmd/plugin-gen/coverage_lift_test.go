package main

import (
	"os"
	"testing"
)

func TestCoverageLiftMain(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"plugin-gen", "--help"}
	main()
}
