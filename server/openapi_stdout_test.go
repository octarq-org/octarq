package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
	cmd.Dir = "."
	cmd.Env = append(os.Environ(), "OCTARQ_LISTEN=127.0.0.1:0")
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
	if len(doc.Paths) < 90 {
		t.Errorf("spec has %d paths, want >= 90 — the generator regressed toward the old hand-written document", len(doc.Paths))
	}
	if strings.Contains(string(out), "record not found") {
		t.Error("stdout carries a GORM log line")
	}
}
