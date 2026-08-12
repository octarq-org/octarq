package api

import (
	"reflect"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestSplitList(t *testing.T) {
	cases := map[string][]string{
		"":                         nil,
		"a,b,c":                    {"a", "b", "c"},
		"a\nb\nc":                  {"a", "b", "c"},
		" A , a ,B ":               {"a", "b"}, // lowercased + de-duped + trimmed
		"go\tlogin pricing":        {"go", "login", "pricing"},
		"x,,,y":                    {"x", "y"}, // empty fields dropped
		"Admin\nadmin\nPOSTMASTER": {"admin", "postmaster"},
	}
	for in, want := range cases {
		got := splitList(in)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("splitList(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestRequireEmailVerificationDefaultOn pins the require_email_verification
// semantics: absent setting → on, only an explicit "false" turns it off. The
// old `== "true"` default-off behavior let sign-up hand a session to an
// unverified email on a fresh instance — the exact multi-tenant abuse vector
// this flag exists to close.
func TestRequireEmailVerificationDefaultOn(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)

	if !h.requireEmailVerification() {
		t.Fatal("absent setting: requireEmailVerification() = false, want true (default on)")
	}
	if err := db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "false"}).Error; err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if h.requireEmailVerification() {
		t.Fatal("explicit \"false\": requireEmailVerification() = true, want false")
	}
	if err := db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "true"}).Error; err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if !h.requireEmailVerification() {
		t.Fatal("explicit \"true\": requireEmailVerification() = false, want true")
	}
}
