package ontology

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

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

	handlerCtx := plugin.WithOrgID(context.Background(), 7)
	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)

	role, err := p.roleForRequest(handlerCtx, req)
	if err != nil {
		t.Fatalf("roleForRequest = %v, want the role from the handler context", err)
	}
	if role != "member" {
		t.Fatalf("role = %q, want member", role)
	}

	if _, err := p.roleForRequest(context.Background(), req); err == nil {
		t.Fatal("roleForRequest without a workspace context = nil, want a refusal")
	}
}
