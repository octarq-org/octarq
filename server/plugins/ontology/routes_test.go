package ontology

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

// TestRoleForRequestReadsTheHandlerContext pins the one thing that decides
// whether an API client can use these routes at all.
//
// The huma middleware authenticates the request and passes the resulting
// context to the handler; the *http.Request the adapter returns still carries
// the context it arrived with. A cookie session hides the difference, because
// the auth manager falls back to reading the cookie — a bearer token has no
// cookie, so asking the host about the unwrapped request answers "no role" and
// every generated route refuses the agent that is supposed to use it.
func TestRoleForRequestReadsTheHandlerContext(t *testing.T) {
	session := &plugin.TestSession{
		OrgRoleFn: func(r *http.Request) string {
			if plugin.OrgIDFromContext(r.Context()) == 0 {
				return ""
			}
			return "member"
		},
	}
	p := New()
	p.Mount(nil, &plugin.Context{Host: &plugin.TestHost{SessionMock: session}})

	// The handler context carries the workspace the middleware resolved; the
	// request itself carries nothing, exactly as it arrives from the adapter.
	handlerCtx := plugin.WithOrgID(context.Background(), 7)
	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)

	role, err := p.roleForRequest(handlerCtx, req)
	if err != nil {
		t.Fatalf("roleForRequest = %v, want the role from the handler context", err)
	}
	if role != "member" {
		t.Fatalf("role = %q, want member", role)
	}

	// Without the handler context there is no role to find, and the refusal is
	// the same one an authenticated non-member gets.
	if _, err := p.roleForRequest(context.Background(), req); err == nil {
		t.Fatal("roleForRequest without a workspace context = nil, want a refusal")
	}
}
