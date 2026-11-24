package api

import (
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/config"
)

func TestExportSignedTokenRefusesAnEmptyWorkspace(t *testing.T) {
	h := &Handler{cfg: &config.Config{SecretKey: strings.Repeat("k", 32)}}
	if _, err := h.GenerateExportSignedToken(0, time.Minute); err == nil {
		t.Fatal("token for org 0: got no error")
	}
}

func TestExportSignedTokenAndURL(t *testing.T) {
	h := &Handler{cfg: &config.Config{SecretKey: strings.Repeat("k", 32)}}

	tok, err := h.GenerateExportSignedToken(5, 0)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if tok == "" {
		t.Fatal("token with zero lifetime is empty")
	}

	link, err := h.GenerateExportSignedURL(5)
	if err != nil {
		t.Fatalf("url: %v", err)
	}
	if !strings.HasPrefix(link, "/api/account/export?token=") {
		t.Fatalf("url = %q, want the export path with a token", link)
	}
}
