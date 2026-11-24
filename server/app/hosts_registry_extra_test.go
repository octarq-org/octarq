package app

import (
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

func TestHostUniqueViolationDetection(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"duplicated key", gorm.ErrDuplicatedKey, true},
		{"sqlite", errors.New("UNIQUE constraint failed: org_hosts.host"), true},
		{"postgres", errors.New(`duplicate key value violates unique constraint "idx_org_hosts"`), true},
		{"generic", errors.New("database is locked"), false},
	}
	for _, tc := range cases {
		if got := isHostUniqueViolation(tc.err); got != tc.want {
			t.Errorf("%s: = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestNormalizeBindHostRejectsMalformedInput(t *testing.T) {
	db := openHostRegistryDB(t)
	for _, raw := range []string{"", "a..b", "localhost", "127.0.0.1", "single", "bad host", "https://acme.example"} {
		if got, err := normalizeBindHost(db, raw); err == nil {
			t.Errorf("normalizeBindHost(%q) = %q, want error", raw, got)
		}
	}
	if got, err := normalizeBindHost(db, "acme.example"); err != nil || got != "acme.example" {
		t.Errorf("normalizeBindHost(acme.example) = (%q, %v)", got, err)
	}
	if err := db.Save(&models.Setting{Key: "base_domain", Value: "acme.example"}).Error; err != nil {
		t.Fatalf("save base domain: %v", err)
	}
	if _, err := normalizeBindHost(db, "acme.example"); err == nil {
		t.Error("normalizeBindHost accepted the configured base domain apex")
	}
}
