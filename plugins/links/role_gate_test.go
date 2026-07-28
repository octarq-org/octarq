package links

import (
	"net/http"
	"testing"
)

// The delete gate must refuse when the host never wired RequireRole.
//
// The check used to read "if p.requireRole != nil && !p.requireRole(...)", which
// waves the caller through on an unwired seam — so a host that forgot to pass
// RequireRole would silently lose every role check in this plugin while looking
// perfectly healthy. For a gate protecting destructive operations, absence of an
// answer has to mean no.
func TestRoleGateFailsClosedWithoutSeam(t *testing.T) {
	p := &Plugin{} // requireRole nil, as an unwired host leaves it
	if p.hasRole(&http.Request{}, "admin") {
		t.Fatal("role gate passed with no RequireRole wired; an unwired host must be refused, not trusted")
	}
}

func TestRoleGateDelegatesWhenWired(t *testing.T) {
	var askedFor string
	p := &Plugin{requireRole: func(_ *http.Request, min string) bool {
		askedFor = min
		return min == "admin"
	}}
	if !p.hasRole(&http.Request{}, "admin") {
		t.Fatal("role gate refused an admin caller")
	}
	if askedFor != "admin" {
		t.Fatalf("gate asked for role %q, want admin", askedFor)
	}
	if p.hasRole(&http.Request{}, "owner") {
		t.Fatal("role gate granted owner to an admin-only caller")
	}
}
