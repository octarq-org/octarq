package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestOpenAPICommandStdoutIsParseableJSON guards the defect where
// `octarq openapi > spec.json` wrote a file that was not JSON: the command boots
// the app to read the live handler registrations, and GORM's default logger
// wrote "record not found" to os.Stdout, interleaved with the document.
//
// It builds and executes the real binary rather than calling run() in-process,
// and that is the whole point of the test. Two in-process formulations were
// tried first and both stayed green with the fix reverted — the polluting query
// only fires against the working directory's own database, so a test that calls
// run() with a temp environment never reaches it. An assertion that cannot fail
// is worse than no assertion, because it reads as coverage.
//
// The failure is silent by construction: go build, go vet and the command's own
// exit code are all clean while the artifact is garbage. Only parsing catches it.
func TestOpenAPICommandStdoutIsParseableJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary; skipped under -short")
	}

	bin := filepath.Join(t.TempDir(), "octarq-openapi-test")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, "openapi")
	cmd.Dir = "." // the repo dir, whose database is what triggered the logging
	cmd.Stderr = os.NewFile(0, os.DevNull)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run openapi: %v", err)
	}

	var doc struct {
		Paths map[string]any `json:"paths"`
	}
	if jsonErr := json.Unmarshal(out, &doc); jsonErr != nil {
		head := string(out)
		if len(head) > 300 {
			head = head[:300]
		}
		t.Fatalf("stdout is not valid JSON (%v); first bytes:\n%s", jsonErr, head)
	}
	if len(doc.Paths) < 100 {
		t.Errorf("spec has %d paths, want >= 100 — the generator regressed toward the old hand-written document", len(doc.Paths))
	}
	if strings.Contains(string(out), "record not found") {
		t.Error("stdout carries a GORM log line")
	}
}
