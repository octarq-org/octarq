package api

import (
	"runtime/debug"
	"testing"
)

func TestBuildInfoFieldsReportsReleaseVersion(t *testing.T) {
	got := buildInfoFields("v0.9.1", nil)
	if got.Version != "v0.9.1" {
		t.Errorf("version = %q, want v0.9.1", got.Version)
	}
	if got.Revision != "" {
		t.Errorf("revision = %q, want empty when the build carries no VCS stamp", got.Revision)
	}
}

func TestBuildInfoFieldsDropsDevelopmentVersion(t *testing.T) {
	for _, version := range []string{"", "   ", "(devel)"} {
		got := buildInfoFields(version, nil)
		if got.Version != "" {
			t.Errorf("version %q reported as %q, want empty", version, got.Version)
		}
	}
}

func TestBuildInfoFieldsTruncatesRevision(t *testing.T) {
	const full = "a5447d8a7cb4deadbeefdeadbeefdeadbeefdead"
	got := buildInfoFields("", []debug.BuildSetting{
		{Key: "vcs.revision", Value: full},
		{Key: "vcs.modified", Value: "false"},
	})
	if len(got.Revision) != revisionLength {
		t.Fatalf("revision = %q, want %d characters", got.Revision, revisionLength)
	}
	if got.Revision != full[:revisionLength] {
		t.Errorf("revision = %q, want the first %d characters of %q", got.Revision, revisionLength, full)
	}
}

func TestBuildInfoFieldsKeepsShortRevision(t *testing.T) {
	got := buildInfoFields("", []debug.BuildSetting{{Key: "vcs.revision", Value: "abc1234"}})
	if got.Revision != "abc1234" {
		t.Errorf("revision = %q, want abc1234", got.Revision)
	}
}
