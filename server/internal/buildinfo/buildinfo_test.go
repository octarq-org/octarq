package buildinfo_test

import (
	"testing"

	"github.com/octarq-org/octarq/internal/buildinfo"
)

func TestGet(t *testing.T) {
	info := buildinfo.Get()
	if info.Version != buildinfo.Version {
		t.Fatalf("expected version %q, got %q", buildinfo.Version, info.Version)
	}
	if info.Commit != buildinfo.Commit {
		t.Fatalf("expected commit %q, got %q", buildinfo.Commit, info.Commit)
	}
	if info.BuiltAt != buildinfo.BuiltAt {
		t.Fatalf("expected builtAt %q, got %q", buildinfo.BuiltAt, info.BuiltAt)
	}
}
