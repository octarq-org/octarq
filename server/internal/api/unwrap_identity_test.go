package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

// A plugin route reads its caller through humago.Unwrap. That call peels the
// auth middleware's huma context override back off and hands back the adapter's
// own http.Request, whose context is the anonymous one the server received.
//
// A cookie survived that: auth.UserID falls back to re-reading the session
// cookie from the header. A bearer token had no such fallback, so the token
// authenticated at the middleware and then read as user 0 inside every plugin
// route — which is how POST /api/llm-gateway/models answered 401 to a valid
// admin token while /api/auth/me answered 200 to the same token.

type unwrapIdentityIn struct {
	Ctx huma.Context `hidden:"true"`
}

// Resolve is how every plugin route receives the huma context; huma does not
// fill a huma.Context field on its own.
func (i *unwrapIdentityIn) Resolve(c huma.Context) []error { i.Ctx = c; return nil }

type unwrapIdentityOut struct {
	Body struct {
		UserID uint `json:"userId"`
		Admin  bool `json:"admin"`
	}
}

// TestPluginRouteSeesBearerIdentity registers the shape every plugin route
// uses — huma.Context in, humago.Unwrap, then the host's session reads — and
// proves a bearer token carries the same identity there as it does on a core
// route.
func TestPluginRouteSeesBearerIdentity(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	huma.Register(h.Huma(), huma.Operation{
		OperationID: "test-unwrap-identity",
		Method:      http.MethodGet,
		Path:        "/api/test-unwrap-identity",
	}, func(_ context.Context, in *unwrapIdentityIn) (*unwrapIdentityOut, error) {
		r, _ := humago.Unwrap(in.Ctx)
		out := &unwrapIdentityOut{}
		out.Body.UserID = h.auth.UserID(r)
		out.Body.Admin = h.RequireRole(r, "admin")
		return out, nil
	})

	const raw = "oct_unwrapidentity0000000000000000001"
	seedMember(t, db, 9, "admin")
	seedToken(t, db, raw, 9, "admin")

	rec := bearer(srv, http.MethodGet, "/api/test-unwrap-identity", raw)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		UserID uint `json:"userId"`
		Admin  bool `json:"admin"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body = %s)", err, rec.Body.String())
	}
	if got.UserID != 9 {
		t.Errorf("a plugin route read user id %d from a bearer token held by user 9", got.UserID)
	}
	if !got.Admin {
		t.Error("a plugin route refused the admin role the same bearer token carries")
	}
}

// TestPluginRouteStillRefusesAnUnknownToken keeps the fail-closed direction:
// stamping the request must not turn an invalid token into an identity.
func TestPluginRouteStillRefusesAnUnknownToken(t *testing.T) {
	h, srv, _ := newTestHandlerRaw(t)

	huma.Register(h.Huma(), huma.Operation{
		OperationID: "test-unwrap-identity-refused",
		Method:      http.MethodGet,
		Path:        "/api/test-unwrap-identity-refused",
	}, func(_ context.Context, in *unwrapIdentityIn) (*unwrapIdentityOut, error) {
		r, _ := humago.Unwrap(in.Ctx)
		out := &unwrapIdentityOut{}
		out.Body.UserID = h.auth.UserID(r)
		return out, nil
	})

	rec := bearer(srv, http.MethodGet, "/api/test-unwrap-identity-refused", "oct_neverminted000000000000000000001")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (body = %s)", rec.Code, rec.Body.String())
	}
}
